//go:build !nop2p

package p2p

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
)

// Signal handles offer/answer negotiation, trickle ICE candidates, and peer
// closure without ever logging SDP bodies.
func (s *Service) Signal(ctx context.Context, in SignalInput) (SignalOutput, error) {
	switch in.Action {
	case "offer":
		return s.handleOffer(ctx, in)
	case "candidate":
		return s.handleCandidate(in)
	case "close":
		if in.PeerID == "" {
			return SignalOutput{}, fmt.Errorf("%w: peerId is required", ErrInvalid)
		}
		s.removePeer(in.PeerID)
		return SignalOutput{PeerID: in.PeerID, Candidates: []ICECandidate{}, STUNServers: s.stunServers()}, nil
	default:
		return SignalOutput{}, fmt.Errorf("%w: unsupported action", ErrInvalid)
	}
}

func (s *Service) stunServers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.STUNServers == nil {
		return []string{}
	}
	return slices.Clone(s.cfg.STUNServers)
}

func (s *Service) allowSignalLocked(now time.Time) bool {
	cutoff := now.Add(-time.Minute)
	keep := s.signalTimes[:0]
	for _, ts := range s.signalTimes {
		if ts.After(cutoff) {
			keep = append(keep, ts)
		}
	}
	s.signalTimes = keep
	if len(s.signalTimes) >= maxSignalPerMinute {
		return false
	}
	s.signalTimes = append(s.signalTimes, now)
	return true
}

func (s *Service) handleOffer(ctx context.Context, in SignalInput) (SignalOutput, error) {
	if strings.TrimSpace(in.SDP) == "" || len(in.SDP) > 64*1024 {
		return SignalOutput{}, fmt.Errorf("%w: invalid offer SDP", ErrInvalid)
	}

	now := time.Now()
	s.mu.Lock()
	if s.closed || !s.cfg.Enabled {
		s.mu.Unlock()
		return SignalOutput{}, ErrDisabled
	}
	if !s.allowSignalLocked(now) {
		s.mu.Unlock()
		return SignalOutput{}, ErrRateLimited
	}
	if len(s.peers) >= s.maxPeers {
		s.evictStaleOrOldestLocked(now)
	}
	if len(s.peers) >= s.maxPeers {
		s.mu.Unlock()
		return SignalOutput{}, ErrTooMany
	}
	stuns := slices.Clone(s.cfg.STUNServers)
	mappedIP := s.mappedIP
	udpMux := s.udpMux
	s.mu.Unlock()

	var se webrtc.SettingEngine
	se.SetSCTPMaxReceiveBufferSize(maxBufferedAmount)
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	se.SetIncludeLoopbackCandidate(true)
	se.SetInterfaceFilter(s.allowInterface)
	se.SetNetworkTypes([]webrtc.NetworkType{
		webrtc.NetworkTypeUDP4,
		webrtc.NetworkTypeUDP6,
	})
	if udpMux != nil {
		se.SetICEUDPMux(udpMux)
	}
	if mappedIP != "" {
		_ = se.SetICEAddressRewriteRules(webrtc.ICEAddressRewriteRule{
			External:        []string{mappedIP},
			AsCandidateType: webrtc.ICECandidateTypeHost,
			Mode:            webrtc.ICEAddressRewriteAppend,
		})
	}

	apiEngine := webrtc.NewAPI(webrtc.WithSettingEngine(se))
	var iceServers []webrtc.ICEServer
	if len(stuns) > 0 {
		iceServers = append(iceServers, webrtc.ICEServer{URLs: stuns})
	}
	pc, err := apiEngine.NewPeerConnection(webrtc.Configuration{ICEServers: iceServers})
	if err != nil {
		return SignalOutput{}, err
	}

	peerCtx, cancel := context.WithCancel(context.Background())
	id := randomPeerID()
	entry := &peerEntry{
		id:        id,
		pc:        pc,
		ctx:       peerCtx,
		cancel:    cancel,
		createdAt: now,
	}
	entry.touch()

	gatherDone := make(chan struct{})
	var gatherOnce sync.Once
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			gatherOnce.Do(func() { close(gatherDone) })
			return
		}
		init := c.ToJSON()
		entry.mu.Lock()
		entry.pendingLocal = append(entry.pendingLocal, ICECandidate{
			Candidate:     init.Candidate,
			SDPMid:        init.SDPMid,
			SDPMLineIndex: init.SDPMLineIndex,
		})
		entry.mu.Unlock()
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		entry.touch()
		switch state {
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			s.removePeer(id)
		}
	})

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		entry.touch()
		label := dc.Label()
		switch {
		case label == controlChannelLabel:
			s.attachControlChannel(entry, dc)
		case strings.HasPrefix(label, streamChannelPrefix):
			path := strings.TrimPrefix(label, streamChannelPrefix)
			s.attachStreamChannel(entry, path, dc)
		case label == "rpc" || strings.HasPrefix(label, rpcChannelPrefix):
			s.attachRPCChannel(entry, dc)
		default:
			_ = dc.Close()
		}
	})

	if err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  in.SDP,
	}); err != nil {
		cancel()
		_ = pc.Close()
		return SignalOutput{}, fmt.Errorf("%w: invalid remote offer", ErrInvalid)
	}
	for _, cand := range in.Candidates {
		if cand.Candidate != "" {
			_ = pc.AddICECandidate(webrtc.ICECandidateInit{
				Candidate:     cand.Candidate,
				SDPMid:        cand.SDPMid,
				SDPMLineIndex: cand.SDPMLineIndex,
			})
		}
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		cancel()
		_ = pc.Close()
		return SignalOutput{}, err
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		cancel()
		_ = pc.Close()
		return SignalOutput{}, err
	}

	s.mu.Lock()
	if s.closed || !s.cfg.Enabled {
		s.mu.Unlock()
		cancel()
		_ = pc.Close()
		return SignalOutput{}, ErrDisabled
	}
	s.peers[id] = entry
	s.mu.Unlock()

	timer := time.NewTimer(gatherWaitTimeout)
	select {
	case <-gatherDone:
	case <-timer.C:
	case <-ctx.Done():
		timer.Stop()
		s.removePeer(id)
		return SignalOutput{}, ctx.Err()
	}
	timer.Stop()

	entry.mu.Lock()
	cands := slices.Clone(entry.pendingLocal)
	entry.pendingLocal = nil
	entry.mu.Unlock()
	if cands == nil {
		cands = []ICECandidate{}
	}
	if stuns == nil {
		stuns = []string{}
	}
	localDesc := pc.LocalDescription()
	sdp := answer.SDP
	if localDesc != nil {
		sdp = localDesc.SDP
	}
	return SignalOutput{
		PeerID:      id,
		SDP:         sdp,
		Candidates:  cands,
		STUNServers: stuns,
	}, nil
}

func (s *Service) handleCandidate(in SignalInput) (SignalOutput, error) {
	if in.PeerID == "" {
		return SignalOutput{}, fmt.Errorf("%w: peerId is required", ErrInvalid)
	}
	s.mu.Lock()
	entry := s.peers[in.PeerID]
	s.mu.Unlock()
	if entry == nil {
		return SignalOutput{}, ErrPeerMissing
	}
	entry.touch()
	for _, cand := range in.Candidates {
		if strings.TrimSpace(cand.Candidate) == "" {
			continue
		}
		if err := entry.pc.AddICECandidate(webrtc.ICECandidateInit{
			Candidate:     cand.Candidate,
			SDPMid:        cand.SDPMid,
			SDPMLineIndex: cand.SDPMLineIndex,
		}); err != nil {
			return SignalOutput{}, fmt.Errorf("%w: invalid ICE candidate", ErrInvalid)
		}
	}
	entry.mu.Lock()
	cands := slices.Clone(entry.pendingLocal)
	entry.pendingLocal = nil
	entry.mu.Unlock()
	if cands == nil {
		cands = []ICECandidate{}
	}
	return SignalOutput{
		PeerID:      in.PeerID,
		Candidates:  cands,
		STUNServers: s.stunServers(),
	}, nil
}

func (s *Service) removePeer(id string) {
	s.mu.Lock()
	entry := s.peers[id]
	delete(s.peers, id)
	s.mu.Unlock()
	if entry != nil {
		entry.cancel()
		_ = entry.pc.Close()
	}
}

func (s *Service) evictStaleOrOldestLocked(now time.Time) {
	for id, p := range s.peers {
		st := p.pc.ConnectionState()
		if st == webrtc.PeerConnectionStateDisconnected || st == webrtc.PeerConnectionStateFailed || st == webrtc.PeerConnectionStateClosed {
			delete(s.peers, id)
			p.cancel()
			go func(pc *webrtc.PeerConnection) { _ = pc.Close() }(p.pc)
			return
		}
		last := time.Unix(0, p.lastSeen.Load())
		if now.Sub(last) > s.idleTimeout {
			delete(s.peers, id)
			p.cancel()
			go func(pc *webrtc.PeerConnection) { _ = pc.Close() }(p.pc)
			return
		}
	}
}

func randomPeerID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
