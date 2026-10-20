//go:build linux || darwin

package session

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLeaderExitWithInheritedPTY(t *testing.T) {
	m := NewManager(Options{DefaultCwd: t.TempDir(), Presets: []Preset{{Name: "child", Cmd: []string{"/bin/bash", "-c", "trap '' HUP; sleep 30 & printf 'CHILD_STARTED\\n'; exit 4"}}}})
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		m.Shutdown(ctx)
	}()
	s, err := m.Create("", "child", nil, nil, "")
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
	for pid, state := range processes() {
		if state.sid == s.cmd.Process.Pid {
			t.Fatalf("session descendant still exists: pid %d state %c", pid, state.state)
		}
	}
}

func TestCloseReapsSeparateJobGroups(t *testing.T) {
	m := NewManager(Options{DefaultCwd: t.TempDir(), Presets: []Preset{{Name: "job", Cmd: []string{"/bin/bash", "-c", `set -m; (trap '' HUP; printf 'CHILD_READY\n'; exec sleep 30) & wait`}}}})
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
	defer cancel()
	defer m.Shutdown(ctx)
	s, err := m.Create("", "job", nil, nil, "")
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
	m := NewManager(Options{DefaultCwd: t.TempDir(), Presets: []Preset{{Name: "blocked", Cmd: []string{"/bin/bash", "-c", `stty raw -echo; printf READY; sleep 30`}}}})
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	}()
	s, err := m.Create("", "blocked", nil, nil, "")
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
	if err = s.Resize(100, 40); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	timer := time.AfterFunc(100*time.Millisecond, cancel)
	defer timer.Stop()
	start := time.Now()
	err = s.WriteContext(ctx, bytes.Repeat([]byte("x"), 512*1024))
	if !errors.Is(err, context.Canceled) || time.Since(start) > 2*time.Second {
		t.Fatalf("uncancelled write: %v", err)
	}
	next, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop()
	start = time.Now()
	_ = s.WriteContext(next, []byte("y"))
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

func TestRealProcessTreeWaiting(t *testing.T) {
	m := NewManager(Options{DefaultCwd: t.TempDir(), Presets: []Preset{
		{Name: "reader", Cmd: []string{"/bin/bash", "-c", `printf READY; read -r value`}},
		{Name: "busy", Cmd: []string{"/bin/bash", "-c", `printf READY; while :; do :; done`}},
	}})
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	}()
	reader, err := m.Create("", "reader", nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	busy, err := m.Create("", "busy", nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	waitForOutput(t, reader, "READY")
	waitForOutput(t, busy, "READY")
	deadline := time.Now().Add(2 * time.Second)
	for !newProcessSnapshot(processes()).foregroundBlocked(reader.cmd.Process.Pid) {
		if time.Now().After(deadline) {
			t.Fatal("shell blocked on terminal input was not detected as waiting")
		}
		time.Sleep(20 * time.Millisecond)
	}
	for range 5 {
		if newProcessSnapshot(processes()).foregroundBlocked(busy.cmd.Process.Pid) {
			t.Fatal("busy loop was classified as waiting")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
