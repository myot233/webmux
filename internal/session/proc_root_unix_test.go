//go:build linux || darwin

package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRootProcessReportsExecutableAndResolvedCwd(t *testing.T) {
	target := filepath.Join(t.TempDir(), "project", "sub")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sleep", "30")
	cmd.Dir = link
	cmd.Env = append(os.Environ(), "PWD="+link)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	exe, cwd := rootProcess(cmd.Process.Pid)
	if exe != "sleep" || cwd != want {
		t.Fatalf("rootProcess = %q, %q; want sleep, %q", exe, cwd, want)
	}
}

func TestRootProcessGone(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if exe, cwd := rootProcess(cmd.Process.Pid); exe != "" || cwd != "" {
		t.Fatalf("rootProcess of exited pid = %q, %q", exe, cwd)
	}
}
