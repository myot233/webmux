package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/wire"
	"github.com/rainchan/webmux/internal/workspace"
)

// requestID is a client request ID as session commands take it.
var requestID = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// forgetTimeout bounds the close a forget waits for.
const forgetTimeout = 30 * time.Second

// sessionRoutes serves the session verbs that act on any session and
// dispatch by its view: listing by state, creating, input, interrupt, view
// changes, resume, and stop, remove and forget.
func (s *Server) sessionRoutes(mux *http.ServeMux) {
	serve(mux, "GET /api/sessions", http.StatusOK, sessionCommandFailure, s.list)
	mux.HandleFunc("POST /api/sessions", s.create)
	serve(mux, "GET /api/sessions/{id}", http.StatusOK, sessionCommandFailure, func(r *http.Request) (contract.SessionInfo, error) {
		return s.sessionInfo(r.PathValue("id"))
	})
	mux.HandleFunc("DELETE /api/sessions/{id}", s.delete)
	serveIn(mux, "POST /api/sessions/{id}/input", http.StatusOK, sessionCommandFailure, s.input)
	serveInDone(mux, "POST /api/sessions/{id}/interrupt", sessionCommandFailure, s.interrupt)
	serveIn(mux, "POST /api/sessions/{id}/resize", http.StatusOK, sessionCommandFailure, s.resize)
	// view moves a session with a chat view to its other view, or answers
	// a change waiting for the user's choice. It answers once the change is
	// over; a draft returned to the terminal goes to this requester only.
	serveIn(mux, "POST /api/sessions/{id}/view", http.StatusOK, sessionCommandFailure, func(r *http.Request, req contract.ViewChangeRequest) (contract.ViewChangeResult, error) {
		id := r.PathValue("id")
		if err := s.chatTarget(id); err != nil {
			return contract.ViewChangeResult{Dropped: []string{}, Unsupported: []string{}}, err
		}
		result, err := s.Hybrid.Switch(r.Context(), id, wire.ViewChangeInput(req))
		return wire.ViewChangeResult(result), err
	})
	// resume resumes a closed or interrupted session under its ID and
	// answers with the running session. Choices the user must make first
	// (another agent session, resuming without the deleted config's
	// environment) are 409s.
	serveIn(mux, "POST /api/sessions/{id}/resume", http.StatusOK, sessionCommandFailure, func(r *http.Request, req contract.ResumeRequest) (contract.SessionInfo, error) {
		id := r.PathValue("id")
		chat, err := s.sessionKind(id)
		switch {
		case err == nil:
			return contract.SessionInfo{}, hybrid.ErrNotArchived
		case !chat || !errors.Is(err, hybrid.ErrNotActive):
			return contract.SessionInfo{}, err
		}
		item, err := s.Workspace.ResumeHybrid(r.Context(), id, wire.ResumeInput(req))
		if err != nil {
			return contract.SessionInfo{}, err
		}
		return wire.Session(item.Info()), nil
	})
	mux.HandleFunc("POST /api/uploads", s.stagedUpload)
}

// sessionStates are the values of GET /api/sessions?state=.
var sessionStates = map[string][]contract.SessionState{
	"active":      {contract.SessionStateActive},
	"closed":      {contract.SessionStateClosed},
	"interrupted": {contract.SessionStateInterrupted},
	"archived":    {contract.SessionStateClosed, contract.SessionStateInterrupted},
	"all":         {contract.SessionStateActive, contract.SessionStateClosed, contract.SessionStateInterrupted},
}

// list answers GET /api/sessions: the active sessions in list order
// (the default), then the archived ones the state filter selects, most
// recently updated first.
func (s *Server) list(r *http.Request) ([]contract.SessionInfo, error) {
	want := map[contract.SessionState]bool{}
	values := r.URL.Query()["state"]
	if len(values) == 0 {
		values = []string{"active"}
	}
	for _, v := range values {
		states, ok := sessionStates[v]
		if !ok {
			return []contract.SessionInfo{}, &badRequest{errors.New("state 只能是 active、closed、interrupted、archived 或 all")}
		}
		for _, state := range states {
			want[state] = true
		}
	}
	out := []contract.SessionInfo{}
	if want[contract.SessionStateActive] {
		for _, info := range s.Manager.List() {
			if wire.Active(info) {
				out = append(out, wire.Session(info))
			}
		}
	}
	if !want[contract.SessionStateClosed] && !want[contract.SessionStateInterrupted] || s.Hybrid == nil {
		return out, nil
	}
	records, err := s.Hybrid.Records(true)
	if err != nil {
		return []contract.SessionInfo{}, err
	}
	for _, rec := range records {
		if want[contract.SessionState(rec.State)] {
			out = append(out, wire.ArchivedSession(rec, s.archived(rec)))
		}
	}
	return out, nil
}

// archived is what converting an archive record needs besides the record.
func (s *Server) archived(rec hybrid.Record) wire.Archived {
	out := wire.Archived{ConfigMissing: s.configMissing(rec.ConfigID)}
	if s.Conversations != nil {
		if binding, err := s.Conversations.Binding(context.Background(), rec.ID); err == nil {
			out.ConversationID, out.ProviderThreadID = binding.ConversationID, binding.ProviderThreadID
		}
	}
	if s.Hybrid != nil {
		out.AgentTitle, out.AgentIcon = s.Hybrid.AgentLabel(rec.Agent)
	}
	return out
}

// sessionInfo is session id in any state: a closed or interrupted session
// is described by its archive record.
func (s *Server) sessionInfo(id string) (contract.SessionInfo, error) {
	item, err := s.Manager.Get(id)
	if err == nil && wire.Active(item.Info()) {
		return wire.Session(item.Info()), nil
	}
	if s.Hybrid != nil {
		rec, recErr := s.Hybrid.Record(id)
		switch {
		case recErr == nil && rec.State != hybrid.StateActive:
			return wire.ArchivedSession(rec, s.archived(rec)), nil
		case err != nil && errors.Is(recErr, hybrid.ErrUnavailable):
			// The archive cannot say whether the session exists.
			return contract.SessionInfo{}, recErr
		}
	}
	if err == nil {
		return wire.Session(item.Info()), nil
	}
	return contract.SessionInfo{}, err
}

// sessionKind reports whether session id has a chat view. A missing session
// is session.ErrNotFound; a closed or interrupted one hybrid.ErrNotActive.
func (s *Server) sessionKind(id string) (chat bool, err error) {
	if item, err := s.Manager.Get(id); err == nil {
		info := item.Info()
		if info.Hybrid == nil {
			return false, nil
		}
		if !wire.Active(info) {
			return true, hybrid.ErrNotActive
		}
		return true, nil
	}
	if s.Hybrid != nil {
		switch _, err := s.Hybrid.Record(id); {
		case err == nil:
			return true, hybrid.ErrNotActive
		case errors.Is(err, hybrid.ErrUnavailable):
			return false, err
		}
	}
	return false, session.ErrNotFound
}

// chatTarget checks that a chat command can go to session id: an ordinary
// session has no chat view (409 no_chat_view), a missing one is a 404. A
// closed or interrupted session is left to the hybrid service, which keeps
// its chat channel for recovery.
func (s *Server) chatTarget(id string) error {
	chat, err := s.sessionKind(id)
	switch {
	case !chat && err == nil:
		return hybrid.ErrNoChatView
	case !chat:
		return err
	case s.Hybrid == nil:
		return hybrid.ErrNotFound
	}
	return nil
}

// inputLabel is the controller label of server-side terminal input: script,
// and the caller's session when a session's process sent it.
func (s *Server) inputLabel(r *http.Request) string {
	if caller := s.callerSession(r); caller != "" {
		return "script · " + caller
	}
	return "script"
}

func (s *Server) input(r *http.Request, req contract.SessionInputRequest) (contract.SessionInputResult, error) {
	if !requestID.MatchString(req.ClientRequestID) {
		return contract.SessionInputResult{}, &badRequest{errors.New("clientRequestId 必须是 8 到 64 个字母、数字、- 或 _")}
	}
	if _, ok := wire.InputKeys(req.Keys); !ok {
		return contract.SessionInputResult{}, &badRequest{errors.New("未知按键")}
	}
	if req.Text == "" && len(req.Keys) == 0 && len(req.Attachments) == 0 && !req.Submit {
		return contract.SessionInputResult{}, &badRequest{errors.New("没有要输入的内容")}
	}
	id := r.PathValue("id")
	chat, err := s.sessionKind(id)
	if err != nil {
		return contract.SessionInputResult{}, err
	}
	in := wire.SessionInput(req, s.inputLabel(r))
	var result hybrid.InputResult
	if chat && s.Hybrid != nil {
		result, err = s.Hybrid.Input(r.Context(), id, in)
	} else {
		item, getErr := s.Manager.Get(id)
		if getErr != nil {
			return contract.SessionInputResult{}, getErr
		}
		result, err = s.Hybrid.TerminalInput(r.Context(), item, in)
	}
	return wire.SessionInputResult(result), err
}

// resize answers POST /api/sessions/{id}/resize: a script sets the PTY size
// of a session's terminal run (see session.ResizeScript).
func (s *Server) resize(r *http.Request, req contract.SessionResizeRequest) (contract.SessionInfo, error) {
	item, err := s.Manager.Get(r.PathValue("id"))
	if err != nil {
		return contract.SessionInfo{}, err
	}
	if err := item.ResizeScript(wire.ScriptResize(req, s.inputLabel(r))); err != nil {
		return contract.SessionInfo{}, err
	}
	return wire.Session(item.Info()), nil
}

func (s *Server) interrupt(r *http.Request, req contract.SessionCommandRequest) error {
	if !requestID.MatchString(req.ClientRequestID) {
		return &badRequest{errors.New("clientRequestId 必须是 8 到 64 个字母、数字、- 或 _")}
	}
	id := r.PathValue("id")
	chat, err := s.sessionKind(id)
	if err != nil {
		return err
	}
	if chat && s.Hybrid != nil {
		return s.Hybrid.Interrupt(r.Context(), id, req.ClientRequestID, s.inputLabel(r))
	}
	item, err := s.Manager.Get(id)
	if err != nil {
		return err
	}
	return s.Hybrid.TerminalInterrupt(r.Context(), item, req.ClientRequestID, s.inputLabel(r))
}

// create answers POST /api/sessions. The create runs to its end (or
// workspace.CreateTimeout) even if the client goes away; a retry with the
// same clientRequestId gets its outcome. Every failure is a
// contract.CreateSessionFailure.
func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	var req contract.CreateSessionRequest
	if err := decodeCreate(w, r, &req); err != nil {
		reply(w, http.StatusBadRequest, contract.CreateSessionFailure{Error: err.Error(), Step: workspace.StepPrecheck})
		return
	}
	// Labels are checked first so an invalid request starts nothing.
	if err := s.Manager.ValidateLabels(req.Labels); err != nil {
		reply(w, http.StatusBadRequest, contract.CreateSessionFailure{Error: err.Error(), Step: workspace.StepPrecheck})
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), workspace.CreateTimeout)
	defer cancel()
	result, err := s.Workspace.Create(ctx, wire.CreateInput(req))
	if err != nil {
		createFailure(w, err)
		return
	}
	id := result.Session.Info().ID
	info, err := s.adopt(r, id, req.Labels)
	if err != nil {
		info = result.Session.Info()
	}
	reply(w, http.StatusCreated, wire.CreateResult(result, info))
}

// decodeCreate reads the body like decode, with room for a prompt of
// hybrid.MaxPromptBytes, but leaves the failure body to the caller: every
// create failure is a contract.CreateSessionFailure.
func decodeCreate(w http.ResponseWriter, r *http.Request, req *contract.CreateSessionRequest) error {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, chatBodyBytes))
	if err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(req); err != nil {
		return err
	}
	if len(bytes.TrimSpace(data[d.InputOffset():])) > 0 {
		return errors.New("expected one JSON object")
	}
	return nil
}

// delete stops a session (the default), removes it from the active list
// (remove=1) or forgets it (forget=1): it is removed, its archive record
// and artifacts folder deleted. Forgetting an active session with a chat
// view closes it first; the agent keeps its own session store.
func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	query := r.URL.Query()
	var err error
	switch {
	case query.Get("forget") == "1":
		err = s.forget(r.Context(), id)
	case query.Get("remove") == "1":
		err = s.Manager.Remove(id)
	default:
		err = s.Manager.Close(id)
	}
	if err != nil {
		sessionCommandFailure(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) forget(ctx context.Context, id string) error {
	chat, err := s.sessionKind(id)
	if err != nil && !errors.Is(err, hybrid.ErrNotActive) {
		return err
	}
	if chat && s.Hybrid != nil {
		ctx, cancel := context.WithTimeout(ctx, forgetTimeout)
		defer cancel()
		rec, recErr := s.Hybrid.Record(id)
		if err := s.Hybrid.Forget(ctx, id); err != nil {
			return err
		}
		// A released record was the last holder of its private ref unless
		// another record shares the release; a failure is left to the
		// workspace's sweep.
		if recErr == nil && rec.Released != nil {
			if err := s.Workspace.DropRelease(ctx, *rec.Released); err != nil {
				slog.Warn("released ref removal failed", "release", rec.Released.ID, "error", err)
			}
		}
	} else if err := s.Manager.Remove(id); err != nil {
		return err
	}
	// The folder outlived the shell for a possible resume; nothing can come
	// back for it now.
	s.Artifacts.Remove(id)
	return nil
}

// stagedUpload stores an attachment of a session that does not exist yet
// in the shared staging folder; the create moves it to its session.
func (s *Server) stagedUpload(w http.ResponseWriter, r *http.Request) {
	upload, received := s.storeUpload(w, r, s.Artifacts.EnsureStaging)
	if !received {
		if upload != nil {
			upload.Discard()
		}
		return
	}
	reply(w, http.StatusCreated, contract.UploadResult{Path: upload.Path})
}
