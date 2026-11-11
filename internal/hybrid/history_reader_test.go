package hybrid

import (
	"bytes"
	"io"
	"testing"

	"github.com/rainchan/webmux/internal/acp"
)

func TestHistoryReaderRefreshDetectsConnectionLostBeforeInstallation(t *testing.T) {
	h := newHarness(t, 0)
	id := h.create(h.input("omp", SideACP)).Info().ID
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	disconnected := acp.NewConn(bytes.NewReader(nil), io.Discard, acp.Options{})
	defer func() { _ = disconnected.Close() }()
	<-disconnected.Done()
	if !a.sync(func() {
		old := a.link
		replacement := *old
		replacement.conn = disconnected
		a.link = nil
		a.opening = true
		// The real disconnect callback may run while the replacement is still
		// outside the actor. It is correctly ignored until installation.
		a.linkLost(&replacement)
		a.finishHistoryReaderRefresh(openResult{link: &replacement}, nil)
		if a.link == &replacement || a.reconnects != 1 || !a.opening {
			t.Errorf("installed dead connection stayed active: link=%v reconnects=%d opening=%v", a.link == &replacement, a.reconnects, a.opening)
		}
	}) {
		t.Fatal("actor stopped")
	}
}
