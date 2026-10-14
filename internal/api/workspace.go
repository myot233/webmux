package api

import (
	"net/http"

	"github.com/rainchan/webmux/internal/workspace"
)

func workspaceFailure(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	failure(w, workspace.Status(err), err)
	return true
}
func (s *Server) workspaceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/repositories", func(w http.ResponseWriter, r *http.Request) {
		repositories, err := s.Workspace.List(r.Context())
		if !workspaceFailure(w, err) {
			reply(w, 200, repositories)
		}
	})
	mux.HandleFunc("POST /api/repositories", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path string `json:"path"`
		}
		if !decode(w, r, &req) {
			return
		}
		repository, created, err := s.Workspace.Register(r.Context(), req.Path)
		if workspaceFailure(w, err) {
			return
		}
		status := 200
		if created {
			status = 201
		}
		reply(w, status, repository)
	})
	mux.HandleFunc("DELETE /api/repositories/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !workspaceFailure(w, s.Workspace.Unregister(r.PathValue("id"))) {
			w.WriteHeader(204)
		}
	})
	mux.HandleFunc("GET /api/repositories/{id}/worktrees", func(w http.ResponseWriter, r *http.Request) {
		repository, err := s.Workspace.Refresh(r.Context(), r.PathValue("id"))
		if !workspaceFailure(w, err) {
			reply(w, 200, repository)
		}
	})
	mux.HandleFunc("GET /api/repositories/{id}/branches", func(w http.ResponseWriter, r *http.Request) {
		branches, err := s.Workspace.Branches(r.Context(), r.PathValue("id"))
		if !workspaceFailure(w, err) {
			reply(w, 200, branches)
		}
	})
	mux.HandleFunc("POST /api/repositories/{id}/worktrees", func(w http.ResponseWriter, r *http.Request) {
		var req workspace.CreateRequest
		if !decode(w, r, &req) {
			return
		}
		tree, err := s.Workspace.CreateWorktree(r.Context(), r.PathValue("id"), req)
		if !workspaceFailure(w, err) {
			reply(w, 201, tree)
		}
	})
	mux.HandleFunc("POST /api/repositories/{id}/worktrees/{worktreeId}/removal-preview", func(w http.ResponseWriter, r *http.Request) {
		var req struct{}
		if !decode(w, r, &req) {
			return
		}
		preview, err := s.Workspace.PreviewRemoval(r.Context(), r.PathValue("id"), r.PathValue("worktreeId"))
		if !workspaceFailure(w, err) {
			reply(w, 200, preview)
		}
	})
	mux.HandleFunc("POST /api/repositories/{id}/worktrees/{worktreeId}/remove", func(w http.ResponseWriter, r *http.Request) {
		var req workspace.RemoveRequest
		if !decode(w, r, &req) {
			return
		}
		err := s.Workspace.RemoveWorktree(r.Context(), r.PathValue("id"), r.PathValue("worktreeId"), req)
		if !workspaceFailure(w, err) {
			w.WriteHeader(204)
		}
	})
}
