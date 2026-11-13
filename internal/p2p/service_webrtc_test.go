//go:build !nop2p

package p2p

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func TestWebRTCControlStreamAndRPCEndToEnd(t *testing.T) {
	for _, tc := range []struct {
		name     string
		networks []webrtc.NetworkType
	}{
		{name: "dual-stack", networks: []webrtc.NetworkType{webrtc.NetworkTypeUDP4, webrtc.NetworkTypeUDP6}},
		{name: "ipv4-only", networks: []webrtc.NetworkType{webrtc.NetworkTypeUDP4}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runWebRTCControlStreamAndRPCEndToEnd(t, tc.networks)
		})
	}
}

func runWebRTCControlStreamAndRPCEndToEnd(t *testing.T, clientNetworks []webrtc.NetworkType) {
	echoDone := make(chan string, 1)
	svc := New(Options{
		AppVersion:  "test-1.0.0",
		Enabled:     true,
		PortMapping: false,
		STUNServers: []string{},
		StreamHandler: func(ctx context.Context, path string, conn StreamConn) {
			echoDone <- path
			conn.SetReadLimit(1024 * 1024)
			typ, msg, err := conn.Read(ctx)
			if err != nil {
				return
			}
			_ = conn.Write(ctx, typ, append([]byte("echo:"), msg...))
			_ = conn.Close(4410, "done")
		},
		RPCHandler: func(_ context.Context, req RPCRequest) RPCResponse {
			return RPCResponse{
				Status:     201,
				StatusText: "Created",
				Headers:    map[string]string{"Content-Type": "application/octet-stream", "X-Path": req.Path},
				Body:       append([]byte(req.Method+":"), req.Body...),
			}
		},
	})
	defer svc.Close()

	var clientSE webrtc.SettingEngine
	clientSE.SetIncludeLoopbackCandidate(true)
	clientSE.SetNetworkTypes(clientNetworks)
	clientAPI := webrtc.NewAPI(webrtc.WithSettingEngine(clientSE))
	clientPC, err := clientAPI.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = clientPC.Close() }()

	ctrlDC, err := clientPC.CreateDataChannel("webmux:control", nil)
	if err != nil {
		t.Fatal(err)
	}

	ctrlOpened := make(chan struct{})
	ctrlReplies := make(chan controlEnvelope, 8)
	ctrlDC.OnOpen(func() { close(ctrlOpened) })
	ctrlDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		var env controlEnvelope
		if json.Unmarshal(msg.Data, &env) == nil {
			ctrlReplies <- env
		}
	})

	var clientCands []ICECandidate
	gatherDone := webrtc.GatheringCompletePromise(clientPC)
	clientPC.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		j := c.ToJSON()
		clientCands = append(clientCands, ICECandidate{
			Candidate:     j.Candidate,
			SDPMid:        j.SDPMid,
			SDPMLineIndex: j.SDPMLineIndex,
		})
	})

	offer, err := clientPC.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := clientPC.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	<-gatherDone

	sigOut, err := svc.Signal(context.Background(), SignalInput{
		Action:     "offer",
		SDP:        clientPC.LocalDescription().SDP,
		Candidates: clientCands,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := clientPC.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  sigOut.SDP,
	}); err != nil {
		t.Fatal(err)
	}
	for _, c := range sigOut.Candidates {
		_ = clientPC.AddICECandidate(webrtc.ICECandidateInit{
			Candidate:     c.Candidate,
			SDPMid:        c.SDPMid,
			SDPMLineIndex: c.SDPMLineIndex,
		})
	}

	select {
	case <-ctrlOpened:
	case <-time.After(5 * time.Second):
		t.Fatal("control channel failed to open")
	}

	// 1. Handshake + Ping over control channel
	_ = ctrlDC.SendText(`{"type":"hello","proto":1,"appVersion":"web-1"}`)
	hello := <-ctrlReplies
	if hello.Type != "hello" || hello.Proto != 1 || hello.PeerID != sigOut.PeerID {
		t.Fatalf("unexpected hello response: %+v", hello)
	}

	_ = ctrlDC.SendText(`{"type":"ping","seq":7,"clientTs":12345}`)
	pong := <-ctrlReplies
	if pong.Type != "pong" || pong.Seq != 7 || pong.ClientTs != 12345 {
		t.Fatalf("unexpected pong response: %+v", pong)
	}

	// An omitted RTT is unknown; zero is a valid, measured browser report.
	if got := svc.Status().Peers[0].RTTMs; got != nil {
		t.Fatalf("unmeasured RTT = %v", *got)
	}
	_ = ctrlDC.SendText(`{"type":"peer_meta","ipFamily":"ipv4"}`)
	_ = ctrlDC.SendText(`{"type":"ping","seq":8}`)
	<-ctrlReplies // Ordered control messages: metadata has been applied.
	if got := svc.Status().Peers[0].RTTMs; got != nil {
		t.Fatalf("missing RTT became %v", *got)
	}
	_ = ctrlDC.SendText(`{"type":"peer_meta","rttMs":0}`)
	_ = ctrlDC.SendText(`{"type":"ping","seq":9}`)
	<-ctrlReplies
	if got := svc.Status().Peers[0].RTTMs; got == nil || *got != 0 {
		t.Fatalf("measured zero RTT = %v", got)
	}

	_ = ctrlDC.SendText(`{"type":"peer_meta","candidateType":"host/host","ipFamily":"ipv4","rttMs":4}`)
	time.Sleep(30 * time.Millisecond)
	st := svc.Status()
	if st.ActivePeers != 1 || len(st.Peers) != 1 || (st.Peers[0].RTTMs == nil || *st.Peers[0].RTTMs != 4) || st.Peers[0].CandidateType != "host/host" {
		t.Fatalf("unexpected service status: %+v", st)
	}

	// 2. Stream channel with >64 KiB payload + 4410 application close
	streamDC, err := clientPC.CreateDataChannel("stream:/ws/s/abc?client=c1&offset=0", nil)
	if err != nil {
		t.Fatal(err)
	}
	clientStream := newFramedConn(&pionRawChannel{dc: streamDC}, nil)
	clientStream.SetReadLimit(1024 * 1024)
	streamOpened := make(chan struct{})
	streamDC.OnOpen(func() { close(streamOpened) })
	streamDC.OnMessage(func(msg webrtc.DataChannelMessage) { clientStream.FeedRaw(msg.Data) })
	streamDC.OnClose(func() { clientStream.HandlePeerClose() })

	select {
	case <-streamOpened:
	case <-time.After(3 * time.Second):
		t.Fatal("stream channel did not open")
	}

	payload := make([]byte, 90*1024)
	_, _ = rand.Read(payload)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := clientStream.Write(ctx, MessageBinary, payload); err != nil {
		t.Fatal(err)
	}
	if gotPath := <-echoDone; gotPath != "/ws/s/abc?client=c1&offset=0" {
		t.Fatalf("unexpected stream path %q", gotPath)
	}
	typ, echoed, err := clientStream.Read(ctx)
	if err != nil || typ != MessageBinary || !bytes.Equal(echoed, append([]byte("echo:"), payload...)) {
		t.Fatalf("stream echo mismatch typ=%v len=%d err=%v", typ, len(echoed), err)
	}
	_, _, err = clientStream.Read(ctx)
	var ce *CloseError
	if !errors.As(err, &ce) || ce.Code != 4410 || ce.Reason != "done" {
		t.Fatalf("expected 4410 done close frame, got %v", err)
	}

	// 3. RPC channel (/api/* request + chunked body)
	rpcDC, err := clientPC.CreateDataChannel("rpc:1", nil)
	if err != nil {
		t.Fatal(err)
	}
	rpcOpen := make(chan struct{})
	var respHdr rpcResponseWire
	var respBody []byte
	rpcDone := make(chan struct{})
	gotHdr := false
	rpcDC.OnOpen(func() { close(rpcOpen) })
	rpcDC.OnMessage(func(msg webrtc.DataChannelMessage) {
		if !gotHdr {
			gotHdr = true
			_ = json.Unmarshal(msg.Data, &respHdr)
			return
		}
		if len(msg.Data) == 0 {
			close(rpcDone)
			return
		}
		respBody = append(respBody, msg.Data...)
	})

	<-rpcOpen
	reqBody := bytes.Repeat([]byte(" upload-chunk "), 5000) // ~70 KiB
	reqHdr, _ := json.Marshal(rpcRequestWire{
		Method:     "POST",
		Path:       "/api/uploads",
		Headers:    map[string]string{"Authorization": "Bearer t"},
		BodyLength: len(reqBody),
	})
	_ = rpcDC.SendText(string(reqHdr))
	_ = rpcDC.Send(reqBody[:40000])
	_ = rpcDC.Send(reqBody[40000:])
	_ = rpcDC.Send([]byte{})

	select {
	case <-rpcDone:
	case <-time.After(5 * time.Second):
		t.Fatal("RPC over DataChannel timed out")
	}
	if respHdr.Status != 201 || respHdr.Headers["X-Path"] != "/api/uploads" || !bytes.Equal(respBody, append([]byte("POST:"), reqBody...)) {
		t.Fatalf("unexpected RPC response hdr=%+v bodyLen=%d", respHdr, len(respBody))
	}

	// Reconfigure(Enabled=false) closes active peers immediately.
	svc.Reconfigure(Config{Enabled: false, PortMapping: false})
	if svc.Status().ActivePeers != 0 {
		t.Fatalf("expected 0 active peers after disabling P2P")
	}
}

func TestStatusReportsActualUDPBindings(t *testing.T) {
	svc := New(Options{Enabled: false, PortMapping: false, STUNServers: []string{}})
	defer svc.Close()
	status := svc.Status()
	if status.UDPPort != 0 || len(status.UDPBindings) != len(svc.udpConns) {
		t.Fatalf("bindings: %+v", status)
	}
	for i, conn := range svc.udpConns {
		addr := conn.LocalAddr().(*net.UDPAddr)
		wantNetwork := "udp6"
		if addr.IP.To4() != nil {
			wantNetwork = "udp4"
		}
		if b := status.UDPBindings[i]; b.Network != wantNetwork || b.Port != addr.Port || b.Port == 0 {
			t.Fatalf("binding: %+v, socket %v", b, addr)
		}
	}
	svc.Close()
	if got := svc.Status().UDPBindings; len(got) != 0 {
		t.Fatalf("closed bindings: %v", got)
	}
}
