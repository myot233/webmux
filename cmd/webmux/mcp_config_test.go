package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/rainchan/webmux/internal/config"
)

func TestMCPRequiresCredentialBeforeExposureGeneratesOne(t *testing.T) {
	for _, tt := range []struct {
		name, listen string
		tunnel       bool
	}{
		{name: "loopback", listen: "127.0.0.1:0"},
		{name: "network", listen: "0.0.0.0:0"},
		{name: "tunnel", listen: "127.0.0.1:0", tunnel: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := config.Config{Listen: tt.listen, MCP: config.MCPConfig{Enabled: true}, Tunnel: config.TunnelConfig{Enabled: tt.tunnel}}
			if err := ensureStartupToken(&c, false, false); !errors.Is(err, errMCPOwnerToken) || c.Token != "" {
				t.Fatalf("empty owner credential accepted or synthesized: generated=%v error=%v", c.Token != "", err)
			}
		})
	}
	// Direct embedders must fail before constructing stores or binding a port.
	state := filepath.Join(t.TempDir(), "not-created")
	if err := serve(context.Background(), serverRun{Config: config.Config{MCP: config.MCPConfig{Enabled: true}, StateDir: state}}, io.Discard); !errors.Is(err, errMCPOwnerToken) {
		t.Fatal("direct serve bypassed the credential prerequisite")
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("refused startup initialized state: %v", err)
	}
}
