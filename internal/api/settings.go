package api

import (
	"errors"
	"net/http"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/hub"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/settings"
	"github.com/rainchan/webmux/internal/wire"
)

func (s *Server) settingsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Settings.PruneSessionPins(s.keepSessionPin)
		if err != nil {
			failure(w, settingsStatus(err), err)
			return
		}
		reply(w, 200, wire.SettingsSnapshot(v))
	})
	mux.HandleFunc("PATCH /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var req contract.SettingsPatch
		if !decodeStrict(w, r, &req, 2<<20, settings.ErrInvalid) {
			return
		}
		changes, err := wire.SettingsChanges(req)
		if err != nil {
			failure(w, 400, err)
			return
		}
		v, err := s.Settings.PatchWithSessionPinFilter(*req.BaseRevision, changes, s.keepSessionPin)
		if err != nil {
			failure(w, settingsStatus(err), err)
			return
		}
		reply(w, 200, wire.SettingsSnapshot(v))
	})
	mux.HandleFunc("GET /ws/settings", func(w http.ResponseWriter, r *http.Request) {
		if _, err := s.Settings.PruneSessionPins(s.keepSessionPin); err != nil {
			failure(w, settingsStatus(err), err)
			return
		}
		sub, err := s.Settings.Subscribe()
		if err != nil {
			failure(w, settingsStatus(err), err)
			return
		}
		defer sub.Close()
		c := s.accept(w, r)
		if c == nil {
			return
		}
		hub.Settings(r.Context(), sub, c)
	})
}

// Retain interrupted hybrids so their original position returns on resume.
// A broken archive cannot tell whether a missing session is recoverable.
func (s *Server) keepSessionPin(id string) bool {
	if item, err := s.Manager.Get(id); err == nil {
		info := item.Info()
		if info.Hybrid == nil {
			return info.Status != session.StatusExited
		}
		if wire.Active(info) && info.Status != session.StatusExited {
			return true
		}
	}
	if s.Hybrid == nil {
		return false
	}
	record, err := s.Hybrid.Record(id)
	return errors.Is(err, hybrid.ErrUnavailable) ||
		(err == nil && record.State != hybrid.StateClosed)
}
