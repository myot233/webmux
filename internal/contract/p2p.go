package contract

// P2PProtocolVersion is the control-channel protocol major version.
const P2PProtocolVersion = 1

// P2PStatus is the body of GET /api/p2p and PUT/DELETE /api/p2p/settings.
// It never carries session credentials or raw SDP strings.
type P2PStatus struct {
	// Supported reports whether this binary was built with WebRTC P2P support
	// (false when compiled with -tags nop2p).
	Supported bool `json:"supported"`
	// Enabled is the effective [p2p] enabled value.
	Enabled bool `json:"enabled"`
	// PortMapping is the effective [p2p] port_mapping value.
	PortMapping bool `json:"portMapping"`
	// STUNServers is the effective STUN URI list.
	STUNServers []string `json:"stunServers"`
	// Source is "config" when TOML values apply, or "web" after a saved update.
	Source string `json:"source"`
	// StorageAvailable reports whether web updates can be persisted.
	StorageAvailable bool `json:"storageAvailable"`
	// StorageError explains an unavailable settings store.
	StorageError string `json:"storageError,omitempty"`
	// PortMap describes the active local gateway port mapping state.
	PortMap P2PPortMapStatus `json:"portMap"`
	// ActivePeers is the count of currently open PeerConnections.
	ActivePeers int `json:"activePeers"`
	// Peers lists sanitized active peer diagnostics.
	Peers []P2PPeerInfo `json:"peers"`
}

// P2PPortMapStatus describes UPnP IGD / NAT-PMP / PCP port mapping state.
type P2PPortMapStatus struct {
	// State is "off", "probing", "mapped", or "unavailable".
	State string `json:"state"`
	// Protocol is "nat-pmp", "pcp", "upnp", or "" when not mapped.
	Protocol string `json:"protocol,omitempty"`
	// ExternalPort is the mapped public UDP port when State == "mapped".
	ExternalPort int `json:"externalPort,omitempty"`
}

// P2PPeerInfo summarizes one active server-side PeerConnection without exposing
// raw candidate IP addresses or SDP credentials.
type P2PPeerInfo struct {
	ID            string `json:"id"`
	State         string `json:"state"`
	CandidateType string `json:"candidateType,omitempty"`
	IPFamily      string `json:"ipFamily,omitempty"`
	RTTMs         int    `json:"rttMs,omitempty"`
	Channels      int    `json:"channels"`
	CreatedAt     string `json:"createdAt"`
}

// P2PSettingsUpdate is the body of PUT /api/p2p/settings.
type P2PSettingsUpdate struct {
	Enabled     bool     `json:"enabled"`
	PortMapping bool     `json:"portMapping"`
	STUNServers []string `json:"stunServers"`
}

// P2PSignalRequest is the body of POST /api/p2p/signal.
// Action is "offer", "candidate", or "close".
type P2PSignalRequest struct {
	Action     string            `json:"action"`
	PeerID     string            `json:"peerId,omitempty"`
	SDP        string            `json:"sdp,omitempty"`
	Candidates []P2PICECandidate `json:"candidates,omitempty"`
}

// P2PICECandidate carries one trickle or bundled ICE candidate.
type P2PICECandidate struct {
	Candidate     string  `json:"candidate"`
	SDPMid        *string `json:"sdpMid,omitempty"`
	SDPMLineIndex *uint16 `json:"sdpMLineIndex,omitempty"`
}

// P2PSignalResponse is the body returned by POST /api/p2p/signal.
type P2PSignalResponse struct {
	PeerID      string            `json:"peerId"`
	SDP         string            `json:"sdp,omitempty"`
	Candidates  []P2PICECandidate `json:"candidates"`
	STUNServers []string          `json:"stunServers"`
}

// P2PPingResponse is the body returned by GET /api/p2p/ping for baseline
// application-layer RTT measurement.
type P2PPingResponse struct {
	Seq      int   `json:"seq"`
	ServerTs int64 `json:"serverTs"`
}
