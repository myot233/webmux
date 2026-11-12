package hybrid

import (
	"testing"

	"github.com/rainchan/webmux/internal/acp"
)

func TestReplayUserChunksSurviveSessionMetadata(t *testing.T) {
	h := newHarness(t, 0)
	id := h.create(h.input("omp", SideACP)).Info().ID
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, update := range []acp.Update{
		&acp.UsageUpdate{Used: 1, Size: 100},
		&acp.ConfigOptionUpdate{},
		&acp.CurrentModeUpdate{},
		&acp.AvailableCommandsUpdate{},
		&acp.SessionInfoUpdate{},
	} {
		if !a.sync(func() {
			a.activity.replay = true
			a.onUpdate(a.epoch, acp.SessionNotification{Update: &acp.UserMessageChunk{MessageID: "one", Content: acp.TextBlock("full historical ")}})
			a.onUpdate(a.epoch, acp.SessionNotification{Update: update})
			a.onUpdate(a.epoch, acp.SessionNotification{Update: &acp.UserMessageChunk{MessageID: "one", Content: acp.TextBlock("envelope")}})
			if !a.replayUser.buffered || a.replayUser.text.String() != "full historical envelope" {
				t.Errorf("%T broke the replay message: %q", update, a.replayUser.text.String())
			}
			a.flushReplayUser()
			if a.replayUser.buffered {
				t.Error("replay completion did not flush")
			}
			a.tr.breakMessage()
		}) {
			t.Fatal("actor stopped")
		}
	}
}
