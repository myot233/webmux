package wire

import (
	"slices"

	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/p2p"
)

// P2PStatus builds the wire representation of P2P configuration, persistence
// status, port mapping state, and active peer diagnostics.
func P2PStatus(cfg config.P2PConfig, source string, storageErr error, st p2p.Status) contract.P2PStatus {
	out := contract.P2PStatus{
		Supported:        st.Supported,
		Enabled:          st.Supported && cfg.Enabled,
		PortMapping:      cfg.PortMapping,
		STUNServers:      required(slices.Clone(cfg.STUNServers)),
		Source:           source,
		StorageAvailable: storageErr == nil,
		PortMap: contract.P2PPortMapStatus{
			State:        st.PortMap.State,
			Protocol:     st.PortMap.Protocol,
			ExternalPort: st.PortMap.ExternalPort,
		},
		ActivePeers: st.ActivePeers,
		Peers: each(st.Peers, func(p p2p.PeerInfo) contract.P2PPeerInfo {
			return contract.P2PPeerInfo{
				ID:            p.ID,
				State:         p.State,
				CandidateType: p.CandidateType,
				IPFamily:      p.IPFamily,
				RTTMs:         p.RTTMs,
				Channels:      p.Channels,
				CreatedAt:     p.CreatedAt,
			}
		}),
	}
	if out.PortMap.State == "" {
		out.PortMap.State = "off"
	}
	if storageErr != nil {
		out.StorageError = storageErr.Error()
	}
	return out
}

// P2PSignalInput converts a wire signaling request into a service SignalInput.
func P2PSignalInput(req contract.P2PSignalRequest) p2p.SignalInput {
	cands := make([]p2p.ICECandidate, len(req.Candidates))
	for i, c := range req.Candidates {
		cands[i] = p2p.ICECandidate{
			Candidate:     c.Candidate,
			SDPMid:        c.SDPMid,
			SDPMLineIndex: c.SDPMLineIndex,
		}
	}
	return p2p.SignalInput{
		Action:     req.Action,
		PeerID:     req.PeerID,
		SDP:        req.SDP,
		Candidates: cands,
	}
}

// P2PSignalResponse converts a service SignalOutput into a wire P2PSignalResponse.
func P2PSignalResponse(out p2p.SignalOutput) contract.P2PSignalResponse {
	return contract.P2PSignalResponse{
		PeerID: out.PeerID,
		SDP:    out.SDP,
		Candidates: each(out.Candidates, func(c p2p.ICECandidate) contract.P2PICECandidate {
			return contract.P2PICECandidate{
				Candidate:     c.Candidate,
				SDPMid:        c.SDPMid,
				SDPMLineIndex: c.SDPMLineIndex,
			}
		}),
		STUNServers: required(slices.Clone(out.STUNServers)),
	}
}
