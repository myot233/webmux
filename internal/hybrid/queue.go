package hybrid

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rainchan/webmux/internal/acp"
)

const (
	// MaxPromptBytes bounds a prompt or queued prompt's text.
	MaxPromptBytes = 64 << 10
	// maxQueueItems bounds the shared prompt queue of one session.
	maxQueueItems = 64
	// Prompt modes: auto sends now or queues, steer injects into the
	// running turn, refuse fails instead of queueing, and now cancels the
	// running turn and sends at once.
	ModeAuto   = "auto"
	ModeSteer  = "steer"
	ModeRefuse = "refuse"
	ModeNow    = "now"
	// MoveUp moves a queued prompt one place towards the head.
	MoveUp = "up"
	// The chat-side card preview: the first lines of the latest agent
	// message, like the terminal preview's rows.
	previewLines     = 3
	previewLineRunes = 120
	previewBufBytes  = 4 << 10
)

// validText checks a prompt's text: at most MaxPromptBytes; blank text is
// allowed only when the prompt has attachments.
func ValidatePromptText(text string, attachments int) error { return validText(text, attachments) }

func validText(text string, attachments int) error {
	switch {
	case strings.TrimSpace(text) == "" && attachments == 0:
		return fmt.Errorf("%w: 消息不能为空", ErrInvalid)
	case len(text) > MaxPromptBytes:
		return fmt.Errorf("%w: 消息超过 %d KiB", ErrInvalid, MaxPromptBytes>>10)
	}
	return nil
}

// validTextLength checks only the byte limit, for edits whose blank check
// needs the queued item's attachments.
func validTextLength(text string) error {
	if len(text) > MaxPromptBytes {
		return fmt.Errorf("%w: 消息超过 %d KiB", ErrInvalid, MaxPromptBytes>>10)
	}
	return nil
}

// FormValue decodes one form answer value: a string, number, boolean or
// array of strings.
func FormValue(raw []byte) (any, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err == nil {
		switch v := v.(type) {
		case string, float64, bool:
			return v, nil
		case []any:
			items := make([]string, len(v))
			ok := true
			for i, item := range v {
				items[i], ok = item.(string)
				if !ok {
					break
				}
			}
			if ok {
				return items, nil
			}
		}
	}
	return nil, fmt.Errorf("%w: 表单内容只能是字符串、数字、布尔值或字符串数组", ErrInvalid)
}

// ready reports why the session cannot take chat commands now. A chat side
// without a connection (reconnecting) is ready: prompts queue until the
// agent is back.
func (a *actor) ready() error {
	if err := a.checkDispatch(false); err != nil {
		return err
	}
	switch {
	case a.state != StateActive || a.closing:
		return fmt.Errorf("%w: 会话已结束", ErrConflict)
	case a.switching != nil:
		return ErrSwitching
	case a.side != SideACP:
		return ErrWrongSide
	}
	return nil
}

// linked is ready for commands that need the agent now.
func (a *actor) linked() error {
	if err := a.ready(); err != nil {
		return err
	}
	if a.link == nil {
		return fmt.Errorf("%w: agent 正在启动，请稍后重试", ErrConflict)
	}
	return nil
}

// setQueue publishes a new queue. Items is never modified in place: the
// published state shares it. An empty queue is never paused.
func (a *actor) setQueue(items []QueueItem, paused bool) {
	if len(items) == 0 {
		items, paused = []QueueItem{}, false
	}
	a.chat.Queue = Queue{Paused: paused, Items: items}
	a.dirty = true
}

// drain sends the queue's head when the session is idle, connected and
// the queue is not paused.
func (a *actor) drain() {
	q := a.chat.Queue
	if a.turn != nil || q.Paused || len(q.Items) == 0 || a.linked() != nil {
		return
	}
	if _, err := a.send(q.Items[0].Text, q.Items[0].Attachments, q.Items[0].ID, ""); err != nil {
		a.emit(Event{Kind: EventNotice, Notice: &Notice{Level: acp.NoticeError, Text: err.Error()}})
		a.setQueue(q.Items, true)
		return
	}
	a.setQueue(slices.Clone(q.Items[1:]), false)
}

// canSendNow reports whether a new prompt would start a turn at once
// instead of queueing.
func (a *actor) canSendNow() bool {
	q := a.chat.Queue
	return a.turn == nil && !q.Paused && len(q.Items) == 0 && a.link != nil
}

// prompt sends text now when the session is idle, connected and the queue
// is empty and running, and otherwise appends it to the queue.
func (a *actor) prompt(text, titleSource string, attachments []Attachment) (PromptResult, error) {
	if err := a.ready(); err != nil {
		return PromptResult{}, err
	}
	a.clearAttention()
	q := a.chat.Queue
	if a.canSendNow() {
		return a.send(text, attachments, "", titleSource)
	}
	if len(q.Items) >= maxQueueItems {
		return PromptResult{}, fmt.Errorf("%w: 队列最多 %d 条", ErrInvalid, maxQueueItems)
	}
	item := a.newItem(text, attachments)
	a.setQueue(append(slices.Clone(q.Items), item), q.Paused)
	a.drain()
	return PromptResult{Queued: true, ItemID: item.ID, Position: a.queuePosition(item.ID)}, nil
}

// promptRefuse sends text when the session is idle and refuses to queue it
// otherwise: a script that must not wait gets ErrBusy.
func (a *actor) promptRefuse(text, titleSource string, attachments []Attachment) (PromptResult, error) {
	if err := a.ready(); err != nil {
		return PromptResult{}, err
	}
	a.clearAttention()
	if !a.canSendNow() {
		return PromptResult{}, fmt.Errorf("%w: 消息未发送", ErrBusy)
	}
	return a.send(text, attachments, "", titleSource)
}

// promptNow sends text at once: when no turn runs it starts a turn now,
// otherwise the running turn is cancelled first and the prompt sent next,
// like a queued prompt's send-now.
func (a *actor) promptNow(text, titleSource string, attachments []Attachment) (PromptResult, error) {
	if err := a.ready(); err != nil {
		return PromptResult{}, err
	}
	a.clearAttention()
	if a.turn == nil && a.link == nil {
		return PromptResult{}, fmt.Errorf("%w: agent 尚未连接，消息未发送", ErrBusy)
	}
	if a.turn != nil && len(a.chat.Queue.Items) >= maxQueueItems {
		return PromptResult{}, fmt.Errorf("%w: 队列最多 %d 条", ErrInvalid, maxQueueItems)
	}
	return a.sendItemNow(a.newItem(text, attachments), titleSource)
}

// newItem makes the next queued prompt.
func (a *actor) newItem(text string, attachments []Attachment) QueueItem {
	a.nextItem++
	return QueueItem{ID: "q" + strconv.Itoa(a.nextItem), Text: text, CreatedAt: time.Now().UTC(), Attachments: attachments}
}

// queuePosition is item id's 1-based place in the queue, or 0 once it is
// no longer queued.
func (a *actor) queuePosition(id string) int {
	if i := slices.IndexFunc(a.chat.Queue.Items, func(it QueueItem) bool { return it.ID == id }); i >= 0 {
		return i + 1
	}
	return 0
}

// queued finds a queued prompt; ErrSent if it was sent or removed.
func (a *actor) queued(id string) (int, error) {
	if err := a.ready(); err != nil {
		return 0, err
	}
	if i := slices.IndexFunc(a.chat.Queue.Items, func(it QueueItem) bool { return it.ID == id }); i >= 0 {
		return i, nil
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(id, "q")); err == nil && strings.HasPrefix(id, "q") && n > 0 && n <= a.nextItem {
		return 0, ErrSent
	}
	return 0, fmt.Errorf("%w: 没有该排队消息", ErrNotFound)
}

// editQueued replaces a queued prompt's text and/or moves it up one place.
func (a *actor) editQueued(id string, text *string, move string) error {
	i, err := a.queued(id)
	if err != nil {
		return err
	}
	items := slices.Clone(a.chat.Queue.Items)
	if text != nil {
		if strings.TrimSpace(*text) == "" && len(items[i].Attachments) == 0 {
			return fmt.Errorf("%w: 消息不能为空", ErrInvalid)
		}
		items[i].Text = *text
	}
	if move == MoveUp && i > 0 {
		items[i-1], items[i] = items[i], items[i-1]
	}
	a.setQueue(items, a.chat.Queue.Paused)
	return nil
}

func (a *actor) removeQueued(id string) error {
	i, err := a.queued(id)
	if err != nil {
		return err
	}
	a.setQueue(slices.Delete(slices.Clone(a.chat.Queue.Items), i, i+1), a.chat.Queue.Paused)
	return nil
}

// sendQueuedNow moves a queued prompt to the head and sends it: at once
// when idle, else after cancelling the running turn.
func (a *actor) sendQueuedNow(id string) error {
	i, err := a.queued(id)
	if err != nil {
		return err
	}
	items := slices.Clone(a.chat.Queue.Items)
	item := items[i]
	a.setQueue(slices.Delete(items, i, i+1), false)
	_, err = a.sendItemNow(item, "")
	return err
}

// sendItemNow sends item at once, ahead of anything already queued: when
// no turn runs it starts a turn now, otherwise the item goes to the head
// and the running turn is cancelled so the queue sends it next.
func (a *actor) sendItemNow(item QueueItem, titleSource string) (PromptResult, error) {
	if a.turn == nil {
		a.setQueue(a.chat.Queue.Items, false)
		return a.send(item.Text, item.Attachments, "", titleSource)
	}
	a.setQueue(append([]QueueItem{item}, a.chat.Queue.Items...), false)
	a.sendAfterCancel = true
	a.cancelTurn()
	return PromptResult{Queued: true, ItemID: item.ID, Position: 1}, nil
}

// resumeQueue unpauses the queue and sends its head when idle.
func (a *actor) resumeQueue() error {
	if err := a.ready(); err != nil {
		return err
	}
	a.setQueue(a.chat.Queue.Items, false)
	a.drain()
	return nil
}

func (a *actor) clearQueue() error {
	if err := a.ready(); err != nil {
		return err
	}
	a.setQueue(nil, false)
	return nil
}

// takeQueue empties the queue and returns its texts joined by blank lines,
// in order: the draft a switch to the terminal returns to the device that
// asked for it.
func (a *actor) takeQueue() string {
	texts := make([]string, len(a.chat.Queue.Items))
	for i, it := range a.chat.Queue.Items {
		text := it.Text
		if len(it.Attachments) > 0 {
			paths := make([]string, len(it.Attachments))
			for j, at := range it.Attachments {
				paths[j] = at.Path
			}
			if text != "" {
				text += "\n\n"
			}
			text += "附件路径：\n" + strings.Join(paths, "\n")
		}
		texts[i] = text
	}
	a.setQueue(nil, false)
	return strings.Join(texts, "\n\n")
}

// cancel stops the running turn; the queue pauses when the turn ends.
func (a *actor) cancel() error {
	if err := a.ready(); err != nil {
		return err
	}
	a.cancelTurn()
	return nil
}
