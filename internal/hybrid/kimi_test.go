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
	"time"

	"github.com/rainchan/webmux/internal/agentprofile/kimi"
	"github.com/rainchan/webmux/internal/agenttrust"
	"github.com/rainchan/webmux/internal/session"
)

func TestKimiRoundTripAndConfigReapply(t *testing.T) {
	h := newHarness(t, 0, func(options *session.Options) {
		options.PreLaunch = agenttrust.New().Prepare
	})
	h.fast()
	in := h.input(kimi.Name, SideTUI)
	in.Cmd = append(in.Cmd, "--model", "kimi-code-plan-cn/kimi-for-coding", "--auto")
	s := h.create(in)
	id := s.Info().ID
	h.waitReason(id, ReasonNoSessionID)
	if s.Info().Hybrid.AgentSessionID != nil {
		t.Fatal("Kimi terminal reported a session ID before its first message")
	}
	tuiTurn(t, s, "one")
	h.waitEligible(id)
	sid := kimiStoredSessionID(t, h.store, "one")
	if !strings.HasPrefix(sid, "session_") {
		t.Fatalf("Kimi session ID %q lacks session_ prefix", sid)
	}

	h.mustSwitch(id, SideACP)
	if got := h.replayedUsers(id); !slices.Equal(got, []string{"one"}) {
		t.Fatalf("replay after first switch %q", got)
	}
	state := h.chatState(id)
	if configValue(state.Config, "model") != "kimi-code-plan-cn/kimi-for-coding" || configValue(state.Config, "thinking") != "high" || configValue(state.Config, "mode") != "auto" {
		t.Fatalf("initial chat config %+v", state.Config)
	}
	log, err := readEventsLog(h.store)
	if err != nil || !strings.Contains(log, "KIMI_CODE_TRUST_WORKSPACE=1") {
		t.Fatalf("Kimi trust environment not inherited: %v", err)
	}
	for _, kv := range []struct{ id, val string }{{"thinking", "low"}, {"mode", "plan"}} {
		val := kv.val
		if err := h.svc.SetConfig(ctxT(t), id, ConfigInput{ClientRequestID: rid(), ConfigID: kv.id, Value: &val}); err != nil {
			t.Fatalf("set config %s=%s: %v", kv.id, kv.val, err)
		}
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
	state = h.chatState(id)
	if configValue(state.Config, "model") != "kimi-code-plan-cn/kimi-for-coding" || configValue(state.Config, "thinking") != "low" || configValue(state.Config, "mode") != "plan" {
		t.Fatalf("config after cold load %+v", state.Config)
	}
	assertSingleWriter(t, h.store)
}

func TestKimiCandidateChoice(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	in := h.input(kimi.Name, SideTUI)
	s := h.create(in)
	id := s.Info().ID
	tuiTurn(t, s, "one")
	h.waitEligible(id)
	sid := kimiStoredSessionID(t, h.store, "one")

	otherInput := h.input(kimi.Name, SideACP)
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

func TestKimiReconnectReappliesOnlyMode(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	in := h.input(kimi.Name, SideACP)
	in.Cmd = append(in.Cmd, "--model", "kimi-code-plan-cn/kimi-for-coding", "--auto")
	id := h.create(in).Info().ID
	for _, kv := range []struct{ id, val string }{{"thinking", "low"}, {"mode", "plan"}} {
		val := kv.val
		if err := h.svc.SetConfig(ctxT(t), id, ConfigInput{ClientRequestID: rid(), ConfigID: kv.id, Value: &val}); err != nil {
			t.Fatal(err)
		}
	}
	chatTurn(t, h, id, "hello")
	pid := h.record(id).PID
	callsBefore := countCalls(t, h.store, "session/set_config_option")
	st := h.follow(id)
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	st.until("reconnected marker", func(events []Event) bool {
		return slices.ContainsFunc(events, func(e Event) bool { return e.Kind == EventMarker && e.Marker.Kind == MarkerReconnected })
	})
	waitFor(t, "reconnected with restored config", func() bool {
		config := h.chatState(id).Config
		rec := h.record(id)
		return rec.PID > 0 && rec.PID != pid && configValue(config, "model") == "kimi-code-plan-cn/kimi-for-coding" && configValue(config, "thinking") == "low" && configValue(config, "mode") == "plan"
	})
	if got := countCalls(t, h.store, "session/set_config_option"); got != callsBefore+1 {
		t.Fatalf("set_config_option calls = %d, want %d (only mode)", got, callsBefore+1)
	}
	mode := "plan"
	if err := h.svc.SetConfig(ctxT(t), id, ConfigInput{ClientRequestID: rid(), ConfigID: "mode", Value: &mode}); err != nil {
		t.Fatalf("manually selecting the active mode: %v", err)
	}
}

func TestKimiNativeRootFallback(t *testing.T) {
	h := newHarness(t, 0)
	cmd, err := h.svc.agents.Parse([]string{"kimi"})
	if err != nil {
		t.Fatal(err)
	}
	a := &actor{cmd: cmd}
	if !a.agentIsRoot(session.Identity{PID: 42, Executable: "kimi"}) {
		t.Fatal("native Kimi is not accepted as the terminal root")
	}
	if a.agentIsRoot(session.Identity{PID: 42, Executable: "kimi-code"}) {
		t.Fatal("OSC app name incorrectly accepted as an executable")
	}
}

func kimiStoredSessionID(t *testing.T, store, message string) string {
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
	t.Fatalf("no stored Kimi session contains %q", message)
	return ""
}

func TestKimiInputSubmitDelay(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	s := h.create(h.input(kimi.Name, SideTUI))
	start := time.Now()
	_, err := h.svc.Input(ctxT(t), s.Info().ID, InputRequest{ClientRequestID: rid(), Text: "delayed", Submit: true})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 200*time.Millisecond {
		t.Fatalf("Kimi text and Enter were not separated: %v", elapsed)
	}
}
