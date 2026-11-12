package api

import (
	"net/http"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/wire"
)

func (s *Server) workspaceRoutes(mux *http.ServeMux) {
	const repo = "/api/repositories/{id}"
	serve(mux, "GET /api/repositories", http.StatusOK, workspaceFailure, func(r *http.Request) ([]contract.Repository, error) {
		repositories, err := s.Workspace.List(r.Context())
		return wire.Repositories(repositories), err
	})
	mux.HandleFunc("POST /api/repositories", func(w http.ResponseWriter, r *http.Request) {
		var req contract.RegisterRepositoryRequest
		if !decode(w, r, &req) {
			return
		}
		repository, created, err := s.Workspace.Register(r.Context(), req.Path)
		if err != nil {
			workspaceFailure(w, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		reply(w, status, wire.Repository(repository))
	})
	serveDone(mux, "DELETE "+repo, workspaceFailure, func(r *http.Request) error {
		return s.Workspace.Unregister(r.PathValue("id"))
	})
	serveIn(mux, "PATCH "+repo, http.StatusOK, workspaceFailure, func(r *http.Request, req contract.UpdateRepositoryRequest) (contract.Repository, error) {
		repository, err := s.Workspace.SetInitInstruction(r.Context(), r.PathValue("id"), req.InitInstruction)
		return wire.Repository(repository), err
	})
	serve(mux, "GET "+repo+"/worktrees", http.StatusOK, workspaceFailure, func(r *http.Request) (contract.Repository, error) {
		repository, err := s.Workspace.Refresh(r.Context(), r.PathValue("id"))
		return wire.Repository(repository), err
	})
	serve(mux, "GET "+repo+"/branches", http.StatusOK, workspaceFailure, func(r *http.Request) (contract.Branches, error) {
		branches, err := s.Workspace.Branches(r.Context(), r.PathValue("id"))
		return wire.Branches(branches), err
	})
	// Remote URLs are redacted by the workspace service before they leave it.
	serve(mux, "GET "+repo+"/remotes", http.StatusOK, workspaceFailure, func(r *http.Request) (contract.Remotes, error) {
		remotes, err := s.Workspace.Remotes(r.Context(), r.PathValue("id"))
		return wire.Remotes(remotes), err
	})
	// Fetching holds no repository gate and outlives a disconnected client;
	// failures are per remote, with Git output redacted by the service.
	serveIn(mux, "POST "+repo+"/fetch", http.StatusOK, workspaceFailure, func(r *http.Request, _ contract.FetchRemotesRequest) (contract.FetchRemotesResponse, error) {
		result, err := s.Workspace.FetchRemotes(r.Context(), r.PathValue("id"))
		return wire.FetchRemotes(result), err
	})
	serve(mux, "GET "+repo+"/commits", http.StatusOK, workspaceFailure, func(r *http.Request) (contract.Commits, error) {
		commits, err := s.Workspace.RecentCommits(r.Context(), r.PathValue("id"), r.URL.Query()["from"])
		return wire.Commits(commits), err
	})
	serveIn(mux, "POST "+repo+"/branches/refresh", http.StatusOK, workspaceFailure, func(r *http.Request, req contract.RefreshBranchBaseRequest) (contract.RefreshBranchBaseResponse, error) {
		result, err := s.Workspace.RefreshBranchBase(r.Context(), r.PathValue("id"), req.Branch)
		return wire.RefreshBranchBase(result), err
	})
	// Branch operations preview first; each confirmation only executes its
	// own operation. A checked-out fast-forward and a push go on when the
	// client leaves, and a refused push answers 200 with its status.
	serveIn(mux, "POST "+repo+"/branches/fast-forward-preview", http.StatusOK, workspaceFailure, func(r *http.Request, req contract.FastForwardPreviewRequest) (contract.FastForwardPreview, error) {
		preview, err := s.Workspace.PreviewFastForward(r.Context(), r.PathValue("id"), req.Branch)
		return wire.FastForwardPreview(preview), err
	})
	serveIn(mux, "POST "+repo+"/branches/fast-forward", http.StatusOK, workspaceFailure, func(r *http.Request, req contract.FastForwardRequest) (contract.FastForwardResult, error) {
		result, err := s.Workspace.FastForward(r.Context(), r.PathValue("id"), wire.FastForward(req))
		return wire.FastForwardResult(result), err
	})
	serveIn(mux, "POST "+repo+"/branches/push-preview", http.StatusOK, workspaceFailure, func(r *http.Request, req contract.PushPreviewRequest) (contract.PushPreview, error) {
		preview, err := s.Workspace.PreviewPush(r.Context(), r.PathValue("id"), req.Branch, req.Remote, req.Target)
		return wire.PushPreview(preview), err
	})
	serveIn(mux, "POST "+repo+"/branches/push", http.StatusOK, workspaceFailure, func(r *http.Request, req contract.PushRequest) (contract.PushResult, error) {
		result, err := s.Workspace.Push(r.Context(), r.PathValue("id"), wire.Push(req))
		return wire.PushResult(result), err
	})
	serveIn(mux, "POST "+repo+"/branches/delete-preview", http.StatusOK, workspaceFailure, func(r *http.Request, req contract.BranchDeletePreviewRequest) (contract.BranchDeletePreview, error) {
		preview, err := s.Workspace.PreviewBranchDelete(r.Context(), r.PathValue("id"), req.Branches)
		return wire.BranchDeletePreview(preview), err
	})
	serveIn(mux, "POST "+repo+"/branches/delete", http.StatusOK, workspaceFailure, func(r *http.Request, req contract.BranchDeleteRequest) (contract.BranchDeleteResult, error) {
		result, err := s.Workspace.DeleteBranches(r.Context(), r.PathValue("id"), wire.DeleteBranches(req))
		return wire.BranchDeleteResult(result), err
	})
	serveIn(mux, "POST "+repo+"/worktrees", http.StatusCreated, workspaceFailure, func(r *http.Request, req contract.CreateWorktreeRequest) (contract.Worktree, error) {
		tree, err := s.Workspace.CreateWorktree(r.Context(), r.PathValue("id"), wire.CreateWorktree(req))
		return wire.Worktree(tree), err
	})
	serveIn(mux, "POST "+repo+"/worktrees/{worktreeId}/removal-preview", http.StatusOK, workspaceFailure, func(r *http.Request, _ contract.RemovalPreviewRequest) (contract.RemovalPreview, error) {
		preview, err := s.Workspace.PreviewRemoval(r.Context(), r.PathValue("id"), r.PathValue("worktreeId"))
		return wire.Preview(preview, s.archived), err
	})
	// The archived records of the removed directory stay, released, with
	// their artifact folders: they can be rebuilt and resumed.
	serveInDone(mux, "POST "+repo+"/worktrees/{worktreeId}/remove", workspaceFailure, func(r *http.Request, req contract.RemoveWorktreeRequest) error {
		return s.Workspace.RemoveWorktree(r.Context(), r.PathValue("id"), r.PathValue("worktreeId"), wire.RemoveWorktree(req))
	})
	serveIn(mux, "POST "+repo+"/worktrees/{worktreeId}/switch-preview", http.StatusOK, workspaceFailure, func(r *http.Request, req contract.SwitchPreviewRequest) (contract.SwitchPreview, error) {
		preview, err := s.Workspace.PreviewSwitch(r.Context(), r.PathValue("id"), r.PathValue("worktreeId"), wire.SwitchTarget(req.Target))
		return wire.SwitchPreview(preview), err
	})
	// Git keeps running when the client goes away; the answer is the
	// worktree as it is afterwards.
	serveIn(mux, "POST "+repo+"/worktrees/{worktreeId}/switch", http.StatusOK, workspaceFailure, func(r *http.Request, req contract.SwitchRequest) (contract.Worktree, error) {
		tree, err := s.Workspace.SwitchWorktree(r.Context(), r.PathValue("id"), r.PathValue("worktreeId"), wire.SwitchWorktree(req))
		return wire.Worktree(tree), err
	})
	serveIn(mux, "POST "+repo+"/worktrees/batch-removal-preview", http.StatusOK, workspaceFailure, func(r *http.Request, req contract.BatchRemovalPreviewRequest) (contract.BatchRemovalPreview, error) {
		preview, err := s.Workspace.PreviewBatchRemoval(r.Context(), r.PathValue("id"), req.WorktreeIDs)
		return wire.BatchPreview(preview, s.archived), err
	})
	serveIn(mux, "POST "+repo+"/worktrees/batch-remove", http.StatusOK, workspaceFailure, func(r *http.Request, req contract.BatchRemoveWorktreesRequest) (contract.BatchRemoveResult, error) {
		result, err := s.Workspace.BatchRemoveWorktrees(r.Context(), r.PathValue("id"), wire.BatchRemoveWorktrees(req))
		return wire.BatchRemoveResult(result), err
	})
	// Walks the worktree directory only when asked: the size is not part of
	// the repository snapshot.
	serveIn(mux, "POST "+repo+"/worktrees/{worktreeId}/usage", http.StatusOK, workspaceFailure, func(r *http.Request, _ contract.WorktreeUsageRequest) (contract.WorktreeUsage, error) {
		usage, err := s.Workspace.MeasureWorktree(r.Context(), r.PathValue("id"), r.PathValue("worktreeId"))
		return wire.WorktreeUsage(usage), err
	})
	serveIn(mux, "POST "+repo+"/worktrees/{worktreeId}/release-preview", http.StatusOK, sessionFailure, func(r *http.Request, _ contract.ReleasePreviewRequest) (contract.ReleasePreview, error) {
		preview, err := s.Workspace.PreviewRelease(r.Context(), r.PathValue("id"), r.PathValue("worktreeId"))
		return wire.ReleasePreview(preview, s.archived), err
	})
	serveIn(mux, "POST "+repo+"/worktrees/{worktreeId}/release", http.StatusOK, sessionFailure, func(r *http.Request, req contract.ReleaseWorktreeRequest) (contract.ReleaseResult, error) {
		result, err := s.Workspace.ReleaseWorktree(r.Context(), r.PathValue("id"), r.PathValue("worktreeId"), wire.ReleaseWorktree(req))
		return wire.ReleaseResult(result), err
	})
	// Rebuilds bring a released record's worktree back; hybrid record
	// errors keep their statuses.
	serveIn(mux, "POST /api/hybrid-sessions/{id}/rebuild-preview", http.StatusOK, sessionFailure, func(r *http.Request, _ contract.RebuildPreviewRequest) (contract.RebuildPreview, error) {
		preview, err := s.Workspace.PreviewRebuild(r.Context(), r.PathValue("id"))
		return wire.RebuildPreview(preview), err
	})
	serveIn(mux, "POST /api/hybrid-sessions/{id}/rebuild", http.StatusOK, sessionFailure, func(r *http.Request, req contract.RebuildRequest) (contract.Worktree, error) {
		tree, err := s.Workspace.RebuildWorktree(r.Context(), r.PathValue("id"), wire.RebuildWorktree(req))
		return wire.Worktree(tree), err
	})
}
