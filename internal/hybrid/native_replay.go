package hybrid

import (
	"context"

	"github.com/rainchan/webmux/internal/conversation"
)

type nativeReplay struct {
	enabled bool
	current conversation.Injection
	seen    []conversation.InjectionItem
}

func (a *actor) startNativeReplay() {
	a.replayNative = nativeReplay{}
	enabled, err := a.svc.conversations.HasInjections(context.Background(), a.activity.thread, a.agentSessionID)
	if err != nil {
		a.activityFailure()
		return
	}
	a.replayNative.enabled = enabled
}

func (a *actor) nativeReplayGap() {
	if err := a.svc.conversations.AddGap(context.Background(), a.activity.thread, "replay_ambiguous"); err != nil {
		a.activityFailure()
	}
	a.replayNative.current = conversation.Injection{}
	a.replayNative.seen = nil
}

// Native imports never become new user work in the Activity Store or chat log.
// Stable IDs or an exact ordered sequence scoped to this native thread identify
// them. A partial match cannot acknowledge an unknown injection.
func (a *actor) filterNativeReplay(role, text, providerID string) bool {
	r := &a.replayNative
	if !r.enabled || a.svc.conversations == nil {
		return false
	}
	item := conversation.InjectionItem{Role: role, TextHash: conversation.TextHash(text), ProviderItemID: providerID}
	if r.current.DeliveryID != "" {
		want := r.current.Items[len(r.seen)]
		if want.Role != role || want.TextHash != item.TextHash || (want.ProviderItemID != "" && want.ProviderItemID != providerID) {
			a.nativeReplayGap()
		}
	}
	if r.current.DeliveryID == "" {
		injection, known, err := a.svc.conversations.InjectionReplay(context.Background(), a.activity.thread, a.agentSessionID, role, item.TextHash, providerID)
		if err != nil {
			a.activityFailure()
			return false
		}
		if injection.DeliveryID == "" {
			return known
		}
		r.current = injection
	}
	r.seen = append(r.seen, item)
	if len(r.seen) == len(r.current.Items) {
		if err := a.svc.conversations.ReconcileInjection(context.Background(), r.current.DeliveryID, a.agentSessionID, r.seen); err != nil {
			a.nativeReplayGap()
			return true
		}
		r.current, r.seen = conversation.Injection{}, nil
	}
	return true
}
