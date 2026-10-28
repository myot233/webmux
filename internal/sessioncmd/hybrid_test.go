//go:build linux || darwin

package sessioncmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/api"
	"github.com/rainchan/webmux/internal/codeview"
	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/files"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/localclient"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/workspace"
)

// realServer serves the local API with a hybrid service on a Unix socket
// and returns an Env talking to it, the manager and the executable "omp"
// script standing in for the agent's terminal UI.
func realServer(t *testing.T) (Env, *bytes.Buffer, *bytes.Buffer, *session.Manager, string) {
	t.Helper()
	cwd := t.TempDir()
	agent := filepath.Join(t.TempDir(), "omp")
	if err := os.WriteFile(agent, []byte("#!/bin/sh\nexec sleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := session.NewManager(session.Options{DefaultCwd: cwd})
	launches := launch.Open(t.TempDir())
	if _, err := launches.Create(launch.Input{Name: "omp-hybrid", Cmd: []string{agent}, Hybrid: true}); err != nil {
		t.Fatal(err)
	}
	hybrids := hybrid.New(hybrid.Options{StateDir: t.TempDir(), Manager: m})
	ws := workspace.New(t.TempDir(), m, launches, cwd)
	ws.UseHybrid(hybrids)
	codes := codeview.New(m)
	handler := api.NewLocal(api.Deps{
		Config: config.Config{DefaultCwd: cwd}, Manager: m, Workspace: ws, Code: codes, Files: files.New(cwd), Launch: launches, Hybrid: hybrids,
	}, api.Local{Status: func() contract.StatusV1 {
		return contract.StatusV1{V: 1, URLs: contract.URLs{Local: "http://127.0.0.1:7681"}}
	}})
	dir, err := os.MkdirTemp("/tmp", "scm")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "s.sock")
	l, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() {
		_ = srv.Close()
		hybrids.Close()
		ws.Close()
		codes.Close()
		launches.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		m.Shutdown(ctx)
		_ = os.RemoveAll(dir)
	})
	var stdout, stderr bytes.Buffer
	return Env{
		Connect:     func() (*localclient.Client, error) { return localclient.New(localclient.UnixDialer{Path: socket}), nil },
		Stdout:      &stdout,
		Stderr:      &stderr,
		OpenConsole: func() (Console, error) { return &fakeConsole{}, nil },
		ResolveCwd:  func(context.Context, string) (string, error) { return cwd, nil },
	}, &stdout, &stderr, m, agent
}

// `webmux run --launch-config` with a hybrid config creates a hybrid
// session on the terminal side, listed with its side; `run -- omp` stays an
// ordinary session.
func TestRunLaunchConfigStartsHybridSessionInTerminal(t *testing.T) {
	e, stdout, stderr, m, agent := realServer(t)
	ctx := context.Background()
	if code := Run(ctx, e, []string{"--detach", "--launch-config", "omp-hybrid"}); code != contract.ExitOK {
		t.Fatalf("run exit %d: %s", code, stderr.String())
	}
	id := strings.Fields(stdout.String())[0]
	s, err := m.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if info := s.Info(); info.Hybrid == nil || info.Hybrid.Side != string(hybrid.SideTUI) || info.Hybrid.State != string(hybrid.StateActive) || info.Hybrid.Agent != "omp" {
		t.Fatalf("created %+v hybrid %+v", info, info.Hybrid)
	}

	stdout.Reset()
	if code := List(ctx, e, []string{"--json"}); code != contract.ExitOK {
		t.Fatalf("ls exit %d: %s", code, stderr.String())
	}
	var list []contract.SessionInfo
	if err := json.Unmarshal(stdout.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != id || list[0].Hybrid == nil || list[0].Hybrid.Side != contract.HybridSideTUI {
		t.Fatalf("ls --json %s", stdout.String())
	}

	stdout.Reset()
	if code := Run(ctx, e, []string{"--detach", "--", agent}); code != contract.ExitOK {
		t.Fatalf("run -- omp exit %d: %s", code, stderr.String())
	}
	plain, err := m.Get(strings.Fields(stdout.String())[0])
	if err != nil {
		t.Fatal(err)
	}
	if info := plain.Info(); info.Hybrid != nil {
		t.Fatalf("direct command became hybrid: %+v", info.Hybrid)
	}
}

// An attached `run --launch-config` ends like a detach (exit 0) when the
// session moves to its chat side.
func TestAttachedHybridRunDetachesWhenMoved(t *testing.T) {
	e, _, stderr, m, _ := realServer(t)
	console := &heldConsole{}
	e.OpenConsole = func() (Console, error) { return console, nil }
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	done := make(chan int, 1)
	go func() { done <- Run(ctx, e, []string{"--launch-config", "omp-hybrid"}) }()
	var s *session.Session
	for s == nil || s.Info().Clients == 0 {
		select {
		case code := <-done:
			t.Fatalf("run ended early with %d: %s", code, stderr.String())
		case <-ctx.Done():
			t.Fatal("terminal never attached")
		case <-time.After(10 * time.Millisecond):
		}
		if list := m.List(); len(list) == 1 {
			s, _ = m.Get(list[0].ID)
		}
	}
	// A switch to the chat takes the terminal clients off first.
	s.MoveClients()
	select {
	case code := <-done:
		if code != contract.ExitOK || !console.restored || !strings.Contains(stderr.String(), "已切换到聊天视图") {
			t.Fatalf("moved run exit %d restored %v: %s", code, console.restored, stderr.String())
		}
	case <-ctx.Done():
		t.Fatal("run did not end after the move")
	}
	if _, err := m.Get(s.Info().ID); err != nil {
		t.Fatal("session ended with the local terminal")
	}
}
