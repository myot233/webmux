//go:build !nop2p

package p2p

import (
	"context"
	"net"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
)

const (
	defaultMaxPeers    = 8
	defaultMaxChannels = 32
	defaultIdleTimeout = 2 * time.Minute
	gatherWaitTimeout  = 1500 * time.Millisecond
	maxSignalPerMinute = 40
)

type peerEntry struct {
	id        string
	pc        *webrtc.PeerConnection
	ctx       context.Context
	cancel    context.CancelFunc
	createdAt time.Time
	lastSeen  atomic.Int64
	channels  atomic.Int32

	mu            sync.Mutex
	candidateType string
	ipFamily      string
	rttMs         *int
	pendingLocal  []ICECandidate
}

func (p *peerEntry) touch() {
	p.lastSeen.Store(time.Now().UnixNano())
}

func (p *peerEntry) tryIncChannel(limit int) bool {
	for {
		cur := p.channels.Load()
		if int(cur) >= limit {
			return false
		}
		if p.channels.CompareAndSwap(cur, cur+1) {
			return true
		}
	}
}

func (p *peerEntry) decChannel() {
	p.channels.Add(-1)
}

func (p *peerEntry) selectedCandidateMeta() (string, string) {
	p.mu.Lock()
	ct, ipf := p.candidateType, p.ipFamily
	p.mu.Unlock()
	if ct != "" {
		return ct, ipf
	}
	sctp := p.pc.SCTP()
	if sctp == nil || sctp.Transport() == nil || sctp.Transport().ICETransport() == nil {
		return "", ""
	}
	pair, err := sctp.Transport().ICETransport().GetSelectedCandidatePair()
	if err != nil || pair == nil || pair.Local == nil || pair.Remote == nil {
		return "", ""
	}
	fam := "ipv4"
	if strings.Contains(pair.Remote.Address, ":") {
		fam = "ipv6"
	}
	return pair.Local.Typ.String() + "/" + pair.Remote.Typ.String(), fam
}

// Service manages WebRTC PeerConnections, UDP port mapping, and stream/RPC
// routing for WebMux.
type Service struct {
	appVersion    string
	udpPortCfg    int
	ifaceFilter   *regexp.Regexp
	maxPeers      int
	maxChannels   int
	idleTimeout   time.Duration
	streamHandler StreamHandler
	rpcHandler    RPCHandler

	mu          sync.Mutex
	closed      bool
	cfg         Config
	peers       map[string]*peerEntry
	signalTimes []time.Time

	udpMux   ice.UDPMux
	udpConns []net.PacketConn
	mapper   *portMapper
	mappedIP string

	reaperCancel context.CancelFunc
	reaperDone   chan struct{}
}

// New constructs and starts the P2P service.
func New(opts Options) *Service {
	maxPeers := opts.MaxPeers
	if maxPeers <= 0 {
		maxPeers = defaultMaxPeers
	}
	maxCh := opts.MaxChannels
	if maxCh <= 0 {
		maxCh = defaultMaxChannels
	}
	idle := opts.IdleTimeout
	if idle <= 0 {
		idle = defaultIdleTimeout
	}
	var ifaceRE *regexp.Regexp
	if opts.InterfaceFilter != "" {
		ifaceRE, _ = regexp.Compile(opts.InterfaceFilter)
	}
	s := &Service{
		appVersion:    opts.AppVersion,
		udpPortCfg:    opts.UDPPort,
		ifaceFilter:   ifaceRE,
		maxPeers:      maxPeers,
		maxChannels:   maxCh,
		idleTimeout:   idle,
		streamHandler: opts.StreamHandler,
		rpcHandler:    opts.RPCHandler,
		cfg: Config{
			Enabled:     opts.Enabled,
			PortMapping: opts.PortMapping,
			STUNServers: slices.Clone(opts.STUNServers),
		},
		peers:      map[string]*peerEntry{},
		reaperDone: make(chan struct{}),
	}
	s.initUDPMux()
	ctx, cancel := context.WithCancel(context.Background())
	s.reaperCancel = cancel
	go s.reapLoop(ctx)
	if opts.Enabled && opts.PortMapping && s.mapper != nil {
		s.mapper.SetEnabled(true)
	}
	return s
}

func (s *Service) initUDPMux() {
	udpPort := s.udpPortCfg
	if udpPort <= 0 {
		udpPort = probeFreeUDPPort()
	}

	var muxes []ice.UDPMux
	seen := map[string]struct{}{}
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if !s.allowInterface(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, raw := range addrs {
			ip := extractInterfaceIP(raw)
			if !usableCandidateIP(ip) {
				continue
			}
			if iface.Flags&net.FlagLoopback != 0 && !ip.IsLoopback() {
				continue
			}
			netw := "udp6"
			bindIP := ip
			if ip4 := ip.To4(); ip4 != nil {
				netw = "udp4"
				bindIP = ip4
			}
			key := netw + "|" + bindIP.String()
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}

			conn, listenErr := net.ListenUDP(netw, &net.UDPAddr{IP: bindIP, Port: udpPort})
			if listenErr != nil && s.udpPortCfg <= 0 {
				conn, listenErr = net.ListenUDP(netw, &net.UDPAddr{IP: bindIP, Port: 0})
			}
			if listenErr != nil {
				continue
			}
			if udpPort <= 0 {
				if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
					udpPort = addr.Port
				}
			}
			s.udpConns = append(s.udpConns, conn)
			muxes = append(muxes, webrtc.NewICEUDPMux(nil, conn))
		}
	}

	if len(muxes) == 1 {
		s.udpMux = muxes[0]
	} else if len(muxes) > 1 {
		s.udpMux = ice.NewMultiUDPMuxDefault(muxes...)
	}
	if udpPort > 0 {
		s.mapper = newPortMapper(udpPort, func(extIP string, _ PortMapStatus) {
			s.mu.Lock()
			s.mappedIP = extIP
			s.mu.Unlock()
		})
	}
}

func (s *Service) allowInterface(name string) bool {
	if s.ifaceFilter != nil {
		return s.ifaceFilter.MatchString(name)
	}
	for _, prefix := range []string{"docker", "br-", "veth", "virbr", "cni", "podman"} {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return true
}

func probeFreeUDPPort() int {
	for _, netw := range []string{"udp4", "udp6"} {
		c, err := net.ListenUDP(netw, &net.UDPAddr{Port: 0})
		if err != nil {
			continue
		}
		addr, _ := c.LocalAddr().(*net.UDPAddr)
		_ = c.Close()
		if addr != nil && addr.Port > 0 {
			return addr.Port
		}
	}
	return 0
}

func extractInterfaceIP(addr net.Addr) net.IP {
	switch v := addr.(type) {
	case *net.IPNet:
		return v.IP
	case *net.IPAddr:
		return v.IP
	default:
		return nil
	}
}

func usableCandidateIP(ip net.IP) bool {
	if len(ip) == 0 || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if ip.To4() != nil {
		return true
	}
	if len(ip) != net.IPv6len {
		return false
	}
	// Skip deprecated IPv6 site-local unicast (fec0::/10).
	return ip[0] != 0xfe || ip[1]&0xc0 != 0xc0
}

func (s *Service) mapperStatus() (PortMapStatus, string) {
	if s.mapper == nil {
		return PortMapStatus{State: "off"}, ""
	}
	return s.mapper.Status()
}

// Supported reports whether WebRTC P2P is compiled into this binary.
func (s *Service) Supported() bool { return true }

// Enabled reports whether opportunistic P2P direct upgrades are enabled.
func (s *Service) Enabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed && s.cfg.Enabled
}

// Reconfigure updates the runtime P2P configuration. Turning P2P off
// immediately closes active PeerConnections and releases any port mapping.
func (s *Service) Reconfigure(next Config) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.cfg = Config{
		Enabled:     next.Enabled,
		PortMapping: next.PortMapping,
		STUNServers: slices.Clone(next.STUNServers),
	}
	var toClose []*peerEntry
	if !next.Enabled {
		for id, p := range s.peers {
			toClose = append(toClose, p)
			delete(s.peers, id)
		}
	}
	mapper := s.mapper
	wantMap := next.Enabled && next.PortMapping
	s.mu.Unlock()

	for _, p := range toClose {
		p.cancel()
		_ = p.pc.Close()
	}
	if mapper != nil {
		mapper.SetEnabled(wantMap)
	}
}

// Close shuts down all PeerConnections, UDP sockets, and port mappings.
func (s *Service) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	peers := make([]*peerEntry, 0, len(s.peers))
	for id, p := range s.peers {
		peers = append(peers, p)
		delete(s.peers, id)
	}
	mapper := s.mapper
	udpMux := s.udpMux
	conns := s.udpConns
	s.mu.Unlock()

	s.reaperCancel()
	<-s.reaperDone
	if mapper != nil {
		mapper.Close()
	}
	for _, p := range peers {
		p.cancel()
		_ = p.pc.Close()
	}
	if udpMux != nil {
		_ = udpMux.Close()
	}
	for _, c := range conns {
		_ = c.Close()
	}
}

// Status returns sanitized diagnostics for active peers and port mapping.
func (s *Service) Status() Status {
	pmStatus, _ := s.mapperStatus()
	s.mu.Lock()
	bindings := make([]UDPBinding, 0, len(s.udpConns))
	if !s.closed {
		for _, conn := range s.udpConns {
			if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
				network := "udp6"
				if addr.IP.To4() != nil {
					network = "udp4"
				}
				bindings = append(bindings, UDPBinding{Network: network, Port: addr.Port})
			}
		}
	}

	entries := make([]*peerEntry, 0, len(s.peers))
	for _, p := range s.peers {
		entries = append(entries, p)
	}
	s.mu.Unlock()

	peers := make([]PeerInfo, 0, len(entries))
	for _, p := range entries {
		ct, fam := p.selectedCandidateMeta()
		p.mu.Lock()
		rtt := p.rttMs
		p.mu.Unlock()
		peers = append(peers, PeerInfo{
			ID:            p.id,
			State:         p.pc.ConnectionState().String(),
			CandidateType: ct,
			IPFamily:      fam,
			RTTMs:         rtt,
			Channels:      int(max(0, p.channels.Load())),
			CreatedAt:     p.createdAt.UTC().Format(time.RFC3339),
		})
	}
	slices.SortFunc(peers, func(a, b PeerInfo) int {
		return strings.Compare(a.ID, b.ID)
	})
	return Status{
		Supported:   true,
		UDPPort:     s.udpPortCfg,
		UDPBindings: bindings,
		PortMap:     pmStatus,
		ActivePeers: len(peers),
		Peers:       peers,
	}
}

func (s *Service) reapLoop(ctx context.Context) {
	defer close(s.reaperDone)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			var stale []*peerEntry
			s.mu.Lock()
			for id, p := range s.peers {
				st := p.pc.ConnectionState()
				last := time.Unix(0, p.lastSeen.Load())
				connectingExpired := (st == webrtc.PeerConnectionStateNew || st == webrtc.PeerConnectionStateConnecting) && now.Sub(p.createdAt) > 25*time.Second
				idleExpired := st != webrtc.PeerConnectionStateConnected && now.Sub(last) > s.idleTimeout
				if connectingExpired || idleExpired || st == webrtc.PeerConnectionStateFailed || st == webrtc.PeerConnectionStateClosed {
					stale = append(stale, p)
					delete(s.peers, id)
				}
			}
			s.mu.Unlock()
			for _, p := range stale {
				p.cancel()
				_ = p.pc.Close()
			}
		}
	}
}
