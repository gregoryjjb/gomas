package main

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/betamos/zeroconf"
	"github.com/stretchr/testify/require"
)

func slaveEvent(op zeroconf.Op, name string, port uint16, ips ...string) zeroconf.Event {
	service := zeroconf.NewService(zeroconf.NewType(slaveService), name, port)
	service.Hostname = "test-host.local"
	for _, ip := range ips {
		service.Addrs = append(service.Addrs, netip.MustParseAddr(ip))
	}
	return zeroconf.Event{Op: op, Service: service}
}

func TestSlaveRegistry(t *testing.T) {
	var registry SlaveRegistry
	first := netip.MustParseAddr("192.168.1.10")
	second := netip.MustParseAddr("192.168.1.11")
	const roof = "roof._gomas._tcp.local"
	const fence = "fence._gomas._tcp.local"
	entry := slaveEvent(zeroconf.OpAdded, "roof", 1225, first.String(), second.String())
	registry.observe(entry)
	snapshot := registry.Snapshot()
	require.Len(t, snapshot, 1, "multiple IPs belong to one service")
	require.Equal(t, Slave{Hostname: "test-host.local", Addrs: []netip.Addr{first, second}, Port: 1225}, snapshot[roof])
	entry.Text = []string{"hostname=lights-b"}
	registry.observe(entry)
	require.Equal(t, "lights-b", registry.Snapshot()[roof].Hostname)

	// Neither the callback's addresses nor a snapshot may expose the live slice.
	entry.Addrs[0] = netip.MustParseAddr("192.168.1.99")
	snapshot[roof].Addrs[1] = netip.MustParseAddr("192.168.1.98")
	delete(snapshot, roof)
	require.Equal(t, []netip.Addr{first, second}, registry.Snapshot()[roof].Addrs)

	// Changing addresses and ports updates the existing service entry.
	registry.observe(slaveEvent(zeroconf.OpUpdated, "roof", 1226, "::ffff:192.168.1.11", second.String()))
	snapshot = registry.Snapshot()
	require.Len(t, snapshot, 1)
	require.Equal(t, Slave{Hostname: "test-host.local", Addrs: []netip.Addr{second}, Port: 1226}, snapshot[roof])

	registry.observe(slaveEvent(zeroconf.OpAdded, "fence", 1225, first.String()))
	// Expiry and goodbye callbacks contain the service name but no addresses or port.
	registry.observe(slaveEvent(zeroconf.OpRemoved, "roof", 0))
	snapshot = registry.Snapshot()
	require.NotContains(t, snapshot, roof)
	require.Contains(t, snapshot, fence, "other services must remain")
	registry.observe(slaveEvent(zeroconf.OpRemoved, "roof", 0))
	require.Len(t, registry.Snapshot(), 1, "repeated removals are harmless")

	registry.observe(slaveEvent(zeroconf.OpAdded, "roof", 1225, second.String()))
	require.Len(t, registry.Snapshot(), 2, "a returning slave is rediscovered")
	registry.observe(slaveEvent(zeroconf.OpUpdated, "roof", 1225))
	require.NotContains(t, registry.Snapshot(), roof, "an empty address update clears the old service")
}

func TestSlaveRegistrySharedIP(t *testing.T) {
	var registry SlaveRegistry
	registry.observe(slaveEvent(zeroconf.OpAdded, "roof", 1225, "192.168.1.10"))
	registry.observe(slaveEvent(zeroconf.OpAdded, "fence", 1226, "192.168.1.10"))
	snapshot := registry.Snapshot()
	require.Len(t, snapshot, 2, "services sharing an IP remain distinct")
	require.Equal(t, 1225, snapshot["roof._gomas._tcp.local"].Port)
	require.Equal(t, 1226, snapshot["fence._gomas._tcp.local"].Port)
	registry.observe(slaveEvent(zeroconf.OpRemoved, "roof", 0))
	snapshot = registry.Snapshot()
	require.Len(t, snapshot, 1)
	require.Contains(t, snapshot, "fence._gomas._tcp.local")
	registry.observe(slaveEvent(zeroconf.OpRemoved, "fence", 0))
	require.Empty(t, registry.Snapshot())
}

func TestSlaveRegistryIgnoresInvalidEntries(t *testing.T) {
	var registry SlaveRegistry
	registry.observe(zeroconf.Event{})
	registry.observe(slaveEvent(zeroconf.OpAdded, "roof", 0, "192.168.1.10"))
	registry.observe(slaveEvent(zeroconf.OpAdded, "", 1225, "192.168.1.10"))
	registry.observe(slaveEvent(zeroconf.Op(99), "roof", 1225, "192.168.1.10"))
	registry.observe(slaveEvent(zeroconf.OpAdded, "roof", 1225, "0.0.0.0", "127.0.0.1", "224.0.0.251", "::1", "2001:db8::1"))
	require.Empty(t, registry.Snapshot())
}

func TestSlaveRegistryConcurrentAccess(t *testing.T) {
	var registry SlaveRegistry
	var workers sync.WaitGroup
	for i := range 8 {
		workers.Go(func() {
			name := fmt.Sprintf("slave-%d", i)
			entry := slaveEvent(zeroconf.OpAdded, name, 1225, fmt.Sprintf("192.168.1.%d", i+1))
			for range 10 {
				registry.observe(entry)
				registry.Snapshot()
				registry.observe(slaveEvent(zeroconf.OpRemoved, name, 0))
			}
			registry.observe(entry)
		})
	}
	workers.Wait()
	require.Len(t, registry.Snapshot(), 8)
}

func TestSlaveDiscoveryRoles(t *testing.T) {
	var registry SlaveRegistry
	stop, err := registry.Start(t.Context(), RoleStandalone, nil)
	require.NoError(t, err)
	stop()
	stop()
	require.Nil(t, registry.Snapshot())
	_, err = registry.Start(t.Context(), "invalid", nil)
	require.Error(t, err)
	for _, addr := range []*net.TCPAddr{nil, {Port: 0}, {IP: net.ParseIP("127.0.0.1"), Port: 1225}} {
		_, err := registry.Start(t.Context(), RoleSlave, addr)
		require.Error(t, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = registry.Start(ctx, RoleMaster, nil)
	require.ErrorIs(t, err, context.Canceled)
}

func multicastListener(t *testing.T) *net.TCPListener {
	t.Helper()
	if os.Getenv("GOMAS_TEST_MDNS") != "1" {
		t.Skip("set GOMAS_TEST_MDNS=1 to exercise local multicast")
	}
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{})
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })
	return listener
}

func hasSlavePort(registry *SlaveRegistry, port int) bool {
	for _, entry := range registry.Snapshot() {
		if entry.Port == port {
			return true
		}
	}
	return false
}

// Real multicast depends on host interfaces and firewall rules, so keep it opt-in.
func TestSlaveMulticast(t *testing.T) {
	listener := multicastListener(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var slave, master SlaveRegistry
	stopMaster, err := master.Start(ctx, RoleMaster, nil)
	require.NoError(t, err)
	defer stopMaster()
	stopSlave, err := slave.Start(ctx, RoleSlave, listener.Addr().(*net.TCPAddr))
	require.NoError(t, err)
	defer stopSlave()
	port := listener.Addr().(*net.TCPAddr).Port
	require.Eventually(t, func() bool { return hasSlavePort(&master, port) }, 15*time.Second, 100*time.Millisecond)
	require.Nil(t, slave.Snapshot(), "slaves keep no peer registry")
	stopSlave()
	require.Eventually(t, func() bool { return !hasSlavePort(&master, port) }, 5*time.Second, 100*time.Millisecond,
		"goodbyes must remove the service without waiting for its original TTL")
	cancel()
	stopMaster()
	stopSlave()
}

func TestSlaveMulticastTTL(t *testing.T) {
	listener := multicastListener(t)
	port := listener.Addr().(*net.TCPAddr).Port
	name := fmt.Sprintf("gomas-ttl-test-%d", port)
	service := zeroconf.NewService(zeroconf.NewType(slaveService), name, uint16(port))
	service.Hostname = name + ".local"
	var offline atomic.Bool
	publisher, err := zeroconf.New().Network("udp4").
		Interfaces(func() ([]net.Interface, error) {
			if offline.Load() {
				return nil, nil
			}
			return net.Interfaces()
		}).Publish(service).Open()
	require.NoError(t, err)
	defer publisher.Close()

	var master SlaveRegistry
	var removals atomic.Int32
	// Shorten TTL only in this integration test. Production uses advertised TTLs.
	browser, err := zeroconf.New().Network("udp4").Expiry(3*time.Second).
		Browse(func(event zeroconf.Event) {
			if event.Name == name {
				if event.Op == zeroconf.OpRemoved {
					removals.Add(1)
				}
				master.observe(event)
			}
		}, zeroconf.NewType(slaveService)).Open()
	require.NoError(t, err)
	defer browser.Close()
	require.Eventually(t, func() bool { return hasSlavePort(&master, port) }, 10*time.Second, 100*time.Millisecond)
	// Stay online across several TTLs and gaps between unsolicited announcements.
	// Library refresh queries must keep the service present without removal/add churn.
	time.Sleep(12 * time.Second)
	require.EqualValues(t, 0, removals.Load(), "live services must be refreshed before expiry")
	require.True(t, hasSlavePort(&master, port))

	// Drop all interfaces without closing the publisher: no goodbye is sent.
	offline.Store(true)
	publisher.Reload()
	require.Eventually(t, func() bool { return !hasSlavePort(&master, port) }, 6*time.Second, 100*time.Millisecond,
		"an unresponsive service must expire using the library's TTL cache")
	require.EqualValues(t, 1, removals.Load())
}
