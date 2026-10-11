package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func commandTestSlave(server *httptest.Server) Slave {
	addr := server.Listener.Addr().(*net.TCPAddr)
	ip, _ := netip.AddrFromSlice(addr.IP)
	return Slave{Addrs: []netip.Addr{ip.Unmap()}, Port: addr.Port}
}

func TestSlaveRegistryCommands(t *testing.T) {
	requests := make(chan *url.URL, 8)
	newServer := func() *httptest.Server {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests <- r.URL
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(server.Close)
		return server
	}
	registry := &SlaveRegistry{masterPort: 1225, slaves: map[string]Slave{
		"roof":  commandTestSlave(newServer()),
		"fence": commandTestSlave(newServer()),
	}}
	startedAt := time.UnixMicro(123456789)
	for _, tc := range []struct {
		name  string
		send  func()
		path  string
		query url.Values
	}{
		{"play", func() { registry.Play(t.Context(), "A & B + snow", strings.Repeat("ab", 32), startedAt) }, "/api/playSlave", url.Values{
			"name": {"A & B + snow"}, "slaveStartTimeMicro": {"123456789"},
			"sha256": {strings.Repeat("ab", 32)}, "masterPort": {"1225"},
		}},
		{"on", func() { registry.Static(t.Context(), true) }, "/api/static", url.Values{"value": {"1"}}},
		{"off", func() { registry.Static(t.Context(), false) }, "/api/static", url.Values{"value": {"0"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.send()
			for range 2 {
				select {
				case request := <-requests:
					require.Equal(t, tc.path, request.Path)
					require.Equal(t, tc.query, request.Query())
				case <-time.After(3 * time.Second):
					t.Fatal("missing slave command")
				}
			}
		})
	}
}

func TestSlaveCommandAddressFallback(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	slave := commandTestSlave(server)
	// No listener on this loopback address; the second address must be tried.
	slave.Addrs = append([]netip.Addr{netip.MustParseAddr("127.0.0.2")}, slave.Addrs...)
	require.NoError(t, sendSlaveCommand(t.Context(), slave, "/api/static", "value=1"))
	require.EqualValues(t, 1, calls.Load())
}

func TestSlaveCommandSendsOnceAfterConnecting(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusInternalServerError, http.StatusTemporaryRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Location", "/another-command")
				w.WriteHeader(status)
			}))
			defer server.Close()
			slave := commandTestSlave(server)
			slave.Addrs = append(slave.Addrs, slave.Addrs[0])
			err := sendSlaveCommand(t.Context(), slave, "/api/playSlave", "")
			if status == http.StatusNoContent {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, http.StatusText(status))
			}
			require.EqualValues(t, 1, calls.Load(), "HTTP errors and redirects must not resend the command")
		})
	}
}

func TestSlaveRegistrySlowPeerDoesNotBlockOthers(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	slowStarted := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(slowStarted)
		<-r.Context().Done()
	}))
	defer slow.Close()
	// Cancel before Close waits for the deliberately stalled request.
	defer cancel()
	fastCalled := make(chan struct{})
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(fastCalled)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer fast.Close()
	registry := &SlaveRegistry{slaves: map[string]Slave{
		"slow": commandTestSlave(slow), "fast": commandTestSlave(fast),
	}}
	registry.Static(ctx, true)
	for _, received := range []chan struct{}{slowStarted, fastCalled} {
		select {
		case <-received:
		case <-time.After(3 * time.Second):
			t.Fatal("one slave blocked another")
		}
	}
}

func TestSlaveCommandCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- sendSlaveCommand(ctx, commandTestSlave(server), "/api/static", "") }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not reach slave")
	}
	cancel()
	select {
	case err := <-result:
		require.True(t, errors.Is(err, context.Canceled), "%v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("request did not cancel")
	}
}
