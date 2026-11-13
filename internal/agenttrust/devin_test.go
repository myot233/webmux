package agenttrust

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readTrustedPaths(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON %q: %v", data, err)
	}
	return doc
}

func TestPrepareDevinCreatesTrustFile(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "p")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(home, ".local", "share", "devin", "cli", "trusted_workspaces.json")
	trust := newTrust(home, nil)
	argv := []string{"bash", "-ic", `exec -- "$@"`, "webmux", "devin"}
	trust.Prepare(work, argv, nil)
	trust.Prepare(work, argv, nil)

	doc := readTrustedPaths(t, config)
	paths, _ := doc["trusted_paths"].([]any)
	if len(paths) != 1 || paths[0] != canonical(t, work) {
		t.Fatalf("config = %v", doc)
	}
	if info, _ := os.Stat(config); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode())
	}
}

func TestPrepareDevinAppendsKeepingOtherFields(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "p")
	dir := filepath.Join(home, ".local", "share", "devin", "cli")
	for _, d := range []string{work, dir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join(dir, "trusted_workspaces.json")
	if err := os.WriteFile(config, []byte(`{"other":1,"trusted_paths":["/old"]}`), 0o640); err != nil {
		t.Fatal(err)
	}
	newTrust(home, nil).Prepare(work, []string{"devin"}, nil)

	doc := readTrustedPaths(t, config)
	paths, _ := doc["trusted_paths"].([]any)
	if doc["other"] != float64(1) || len(paths) != 2 || paths[0] != "/old" || paths[1] != canonical(t, work) {
		t.Fatalf("config = %v", doc)
	}
	if info, _ := os.Stat(config); info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v", info.Mode())
	}
}

func TestPrepareDevinHonoursXDGDataHome(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "p")
	fromEnv := filepath.Join(home, "xdg-env")
	fromServer := filepath.Join(home, "xdg-server")
	for _, d := range []string{work, fromEnv, fromServer} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	trust := newTrust(home, map[string]string{"XDG_DATA_HOME": fromServer})
	trust.Prepare(work, []string{"devin"}, map[string]string{"XDG_DATA_HOME": fromEnv})
	if doc := readTrustedPaths(t, filepath.Join(fromEnv, "devin", "cli", "trusted_workspaces.json")); doc["trusted_paths"] == nil {
		t.Fatalf("session XDG_DATA_HOME config = %v", doc)
	}
	trust.Prepare(work, []string{"devin"}, nil)
	if doc := readTrustedPaths(t, filepath.Join(fromServer, "devin", "cli", "trusted_workspaces.json")); doc["trusted_paths"] == nil {
		t.Fatalf("server XDG_DATA_HOME config = %v", doc)
	}
}

func TestPrepareDevinRefusesBroadDirectories(t *testing.T) {
	parent := t.TempDir()
	home := filepath.Join(parent, "home")
	if err := os.Mkdir(home, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{home, parent, "/"} {
		newTrust(home, nil).Prepare(dir, []string{"devin"}, nil)
	}
	config := filepath.Join(home, ".local", "share", "devin", "cli", "trusted_workspaces.json")
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatalf("broad directory trusted: %v", err)
	}
}
