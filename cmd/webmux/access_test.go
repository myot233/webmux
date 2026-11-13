package main

import (
	"net"
	"net/url"
	"strings"
	"testing"
	"time"

	"rsc.io/qr"
)

func TestLocalOriginConnectsToBoundListener(t *testing.T) {
	for _, row := range []struct{ network, address string }{
		{"tcp4", "127.0.0.1:0"},
		{"tcp4", "0.0.0.0:0"},
		{"tcp6", "[::1]:0"},
		{"tcp6", "[::]:0"},
		{"tcp", "0.0.0.0:0"},
	} {
		t.Run(row.network+"/"+row.address, func(t *testing.T) {
			listener, err := net.Listen(row.network, row.address)
			if err != nil {
				if row.network == "tcp6" {
					t.Skipf("IPv6 unavailable: %v", err)
				}
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = listener.Close() })
			origin := localOrigin(listener.Addr())
			target, err := url.Parse(origin)
			if err != nil || target.Scheme != "http" {
				t.Fatalf("invalid local origin %q: %v", origin, err)
			}
			conn, err := net.DialTimeout(row.network, target.Host, time.Second)
			if err != nil {
				t.Fatalf("origin does not reach actual listener: %s: %v", origin, err)
			}
			t.Cleanup(func() { _ = conn.Close() })
			tcp := listener.(*net.TCPListener)
			if err := tcp.SetDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			accepted, err := tcp.AcceptTCP()
			if err != nil {
				t.Fatalf("origin reached a different listener: %s: %v", origin, err)
			}
			_ = accepted.Close()
		})
	}
}

func TestGenerateToken(t *testing.T) {
	a, b := generateToken(), generateToken()
	if len(a) != 43 || a == b || url.PathEscape(a) != a {
		t.Fatalf("tokens %q %q", a, b)
	}
}

func TestRenderQRRoundTrips(t *testing.T) {
	text := "https://abc-def.trycloudflare.com/#token=" + strings.Repeat("x", 43)
	rendered, err := renderQR(text)
	if err != nil {
		t.Fatal(err)
	}
	code, err := qr.Encode(text, qr.L)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(rendered, "\n"), "\n")
	side := code.Size + 2*qrQuiet
	if len(lines) != (side+1)/2 {
		t.Fatalf("%d lines for %d modules", len(lines), side)
	}
	for row, line := range lines {
		cells := []rune(line)
		if len(cells) != side {
			t.Fatalf("line %d has %d cells, want %d", row, len(cells), side)
		}
		for col, cell := range cells {
			top := cell == '█' || cell == '▀'
			bottom := cell == '█' || cell == '▄'
			x, y := col-qrQuiet, 2*row-qrQuiet
			if top != light(code, x, y) || (y+1 < code.Size+qrQuiet && bottom != light(code, x, y+1)) {
				t.Fatalf("module mismatch at x=%d y=%d: %q", x, y, cell)
			}
		}
	}
}

func light(code *qr.Code, x, y int) bool { return !code.Black(x, y) }
