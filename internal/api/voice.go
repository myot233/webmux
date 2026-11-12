package api

import (
	"net/http"

	"github.com/rainchan/webmux/internal/hub"
	"github.com/rainchan/webmux/internal/voice"
)

func (s *Server) voiceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /ws/voice", s.voiceWS)
	s.voiceSettingsRoutes(mux)
}

func (s *Server) voiceWS(w http.ResponseWriter, r *http.Request) {
	if s.Voice == nil || !s.Voice.Enabled() {
		failure(w, http.StatusNotFound, voice.ErrDisabled)
		return
	}
	sess, err := s.Voice.Start(r.Context())
	if err != nil {
		failure(w, voiceStatus(err), err)
		return
	}
	c := s.accept(w, r)
	if c == nil {
		sess.Close()
		return
	}
	hub.Voice(r.Context(), sess, s.Voice.FinishTimeout(), c)
}
