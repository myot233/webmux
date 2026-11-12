package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/p2p"
	"github.com/rainchan/webmux/internal/p2psettings"
)

func TestP2PControllerSettingsLifecycle(t *testing.T) {
	dir := t.TempDir()
	defaults := config.P2PConfig{
		Enabled:     true,
		PortMapping: true,
		STUNServers: config.DefaultSTUNServers(),
	}
	store := p2psettings.Open(dir, defaults)
	defer store.Close()
	svc := p2p.New(p2p.Options{
		AppVersion:  "test",
		Enabled:     defaults.Enabled,
		PortMapping: false,
		STUNServers: defaults.STUNServers,
	})
	defer svc.Close()

	ctl := newP2PController(store, svc)
	initial := ctl.Status()
	if initial.Source != "config" || !initial.StorageAvailable {
		t.Fatalf("unexpected initial status: %+v", initial)
	}

	updated, err := ctl.UpdateSettings(contract.P2PSettingsUpdate{
		Enabled:     false,
		PortMapping: false,
		STUNServers: []string{"stun:stun.cloudflare.com:3478"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Enabled || updated.Source != "web" || len(updated.STUNServers) != 1 {
		t.Fatalf("unexpected updated status: %+v", updated)
	}
	if ctl.Enabled() {
		t.Fatal("expected controller Enabled() to be false after disabling P2P")
	}

	reset, err := ctl.ResetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if reset.Source != "config" || !slices.Equal(reset.STUNServers, defaults.STUNServers) {
		t.Fatalf("unexpected reset status: %+v", reset)
	}
}

func TestConfiguredViewCorruptP2POverrideFailsClosed(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	file := filepath.Join(root, "p2p-settings.json")
	if err := os.WriteFile(path, []byte(fmt.Sprintf("state_dir = %q\n[p2p]\nenabled = true\nport_mapping = true\n", root)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(`{"v":1,broken`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := configShow([]string{"--config", path, "--json"}, &out, &stderr); code != 0 {
		t.Fatalf("configShow exit %d: %s", code, stderr.String())
	}
	var v contract.Configuration
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Fields["p2p.enabled"].Value != false || v.Fields["p2p.port_mapping"].Value != false {
		t.Fatalf("corrupt p2p-settings.json did not fail closed: %+v", v.Fields)
	}
}
