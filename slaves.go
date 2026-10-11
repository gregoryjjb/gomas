package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/betamos/zeroconf"
	"github.com/rs/zerolog/log"
)

const slaveService = "_gomas._tcp"

// Slave describes the addresses and HTTP port of a discovered service.
type Slave struct {
	Addrs []netip.Addr
	Port  int
}

// SlaveRegistry is keyed by full DNS-SD service instance name.
// Only the master discovers peers and sends commands. Its zero value is usable.
type SlaveRegistry struct {
	mu         sync.RWMutex
	slaves     map[string]Slave
	masterPort int
}

// Snapshot returns a copy of the currently discovered slaves.
func (s *SlaveRegistry) Snapshot() map[string]Slave {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot := maps.Clone(s.slaves)
	for service, slave := range snapshot {
		slave.Addrs = slices.Clone(slave.Addrs)
		snapshot[service] = slave
	}
	return snapshot
}

// Play tells every discovered slave to join the show at the master's start time.
func (s *SlaveRegistry) Play(ctx context.Context, id, hash string, startTime time.Time) {
	s.mu.RLock()
	masterPort := s.masterPort
	s.mu.RUnlock()
	if masterPort == 0 {
		log.Error().Msg("Cannot notify slaves before the master's HTTP listener is ready")
		return
	}
	s.send(ctx, "/api/playSlave", url.Values{
		"name":                {id},
		"sha256":              {hash},
		"masterPort":          {strconv.Itoa(masterPort)},
		"slaveStartTimeMicro": {MarshalStartTime(startTime)},
	})
}

// Static tells every discovered slave to stop playback and set its outputs.
func (s *SlaveRegistry) Static(ctx context.Context, state bool) {
	value := "0"
	if state {
		value = "1"
	}
	s.send(ctx, "/api/static", url.Values{"value": {value}})
}

func (s *SlaveRegistry) send(ctx context.Context, path string, query url.Values) {
	encoded := query.Encode()
	for service, slave := range s.Snapshot() {
		go func() {
			if err := sendSlaveCommand(ctx, slave, path, encoded); err != nil {
				log.Error().Err(err).Str("service", service).Str("path", path).
					Msg("Failed to notify slave")
			}
		}()
	}
}

// Try alternative addresses only while connecting. Once connected, an HTTP
// failure must not replay a command that the slave may already have processed.
func sendSlaveCommand(ctx context.Context, slave Slave, path, query string) error {
	if len(slave.Addrs) == 0 {
		return errors.New("slave has no addresses")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	transport := &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			dialer := net.Dialer{Timeout: time.Second}
			var failures []error
			for _, addr := range slave.Addrs {
				endpoint := netip.AddrPortFrom(addr, uint16(slave.Port)).String()
				conn, err := dialer.DialContext(ctx, network, endpoint)
				if err == nil {
					return conn, nil
				}
				failures = append(failures, err)
				if ctx.Err() != nil {
					break
				}
			}
			return nil, errors.Join(failures...)
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	u := url.URL{
		Scheme:   "http",
		Host:     netip.AddrPortFrom(slave.Addrs[0], uint16(slave.Port)).String(),
		Path:     path,
		RawQuery: query,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slave returned %s", resp.Status)
	}
	_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return err
}

// Start enables discovery for the configured role. The returned stop function
// cancels discovery and waits for its resources to close. Call once per registry.
// Slaves advertise the bound HTTP address and keep no registry of peers.
func (s *SlaveRegistry) Start(ctx context.Context, role Role, address *net.TCPAddr) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var client *zeroconf.Client
	var err error
	switch role {
	case RoleStandalone:
		return func() {}, nil
	case RoleMaster:
		// Keep one browser running. It backs off discovery queries and refreshes
		// records according to their advertised TTLs, reporting expiry/goodbyes.
		client, err = zeroconf.New().Network("udp4").
			Browse(s.observe, zeroconf.NewType(slaveService)).Open()
	case RoleSlave:
		client, err = advertiseSlave(address)
	default:
		return nil, fmt.Errorf("invalid discovery role %q", role)
	}
	if err != nil {
		return nil, err
	}

	if role == RoleMaster && address != nil {
		s.mu.Lock()
		s.masterPort = address.Port
		s.mu.Unlock()
	}

	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		if err := client.Close(); err != nil {
			log.Error().Err(err).Msg("Closing slave discovery")
		}
	}()
	return func() { cancel(); <-done }, nil
}

func (s *SlaveRegistry) observe(event zeroconf.Event) {
	if event.Service == nil || event.Name == "" {
		return
	}
	switch event.Op {
	case zeroconf.OpAdded, zeroconf.OpUpdated:
		if event.Port == 0 {
			return
		}
	case zeroconf.OpRemoved:
	default:
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	service := event.Service.String()
	if event.Op == zeroconf.OpRemoved {
		delete(s.slaves, service)
		log.Info().Str("service", service).Msg("Slave disappeared")
		return
	}
	var addrs []netip.Addr
	for _, ip := range event.Addrs {
		ip = ip.Unmap()
		if !ip.Is4() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLoopback() {
			continue
		}
		addrs = append(addrs, ip)
	}
	if len(addrs) == 0 {
		delete(s.slaves, service)
		return
	}
	slices.SortFunc(addrs, netip.Addr.Compare)
	addrs = slices.Compact(addrs)
	if s.slaves == nil {
		s.slaves = make(map[string]Slave)
	}
	// Updates replace the complete address set without changing the service key.
	s.slaves[service] = Slave{Addrs: addrs, Port: int(event.Port)}
	log.Info().Str("service", service).Msg("Slave discovery updated")
}

func advertiseSlave(address *net.TCPAddr) (*zeroconf.Client, error) {
	if address == nil || address.Port < 1 || address.Port > 65535 {
		return nil, fmt.Errorf("slave discovery requires a bound HTTP port")
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var selected []net.Interface
	var ips []netip.Addr
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		found := false
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil || ip.To4() == nil || ip.IsLoopback() || ip.IsUnspecified() {
				continue
			}
			if len(address.IP) != 0 && !address.IP.IsUnspecified() && !address.IP.Equal(ip) {
				continue
			}
			parsed, _ := netip.AddrFromSlice(ip)
			ips = append(ips, parsed.Unmap())
			found = true
		}
		if found {
			selected = append(selected, iface)
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("slave discovery requires HTTP listening on a LAN IPv4 address (check HOST)")
	}
	host := "gomas-" + strings.ReplaceAll(ips[0].String(), ".", "-")
	service := zeroconf.NewService(zeroconf.NewType(slaveService),
		fmt.Sprintf("%s-%d", host, address.Port), uint16(address.Port))
	service.Hostname = host + ".local"
	// For a wildcard HTTP listener let the library advertise each interface's
	// own addresses. A listener bound to one IP must advertise only that IP.
	if len(address.IP) != 0 && !address.IP.IsUnspecified() {
		service.Addrs = ips[:1]
	}
	return zeroconf.New().Network("udp4").
		Interfaces(func() ([]net.Interface, error) { return selected, nil }).
		Publish(service).Open()
}
