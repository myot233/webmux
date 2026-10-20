package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestFixtureIsolatesAPIGitAndRestoresEnvironment(t *testing.T) {
	host := t.TempDir()
	hooks := filepath.Join(host, "hooks")
	if err := os.Mkdir(hooks, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(host, "hook-ran")
	if err := os.WriteFile(filepath.Join(hooks, "post-checkout"), []byte("#!/bin/sh\nprintf hook >> \"$BASELINE_HOST_HOOK_MARKER\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	global := filepath.Join(host, "global.gitconfig")
	if err := os.WriteFile(global, []byte(fmt.Sprintf("[core]\n\thooksPath = %q\n", hooks)), 0600); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"HOME": host, "XDG_CONFIG_HOME": filepath.Join(host, "config"),
		"XDG_STATE_HOME": filepath.Join(host, "state"), "XDG_CACHE_HOME": filepath.Join(host, "cache"),
		"GIT_CONFIG_GLOBAL": global, "GIT_CONFIG_COUNT": "1", "GIT_CONFIG_KEY_0": "core.hooksPath", "GIT_CONFIG_VALUE_0": hooks,
		"BASELINE_HOST_HOOK_MARKER": marker,
	} {
		t.Setenv(key, value)
	}
	f := newFixture()
	t.Cleanup(f.close)
	for _, key := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		path := os.Getenv(key)
		rel, err := filepath.Rel(f.root, path)
		if err != nil || rel == ".." || filepath.IsAbs(rel) || len(rel) >= 3 && rel[:3] == "../" {
			t.Errorf("%s not isolated under fixture: %s", key, path)
		}
	}
	m := f.worktrees(2)
	if m.Summary.Passed != 2 {
		t.Errorf("API worktrees failed: %+v", m)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("inherited post-checkout hook executed: %v", err)
	}
	f.close()
	if os.Getenv("HOME") != host || os.Getenv("GIT_CONFIG_GLOBAL") != global || os.Getenv("GIT_CONFIG_COUNT") != "1" {
		t.Fatal("fixture did not restore caller environment")
	}
}
