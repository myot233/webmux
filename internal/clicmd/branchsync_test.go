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

// branchServer fakes the repository list, the branches list and the three
// branch operations. Delete previews group the requested branches by
// groups (default contained) and block those in blocked; the delete then
// fails those in failed and skips unmerged ones unless acknowledged.
type branchServer struct {
	mu          sync.Mutex
	branches    contract.Branches
	ff          contract.FastForwardPreview
	ffRuns      []contract.FastForwardRequest
	push        contract.PushPreview
	pushAsked   []contract.PushPreviewRequest
	pushResult  contract.PushResult
	pushRuns    []contract.PushRequest
	groups      map[string]string
	blocked     map[string]bool
	failed      map[string]bool
	deleteAsked [][]string
	deleteRuns  []contract.BranchDeleteRequest
}

// branchRepo is the fake repository; "." resolves to its main worktree.
var branchRepo = contract.Repository{ID: "r1", Name: "app", Availability: "available", Worktrees: []contract.Worktree{{ID: "main", Kind: "main", Path: "/w/app"}}}

func (s *branchServer) env(t *testing.T, stdin string, interactive bool) (Env, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "scb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s.sock")
	l, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	reply := func(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }
	read := func(r *http.Request, v any) {
		_ = json.NewDecoder(r.Body).Decode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/repositories", func(w http.ResponseWriter, r *http.Request) {
		reply(w, []contract.Repository{branchRepo})
	})
	mux.HandleFunc("GET /api/repositories/r1/branches", func(w http.ResponseWriter, r *http.Request) { reply(w, s.branches) })
	mux.HandleFunc("POST /api/repositories/r1/branches/fast-forward-preview", func(w http.ResponseWriter, r *http.Request) { reply(w, s.ff) })
	mux.HandleFunc("POST /api/repositories/r1/branches/fast-forward", func(w http.ResponseWriter, r *http.Request) {
		var req contract.FastForwardRequest
		read(r, &req)
		s.mu.Lock()
		s.ffRuns = append(s.ffRuns, req)
		s.mu.Unlock()
		reply(w, contract.FastForwardResult{Repository: branchRepo})
	})
	mux.HandleFunc("POST /api/repositories/r1/branches/push-preview", func(w http.ResponseWriter, r *http.Request) {
		var req contract.PushPreviewRequest
		read(r, &req)
		s.mu.Lock()
		s.pushAsked = append(s.pushAsked, req)
		s.mu.Unlock()
		reply(w, s.push)
	})
	mux.HandleFunc("POST /api/repositories/r1/branches/push", func(w http.ResponseWriter, r *http.Request) {
		var req contract.PushRequest
		read(r, &req)
		s.mu.Lock()
		s.pushRuns = append(s.pushRuns, req)
		s.mu.Unlock()
		reply(w, s.pushResult)
	})
	mux.HandleFunc("POST /api/repositories/r1/branches/delete-preview", func(w http.ResponseWriter, r *http.Request) {
		var req contract.BranchDeletePreviewRequest
		read(r, &req)
		s.mu.Lock()
		s.deleteAsked = append(s.deleteAsked, req.Branches)
		s.mu.Unlock()
		preview := contract.BranchDeletePreview{Items: []contract.BranchDeleteItem{}}
		for _, name := range req.Branches {
			item := contract.BranchDeleteItem{Branch: name, Commit: strings.Repeat("a", 40), Group: "contained", Blockers: []contract.Issue{},
				Unmerged: contract.CommitList{Items: []contract.Commit{}}}
			if s.groups[name] == "unmerged" {
				item.Group = "unmerged"
				item.Unmerged = contract.CommitList{Total: 3, Items: []contract.Commit{{Hash: strings.Repeat("b", 40), Subject: "squashed work"}}}
			}
			if s.blocked[name] {
				item.Blockers = append(item.Blockers, contract.Issue{Code: "occupied", Message: "分支 " + name + " 已被工作树检出，不能删除", Path: "/w/" + name})
			} else {
				preview.ConfirmationID = "delete1"
			}
			preview.Items = append(preview.Items, item)
		}
		reply(w, preview)
	})
	mux.HandleFunc("POST /api/repositories/r1/branches/delete", func(w http.ResponseWriter, r *http.Request) {
		var req contract.BranchDeleteRequest
		read(r, &req)
		s.mu.Lock()
		s.deleteRuns = append(s.deleteRuns, req)
		names := s.deleteAsked[len(s.deleteAsked)-1]
		s.mu.Unlock()
		result := contract.BranchDeleteResult{Results: []contract.BranchDeleteItemResult{}, Repository: branchRepo}
		for _, name := range names {
			if s.blocked[name] {
				continue
			}
			item := contract.BranchDeleteItemResult{Branch: name, Commit: strings.Repeat("a", 40), Status: "deleted", Message: "已删除；可用 git branch " + name + " " + strings.Repeat("a", 40) + " 恢复"}
			switch {
			case s.groups[name] == "unmerged" && !req.AcknowledgeUnmerged:
				item.Status, item.Message = "skipped", "有独有提交，未确认删除"
			case s.failed[name]:
				item.Status, item.Message = "failed", "Git 未删除分支"
			}
			result.Results = append(result.Results, item)
		}
		reply(w, result)
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	var stdout, stderr bytes.Buffer
	return Env{
		Connect:     func() (*localclient.Client, error) { return localclient.New(localclient.UnixDialer{Path: socket}), nil },
		Stdin:       strings.NewReader(stdin),
		Stdout:      &stdout,
		Stderr:      &stderr,
		Interactive: func() bool { return interactive },
		ResolveCwd:  func(context.Context, string) (string, error) { return "/w/app", nil },
	}, &stdout, &stderr
}

func TestRepoFastForward(t *testing.T) {
	from, to := strings.Repeat("1", 40), strings.Repeat("2", 40)
	ready := contract.FastForwardPreview{ConfirmationID: "ff1", Branch: "feat/a", From: from, To: to, Upstream: "origin/feat/a",
		Commits: contract.CommitList{Total: 3, Items: []contract.Commit{{Hash: to, Subject: "upstream work"}}}, Blockers: []contract.Issue{}, Warnings: []contract.Issue{}}
	t.Run("no sessions", func(t *testing.T) {
		s := &branchServer{ff: ready}
		e, stdout, stderr := s.env(t, "", false)
		if code := e.Run(context.Background(), repoFastForwardCommand, []string{"r1", "feat/a"}); code != contract.ExitOK || stdout.String() != "feat/a\t"+to+"\n" ||
			!strings.Contains(stderr.String(), "3 commits") || !strings.Contains(stderr.String(), "upstream work") || !strings.Contains(stderr.String(), "and 2 more") {
			t.Fatalf("fast-forward: %d %q %q", code, stdout, stderr)
		}
		if len(s.ffRuns) != 1 || s.ffRuns[0] != (contract.FastForwardRequest{ConfirmationID: "ff1"}) {
			t.Fatalf("requests %+v", s.ffRuns)
		}
		stdout.Reset()
		var result contract.FastForwardResult
		if code := e.Run(context.Background(), repoFastForwardCommand, []string{"r1", "feat/a", "--json"}); code != contract.ExitOK ||
			json.Unmarshal(stdout.Bytes(), &result) != nil || result.Repository.ID != "r1" {
			t.Fatalf("--json: %d %q", code, stdout)
		}
	})
	t.Run("blocked", func(t *testing.T) {
		blocked := ready
		blocked.ConfirmationID, blocked.Blockers = "", []contract.Issue{{Code: "diverged", Message: "已分叉，需要在终端合并或变基"}}
		s := &branchServer{ff: blocked}
		e, _, stderr := s.env(t, "", false)
		if code := e.Run(context.Background(), repoFastForwardCommand, []string{"r1", "feat/a"}); code != contract.ExitError || len(s.ffRuns) != 0 ||
			!strings.Contains(stderr.String(), "blocked: 已分叉") {
			t.Fatalf("blocked: %d %q", code, stderr)
		}
	})
	withSessions := ready
	withSessions.Sessions = []contract.SessionInfo{{ID: "s1", Status: "running", Title: "agent"}}
	t.Run("sessions without a terminal", func(t *testing.T) {
		s := &branchServer{ff: withSessions}
		e, _, stderr := s.env(t, "", false)
		if code := e.Run(context.Background(), repoFastForwardCommand, []string{".", "feat/a"}); code != contract.ExitUsage || len(s.ffRuns) != 0 ||
			!strings.Contains(stderr.String(), "session s1") || !strings.Contains(stderr.String(), "--yes") {
			t.Fatalf("non-interactive: %d %q", code, stderr)
		}
	})
	t.Run("sessions with --yes", func(t *testing.T) {
		s := &branchServer{ff: withSessions}
		e, _, _ := s.env(t, "", false)
		if code := e.Run(context.Background(), repoFastForwardCommand, []string{"r1", "feat/a", "--yes"}); code != contract.ExitOK ||
			len(s.ffRuns) != 1 || !s.ffRuns[0].AcknowledgeSessions {
			t.Fatalf("--yes: %d %+v", code, s.ffRuns)
		}
	})
	t.Run("sessions confirmed on a terminal", func(t *testing.T) {
		s := &branchServer{ff: withSessions}
		e, _, _ := s.env(t, "yes\n", true)
		if code := e.Run(context.Background(), repoFastForwardCommand, []string{"r1", "feat/a"}); code != contract.ExitOK || len(s.ffRuns) != 1 {
			t.Fatalf("confirmed: %d %+v", code, s.ffRuns)
		}
	})
}

func TestRepoPush(t *testing.T) {
	commit := strings.Repeat("3", 40)
	publish := contract.PushPreview{ConfirmationID: "push1", Branch: "feat/c", Commit: commit, Remote: "origin", PushURL: "https://***@example.invalid/app.git",
		Target: "feat/c", Publish: true, Commits: contract.CommitList{Total: 1, Items: []contract.Commit{{Hash: commit, Subject: "agent work"}}},
		Blockers: []contract.Issue{}, Warnings: []contract.Issue{}}
	ok := contract.PushResult{Status: "ok", Repository: branchRepo}
	t.Run("publish with --yes", func(t *testing.T) {
		s := &branchServer{push: publish, pushResult: ok}
		e, stdout, stderr := s.env(t, "", false)
		if code := e.Run(context.Background(), repoPushCommand, []string{".", "feat/c", "--yes"}); code != contract.ExitOK || stdout.String() != "ok\torigin/feat/c\t\n" ||
			!strings.Contains(stderr.String(), "publishing feat/c") || !strings.Contains(stderr.String(), "https://***@example.invalid/app.git") {
			t.Fatalf("publish: %d %q %q", code, stdout, stderr)
		}
		if len(s.pushRuns) != 1 || s.pushRuns[0].ConfirmationID != "push1" || s.pushAsked[0] != (contract.PushPreviewRequest{Branch: "feat/c"}) {
			t.Fatalf("requests %+v %+v", s.pushAsked, s.pushRuns)
		}
		stdout.Reset()
		var result contract.PushResult
		if code := e.Run(context.Background(), repoPushCommand, []string{"r1", "feat/c", "--yes", "--json"}); code != contract.ExitOK ||
			json.Unmarshal(stdout.Bytes(), &result) != nil || result.Status != "ok" || result.Repository.ID != "r1" {
			t.Fatalf("--json: %d %q", code, stdout)
		}
	})
	t.Run("without a terminal", func(t *testing.T) {
		s := &branchServer{push: publish, pushResult: ok}
		e, _, stderr := s.env(t, "", false)
		if code := e.Run(context.Background(), repoPushCommand, []string{".", "feat/c"}); code != contract.ExitUsage || len(s.pushRuns) != 0 ||
			!strings.Contains(stderr.String(), "agent work") || !strings.Contains(stderr.String(), "--yes") {
			t.Fatalf("non-interactive: %d %q", code, stderr)
		}
	})
	t.Run("refused on a terminal", func(t *testing.T) {
		s := &branchServer{push: publish, pushResult: ok}
		e, _, _ := s.env(t, "no\n", true)
		if code := e.Run(context.Background(), repoPushCommand, []string{".", "feat/c"}); code != contract.ExitError || len(s.pushRuns) != 0 {
			t.Fatalf("refused: %d", code)
		}
	})
	t.Run("rejected", func(t *testing.T) {
		s := &branchServer{push: publish, pushResult: contract.PushResult{Status: "rejected", Message: "远端有新的提交，请先获取远程再快进", Repository: branchRepo}}
		e, stdout, _ := s.env(t, "", false)
		if code := e.Run(context.Background(), repoPushCommand, []string{".", "feat/c", "--yes"}); code != contract.ExitError ||
			stdout.String() != "rejected\torigin/feat/c\t远端有新的提交，请先获取远程再快进\n" {
			t.Fatalf("rejected: %d %q", code, stdout)
		}
	})
	t.Run("blocked", func(t *testing.T) {
		blocked := publish
		blocked.ConfirmationID, blocked.Publish = "", false
		blocked.Blockers = []contract.Issue{{Code: "behind", Message: "分支落后上游 2 个提交，请先快进"}}
		s := &branchServer{push: blocked}
		e, _, stderr := s.env(t, "", false)
		if code := e.Run(context.Background(), repoPushCommand, []string{".", "feat/c", "--yes"}); code != contract.ExitError || len(s.pushRuns) != 0 ||
			!strings.Contains(stderr.String(), "落后上游") {
			t.Fatalf("blocked: %d %q", code, stderr)
		}
	})
	t.Run("no remote decided", func(t *testing.T) {
		undecided := publish
		undecided.ConfirmationID, undecided.Remote = "", ""
		undecided.Blockers = []contract.Issue{{Code: "remote", Message: "无法确定发布到哪个远程，请指定其中之一: fork, upstream"}}
		s := &branchServer{push: undecided}
		e, _, stderr := s.env(t, "", false)
		if code := e.Run(context.Background(), repoPushCommand, []string{".", "feat/c", "--yes"}); code != contract.ExitUsage || len(s.pushRuns) != 0 ||
			!strings.Contains(stderr.String(), "fork, upstream") || !strings.Contains(stderr.String(), "--remote") {
			t.Fatalf("undecided: %d %q", code, stderr)
		}
	})
	t.Run("--remote for a branch with an upstream", func(t *testing.T) {
		upstream := "origin/feat/a"
		s := &branchServer{branches: contract.Branches{Branches: []contract.Branch{{Name: "feat/a", Upstream: &upstream}}}, push: publish}
		e, _, stderr := s.env(t, "", false)
		if code := e.Run(context.Background(), repoPushCommand, []string{".", "feat/a", "--remote", "fork", "--yes"}); code != contract.ExitUsage || len(s.pushAsked) != 0 ||
			!strings.Contains(stderr.String(), "tracks origin/feat/a") {
			t.Fatalf("--remote with upstream: %d %q", code, stderr)
		}
	})
	t.Run("--remote and --to publish", func(t *testing.T) {
		s := &branchServer{branches: contract.Branches{Branches: []contract.Branch{{Name: "feat/c"}}}, push: publish, pushResult: ok}
		e, _, _ := s.env(t, "", false)
		if code := e.Run(context.Background(), repoPushCommand, []string{".", "feat/c", "--remote", "fork", "--to", "review/c", "--yes"}); code != contract.ExitOK ||
			len(s.pushAsked) != 1 || s.pushAsked[0] != (contract.PushPreviewRequest{Branch: "feat/c", Remote: "fork", Target: "review/c"}) {
			t.Fatalf("publish options: %d %+v", code, s.pushAsked)
		}
	})
}

func TestRepoBranchRemove(t *testing.T) {
	cleanupBranches := contract.Branches{Branches: []contract.Branch{
		{Name: "main", WorktreePaths: []string{"/w/main"}, MergedIntoDefault: true},
		{Name: "feat/old", WorktreePaths: []string{}, MergedIntoDefault: true},
		{Name: "feat/sq", WorktreePaths: []string{}, UpstreamGone: true},
		{Name: "feat/live", WorktreePaths: []string{}},
	}}
	t.Run("cleanup skips unmerged branches", func(t *testing.T) {
		s := &branchServer{branches: cleanupBranches, groups: map[string]string{"feat/sq": "unmerged"}}
		e, stdout, stderr := s.env(t, "", false)
		code := e.Run(context.Background(), repoBranchRemoveCommand, []string{".", "--cleanup", "--yes"})
		lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
		if code != contract.ExitOK || len(lines) != 2 || !strings.HasPrefix(lines[0], "deleted\tfeat/old\t"+strings.Repeat("a", 40)+"\t") ||
			!strings.Contains(lines[0], "git branch feat/old") || !strings.HasPrefix(lines[1], "skipped\tfeat/sq\t") || !strings.Contains(lines[1], "--unmerged") {
			t.Fatalf("cleanup: %d %q %q", code, stdout, stderr)
		}
		if len(s.deleteAsked) != 1 || !slices.Equal(s.deleteAsked[0], []string{"feat/old", "feat/sq"}) || s.deleteRuns[0].AcknowledgeUnmerged {
			t.Fatalf("requests %v %+v", s.deleteAsked, s.deleteRuns)
		}
		if !strings.Contains(stderr.String(), "contained feat/old") || !strings.Contains(stderr.String(), "unmerged feat/sq") ||
			!strings.Contains(stderr.String(), "squashed work") {
			t.Fatalf("preview: %q", stderr)
		}
	})
	t.Run("unmerged with --unmerged", func(t *testing.T) {
		s := &branchServer{groups: map[string]string{"feat/sq": "unmerged"}}
		e, stdout, _ := s.env(t, "", false)
		if code := e.Run(context.Background(), repoBranchRemoveCommand, []string{".", "feat/sq", "--unmerged", "--yes"}); code != contract.ExitOK ||
			!strings.HasPrefix(stdout.String(), "deleted\tfeat/sq\t") || !s.deleteRuns[0].AcknowledgeUnmerged {
			t.Fatalf("--unmerged: %d %q", code, stdout)
		}
	})
	t.Run("without a terminal", func(t *testing.T) {
		s := &branchServer{}
		e, _, stderr := s.env(t, "", false)
		if code := e.Run(context.Background(), repoBranchRemoveCommand, []string{".", "feat/old"}); code != contract.ExitUsage || len(s.deleteRuns) != 0 ||
			!strings.Contains(stderr.String(), "--yes") {
			t.Fatalf("non-interactive: %d %q", code, stderr)
		}
	})
	t.Run("confirmed on a terminal", func(t *testing.T) {
		s := &branchServer{}
		e, _, stderr := s.env(t, "y\n", true)
		if code := e.Run(context.Background(), repoBranchRemoveCommand, []string{".", "feat/old", "feat/new"}); code != contract.ExitOK || len(s.deleteRuns) != 1 ||
			!strings.Contains(stderr.String(), "Delete 2 local branches of app?") {
			t.Fatalf("confirmed: %d %q", code, stderr)
		}
	})
	t.Run("blocked", func(t *testing.T) {
		s := &branchServer{blocked: map[string]bool{"feat/a": true}}
		e, _, stderr := s.env(t, "", false)
		if code := e.Run(context.Background(), repoBranchRemoveCommand, []string{".", "feat/a", "--yes"}); code != contract.ExitError || len(s.deleteRuns) != 0 ||
			!strings.Contains(stderr.String(), "blocked feat/a") || !strings.Contains(stderr.String(), "/w/feat/a") {
			t.Fatalf("blocked: %d %q", code, stderr)
		}
	})
	t.Run("failure", func(t *testing.T) {
		s := &branchServer{failed: map[string]bool{"feat/old": true}}
		e, stdout, _ := s.env(t, "", false)
		var result contract.BranchDeleteResult
		if code := e.Run(context.Background(), repoBranchRemoveCommand, []string{".", "feat/old", "feat/new", "--yes", "--json"}); code != contract.ExitError ||
			json.Unmarshal(stdout.Bytes(), &result) != nil || len(result.Results) != 2 || result.Results[0].Status != "failed" || result.Repository.ID != "r1" {
			t.Fatalf("failure --json: %d %q", code, stdout)
		}
	})
	t.Run("nothing to clean up", func(t *testing.T) {
		s := &branchServer{branches: contract.Branches{Branches: []contract.Branch{{Name: "main", WorktreePaths: []string{"/w/main"}, MergedIntoDefault: true}}}}
		e, stdout, stderr := s.env(t, "", false)
		if code := e.Run(context.Background(), repoBranchRemoveCommand, []string{".", "--cleanup", "--json"}); code != contract.ExitOK || len(s.deleteAsked) != 0 ||
			!strings.Contains(stdout.String(), `"results": []`) || !strings.Contains(stderr.String(), "no branches to clean up") {
			t.Fatalf("nothing: %d %q %q", code, stdout, stderr)
		}
	})
	for _, args := range [][]string{{"."}, {".", "feat/a", "--cleanup"}} {
		s := &branchServer{}
		e, _, _ := s.env(t, "", false)
		if code := e.Run(context.Background(), repoBranchRemoveCommand, args); code != contract.ExitUsage {
			t.Errorf("%v: %d, want usage", args, code)
		}
	}
}
