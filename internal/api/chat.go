package api

import (
	"errors"
	"net/http"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/hub"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/wire"
)

// chatRoutes registers the chat view's own channel: the event stream and
// the commands only the chat view has. Prompts and cancellation are the
// session's input and interrupt. Commands answer 204; a session without a
// chat view answers 409 no_chat_view.
func (s *Server) chatRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /ws/s/{id}/chat", s.chatWS)
	const base = "/api/sessions/{id}/chat/"
	serveInDone(mux, "PATCH "+base+"queue/{qid}", sessionCommandFailure, chatCommand(s, func(h *hybrid.Service, r *http.Request, req contract.ChatQueueRequest) error {
		return h.EditQueued(r.Context(), r.PathValue("id"), r.PathValue("qid"), wire.ChatQueueEdit(req))
	}))
	serveInDone(mux, "DELETE "+base+"queue/{qid}", sessionCommandFailure, chatCommand(s, func(h *hybrid.Service, r *http.Request, req contract.ChatQueueRequest) error {
		return h.RemoveQueued(r.Context(), r.PathValue("id"), req.ClientRequestID, r.PathValue("qid"))
	}))
	serveInDone(mux, "POST "+base+"queue/{qid}/send-now", sessionCommandFailure, chatCommand(s, func(h *hybrid.Service, r *http.Request, req contract.ChatQueueRequest) error {
		return h.SendQueuedNow(r.Context(), r.PathValue("id"), req.ClientRequestID, r.PathValue("qid"))
	}))
	serveInDone(mux, "POST "+base+"queue/resume", sessionCommandFailure, chatCommand(s, func(h *hybrid.Service, r *http.Request, req contract.ChatQueueRequest) error {
		return h.ResumeQueue(r.Context(), r.PathValue("id"), req.ClientRequestID)
	}))
	serveInDone(mux, "POST "+base+"queue/clear", sessionCommandFailure, chatCommand(s, func(h *hybrid.Service, r *http.Request, req contract.ChatQueueRequest) error {
		return h.ClearQueue(r.Context(), r.PathValue("id"), req.ClientRequestID)
	}))
	serveInDone(mux, "POST "+base+"requests/{requestId}", sessionCommandFailure, chatCommand(s, func(h *hybrid.Service, r *http.Request, req contract.ChatAnswerRequest) error {
		answer, err := wire.ChatAnswer(req)
		if err != nil {
			return &badRequest{err}
		}
		return h.Answer(r.Context(), r.PathValue("id"), req.ClientRequestID, r.PathValue("requestId"), answer)
	}))
	serveInDone(mux, "PUT "+base+"config/{configId}", sessionCommandFailure, chatCommand(s, func(h *hybrid.Service, r *http.Request, req contract.ChatConfigRequest) error {
		return h.SetConfig(r.Context(), r.PathValue("id"), wire.ChatConfigInput(r.PathValue("configId"), req))
	}))
	serveInDone(mux, "PUT "+base+"permission-mode", sessionCommandFailure, chatCommand(s, func(h *hybrid.Service, r *http.Request, req contract.ChatPermissionModeRequest) error {
		return h.SetPermissionMode(r.Context(), r.PathValue("id"), wire.ChatPermissionModeInput(req))
	}))
}

// chatCommand adapts a command on the hybrid service to a serveInDone
// handler; a session without a chat view is refused first.
func chatCommand[Req any](s *Server, run func(h *hybrid.Service, r *http.Request, req Req) error) func(*http.Request, Req) error {
	return func(r *http.Request, req Req) error {
		if err := s.chatTarget(r.PathValue("id")); err != nil {
			return err
		}
		return run(s.Hybrid, r, req)
	}
}

// chatWS streams a session's chat view. An ordinary session is refused
// with 409 no_chat_view before the upgrade; a missing one closes 4404.
func (s *Server) chatWS(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.chatTarget(id); errors.Is(err, hybrid.ErrNoChatView) {
		sessionCommandFailure(w, err)
		return
	}
	var sub *hybrid.Subscription
	err := hybrid.ErrNotFound
	if s.Hybrid != nil {
		sub, err = s.Hybrid.Subscribe(id, hub.ChatResume(r.URL.Query()))
	}
	c := s.accept(w, r)
	if c == nil {
		if sub != nil {
			sub.Close()
		}
		return
	}
	if err != nil {
		_ = c.Close(hub.StatusNotFound, "session not found")
		return
	}
	hub.Chat(r.Context(), sub, c)
}

// configMissing reports whether launch config id was deleted; a store that
// cannot be read cannot tell and says no.
func (s *Server) configMissing(id string) bool {
	_, err := s.Launch.Get(id)
	return errors.Is(err, launch.ErrNotFound)
}
