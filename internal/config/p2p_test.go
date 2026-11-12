package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestP2PDefaultsAndTOMLOverrides(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "missing.toml"), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if !c.P2P.Enabled || !c.P2P.PortMapping || !reflect.DeepEqual(c.P2P.STUNServers, DefaultSTUNServers()) {
		t.Fatalf("unexpected defaults: %+v", c.P2P)
	}
	if c.Source("p2p.enabled") != "default" || c.Source("p2p.port_mapping") != "default" || c.Source("p2p.stun_servers") != "default" {
		t.Fatalf("expected default sources: %v", c.Sources)
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	raw := "[p2p]\nenabled = false\nport_mapping = false\nstun_servers = ['STUN:Example.COM:3478', 'stun:[2001:db8::1]:3478']\n"
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	c, err = Load(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	want := []string{"stun:example.com:3478", "stun:[2001:db8::1]:3478"}
	if c.P2P.Enabled || c.P2P.PortMapping || !reflect.DeepEqual(c.P2P.STUNServers, want) {
		t.Fatalf("loaded p2p = %+v, want %v", c.P2P, want)
	}
	if c.Source("p2p.enabled") != "toml" || c.Source("p2p.port_mapping") != "toml" || c.Source("p2p.stun_servers") != "toml" {
		t.Fatalf("sources = %v", c.Sources)
	}
}

func TestP2PValidationRejectsInvalidSTUN(t *testing.T) {
	badCases := [][]string{
		{""},
		{"https://stun.example.com"},
		{"turn:turn.example.com:3478"},
		{"stun:stun.example.com/path"},
		{"stun:stun.example.com?transport=udp"},
		{"stun:user@stun.example.com:3478"},
		{"stun:stun.example.com:0"},
		{"stun:stun.example.com:70000"},
		{"stun:2001:db8::1:3478"},
		{"stun:stun.example.com:3478", "STUN:stun.example.com:3478"},
		{
			"stun:s1.example.com:3478", "stun:s2.example.com:3478", "stun:s3.example.com:3478",
			"stun:s4.example.com:3478", "stun:s5.example.com:3478", "stun:s6.example.com:3478",
			"stun:s7.example.com:3478", "stun:s8.example.com:3478", "stun:s9.example.com:3478",
		},
	}
	for _, servers := range badCases {
		if _, err := (P2PConfig{Enabled: true, PortMapping: true, STUNServers: servers}).Validated(); err == nil {
			t.Fatalf("expected error for %v", servers)
		}
	}
	empty, err := (P2PConfig{Enabled: true, PortMapping: false, STUNServers: []string{}}).Validated()
	if err != nil || len(empty.STUNServers) != 0 {
		t.Fatalf("explicit empty STUN list should remain empty (host-only ICE): %+v, %v", empty, err)
	}
}
