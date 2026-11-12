//go:build nop2p

package p2p

import "context"

// Service is the nop2p stub compiled when -tags nop2p is enabled.
type Service struct{}

// New returns an inert P2P service stub.
func New(_ Options) *Service {
	return &Service{}
}

// Supported reports that WebRTC P2P is compiled out of this binary.
func (s *Service) Supported() bool { return false }

// Enabled always returns false in nop2p builds.
func (s *Service) Enabled() bool { return false }

// Reconfigure is a no-op in nop2p builds.
func (s *Service) Reconfigure(_ Config) {}

// Close is a no-op in nop2p builds.
func (s *Service) Close() {}

// Status reports Supported=false and empty peer/port-map state.
func (s *Service) Status() Status {
	return Status{
		Supported:   false,
		PortMap:     PortMapStatus{State: "off"},
		ActivePeers: 0,
		Peers:       []PeerInfo{},
	}
}

// Signal rejects every signaling attempt with ErrUnsupported.
func (s *Service) Signal(_ context.Context, _ SignalInput) (SignalOutput, error) {
	return SignalOutput{}, ErrUnsupported
}
