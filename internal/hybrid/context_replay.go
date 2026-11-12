package hybrid

import (
	"context"
	"strings"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/conversation"
)

type replayUserBuffer struct {
	text                  strings.Builder
	messageID             string
	role                  string
	buffered, passthrough bool
	epoch                 int
}

// onUpdate holds at most one prompt-sized replayed user message. Verification
// happens before either durable capture or the bounded UI translator sees it.
func (a *actor) onUpdate(epoch int, n acp.SessionNotification) {
	if epoch != a.epoch || a.stopped {
		return
	}
	if a.activity.replay {
		role, id, text, ok := replayTextChunk(n.Update)
		if ok && (role == "user" || a.replayNative.enabled) {
			b := &a.replayUser
			if b.buffered && (b.messageID != id || b.role != role) {
				a.flushReplayUser()
			}
			if b.passthrough && b.messageID == id && b.role == role {
				a.onUpdateDirect(epoch, n)
				return
			}
			if b.messageID != id || b.role != role {
				b.passthrough = false
			}
			b.messageID, b.role, b.epoch = id, role, epoch
			if b.text.Len()+len(text) > MaxPromptBytes {
				a.flushReplayUser()
				b.passthrough = true
				b.messageID, b.role = id, role
				a.onUpdateDirect(epoch, n)
				return
			}
			b.buffered = true
			b.text.WriteString(text)
			return
		}
		// Session metadata can be interleaved between chunks of one message.
		// It is not evidence that the buffered replay message has ended.
		switch n.Update.(type) {
		case *acp.UsageUpdate, *acp.ConfigOptionUpdate, *acp.CurrentModeUpdate,
			*acp.AvailableCommandsUpdate, *acp.SessionInfoUpdate:
			a.onUpdateDirect(epoch, n)
			return
		}
	}
	a.flushReplayUser()
	if u, ok := n.Update.(*acp.UserMessageChunk); ok && a.activity.replay && a.replayOwnedMessage && u.MessageID == a.replayOwnedMessageID {
		return
	}
	a.replayOwnedMessage = false
	a.replayUser.passthrough = false
	a.onUpdateDirect(epoch, n)
}

func (a *actor) flushReplayUser() {
	b := &a.replayUser
	if !b.buffered {
		return
	}
	text := b.text.String()
	if a.filterNativeReplay(b.role, text, b.messageID) {
		*b = replayUserBuffer{}
		return
	}
	var attachments []Attachment
	if a.svc.conversations != nil && b.role == "user" {
		normalized, matched, err := a.svc.conversations.NormalizeInlineUser(context.Background(), a.activity.thread, a.agentSessionID, text, b.messageID)
		if err != nil {
			a.activityFailure()
		} else if matched && b.messageID == "" && a.activity.replayReceipts[normalized.ExecutionID] {
			// The first exact packet proves this receipt. An identical later
			// no-ID user message can be a legitimate quote, so keep it intact.
			if err := a.svc.conversations.AddGap(context.Background(), a.activity.thread, "replay_ambiguous"); err != nil {
				a.activityFailure()
			}
		} else if matched {
			if a.activity.replayReceipts == nil {
				a.activity.replayReceipts = map[string]bool{}
			}
			a.activity.replayReceipts[normalized.ExecutionID] = true
			text = normalized.Text
			for _, at := range normalized.Attachments {
				attachments = append(attachments, Attachment{Path: at.Reference, Name: at.Name, MimeType: at.MimeType, Size: at.Size, Kind: at.Kind})
			}
			a.replayOwnedMessageID, a.replayOwnedMessage = b.messageID, true
		}
	}
	n := acp.SessionNotification{SessionID: a.agentSessionID, Update: &acp.UserMessageChunk{Content: acp.TextBlock(text), MessageID: b.messageID}}
	if b.role == "assistant" {
		n.Update = &acp.AgentMessageChunk{Content: acp.TextBlock(text), MessageID: b.messageID}
	}
	epoch := b.epoch
	*b = replayUserBuffer{}
	a.onUpdateDirect(epoch, n)
	if len(attachments) > 0 {
		if err := a.writeActivity(conversation.ActivityInput{LocalKey: a.activity.message.key, ProviderItemID: a.activity.message.provider, Kind: "message", Role: "user", Status: "in_progress", Attachments: activityAttachments(attachments)}); err != nil {
			a.activityFailure()
		}
		a.emit(Event{Kind: EventMessage, Message: &MessageChunk{MessageID: a.tr.last.id, Role: RoleUser, Attachments: attachments}})
	}
}

func replayTextChunk(update acp.Update) (role, id, text string, ok bool) {
	var content acp.ContentBlock
	switch u := update.(type) {
	case *acp.UserMessageChunk:
		role, id, content = "user", u.MessageID, u.Content
	case *acp.AgentMessageChunk:
		role, id, content = "assistant", u.MessageID, u.Content
	default:
		return "", "", "", false
	}
	return role, id, content.Text, content.Type == acp.ContentText
}
