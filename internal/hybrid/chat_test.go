package hybrid

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/session"
)

// stream follows a subscription, collecting its events.
type stream struct {
	t      *testing.T
	sub    *Subscription
	events []Event
	state  ChatState
}

func (h *harness) follow(id string) *stream {
	h.t.Helper()
	sub, err := h.svc.Subscribe(id, nil)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(sub.Close)
	return &stream{t: h.t, sub: sub, events: append([]Event(nil), sub.Events...), state: sub.Snapshot.State}
}

// until reads frames until ok holds for the collected events.
func (s *stream) until(what string, ok func([]Event) bool) []Event {
	s.t.Helper()
	timeout := time.After(testTimeout)
	for !ok(s.events) {
		select {
		case f := <-s.sub.Out:
			s.events = append(s.events, f.Events...)
			if f.State != nil {
				s.state = *f.State
			}
		case <-s.sub.Done:
			s.t.Fatalf("subscription ended (%v) waiting for %s", s.sub.Err(), what)
		case <-timeout:
			s.t.Fatalf("timed out waiting for %s; kinds %v", what, kinds(s.events))
		}
	}
	return s.events
}

func kinds(events []Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Kind
	}
	return out
}

func hasKind(kind string) func([]Event) bool {
	return func(events []Event) bool {
		for _, e := range events {
			if e.Kind == kind {
				return true
			}
		}
		return false
	}
}

func count(kind string, n int) func([]Event) bool {
	return func(events []Event) bool {
		c := 0
		for _, e := range events {
			if e.Kind == kind {
				c++
			}
		}
		return c >= n
	}
}

// turnEvents drops the commands and config events agents send on their own
// schedule after session/new.
func turnEvents(events []Event) []Event {
	var out []Event
	for _, e := range events {
		if e.Kind != EventCommands && e.Kind != EventConfig {
			out = append(out, e)
		}
	}
	return out
}

var requestIDs atomic.Int64

// rid returns a fresh client request ID.
func rid() string { return fmt.Sprintf("req-%08d", requestIDs.Add(1)) }

func prompt(t *testing.T, h *harness, id, text string) PromptResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	r, err := h.svc.Prompt(ctx, id, PromptInput{ClientRequestID: rid(), Text: text})
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	return r
}

func TestChatTurnEventSequence(t *testing.T) {
	h := newHarness(t, 0)
	s := h.create(h.input(agentprofile.OMP, SideACP))
	id := s.Info().ID
	st := h.follow(id)
	started := prompt(t, h, id, "hello /plan")
	events := turnEvents(st.until("turn end", hasKind(EventTurnEnd)))
	want := []string{
		EventTurnStart, EventMessage, EventMessage, EventPlan,
		EventTool, EventTool, EventTool, EventMessage, EventMessage, EventUsage, EventTurnEnd,
	}
	if strings.Join(kinds(events), " ") != strings.Join(want, " ") {
		t.Fatalf("event kinds %v, want %v", kinds(events), want)
	}
	all := st.events
	for i := 1; i < len(all); i++ {
		if all[i].Seq != all[i-1].Seq+1 || all[i].At.IsZero() {
			t.Fatalf("sequence gap at %d: %d after %d", i, all[i].Seq, all[i-1].Seq)
		}
	}
	if ts := events[0].TurnStart; ts.TurnID != started.TurnID || ts.PromptID != started.PromptID {
		t.Fatalf("turn start %+v, prompt %+v", ts, started)
	}
	user, thought, agent1, agent2 := events[1].Message, events[2].Message, events[7].Message, events[8].Message
	if user.Role != RoleUser || user.Text != "hello /plan" || thought.Role != RoleThought || agent1.Role != RoleAgent {
		t.Fatalf("messages %+v %+v %+v", user, thought, agent1)
	}
	if user.MessageID == thought.MessageID || thought.MessageID == agent1.MessageID || agent1.MessageID != agent2.MessageID {
		t.Fatalf("message IDs user %q thought %q agent %q/%q", user.MessageID, thought.MessageID, agent1.MessageID, agent2.MessageID)
	}
	if !strings.HasPrefix(thought.MessageID, "wm") || agent1.Text+agent2.Text != "echo: hello /plan" {
		t.Fatalf("thought ID %q, agent text %q", thought.MessageID, agent1.Text+agent2.Text)
	}
	if p := events[3].Plan; len(p.Entries) != 2 || p.Entries[1].Status != "in_progress" {
		t.Fatalf("plan %+v", p)
	}
	end := events[len(events)-1].TurnEnd
	if end.TurnID != started.TurnID || end.StopReason != acp.StopEndTurn || end.Error != "" {
		t.Fatalf("turn end %+v", end)
	}
	if u := events[9].Usage; u.Size != 200000 || u.Cost == nil || u.Currency != "USD" {
		t.Fatalf("usage %+v", u)
	}
	st.until("idle state", func([]Event) bool { return !st.state.Turn.Running && st.state.Turn.StopReason == acp.StopEndTurn })
	if st.state.Plan == nil || st.state.Usage == nil || len(st.state.Commands) == 0 || len(st.state.Config) != 2 || st.state.Turn.TurnID != started.TurnID {
		t.Fatalf("state after the turn %+v", st.state)
	}
	// end_turn latches done attention: the session waits for the user.
	if info := s.Info(); info.Status != session.StatusWaiting || info.Attention == nil {
		t.Fatalf("status after the turn %s, attention %+v", info.Status, info.Attention)
	}
}

func TestChatToolCallUpsert(t *testing.T) {
	h := newHarness(t, 0)
	s := h.create(h.input(agentprofile.OpenCode, SideACP))
	id := s.Info().ID
	st := h.follow(id)
	prompt(t, h, id, "read it")
	var tools []*Tool
	for _, e := range st.until("turn end", hasKind(EventTurnEnd)) {
		if e.Kind == EventTool {
			tools = append(tools, e.Tool)
		}
	}
	if len(tools) != 3 {
		t.Fatalf("%d tool events", len(tools))
	}
	for _, tool := range tools {
		if tool.ToolCallID != "call_1" {
			t.Fatalf("tool call ID %q", tool.ToolCallID)
		}
	}
	first, progress, done := tools[0], tools[1], tools[2]
	if first.Title == nil || *first.Title != "Read file" || first.Kind == nil || *first.Kind != "read" || *first.Status != acp.ToolPending || len(first.Locations) != 1 {
		t.Fatalf("first upsert %+v", first)
	}
	// A patch carries only what changed.
	if progress.Title != nil || progress.Kind != nil || progress.Content != nil || progress.Locations != nil || *progress.Status != acp.ToolInProgress {
		t.Fatalf("progress patch %+v", progress)
	}
	if *done.Status != acp.ToolCompleted || len(done.Content) != 1 || done.Content[0].Type != "text" || done.Content[0].Text != "file contents" {
		t.Fatalf("completion patch %+v", done)
	}
}

func TestChatOversizeFieldTruncated(t *testing.T) {
	h := newHarness(t, 0)
	s := h.create(h.input(agentprofile.OMP, SideACP))
	id := s.Info().ID
	st := h.follow(id)
	// Prompts are limited far below the field limit; send past the
	// command check to exercise the translator's cut.
	text := strings.Repeat("x", maxFieldBytes+1000)
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call(context.Background(), a, func() (PromptResult, error) { return a.send(text), nil }); err != nil {
		t.Fatal(err)
	}
	var user, reply *MessageChunk
	for _, e := range st.until("turn end", hasKind(EventTurnEnd)) {
		if e.Kind == EventMessage && e.Message.Role == RoleUser {
			user = e.Message
		}
		if e.Kind == EventMessage && e.Message.Role == RoleAgent && reply == nil {
			reply = e.Message
		}
	}
	if user == nil || !user.Truncated || len(user.Text) != maxFieldBytes {
		t.Fatalf("user message not cut to the field limit: %d bytes", len(user.Text))
	}
	// The agent got the whole prompt: its echo (in two halves) is longer.
	if reply == nil || reply.Truncated || len(reply.Text) <= maxFieldBytes/2 {
		t.Fatalf("reply %d bytes, truncated %v", len(reply.Text), reply.Truncated)
	}
}

func TestChatLogTruncationAdvancesFrom(t *testing.T) {
	h := newHarness(t, 4<<10)
	s := h.create(h.input(agentprofile.OMP, SideACP))
	id := s.Info().ID
	st := h.follow(id)
	if st.sub.Snapshot.From != 1 || st.sub.Snapshot.Truncated {
		t.Fatalf("fresh snapshot %+v", st.sub.Snapshot)
	}
	for i := range 3 {
		prompt(t, h, id, strings.Repeat("long prompt ", 100))
		st.until("turn end", count(EventTurnEnd, i+1))
	}
	sub, err := h.svc.Subscribe(id, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	snap := sub.Snapshot
	if !snap.Truncated || snap.From <= 1 || len(sub.Events) == 0 || sub.Events[0].Seq != snap.From || sub.Events[len(sub.Events)-1].Seq != snap.Next-1 {
		t.Fatalf("truncated snapshot %+v with %d events", snap, len(sub.Events))
	}
	size := 0
	for _, e := range sub.Events {
		size += eventSize(e)
	}
	if size > 4<<10 {
		t.Fatalf("log holds %d bytes over its 4 KiB limit", size)
	}
	// A client that last saw a dropped event starts over.
	old, err := h.svc.Subscribe(id, &ResumePoint{Generation: snap.Generation, Seq: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if !old.Snapshot.Reset || old.Events[0].Seq != snap.From {
		t.Fatalf("stale resume %+v", old.Snapshot)
	}
}

func TestChatPermissionAnswerAndCancel(t *testing.T) {
	h := newHarness(t, 0)
	s := h.create(h.input(agentprofile.OMP, SideACP, "FAKEAGENT_PERMISSION=1"))
	id := s.Info().ID
	st := h.follow(id)
	ctx := context.Background()

	prompt(t, h, id, "first")
	events := st.until("permission request", hasKind(EventRequest))
	req := events[len(events)-1].Request
	if req.Kind != RequestPermission || req.ToolCallID != "call_1" || req.Title != "Read file" || len(req.Options) != 2 || req.Options[0].Kind != acp.PermissionAllowOnce {
		t.Fatalf("request %+v", req)
	}
	st.until("pending request state", func([]Event) bool { return len(st.state.Requests) == 1 })
	if st.state.Requests[0].RequestID != req.RequestID {
		t.Fatalf("pending requests in state %+v", st.state.Requests)
	}
	if err := h.svc.Answer(ctx, id, rid(), req.RequestID, Answer{Outcome: OutcomeSelected, OptionID: "nope"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown option: %v", err)
	}
	if err := h.svc.Answer(ctx, id, rid(), req.RequestID, Answer{Outcome: OutcomeSelected, OptionID: "allow"}); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Answer(ctx, id, rid(), req.RequestID, Answer{Outcome: OutcomeCancelled}); !errors.Is(err, ErrAnswered) {
		t.Fatalf("second answer: %v", err)
	}
	events = st.until("first turn end", hasKind(EventTurnEnd))
	var resolved *RequestOutcome
	for _, e := range events {
		if e.Kind == EventResolved {
			resolved = e.Resolved
		}
	}
	if resolved == nil || resolved.RequestID != req.RequestID || resolved.Outcome != OutcomeSelected || resolved.OptionID != "allow" {
		t.Fatalf("resolution %+v", resolved)
	}
	if end := events[len(events)-1].TurnEnd; end.StopReason != acp.StopEndTurn {
		t.Fatalf("first turn %+v", end)
	}

	prompt(t, h, id, "second")
	events = st.until("second request", count(EventRequest, 2))
	second := events[len(events)-1].Request
	if err := h.svc.Cancel(ctx, id, rid()); err != nil {
		t.Fatal(err)
	}
	events = st.until("second turn end", count(EventTurnEnd, 2))
	var cancelled *RequestOutcome
	for _, e := range events {
		if e.Kind == EventResolved && e.Resolved.RequestID == second.RequestID {
			cancelled = e.Resolved
		}
	}
	if cancelled == nil || cancelled.Outcome != OutcomeCancelled {
		t.Fatalf("cancelled request %+v", cancelled)
	}
	if end := events[len(events)-1].TurnEnd; end.StopReason != acp.StopCancelled {
		t.Fatalf("cancelled turn %+v", end)
	}
	st.until("no pending requests", func([]Event) bool { return len(st.state.Requests) == 0 && !st.state.Turn.Running })
}

func TestChatTurnErrors(t *testing.T) {
	for _, row := range []struct {
		code, stop string
		auth       bool
	}{
		{"-32000", StopAuthRequired, true},
		{"-32603", StopError, false},
	} {
		t.Run(row.code, func(t *testing.T) {
			h := newHarness(t, 0)
			s := h.create(h.input(agentprofile.OpenCode, SideACP, "FAKEAGENT_FAIL=session/prompt="+row.code))
			id := s.Info().ID
			st := h.follow(id)
			prompt(t, h, id, "hi")
			events := st.until("turn end", hasKind(EventTurnEnd))
			end := events[len(events)-1].TurnEnd
			if end.StopReason != row.stop || !strings.Contains(end.Error, "injected failure") {
				t.Fatalf("turn end %+v", end)
			}
			st.until("auth state", func([]Event) bool { return !st.state.Turn.Running })
			if st.state.AuthRequired != row.auth {
				t.Fatalf("authRequired %v", st.state.AuthRequired)
			}
		})
	}
}

func TestPromptNeedsChatSide(t *testing.T) {
	h := newHarness(t, 0)
	s := h.create(h.input(agentprofile.OMP, SideTUI))
	if _, err := h.svc.Prompt(context.Background(), s.Info().ID, PromptInput{ClientRequestID: rid(), Text: "hi"}); !errors.Is(err, ErrWrongSide) {
		t.Fatalf("prompt on the terminal side: %v", err)
	}
	if _, err := h.svc.Prompt(context.Background(), "zzzzzzzz", PromptInput{ClientRequestID: rid(), Text: "hi"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("prompt to an unknown session: %v", err)
	}
}

// An ACP crash mid-turn ends the turn as cancelled and reconnects once
// with session/resume (D24); crashing again before a turn completes
// interrupts the session.
func TestChatAgentCrashReconnectsOnce(t *testing.T) {
	h := newHarness(t, 0)
	s := h.create(h.input(agentprofile.OMP, SideACP, "FAKEAGENT_CRASH=prompt-mid"))
	id := s.Info().ID
	first := h.record(id).PID
	st := h.follow(id)
	generation := st.sub.Snapshot.Generation
	prompt(t, h, id, "boom")
	events := st.until("reconnected", func(events []Event) bool {
		for _, e := range events {
			if e.Kind == EventMarker && e.Marker.Kind == MarkerReconnected {
				return true
			}
		}
		return false
	})
	var end *TurnEnd
	for _, e := range events {
		if e.Kind == EventTurnEnd {
			end = e.TurnEnd
		}
	}
	if end == nil || end.StopReason != acp.StopCancelled {
		t.Fatalf("turn end after a crash %+v", end)
	}
	waitFor(t, "new agent process", func() bool { rec := h.record(id); return rec.PID > 0 && rec.PID != first })
	if rec := h.record(id); rec.State != StateActive {
		t.Fatalf("record after reconnecting %+v", rec)
	}
	// session/resume keeps the log: no new generation.
	if sub, err := h.svc.Subscribe(id, nil); err != nil || sub.Snapshot.Generation != generation {
		t.Fatalf("generation after resume: %v", err)
	} else {
		sub.Close()
	}
	prompt(t, h, id, "again")
	waitFor(t, "interrupted record", func() bool { return h.record(id).State == StateInterrupted })
	if rec := h.record(id); rec.InterruptedAt == nil {
		t.Fatalf("record %+v", rec)
	}
	// The crashed process is forgotten once its stop confirms it gone.
	waitFor(t, "stopped agent forgotten", func() bool { rec := h.record(id); return rec.PID == 0 && rec.Process == nil })
	if _, err := h.svc.Prompt(context.Background(), id, PromptInput{ClientRequestID: rid(), Text: "later"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("prompt after a second crash: %v", err)
	}
}
