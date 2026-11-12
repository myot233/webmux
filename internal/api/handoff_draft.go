package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/wire"
)

func (s *Server) handoffDraftRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/conversations/{id}/handoff-previews", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		value, err := s.handoffDraft(r)
		if err != nil {
			conversationFailure(w, err)
			return
		}
		reply(w, http.StatusOK, value)
	})
}

func (s *Server) handoffDraft(r *http.Request) (contract.HandoffDraft, error) {
	var out contract.HandoffDraft
	if s.Handoffs == nil || !s.Conversations.HistoryEnabled() {
		return out, conversation.ErrUnavailable
	}
	id := r.PathValue("id")
	if !conversationID.MatchString(id) {
		return out, conversation.ErrInvalid
	}
	q, err := readConversationQuery(r, "fork")
	if err != nil || (q.Get("fork") != "0" && q.Get("fork") != "1") {
		return out, conversation.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	source, err := s.Conversations.Conversation(ctx, id)
	if err != nil {
		return out, err
	}
	out.Source = wire.ConversationContext(source)
	p, err := s.Conversations.LatestPreview(ctx, id, q.Get("fork") == "1")
	if errors.Is(err, conversation.ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	p, err = s.Handoffs.Get(ctx, p.ID)
	if err != nil {
		return out, err
	}
	preview := wire.HandoffPreview(p)
	out.Preview = &preview
	e, err := s.Handoffs.Delivery(ctx, p.ID)
	if errors.Is(err, conversation.ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	delivery := s.handoffDelivery(e)
	out.Delivery = &delivery
	return out, nil
}
