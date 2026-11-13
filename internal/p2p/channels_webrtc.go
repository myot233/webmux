//go:build !nop2p

package p2p

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

const (
	controlChannelLabel = "webmux:control"
	streamChannelPrefix = "stream:"
	rpcChannelPrefix    = "rpc:"

	maxRPCBodyBytes = 128 * 1024 * 1024 // 128 MiB ceiling matching upload limits
	rpcChunkBytes   = 32 * 1024
)

type controlEnvelope struct {
	Type          string   `json:"type"`
	Proto         int      `json:"proto,omitempty"`
	AppVersion    string   `json:"appVersion,omitempty"`
	Features      []string `json:"features,omitempty"`
	PeerID        string   `json:"peerId,omitempty"`
	CandidateType string   `json:"candidateType,omitempty"`
	IPFamily      string   `json:"ipFamily,omitempty"`
	PortMapped    bool     `json:"portMapped,omitempty"`
	Seq           int      `json:"seq,omitempty"`
	ClientTs      int64    `json:"clientTs,omitempty"`
	RTTMs         *int     `json:"rttMs,omitempty"`
}

type rpcRequestWire struct {
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Headers    map[string]string `json:"headers"`
	BodyLength int               `json:"bodyLength"`
}

type rpcResponseWire struct {
	Status     int               `json:"status"`
	StatusText string            `json:"statusText"`
	Headers    map[string]string `json:"headers"`
}

func encodeControlJSON(v controlEnvelope) ([]byte, error) {
	return json.Marshal(v)
}

func encodeRPCResponseJSON(v rpcResponseWire) ([]byte, error) {
	return json.Marshal(v)
}

type pionRawChannel struct {
	dc *webrtc.DataChannel
}

func (p *pionRawChannel) Send(data []byte) error {
	return p.dc.Send(data)
}

func (p *pionRawChannel) Close() error {
	return p.dc.Close()
}

func (p *pionRawChannel) BufferedAmount() uint64 {
	return p.dc.BufferedAmount()
}

func (p *pionRawChannel) SetBufferedAmountLowThreshold(th uint64) {
	p.dc.SetBufferedAmountLowThreshold(th)
}

func (p *pionRawChannel) OnBufferedAmountLow(cb func()) {
	p.dc.OnBufferedAmountLow(cb)
}

func (s *Service) attachControlChannel(peer *peerEntry, dc *webrtc.DataChannel) {
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		peer.touch()
		var env controlEnvelope
		if err := json.Unmarshal(msg.Data, &env); err != nil {
			return
		}
		switch env.Type {
		case "hello":
			if env.Proto != 1 {
				_ = dc.Close()
				s.removePeer(peer.id)
				return
			}
			candType, ipFamily := peer.selectedCandidateMeta()
			pmStatus, _ := s.mapperStatus()
			reply, err := encodeControlJSON(controlEnvelope{
				Type:          "hello",
				Proto:         1,
				AppVersion:    s.appVersion,
				Features:      []string{"stream-v1", "rpc-v1"},
				PeerID:        peer.id,
				CandidateType: candType,
				IPFamily:      ipFamily,
				PortMapped:    pmStatus.State == "mapped",
			})
			if err == nil {
				_ = dc.SendText(string(reply))
			}

		case "ping":
			reply, err := encodeControlJSON(controlEnvelope{
				Type:     "pong",
				Seq:      env.Seq,
				ClientTs: env.ClientTs,
			})
			if err == nil {
				_ = dc.SendText(string(reply))
			}

		case "peer_meta":
			peer.mu.Lock()
			if env.CandidateType != "" {
				peer.candidateType = sanitizeToken(env.CandidateType, 24)
			}
			if env.IPFamily == "ipv4" || env.IPFamily == "ipv6" {
				peer.ipFamily = env.IPFamily
			}
			if env.RTTMs != nil && *env.RTTMs >= 0 && *env.RTTMs <= 60000 {
				peer.rttMs = env.RTTMs
			}
			peer.mu.Unlock()
		}
	})
}

func sanitizeToken(raw string, maxLen int) string {
	if len(raw) > maxLen {
		raw = raw[:maxLen]
	}
	out := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '/' {
			out = append(out, ch)
		}
	}
	return string(out)
}

func (s *Service) attachStreamChannel(peer *peerEntry, path string, dc *webrtc.DataChannel) {
	if s.streamHandler == nil {
		_ = dc.Close()
		return
	}
	if !peer.tryIncChannel(s.maxChannels) {
		_ = dc.Close()
		return
	}
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(peer.decChannel)
	}

	fc := newFramedConn(&pionRawChannel{dc: dc}, release)
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		peer.touch()
		fc.FeedRaw(msg.Data)
	})
	dc.OnClose(func() {
		fc.HandlePeerClose()
	})
	dc.OnError(func(_ error) {
		fc.HandlePeerClose()
	})
	dc.OnOpen(func() {
		go func() {
			defer func() { _ = fc.CloseNow() }()
			s.streamHandler(peer.ctx, path, fc)
		}()
	})
}

func (s *Service) attachRPCChannel(peer *peerEntry, dc *webrtc.DataChannel) {
	if s.rpcHandler == nil {
		_ = dc.Close()
		return
	}
	if !peer.tryIncChannel(s.maxChannels) {
		_ = dc.Close()
		return
	}
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			peer.decChannel()
			_ = dc.Close()
		})
	}

	ctx, cancel := context.WithCancel(peer.ctx)
	var mu sync.Mutex
	var header *rpcRequestWire
	var bodyBuf []byte
	dispatched := false

	lowCh := make(chan struct{}, 1)
	dc.SetBufferedAmountLowThreshold(bufferLowThreshold)
	dc.OnBufferedAmountLow(func() {
		select {
		case lowCh <- struct{}{}:
		default:
		}
	})
	dc.OnClose(func() {
		cancel()
		release()
	})
	dc.OnError(func(_ error) {
		cancel()
		release()
	})

	dispatch := func(req rpcRequestWire, body []byte) {
		defer release()
		defer cancel()
		resp := s.rpcHandler(ctx, RPCRequest{
			Method:  req.Method,
			Path:    req.Path,
			Headers: req.Headers,
			Body:    body,
		})
		hdrBytes, err := encodeRPCResponseJSON(rpcResponseWire{
			Status:     resp.Status,
			StatusText: resp.StatusText,
			Headers:    resp.Headers,
		})
		if err != nil {
			return
		}
		if err := sendRPCPacket(ctx, dc, lowCh, hdrBytes, true); err != nil {
			return
		}
		for off := 0; off < len(resp.Body); off += rpcChunkBytes {
			end := off + rpcChunkBytes
			if end > len(resp.Body) {
				end = len(resp.Body)
			}
			if err := sendRPCPacket(ctx, dc, lowCh, resp.Body[off:end], false); err != nil {
				return
			}
		}
		// Zero-length binary frame marks end-of-response.
		_ = sendRPCPacket(ctx, dc, lowCh, []byte{}, false)
	}

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		peer.touch()
		mu.Lock()
		if dispatched {
			mu.Unlock()
			return
		}
		if header == nil {
			if !msg.IsString {
				mu.Unlock()
				release()
				return
			}
			var h rpcRequestWire
			if err := json.Unmarshal(msg.Data, &h); err != nil || h.BodyLength < 0 || h.BodyLength > maxRPCBodyBytes {
				mu.Unlock()
				release()
				return
			}
			header = &h
			if h.BodyLength > 0 {
				bodyBuf = make([]byte, 0, min(h.BodyLength, 64*1024))
			}
			mu.Unlock()
			return
		}
		// Binary chunk or zero-length EOF marker
		if len(msg.Data) == 0 {
			dispatched = true
			req := *header
			body := bodyBuf
			mu.Unlock()
			go dispatch(req, body)
			return
		}
		if len(bodyBuf)+len(msg.Data) > maxRPCBodyBytes {
			mu.Unlock()
			release()
			return
		}
		bodyBuf = append(bodyBuf, msg.Data...)
		mu.Unlock()
	})
}

func sendRPCPacket(ctx context.Context, dc *webrtc.DataChannel, lowCh <-chan struct{}, data []byte, isText bool) error {
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	for dc.BufferedAmount() > maxBufferedAmount {
		select {
		case <-ctx.Done():
			return io.ErrClosedPipe
		case <-deadline.C:
			return errors.New("rpc channel write timeout")
		case <-lowCh:
		}
	}
	if isText {
		return dc.SendText(string(data))
	}
	return dc.Send(data)
}
