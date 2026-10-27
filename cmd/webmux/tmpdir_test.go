package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// macOS places TMPDIR behind the /var -> /private/var symlink, while sessions
// and worktrees report canonical paths. Compare against the canonical form.
// WEBMUX_TEST_EXEC runs the test binary as the webmux executable: the value
// "argv" dispatches its real arguments, so a spawned command is exercised
// end to end; other values are a JSON-encoded argument list.
func TestMain(m *testing.M) {
	if encoded := os.Getenv("WEBMUX_TEST_EXEC"); encoded != "" {
		args := os.Args[1:]
		if encoded != "argv" {
			args = nil
			if err := json.Unmarshal([]byte(encoded), &args); err != nil {
				panic(err)
			}
		}
		_ = os.Unsetenv("WEBMUX_TEST_EXEC")
		os.Exit(dispatch(args, os.Stdout, os.Stderr))
	}
	if dir, err := filepath.EvalSymlinks(os.TempDir()); err == nil {
		_ = os.Setenv("TMPDIR", dir)
	}
	os.Exit(m.Run())
}
