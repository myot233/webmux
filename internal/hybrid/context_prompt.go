package hybrid

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// ContextPrompt is an internal command with separate visible and provider text.
// The coordinator already persisted the sending intent; this command cannot
// queue, steer, title a chat from historical text, or accept arbitrary file roots.
type ContextPrompt struct {
	ExecutionID, Text, WireText string
	Attachments                 []Attachment
	CheckAttachment             func(string) (string, error)
}

func (s *Service) PromptContext(ctx context.Context, id string, in ContextPrompt) (PromptResult, error) {
	if s.conversations == nil || in.CheckAttachment == nil {
		return PromptResult{}, ErrUnavailable
	}
	e, err := s.conversations.Execution(ctx, in.ExecutionID)
	if err != nil {
		return PromptResult{}, err
	}
	digest := sha256.Sum256([]byte(in.WireText))
	if e.RuntimeID != id || e.Phase != "sending" || e.Status != "pending" || e.PromptHash != hex.EncodeToString(digest[:]) {
		return PromptResult{}, ErrConflict
	}
	if err := validText(in.Text, len(in.Attachments)); err != nil {
		return PromptResult{}, err
	}
	if err := validText(in.WireText, len(in.Attachments)); err != nil {
		return PromptResult{}, err
	}
	attachments, err := resolveAttachments(in.CheckAttachment, in.Attachments)
	if err != nil {
		return PromptResult{}, err
	}
	for i, actual := range attachments {
		if actual.Path != in.Attachments[i].Path || actual.Size != in.Attachments[i].Size || actual.Kind != in.Attachments[i].Kind || actual.MimeType != in.Attachments[i].MimeType {
			return PromptResult{}, ErrConflict
		}
	}
	for i := range attachments {
		attachments[i].Name = in.Attachments[i].Name
	}
	a, err := s.actor(id)
	if err != nil {
		return PromptResult{}, err
	}
	return call(ctx, a, func() (PromptResult, error) {
		if err := a.ready(); err != nil {
			return PromptResult{}, err
		}
		if !a.canSendNow() {
			return PromptResult{}, fmt.Errorf("%w: 消息未发送", ErrBusy)
		}
		if a.agentSessionID != e.NativeThreadID {
			return PromptResult{}, ErrConflict
		}
		return a.sendContext(in.Text, in.WireText, attachments, "", in.Text, in.ExecutionID, in.CheckAttachment)
	})
}

func (a *actor) finishContextExecution(t *turn, status, class string) {
	if t.executionID == "" || a.svc.conversations == nil {
		return
	}
	if err := a.svc.conversations.FinishExecution(context.Background(), t.executionID, status, class); err != nil {
		a.svc.log.Warn("handoff outcome persistence failed", "id", a.id)
	}
}
