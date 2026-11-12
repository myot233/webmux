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

// maintenanceServer fakes the repository list, fetch and batch removal
// endpoints. Fetch answers per repository ID from fetches; a missing entry
// answers 409 like a repository without remotes.
type maintenanceServer struct {
	mu       sync.Mutex
	repos    []contract.Repository
	fetches  map[string][]contract.FetchResult
	previews [][]string
	removes  []contract.BatchRemoveWorktreesRequest
	// results maps worktree IDs to their removal status.
	results map[string]string
}

func (s *maintenanceServer) env(t *testing.T, stdin string, interactive bool) (Env, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "scm")
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
	mux.HandleFunc("GET /api/repositories", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(s.repos)
	})
	mux.HandleFunc("POST /api/repositories/{id}/fetch", func(w http.ResponseWriter, r *http.Request) {
		results, ok := s.fetches[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(contract.ErrorResponse{Error: "仓库没有配置远程，无法获取"})
			return
		}
		_ = json.NewEncoder(w).Encode(contract.FetchRemotesResponse{Results: results, Repository: s.repos[0]})
	})
	mux.HandleFunc("POST /api/repositories/{id}/worktrees/batch-removal-preview", func(w http.ResponseWriter, r *http.Request) {
		var req contract.BatchRemovalPreviewRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.mu.Lock()
		s.previews = append(s.previews, req.WorktreeIDs)
		s.mu.Unlock()
		preview := contract.BatchRemovalPreview{ConfirmationID: "confirm1"}
		for _, id := range req.WorktreeIDs {
			for _, tree := range s.repos[0].Worktrees {
				if tree.ID == id {
					preview.Items = append(preview.Items, contract.RemovalPreviewItem{Worktree: tree, Blockers: []string{}})
				}
			}
		}
		_ = json.NewEncoder(w).Encode(preview)
	})
	mux.HandleFunc("POST /api/repositories/{id}/worktrees/batch-remove", func(w http.ResponseWriter, r *http.Request) {
		var req contract.BatchRemoveWorktreesRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.mu.Lock()
		s.removes = append(s.removes, req)
		ids := s.previews[len(s.previews)-1]
		s.mu.Unlock()
		result := contract.BatchRemoveResult{Results: []contract.BatchRemoveItemResult{}}
		for _, id := range ids {
			item := contract.BatchRemoveItemResult{WorktreeID: id, Path: "/w/" + id, Status: s.results[id]}
			if item.Status == "failed" {
				item.Error = "Git refused"
			}
			result.Results = append(result.Results, item)
		}
		_ = json.NewEncoder(w).Encode(result)
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
	}, &stdout, &stderr
}

func TestRepoFetchExitCodes(t *testing.T) {
	s := &maintenanceServer{
		repos: []contract.Repository{{ID: "good", Name: "good"}, {ID: "partial", Name: "partial"}, {ID: "bare", Name: "bare"}},
		fetches: map[string][]contract.FetchResult{
			"good":    {{Remote: "origin", Status: "ok"}, {Remote: "upstream", Status: "ok"}},
			"partial": {{Remote: "fork", Status: "auth", Message: "认证失败，请在终端完成认证后重试"}, {Remote: "origin", Status: "ok"}},
		},
	}
	e, stdout, stderr := s.env(t, "", false)
	if code := e.Run(context.Background(), repoFetchCommand, []string{"good"}); code != contract.ExitOK || stdout.String() != "origin\tok\t\nupstream\tok\t\n" {
		t.Fatalf("all ok: %d %q %q", code, stdout, stderr)
	}
	stdout.Reset()
	if code := e.Run(context.Background(), repoFetchCommand, []string{"partial"}); code != contract.ExitError ||
		stdout.String() != "fork\tauth\t认证失败，请在终端完成认证后重试\norigin\tok\t\n" {
		t.Fatalf("partial failure: %d %q", code, stdout)
	}
	stdout.Reset()
	var fetched contract.FetchRemotesResponse
	if code := e.Run(context.Background(), repoFetchCommand, []string{"partial", "--json"}); code != contract.ExitError ||
		json.Unmarshal(stdout.Bytes(), &fetched) != nil || len(fetched.Results) != 2 || fetched.Repository.ID != "good" {
		t.Fatalf("partial failure --json: %d %q", code, stdout)
	}
	stdout.Reset()
	if code := e.Run(context.Background(), repoFetchCommand, []string{"bare"}); code != contract.ExitError || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "没有配置远程") {
		t.Fatalf("no remotes: %d %q %q", code, stdout, stderr)
	}
}

func pruneRepo() contract.Repository {
	branch := func(name string) *string { return &name }
	return contract.Repository{ID: "r1", Name: "app", Availability: "available", Worktrees: []contract.Worktree{
		{ID: "main", Kind: "main", Path: "/w/main", Prunable: true},
		{ID: "gone1", Kind: "linked", Path: "/w/gone1", Branch: branch("fix/a"), Prunable: true},
		{ID: "gone2", Kind: "linked", Path: "/w/gone2", Branch: branch("fix/b"), Prunable: true},
		{ID: "locked", Kind: "linked", Path: "/w/locked", Prunable: true, Locked: true},
		{ID: "live", Kind: "linked", Path: "/w/live", Available: true},
	}}
}

func TestWorktreePruneSelectsStaleRegistrations(t *testing.T) {
	t.Run("nothing to prune", func(t *testing.T) {
		repo := pruneRepo()
		repo.Worktrees = slices.DeleteFunc(repo.Worktrees, stale)
		s := &maintenanceServer{repos: []contract.Repository{repo}}
		e, stdout, stderr := s.env(t, "", false)
		if code := e.Run(context.Background(), worktreePruneCommand, []string{"r1"}); code != contract.ExitOK || stdout.Len() != 0 ||
			!strings.Contains(stderr.String(), "no stale worktree registrations") || len(s.previews) != 0 {
			t.Fatalf("nothing to prune: %d %q %q", code, stdout, stderr)
		}
		if code := e.Run(context.Background(), worktreePruneCommand, []string{"r1", "--json"}); code != contract.ExitOK ||
			strings.TrimSpace(stdout.String()) != "{\n  \"results\": []\n}" {
			t.Fatalf("nothing to prune --json: %d %q", code, stdout)
		}
	})
	t.Run("needs --yes without a terminal", func(t *testing.T) {
		s := &maintenanceServer{repos: []contract.Repository{pruneRepo()}}
		e, _, stderr := s.env(t, "", false)
		code := e.Run(context.Background(), worktreePruneCommand, []string{"r1"})
		if code != contract.ExitUsage || len(s.removes) != 0 || !strings.Contains(stderr.String(), "--yes") ||
			!strings.Contains(stderr.String(), "/w/gone1 (fix/a)") || !strings.Contains(stderr.String(), "/w/gone2 (fix/b)") ||
			strings.Contains(stderr.String(), "/w/locked") {
			t.Fatalf("non-interactive: %d %q", code, stderr)
		}
		if len(s.previews) != 1 || !slices.Equal(s.previews[0], []string{"gone1", "gone2"}) {
			t.Fatalf("previewed %v", s.previews)
		}
	})
	t.Run("refused on a terminal", func(t *testing.T) {
		s := &maintenanceServer{repos: []contract.Repository{pruneRepo()}}
		e, _, _ := s.env(t, "n\n", true)
		if code := e.Run(context.Background(), worktreePruneCommand, []string{"r1"}); code != contract.ExitError || len(s.removes) != 0 {
			t.Fatalf("refused: %d %v", code, s.removes)
		}
	})
	t.Run("removes with --yes", func(t *testing.T) {
		s := &maintenanceServer{repos: []contract.Repository{pruneRepo()}, results: map[string]string{"gone1": "removed", "gone2": "removed"}}
		e, stdout, stderr := s.env(t, "", false)
		if code := e.Run(context.Background(), worktreePruneCommand, []string{"r1", "--yes"}); code != contract.ExitOK ||
			stdout.String() != "removed\t/w/gone1\t\t\nremoved\t/w/gone2\t\t\n" {
			t.Fatalf("--yes: %d %q %q", code, stdout, stderr)
		}
		if len(s.removes) != 1 || s.removes[0] != (contract.BatchRemoveWorktreesRequest{ConfirmationID: "confirm1"}) {
			t.Fatalf("batch remove requests: %+v", s.removes)
		}
	})
	t.Run("confirmed on a terminal", func(t *testing.T) {
		s := &maintenanceServer{repos: []contract.Repository{pruneRepo()}, results: map[string]string{"gone1": "removed", "gone2": "removed"}}
		e, _, stderr := s.env(t, "y\n", true)
		if code := e.Run(context.Background(), worktreePruneCommand, []string{"r1"}); code != contract.ExitOK || len(s.removes) != 1 ||
			!strings.Contains(stderr.String(), "Remove 2 stale worktree registrations of app?") {
			t.Fatalf("confirmed: %d %q", code, stderr)
		}
	})
	t.Run("partial failure", func(t *testing.T) {
		s := &maintenanceServer{repos: []contract.Repository{pruneRepo()}, results: map[string]string{"gone1": "failed", "gone2": "removed"}}
		e, stdout, _ := s.env(t, "", false)
		if code := e.Run(context.Background(), worktreePruneCommand, []string{"r1", "--yes"}); code != contract.ExitError ||
			stdout.String() != "failed\t/w/gone1\t\tGit refused\nremoved\t/w/gone2\t\t\n" {
			t.Fatalf("partial failure: %d %q", code, stdout)
		}
		stdout.Reset()
		var result contract.BatchRemoveResult
		if code := e.Run(context.Background(), worktreePruneCommand, []string{"r1", "--yes", "--json"}); code != contract.ExitError ||
			json.Unmarshal(stdout.Bytes(), &result) != nil || len(result.Results) != 2 || result.Results[0].Status != "failed" {
			t.Fatalf("partial failure --json: %d %q", code, stdout)
		}
	})
}
