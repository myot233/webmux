package session

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateRejectsInvalidInput(t *testing.T) {
	manager := NewManager(Options{DefaultCwd: t.TempDir()})
	t.Cleanup(func() { manager.Shutdown(context.Background()) })
	for _, input := range []CreateInput{
		{Cwd: filepath.Join(t.TempDir(), "missing"), Cmd: []string{"/bin/sh"}},
		{Cmd: nil},
		{Cmd: []string{"/bin/sh"}, Env: map[string]string{"INVALID-NAME": "x"}},
	} {
		if _, err := manager.Create(input); !errors.Is(err, ErrInvalid) {
			t.Fatalf("input %+v: err = %v", input, err)
		}
	}
}

func TestCreateAfterShutdown(t *testing.T) {
	manager := NewManager(Options{DefaultCwd: t.TempDir()})
	manager.Shutdown(context.Background())
	if _, err := manager.Create(CreateInput{Cmd: []string{"/bin/sh"}}); !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("err = %v", err)
	}
}

func TestRenameOutranksTerminalTitle(t *testing.T) {
	cwd := t.TempDir()
	manager := NewManager(Options{DefaultCwd: cwd})
	t.Cleanup(func() { manager.Shutdown(context.Background()) })
	s, err := manager.Create(CreateInput{Cmd: []string{"/bin/sh", "-c", `sleep 0.3; printf '\033]0;from terminal\007'; sleep 30`}})
	if err != nil {
		t.Fatal(err)
	}
	id := s.Info().ID
	sub := manager.Subscribe()
	defer manager.Unsubscribe(sub)

	info, err := manager.Rename(id, "  my task ")
	if err != nil || info.Title != "my task" || info.PlaceholderTitle {
		t.Fatalf("rename: %+v %v", info, err)
	}
	if e := <-sub.Out; e.Type != "snapshot" {
		t.Fatalf("first event %+v", e)
	}
	if e := <-sub.Out; e.Type != "updated" || e.Session == nil || e.Session.Title != "my task" {
		t.Fatalf("list event %+v", e)
	}
	time.Sleep(700 * time.Millisecond)
	if got := s.Info().Title; got != "my task" {
		t.Fatalf("terminal title replaced the name: %q", got)
	}

	// An empty name hands the title back: the directory until the terminal
	// reports one again.
	info, err = manager.Rename(id, "")
	if err != nil || info.Title != filepath.Base(cwd) || !info.PlaceholderTitle {
		t.Fatalf("clear: %+v %v", info, err)
	}
}

func TestRenameRejectsInvalidNames(t *testing.T) {
	manager := NewManager(Options{DefaultCwd: t.TempDir()})
	t.Cleanup(func() { manager.Shutdown(context.Background()) })
	s, err := manager.Create(CreateInput{Cmd: []string{"/bin/sh", "-c", "sleep 30"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a\nb", "a\x1b[31m", strings.Repeat("长", MaxNameBytes), "\xff"} {
		if _, err := manager.Rename(s.Info().ID, name); !errors.Is(err, ErrInvalid) {
			t.Errorf("name %q: %v", name, err)
		}
	}
	if _, err := manager.Rename("missing1", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing session: %v", err)
	}
	if got := s.Info().Title; got == "" || strings.ContainsAny(got, "\n\x1b") {
		t.Fatalf("rejected rename changed the title to %q", got)
	}
}

func TestCreateRunsPreLaunchOnlyForValidLaunches(t *testing.T) {
	cwd := t.TempDir()
	type call struct {
		cwd  string
		argv []string
		env  map[string]string
	}
	var calls []call
	manager := NewManager(Options{DefaultCwd: cwd, PreLaunch: func(cwd string, argv []string, env map[string]string) map[string]string {
		calls = append(calls, call{cwd, argv, env})
		env["PRELAUNCH_VALUE"] = "present"
		return env
	}})
	t.Cleanup(func() { manager.Shutdown(context.Background()) })

	if _, err := manager.Create(CreateInput{Cwd: filepath.Join(cwd, "missing"), Cmd: []string{"/bin/sh"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("hook ran for an invalid directory: %+v", calls)
	}
	s, err := manager.Create(CreateInput{Cmd: []string{"/bin/sh", "-c", `test "$PRELAUNCH_VALUE" = present`}, Env: map[string]string{"A": "b"}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("session did not exit")
	}
	if info := s.Info(); info.ExitCode == nil || *info.ExitCode != 0 {
		t.Fatalf("prepared environment did not reach process: %+v", info)
	}
	if len(calls) != 1 || calls[0].cwd != cwd || calls[0].argv[0] != "/bin/sh" || calls[0].env["A"] != "b" {
		t.Fatalf("calls = %+v", calls)
	}
}
