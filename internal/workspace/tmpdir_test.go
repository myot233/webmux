package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	os.Exit(runWorkspaceTests(m))
}

func runWorkspaceTests(m *testing.M) (status int) {
	// macOS paths must agree with canonical session/worktree paths.
	if dir, err := filepath.EvalSymlinks(os.TempDir()); err == nil {
		_ = os.Setenv("TMPDIR", dir)
	}
	home, err := os.MkdirTemp("", "webmux-workspace-test-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() {
		if err := os.RemoveAll(home); err != nil {
			fmt.Fprintln(os.Stderr, err)
			status = 1
		}
	}()
	// Real Git fixtures never write global config. Set the common isolated
	// environment once, so independent repositories can run in parallel.
	// Tests that override PATH or hook variables stay serial via t.Setenv.
	for key, value := range map[string]string{
		"HOME":                home,
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_CONFIG_GLOBAL":   os.DevNull,
		"GIT_AUTHOR_NAME":     "Workspace test",
		"GIT_AUTHOR_EMAIL":    "workspace@example.invalid",
		"GIT_COMMITTER_NAME":  "Workspace test",
		"GIT_COMMITTER_EMAIL": "workspace@example.invalid",
	} {
		if err := os.Setenv(key, value); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	return m.Run()
}
