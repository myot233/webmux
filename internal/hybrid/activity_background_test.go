package hybrid

import (
	"context"
	"testing"

	"github.com/rainchan/webmux/internal/agentprofile/agy"
	"github.com/rainchan/webmux/internal/attentionkind"
)

func TestActivityRecordsBackgroundStatusRefresh(t *testing.T) {
	h, db := historyHarness(t)
	h.fast()
	shell := h.create(h.input(agy.Name, SideACP))
	id := shell.Info().ID
	prompt(t, h, id, "hi /background")
	waitFor(t, "background status response committed", func() bool {
		attention := shell.Info().Attention
		return attention != nil && attention.Kind == attentionkind.Done
	})
	binding, err := db.Binding(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	page, err := db.Activities(context.Background(), binding.ConversationID, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	user, agent := false, false
	for _, item := range page.Items {
		a := item.Activity
		if a.RunOrdinal != 2 || a.Kind != "message" {
			continue
		}
		if a.RunID == "" || a.Status != "completed" {
			t.Fatalf("uncommitted refresh: %+v", a)
		}
		if a.Role == "user" && a.Text == agy.BackgroundStatusPrompt {
			user = true
		}
		if a.Role == "agent" && a.Text != "" {
			agent = true
		}
	}
	if !user || !agent {
		t.Fatalf("refresh history lost: user=%v agent=%v", user, agent)
	}
}
