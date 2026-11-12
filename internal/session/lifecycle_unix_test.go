//go:build linux || darwin

package session

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/procpeek"
)

func TestLeaderExitWithInheritedPTY(t *testing.T) {
	t.Parallel()
	m := NewManager(Options{DefaultCwd: t.TempDir()})
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		m.Shutdown(ctx)
	}()
	s, err := m.Create(CreateInput{Cwd: "", Cmd: []string{"/bin/bash", "-c", "trap '' HUP; sleep 30 & printf 'CHILD_STARTED\\n'; exit 4"}, Name: "", Env: nil, ConfigID: ""})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Done():
	case <-time.After(7 * time.Second):
		t.Fatal("leader exited but retained slave PTY prevented final exit")
	}
	if info := s.Info(); info.ExitCode == nil || *info.ExitCode != 4 {
		t.Fatalf("exit info: %+v", info)
	}
	assertNoSessionProcesses(t, s)
}

func assertNoSessionProcesses(t *testing.T, s *Session) {
	t.Helper()
	for pid, state := range procpeek.Processes() {
		if state.SID == s.run.cmd.Process.Pid {
			t.Fatalf("session descendant still exists: pid %d state %c", pid, state.State)
		}
	}
}

func TestCloseReapsSeparateJobGroups(t *testing.T) {
	t.Parallel()
	m := NewManager(Options{DefaultCwd: t.TempDir()})
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
	defer cancel()
	defer m.Shutdown(ctx)
	s, err := m.Create(CreateInput{Cwd: "", Cmd: []string{"/bin/bash", "-c", `set -m; (trap '' HUP; printf 'CHILD_READY\n'; exec sleep 30) & wait`}, Name: "", Env: nil, ConfigID: ""})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(string(s.Buffer()), "CHILD_READY") {
		if time.Now().After(deadline) {
			t.Fatal("child never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.Close()
	select {
	case <-s.Done():
	case <-ctx.Done():
		t.Fatal("close failed to terminate child job")
	}
	assertNoSessionProcesses(t, s)
}

func TestCancelledPTYWriteDoesNotHoldInputGate(t *testing.T) {
	t.Parallel()
	m := NewManager(Options{DefaultCwd: t.TempDir()})
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	}()
	s, err := m.Create(CreateInput{Cwd: "", Cmd: []string{"/bin/bash", "-c", `stty raw -echo; printf READY; cat >/dev/null`}, Name: "", Env: nil, ConfigID: ""})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(string(s.Buffer()), "READY") {
		if time.Now().After(deadline) {
			t.Fatal("terminal never entered raw mode")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err = s.ResizeScript(ScriptResize{Cols: 100, Rows: 40}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	timer := time.AfterFunc(100*time.Millisecond, cancel)
	defer timer.Stop()
	start := time.Now()
	err = s.WriteContext(ctx, bytes.Repeat([]byte("x"), 64*1024*1024))
	if !errors.Is(err, context.Canceled) || time.Since(start) > 2*time.Second {
		t.Fatalf("uncancelled write: %v", err)
	}
	next, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	start = time.Now()
	if err := s.WriteContext(next, []byte("y")); err != nil {
		t.Fatalf("cancelled writer retained serialization gate: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancelled writer retained serialization gate")
	}
}

func waitForOutput(t *testing.T, s *Session, text string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(string(s.Buffer()), text) {
		if time.Now().After(deadline) {
			t.Fatalf("output %q never appeared: %q", text, s.Buffer())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestUnattendedSleepIsNotUserWait(t *testing.T) {
	m := NewManager(Options{DefaultCwd: t.TempDir()})
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	}()
	sleeper, err := m.Create(CreateInput{Cwd: "", Cmd: []string{"/bin/bash", "--noprofile", "--norc", "-c", "printf 'background timer\\n'; sleep 60"}, Name: "", Env: nil, ConfigID: ""})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := m.Create(CreateInput{Cwd: "", Cmd: []string{"/bin/bash", "--noprofile", "--norc", "-c", "printf READY; read -r value"}, Name: "", Env: nil, ConfigID: ""})
	if err != nil {
		t.Fatal(err)
	}
	waitForOutput(t, sleeper, "background timer")
	waitForOutput(t, reader, "READY")
	deadline := time.Now().Add(3 * time.Second)
	var table procpeek.Table
	var snapshot procpeek.Snapshot
	for {
		table = procpeek.Processes()
		snapshot = procpeek.NewSnapshot(table)
		sleepQuiet := snapshot.ForegroundBlocked(sleeper.run.cmd.Process.Pid) && !snapshot.AwaitingInput(sleeper.run.cmd.Process.Pid)
		if sleepQuiet && snapshot.AwaitingInput(reader.run.cmd.Process.Pid) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("sleep/read wait not distinguished: sleep pid %d blocked=%t awaiting=%t %+v children=%v, read pid %d awaiting=%t %+v", sleeper.run.cmd.Process.Pid, snapshot.ForegroundBlocked(sleeper.run.cmd.Process.Pid), snapshot.AwaitingInput(sleeper.run.cmd.Process.Pid), table[sleeper.run.cmd.Process.Pid], directChildren(sleeper.run.cmd.Process.Pid, table), reader.run.cmd.Process.Pid, snapshot.AwaitingInput(reader.run.cmd.Process.Pid), table[reader.run.cmd.Process.Pid])
		}
		time.Sleep(20 * time.Millisecond)
	}
	now := time.Now()
	quiet := &Session{info: Info{LastActiveAt: now.Add(-4 * time.Second)}, run: &run{cmd: sleeper.run.cmd}}
	quiet.evaluate(now, snapshot, 30*time.Second, nil)
	if quiet.info.Status != StatusRunning {
		t.Fatalf("unattended sleep status = %s, want running", quiet.info.Status)
	}
	blocked := &Session{info: Info{LastActiveAt: now.Add(-4 * time.Second)}, run: &run{cmd: reader.run.cmd}}
	blocked.evaluate(now, snapshot, 30*time.Second, nil)
	if blocked.info.Status != StatusWaiting {
		t.Fatalf("terminal read status = %s, want waiting", blocked.info.Status)
	}
}

func directChildren(pid int, table procpeek.Table) map[int]procpeek.State {
	kids := map[int]procpeek.State{}
	for child, state := range table {
		if state.PPID == pid {
			kids[child] = state
		}
	}
	return kids
}

func TestRealProcessTreeWaiting(t *testing.T) {
	t.Parallel()
	m := NewManager(Options{DefaultCwd: t.TempDir()})
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	}()
	reader, err := m.Create(CreateInput{Cwd: "", Cmd: []string{"/bin/bash", "-c", `printf READY; read -r value`}, Name: "", Env: nil, ConfigID: ""})
	if err != nil {
		t.Fatal(err)
	}
	busy, err := m.Create(CreateInput{Cwd: "", Cmd: []string{"/bin/bash", "-c", `printf READY; while :; do :; done`}, Name: "", Env: nil, ConfigID: ""})
	if err != nil {
		t.Fatal(err)
	}
	waitForOutput(t, reader, "READY")
	waitForOutput(t, busy, "READY")
	deadline := time.Now().Add(2 * time.Second)
	for !procpeek.NewSnapshot(procpeek.Processes()).ForegroundBlocked(reader.run.cmd.Process.Pid) {
		if time.Now().After(deadline) {
			t.Fatal("shell blocked on terminal input was not detected as waiting")
		}
		time.Sleep(20 * time.Millisecond)
	}
	for range 5 {
		if procpeek.NewSnapshot(procpeek.Processes()).ForegroundBlocked(busy.run.cmd.Process.Pid) {
			t.Fatal("busy loop was classified as waiting")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestShutdownDuringRepeatedPreviewControls(t *testing.T) {
	t.Parallel()
	m := NewManager(Options{DefaultCwd: t.TempDir()})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})
	s, err := m.Create(CreateInput{Cwd: "", Cmd: []string{"/bin/sh", "-c",
		`while :; do printf '\033[100000Smarker\n'; sleep 0.01; done`,
	}, Name: "", Env: nil, ConfigID: ""})
	if err != nil {
		t.Fatal(err)
	}
	// The preview's count clamp must never rewrite the original PTY stream.
	waitForOutput(t, s, "\x1b[100000Smarker")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stopped := make(chan struct{})
	go func() {
		m.Shutdown(ctx)
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-ctx.Done():
		t.Fatal("preview output prevented bounded shutdown")
	}
	select {
	case <-s.Done():
	default:
		t.Fatal("session cleanup did not finish")
	}
}
