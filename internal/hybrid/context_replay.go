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
		if u, ok := n.Update.(*acp.UserMessageChunk); ok && u.Content.Type == acp.ContentText {
			b := &a.replayUser
			if b.buffered && b.messageID != u.MessageID {
				a.flushReplayUser()
			}
			if b.passthrough && b.messageID == u.MessageID {
				a.onUpdateDirect(epoch, n)
				return
			}
			if b.messageID != u.MessageID {
				b.passthrough = false
			}
			b.messageID, b.epoch = u.MessageID, epoch
			if b.text.Len()+len(u.Content.Text) > MaxPromptBytes {
				a.flushReplayUser()
				b.passthrough = true
				b.messageID = u.MessageID
				a.onUpdateDirect(epoch, n)
				return
			}
			b.buffered = true
			b.text.WriteString(u.Content.Text)
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
	var attachments []Attachment
	if a.svc.conversations != nil {
		normalized, matched, err := a.svc.conversations.NormalizeInlineUser(context.Background(), a.activity.thread, a.agentSessionID, text, b.messageID)
		if err != nil {
			a.activityFailure()
		} else if matched {
			text = normalized.Text
			for _, at := range normalized.Attachments {
				attachments = append(attachments, Attachment{Path: at.Reference, Name: at.Name, MimeType: at.MimeType, Size: at.Size, Kind: at.Kind})
			}
			a.replayOwnedMessageID, a.replayOwnedMessage = b.messageID, true
		}
	}
	n := acp.SessionNotification{SessionID: a.agentSessionID, Update: &acp.UserMessageChunk{Content: acp.TextBlock(text), MessageID: b.messageID}}
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
