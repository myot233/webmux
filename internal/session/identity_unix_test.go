//go:build linux || darwin

package session

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func resolvedBase(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Base(resolved)
}

func runPID(s *Session) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.run.cmd.Process.Pid
}

func TestIdentityOfOrdinarySessionClearsOnExit(t *testing.T) {
	t.Parallel()
	m := shellManager(t)
	script := `printf '\033]7501;state=idle:app=omp\007\033]777;notify;warp://cli-agent;{"v":1,"event":"session_start","session_id":"real-id"}\007READY'; read _`
	s, err := m.Create(CreateInput{Cmd: []string{"/bin/sh", "-c", script}})
	if err != nil {
		t.Fatal(err)
	}
	waitForOutput(t, s, "READY")
	identity, err := m.Identity(s.Info().ID)
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := filepath.EvalSymlinks(s.Info().Cwd)
	if err != nil {
		t.Fatal(err)
	}
	// macOS's /bin/sh is a shim that execs another shell, so the executable
	// name is only required to be read; TestRootProcessReportsExecutableAndResolvedCwd
	// pins it for a real binary.
	if identity.Executable == "" {
		t.Fatalf("identity %+v lacks the root executable", identity)
	}
	identity.Executable = ""
	want := Identity{ProgramApp: "omp", WarpSessionID: "real-id", PID: runPID(s), Cwd: cwd}
	if identity != want {
		t.Fatalf("identity %+v, want %+v", identity, want)
	}
	if err := s.Write([]byte("\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("session did not exit")
	}
	if identity := s.Identity(); identity != (Identity{}) {
		t.Fatalf("identity after exit: %+v", identity)
	}
	if _, err := m.Identity("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("identity of a missing session: %v", err)
	}
}

func TestIdentityBelongsToTheCurrentRun(t *testing.T) {
	t.Parallel()
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Fatal(err)
	}
	m := shellManager(t)
	s := createShell(t, m, newFakeDriver(), "/bin/sh", "-c", `printf '\033]7501;state=working:id=ses_first\007FIRST'; exec sleep 30`)
	waitForOutput(t, s, "FIRST")
	if identity := s.Identity(); identity.ProgramChildID != "ses_first" || identity.PID != runPID(s) {
		t.Fatalf("first run: %+v", identity)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.EndRun(ctx); err != nil {
		t.Fatal(err)
	}
	if identity := s.Identity(); identity != (Identity{}) {
		t.Fatalf("identity without a run: %+v", identity)
	}
	if err := s.StartRun([]string{"/bin/sh", "-c", "printf SECOND; exec sleep 30"}); err != nil {
		t.Fatal(err)
	}
	waitForOutput(t, s, "SECOND")
	// The root process becomes sleep once the shell has exec'd it.
	want := resolvedBase(t, sleep)
	deadline := time.Now().Add(5 * time.Second)
	for {
		identity := s.Identity()
		if identity.ProgramChildID != "" || identity.PID != runPID(s) {
			t.Fatalf("second run: %+v", identity)
		}
		if identity.Executable == want {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("second run executable: %+v, want %s", identity, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
