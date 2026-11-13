package hybrid

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/rainchan/webmux/internal/session"
)

// TestDevinRoundTrip exercises Devin's whole switch cycle: the terminal
// reports only app=devin, so the agent session comes from session/list
// candidates; its ID is Devin's slug, launch arguments seed chat config,
// and a persisted mode survives every load without reapply.
func TestDevinRoundTrip(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	in := h.input("devin", SideTUI)
	in.Cmd = append(in.Cmd, "--model", "claude-fable-5-2-medium", "--permission-mode", "dangerous", "--sandbox")
	s := h.create(in)
	id := s.Info().ID
	h.waitReason(id, ReasonNoSessionID)
	if s.Info().Hybrid.AgentSessionID != nil {
		t.Fatal("Devin terminal reported a session ID before its first message")
	}
	tuiTurn(t, s, "one")
	h.waitEligible(id)
	sid := devinStoredSessionID(t, h.store, "one")
	if parts := strings.Split(sid, "-"); len(parts) != 2 {
		t.Fatalf("Devin session ID %q is not an adjective-noun slug", sid)
	}

	result := h.mustSwitch(id, SideACP)
	if !slices.Equal(result.Unsupported, []string{"--sandbox"}) {
		t.Fatalf("unsupported %q", result.Unsupported)
	}
	if got := h.replayedUsers(id); !slices.Equal(got, []string{"one"}) {
		t.Fatalf("replay after first switch %q", got)
	}
	st := h.chatState(id)
	if configValue(st.Config, "model") != "claude-fable-5-2-medium" || configValue(st.Config, "mode") != "bypass" || configValue(st.Config, "thought_level") != "medium" || configValue(st.Config, "speed") != "standard" {
		t.Fatalf("chat config %+v", st.Config)
	}
	for _, hidden := range []string{"login", "logout"} {
		if slices.ContainsFunc(st.Commands, func(c Command) bool { return c.Name == hidden }) {
			t.Fatalf("commands include %s: %+v", hidden, st.Commands)
		}
	}
	plan := "plan"
	if err := h.svc.SetConfig(ctxT(t), id, ConfigInput{ClientRequestID: rid(), ConfigID: "mode", Value: &plan}); err != nil {
		t.Fatal(err)
	}
	chatTurn(t, h, id, "two")

	h.mustSwitch(id, SideTUI)
	waitFor(t, "resumed transcript", func() bool { return strings.Contains(string(s.Buffer()), "> two") })
	h.waitEligible(id)
	if got := *s.Info().Hybrid.AgentSessionID; got != sid {
		t.Fatalf("resumed terminal session %s, want %s", got, sid)
	}
	tuiTurn(t, s, "three")
	h.waitEligible(id)
	h.mustSwitch(id, SideACP)
	if got := h.replayedUsers(id); !slices.Equal(got, []string{"one", "two", "three"}) {
		t.Fatalf("replay after round trip %q", got)
	}
	st = h.chatState(id)
	if configValue(st.Config, "mode") != "plan" || configValue(st.Config, "model") != "claude-fable-5-2-medium" {
		t.Fatalf("config after cold load %+v", st.Config)
	}
	assertSingleWriter(t, h.store)
}

// TestDevinCandidateChoice: Devin's terminal never names its session, so
// session/list candidates decide; two sessions in one directory ask which.
func TestDevinCandidateChoice(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	in := h.input("devin", SideTUI)
	s := h.create(in)
	id := s.Info().ID
	tuiTurn(t, s, "one")
	h.waitEligible(id)
	sid := devinStoredSessionID(t, h.store, "one")

	otherInput := h.input("devin", SideACP)
	otherInput.Cwd = in.Cwd
	otherID := h.create(otherInput).Info().ID
	chatTurn(t, h, otherID, "other")
	_, err := h.switchTo(id, SideACP)
	var ambiguous *SessionAmbiguousError
	if !errors.As(err, &ambiguous) || len(ambiguous.Candidates) != 2 {
		t.Fatalf("switch with two candidates: %+v %v", ambiguous, err)
	}
	if _, err := h.svc.Switch(ctxT(t), id, SwitchInput{ClientRequestID: rid(), To: SideACP, AgentSessionID: sid}); err != nil {
		t.Fatalf("switch with selected candidate: %v", err)
	}
	if got := h.record(id).AgentSessionID; got != sid {
		t.Fatalf("selected candidate = %s, want %s", got, sid)
	}
}

// TestDevinStaleLockRecovery: a terminal that died without releasing its
// session leaves a lock whose holder is gone; the chat's session/load
// answers session_locked, releases it with a force session/close and loads.
func TestDevinStaleLockRecovery(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	in := h.input("devin", SideTUI)
	s := h.create(in)
	id := s.Info().ID
	tuiTurn(t, s, "one")
	h.waitEligible(id)
	sid := devinStoredSessionID(t, h.store, "one")
	pid := h.record(id).PID
	if pid <= 0 {
		t.Fatalf("terminal pid %d", pid)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "interrupted terminal", func() bool { return h.record(id).State == StateInterrupted })

	ctx := ctxT(t)
	if _, err := h.svc.Resume(ctx, id, ResumeInput{ClientRequestID: rid(), Side: SideACP, AgentSessionID: sid, Env: in.Env}); err != nil {
		t.Fatalf("resume past a stale lock: %v", err)
	}
	if got := h.replayedUsers(id); !slices.Contains(got, "one") {
		t.Fatalf("replayed %q after lock recovery", got)
	}
	log, err := readEventsLog(h.store)
	if err != nil || !strings.Contains(log, "force-close "+sid) || !strings.Contains(log, "session_locked "+sid) {
		t.Fatalf("lock recovery not exercised: %v %q", err, log)
	}
	chatTurn(t, h, id, "two")
}

// TestDevinExternalWriter: a live Devin client holding the session refuses
// resuming it to the chat with occupied_external.
func TestDevinExternalWriter(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	in := h.input("devin", SideTUI)
	s := h.create(in)
	id := s.Info().ID
	tuiTurn(t, s, "one")
	h.waitEligible(id)
	sid := devinStoredSessionID(t, h.store, "one")
	h.closeSession(id)
	other, err := h.manager.Create(session.CreateInput{Cwd: in.Cwd, Cmd: []string{in.Cmd[0], "--resume", sid}, Env: in.Env})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.manager.Close(other.Info().ID) })
	waitFor(t, "outside client holds the session", func() bool { return strings.Contains(string(other.Buffer()), "> one") })
	ctx := ctxT(t)
	if _, err := h.svc.Resume(ctx, id, ResumeInput{ClientRequestID: rid(), Side: SideACP, AgentSessionID: sid, Env: in.Env}); !errors.Is(err, ErrExternalWriter) {
		t.Fatalf("resume while held: %v", err)
	}
	if rec := h.record(id); rec.State != StateClosed {
		t.Fatalf("record after the refused resume %+v", rec)
	}
}

// devinStoredSessionID finds the stored Devin session whose messages
// contain message.
func devinStoredSessionID(t *testing.T, store, message string) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(store, "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(store, "sessions", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var stored struct {
			ID       string `json:"id"`
			Messages []struct {
				Text string `json:"text"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(data, &stored); err != nil {
			t.Fatal(err)
		}
		for _, msg := range stored.Messages {
			if msg.Text == message {
				return stored.ID
			}
		}
	}
	t.Fatalf("no stored Devin session contains %q", message)
	return ""
}
