package hybrid

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/settings"
)

// idleUnit stands in for a minute of chatIdleSleepMinutes.
const idleUnit = 100 * time.Millisecond

// sleepyHarness is a harness whose settings set chatIdleSleepMinutes to
// minutes, each lasting idleUnit.
func sleepyHarness(t *testing.T, minutes int) *harness {
	t.Helper()
	h := &harness{t: t, state: t.TempDir(), store: t.TempDir(), logs: &syncBuffer{}, prefs: settings.Open(t.TempDir())}
	t.Cleanup(h.prefs.Close)
	h.setIdle(minutes)
	h.open(0)
	h.fast()
	h.svc.timing.minute = idleUnit
	waitFor(t, "idle setting", func() bool { return h.svc.sleepAfter() == time.Duration(minutes)*idleUnit })
	return h
}

func (h *harness) setIdle(minutes int) {
	h.t.Helper()
	snap, err := h.prefs.Snapshot()
	if err != nil {
		h.t.Fatal(err)
	}
	if _, err := h.prefs.Patch(snap.Revision, settings.Changes{Fields: map[string]bool{"chatIdleSleepMinutes": true}, Values: settings.Values{ChatIdleSleepMinutes: &minutes}}); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) dormant(id string) bool {
	return actorState(h, id, func(a *actor) bool { return a.chat.Dormant && !a.sleeping })
}

func (h *harness) generation(id string) string {
	h.t.Helper()
	return actorState(h, id, func(a *actor) string {
		a.log.mu.Lock()
		defer a.log.mu.Unlock()
		return a.log.generation
	})
}

func hasMarker(kind string) func([]Event) bool {
	return func(events []Event) bool {
		return slices.ContainsFunc(events, func(e Event) bool { return e.Kind == EventMarker && e.Marker.Kind == kind })
	}
}

// TestIdleSleepAndWake (D27): an idle chat side closes its agent session
// and stops the process; opening the chat wakes it with session/resume
// (same log) when the agent declares it, else session/load (new
// generation).
func TestIdleSleepAndWake(t *testing.T) {
	for _, row := range []struct {
		name   string
		env    []string
		marker string
	}{
		{"resume", nil, MarkerResumed},
		{"load", []string{"FAKEAGENT_NO_RESUME=1"}, MarkerLoaded},
	} {
		t.Run(row.name, func(t *testing.T) {
			h := sleepyHarness(t, 2)
			s := h.create(h.input(agentprofile.OMP, SideACP, row.env...))
			id := s.Info().ID
			chatTurn(t, h, id, "hello")
			generation := h.generation(id)
			waitFor(t, "dormant", func() bool { return h.dormant(id) })
			if info := s.Info(); !info.Hybrid.Dormant || info.Hybrid.Side != string(SideACP) || info.Hybrid.State != string(StateActive) {
				t.Fatalf("dormant overlay %+v", info.Hybrid)
			}
			if rec := h.record(id); rec.PID != 0 || rec.State != StateActive {
				t.Fatalf("dormant record %+v", rec)
			}
			if countCalls(t, h.store, "session/close") != 1 {
				t.Fatal("sleep did not close the agent session")
			}
			st := h.follow(id)
			waitFor(t, "awake", func() bool { return !h.dormant(id) && h.record(id).PID > 0 })
			if row.marker == MarkerResumed {
				st.until("resumed marker", hasMarker(MarkerResumed))
				if h.generation(id) != generation {
					t.Fatal("resume started a new generation")
				}
			} else {
				if h.generation(id) == generation {
					t.Fatal("load kept the generation")
				}
				if got := h.replayedUsers(id); !slices.Equal(got, []string{"hello"}) {
					t.Fatalf("replay after wake %q", got)
				}
			}
			chatTurn(t, h, id, "again")
			assertSingleWriter(t, h.store)
		})
	}
}

// TestPromptWhileDormantIsSentAfterWake: a prompt wakes the session, waits
// in the queue and is sent once the agent is back.
func TestPromptWhileDormantIsSentAfterWake(t *testing.T) {
	h := sleepyHarness(t, 1)
	s := h.create(h.input(agentprofile.OMP, SideACP))
	id := s.Info().ID
	waitFor(t, "dormant", func() bool { return h.dormant(id) })
	h.setIdle(0)
	if r := prompt(t, h, id, "wake up"); !r.Queued {
		t.Fatalf("prompt while dormant %+v", r)
	}
	waitFor(t, "prompt sent after wake", func() bool {
		st := h.chatState(id)
		return !st.Dormant && st.Turn.StopReason == "end_turn" && len(st.Queue.Items) == 0
	})
	if got := h.replayedUsers(id); !slices.Contains(got, "wake up") {
		t.Fatalf("sent %q", got)
	}
}

// TestIdleSleepConditions: a pending request keeps the session awake; it
// sleeps once the turn is over.
func TestIdleSleepConditions(t *testing.T) {
	h := sleepyHarness(t, 1)
	s := h.create(h.input(agentprofile.OMP, SideACP, "FAKEAGENT_PERMISSION=1"))
	id := s.Info().ID
	st := h.follow(id)
	prompt(t, h, id, "ask")
	events := st.until("permission", hasKind(EventRequest))
	time.Sleep(4 * idleUnit)
	if h.dormant(id) {
		t.Fatal("slept with a pending request")
	}
	req := events[len(events)-1].Request
	if err := h.svc.Answer(ctxT(t), id, rid(), req.RequestID, Answer{Outcome: OutcomeSelected, OptionID: "allow"}); err != nil {
		t.Fatal(err)
	}
	st.until("turn end", hasKind(EventTurnEnd))
	waitFor(t, "dormant after the turn", func() bool { return h.dormant(id) })
}

// TestIdleSleepSettingAppliesImmediately: changing chatIdleSleepMinutes
// re-arms running sessions at once; 0 never sleeps.
func TestIdleSleepSettingAppliesImmediately(t *testing.T) {
	h := sleepyHarness(t, 0)
	s := h.create(h.input(agentprofile.OMP, SideACP))
	id := s.Info().ID
	time.Sleep(5 * idleUnit)
	if h.dormant(id) {
		t.Fatal("slept with idle sleep off")
	}
	h.setIdle(1)
	waitFor(t, "dormant after the change", func() bool { return h.dormant(id) })
}

// TestIdleSleepDefaults: without settings, or with unreadable ones, the
// chat side sleeps after 30 minutes.
func TestIdleSleepDefaults(t *testing.T) {
	h := newHarness(t, 0)
	if got := h.svc.sleepAfter(); got != 30*time.Minute {
		t.Fatalf("without settings %v", got)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	broken := &harness{t: t, state: t.TempDir(), store: t.TempDir(), logs: &syncBuffer{}, prefs: settings.Open(dir)}
	t.Cleanup(broken.prefs.Close)
	broken.open(0)
	<-broken.svc.settingsDone
	if got := broken.svc.sleepAfter(); got != 30*time.Minute {
		t.Fatalf("with unavailable settings %v", got)
	}
}
