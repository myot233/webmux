package hybrid

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/session"
)

// restart stops the service and manager as a server shutdown does and opens
// new ones on the same state directory.
func (h *harness) restart() {
	h.t.Helper()
	h.svc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h.manager.Shutdown(ctx)
	h.open(0)
}

func (h *harness) resume(id string, in ResumeInput) (*session.Session, error) {
	h.t.Helper()
	if in.ClientRequestID == "" {
		in.ClientRequestID = rid()
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	return h.svc.Resume(ctx, id, in)
}

func (h *harness) mustResume(id string, in ResumeInput) *session.Session {
	h.t.Helper()
	s, err := h.resume(id, in)
	if err != nil {
		h.t.Fatalf("resume %s: %v", id, err)
	}
	if s.Info().ID != id {
		h.t.Fatalf("resumed as %s, want %s", s.Info().ID, id)
	}
	return s
}

func (h *harness) recoverable() []Record {
	h.t.Helper()
	list, err := h.svc.Records(true)
	if err != nil {
		h.t.Fatal(err)
	}
	return list
}

// storeSession writes an agent session into the fake agent's store, as if
// the agent had saved it.
func (h *harness) storeSession(id, cwd, title string, updated time.Time, messages ...string) {
	h.t.Helper()
	type message struct {
		ID   string `json:"id"`
		Role string `json:"role"`
		Text string `json:"text"`
	}
	stored := struct {
		ID        string            `json:"id"`
		Cwd       string            `json:"cwd"`
		Title     string            `json:"title"`
		UpdatedAt string            `json:"updatedAt"`
		Messages  []message         `json:"messages"`
		Config    map[string]string `json:"config"`
	}{ID: id, Cwd: cwd, Title: title, UpdatedAt: updated.UTC().Format(time.RFC3339Nano), Messages: []message{}, Config: map[string]string{"mode": "code", "model": "fast"}}
	for i, text := range messages {
		stored.Messages = append(stored.Messages, message{ID: "msg_" + id + string(rune('a'+i)), Role: "user", Text: text})
	}
	b, err := json.Marshal(stored)
	if err == nil {
		err = os.WriteFile(filepath.Join(h.store, "sessions", id+".json"), b, 0o600)
	}
	if err != nil {
		h.t.Fatal(err)
	}
}

func noViolations(t *testing.T, store string) {
	t.Helper()
	for _, e := range writerEvents(t, store) {
		if e.event == "violation" {
			t.Fatalf("two writers opened %s", e.id)
		}
	}
}

// A chat session goes through a restart; it can then be resumed on either
// side under its ID, with its conversation and recorded options.
func TestResumeAfterRestart(t *testing.T) {
	for _, side := range []Side{SideTUI, SideACP} {
		t.Run(string(side), func(t *testing.T) {
			h := newHarness(t, 0)
			in := h.input(agentprofile.OMP, SideACP)
			id := h.create(in).Info().ID
			plan := "plan"
			if err := h.svc.SetConfig(ctxT(t), id, ConfigInput{ClientRequestID: rid(), ConfigID: "mode", Value: &plan}); err != nil {
				t.Fatal(err)
			}
			chatTurn(t, h, id, "remember the lighthouse")
			sid := h.record(id).AgentSessionID

			h.restart()
			if list := h.manager.List(); len(list) != 0 {
				t.Fatalf("restart left sessions running: %+v", list)
			}
			list := h.recoverable()
			if len(list) != 1 || list[0].ID != id || list[0].State != StateInterrupted || list[0].Side != SideACP || list[0].InterruptedAt == nil {
				t.Fatalf("recoverable after restart %+v", list)
			}

			h.fast()
			s := h.mustResume(id, ResumeInput{Side: side, Env: in.Env})
			info := s.Info()
			if info.Hybrid == nil || info.Hybrid.Side != string(side) || info.Hybrid.State != string(StateActive) || info.Hybrid.AgentSessionID == nil || *info.Hybrid.AgentSessionID != sid {
				t.Fatalf("resumed hybrid info %+v", info.Hybrid)
			}
			rec := h.record(id)
			if rec.State != StateActive || rec.Side != side || rec.AgentSessionID != sid || rec.InterruptedAt != nil || rec.Activity != "" || rec.PID <= 0 || rec.Process == nil {
				t.Fatalf("resumed record %+v", rec)
			}
			if len(h.recoverable()) != 0 {
				t.Fatal("resumed session still recoverable")
			}
			if strings.Contains(h.file(), "secret-value") {
				t.Fatal("record file holds an environment value")
			}
			switch side {
			case SideTUI:
				waitFor(t, "previous conversation in the terminal", func() bool {
					return strings.Contains(string(s.Buffer()), "> remember the lighthouse")
				})
				if info.Status != session.StatusRunning || s.Identity().PID != rec.PID {
					t.Fatalf("terminal side: status %s pid %d", info.Status, s.Identity().PID)
				}
				h.waitEligible(id)
				h.mustSwitch(id, SideACP)
			case SideACP:
				if users := h.replayedUsers(id); !slices.Contains(users, "remember the lighthouse") {
					t.Fatalf("replayed users %q", users)
				}
				st := h.chatState(id)
				mode := ""
				for _, o := range st.Config {
					if o.ID == "mode" {
						mode = o.Value
					}
				}
				if mode != plan || st.Recovery != nil || st.AgentSessionID == nil || *st.AgentSessionID != sid {
					t.Fatalf("resumed chat: mode %q state %+v", mode, st)
				}
				chatTurn(t, h, id, "and after the restart")
				h.mustSwitch(id, SideTUI)
			}
			noViolations(t, h.store)
		})
	}
}

// A restart records what the session was doing and how many queued
// prompts it lost; the resumed chat says so.
func TestRestartRecordsLostQueue(t *testing.T) {
	h := newHarness(t, 0)
	in := h.input(agentprofile.OMP, SideACP, "FAKEAGENT_PERMISSION=1")
	id := h.create(in).Info().ID
	h.ask(id)
	st := h.follow(id)
	prompt(t, h, id, "needs permission")
	st.until("permission request", hasKind(EventRequest))
	if r := prompt(t, h, id, "waits in the queue"); !r.Queued {
		t.Fatalf("prompt during a turn %+v", r)
	}
	h.restart()
	rec := h.record(id)
	if rec.State != StateInterrupted || rec.Activity != "turn" || rec.QueuedCount != 1 {
		t.Fatalf("record after restart %+v", rec)
	}
	if strings.Contains(h.file(), "waits in the queue") {
		t.Fatal("record file holds queued text")
	}
	h.mustResume(id, ResumeInput{Env: in.Env})
	if lost := h.chatState(id).QueueLost; lost != 1 {
		t.Fatalf("queue lost %d", lost)
	}
	if rec := h.record(id); rec.QueuedCount != 0 || rec.Activity != "" || rec.Side != SideACP {
		t.Fatalf("resumed record %+v", rec)
	}
}

// An agent session deleted in the agent is offered alternatives from the
// same directory; picking one resumes it and keeps the old ID in history.
func TestResumeOffersCandidates(t *testing.T) {
	h := newHarness(t, 0)
	in := h.input(agentprofile.OMP, SideACP)
	id := h.create(in).Info().ID
	chatTurn(t, h, id, "first conversation")
	h.closeSession(id)
	rec := h.record(id)
	if err := os.Remove(filepath.Join(h.store, "sessions", rec.AgentSessionID+".json")); err != nil {
		t.Fatal(err)
	}
	h.storeSession("candold", rec.Cwd, "older work", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "older question")
	h.storeSession("candnew", rec.Cwd, "newer work", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), "newer question")
	h.storeSession("elsewhere", t.TempDir(), "other directory", time.Now())

	_, err := h.resume(id, ResumeInput{Side: SideACP, Env: in.Env})
	var missing *SessionMissingError
	if !errors.As(err, &missing) || !errors.Is(err, ErrSessionMissing) {
		t.Fatalf("resume of a deleted agent session: %v", err)
	}
	got := make([]string, len(missing.Candidates))
	for i, c := range missing.Candidates {
		got[i] = c.AgentSessionID
	}
	if !slices.Equal(got, []string{"candnew", "candold"}) || missing.Candidates[0].Title != "newer work" || missing.Candidates[0].UpdatedAt.IsZero() {
		t.Fatalf("candidates %+v", missing.Candidates)
	}
	if after := h.record(id); after.State != StateClosed || after.AgentSessionID != rec.AgentSessionID {
		t.Fatalf("failed resume changed the record %+v", after)
	}
	if _, err := h.manager.Get(id); err == nil {
		t.Fatal("failed resume left a shell")
	}
	if _, err := h.resume(id, ResumeInput{Side: SideTUI, AgentSessionID: "unlisted", Env: in.Env}); !errors.Is(err, ErrSessionMissing) {
		t.Fatalf("resume with an unlisted choice: %v", err)
	}

	s := h.mustResume(id, ResumeInput{Side: SideTUI, AgentSessionID: "candnew", Env: in.Env})
	waitFor(t, "chosen conversation", func() bool { return strings.Contains(string(s.Buffer()), "> newer question") })
	after := h.record(id)
	if after.AgentSessionID != "candnew" || !slices.Contains(after.PreviousIDs, rec.AgentSessionID) || after.State != StateActive || after.Side != SideTUI {
		t.Fatalf("record after choosing %+v", after)
	}
}

// A terminal UI that exited by itself leaves its shell for its last output;
// resuming replaces that shell under the same ID.
func TestResumeInterruptedTerminal(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	in := h.input(agentprofile.OMP, SideTUI)
	s := h.create(in)
	id := s.Info().ID
	tuiTurn(t, s, "hello there")
	waitFor(t, "recorded agent session", func() bool { return h.record(id).AgentSessionID != "" })
	if err := syscall.Kill(s.Identity().PID, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "interrupted", func() bool { return h.record(id).State == StateInterrupted })
	waitFor(t, "exited shell", func() bool { return s.Info().Status == session.StatusExited })

	resumed := h.mustResume(id, ResumeInput{Env: in.Env})
	if resumed == s {
		t.Fatal("exited shell reused")
	}
	select {
	case <-s.Done():
	default:
		t.Fatal("exited shell not removed")
	}
	if got, err := h.manager.Get(id); err != nil || got != resumed {
		t.Fatalf("manager holds %v, %v", got, err)
	}
	waitFor(t, "previous conversation", func() bool { return strings.Contains(string(resumed.Buffer()), "> hello there") })
	if info := resumed.Info(); info.Status == session.StatusExited || info.Hybrid.State != string(StateActive) || info.Hybrid.Side != string(SideTUI) {
		t.Fatalf("resumed %+v hybrid %+v", info, info.Hybrid)
	}
	tuiTurn(t, resumed, "still here")
	noViolations(t, h.store)
}

// Resuming needs a closed or interrupted record; a retry with the same
// request ID gets the first outcome.
func TestResumeRefusals(t *testing.T) {
	h := newHarness(t, 0)
	in := h.input(agentprofile.OMP, SideACP)
	id := h.create(in).Info().ID
	repeated := rid()
	if _, err := h.resume(id, ResumeInput{ClientRequestID: repeated, Env: in.Env}); !errors.Is(err, ErrConflict) {
		t.Fatalf("resume of an active session: %v", err)
	}
	if err := h.svc.RemoveRecord(ctxT(t), id); !errors.Is(err, ErrConflict) {
		t.Fatalf("removing an active record: %v", err)
	}
	if _, err := h.resume("zzzzzzzz", ResumeInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resume of an unknown record: %v", err)
	}
	if _, err := h.resume(id, ResumeInput{Side: "chat"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("resume to an unknown side: %v", err)
	}
	h.closeSession(id)
	if _, err := h.resume(id, ResumeInput{ClientRequestID: repeated, Env: in.Env}); !errors.Is(err, ErrConflict) {
		t.Fatalf("retried request: %v", err)
	}
	release, err := h.svc.claim(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.resume(id, ResumeInput{Env: in.Env}); !errors.Is(err, ErrSwitching) {
		t.Fatalf("resume during another resume: %v", err)
	}
	if err := h.svc.RemoveRecord(ctxT(t), id); !errors.Is(err, ErrSwitching) {
		t.Fatalf("removal during a resume: %v", err)
	}
	release()
	h.mustResume(id, ResumeInput{Env: in.Env})
}

// Records list newest first; removing one forgets it for good (and frees
// its place under the limit) but leaves the agent's session alone.
func TestRemoveRecord(t *testing.T) {
	h := &harness{t: t, state: t.TempDir(), store: t.TempDir(), logs: &syncBuffer{}}
	r := openRecords(h.state)
	for i := range MaxRecords - 2 {
		rec := testRecord(limitID(i))
		rec.State = StateClosed
		rec.UpdatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		if err := r.add(rec); err != nil {
			t.Fatal(err)
		}
	}
	r.close()
	h.open(0)
	in := h.input(agentprofile.OMP, SideACP)
	closed := h.create(in).Info().ID
	chatTurn(t, h, closed, "keep me")
	h.closeSession(closed)
	agentID := h.record(closed).AgentSessionID
	terminal := h.create(h.input(agentprofile.OMP, SideTUI))
	interrupted := terminal.Info().ID
	if err := syscall.Kill(terminal.Identity().PID, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "interrupted", func() bool { return h.record(interrupted).State == StateInterrupted })
	if _, err := h.svc.Create(ctxT(t), h.input(agentprofile.OMP, SideTUI)); !errors.Is(err, ErrLimit) {
		t.Fatalf("create at the limit: %v", err)
	}

	list := h.recoverable()
	if len(list) != MaxRecords || list[0].ID != interrupted || list[1].ID != closed {
		t.Fatalf("recoverable order: %s, %s of %d", list[0].ID, list[1].ID, len(list))
	}
	all, err := h.svc.Records(false)
	if err != nil || len(all) != MaxRecords {
		t.Fatalf("all records: %d, %v", len(all), err)
	}

	if err := h.svc.RemoveRecord(ctxT(t), closed); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Record(closed); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed record: %v", err)
	}
	if h.svc.records.reserved(closed) || strings.Contains(h.file(), closed) {
		t.Fatal("removed record kept")
	}
	if _, err := os.Stat(filepath.Join(h.store, "sessions", agentID+".json")); err != nil {
		t.Fatalf("agent session removed with the record: %v", err)
	}
	if err := h.svc.RemoveRecord(ctxT(t), interrupted); err != nil {
		t.Fatal(err)
	}
	if _, err := h.manager.Get(interrupted); err == nil {
		t.Fatal("interrupted shell kept after removal")
	}
	if err := h.svc.RemoveRecord(ctxT(t), interrupted); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second removal: %v", err)
	}
	h.create(h.input(agentprofile.OMP, SideTUI))
}

func TestRecoveryUnavailableOnCorruptStore(t *testing.T) {
	h := &harness{t: t, state: t.TempDir(), store: t.TempDir(), logs: &syncBuffer{}}
	path := filepath.Join(h.state, "hybrid-sessions.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.open(0)
	if _, err := h.svc.Records(true); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("records: %v", err)
	}
	if _, err := h.resume("abcdefgh", ResumeInput{}); !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), path) {
		t.Fatalf("resume: %v", err)
	}
	if err := h.svc.RemoveRecord(ctxT(t), "abcdefgh"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("remove: %v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "{broken" {
		t.Fatalf("corrupt store rewritten: %q", b)
	}
}
