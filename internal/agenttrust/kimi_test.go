package agenttrust

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareKimiTrustsOnlyCurrentWorkspace(t *testing.T) {
	for _, argv := range [][]string{
		{"kimi", "--auto"},
		{"/home/u/.kimi-code/bin/kimi"},
		{"bash", "-ic", `exec -- "$@"`, "webmux", "kimi", "--auto"},
		{"zsh", "-ic", `exec -- "$@"`, "webmux", "/opt/kimi/bin/kimi"},
	} {
		home := t.TempDir()
		work := filepath.Join(home, "project")
		if err := os.Mkdir(work, 0o755); err != nil {
			t.Fatal(err)
		}
		trust := newTrust(home, nil)
		env := trust.Prepare(work, argv, nil)
		if env[kimiTrustWorkspaceEnv] != "1" {
			t.Fatalf("Prepare(%q) env = %v", argv, env)
		}
		env = trust.Prepare(work, argv, env)
		if env[kimiTrustWorkspaceEnv] != "1" {
			t.Fatalf("second Prepare(%q) env = %v", argv, env)
		}
		if _, err := os.Stat(filepath.Join(home, ".kimi-code", "config.toml")); !os.IsNotExist(err) {
			t.Fatalf("Prepare(%q) created persistent Kimi config: %v", argv, err)
		}
	}
}

func TestPrepareKimiRespectsExplicitTrustChoice(t *testing.T) {
	for _, tc := range []struct {
		name      string
		session   map[string]string
		server    map[string]string
		wantValue string
		wantSet   bool
	}{
		{name: "session false", session: map[string]string{kimiTrustWorkspaceEnv: "0"}, wantValue: "0", wantSet: true},
		{name: "session empty", session: map[string]string{kimiTrustWorkspaceEnv: ""}, wantValue: "", wantSet: true},
		{name: "server false", server: map[string]string{kimiTrustWorkspaceEnv: "0"}},
		{name: "server empty", server: map[string]string{kimiTrustWorkspaceEnv: ""}},
		{name: "server true", server: map[string]string{kimiTrustWorkspaceEnv: "1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			work := filepath.Join(home, "project")
			if err := os.Mkdir(work, 0o755); err != nil {
				t.Fatal(err)
			}
			env := newTrust(home, tc.server).Prepare(work, []string{"kimi"}, tc.session)
			value, ok := env[kimiTrustWorkspaceEnv]
			if ok != tc.wantSet || value != tc.wantValue {
				t.Fatalf("env = %v, want value %q set %v", env, tc.wantValue, tc.wantSet)
			}
		})
	}
}

func TestPrepareKimiRefusesBroadDirectories(t *testing.T) {
	parent := t.TempDir()
	home := filepath.Join(parent, "home")
	if err := os.Mkdir(home, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{home, parent, "/"} {
		env := newTrust(home, nil).Prepare(dir, []string{"kimi"}, nil)
		if _, ok := env[kimiTrustWorkspaceEnv]; ok {
			t.Fatalf("trusted broad directory %q", dir)
		}
	}
}
