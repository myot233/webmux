package api

import (
	"os"
	"path/filepath"
	"testing"
)

// macOS places TMPDIR behind the /var -> /private/var symlink, while sessions
// and worktrees report canonical paths. Compare against the canonical form.
func TestMain(m *testing.M) {
	if dir, err := filepath.EvalSymlinks(os.TempDir()); err == nil {
		_ = os.Setenv("TMPDIR", dir)
	}
	// Tests may point TMPDIR and HOME at their own temporary directories;
	// the fake agent is built once for the whole run, in this environment.
	fakeAgent.base, fakeAgent.env = os.TempDir(), os.Environ()
	code := m.Run()
	if fakeAgent.dir != "" {
		_ = os.RemoveAll(fakeAgent.dir)
	}
	os.Exit(code)
}
