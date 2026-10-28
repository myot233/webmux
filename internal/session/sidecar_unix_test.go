//go:build linux || darwin

package session

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func waitSidecarDone(t *testing.T, c *Sidecar) {
	t.Helper()
	select {
	case <-c.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("sidecar did not finish")
	}
}

func TestSidecarPipesAndStop(t *testing.T) {
	t.Parallel()
	m := shellManager(t)
	s := createShell(t, m, nil, "/bin/sleep", "30")
	sidecar, err := s.StartSidecar([]string{"/bin/sh", "-c", `echo "$WEBMUX_SESSION_ID $PWD" >&2; exec cat`}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(sidecar.Stdin(), "ping\n"); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(sidecar.Stdout()).ReadString('\n')
	if err != nil || line != "ping\n" {
		t.Fatalf("stdout %q, %v", line, err)
	}
	if code := sidecar.ExitCode(); code != -1 {
		t.Fatalf("exit code %d before Done", code)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = sidecar.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if code := sidecar.ExitCode(); code != 0 {
		t.Fatalf("cat after stdin closed: exit %d", code)
	}
	info := s.Info()
	cwd, _ := filepath.EvalSymlinks(info.Cwd)
	if stderr := strings.TrimSpace(string(sidecar.Stderr())); stderr != info.ID+" "+info.Cwd && stderr != info.ID+" "+cwd {
		t.Fatalf("sidecar environment %q, want session %s in %s", stderr, info.ID, info.Cwd)
	}
	// Stopping a sidecar follows its own tree, not the shared session marker.
	if info.Status == StatusExited {
		t.Fatal("stopping the sidecar ended the PTY run")
	}
	if err = m.Remove(info.ID); err != ErrRunning {
		t.Fatalf("remove with a live run: %v", err)
	}
}

func TestShutdownKillsSidecars(t *testing.T) {
	t.Parallel()
	m := NewManager(Options{DefaultCwd: t.TempDir()})
	s := createShell(t, m, nil)
	sidecar, err := s.StartSidecar([]string{"/bin/sh", "-c", "sleep 60 & echo $!; wait"}, false)
	if err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(sidecar.Stdout()).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	child, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Remove(s.Info().ID); err != ErrRunning {
		t.Fatalf("remove with a live sidecar: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m.Shutdown(ctx)
	waitSidecarDone(t, sidecar)
	for _, pid := range []int{sidecar.pid, child} {
		deadline := time.Now().Add(2 * time.Second)
		for !processGone(pid) {
			if time.Now().After(deadline) {
				t.Fatalf("sidecar process %d survived shutdown", pid)
			}
			reapAdopted(processes())
			time.Sleep(20 * time.Millisecond)
		}
	}
	if _, err = s.StartSidecar([]string{"/bin/true"}, false); err != ErrClosed {
		t.Fatalf("sidecar after shutdown: %v", err)
	}
}

func TestSidecarBashrcOutputStaysOffProtocol(t *testing.T) {
	t.Parallel()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not installed")
	}
	home := t.TempDir()
	if err = os.WriteFile(filepath.Join(home, ".bashrc"), []byte("echo BASHRC_NOISE\nexport FROM_BASHRC=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := shellManager(t)
	s, err := m.CreateShell(ShellInput{CreateInput: CreateInput{Env: map[string]string{"HOME": home}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sidecar, err := s.StartSidecar([]string{bash, "-ic", `exec -- "$@" 1>&3 3>&-`, "webmux", "/bin/sh", "-c", `printf "PROTOCOL:$FROM_BASHRC"`}, true)
	if err != nil {
		t.Fatal(err)
	}
	protocol, err := io.ReadAll(sidecar.Stdout())
	if err != nil {
		t.Fatal(err)
	}
	waitSidecarDone(t, sidecar)
	if string(protocol) != "PROTOCOL:1" {
		t.Fatalf("protocol stdout %q", protocol)
	}
	if !strings.Contains(string(sidecar.Stderr()), "BASHRC_NOISE") {
		t.Fatalf(".bashrc did not run or its output was lost: %q", sidecar.Stderr())
	}
	if code := sidecar.ExitCode(); code != 0 {
		t.Fatalf("exit %d", code)
	}
}

func TestSidecarStderrIsBounded(t *testing.T) {
	t.Parallel()
	m := shellManager(t)
	s := createShell(t, m, nil)
	sidecar, err := s.StartSidecar([]string{"/bin/sh", "-c", "head -c 200000 /dev/zero | tr '\\0' e >&2; printf END >&2"}, false)
	if err != nil {
		t.Fatal(err)
	}
	waitSidecarDone(t, sidecar)
	stderr := sidecar.Stderr()
	if len(stderr) != sidecarStderrCapacity || !strings.HasSuffix(string(stderr), "END") {
		t.Fatalf("stderr tail: %d bytes, suffix %q", len(stderr), stderr[max(0, len(stderr)-3):])
	}
}
