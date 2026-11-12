package hybrid

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/rainchan/webmux/internal/acp"
	"strconv"
	"time"
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
	binding, err := s.conversations.Binding(ctx, id)
	if err != nil || binding.ProviderThreadID != e.TargetProviderThreadID {
		return PromptResult{}, ErrConflict
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

func (a *actor) sendContext(text, wireText string, attachments []Attachment, queueItem, titleSource, executionID string, check func(string) (string, error), refresh ...bool) (PromptResult, error) {
	if err := a.checkDispatch(false); err != nil {
		return PromptResult{}, err
	}
	backgroundRefresh := len(refresh) > 0 && refresh[0]
	blocks, notes, err := promptBlocks(wireText, attachments, a.chat.PromptCapabilities, check)
	if err != nil {
		return PromptResult{}, err
	}
	if executionID != "" {
		if a.svc.conversations == nil {
			return PromptResult{}, ErrUnavailable
		}
		base := sha256.Sum256([]byte(wireText))
		actualText := ""
		for _, block := range blocks {
			if block.Type == acp.ContentText {
				actualText += block.Text
			}
		}
		actual := sha256.Sum256([]byte(actualText))
		if len(actualText) > MaxPromptBytes {
			return PromptResult{}, ErrTooLarge
		}
		if err := a.svc.conversations.ClaimExecutionText(context.Background(), executionID, hex.EncodeToString(base[:]), hex.EncodeToString(actual[:])); err != nil {
			return PromptResult{}, err
		}
	}
	promptID := executionID
	if promptID == "" {
		promptID = rand.Text()
	}
	if err := a.beginActivityRunID(text, attachments, promptID); err != nil {
		return PromptResult{}, err
	}
	a.nextTurn++
	if a.promptTitle {
		a.promptTitle = false
		if titleSource == "" {
			titleSource = text
		}
		if title := PromptTitle(titleSource); title != "" && a.title == "" {
			a.title, a.titleSession = title, a.agentSessionID
		}
	}
	n := strconv.Itoa(a.nextTurn)
	result := PromptResult{TurnID: "t" + n, PromptID: "p" + n}
	ctx, cancel := context.WithCancel(context.Background())
	t := &turn{id: result.TurnID, cancel: cancel, done: make(chan struct{}), executionID: executionID, backgroundRefresh: backgroundRefresh}
	a.turn = t
	a.snapshotProcesses()
	a.work.endTurn()
	startedAt := time.Now().UTC()
	a.chat.Turn = Turn{Running: true, TurnID: t.id, StartedAt: &startedAt}
	a.dirty = true
	a.clearAttention()
	a.emit(Event{Kind: EventTurnStart, TurnStart: &TurnStart{TurnID: result.TurnID, PromptID: result.PromptID, QueueItemID: queueItem}}, a.tr.userMessage(text, attachments))
	a.tr.breakMessage()
	for _, note := range notes {
		a.emit(Event{Kind: EventNotice, Notice: &Notice{Level: acp.NoticeWarning, Text: note}})
	}
	conn, sid := a.link.conn, a.link.sessionID
	go func() {
		defer close(t.done)
		resp, err := conn.Prompt(ctx, acp.PromptRequest{SessionID: sid, Prompt: blocks})
		a.post(func() { a.turnEnded(t, resp, err) })
	}()
	return result, nil
}
