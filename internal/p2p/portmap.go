package p2p

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"time"
)

const (
	natPMPPort    = 5351
	leaseSeconds  = 1800
	renewInterval = 15 * time.Minute
	probeTimeout  = 2 * time.Second
)

type mappingLease struct {
	protocol     string
	externalIP   net.IP
	externalPort int
	release      func(context.Context)
}

type portMapper struct {
	mu          sync.Mutex
	internalUDP int
	enabled     bool
	status      PortMapStatus
	externalIP  string
	cancel      context.CancelFunc
	done        chan struct{}
	onChange    func(externalIP string, status PortMapStatus)
	// gatewayOverride and upnpOverride allow deterministic unit tests without
	// touching LAN routers.
	gatewayOverride net.IP
	upnpOverride    string
}

func newPortMapper(internalUDP int, onChange func(string, PortMapStatus)) *portMapper {
	return &portMapper{
		internalUDP: internalUDP,
		status:      PortMapStatus{State: "off"},
		onChange:    onChange,
	}
}

func (m *portMapper) Status() (PortMapStatus, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status, m.externalIP
}

func (m *portMapper) SetEnabled(enabled bool) {
	m.mu.Lock()
	if m.enabled == enabled {
		m.mu.Unlock()
		return
	}
	m.enabled = enabled
	prevCancel := m.cancel
	prevDone := m.done
	m.cancel = nil
	m.done = nil
	if !enabled {
		m.status = PortMapStatus{State: "off"}
		m.externalIP = ""
	} else {
		m.status = PortMapStatus{State: "probing"}
	}
	st := m.status
	m.mu.Unlock()

	if prevCancel != nil {
		prevCancel()
		<-prevDone
	}
	if m.onChange != nil {
		m.onChange("", st)
	}
	if enabled && m.internalUDP > 0 {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		m.mu.Lock()
		if !m.enabled {
			m.mu.Unlock()
			cancel()
			return
		}
		m.cancel = cancel
		m.done = done
		m.mu.Unlock()
		go m.loop(ctx, done)
	}
}

func (m *portMapper) Close() {
	m.SetEnabled(false)
}

func (m *portMapper) loop(ctx context.Context, done chan struct{}) {
	defer close(done)
	var active *mappingLease
	defer func() {
		if active != nil && active.release != nil {
			relCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			active.release(relCtx)
			cancel()
		}
	}()

	apply := func() {
		lease, err := m.acquireOnce(ctx)
		m.mu.Lock()
		if !m.enabled {
			m.mu.Unlock()
			if lease != nil && lease.release != nil {
				lease.release(context.Background())
			}
			return
		}
		if err != nil || lease == nil {
			active = nil
			m.externalIP = ""
			m.status = PortMapStatus{State: "unavailable"}
		} else {
			active = lease
			m.externalIP = ""
			if lease.externalIP != nil && lease.externalPort == m.internalUDP && isPublicIP(lease.externalIP) {
				m.externalIP = lease.externalIP.String()
			}
			m.status = PortMapStatus{
				State:        "mapped",
				Protocol:     lease.protocol,
				ExternalPort: lease.externalPort,
			}
		}
		ext := m.externalIP
		st := m.status
		m.mu.Unlock()
		if m.onChange != nil {
			m.onChange(ext, st)
		}
	}

	apply()
	ticker := time.NewTicker(renewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			apply()
		}
	}
}

func (m *portMapper) acquireOnce(ctx context.Context) (*mappingLease, error) {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	for _, gw := range m.candidateGateways() {
		if lease, err := mapPCP(probeCtx, gw, m.internalUDP); err == nil {
			return lease, nil
		}
		if lease, err := mapNATPMP(probeCtx, gw, m.internalUDP); err == nil {
			return lease, nil
		}
	}
	if lease, err := m.mapUPnP(probeCtx); err == nil {
		return lease, nil
	}
	return nil, errors.New("no local gateway port mapping available")
}

func (m *portMapper) candidateGateways() []net.IP {
	m.mu.Lock()
	override := m.gatewayOverride
	m.mu.Unlock()
	if override != nil {
		return []net.IP{override}
	}
	seen := map[string]bool{}
	var out []net.IP
	if conn, err := net.Dial("udp4", "1.1.1.1:80"); err == nil {
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			if ip4 := addr.IP.To4(); ip4 != nil && ip4.IsPrivate() {
				gw := net.IPv4(ip4[0], ip4[1], ip4[2], 1)
				seen[gw.String()] = true
				out = append(out, gw)
			}
		}
		_ = conn.Close()
	}
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipNet.IP.To4()
			if ip4 == nil || !ip4.IsPrivate() {
				continue
			}
			gw := net.IPv4(ip4[0], ip4[1], ip4[2], 1)
			if !seen[gw.String()] {
				seen[gw.String()] = true
				out = append(out, gw)
			}
		}
	}
	return out
}

func isPublicIP(ip net.IP) bool {
	return ip != nil && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified()
}

// mapPCP sends an RFC 6887 MAP request over UDP/5351.
func mapPCP(ctx context.Context, gw net.IP, internalPort int) (*mappingLease, error) {
	return mapPCPWithLifetime(ctx, &net.UDPAddr{IP: gw, Port: natPMPPort}, internalPort, leaseSeconds)
}

func mapPCPWithLifetime(ctx context.Context, target *net.UDPAddr, internalPort int, lifetime uint32) (*mappingLease, error) {
	conn, err := net.DialUDP("udp4", nil, target)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return nil, errors.New("pcp: missing local udp addr")
	}

	var nonce [12]byte
	_, _ = rand.Read(nonce[:])

	req := make([]byte, 60)
	req[0] = 2 // PCP version 2
	req[1] = 1 // Opcode MAP
	binary.BigEndian.PutUint32(req[4:8], lifetime)
	req[18], req[19] = 0xff, 0xff
	copy(req[20:24], localAddr.IP.To4())
	copy(req[24:36], nonce[:])
	req[36] = 17 // UDP
	binary.BigEndian.PutUint16(req[40:42], uint16(internalPort))
	binary.BigEndian.PutUint16(req[42:44], uint16(internalPort))
	req[54], req[55] = 0xff, 0xff

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(600 * time.Millisecond)
	}
	_ = conn.SetDeadline(deadline)
	if _, err := conn.Write(req); err != nil {
		return nil, err
	}
	resp := make([]byte, 128)
	n, err := conn.Read(resp)
	if err != nil || n < 60 || resp[0] != 2 || resp[1] != 0x81 || resp[3] != 0 {
		return nil, errors.New("pcp mapping rejected")
	}
	if !bytes.Equal(resp[24:36], nonce[:]) {
		return nil, errors.New("pcp nonce mismatch")
	}
	extPort := int(binary.BigEndian.Uint16(resp[42:44]))
	var extIP net.IP
	if resp[54] == 0xff && resp[55] == 0xff {
		extIP = net.IP(append([]byte(nil), resp[56:60]...))
	} else {
		extIP = net.IP(append([]byte(nil), resp[44:60]...))
	}
	if lifetime == 0 {
		return &mappingLease{protocol: "pcp"}, nil
	}
	addrCopy := *target
	return &mappingLease{
		protocol:     "pcp",
		externalIP:   extIP,
		externalPort: extPort,
		release: func(relCtx context.Context) {
			_, _ = mapPCPWithLifetime(relCtx, &addrCopy, internalPort, 0)
		},
	}, nil
}

// mapNATPMP sends RFC 6886 external-address + UDP mapping requests on UDP/5351.
func mapNATPMP(ctx context.Context, gw net.IP, internalPort int) (*mappingLease, error) {
	return mapNATPMPAddr(ctx, &net.UDPAddr{IP: gw, Port: natPMPPort}, internalPort, leaseSeconds)
}

func mapNATPMPAddr(ctx context.Context, target *net.UDPAddr, internalPort int, lifetime uint32) (*mappingLease, error) {
	conn, err := net.DialUDP("udp4", nil, target)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(600 * time.Millisecond)
	}
	_ = conn.SetDeadline(deadline)

	var extIP net.IP
	if lifetime > 0 {
		if _, err := conn.Write([]byte{0, 0}); err == nil {
			buf := make([]byte, 16)
			if n, err := conn.Read(buf); err == nil && n >= 12 && buf[0] == 0 && buf[1] == 128 && binary.BigEndian.Uint16(buf[2:4]) == 0 {
				extIP = net.IPv4(buf[8], buf[9], buf[10], buf[11])
			}
		}
	}

	req := make([]byte, 12)
	req[0] = 0
	req[1] = 1
	binary.BigEndian.PutUint16(req[4:6], uint16(internalPort))
	binary.BigEndian.PutUint16(req[6:8], uint16(internalPort))
	binary.BigEndian.PutUint32(req[8:12], lifetime)
	if _, err := conn.Write(req); err != nil {
		return nil, err
	}
	resp := make([]byte, 16)
	n, err := conn.Read(resp)
	if err != nil || n < 16 || resp[0] != 0 || resp[1] == 0 || resp[1] != 129 || binary.BigEndian.Uint16(resp[2:4]) != 0 {
		return nil, errors.New("nat-pmp mapping rejected")
	}
	extPort := int(binary.BigEndian.Uint16(resp[10:12]))
	if lifetime == 0 {
		return &mappingLease{protocol: "nat-pmp"}, nil
	}
	addrCopy := *target
	return &mappingLease{
		protocol:     "nat-pmp",
		externalIP:   extIP,
		externalPort: extPort,
		release: func(relCtx context.Context) {
			_, _ = mapNATPMPAddr(relCtx, &addrCopy, internalPort, 0)
		},
	}, nil
}
