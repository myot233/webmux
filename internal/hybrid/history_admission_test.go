package hybrid

import (
	"errors"
	"testing"
)

func TestHistoryWriterRejectsUnreportedChildWork(t *testing.T) {
	h := newHarness(t, 0)
	shell := h.create(h.codexInput(SideACP, ""))
	id := shell.Info().ID
	stream := h.follow(id)
	prompt(t, h, id, "hi /background")
	stream.until("turn ended with surviving process", hasKind(EventTurnEnd))
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	if !a.sync(func() {
		// Reproduce an agent that omits background-work updates. Only the
		// process-tree evidence remains after the normal turn tracker clears.
		a.chat.Turn.Unfinished = false
		a.work.endTurn()
		a.stopBackgroundWatch()
		if !a.canSendNow() {
			t.Error("fixture is not otherwise idle")
			return
		}
		before := a.link
		if !hasNewProcess(before.sidecar.PID(), a.bgBase) {
			t.Error("background child missing")
			return
		}
		if err := a.admitHistoryWriter(); !errors.Is(err, ErrBusy) {
			t.Errorf("admitted surviving child: %v", err)
		}
		if a.link != before || !hasNewProcess(before.sidecar.PID(), a.bgBase) {
			t.Error("refusal replaced or killed the existing writer")
		}
	}) {
		t.Fatal("actor stopped")
	}
}
