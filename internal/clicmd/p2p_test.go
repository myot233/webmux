//go:build linux || darwin

package clicmd

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/contract"
)

// All network tests use local fake STUN and isolated Unix HTTP servers.
func fakeSTUN(t *testing.T, network string, reply bool) string {
	t.Helper()
	host := "127.0.0.1:0"
	if network == "udp6" {
		host = "[::1]:0"
	}
	conn, err := net.ListenPacket(network, host)
	if err != nil {
		if network == "udp6" {
			t.Skipf("IPv6 unavailable: %v", err)
		}
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		packet := make([]byte, 2048)
		for {
			n, addr, err := conn.ReadFrom(packet)
			if err != nil {
				return
			}
			if !reply || n != 20 {
				continue
			}
			response := mappedSTUNResponse(packet[:n], addr.(*net.UDPAddr))
			if _, err := conn.WriteTo(response, addr); err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() { _ = conn.Close(); <-done })
	return "stun:" + conn.LocalAddr().String()
}

func mappedSTUNResponse(request []byte, addr *net.UDPAddr) []byte {
	ip, family := addr.IP.To4(), byte(1)
	if ip == nil {
		ip, family = addr.IP.To16(), 2
	}
	packet := make([]byte, 28+len(ip))
	copy(packet, request)
	binary.BigEndian.PutUint16(packet, 0x0101)
	binary.BigEndian.PutUint16(packet[2:], uint16(len(packet)-20))
	binary.BigEndian.PutUint16(packet[20:], 0x0020)
	binary.BigEndian.PutUint16(packet[22:], uint16(4+len(ip)))
	packet[25] = family
	binary.BigEndian.PutUint16(packet[26:], uint16(addr.Port)^0x2112)
	for i := range ip {
		packet[28+i] = ip[i] ^ request[4+i]
	}
	return packet
}

func TestP2PSTUNProbe(t *testing.T) {
	for _, network := range []string{"udp4", "udp6"} {
		t.Run(network, func(t *testing.T) {
			target := fakeSTUN(t, network, true)
			result := probeSTUN(context.Background(), target, network)
			if !result.Success || result.MappedAddress != result.LocalAddress || result.RTTMs == nil || result.RemoteAddress == "" {
				t.Fatalf("probe: %+v", result)
			}
		})
	}
}

func TestSTUNRejectsInvalidResponses(t *testing.T) {
	request, err := stunBindingRequest()
	if err != nil {
		t.Fatal(err)
	}
	good := mappedSTUNResponse(request, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 32123})
	tests := []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"truncated header", func(p []byte) []byte { return p[:18] }},
		{"transaction", func(p []byte) []byte { p[8] ^= 1; return p }},
		{"cookie", func(p []byte) []byte { p[4] ^= 1; return p }},
		{"wrong message type", func(p []byte) []byte { p[0] = 0; return p }},
		{"error response", func(p []byte) []byte { p[1] = 0x11; return p }},
		{"invalid length", func(p []byte) []byte { p[3]++; return p }},
		{"attribute overrun", func(p []byte) []byte { p[23] = 255; return p }},
		{"missing mapping", func(p []byte) []byte { p[21] = 0x24; return p }},
		{"invalid family", func(p []byte) []byte { p[25] = 3; return p }},
		{"zero port", func(p []byte) []byte { p[26], p[27] = 0x21, 0x12; return p }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := stunMappedAddress(tt.mutate(append([]byte(nil), good...)), request); err == nil {
				t.Fatal("accepted invalid response")
			}
		})
	}
}

func TestP2PProbeCancellationAndTimeout(t *testing.T) {
	target := fakeSTUN(t, "udp4", false)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(30*time.Millisecond, cancel)
	start := time.Now()
	result := probeSTUN(ctx, target, "udp4")
	if result.Success || !strings.Contains(result.Error, "canceled") || time.Since(start) > time.Second {
		t.Fatalf("cancel: %+v", result)
	}
	e, stdout, _ := serveUnix(t, make(chan struct{}), http.NotFoundHandler())
	if code := e.Run(context.Background(), p2pProbeCommand, []string{"--server", target, "--timeout", "30ms", "--json"}); code != contract.ExitTimeout {
		t.Fatalf("timeout exit %d", code)
	}
	var out contract.P2PProbe
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil || len(out.Results) != 1 || out.Results[0].Success || out.Results[0].Error == "" {
		t.Fatalf("timeout JSON: %s, %v", stdout, err)
	}
}

func TestP2PCommandsReadLocalDiagnostics(t *testing.T) {
	target := fakeSTUN(t, "udp4", true)
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+contract.PathP2PDiagnostics, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"supported":true,"enabled":false,"udpPort":0,"udpBindings":[{"network":"udp4","port":43210}],"capabilities":{"networks":["udp4","udp6"],"iceMode":"full","turn":false},"source":"web","portMapping":true,"portMap":{"state":"unavailable"},"activePeers":1,"peers":[{"id":"peer1","state":"connecting","channels":0,"createdAt":"now"}],"stunServers":[`)
		b, _ := json.Marshal(target)
		_, _ = w.Write(b)
		_, _ = io.WriteString(w, `],"storageAvailable":true}`)
	})
	e, stdout, stderr := serveUnix(t, make(chan struct{}), mux)
	for _, args := range [][]string{{"p2p", "status"}, {"p2p", "peers"}, {"p2p", "status", "--json"}, {"p2p", "peers", "--json"}, {"p2p", "probe", "--json"}} {
		stdout.Reset()
		if code := run(t, e, args...); code != 0 {
			t.Fatalf("%v: %d %s", args, code, stderr)
		}
		if len(args) == 2 && args[1] == "peers" && (!strings.Contains(stdout.String(), "unknown") || strings.Contains(stdout.String(), "healthy")) {
			t.Fatalf("unknown metrics: %s", stdout)
		}
		if args[1] == "probe" && (!strings.Contains(stdout.String(), `"scope": "cli-ephemeral-udp"`) || !strings.Contains(stdout.String(), `"success": true`)) {
			t.Fatalf("probe: %s", stdout)
		}
	}
	// Explicit target operates offline, without looking up service config.
	e.Connect = nil
	if code := e.Run(context.Background(), p2pProbeCommand, []string{"--server", target}); code != 0 {
		t.Fatalf("offline probe %d", code)
	}
}

func TestP2PUsageAndHelpDoNotConnect(t *testing.T) {
	e, stdout, _ := serveUnix(t, make(chan struct{}), http.NotFoundHandler())
	e.Connect = nil
	for _, args := range [][]string{
		{"p2p", "status", "extra"}, {"p2p", "peers", "--timeout", "0"},
		{"p2p", "probe", "--timeout", "61s"}, {"p2p", "probe", "--network", "tcp"},
		{"p2p", "probe", "--server", "http://bad"}, {"p2p", "probe", "--server", "stun:host:0"},
	} {
		if code := run(t, e, args...); code != contract.ExitUsage {
			t.Fatalf("%v: %d", args, code)
		}
	}
	for _, name := range []string{"status", "peers", "probe"} {
		stdout.Reset()
		if code := run(t, e, "p2p", name, "--help"); code != 0 || !strings.Contains(stdout.String(), "Exit status:") {
			t.Fatalf("help %s: %d %s", name, code, stdout)
		}
	}
}

func TestP2PProbeReportsUnsupportedAndNoTargets(t *testing.T) {
	e, stdout, _ := serveUnix(t, make(chan struct{}), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"stunServers":[],"peers":[]}`)
	}))
	if code := e.Run(context.Background(), p2pProbeCommand, []string{"--json"}); code != contract.ExitError || !strings.Contains(stdout.String(), `"results": []`) {
		t.Fatalf("no targets: %d %s", code, stdout)
	}
	stdout.Reset()
	if code := e.Run(context.Background(), p2pProbeCommand, []string{"--server", "stuns:example.invalid", "--json"}); code != contract.ExitError || !strings.Contains(stdout.String(), "TLS/TCP") {
		t.Fatalf("stuns: %d %s", code, stdout)
	}
}

func TestP2PUnknownAndZeroRTTAndUnsupportedStatus(t *testing.T) {
	e, stdout, _ := serveUnix(t, make(chan struct{}), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"supported":false,"enabled":false,"udpBindings":[],"capabilities":{"networks":[],"iceMode":"unsupported","turn":false},"peers":[{"id":"unknown","state":"new","channels":0},{"id":"zero","state":"connected","rttMs":0,"channels":1}]}`)
	}))
	if code := e.Run(context.Background(), p2pStatusCommand, nil); code != 0 {
		t.Fatal(code)
	}
	if text := stdout.String(); !strings.Contains(text, "compiled out") || strings.Contains(text, "UDP4/UDP6") || strings.Contains(text, "full ICE") || strings.Contains(text, "per-peer sockets may") {
		t.Fatalf("unsupported: %s", text)
	}
	stdout.Reset()
	if code := e.Run(context.Background(), p2pPeersCommand, []string{"--json"}); code != 0 {
		t.Fatal(code)
	}
	var peers []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &peers); err != nil {
		t.Fatal(err)
	}
	if _, ok := peers[0]["rttMs"]; ok {
		t.Fatal("unknown RTT encoded as measured")
	}
	if peers[1]["rttMs"] != float64(0) {
		t.Fatal("measured zero RTT lost")
	}
	stdout.Reset()
	if code := e.Run(context.Background(), p2pPeersCommand, nil); code != 0 {
		t.Fatal(code)
	}
	if text := stdout.String(); !strings.Contains(text, "unknown") || !strings.Contains(text, "connected") || !strings.Contains(text, "0") {
		t.Fatalf("human RTT: %s", text)
	}
}
