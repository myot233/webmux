package hybrid

import (
	"context"
	"errors"
	"fmt"
	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/conversation"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// agentCall is a command's ACP call, run outside the actor.
type agentCall func(ctx context.Context) error

// Steering outcomes of an agent whose steering requests carry _meta (it
// was asked to hand idle prompts back); other agents answer nothing.
const (
	steerInjected       = "injected"
	steerPromptRequired = "promptRequired"
	steerFailed         = "failed"
	steerStartedNewTurn = "startedNewTurn"
)

// steer prepares injecting text into the running turn. It needs an agent
// adapter with a steering extension (agentprofile.Steerer), an agent that declared it, and a turn. A
// turn that ended before the text got in (promptRequired) puts the text at
// the head of the queue.
func (a *actor) steer(text string, attachments []Attachment) (agentCall, error) {
	if err := a.linked(); err != nil {
		return nil, err
	}
	steerer, ok := a.cmd.Agent.(agentprofile.Steerer)
	if !ok || !a.chat.Steering || a.turn == nil {
		return nil, ErrSteeringUnavailable
	}
	blocks, notes, err := promptBlocks(text, attachments, a.chat.PromptCapabilities, a.svc.uploadCheck(a.id))
	if err != nil {
		return nil, err
	}

	key, run, thread := a.activityKey("steer"), a.activity.run, a.activity.thread
	if a.svc.conversations != nil {
		if err := a.writeActivity(conversation.ActivityInput{LocalKey: key, RunID: run, Kind: "message", Role: "user", Status: "pending", Text: text, Attachments: activityAttachments(attachments)}); err != nil {
			return nil, err
		}
		thread = a.activity.thread
	}
	a.clearAttention()
	conn, sid, epoch := a.link.conn, a.link.sessionID, a.epoch
	req := acp.PromptRequest{SessionID: sid, Prompt: blocks}
	var resp struct {
		Outcome string `json:"outcome"`
	}
	// Only agents asked for promptRequired promise an outcome; others may
	// answer anything.
	var result any
	if meta := steerer.SteeringMeta(); meta != nil {
		req.Meta = meta
		result = &resp
	}
	method := steerer.SteeringMethod()
	return func(ctx context.Context) error {
		err := conn.CallExtension(ctx, method, req, result)
		if err != nil && a.svc.conversations != nil {
			if gapErr := a.svc.conversations.AddGap(context.Background(), thread, "unconfirmed_prompt"); gapErr != nil {
				a.svc.log.Warn("steering uncertainty could not be persisted", "id", a.id)
			}
		}
		if err != nil {
			return a.agentCallFailed(method, err)
		}
		if resp.Outcome == steerFailed {
			if a.svc.conversations != nil {
				if err := a.svc.conversations.DiscardDraftActivity(context.Background(), thread, key); err != nil {
					a.svc.log.Warn("failed steering draft cleanup failed", "id", a.id)
				}
			}
			a.svc.log.Warn("hybrid steering failed", "id", a.id, "method", method, "outcome", resp.Outcome)
			return fmt.Errorf("%w: agent 未能插话", ErrAgentCall)
		}
		_, err = call(ctx, a, func() (struct{}, error) {
			if a.epoch != epoch || a.stopped {
				return struct{}{}, nil
			}
			switch resp.Outcome {
			case steerPromptRequired:
				if a.svc.conversations != nil {
					if err := a.svc.conversations.DiscardDraftActivity(context.Background(), thread, key); err != nil {
						a.activityFailure()
					}
				}
				a.requeue(text, attachments)
				return struct{}{}, nil
			case steerStartedNewTurn:
				a.svc.log.Warn("hybrid steering started a turn", "id", a.id, "method", method, "outcome", resp.Outcome)
				a.emit(Event{Kind: EventNotice, Notice: &Notice{Level: acp.NoticeError, Text: "agent 自行开始了一个回合，该回合不受 WebMux 跟踪"}})
			}
			if err := a.writeActivity(conversation.ActivityInput{LocalKey: key, RunID: run, Kind: "message", Role: "user", Status: "completed"}); err != nil {
				a.activityFailure()
			}
			a.activity.message.open = false
			a.emit(a.tr.userMessage(text, attachments))
			a.tr.breakMessage()
			for _, note := range notes {
				a.emit(Event{Kind: EventNotice, Notice: &Notice{Level: acp.NoticeWarning, Text: note}})
			}
			return struct{}{}, nil
		})
		return err
	}, nil
}

// requeue puts a prompt that missed its turn at the head of the queue, to
// go out as the next turn.
func (a *actor) requeue(text string, attachments []Attachment) {
	a.nextItem++
	item := QueueItem{ID: "q" + strconv.Itoa(a.nextItem), Text: text, CreatedAt: time.Now().UTC(), Attachments: attachments}
	a.setQueue(slices.Insert(slices.Clone(a.chat.Queue.Items), 0, item), a.chat.Queue.Paused)
	a.drain()
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
		var options []acp.ConfigOption
		if err != nil {
			if !agentprofile.ConfigErrorHandled(a.cmd.Agent, err, id, v.ID) {
				return a.agentCallFailed(acp.MethodSessionSetConfigOption, err)
			}
		} else {
			options = resp.ConfigOptions
		}
		_, err = call(ctx, a, func() (struct{}, error) {
			if a.epoch == epoch && !a.stopped {
				a.configApplied(id, v, options)
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
		a.emit(Event{Kind: EventConfig, Config: configOptions(options, a.cmd.Profile.ModelChoiceValues)})
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
