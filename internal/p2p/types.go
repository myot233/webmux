// Package p2p manages opportunistic WebRTC PeerConnections, local gateway UDP
// port mappings, DataChannel stream framing, and DataChannel API RPCs.
package p2p

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUnsupported = errors.New("p2p support is disabled in this build")
	ErrDisabled    = errors.New("p2p direct upgrade is disabled")
	ErrRateLimited = errors.New("too many p2p signaling requests")
	ErrTooMany     = errors.New("maximum concurrent p2p peers reached")
	ErrPeerMissing = errors.New("p2p peer not found")
	ErrInvalid     = errors.New("invalid p2p signaling payload")
	ErrSlowClient  = errors.New("p2p data channel slow client")
)

// MessageType matches hub.MessageType without coupling service to controller.
type MessageType int

const (
	MessageText MessageType = iota + 1
	MessageBinary
)

// StreamConn is the framed stream interface matching hub.MessageConn.
type StreamConn interface {
	Read(ctx context.Context) (MessageType, []byte, error)
	Write(ctx context.Context, typ MessageType, p []byte) error
	Close(code uint16, reason string) error
	CloseNow() error
	SetReadLimit(n int64)
	CloseRead(ctx context.Context) context.Context
}

// StreamHandler attaches a newly opened stream DataChannel to a path such as
// "/ws/s/abc?client=..." or "/ws/sessions".
type StreamHandler func(ctx context.Context, path string, conn StreamConn)

// RPCRequest represents one HTTP-equivalent API request sent over DataChannel.
type RPCRequest struct {
	Method  string
	Path    string
	Headers map[string]string
	Body    []byte
}

// RPCResponse represents the server's answer to an RPCRequest.
type RPCResponse struct {
	Status     int
	StatusText string
	Headers    map[string]string
	Body       []byte
}

// RPCHandler executes an RPCRequest against the server's internal API router.
type RPCHandler func(ctx context.Context, req RPCRequest) RPCResponse

// Options configures the P2P service.
type Options struct {
	AppVersion      string
	Enabled         bool
	PortMapping     bool
	STUNServers     []string
	UDPPort         int
	InterfaceFilter string
	MaxPeers        int
	MaxChannels     int
	IdleTimeout     time.Duration
	StreamHandler   StreamHandler
	RPCHandler      RPCHandler
}

// Config is the runtime-reconfigurable subset of Options.
type Config struct {
	Enabled     bool
	PortMapping bool
	STUNServers []string
}

// ICECandidate represents one trickle or bundled ICE candidate.
type ICECandidate struct {
	Candidate     string
	SDPMid        *string
	SDPMLineIndex *uint16
}

// SignalInput carries one signaling action ("offer", "candidate", "close").
type SignalInput struct {
	Action     string
	PeerID     string
	SDP        string
	Candidates []ICECandidate
}

// SignalOutput is the signaling response returned to the client.
type SignalOutput struct {
	PeerID      string
	SDP         string
	Candidates  []ICECandidate
	STUNServers []string
}

// PortMapStatus describes the local gateway UDP port mapping state.
type PortMapStatus struct {
	State        string
	Protocol     string
	ExternalPort int
}

// PeerInfo summarizes an active PeerConnection without exposing raw IPs or SDP.
type PeerInfo struct {
	ID            string
	State         string
	CandidateType string
	IPFamily      string
	RTTMs         *int
	Channels      int
	CreatedAt     string
}

// UDPBinding reports an actually bound ICE UDP socket, without exposing IPs.
type UDPBinding struct {
	Network string
	Port    int
}

// Status describes the live P2P runtime state.
type Status struct {
	Supported   bool
	UDPPort     int
	UDPBindings []UDPBinding
	PortMap     PortMapStatus
	ActivePeers int
	Peers       []PeerInfo
}
