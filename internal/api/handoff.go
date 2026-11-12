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
	s.handoffDraftRoutes(mux)
	mux.HandleFunc("GET /api/handoffs/{id}/selection", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if s.Conversations == nil || !s.Conversations.HistoryEnabled() {
			conversationFailure(w, conversation.ErrUnavailable)
			return
		}
		if !conversationID.MatchString(r.PathValue("id")) {
			conversationFailure(w, conversation.ErrInvalid)
			return
		}
		q, err := readConversationQuery(r, "afterPosition", "limit", "itemId")
		if err != nil {
			conversationFailure(w, err)
			return
		}
		after, err := conversationNumber(q, "afterPosition", 0)
		if err != nil {
			conversationFailure(w, err)
			return
		}
		limit, err := conversationNumber(q, "limit", 20)
		if err != nil || limit < 1 || limit > 100 || (q.Has("itemId") && !conversationID.MatchString(q.Get("itemId"))) {
			conversationFailure(w, conversation.ErrInvalid)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		page, err := s.Conversations.Selection(ctx, r.PathValue("id"), after, int(limit), q.Get("itemId"))
		if err != nil {
			conversationFailure(w, err)
			return
		}
		reply(w, http.StatusOK, wire.HandoffSelection(page))
	})
	mux.HandleFunc("POST /api/conversations/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if s.Handoffs == nil {
			conversationFailure(w, conversation.ErrUnavailable)
			return
		}
		if !conversationID.MatchString(r.PathValue("id")) {
			conversationFailure(w, conversation.ErrInvalid)
			return
		}
		var request contract.ConversationMessageRequest
		if !decodeStrict(w, r, &request, 8<<20, conversation.ErrInvalid) {
			return
		}
		if !conversationID.MatchString(request.HandoffID) || request.Revision < 1 {
			conversationFailure(w, conversation.ErrInvalid)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		delivery, err := s.Handoffs.Deliver(ctx, r.PathValue("id"), wire.HandoffMessage(request))
		if err != nil {
			if hybridFailure(w, err) {
				return
			}
			conversationFailure(w, err)
			return
		}
		reply(w, http.StatusAccepted, s.handoffDelivery(delivery))
	})
	mux.HandleFunc("GET /api/handoffs/{id}/delivery", func(w http.ResponseWriter, r *http.Request) {
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
		delivery, err := s.Handoffs.Delivery(ctx, r.PathValue("id"))
		if err != nil {
			conversationFailure(w, err)
			return
		}
		reply(w, http.StatusOK, s.handoffDelivery(delivery))
	})
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
		preview, err := s.Handoffs.EditDraft(ctx, r.PathValue("id"), request.Revision, request.Text, request.UserText)
		if err != nil {
			conversationFailure(w, err)
			return
		}
		reply(w, http.StatusOK, wire.HandoffPreview(preview))
	})
}

func (s *Server) handoffDelivery(e conversation.Execution) contract.HandoffDelivery {
	out := wire.HandoffDelivery(e)
	if session, err := s.sessionInfo(e.RuntimeID); err == nil {
		out.Session = &session
	}
	return out
}
