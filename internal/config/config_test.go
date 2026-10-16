package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCustomPresetNameIsReserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "[[presets]]\nname = \"custom\"\nlabel = \"Configured command\"\ncmd = [\"/bin/echo\"]\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("custom preset validation = %v, want reserved-name error", err)
	}
}

func TestStateDirectoryDefaultsAndOptionalStorage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	if got := DefaultStateDir(); got != filepath.Join(home, ".local", "state", "webmux") {
		t.Fatalf("home state directory: %q", got)
	}
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	c, err := Load(filepath.Join(t.TempDir(), "absent"), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.StateDir != filepath.Join(state, "webmux") {
		t.Fatalf("XDG state directory: %q", c.StateDir)
	}
	c.StateDir = "/missing/unwritable/state"
	if err := c.Validate(); err != nil {
		t.Fatalf("optional storage prevented startup: %v", err)
	}
}

func TestAttentionSignalOptions(t *testing.T) {
	dir := t.TempDir()
	for _, row := range []struct {
		name, body                  string
		disableProtocol, disableBel bool
	}{
		{"defaults", "", false, false},
		{"legacy file", "idle_after_seconds = 30\n", false, false},
		{"disabled", "agent_protocol = false\nbell_attention = false\n", true, true},
		{"protocol only off", "agent_protocol = false\n", true, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			path := filepath.Join(dir, row.name+".toml")
			if err := os.WriteFile(path, []byte(row.body), 0600); err != nil {
				t.Fatal(err)
			}
			c, err := Load(path, true)
			if err != nil {
				t.Fatal(err)
			}
			if err := c.Validate(); err != nil {
				t.Fatal(err)
			}
			o := c.SessionOptions()
			if o.DisableAgentProtocol != row.disableProtocol || o.DisableBellAttention != row.disableBel {
				t.Fatalf("options = %+v", o)
			}
		})
	}
}
