package api

import (
	"context"
	"net/http"
	"time"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/wire"
)

func (s *Server) handoffRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/conversations/{id}/handoff-previews", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if s.Handoffs == nil {
			conversationFailure(w, conversation.ErrUnavailable)
			return
		}
		if !conversationID.MatchString(r.PathValue("id")) {
			conversationFailure(w, conversation.ErrInvalid)
			return
		}
		var request contract.HandoffPreviewRequest
		if !decodeStrict(w, r, &request, 8<<20, conversation.ErrInvalid) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		preview, err := s.Handoffs.Preview(ctx, r.PathValue("id"), wire.HandoffRequest(request))
		if err != nil {
			conversationFailure(w, err)
			return
		}
		reply(w, http.StatusCreated, wire.HandoffPreview(preview))
	})
	mux.HandleFunc("GET /api/handoffs/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if s.Handoffs == nil {
			conversationFailure(w, conversation.ErrUnavailable)
			return
		}
		if !conversationID.MatchString(r.PathValue("id")) {
			conversationFailure(w, conversation.ErrInvalid)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		preview, err := s.Handoffs.Get(ctx, r.PathValue("id"))
		if err != nil {
			conversationFailure(w, err)
			return
		}
		reply(w, http.StatusOK, wire.HandoffPreview(preview))
	})
	mux.HandleFunc("PATCH /api/handoffs/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if s.Handoffs == nil {
			conversationFailure(w, conversation.ErrUnavailable)
			return
		}
		if !conversationID.MatchString(r.PathValue("id")) {
			conversationFailure(w, conversation.ErrInvalid)
			return
		}
		var request contract.HandoffEditRequest
		if !decodeStrict(w, r, &request, 8<<20, conversation.ErrInvalid) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		preview, err := s.Handoffs.Edit(ctx, r.PathValue("id"), request.Revision, request.Text)
		if err != nil {
			conversationFailure(w, err)
			return
		}
		reply(w, http.StatusOK, wire.HandoffPreview(preview))
	})
}
