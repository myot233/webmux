//go:build linux || darwin

package session

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/contract"
)

func relationsManager(t *testing.T, o Options) *Manager {
	t.Helper()
	if o.DefaultCwd == "" {
		o.DefaultCwd = t.TempDir()
	}
	m := NewManager(o)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})
	return m
}

func TestValidateLabels(t *testing.T) {
	many := map[string]string{}
	for i := range MaxLabels + 1 {
		many["k"+strings.Repeat("x", i)] = "v"
	}
	for _, tc := range []struct {
		name   string
		labels map[string]string
		ok     bool
	}{
		{"empty", nil, true},
		{"simple", map[string]string{"run": "42", "task.name": "lint", "a-b_c": ""}, true},
		{"uppercase", map[string]string{"Task": "x"}, false},
		{"space", map[string]string{"task name": "x"}, false},
		{"leading dash", map[string]string{"-x": "x"}, false},
		{"long key", map[string]string{strings.Repeat("a", 64): "x"}, false},
		{"control value", map[string]string{"k": "a\nb"}, false},
		{"long value", map[string]string{"k": strings.Repeat("值", MaxLabelValueRunes+1)}, false},
		{"max value", map[string]string{"k": strings.Repeat("值", MaxLabelValueRunes)}, true},
		{"too many", many, false},
	} {
		err := validateLabels(tc.labels)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err = %v, want ok=%v", tc.name, err, tc.ok)
		}
		if err != nil && !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v is not ErrInvalid", tc.name, err)
		}
	}
}

func TestUpdateLabelsAndParent(t *testing.T) {
	m := relationsManager(t, Options{})
	parent, err := m.Create(CreateInput{Cmd: []string{"/bin/sh", "-c", "sleep 30"}})
	if err != nil {
		t.Fatal(err)
	}
	child, err := m.Create(CreateInput{Cmd: []string{"/bin/sh", "-c", "sleep 30"}})
	if err != nil {
		t.Fatal(err)
	}
	sub := m.Subscribe()
	defer m.Unsubscribe(sub)

	info, err := m.UpdateLabels(child.info.ID, map[string]string{"run": "1", "task": "lint"}, []string{"task"})
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Labels) != 1 || info.Labels["run"] != "1" {
		t.Fatalf("labels = %v", info.Labels)
	}
	// An invalid change applies nothing.
	if _, err := m.UpdateLabels(child.info.ID, map[string]string{"ok": "1", "Bad": "x"}, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid label: %v", err)
	}
	if got := child.Info().Labels; len(got) != 1 {
		t.Fatalf("invalid change applied: %v", got)
	}
	// The returned copy is not the session's map.
	info.Labels["run"] = "changed"
	if child.Info().Labels["run"] != "1" {
		t.Fatal("Info shares the labels map")
	}
	if _, err := m.UpdateLabels("missing1", map[string]string{"a": "b"}, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing session: %v", err)
	}
	// Removing the last label omits the map.
	info, err = m.UpdateLabels(child.info.ID, nil, []string{"run"})
	if err != nil || info.Labels != nil {
		t.Fatalf("labels after removing all = %v, %v", info.Labels, err)
	}

	m.SetParent(child.info.ID, parent.info.ID)
	m.SetParent(child.info.ID, "other123") // set once
	if got := child.Info().ParentID; got != parent.info.ID {
		t.Fatalf("parent = %q", got)
	}
	if got := m.Children(parent.info.ID); !slices.Equal(got, []string{child.info.ID}) {
		t.Fatalf("children = %v", got)
	}
	m.SetParent(parent.info.ID, parent.info.ID)
	if parent.Info().ParentID != "" {
		t.Fatal("a session became its own parent")
	}

	deadline := time.After(5 * time.Second)
	for {
		select {
		case e := <-sub.Out:
			if e.Type == "updated" && e.Session != nil && e.Session.ParentID == parent.info.ID {
				return
			}
		case <-deadline:
			t.Fatal("no updated event carried the parent")
		}
	}
}

func TestSessionEnvAdvertisesInstance(t *testing.T) {
	dir := t.TempDir()
	m := relationsManager(t, Options{DefaultCwd: dir, Executable: "/opt/webmux/bin/webmux"})
	m.SetSocket("/run/user/1000/webmux/webmux.sock")
	s, err := m.Create(CreateInput{Cmd: []string{"/bin/sh", "-c", "env > env.txt; sleep 30"}})
	if err != nil {
		t.Fatal(err)
	}
	var env []byte
	deadline := time.Now().Add(5 * time.Second)
	for {
		env, err = os.ReadFile(dir + "/env.txt")
		if err == nil && strings.Contains(string(env), "WEBMUX_SESSION_ID") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session never wrote its environment: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	lines := strings.Split(string(env), "\n")
	for _, want := range []string{
		contract.SocketEnv + "=/run/user/1000/webmux/webmux.sock",
		contract.BinEnv + "=/opt/webmux/bin/webmux",
		contract.SessionIDEnv + "=" + s.info.ID,
	} {
		if !slices.Contains(lines, want) {
			t.Errorf("session environment lacks %s", want)
		}
	}
}

func TestSessionEnvOmitsUnknownInstance(t *testing.T) {
	env := sessionEnvWithVersion([]string{"WEBMUX_SOCKET=/elsewhere", "WEBMUX_BIN=/old", "HOME=/h"}, "abcdefgh", false, nil, "1", nil)
	for _, entry := range env {
		if strings.HasPrefix(entry, contract.SocketEnv+"=") || strings.HasPrefix(entry, contract.BinEnv+"=") {
			t.Fatalf("inherited %s from the server's own environment", entry)
		}
	}
}

func TestValidateEnvReservesInstanceVariables(t *testing.T) {
	for _, name := range []string{contract.SessionIDEnv, contract.SocketEnv, contract.BinEnv} {
		if err := ValidateEnv(map[string]string{name: "x"}); err == nil {
			t.Errorf("%s accepted in a create request", name)
		}
	}
	if !contract.ServerOwnedEnv(contract.SocketEnv) || !contract.ServerOwnedEnv(contract.BinEnv) {
		t.Fatal("instance variables are not server-owned; run would forward them")
	}
}

func TestProcessSessionFindsCreator(t *testing.T) {
	if os.Getenv(contract.SessionIDEnv) != "" {
		t.Skip("the real parent belongs to the outer WebMux session")
	}
	m := relationsManager(t, Options{})
	s, err := m.Create(CreateInput{Cmd: []string{"/bin/sh", "-c", "sleep 30"}})
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	leader := s.run.cmd.Process.Pid
	s.mu.Unlock()
	if id, inside := m.ProcessSession(leader); !inside || id != s.info.ID {
		t.Fatalf("leader: id %q inside %v", id, inside)
	}
	// The test binary's parent is outside every session; a child of the test
	// would count as an orphan the "server" adopted.
	if id, inside := m.ProcessSession(os.Getppid()); inside || id != "" {
		t.Fatalf("outside process: id %q inside %v", id, inside)
	}
}

// A stale snapshot published after a fresh labels or parent event must look
// different to the next tick, so the list is repaired.
func TestSameDisplayComparesLabelsAndParent(t *testing.T) {
	base := Info{ID: "abcdefgh", Labels: map[string]string{"run": "1"}, ParentID: "p4rent22"}
	if !sameDisplay(base, base) {
		t.Fatal("identical infos differ")
	}
	for name, other := range map[string]Info{
		"label value":  {ID: "abcdefgh", Labels: map[string]string{"run": "2"}, ParentID: "p4rent22"},
		"no labels":    {ID: "abcdefgh", ParentID: "p4rent22"},
		"other parent": {ID: "abcdefgh", Labels: map[string]string{"run": "1"}},
	} {
		if sameDisplay(base, other) {
			t.Errorf("%s: change not seen", name)
		}
	}
}
