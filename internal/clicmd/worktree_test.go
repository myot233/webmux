package clicmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

func homeRepo() contract.Repository {
	return contract.Repository{ID: "r1", Availability: "available", Worktrees: []contract.Worktree{
		{ID: "main", Kind: "main"}, {ID: "a", Kind: "linked"}, {ID: "b", Kind: "linked"}, {ID: "c", Kind: "linked"},
	}}
}

// TestHomeStatusFollowsTheWebRules mirrors homeLayout in
// web/src/domains/workspace/home-worktrees.ts: an uncurated repository shows
// its main worktree, curated lists are taken as they are, and sessions or
// archived records always show a worktree.
func TestHomeStatusFollowsTheWebRules(t *testing.T) {
	repo := homeRepo()
	for _, tc := range []struct {
		name string
		h    home
		want map[string]string
	}{
		{"uncurated", home{}, map[string]string{"main": homePinned, "a": homeHidden, "b": homeHidden, "c": homeHidden}},
		{"curated", home{pins: map[string][]string{"r1": {"a"}}}, map[string]string{"main": homeHidden, "a": homePinned, "b": homeHidden}},
		{"curated empty", home{pins: map[string][]string{"r1": {}}}, map[string]string{"main": homeHidden, "a": homeHidden}},
		{"sessions and archived", home{sessions: map[string][]contract.SessionInfo{"b": {{ID: "s1"}}}, archived: map[string][]contract.SessionInfo{"c": {{ID: "h1"}, {ID: "h2"}}}},
			map[string]string{"main": homePinned, "a": homeHidden, "b": homeSessions, "c": homeSessions}},
		{"pinned beats sessions", home{pins: map[string][]string{"r1": {"b"}}, sessions: map[string][]contract.SessionInfo{"b": {{ID: "s1"}}}}, map[string]string{"b": homePinned}},
	} {
		for _, tree := range repo.Worktrees {
			want, ok := tc.want[tree.ID]
			if !ok {
				continue
			}
			if got := tc.h.status(repo, tree); got != want {
				t.Errorf("%s: %s is %s, want %s", tc.name, tree.ID, got, want)
			}
		}
	}
}

// TestArchivedWorktreeFollowsTheWeb mirrors the archived session placement
// of web/src/domains/sessions/archived.ts: a recorded association wins, and a
// session without one joins the deepest registered worktree containing its cwd.
func TestArchivedWorktreeFollowsTheWeb(t *testing.T) {
	repos := []contract.Repository{{ID: "r1", Worktrees: []contract.Worktree{
		{ID: "main", Path: "/w/repo"}, {ID: "nested", Path: "/w/repo/sub"},
	}}}
	for _, tc := range []struct {
		record contract.SessionInfo
		want   string
		ok     bool
	}{
		{contract.SessionInfo{Cwd: "/w/repo/x", Workspace: &contract.SessionWorkspace{RepositoryID: "r1", WorktreeID: "nested"}}, "nested", true},
		{contract.SessionInfo{Cwd: "/w/repo/sub/deep"}, "nested", true},
		{contract.SessionInfo{Cwd: "/w/repo"}, "main", true},
		{contract.SessionInfo{Cwd: "/w/repository"}, "", false},
	} {
		got, ok := archivedWorktree(tc.record, repos)
		if got != tc.want || ok != tc.ok {
			t.Errorf("%+v: %q %v, want %q %v", tc.record, got, ok, tc.want, tc.ok)
		}
	}
}

// TestWorktreeSyncColumns covers the upstream states the Web UI also
// distinguishes: none, gone, not counted, counted, and unknown status.
func TestWorktreeSyncColumns(t *testing.T) {
	clean := &contract.WorktreeChanges{}
	tests := []struct {
		name                string
		give                contract.Worktree
		wantState, wantName string
		wantChanges         string
	}{
		{"no upstream", contract.Worktree{Available: true, Changes: clean}, "-", "none", "clean"},
		{"gone", contract.Worktree{Available: true, Upstream: new("origin/x"), UpstreamGone: true, Changes: clean}, "gone", "origin/x (gone)", "clean"},
		{"not counted", contract.Worktree{Available: true, Upstream: new("origin/x"), Changes: clean}, "+?/-?", "origin/x", "clean"},
		{"counted", contract.Worktree{Available: true, Upstream: new("origin/x"), Ahead: new(2), Behind: new(0), Dirty: new(true),
			Changes: &contract.WorktreeChanges{Staged: 1, Unstaged: 2, Conflicted: 1}}, "changed,+2/-0", "origin/x", "1 staged, 2 unstaged, 1 conflicted"},
		{"unknown", contract.Worktree{Available: true}, "-", "unknown", "unknown"},
	}
	for _, tt := range tests {
		if got := worktreeState(tt.give); got != tt.wantState {
			t.Errorf("%s: state %q, want %q", tt.name, got, tt.wantState)
		}
		if got := upstreamName(tt.give); got != tt.wantName {
			t.Errorf("%s: upstream %q, want %q", tt.name, got, tt.wantName)
		}
		if got := changeCounts(tt.give.Changes); got != tt.wantChanges {
			t.Errorf("%s: changes %q, want %q", tt.name, got, tt.wantChanges)
		}
	}
	if got := commitLine(&contract.Commit{Hash: "0123456789abcdef", Subject: "fix\nbody"}); got != "0123456789ab fix" {
		t.Errorf("commit line %q", got)
	}
	if got := commitLine(nil); got != "-" {
		t.Errorf("no commit %q", got)
	}
}

// settingsServer is a /api/settings fake that rejects the first n PATCHes
// with 409 after another client changed another repository.
type settingsServer struct {
	mu        sync.Mutex
	revision  int64
	home      map[string][]string
	conflicts int
	patches   int
}

func (s *settingsServer) serve(t *testing.T) Env {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "scw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s.sock")
	l, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	// Pins are pruned against the repository list read on each attempt.
	mux.HandleFunc("GET /api/repositories", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]contract.Repository{homeRepo()})
	})
	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		_ = json.NewEncoder(w).Encode(contract.SettingsSnapshot{Revision: s.revision, Values: contract.SettingsValues{HomeWorktrees: s.home}})
	})
	mux.HandleFunc("PATCH /api/settings", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.patches++
		var patch contract.SettingsPatch
		_ = json.NewDecoder(r.Body).Decode(&patch)
		if s.conflicts > 0 {
			// Another browser changed another repository meanwhile.
			s.conflicts--
			s.revision++
			s.home["other"] = append(s.home["other"], "x")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"conflict"}`))
			return
		}
		if patch.BaseRevision == nil || *patch.BaseRevision != s.revision {
			w.WriteHeader(http.StatusConflict)
			return
		}
		_ = json.Unmarshal(patch.Changes["homeWorktrees"], &s.home)
		s.revision++
		_ = json.NewEncoder(w).Encode(contract.SettingsSnapshot{Revision: s.revision})
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	return Env{Connect: func() (*localclient.Client, error) { return localclient.New(localclient.UnixDialer{Path: socket}), nil }}
}

func TestPinReplaysAfterAConflict(t *testing.T) {
	s := &settingsServer{revision: 3, home: map[string][]string{"other": {"o1"}}, conflicts: 2}
	e := s.serve(t)
	client, _ := e.Connect()
	defer client.Close()
	// The caller's snapshot is stale: the server no longer lists "gone".
	repo := homeRepo()
	repo.Worktrees = append(repo.Worktrees, contract.Worktree{ID: "gone"})
	s.mu.Lock()
	s.home["r1"] = []string{"main", "gone"}
	s.mu.Unlock()
	if err := pinWorktrees(context.Background(), client, repo, []string{"a"}, true); err != nil {
		t.Fatal(err)
	}
	if s.patches != 3 || !slices.Equal(s.home["r1"], []string{"main", "a"}) || !slices.Equal(s.home["other"], []string{"o1", "x", "x"}) {
		t.Fatalf("after replay: %d patches, %v", s.patches, s.home)
	}
	// Pins of worktrees a healthy repository no longer lists are dropped.
	s.home["r1"] = []string{"main", "a", "stale"}
	if err := pinWorktrees(context.Background(), client, homeRepo(), []string{"a"}, false); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.home["r1"], []string{"main"}) {
		t.Fatalf("after unpin: %v", s.home["r1"])
	}
	// Three conflicts in a row give up.
	s.conflicts = 3
	if err := pinWorktrees(context.Background(), client, homeRepo(), []string{"b"}, true); err == nil {
		t.Fatal("kept conflicting but succeeded")
	}
}

// switchServer is a switch-preview and switch fake for repository r1,
// whose worktree a is on work at commit c0.
type switchServer struct {
	mu       sync.Mutex
	sessions []contract.SessionInfo
	blockers []contract.Issue
	targets  []contract.SwitchTarget
	switches []contract.SwitchRequest
}

func (s *switchServer) serve(t *testing.T) (Env, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "scw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s.sock")
	l, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	tree := contract.Worktree{ID: "a", RepositoryID: "r1", Path: "/w/a", Kind: "linked", Branch: new("work"), Head: new("c0"), Available: true}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/repositories", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]contract.Repository{{ID: "r1", Name: "app", Availability: "available", Worktrees: []contract.Worktree{tree}}})
	})
	mux.HandleFunc("POST /api/repositories/r1/worktrees/a/switch-preview", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var req contract.SwitchPreviewRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.targets = append(s.targets, req.Target)
		branch := req.Target.Branch + req.Target.Name
		preview := contract.SwitchPreview{Worktree: tree, Target: contract.SwitchTargetInfo{Kind: req.Target.Kind, Branch: &branch, Commit: "c1", Subject: "next"},
			Sessions: s.sessions, Blockers: s.blockers, Warnings: []contract.Issue{}}
		if len(s.blockers) == 0 {
			preview.ConfirmationID = "confirm"
		}
		_ = json.NewEncoder(w).Encode(preview)
	})
	mux.HandleFunc("POST /api/repositories/r1/worktrees/a/switch", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var req contract.SwitchRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.switches = append(s.switches, req)
		switched := tree
		switched.Branch, switched.Head = new(s.targets[len(s.targets)-1].Branch+s.targets[len(s.targets)-1].Name), new("c1")
		_ = json.NewEncoder(w).Encode(switched)
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	var stdout, stderr bytes.Buffer
	return Env{Connect: func() (*localclient.Client, error) { return localclient.New(localclient.UnixDialer{Path: socket}), nil },
		Stdout: &stdout, Stderr: &stderr}, &stdout, &stderr
}

func TestWorktreeSwitch(t *testing.T) {
	run := func(s *switchServer, args ...string) (int, string, string) {
		t.Helper()
		e, stdout, stderr := s.serve(t)
		code := e.Run(context.Background(), worktreeSwitchCommand, args)
		return code, stdout.String(), stderr.String()
	}

	clean := &switchServer{}
	if code, out, errOut := run(clean, "r1", "a", "feat/a"); code != contract.ExitOK || out != "feat/a\n" || !strings.Contains(errOut, "switching /w/a (work) to feat/a at c1 next") {
		t.Fatalf("clean switch: %d %q %q", code, out, errOut)
	}
	if len(clean.switches) != 1 || clean.switches[0] != (contract.SwitchRequest{ConfirmationID: "confirm"}) || clean.targets[0] != (contract.SwitchTarget{Kind: "local", Branch: "feat/a"}) {
		t.Fatalf("clean requests: %+v %+v", clean.targets, clean.switches)
	}

	created := &switchServer{}
	code, out, _ := run(created, "r1", "a", "--create", "exp/b", "--json")
	var tree contract.Worktree
	if code != contract.ExitOK || json.Unmarshal([]byte(out), &tree) != nil || tree.Branch == nil || *tree.Branch != "exp/b" {
		t.Fatalf("--json: %d %q", code, out)
	}
	if created.targets[0] != (contract.SwitchTarget{Kind: "new", Name: "exp/b", StartPoint: "c0"}) {
		t.Fatalf("--create starts at the worktree's commit: %+v", created.targets[0])
	}

	running := &switchServer{sessions: []contract.SessionInfo{{ID: "s1", Status: "running", Title: "claude"}}}
	if code, _, errOut := run(running, "r1", "a", "--track", "origin/fix/b"); code != contract.ExitUsage || !strings.Contains(errOut, "session s1 (running) claude") || !strings.Contains(errOut, "--yes") {
		t.Fatalf("sessions without --yes: %d %q", code, errOut)
	}
	if len(running.switches) != 0 {
		t.Fatal("switched without confirmation")
	}
	if code, _, _ := run(running, "r1", "a", "--track", "origin/fix/b", "--yes"); code != contract.ExitOK || len(running.switches) != 1 || !running.switches[0].AcknowledgeSessions {
		t.Fatalf("sessions with --yes: %d %+v", code, running.switches)
	}

	dirty := &switchServer{blockers: []contract.Issue{{Code: "changes", Message: "有未提交的修改，请先提交或贮藏"}}}
	if code, out, errOut := run(dirty, "r1", "a", "--detach", "v1"); code != contract.ExitError || out != "" || !strings.Contains(errOut, "blocked: 有未提交的修改") {
		t.Fatalf("blocked: %d %q %q", code, out, errOut)
	}
	if len(dirty.switches) != 0 {
		t.Fatal("switched despite blockers")
	}

	for _, args := range [][]string{{"r1", "a"}, {"r1", "a", "feat/a", "--detach", "v1"}, {"r1", "a", "--track", "origin/x", "--from", "main"}} {
		if code, _, _ := run(&switchServer{}, args...); code != contract.ExitUsage {
			t.Errorf("%v: %d, want usage error", args, code)
		}
	}
}
