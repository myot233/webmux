package clicmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/coder/websocket"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

// The chat commands act on what only a session's chat view has: requests,
// agent settings, the permission mode and the queue. Input, interrupt,
// view changes, listing, resume and forget are session commands. Every
// write carries a fresh clientRequestId, so the server applies it once.

var chatSessionArgument = Item{Name: "SESSION", Text: `A session with a chat view, or "self" for the current session.`}

func chatExits(extra ...ExitItem) []ExitItem {
	return exitsCommon(append([]ExitItem{{Code: 1, Text: "The session has no chat view, is on its terminal view, or the server refused the request."}}, extra...)...)
}

// hasChatView reports whether s has a chat view besides its terminal.
func hasChatView(s contract.SessionInfo) bool {
	return slices.Contains(s.Views, contract.ViewChat)
}

// onChat reports whether s settled on its chat view.
func onChat(s contract.SessionInfo) bool {
	return s.View == contract.ViewChat && s.Transition == nil
}

// chatSession reads session arg and checks that it has a chat view, and
// is on it when chat is set.
func (e Env) chatSession(ctx context.Context, client *localclient.Client, arg string, chat bool) (contract.SessionInfo, int, bool) {
	id, code, ok := e.SessionID(arg)
	if !ok {
		return contract.SessionInfo{}, code, false
	}
	var info contract.SessionInfo
	if err := client.Do(ctx, http.MethodGet, "/api/sessions/"+url.PathEscape(id), nil, &info); err != nil {
		return info, e.sessionFail(err), false
	}
	if !hasChatView(info) {
		_, _ = fmt.Fprintf(e.Stderr, "webmux: session %s has no chat view (no_chat_view); only a launch config with the chat view starts one\n", id)
		return info, contract.ExitError, false
	}
	if chat && !onChat(info) {
		_, _ = fmt.Fprintf(e.Stderr, "webmux: session %s is not on its chat view; run webmux view %s chat first\n", id, id)
		return info, contract.ExitError, false
	}
	return info, contract.ExitOK, true
}

// sessionFail prints a failed session, view, input or chat command with
// its conflict reason and candidates; it exits 1, or 3 when the server is
// not running.
func (e Env) sessionFail(err error) int {
	conflict, ok := sessionConflict(err)
	if !ok {
		return e.Fail(err)
	}
	_, _ = fmt.Fprintf(e.Stderr, "webmux: %s (%s)\n", conflict.Error, conflict.Reason)
	switch conflict.Reason {
	case "no_chat_view":
		_, _ = fmt.Fprintln(e.Stderr, "webmux: the session has no chat view; only a launch config with the chat view starts one")
	case "input_failed":
		if conflict.Submission != nil {
			_, _ = fmt.Fprintf(e.Stderr, "webmux: the input was not written: %s\n", conflict.Submission.State)
		}
	case "connector_missing":
		if conflict.Connector != nil {
			_, _ = fmt.Fprintf(e.Stderr, "webmux: install %s: %s\n", conflict.Connector.Title, conflict.Connector.InstallCommand)
		}
	}
	if len(conflict.Candidates) > 0 {
		_, _ = fmt.Fprintln(e.Stderr, "webmux: pick an agent session with --agent-session:")
		for _, c := range conflict.Candidates {
			_, _ = fmt.Fprintf(e.Stderr, "webmux:   %s\t%s\t%s\n", c.AgentSessionID, c.UpdatedAt.Local().Format(time.DateTime), c.Title)
		}
	}
	return contract.ExitError
}

// sessionConflict decodes the 409 body of a session command.
func sessionConflict(err error) (contract.SessionConflict, bool) {
	var apiErr *localclient.APIError
	var conflict contract.SessionConflict
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict || json.Unmarshal(apiErr.Body, &conflict) != nil || conflict.Reason == "" {
		return conflict, false
	}
	return conflict, true
}

// chatCommand runs one chat view write for the session in args[0].
func (e Env) chatCommand(ctx context.Context, args []string, method, path string, body any) int {
	client, code := e.connect()
	if client == nil {
		return code
	}
	defer client.Close()
	info, code, ok := e.chatSession(ctx, client, args[0], true)
	if !ok {
		return code
	}
	if err := client.Do(ctx, method, "/api/sessions/"+url.PathEscape(info.ID)+"/chat/"+path, body, nil); err != nil {
		return e.sessionFail(err)
	}
	return contract.ExitOK
}

// queuedGone ends a wait for a queued prompt once it left the queue
// without a turn sending it. The turn.start that sends it is published
// before the state that drops it, so a missing item means it was removed.
func queuedGone(item string) func(*contract.ChatState) error {
	seen := false
	return func(state *contract.ChatState) error {
		present := slices.ContainsFunc(state.Queue.Items, func(it contract.ChatQueueItem) bool { return it.ID == item })
		if present {
			seen = true
			return nil
		}
		if seen {
			return fmt.Errorf("the queued prompt %s was removed before it was sent", item)
		}
		return nil
	}
}

var chatWatchCommand = &Command{
	Name: "chat watch", Group: GroupChat, Args: "SESSION",
	Summary: "Stream a session's chat view: messages, tool calls, requests and turns.",
	Description: "Watching follows the chat stream until interrupted or the session ends. Text output shows each user and agent " +
		"message, tool call titles with their status, requests and turn boundaries; --json prints every server frame as one " +
		"line of JSON (the ChatFrame of /ws/s/{id}/chat). Earlier history is skipped unless --history is given; the history " +
		"shows each tool call once, in its final state, while live frames are printed as they arrive. Watching " +
		"works on either view and changes nothing.",
	Arguments: []Item{chatSessionArgument},
	Env:       []Item{sessionIDEnv},
	Output:    "Text lines, or NDJSON frames with --json.",
	Exit:      chatExits(ExitItem{Code: 130, Text: "Interrupted."}),
	Examples: []Example{
		{Command: "webmux chat watch abcd2345", Text: "Follow what an agent is doing."},
		{Command: "webmux chat watch abcd2345 --json | jq -c 'select(.type==\"events\") | .items[]'", Text: "Process chat events in a script."},
	},
	SeeAlso: []string{"webmux chat log", "webmux chat status", "webmux events"},
	Bind: func(fs *flag.FlagSet) Action {
		asJSON := fs.Bool("json", false, "print each frame as one line of JSON")
		history := fs.Bool("history", false, "also print the history the server still holds")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 1 {
				return e.Usagef("exactly one session is required")
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			info, code, ok := e.chatSession(ctx, client, args[0], false)
			if !ok {
				return code
			}
			stream, err := openChat(ctx, client, info.ID)
			if err != nil {
				return e.Fail(err)
			}
			defer stream.close()
			err = e.watchChat(ctx, stream, *asJSON, *history)
			if ctx.Err() != nil {
				return 130
			}
			if err != nil {
				return e.Fail(err)
			}
			return contract.ExitOK
		}
	},
}

func (e Env) watchChat(ctx context.Context, stream *chatStream, asJSON, history bool) error {
	out := bufio.NewWriter(e.Stdout)
	defer func() { _ = out.Flush() }()
	text := chatText{w: out}
	replay := false
	for {
		frame, err := stream.read(ctx)
		if err != nil {
			if status := websocket.CloseStatus(err); status == websocket.StatusNormalClosure || status == contract.CloseMoved {
				return nil
			}
			return err
		}
		// The events frame right after a snapshot replays the history; it
		// shows each tool call once, in its final state.
		replayed := replay && frame.Type == contract.ChatFrameEvents
		replay = frame.Type == contract.ChatFrameSnapshot
		if replayed && !history {
			continue
		}
		if replayed {
			frame.Items = foldTools(frame.Items)
		}
		if asJSON {
			line, _ := json.Marshal(frame)
			_, _ = out.Write(append(line, '\n'))
		} else {
			for _, ev := range frame.Items {
				text.event(ev)
			}
		}
		if err := out.Flush(); err != nil {
			return err
		}
	}
}

// chatText renders chat events as lines; message chunks continue the
// line of the message they belong to.
type chatText struct {
	w       *bufio.Writer
	message string
}

func (t *chatText) line(format string, args ...any) {
	if t.message != "" {
		_, _ = t.w.WriteString("\n")
		t.message = ""
	}
	_, _ = fmt.Fprintf(t.w, format+"\n", args...)
}

// end finishes the line of the message still open.
func (t *chatText) end() {
	if t.message != "" {
		_, _ = t.w.WriteString("\n")
		t.message = ""
	}
}

func (t *chatText) event(ev contract.ChatEvent) {
	switch {
	case ev.Message != nil:
		if ev.Message.Role == "thought" {
			return
		}
		if t.message != ev.Message.MessageID {
			if t.message != "" {
				_, _ = t.w.WriteString("\n")
			}
			t.message = ev.Message.MessageID
			_, _ = fmt.Fprintf(t.w, "%s: ", ev.Message.Role)
		}
		_, _ = t.w.WriteString(ev.Message.Text)
	case ev.Tool != nil && (ev.Tool.Title != nil || ev.Tool.Status != nil):
		title, status := "", ""
		if ev.Tool.Title != nil {
			title = *ev.Tool.Title
		}
		if ev.Tool.Status != nil {
			status = " (" + *ev.Tool.Status + ")"
		}
		t.line("tool %s: %s%s", ev.Tool.ToolCallID, title, status)
	case ev.TurnStart != nil:
		t.line("--- turn %s started", ev.TurnStart.TurnID)
	case ev.TurnEnd != nil:
		reason := ev.TurnEnd.StopReason
		if ev.TurnEnd.Error != "" {
			reason += ": " + ev.TurnEnd.Error
		}
		t.line("--- turn %s ended: %s", ev.TurnEnd.TurnID, reason)
	case ev.Request != nil:
		t.line("request %s (%s): %s", ev.Request.RequestID, ev.Request.Kind, requestSummary(*ev.Request))
	case ev.Resolved != nil:
		t.line("request %s closed: %s", ev.Resolved.RequestID, ev.Resolved.Outcome)
	case ev.Notice != nil:
		t.line("%s: %s", ev.Notice.Level, ev.Notice.Text)
	}
}

func requestSummary(r contract.ChatRequest) string {
	if r.Title != "" {
		return r.Title
	}
	if r.Form != nil {
		return r.Form.Message
	}
	return ""
}

// chatSnapshot reads the chat state once.
func chatSnapshot(ctx context.Context, client *localclient.Client, id string) (contract.ChatState, error) {
	stream, err := openChat(ctx, client, id)
	if err != nil {
		return contract.ChatState{}, err
	}
	defer stream.close()
	for stream.state == nil {
		if _, err := stream.read(ctx); err != nil {
			return contract.ChatState{}, err
		}
	}
	return *stream.state, nil
}

var chatStatusCommand = &Command{
	Name: "chat status", Group: GroupChat, Args: "SESSION",
	Summary: "Show a chat session's view, turn, queue, permission mode, pending requests and agent settings.",
	Description: "Status reads the chat state once. Pending requests are listed with their IDs and options, which webmux chat " +
		"answer takes; the agent settings list the IDs and values webmux chat set takes.",
	Arguments: []Item{chatSessionArgument},
	Env:       []Item{sessionIDEnv},
	Output:    "Aligned lines; with --json, the ChatState object plus the session's view, transition and agent.",
	Exit:      chatExits(),
	Examples: []Example{
		{Command: "webmux chat status abcd2345", Text: "See whether the agent is busy or waiting for an answer."},
		{Command: "webmux chat status self --json | jq '.requests[].requestId'", Text: "List pending request IDs."},
	},
	SeeAlso: []string{"webmux chat answer", "webmux chat set", "webmux get"},
	Bind: func(fs *flag.FlagSet) Action {
		asJSON := fs.Bool("json", false, "print the ChatState object with the view, transition and agent")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 1 {
				return e.Usagef("exactly one session is required")
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			info, code, ok := e.chatSession(ctx, client, args[0], false)
			if !ok {
				return code
			}
			state, err := chatSnapshot(ctx, client, info.ID)
			if err != nil {
				return e.Fail(err)
			}
			if *asJSON {
				writeJSON(e.Stdout, chatStatus{View: info.View, Transition: info.Transition, Agent: info.Agent, ChatState: state})
				return contract.ExitOK
			}
			e.printChatState(info, state)
			return contract.ExitOK
		}
	},
}

// chatStatus is chat status --json: the chat state and where the session
// is, which the session info carries.
type chatStatus struct {
	View       contract.SessionView        `json:"view"`
	Transition *contract.SessionTransition `json:"transition"`
	Agent      *contract.SessionAgent      `json:"agent"`
	contract.ChatState
}

func (e Env) printChatState(info contract.SessionInfo, state contract.ChatState) {
	w := tabwriter.NewWriter(e.Stdout, 0, 4, 2, ' ', 0)
	view := string(info.View)
	if info.Transition != nil {
		view += " → " + string(info.Transition.To) + " (" + string(info.Transition.Phase) + ")"
	}
	agent, agentSession := "-", "-"
	if info.Agent != nil {
		agent = info.Agent.Name
		if info.Agent.SessionID != nil {
			agentSession = *info.Agent.SessionID
		}
	}
	turn := "idle"
	switch {
	case state.Turn.Running:
		turn = "running " + state.Turn.TurnID
	case state.Turn.TurnID != "":
		turn = "idle; " + state.Turn.TurnID + " ended with " + state.Turn.StopReason
	}
	queue := fmt.Sprintf("%d", len(state.Queue.Items))
	if state.Queue.Paused {
		queue += " (paused)"
	}
	for _, row := range [][2]string{
		{"session", info.ID}, {"agent", agent}, {"agent session", agentSession}, {"view", view},
		{"turn", turn}, {"queue", queue}, {"permission", string(state.PermissionMode)},
	} {
		_, _ = fmt.Fprintf(w, "%s\t%s\n", row[0], row[1])
	}
	_ = w.Flush()
	if len(state.Requests) > 0 {
		_, _ = fmt.Fprintln(e.Stdout, "\nPending requests:")
		for _, r := range state.Requests {
			_, _ = fmt.Fprintf(e.Stdout, "  %s (%s) %s\n", r.RequestID, r.Kind, requestSummary(r))
			for _, o := range r.Options {
				_, _ = fmt.Fprintf(e.Stdout, "      --option %s  %s\n", o.OptionID, o.Name)
			}
			if r.Form != nil {
				for _, f := range r.Form.Fields {
					_, _ = fmt.Fprintf(e.Stdout, "      field %s (%s) %s\n", f.Name, f.Type, f.Title)
				}
			}
		}
	}
	if len(state.Config) > 0 {
		_, _ = fmt.Fprintln(e.Stdout, "\nAgent settings (webmux chat set ID=VALUE):")
		for _, c := range state.Config {
			value := c.Value
			if c.Type == "boolean" && c.Enabled != nil {
				value = fmt.Sprint(*c.Enabled)
			}
			choices := make([]string, 0, len(c.Options))
			for _, o := range c.Options {
				choices = append(choices, o.Value)
			}
			extra := ""
			if len(choices) > 0 {
				extra = "  [" + strings.Join(choices, ", ") + "]"
			}
			_, _ = fmt.Fprintf(e.Stdout, "  %s=%s  %s%s\n", c.ID, value, c.Name, extra)
		}
	}
}
