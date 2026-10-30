package hybrid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rainchan/webmux/internal/acp"
)

const (
	// MaxPromptBytes bounds a prompt or queued prompt's text.
	MaxPromptBytes = 64 << 10
	// maxQueueItems bounds the shared prompt queue of one session.
	maxQueueItems = 64
	// methodSteering is omp's steering extension: it injects a user
	// message into the running turn.
	methodSteering = "_session/steering"
	// Prompt modes.
	ModeAuto  = "auto"
	ModeSteer = "steer"
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
// without a connection (dormant, waking or reconnecting) is ready: prompts
// queue until the agent is back.
func (a *actor) ready() error {
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
	if _, err := a.send(q.Items[0].Text, q.Items[0].Attachments); err != nil {
		a.emit(Event{Kind: EventNotice, Notice: &Notice{Level: acp.NoticeError, Text: err.Error()}})
		a.setQueue(q.Items, true)
		return
	}
	a.setQueue(slices.Clone(q.Items[1:]), false)
}

// prompt sends text now when the session is idle, connected and the queue
// is empty and running, and otherwise appends it to the queue.
func (a *actor) prompt(text string, attachments []Attachment) (PromptResult, error) {
	if err := a.ready(); err != nil {
		return PromptResult{}, err
	}
	a.clearAttention()
	q := a.chat.Queue
	if a.turn == nil && !q.Paused && len(q.Items) == 0 && a.link != nil {
		return a.send(text, attachments)
	}
	if len(q.Items) >= maxQueueItems {
		return PromptResult{}, fmt.Errorf("%w: 队列最多 %d 条", ErrInvalid, maxQueueItems)
	}
	a.nextItem++
	item := QueueItem{ID: "q" + strconv.Itoa(a.nextItem), Text: text, CreatedAt: time.Now().UTC(), Attachments: attachments}
	a.setQueue(append(slices.Clone(q.Items), item), q.Paused)
	a.drain()
	return PromptResult{Queued: true, ItemID: item.ID}, nil
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
	items = slices.Insert(slices.Delete(items, i, i+1), 0, item)
	a.setQueue(items, false)
	if a.turn != nil {
		a.sendAfterCancel = true
		a.cancelTurn()
		return nil
	}
	a.drain()
	return nil
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

// agentCall is a command's ACP call, run outside the actor.
type agentCall func(ctx context.Context) error

// steer prepares injecting text into the running turn. It needs a profile
// with the steering extension, an agent that declared it, and a turn.
func (a *actor) steer(text string, attachments []Attachment) (agentCall, error) {
	if err := a.linked(); err != nil {
		return nil, err
	}
	if !a.chat.Steering || a.turn == nil {
		return nil, ErrSteeringUnavailable
	}
	blocks, notes, err := promptBlocks(text, attachments, a.chat.PromptCapabilities, a.svc.uploadCheck(a.id))
	if err != nil {
		return nil, err
	}
	a.clearAttention()
	conn, sid, epoch := a.link.conn, a.link.sessionID, a.epoch
	return func(ctx context.Context) error {
		err := conn.CallExtension(ctx, methodSteering, acp.PromptRequest{SessionID: sid, Prompt: blocks}, nil)
		if err != nil {
			return a.agentCallFailed(methodSteering, err)
		}
		_, err = call(ctx, a, func() (struct{}, error) {
			if a.epoch == epoch && !a.stopped {
				a.emit(a.tr.userMessage(text, attachments))
				a.tr.breakMessage()
				for _, note := range notes {
					a.emit(Event{Kind: EventNotice, Notice: &Notice{Level: acp.NoticeWarning, Text: note}})
				}
			}
			return struct{}{}, nil
		})
		return err
	}, nil
}

// setConfig prepares session/set_config_option for one option: Value for
// a select, Enabled for a boolean. The agent's answer replaces the
// options, and the record keeps the values for later loads.
func (a *actor) setConfig(id string, value *string, enabled *bool) (agentCall, error) {
	if err := a.linked(); err != nil {
		return nil, err
	}
	i := slices.IndexFunc(a.chat.Config, func(o ConfigOption) bool { return o.ID == id })
	if i < 0 {
		return nil, fmt.Errorf("%w: 未知的配置项", ErrInvalid)
	}
	option := a.chat.Config[i]
	var v acp.ConfigValue
	switch {
	case option.Type == acp.ConfigBoolean && enabled != nil && value == nil:
		b := *enabled
		v.Bool = &b
	case option.Type == acp.ConfigSelect && value != nil && enabled == nil:
		if !slices.ContainsFunc(option.Options, func(c ConfigChoice) bool { return c.Value == *value }) {
			return nil, fmt.Errorf("%w: 未知的配置值", ErrInvalid)
		}
		v.ID = *value
	default:
		return nil, fmt.Errorf("%w: 选择类配置项需要 value，布尔类需要 enabled", ErrInvalid)
	}
	conn, sid, epoch := a.link.conn, a.link.sessionID, a.epoch
	return func(ctx context.Context) error {
		resp, err := conn.SetConfigOption(ctx, acp.SetConfigOptionRequest{SessionID: sid, ConfigID: id, Value: v})
		if err != nil {
			return a.agentCallFailed(acp.MethodSessionSetConfigOption, err)
		}
		_, err = call(ctx, a, func() (struct{}, error) {
			if a.epoch == epoch && !a.stopped {
				a.configApplied(id, v, resp.ConfigOptions)
			}
			return struct{}{}, nil
		})
		return err
	}, nil
}

// configApplied publishes the options after a change: the agent's complete
// set, or the previous set with the new value when it sent none.
func (a *actor) configApplied(id string, v acp.ConfigValue, options []acp.ConfigOption) {
	if options != nil {
		a.emit(Event{Kind: EventConfig, Config: configOptions(options)})
		return
	}
	config := slices.Clone(a.chat.Config)
	for i := range config {
		if config[i].ID != id {
			continue
		}
		if v.Bool != nil {
			b := *v.Bool
			config[i].Enabled = &b
		} else {
			config[i].Value = v.ID
		}
	}
	a.emit(Event{Kind: EventConfig, Config: config})
}

// agentCallFailed logs a failed agent command by method and code only and
// describes it for the requesting user.
func (a *actor) agentCallFailed(method string, err error) error {
	var rpc *acp.RPCError
	code := 0
	detail := "与 agent 的连接已断开"
	switch {
	case errors.As(err, &rpc):
		code = rpc.Code
		detail = limitText(rpc.Message, errorTextBytes)
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		detail = "agent 未及时答复"
	}
	a.svc.log.Warn("hybrid agent call failed", "id", a.id, "method", method, "code", code)
	return fmt.Errorf("%w: %s: %s", ErrAgentCall, method, detail)
}

// preview keeps the first lines of the latest agent message for the
// session card.
type preview struct {
	messageID string
	buf       []byte
	full      bool
	text      string
}

// add takes an agent message chunk and reports whether the preview text
// changed. A new message ID starts over.
func (p *preview) add(m *MessageChunk) bool {
	if m.MessageID != p.messageID {
		p.messageID, p.buf, p.full = m.MessageID, p.buf[:0], false
	}
	if p.full {
		return false
	}
	room := previewBufBytes - len(p.buf)
	chunk := m.Text
	if len(chunk) >= room {
		chunk, p.full = chunk[:room], true
	}
	p.buf = append(p.buf, chunk...)
	text, complete := previewText(p.buf)
	p.full = p.full || complete
	if text == p.text {
		return false
	}
	p.text = text
	return true
}

// previewText is the first previewLines non-blank lines of b, each cut to
// previewLineRunes runes; complete once the last of them has ended.
func previewText(b []byte) (string, bool) {
	var lines []string
	rest := string(b)
	for len(lines) < previewLines && rest != "" {
		line, after, ended := strings.Cut(rest, "\n")
		rest = after
		line = strings.TrimRight(strings.ToValidUTF8(line, ""), " \t\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if utf8.RuneCountInString(line) > previewLineRunes {
			line = string([]rune(line)[:previewLineRunes])
		}
		lines = append(lines, line)
		if len(lines) == previewLines {
			return strings.Join(lines, "\n"), ended
		}
	}
	return strings.Join(lines, "\n"), false
}
