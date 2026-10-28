package hybrid

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/attentionkind"
	"github.com/rainchan/webmux/internal/session"
)

func ctxT(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	t.Cleanup(cancel)
	return ctx
}

// userTexts lists the user messages of events in order.
func userTexts(events []Event) []string {
	var out []string
	for _, e := range events {
		if e.Kind == EventMessage && e.Message.Role == RoleUser {
			out = append(out, e.Message.Text)
		}
	}
	return out
}

// settle collects frames for d.
func (s *stream) settle(d time.Duration) {
	s.t.Helper()
	timeout := time.After(d)
	for {
		select {
		case f := <-s.sub.Out:
			s.events = append(s.events, f.Events...)
			if f.State != nil {
				s.state = *f.State
			}
		case <-s.sub.Done:
			s.t.Fatalf("subscription ended: %v", s.sub.Err())
		case <-timeout:
			return
		}
	}
}

// readEventsLog reads the fake agent store's event log.
func readEventsLog(store string) (string, error) {
	b, err := os.ReadFile(filepath.Join(store, "events.log"))
	return string(b), err
}

func queueTexts(q Queue) []string {
	out := make([]string, len(q.Items))
	for i, it := range q.Items {
		out[i] = it.Text
	}
	return out
}

// slowChat creates an omp chat session whose turns take a while.
func slowChat(t *testing.T, h *harness, profile string, env ...string) (string, *stream) {
	t.Helper()
	s := h.create(h.input(profile, SideACP, append([]string{"FAKEAGENT_TURN_DELAY=40ms"}, env...)...))
	id := s.Info().ID
	if slices.Contains(env, "FAKEAGENT_PERMISSION=1") {
		h.ask(id)
	}
	return id, h.follow(id)
}

func TestQueueRunWhileBusy(t *testing.T) {
	h := newHarness(t, 0)
	id, st := slowChat(t, h, agentprofile.OMP)
	ctx := ctxT(t)
	if r := prompt(t, h, id, "first"); r.Queued || r.TurnID == "" {
		t.Fatalf("idle prompt %+v", r)
	}
	a, err := h.svc.Prompt(ctx, id, PromptInput{ClientRequestID: rid(), Text: "A", Mode: ModeAuto})
	if err != nil {
		t.Fatal(err)
	}
	b, err := h.svc.Prompt(ctx, id, PromptInput{ClientRequestID: rid(), Text: "B"})
	if err != nil {
		t.Fatal(err)
	}
	if !a.Queued || !b.Queued || a.ItemID == b.ItemID || a.ItemID == "" {
		t.Fatalf("busy prompts %+v %+v", a, b)
	}
	st.until("queue of two", func([]Event) bool { return strings.Join(queueTexts(st.state.Queue), ",") == "A,B" })
	events := st.until("three turns", count(EventTurnEnd, 3))
	if got := strings.Join(userTexts(events), ","); got != "first,A,B" {
		t.Fatalf("sent order %s", got)
	}
	// Each queued prompt starts only after the previous turn ended.
	var order []string
	for _, e := range events {
		if e.Kind == EventTurnStart || e.Kind == EventTurnEnd {
			order = append(order, e.Kind)
		}
	}
	if strings.Join(order, " ") != "turn.start turn.end turn.start turn.end turn.start turn.end" {
		t.Fatalf("turn order %v", order)
	}
	st.until("empty queue", func([]Event) bool { return len(st.state.Queue.Items) == 0 && !st.state.Turn.Running })
}

func TestPromptRetryDedupe(t *testing.T) {
	h := newHarness(t, 0)
	id, st := slowChat(t, h, agentprofile.OMP)
	ctx := ctxT(t)
	in := PromptInput{ClientRequestID: rid(), Text: "once"}
	first, err := h.svc.Prompt(ctx, id, in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := h.svc.Prompt(ctx, id, in)
	if err != nil || again != first {
		t.Fatalf("retry %+v %v, first %+v", again, err, first)
	}
	// A queued retry neither queues twice.
	queued := PromptInput{ClientRequestID: rid(), Text: "later"}
	q1, err := h.svc.Prompt(ctx, id, queued)
	if err != nil {
		t.Fatal(err)
	}
	q2, err := h.svc.Prompt(ctx, id, queued)
	if err != nil || q2 != q1 || !q1.Queued {
		t.Fatalf("queued retry %+v %v, first %+v", q2, err, q1)
	}
	events := st.until("two turns", count(EventTurnEnd, 2))
	if got := strings.Join(userTexts(events), ","); got != "once,later" {
		t.Fatalf("sent %s", got)
	}
	// The first error is replayed too, and an ID cannot change command.
	bad := rid()
	if _, err := h.svc.Prompt(ctx, "zzzzzzzz", PromptInput{ClientRequestID: bad, Text: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown session %v", err)
	}
	if err := h.svc.Cancel(ctx, id, in.ClientRequestID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("reused ID for cancel %v", err)
	}
	if _, err := h.svc.Prompt(ctx, id, PromptInput{ClientRequestID: "short", Text: "x"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("malformed ID %v", err)
	}
	for _, text := range []string{"  \n\t", strings.Repeat("x", MaxPromptBytes+1)} {
		if _, err := h.svc.Prompt(ctx, id, PromptInput{ClientRequestID: rid(), Text: text}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("text of %d bytes: %v", len(text), err)
		}
	}
}

func TestQueueRemovedItemConflicts(t *testing.T) {
	h := newHarness(t, 0)
	id, st := slowChat(t, h, agentprofile.OMP, "FAKEAGENT_PERMISSION=1")
	ctx := ctxT(t)
	prompt(t, h, id, "busy")
	item, err := h.svc.Prompt(ctx, id, PromptInput{ClientRequestID: rid(), Text: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	// The desktop deletes it; the phone's edit and delete then conflict.
	if err := h.svc.RemoveQueued(ctx, id, rid(), item.ItemID); err != nil {
		t.Fatal(err)
	}
	st.until("empty queue", func([]Event) bool { return len(st.state.Queue.Items) == 0 })
	text := "edited"
	if err := h.svc.EditQueued(ctx, id, item.ItemID, QueueEdit{ClientRequestID: rid(), Text: &text}); !errors.Is(err, ErrSent) {
		t.Fatalf("edit of a removed item: %v", err)
	}
	if err := h.svc.RemoveQueued(ctx, id, rid(), item.ItemID); !errors.Is(err, ErrSent) {
		t.Fatalf("second delete: %v", err)
	}
	if err := h.svc.SendQueuedNow(ctx, id, rid(), "q99"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown item: %v", err)
	}
	if err := h.svc.EditQueued(ctx, id, "q1", QueueEdit{ClientRequestID: rid(), Move: "down"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad move: %v", err)
	}
}

func TestQueuePausesAfterStop(t *testing.T) {
	h := newHarness(t, 0)
	id, st := slowChat(t, h, agentprofile.OMP, "FAKEAGENT_PERMISSION=1")
	ctx := ctxT(t)
	prompt(t, h, id, "busy")
	for _, text := range []string{"one", "two"} {
		if _, err := h.svc.Prompt(ctx, id, PromptInput{ClientRequestID: rid(), Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	st.until("permission", hasKind(EventRequest))
	if err := h.svc.Cancel(ctx, id, rid()); err != nil {
		t.Fatal(err)
	}
	events := st.until("cancelled turn", hasKind(EventTurnEnd))
	if end := events[len(events)-1].TurnEnd; end.StopReason != acp.StopCancelled {
		t.Fatalf("turn end %+v", end)
	}
	st.until("paused queue", func([]Event) bool { return st.state.Queue.Paused })
	// Nothing was sent; a new prompt joins the paused queue.
	r, err := h.svc.Prompt(ctx, id, PromptInput{ClientRequestID: rid(), Text: "three"})
	if err != nil || !r.Queued {
		t.Fatalf("prompt while paused %+v %v", r, err)
	}
	st.until("three queued", func([]Event) bool { return len(st.state.Queue.Items) == 3 })
	st.settle(300 * time.Millisecond)
	if !st.state.Queue.Paused || st.state.Turn.Running {
		t.Fatalf("queue %+v turn %+v", st.state.Queue, st.state.Turn)
	}
	if got := strings.Join(userTexts(st.events), ","); got != "busy" {
		t.Fatalf("sent while paused: %s", got)
	}
	// Resume sends from the head.
	if err := h.svc.ResumeQueue(ctx, id, rid()); err != nil {
		t.Fatal(err)
	}
	st.until("head sent", count(EventTurnStart, 2))
	if got := userTexts(st.events); got[len(got)-1] != "one" {
		t.Fatalf("resumed with %v", got)
	}
	if err := h.svc.ClearQueue(ctx, id, rid()); err != nil {
		t.Fatal(err)
	}
	st.until("cleared", func([]Event) bool { return len(st.state.Queue.Items) == 0 && !st.state.Queue.Paused })
}

func TestQueueEditMoveAndSendNow(t *testing.T) {
	h := newHarness(t, 0)
	id, st := slowChat(t, h, agentprofile.OMP, "FAKEAGENT_PERMISSION=1")
	ctx := ctxT(t)
	prompt(t, h, id, "busy")
	var ids []string
	for _, text := range []string{"one", "two", "three"} {
		r, err := h.svc.Prompt(ctx, id, PromptInput{ClientRequestID: rid(), Text: text})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, r.ItemID)
	}
	text := "TWO"
	if err := h.svc.EditQueued(ctx, id, ids[1], QueueEdit{ClientRequestID: rid(), Text: &text, Move: MoveUp}); err != nil {
		t.Fatal(err)
	}
	st.until("edited queue", func([]Event) bool { return strings.Join(queueTexts(st.state.Queue), ",") == "TWO,one,three" })
	st.until("permission", hasKind(EventRequest))
	// Send-now cancels the running turn and sends "three" first.
	if err := h.svc.SendQueuedNow(ctx, id, rid(), ids[2]); err != nil {
		t.Fatal(err)
	}
	events := st.until("next turn", count(EventTurnStart, 2))
	if got := userTexts(events); got[len(got)-1] != "three" {
		t.Fatalf("send-now sent %v", got)
	}
	var stops []string
	for _, e := range events {
		if e.Kind == EventTurnEnd {
			stops = append(stops, e.TurnEnd.StopReason)
		}
	}
	if len(stops) != 1 || stops[0] != acp.StopCancelled {
		t.Fatalf("stops %v", stops)
	}
	st.until("rest queued, running", func([]Event) bool {
		return strings.Join(queueTexts(st.state.Queue), ",") == "TWO,one" && !st.state.Queue.Paused && st.state.Turn.Running
	})
}

func TestSteering(t *testing.T) {
	h := newHarness(t, 0)
	id, st := slowChat(t, h, agentprofile.OMP, "FAKEAGENT_PERMISSION=1")
	ctx := ctxT(t)
	if !st.state.Steering {
		t.Fatal("omp chat does not offer steering")
	}
	if _, err := h.svc.Prompt(ctx, id, PromptInput{ClientRequestID: rid(), Text: "nudge", Mode: ModeSteer}); !errors.Is(err, ErrSteeringUnavailable) {
		t.Fatalf("steer while idle: %v", err)
	}
	prompt(t, h, id, "busy")
	r, err := h.svc.Prompt(ctx, id, PromptInput{ClientRequestID: rid(), Text: "nudge", Mode: ModeSteer})
	if err != nil || r.Queued {
		t.Fatalf("steer %+v %v", r, err)
	}
	st.until("steered message", func(events []Event) bool { return strings.Join(userTexts(events), ",") == "busy,nudge" })
	if len(st.state.Queue.Items) != 0 {
		t.Fatalf("steering queued %+v", st.state.Queue)
	}
	waitFor(t, "steering call", func() bool {
		b, err := readEventsLog(h.store)
		return err == nil && strings.Contains(b, "call "+methodSteering)
	})
}

func TestOpenCodeHasNoSteering(t *testing.T) {
	h := newHarness(t, 0)
	id, st := slowChat(t, h, agentprofile.OpenCode, "FAKEAGENT_PERMISSION=1")
	if st.state.Steering {
		t.Fatal("OpenCode offers steering")
	}
	prompt(t, h, id, "busy")
	st.until("running", func([]Event) bool { return st.state.Turn.Running })
	if _, err := h.svc.Prompt(ctxT(t), id, PromptInput{ClientRequestID: rid(), Text: "nudge", Mode: ModeSteer}); !errors.Is(err, ErrSteeringUnavailable) {
		t.Fatalf("steer on OpenCode: %v", err)
	}
}

func TestChatCommandsNeedChatSide(t *testing.T) {
	h := newHarness(t, 0)
	id := h.create(h.input(agentprofile.OMP, SideTUI)).Info().ID
	ctx := ctxT(t)
	for name, err := range map[string]error{
		"cancel": h.svc.Cancel(ctx, id, rid()),
		"resume": h.svc.ResumeQueue(ctx, id, rid()),
		"clear":  h.svc.ClearQueue(ctx, id, rid()),
		"config": h.svc.SetConfig(ctx, id, ConfigInput{ClientRequestID: rid(), ConfigID: "model"}),
	} {
		if !errors.Is(err, ErrWrongSide) {
			t.Errorf("%s on the terminal side: %v", name, err)
		}
	}
}

// A switch (driven by a later milestone) refuses commands and clears the
// chat side's attention.
func TestChatSwitchingRefusesCommandsAndClearsAttention(t *testing.T) {
	h := newHarness(t, 0)
	id, st := slowChat(t, h, agentprofile.OMP)
	shell, err := h.manager.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	prompt(t, h, id, "hi")
	st.until("turn end", hasKind(EventTurnEnd))
	waitFor(t, "done attention", func() bool { a := shell.Info().Attention; return a != nil && a.Kind == attentionkind.Done })
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call(ctxT(t), a, func() (struct{}, error) {
		a.side, a.switching, a.dirty = SideSwitching, &switchOp{}, true
		return struct{}{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Prompt(ctxT(t), id, PromptInput{ClientRequestID: rid(), Text: "hi"}); !errors.Is(err, ErrSwitching) {
		t.Fatalf("prompt while switching: %v", err)
	}
	if info := shell.Info(); info.Attention != nil || info.Hybrid.Side != string(SideSwitching) {
		t.Fatalf("info while switching %+v", info)
	}
	cleared, err := call(ctxT(t), a, func() (bool, error) {
		a.side, a.switching, a.dirty = SideACP, nil, true
		return a.attention == nil, nil
	})
	if err != nil || !cleared {
		t.Fatalf("latched attention survived the switch: %v", err)
	}
}

func TestChatSetConfig(t *testing.T) {
	h := newHarness(t, 0)
	id, st := slowChat(t, h, agentprofile.OMP)
	ctx := ctxT(t)
	st.until("options", func([]Event) bool { return len(st.state.Config) == 2 })
	smart := "smart"
	if err := h.svc.SetConfig(ctx, id, ConfigInput{ClientRequestID: rid(), ConfigID: "model", Value: &smart}); err != nil {
		t.Fatal(err)
	}
	st.until("model smart", func([]Event) bool {
		for _, o := range st.state.Config {
			if o.ID == "model" {
				return o.Value == "smart"
			}
		}
		return false
	})
	waitFor(t, "recorded value", func() bool { return h.record(id).Config["model"] == "smart" })
	bad := "huge"
	if err := h.svc.SetConfig(ctx, id, ConfigInput{ClientRequestID: rid(), ConfigID: "model", Value: &bad}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown value: %v", err)
	}
	on := true
	if err := h.svc.SetConfig(ctx, id, ConfigInput{ClientRequestID: rid(), ConfigID: "model", Enabled: &on}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("boolean for a select: %v", err)
	}
	if err := h.svc.SetConfig(ctx, id, ConfigInput{ClientRequestID: rid(), ConfigID: "nope", Value: &smart}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown option: %v", err)
	}
}

func TestChatAttentionAndPreview(t *testing.T) {
	h := newHarness(t, 0)
	s := h.create(h.input(agentprofile.OMP, SideACP, "FAKEAGENT_PERMISSION=1"))
	id := s.Info().ID
	h.ask(id)
	st := h.follow(id)
	ctx := ctxT(t)
	info := func() session.Info { return s.Info() }

	prompt(t, h, id, "hello")
	events := st.until("permission", hasKind(EventRequest))
	req := events[len(events)-1].Request
	waitFor(t, "permission attention", func() bool {
		i := info()
		return i.Attention != nil && i.Attention.Kind == attentionkind.Permission && i.Attention.Source == session.SourceAgent && i.Status == session.StatusWaiting
	})
	if err := h.svc.Answer(ctx, id, rid(), req.RequestID, Answer{Outcome: OutcomeSelected, OptionID: "allow"}); err != nil {
		t.Fatal(err)
	}
	st.until("turn end", hasKind(EventTurnEnd))
	waitFor(t, "done attention", func() bool {
		i := info()
		return i.Attention != nil && i.Attention.Kind == attentionkind.Done && i.Status == session.StatusWaiting && i.Preview == "echo: hello"
	})
	// Sending clears it; the turn runs.
	prompt(t, h, id, "again")
	waitFor(t, "cleared attention", func() bool {
		i := info()
		return i.Attention == nil || i.Attention.Kind == attentionkind.Permission
	})
	st.until("second permission", count(EventRequest, 2))
	if err := h.svc.Cancel(ctx, id, rid()); err != nil {
		t.Fatal(err)
	}
	st.until("cancelled", count(EventTurnEnd, 2))
	waitFor(t, "idle without attention", func() bool { i := info(); return i.Attention == nil && i.Status == session.StatusIdle })
}

func TestChatErrorAttention(t *testing.T) {
	h := newHarness(t, 0)
	s := h.create(h.input(agentprofile.OpenCode, SideACP, "FAKEAGENT_FAIL=session/prompt=-32603"))
	id := s.Info().ID
	st := h.follow(id)
	prompt(t, h, id, "hi")
	st.until("turn end", hasKind(EventTurnEnd))
	waitFor(t, "error attention", func() bool {
		i := s.Info()
		return i.Attention != nil && i.Attention.Kind == attentionkind.Error && i.Status == session.StatusWaiting
	})
}

func TestTakeQueueMergesDraft(t *testing.T) {
	a := &actor{}
	a.setQueue([]QueueItem{{ID: "q1", Text: "one"}, {ID: "q2", Text: "two\nlines"}}, true)
	if got := a.takeQueue(); got != "one\n\ntwo\nlines" {
		t.Fatalf("draft %q", got)
	}
	if len(a.chat.Queue.Items) != 0 || a.chat.Queue.Paused || a.chat.Queue.Items == nil {
		t.Fatalf("queue after take %+v", a.chat.Queue)
	}
}

func TestPreviewText(t *testing.T) {
	var p preview
	for _, chunk := range []string{"\n  first", " line\n\n", "second\nthird\nfourth"} {
		p.add(&MessageChunk{MessageID: "m1", Role: RoleAgent, Text: chunk})
	}
	if p.text != "  first line\nsecond\nthird" || !p.full {
		t.Fatalf("preview %q full %v", p.text, p.full)
	}
	if p.add(&MessageChunk{MessageID: "m1", Role: RoleAgent, Text: "more"}) {
		t.Fatal("a full preview changed")
	}
	long := strings.Repeat("é", previewLineRunes+10)
	if !p.add(&MessageChunk{MessageID: "m2", Role: RoleAgent, Text: long}) || p.text != strings.Repeat("é", previewLineRunes) {
		t.Fatalf("new message preview %q", p.text)
	}
}

func TestDedupeBounds(t *testing.T) {
	var d dedupe
	ctx := context.Background()
	runs := 0
	run := func(context.Context) (int, error) { runs++; return runs, nil }
	for i := range dedupeEntries + 1 {
		if _, err := once(ctx, &d, "key-"+strings.Repeat("0", 4)+string(rune('a'+i%26))+string(rune('a'+i/26)), "x", run); err != nil {
			t.Fatal(err)
		}
	}
	if len(d.order) != dedupeEntries || len(d.entries) != dedupeEntries {
		t.Fatalf("%d entries, %d ordered", len(d.entries), len(d.order))
	}
	// The oldest was evicted and runs again; expired entries go too.
	if v, _ := once(ctx, &d, "key-0000aa", "x", run); v != dedupeEntries+2 {
		t.Fatalf("evicted key result %d", v)
	}
	oldest := d.order[0]
	oldest.at = time.Now().Add(-dedupeTTL - time.Second)
	d.claim("other-key", "x", time.Now())
	if _, ok := d.entries[oldest.key]; ok {
		t.Fatal("expired entry kept")
	}
	failing := errors.New("boom")
	calls := 0
	fail := func(context.Context) (int, error) { calls++; return 0, failing }
	for range 2 {
		if _, err := once(ctx, &d, "fail-key-1", "y", fail); !errors.Is(err, failing) {
			t.Fatalf("replayed error %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("failed command ran %d times", calls)
	}
}
