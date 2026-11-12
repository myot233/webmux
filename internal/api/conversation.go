package api

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/wire"
)

var conversationID = regexp.MustCompile(`^[a-f0-9]{32}$`)
var conversationCursor = regexp.MustCompile(`^(0|[1-9][0-9]{0,18})$`)

func (s *Server) conversationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/conversations/{id}/activities", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		value, err := s.conversationActivities(r)
		if err != nil {
			conversationFailure(w, err)
			return
		}
		reply(w, http.StatusOK, value)
	})
	mux.HandleFunc("GET /api/conversations/{id}/activities/{activityId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		value, err := s.conversationActivity(r)
		if err != nil {
			conversationFailure(w, err)
			return
		}
		reply(w, http.StatusOK, value)
	})
}

func readConversationQuery(r *http.Request, allowed ...string) (url.Values, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, conversation.ErrInvalid
	}
	for key, v := range values {
		known := false
		for _, name := range allowed {
			if key == name {
				known = true
				break
			}
		}
		if !known || len(v) != 1 || v[0] == "" {
			return nil, conversation.ErrInvalid
		}
	}
	return values, nil
}

func conversationNumber(q url.Values, key string, fallback int64) (int64, error) {
	if !q.Has(key) {
		return fallback, nil
	}
	text := q.Get(key)
	if !conversationCursor.MatchString(text) {
		return 0, conversation.ErrInvalid
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, conversation.ErrInvalid
	}
	return n, nil
}

func (s *Server) conversationActivities(r *http.Request) (contract.ConversationActivities, error) {
	var out contract.ConversationActivities
	if s.Conversations == nil {
		return out, conversation.ErrUnavailable
	}
	id := r.PathValue("id")
	if !conversationID.MatchString(id) {
		return out, conversation.ErrInvalid
	}
	q, err := readConversationQuery(r, "afterPosition", "limit", "maxBytesPerItem")
	if err != nil {
		return out, err
	}
	after, err := conversationNumber(q, "afterPosition", 0)
	if err != nil {
		return out, err
	}
	limit, err := conversationNumber(q, "limit", 20)
	if err != nil || limit < 1 || limit > 100 {
		return out, conversation.ErrInvalid
	}
	bytes, err := conversationNumber(q, "maxBytesPerItem", 4000)
	if err != nil || bytes < 4 || bytes > conversation.MaxActivityBytes {
		return out, conversation.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	page, err := s.Conversations.Activities(ctx, id, after, int(limit), int(bytes))
	if err != nil {
		return out, err
	}
	return wire.ConversationActivities(page), nil
}

func (s *Server) conversationActivity(r *http.Request) (contract.ConversationActivity, error) {
	var out contract.ConversationActivity
	if s.Conversations == nil {
		return out, conversation.ErrUnavailable
	}
	id, item := r.PathValue("id"), r.PathValue("activityId")
	if !conversationID.MatchString(id) || !conversationID.MatchString(item) {
		return out, conversation.ErrInvalid
	}
	q, err := readConversationQuery(r, "textOffset", "maxBytes")
	if err != nil {
		return out, err
	}
	offset, err := conversationNumber(q, "textOffset", 0)
	if err != nil {
		return out, err
	}
	bytes, err := conversationNumber(q, "maxBytes", 4000)
	if err != nil || bytes < 4 || bytes > conversation.MaxActivityBytes {
		return out, conversation.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	part, err := s.Conversations.Activity(ctx, id, item, offset, int(bytes))
	if err != nil {
		return out, err
	}
	return wire.ConversationActivity(part), nil
}
