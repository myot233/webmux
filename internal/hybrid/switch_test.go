package hybrid

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/session"
)

// fast shortens the service's waits; call it before creating sessions.
func (h *harness) fast() {
	h.svc.timing = timing{interrupt: 300 * time.Millisecond, probeDelay: 50 * time.Millisecond, observe: 20 * time.Millisecond, minute: time.Minute}
}

func (h *harness) switchTo(id string, side Side) (SwitchResult, error) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	return h.svc.Switch(ctx, id, SwitchInput{ClientRequestID: rid(), To: side})
}

func (h *harness) mustSwitch(id string, side Side) SwitchResult {
	h.t.Helper()
	r, err := h.switchTo(id, side)
	if err != nil {
		h.t.Fatalf("switch to %s: %v", side, err)
	}
	if r.Side != side || r.Dropped == nil || r.Unsupported == nil {
		h.t.Fatalf("switch result %+v", r)
	}
	return r
}

// actorState reads a value owned by session id's actor.
func actorState[T any](h *harness, id string, read func(a *actor) T) T {
	h.t.Helper()
	a, err := h.svc.actor(id)
	if err != nil {
		h.t.Fatal(err)
	}
	v, err := call(ctxT(h.t), a, func() (T, error) { return read(a), nil })
	if err != nil {
		h.t.Fatal(err)
	}
	return v
}

func (h *harness) eligibility(id string) Eligibility {
	return actorState(h, id, func(a *actor) Eligibility { return a.chat.Eligibility })
}

func (h *harness) chatState(id string) ChatState {
	return actorState(h, id, func(a *actor) ChatState { return a.chat })
}

// waitEligible waits until switching session id to the chat is possible.
func (h *harness) waitEligible(id string) {
	h.t.Helper()
	waitFor(h.t, "eligible", func() bool { return h.eligibility(id).Available })
}

// waitReason waits for an eligibility reason.
func (h *harness) waitReason(id, reason string) {
	h.t.Helper()
	waitFor(h.t, "eligibility "+reason, func() bool { return h.eligibility(id).Reason == reason })
}

// typeLine submits a line to the terminal UI.
func typeLine(t *testing.T, s *session.Session, line string) {
	t.Helper()
	if err := s.WriteContext(ctxT(t), []byte(line+"\r")); err != nil {
		t.Fatal(err)
	}
}

// tuiTurn submits a line and waits for its reply and the idle signal.
func tuiTurn(t *testing.T, s *session.Session, line string) {
	t.Helper()
	typeLine(t, s, line)
	waitFor(t, "reply to "+line, func() bool { return strings.Contains(string(s.Buffer()), "echo: "+line) })
	waitFor(t, "idle terminal", func() bool { return !tuiBusy(s.Info()) })
}

// chatTurn sends a prompt and waits for its turn to end normally.
func chatTurn(t *testing.T, h *harness, id, text string) {
	t.Helper()
	st := h.follow(id)
	before := 0
	for _, e := range st.events {
		if e.Kind == EventTurnEnd {
			before++
		}
	}
	prompt(t, h, id, text)
	events := st.until("turn end", count(EventTurnEnd, before+1))
	if end := events[len(events)-1]; end.Kind == EventTurnEnd && end.TurnEnd.StopReason != acp.StopEndTurn {
		t.Fatalf("turn %q ended %+v", text, end.TurnEnd)
	}
}

// replayedUsers is the user messages of session id's current log.
func (h *harness) replayedUsers(id string) []string {
	h.t.Helper()
	sub, err := h.svc.Subscribe(id, nil)
	if err != nil {
		h.t.Fatal(err)
	}
	defer sub.Close()
	return userTexts(sub.Events)
}

// writerEvent is one events.log line of the fake agent store.
type writerEvent struct {
	pid, mode, event, id string
}

func writerEvents(t *testing.T, store string) []writerEvent {
	t.Helper()
	text, err := readEventsLog(store)
	if err != nil {
		t.Fatal(err)
	}
	var out []writerEvent
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		f := strings.Fields(line)
		if len(f) >= 5 && f[3] != "call" {
			out = append(out, writerEvent{pid: f[1], mode: f[2], event: f[3], id: f[4]})
		}
	}
	return out
}

// assertSingleWriter replays the store's lease log: no agent session is
// ever opened by a process while another holds it, and no agent saw a
// second writer (violation).
func assertSingleWriter(t *testing.T, store string) {
	t.Helper()
	holders := map[string]writerEvent{}
	for _, e := range writerEvents(t, store) {
		switch e.event {
		case "violation", "fork":
			t.Fatalf("second writer: %+v", e)
		case "open":
			if held, ok := holders[e.id]; ok && held.pid != e.pid {
				t.Fatalf("%s opened %s while %s %s held it", e.mode, e.id, held.mode, held.pid)
			}
			holders[e.id] = e
		case "close":
			delete(holders, e.id)
		}
	}
}

func countCalls(t *testing.T, store, method string) int {
	t.Helper()
	text, err := readEventsLog(store)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return strings.Count(text, " call "+method+" ")
}

// TestSwitchRoundTripFromTerminal: each side sees every earlier message,
// the TUI's resume command reopens the chat's session, and no two
// processes ever hold the agent session.
func TestSwitchRoundTripFromTerminal(t *testing.T) {
	for _, profile := range []string{agentprofile.OMP, agentprofile.OpenCode} {
		t.Run(profile, func(t *testing.T) {
			h := newHarness(t, 0)
			h.fast()
			s := h.create(h.input(profile, SideTUI))
			id := s.Info().ID
			tuiTurn(t, s, "one")
			h.waitEligible(id)
			sid := *s.Info().Hybrid.AgentSessionID
			h.mustSwitch(id, SideACP)
			if info := s.Info(); info.Hybrid.Side != string(SideACP) || s.Identity().PID != 0 {
				t.Fatalf("after switch to chat %+v", info)
			}
			if got := h.replayedUsers(id); !slices.Equal(got, []string{"one"}) {
				t.Fatalf("replay after the first switch %q", got)
			}
			if rec := h.record(id); rec.Side != SideACP || rec.AgentSessionID != sid || rec.PID <= 0 {
				t.Fatalf("record %+v", rec)
			}
			chatTurn(t, h, id, "two")
			h.mustSwitch(id, SideTUI)
			waitFor(t, "resumed transcript", func() bool { return strings.Contains(string(s.Buffer()), "> two") })
			tuiTurn(t, s, "three")
			h.waitEligible(id)
			h.mustSwitch(id, SideACP)
			if got := h.replayedUsers(id); !slices.Equal(got, []string{"one", "two", "three"}) {
				t.Fatalf("replay after the round trip %q", got)
			}
			if got := *h.chatState(id).AgentSessionID; got != sid {
				t.Fatalf("agent session %s, want %s", got, sid)
			}
			assertSingleWriter(t, h.store)
		})
	}
}

// TestSwitchRoundTripFromChat starts in the chat, continues in the
// terminal and comes back; the switches publish markers and drop terminal
// clients with Moved.
func TestSwitchRoundTripFromChat(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	s := h.create(h.input(agentprofile.OMP, SideACP))
	id := s.Info().ID
	chatTurn(t, h, id, "alpha")
	st := h.follow(id)
	h.mustSwitch(id, SideTUI)
	st.until("switched marker", func(events []Event) bool {
		for _, e := range events {
			if e.Kind == EventMarker && e.Marker.Kind == MarkerSwitched && e.Marker.Side == SideTUI {
				return true
			}
		}
		return false
	})
	if rec := h.record(id); rec.Side != SideTUI || rec.PID != s.Identity().PID {
		t.Fatalf("record %+v, terminal pid %d", rec, s.Identity().PID)
	}
	waitFor(t, "resumed transcript", func() bool { return strings.Contains(string(s.Buffer()), "> alpha") })
	sub, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	tuiTurn(t, s, "beta")
	h.waitEligible(id)
	h.mustSwitch(id, SideACP)
	moved := false
	for !moved {
		select {
		case m := <-sub.Out:
			moved = m.Moved != nil
		case <-time.After(testTimeout):
			t.Fatal("terminal client not moved")
		}
	}
	if got := h.replayedUsers(id); !slices.Equal(got, []string{"alpha", "beta"}) {
		t.Fatalf("replay %q", got)
	}
	chatTurn(t, h, id, "gamma")
	assertSingleWriter(t, h.store)
}

// TestSwitchBackWithoutTerminalInput: the terminal UI resumed by a switch
// counts as having its session ID even before it reports one, so the user
// can look at the terminal and switch straight back. OpenCode names its
// session only after a message.
func TestSwitchBackWithoutTerminalInput(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	s := h.create(h.input(agentprofile.OpenCode, SideACP))
	id := s.Info().ID
	chatTurn(t, h, id, "alpha")
	h.mustSwitch(id, SideTUI)
	waitFor(t, "resumed transcript", func() bool { return strings.Contains(string(s.Buffer()), "> alpha") })
	h.waitEligible(id)
	h.mustSwitch(id, SideACP)
	if got := h.replayedUsers(id); !slices.Equal(got, []string{"alpha"}) {
		t.Fatalf("replay %q", got)
	}
	chatTurn(t, h, id, "beta")
	assertSingleWriter(t, h.store)
}

// TestSwitchMidTurnInterrupts: a running terminal turn is interrupted with
// the profile's key and the switch goes on once the terminal is idle; the
// interrupted turn is in the loaded history.
func TestSwitchMidTurnInterrupts(t *testing.T) {
	for _, profile := range []string{agentprofile.OMP, agentprofile.OpenCode} {
		t.Run(profile, func(t *testing.T) {
			h := newHarness(t, 0)
			h.fast()
			h.svc.timing.interrupt = 5 * time.Second
			s := h.create(h.input(profile, SideTUI, "FAKEAGENT_TURN_DELAY=150ms"))
			id := s.Info().ID
			tuiTurn(t, s, "first")
			h.waitEligible(id)
			typeLine(t, s, strings.Repeat("word ", 40))
			waitFor(t, "terminal turn", func() bool { return tuiBusy(s.Info()) })
			started := time.Now()
			h.mustSwitch(id, SideACP)
			if time.Since(started) > 4*time.Second {
				t.Fatal("the switch waited for the turn instead of interrupting it")
			}
			sub, err := h.svc.Subscribe(id, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			interrupted := false
			for _, e := range sub.Events {
				interrupted = interrupted || (e.Kind == EventMessage && strings.Contains(e.Message.Text, "(interrupted)"))
			}
			if !interrupted {
				t.Fatalf("interrupted turn missing from the history: %q", userTexts(sub.Events))
			}
			chatTurn(t, h, id, "after")
			assertSingleWriter(t, h.store)
		})
	}
}

// startSwitch runs a switch in the background.
func startSwitch(h *harness, id string, side Side) <-chan switchOutcome {
	out := make(chan switchOutcome, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*testTimeout)
		defer cancel()
		r, err := h.svc.Switch(ctx, id, SwitchInput{ClientRequestID: rid(), To: side})
		out <- switchOutcome{r, err}
	}()
	return out
}

func (h *harness) waitPhase(id string, phase Phase) {
	h.t.Helper()
	waitFor(h.t, "phase "+string(phase), func() bool { return h.chatState(id).Phase == phase })
}

func (h *harness) choose(id, choice string) SwitchResult {
	h.t.Helper()
	r, err := h.svc.Switch(ctxT(h.t), id, SwitchInput{ClientRequestID: rid(), Choice: choice})
	if err != nil {
		h.t.Fatalf("choice %s: %v", choice, err)
	}
	return r
}

// TestSwitchAwaitingChoice: a terminal that ignores the interrupt key
// pauses the switch for the user (D21): wait asks again after another
// period, cancel stays in the terminal with the TUI running, force ends it.
func TestSwitchAwaitingChoice(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	s := h.create(h.input(agentprofile.OMP, SideTUI, "FAKEAGENT_IGNORE_INTERRUPT=1", "FAKEAGENT_TURN_DELAY=200ms"))
	id := s.Info().ID
	tuiTurn(t, s, "first")
	h.waitEligible(id)
	typeLine(t, s, strings.Repeat("slow ", 200))
	waitFor(t, "terminal turn", func() bool { return tuiBusy(s.Info()) })
	pid := s.Identity().PID

	done := startSwitch(h, id, SideACP)
	h.waitPhase(id, PhaseAwaitingChoice)
	if info := s.Info(); info.Hybrid.Side != string(SideSwitching) || info.Hybrid.Phase != string(PhaseAwaitingChoice) {
		t.Fatalf("overlay while awaiting %+v", info.Hybrid)
	}
	if _, err := s.Attach(); err != nil {
		t.Fatalf("run gone while awaiting: %v", err)
	}
	if r := h.choose(id, ChoiceWait); r.Side != SideSwitching {
		t.Fatalf("wait %+v", r)
	}
	if p := h.chatState(id).Phase; p != PhaseInterrupting && p != PhaseAwaitingChoice {
		t.Fatalf("phase after wait %s", p)
	}
	h.waitPhase(id, PhaseAwaitingChoice)
	if r := h.choose(id, ChoiceCancel); r.Side != SideTUI {
		t.Fatalf("cancel %+v", r)
	}
	out := <-done
	if out.err != nil || out.result.Side != SideTUI {
		t.Fatalf("cancelled switch %+v %v", out.result, out.err)
	}
	if info := s.Info(); info.Hybrid.Side != string(SideTUI) || info.Hybrid.Phase != "" || s.Identity().PID != pid {
		t.Fatalf("after cancel %+v pid %d", info.Hybrid, s.Identity().PID)
	}
	if _, err := h.svc.Switch(ctxT(t), id, SwitchInput{ClientRequestID: rid(), Choice: ChoiceForce}); !errors.Is(err, ErrConflict) {
		t.Fatalf("choice without a waiting switch: %v", err)
	}

	done = startSwitch(h, id, SideACP)
	h.waitPhase(id, PhaseAwaitingChoice)
	h.choose(id, ChoiceForce)
	out = <-done
	if out.err != nil || out.result.Side != SideACP {
		t.Fatalf("forced switch %+v %v", out.result, out.err)
	}
	chatTurn(t, h, id, "after")
	assertSingleWriter(t, h.store)
}

// TestSwitchChoiceRacesIdle: a choice that reaches the actor while the
// terminal UI goes idle is acted on, not lost: an accepted cancel keeps
// the terminal side even though the switch saw the terminal go idle.
func TestSwitchChoiceRacesIdle(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	s := h.create(h.input(agentprofile.OMP, SideTUI, "FAKEAGENT_IGNORE_INTERRUPT=1", "FAKEAGENT_TURN_DELAY=100ms"))
	id := s.Info().ID
	tuiTurn(t, s, "first")
	h.waitEligible(id)
	typeLine(t, s, strings.Repeat("slow ", 40))
	waitFor(t, "terminal turn", func() bool { return tuiBusy(s.Info()) })
	done := startSwitch(h, id, SideACP)
	h.waitPhase(id, PhaseAwaitingChoice)

	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	// Hold the actor: the cancel queues first, the switch's idle check
	// after it, once the terminal turn ends.
	held, release := make(chan struct{}), make(chan struct{})
	a.post(func() { close(held); <-release })
	<-held
	type answer struct {
		result SwitchResult
		err    error
	}
	chosen := make(chan answer, 1)
	a.post(func() { r, err := a.choose(ChoiceCancel); chosen <- answer{r, err} })
	waitFor(t, "idle terminal", func() bool { return !tuiBusy(s.Info()) })
	time.Sleep(5 * idlePoll)
	close(release)

	got := <-chosen
	if got.err != nil || got.result.Side != SideTUI {
		t.Fatalf("cancel %+v %v", got.result, got.err)
	}
	out := <-done
	if out.err != nil || out.result.Side != SideTUI {
		t.Fatalf("switch after an accepted cancel %+v %v", out.result, out.err)
	}
	if info := s.Info(); info.Hybrid.Side != string(SideTUI) || info.Status == session.StatusExited {
		t.Fatalf("after cancel %+v", info.Hybrid)
	}
}

// TestSwitchToTerminalReturnsDraft (D20): a running turn is cancelled, its
// pending permission answered cancelled, and the queued prompts become the
// requester's draft only; every device gets a notice without the text.
func TestSwitchToTerminalReturnsDraft(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	s := h.create(h.input(agentprofile.OMP, SideACP, "FAKEAGENT_PERMISSION=1"))
	id := s.Info().ID
	h.ask(id)
	st := h.follow(id)
	prompt(t, h, id, "busy")
	st.until("permission", hasKind(EventRequest))
	for _, text := range []string{"queued one", "queued two"} {
		if r := prompt(t, h, id, text); !r.Queued {
			t.Fatalf("%q not queued: %+v", text, r)
		}
	}
	r := h.mustSwitch(id, SideTUI)
	if r.Draft != "queued one\n\nqueued two" {
		t.Fatalf("draft %q", r.Draft)
	}
	events := st.until("notice", hasKind(EventNotice))
	for _, e := range events {
		if e.Kind == EventNotice && strings.Contains(e.Notice.Text, "queued") {
			t.Fatalf("notice carries the draft: %q", e.Notice.Text)
		}
	}
	state := h.chatState(id)
	if len(state.Queue.Items) != 0 || len(state.Requests) != 0 || state.Turn.Running {
		t.Fatalf("state after the switch %+v", state)
	}
	log, err := readEventsLog(h.store)
	if err != nil || !strings.Contains(log, "permission cancelled") {
		t.Fatalf("permission not cancelled: %v\n%s", err, log)
	}
	assertSingleWriter(t, h.store)
}

// TestSwitchReappliesModeAfterLoad (D25): omp loads a session in its
// default mode, so the recorded mode is set again after a switch.
func TestSwitchReappliesModeAfterLoad(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	s := h.create(h.input(agentprofile.OMP, SideACP))
	id := s.Info().ID
	plan := "plan"
	if err := h.svc.SetConfig(ctxT(t), id, ConfigInput{ClientRequestID: rid(), ConfigID: "mode", Value: &plan}); err != nil {
		t.Fatal(err)
	}
	chatTurn(t, h, id, "hello")
	if rec := h.record(id); rec.Config["mode"] != "plan" {
		t.Fatalf("recorded config %v", rec.Config)
	}
	h.mustSwitch(id, SideTUI)
	h.waitEligible(id)
	before := countCalls(t, h.store, acp.MethodSessionSetConfigOption)
	h.mustSwitch(id, SideACP)
	if got := countCalls(t, h.store, acp.MethodSessionSetConfigOption); got != before+1 {
		t.Fatalf("set_config_option calls %d, want %d", got, before+1)
	}
	mode := ""
	for _, o := range h.chatState(id).Config {
		if o.ID == "mode" {
			mode = o.Value
		}
	}
	if mode != "plan" || h.record(id).Config["mode"] != "plan" {
		t.Fatalf("mode after the switch %q, record %v", mode, h.record(id).Config)
	}
}

// TestSwitchConflicts: while a switch runs, another switch, a prompt and
// a close get ErrSwitching; a retried request ID gets the first outcome.
func TestSwitchConflicts(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	s := h.create(h.input(agentprofile.OMP, SideTUI, "FAKEAGENT_DELAY=session/load=800ms"))
	id := s.Info().ID
	tuiTurn(t, s, "one")
	h.waitEligible(id)
	key := rid()
	done := make(chan switchOutcome, 1)
	go func() {
		r, err := h.svc.Switch(context.Background(), id, SwitchInput{ClientRequestID: key, To: SideACP})
		done <- switchOutcome{r, err}
	}()
	h.waitPhase(id, PhaseLoading)
	if _, err := h.switchTo(id, SideACP); !errors.Is(err, ErrSwitching) {
		t.Fatalf("second switch: %v", err)
	}
	if _, err := h.svc.Prompt(ctxT(t), id, PromptInput{ClientRequestID: rid(), Text: "hi"}); !errors.Is(err, ErrSwitching) {
		t.Fatalf("prompt while switching: %v", err)
	}
	if err := h.manager.Close(id); !errors.Is(err, ErrSwitching) {
		t.Fatalf("close while switching: %v", err)
	}
	if s.Info().Hybrid.Side != string(SideSwitching) {
		t.Fatalf("side while switching %+v", s.Info().Hybrid)
	}
	if _, err := s.Attach(); err == nil {
		t.Fatal("terminal attachable while loading")
	}
	out := <-done
	if out.err != nil || out.result.Side != SideACP {
		t.Fatalf("switch %+v %v", out.result, out.err)
	}
	again, err := h.svc.Switch(ctxT(t), id, SwitchInput{ClientRequestID: key, To: SideACP})
	if err != nil || again.Side != SideACP {
		t.Fatalf("retried switch %+v %v", again, err)
	}
	if got := countCalls(t, h.store, acp.MethodSessionLoad); got != 1 {
		t.Fatalf("session/load ran %d times", got)
	}
}

// TestSwitchLoadFailureRollsBack (D22): a failed session/load returns to
// the terminal with the TUI resumed and the reason on every device.
func TestSwitchLoadFailureRollsBack(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	s := h.create(h.input(agentprofile.OMP, SideTUI, "FAKEAGENT_FAIL=session/load=-32603"))
	id := s.Info().ID
	tuiTurn(t, s, "one")
	h.waitEligible(id)
	_, err := h.switchTo(id, SideACP)
	if !errors.Is(err, ErrAgent) || !strings.Contains(err.Error(), "-32603") {
		t.Fatalf("switch: %v", err)
	}
	state := h.chatState(id)
	if state.Side != SideTUI || !strings.Contains(state.SwitchError, "session/load") || state.Phase != "" {
		t.Fatalf("state after rollback %+v", state)
	}
	if rec := h.record(id); rec.State != StateActive || rec.Side != SideTUI || rec.PID != s.Identity().PID || rec.PID == 0 {
		t.Fatalf("record after rollback %+v", rec)
	}
	waitFor(t, "resumed transcript", func() bool {
		return strings.Count(string(s.Buffer()), "> one") >= 1 && s.Info().Status != session.StatusExited
	})
	tuiTurn(t, s, "two")
	assertSingleWriter(t, h.store)
}

// TestSwitchRollbackFailureInterrupts: when the terminal cannot start and
// the chat cannot be reloaded either, the session is interrupted and its
// shell kept.
func TestSwitchRollbackFailureInterrupts(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	in := h.input(agentprofile.OMP, SideACP)
	exe := filepath.Join(t.TempDir(), agentprofile.OMP)
	copyFile(t, in.Cmd[0], exe)
	in.Cmd = []string{exe}
	s := h.create(in)
	id := s.Info().ID
	chatTurn(t, h, id, "hello")
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	if _, err := h.switchTo(id, SideTUI); err == nil {
		t.Fatal("switch without an executable succeeded")
	}
	rec := h.record(id)
	if rec.State != StateInterrupted || rec.Activity != "switching" || rec.PID != 0 {
		t.Fatalf("record %+v", rec)
	}
	if _, err := h.manager.Get(id); err != nil {
		t.Fatal("interrupted shell removed")
	}
	if state := h.chatState(id); state.SwitchError == "" || state.Side != SideACP {
		t.Fatalf("state %+v", state)
	}
}

// TestSwitchFailureKeepsQueue: a switch to the terminal that can neither
// start the terminal UI nor reload the chat gives the queued prompts back
// to the chat, paused, and records them as lost; no device is told they
// went to a draft.
func TestSwitchFailureKeepsQueue(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	in := h.input(agentprofile.OMP, SideACP, "FAKEAGENT_PERMISSION=1")
	exe := filepath.Join(t.TempDir(), agentprofile.OMP)
	copyFile(t, in.Cmd[0], exe)
	in.Cmd = []string{exe}
	s := h.create(in)
	id := s.Info().ID
	h.ask(id)
	st := h.follow(id)
	prompt(t, h, id, "busy")
	st.until("permission", hasKind(EventRequest))
	queued := []string{"queued one", "queued two"}
	for _, text := range queued {
		if r := prompt(t, h, id, text); !r.Queued {
			t.Fatalf("%q not queued: %+v", text, r)
		}
	}
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	if _, err := h.switchTo(id, SideTUI); err == nil {
		t.Fatal("switch without an executable succeeded")
	}
	state := h.chatState(id)
	if state.Side != SideACP || !state.Queue.Paused || !slices.Equal(queueTexts(state.Queue), queued) {
		t.Fatalf("state after the failed switch %+v", state)
	}
	if rec := h.record(id); rec.State != StateInterrupted || rec.QueuedCount != len(queued) {
		t.Fatalf("record %+v", rec)
	}
	sub, err := h.svc.Subscribe(id, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	for _, e := range sub.Events {
		if e.Kind == EventNotice && strings.Contains(e.Notice.Text, "退回") {
			t.Fatalf("notice of a draft that was never returned: %q", e.Notice.Text)
		}
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	src, err := os.Open(from)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	if err := dst.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestEligibilityBeforeFirstMessage: OpenCode reports no session before
// its first message; omp reports one that is not listed until then.
func TestEligibilityBeforeFirstMessage(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	open := h.create(h.input(agentprofile.OpenCode, SideTUI))
	h.waitReason(open.Info().ID, ReasonNoSessionID)
	var ineligible *IneligibleError
	if _, err := h.switchTo(open.Info().ID, SideACP); !errors.As(err, &ineligible) || ineligible.Reason != ReasonNoSessionID {
		t.Fatalf("switch before the first message: %v", err)
	}
	if info := open.Info(); info.Hybrid.Side != string(SideTUI) || open.Identity().PID == 0 {
		t.Fatalf("refused switch changed the session %+v", info.Hybrid)
	}

	omp := h.create(h.input(agentprofile.OMP, SideTUI))
	id := omp.Info().ID
	waitFor(t, "omp session id", func() bool { return omp.Info().Hybrid.AgentSessionID != nil })
	h.waitReason(id, ReasonNotListed)
	if _, err := h.switchTo(id, SideACP); !errors.As(err, &ineligible) || ineligible.Reason != ReasonNotListed {
		t.Fatalf("switch before the session is saved: %v", err)
	}
	tuiTurn(t, omp, "first")
	h.waitEligible(id)
	if c := h.chatState(id).IDChange; c != nil {
		t.Fatalf("first ID reported as a change %+v", c)
	}
}

// TestEligibilityNotRoot: an agent started by another program in the PTY
// is not the root process.
func TestEligibilityNotRoot(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	in := h.input(agentprofile.OMP, SideTUI)
	script := filepath.Join(t.TempDir(), agentprofile.OMP)
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+in.Cmd[0]+" \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	in.Cmd = []string{script}
	s := h.create(in)
	tuiTurn(t, s, "first")
	h.waitReason(s.Info().ID, ReasonNotRoot)
	var ineligible *IneligibleError
	if _, err := h.switchTo(s.Info().ID, SideACP); !errors.As(err, &ineligible) || ineligible.Reason != ReasonNotRoot {
		t.Fatalf("switch: %v", err)
	}
}

// TestEligibilityOccupied: an ordinary session reporting the same agent
// session makes it occupied (D26).
func TestEligibilityOccupied(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	s := h.create(h.input(agentprofile.OMP, SideTUI))
	id := s.Info().ID
	tuiTurn(t, s, "first")
	h.waitEligible(id)
	sid := *s.Info().Hybrid.AgentSessionID
	event := `{"v":1,"event":"session_start","agent":"omp","plugin_version":"1.0.0","session_id":"` + sid + `","cwd":"/tmp"}`
	plain, err := h.manager.Create(session.CreateInput{Cwd: t.TempDir(), Cmd: []string{"sh", "-c", `printf '\033]777;notify;warp://cli-agent;%s\007' '` + event + `'; exec sleep 60`}})
	if err != nil {
		t.Fatal(err)
	}
	h.waitReason(id, ReasonOccupied)
	var ineligible *IneligibleError
	if _, err := h.switchTo(id, SideACP); !errors.As(err, &ineligible) || ineligible.Reason != ReasonOccupied {
		t.Fatalf("switch: %v", err)
	}
	if plain.Info().Hybrid != nil {
		t.Fatal("ordinary session got hybrid info")
	}
	plain.Close()
	h.waitEligible(id)
}

// TestAgentSessionIDChange: /new in the terminal moves to another agent
// session; the record keeps the old ID, every device gets the notice, and
// the switch loads the new session.
func TestAgentSessionIDChange(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	s := h.create(h.input(agentprofile.OMP, SideTUI))
	id := s.Info().ID
	tuiTurn(t, s, "first")
	h.waitEligible(id)
	old := *s.Info().Hybrid.AgentSessionID
	typeLine(t, s, "/new")
	waitFor(t, "new agent session", func() bool { p := s.Info().Hybrid.AgentSessionID; return p != nil && *p != old })
	fresh := *s.Info().Hybrid.AgentSessionID
	if c := h.chatState(id).IDChange; c == nil || c.From != old || c.To != fresh {
		t.Fatalf("id change %+v", c)
	}
	if rec := h.record(id); rec.AgentSessionID != fresh || !slices.Contains(rec.PreviousIDs, old) {
		t.Fatalf("record %+v", rec)
	}
	tuiTurn(t, s, "second")
	h.waitEligible(id)
	h.mustSwitch(id, SideACP)
	if got := h.replayedUsers(id); !slices.Equal(got, []string{"second"}) {
		t.Fatalf("loaded %q", got)
	}
	if got := *h.chatState(id).AgentSessionID; got != fresh {
		t.Fatalf("loaded agent session %s, want %s", got, fresh)
	}
}

// TestProbeDebounce: quick ID changes probe only the last one, once.
func TestProbeDebounce(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	h.svc.timing.probeDelay = 600 * time.Millisecond
	s := h.create(h.input(agentprofile.OMP, SideTUI))
	id := s.Info().ID
	waitFor(t, "omp session id", func() bool { return s.Info().Hybrid.AgentSessionID != nil })
	if err := s.WriteContext(ctxT(t), []byte("/new\r/new\r/new\r")); err != nil {
		t.Fatal(err)
	}
	h.waitReason(id, ReasonNotListed)
	time.Sleep(h.svc.timing.probeDelay + 300*time.Millisecond)
	if got := countCalls(t, h.store, acp.MethodSessionList); got != 1 {
		t.Fatalf("session/list ran %d times, want 1", got)
	}
	if got := countCalls(t, h.store, acp.MethodSessionLoad) + countCalls(t, h.store, acp.MethodSessionResume); got != 0 {
		t.Fatalf("a probe opened the session %d times", got)
	}
}

// TestProbeConcurrency: at most maxProbes probes run at once, server-wide.
func TestProbeConcurrency(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	const sessions = 3
	ids := make([]string, sessions)
	var wg sync.WaitGroup
	for i := range sessions {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids[i] = h.create(h.input(agentprofile.OMP, SideTUI, "FAKEAGENT_DELAY=session/list=700ms")).Info().ID
		}()
	}
	wg.Wait()
	for _, id := range ids {
		h.waitReason(id, ReasonNotListed)
	}
	text, err := readEventsLog(h.store)
	if err != nil {
		t.Fatal(err)
	}
	var starts []int64
	for _, line := range strings.Split(text, "\n") {
		if f := strings.Fields(line); len(f) >= 5 && f[3] == "call" && f[4] == acp.MethodSessionList {
			n, err := strconv.ParseInt(f[0], 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			starts = append(starts, n)
		}
	}
	slices.Sort(starts)
	if len(starts) != sessions {
		t.Fatalf("%d probes, want %d", len(starts), sessions)
	}
	if gap := time.Duration(starts[maxProbes] - starts[0]); gap < 500*time.Millisecond {
		t.Fatalf("probe %d started %v after the first: more than %d at once", maxProbes+1, gap, maxProbes)
	}
}

// TestChatAgentKilledMidTurnReconnects (D24): SIGKILL of the ACP process
// mid-turn ends the turn as cancelled; the session reconnects and takes
// prompts again.
func TestChatAgentKilledMidTurnReconnects(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	s := h.create(h.input(agentprofile.OMP, SideACP, "FAKEAGENT_TURN_DELAY=300ms"))
	id := s.Info().ID
	pid := h.record(id).PID
	st := h.follow(id)
	prompt(t, h, id, "long")
	st.until("thought", hasKind(EventMessage))
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	events := st.until("reconnected", func(events []Event) bool {
		return slices.ContainsFunc(events, func(e Event) bool { return e.Kind == EventMarker && e.Marker.Kind == MarkerReconnected })
	})
	end := slices.IndexFunc(events, func(e Event) bool { return e.Kind == EventTurnEnd })
	if end < 0 || events[end].TurnEnd.StopReason != acp.StopCancelled {
		t.Fatalf("turn after SIGKILL %v", kinds(events))
	}
	waitFor(t, "new agent process", func() bool { p := h.record(id).PID; return p > 0 && p != pid })
	chatTurn(t, h, id, "after")
	if rec := h.record(id); rec.State != StateActive {
		t.Fatalf("record %+v", rec)
	}
}

// TestCloseInterruptsTerminalTurn: closing during a terminal turn sends
// the interrupt key and waits (bounded) for the turn to end before the
// terminal UI is stopped.
func TestCloseInterruptsTerminalTurn(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	s := h.create(h.input(agentprofile.OMP, SideTUI, "FAKEAGENT_IGNORE_INTERRUPT=1", "FAKEAGENT_TURN_DELAY=200ms"))
	id := s.Info().ID
	typeLine(t, s, strings.Repeat("slow ", 100))
	waitFor(t, "terminal turn", func() bool { return tuiBusy(s.Info()) })
	started := time.Now()
	h.closeSession(id)
	if waited := time.Since(started); waited < h.svc.timing.interrupt {
		t.Fatalf("close waited %v, less than the interrupt wait", waited)
	}
	if rec := h.record(id); rec.State != StateClosed {
		t.Fatalf("record %+v", rec)
	}
}
