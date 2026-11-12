package hybrid

import (
	"errors"
	"testing"
)

func TestBackgroundRefreshCannotBypassUnresolvedDispatch(t *testing.T) {
	h := newHarness(t, 0)
	id := h.create(h.input("omp", SideACP)).Info().ID
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	if !a.sync(func() {
		a.contextExecutionID = "unresolved-handoff"
		_, err := a.sendTurn("background status refresh", nil, "", "", true)
		if !errors.Is(err, ErrSwitching) {
			t.Errorf("background refresh bypassed writer admission: %v", err)
		}
		if a.turn != nil {
			t.Error("blocked background refresh started a provider turn")
		}
	}) {
		t.Fatal("actor stopped")
	}
}
