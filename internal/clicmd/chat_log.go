package clicmd

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"slices"

	"github.com/coder/websocket"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/sessionobserve"
)

// maxChatLogLast bounds --last; the server's log is bounded by bytes, so a
// larger count could only ever return the whole log.
const maxChatLogLast = 1000

var chatLogCommand = &Command{
	Name: "chat log", Group: GroupChat, Args: "SESSION",
	Summary: "Print the last messages of a session's chat view, then exit.",
	Description: "Log reads the history the server still holds once and prints the last --last user and agent messages " +
		"(default 1: usually the agent's last reply). It does not follow the chat, so it also answers what a session " +
		"said after it finished; unlike send --reply it does not send anything and does not wait for a turn. Tool " +
		"calls are left out unless --tools is given; then each tool call is one line with its title and final status, " +
		"however many progress updates it had. The server keeps a bounded history: older messages may be gone, and a " +
		"message of a turn still running may be incomplete. --json prints one ChatFrame of type events holding the " +
		"selected events: messages with their chunks joined, tool calls with their patches folded into the final state. " +
		"Log works on either view and changes nothing.",
	Arguments: []Item{chatSessionArgument},
	Env:       []Item{sessionIDEnv},
	Output:    "Text lines, or one JSON ChatFrame with --json.",
	Exit:      chatExits(),
	Examples: []Example{
		{Command: "webmux chat log abcd2345", Text: "Read what the agent said last."},
		{Command: "webmux chat log abcd2345 --last 3 --tools", Text: "Read the last three messages and the tool calls between them."},
		{Command: "webmux chat log abcd2345 --json | jq -r '.items[].message.text'", Text: "Process the messages in a script."},
	},
	SeeAlso: []string{"webmux chat watch", "webmux chat status", "webmux send"},
	Bind: func(fs *flag.FlagSet) Action {
		last := fs.Int("last", 1, "print the last `N` messages (1 to 1000)")
		tools := fs.Bool("tools", false, "also print tool calls with their final status")
		asJSON := fs.Bool("json", false, "print the selected events as one ChatFrame of JSON")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 1 {
				return e.Usagef("exactly one session is required")
			}
			if *last < 1 || *last > maxChatLogLast {
				return e.Usagef("--last must be between 1 and %d", maxChatLogLast)
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
			stream, err := sessionobserve.OpenChat(ctx, client, info.ID, 64<<20)
			if err != nil {
				return e.Fail(err)
			}
			defer stream.Close()
			snapshot, items, err := readChatHistory(ctx, stream)
			if err != nil {
				return e.Fail(err)
			}
			selected := selectChatLog(items, *last, *tools)
			if snapshot.Truncated && countMessages(selected) < *last {
				_, _ = fmt.Fprintln(e.Stderr, "webmux: the server dropped older history; fewer messages are available")
			}
			if stream.State() != nil && stream.State().Turn.Running {
				_, _ = fmt.Fprintln(e.Stderr, "webmux: a turn is running; the last message may be incomplete")
			}
			if *asJSON {
				frame := contract.ChatFrame{Type: contract.ChatFrameEvents, Generation: snapshot.Generation, Items: selected}
				if len(selected) > 0 {
					frame.From = selected[0].Seq
				}
				writeJSON(e.Stdout, frame)
				return contract.ExitOK
			}
			out := bufio.NewWriter(e.Stdout)
			text := chatText{w: out}
			for _, ev := range selected {
				text.event(ev)
			}
			text.end()
			if err := out.Flush(); err != nil {
				return e.Fail(err)
			}
			return contract.ExitOK
		}
	},
}

// readChatHistory reads the snapshot and the events frame the server sends
// right after it, which holds the whole cached log of a fresh connection.
func readChatHistory(ctx context.Context, stream *sessionobserve.ChatSubscription) (contract.ChatFrame, []contract.ChatEvent, error) {
	var snapshot contract.ChatFrame
	for {
		frame, err := stream.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) == contract.CloseMoved {
				return snapshot, nil, errors.New("the session moved to its terminal view while reading")
			}
			return snapshot, nil, err
		}
		switch frame.Type {
		case contract.ChatFrameSnapshot:
			snapshot = frame
		case contract.ChatFrameEvents:
			return snapshot, frame.Items, nil
		}
	}
}

func countMessages(events []contract.ChatEvent) int {
	n := 0
	for _, ev := range events {
		if ev.Message != nil {
			n++
		}
	}
	return n
}

// selectChatLog returns the events of the last n user and agent messages:
// from the first chunk of the n-th message from the end to the end of the
// log. Thoughts and every other event kind are left out; tool calls stay
// only with tools.
func selectChatLog(items []contract.ChatEvent, n int, tools bool) []contract.ChatEvent {
	kept := make([]contract.ChatEvent, 0, len(items))
	for _, ev := range items {
		switch {
		case ev.Message != nil && ev.Message.Role != "thought":
			kept = append(kept, ev)
		case ev.Tool != nil && tools, ev.TurnStart != nil:
			kept = append(kept, ev)
		}
	}
	kept = slices.DeleteFunc(foldTools(mergeMessages(kept)), func(ev contract.ChatEvent) bool { return ev.TurnStart != nil })
	start, seen := 0, 0
	for i := len(kept) - 1; i >= 0; i-- {
		if kept[i].Message == nil {
			continue
		}
		if seen++; seen == n {
			start = i
			break
		}
	}
	return kept[start:]
}

// mergeMessages joins the chunks of one message into the event at its first
// chunk, which carries the last chunk's sequence as the server's merged
// events do. Events are copied, never modified.
func mergeMessages(items []contract.ChatEvent) []contract.ChatEvent {
	out := make([]contract.ChatEvent, 0, len(items))
	at := map[string]int{}
	for _, ev := range items {
		if ev.Message == nil {
			out = append(out, ev)
			continue
		}
		key := ev.Message.Role + "\x00" + ev.Message.MessageID
		i, ok := at[key]
		if !ok {
			at[key] = len(out)
			out = append(out, ev)
			continue
		}
		merged := *out[i].Message
		merged.Text += ev.Message.Text
		merged.Truncated = merged.Truncated || ev.Message.Truncated
		merged.Attachments = append(merged.Attachments[:len(merged.Attachments):len(merged.Attachments)], ev.Message.Attachments...)
		out[i].Message, out[i].Seq = &merged, ev.Seq
	}
	return out
}

// foldTools folds the tool.upsert patches of one tool call into the event
// at its first patch, so only its final state remains. A patch changes only
// the fields it sets; the folded event takes the last patch's sequence. A
// turn start ends every open tool call, so an agent that reuses a tool call
// ID in a later turn does not alter the earlier call. Events are copied,
// never modified.
func foldTools(items []contract.ChatEvent) []contract.ChatEvent {
	out := make([]contract.ChatEvent, 0, len(items))
	at := map[string]int{}
	for _, ev := range items {
		if ev.Tool == nil {
			if ev.TurnStart != nil {
				clear(at)
			}
			out = append(out, ev)
			continue
		}
		i, ok := at[ev.Tool.ToolCallID]
		if !ok {
			at[ev.Tool.ToolCallID] = len(out)
			out = append(out, ev)
			continue
		}
		folded, p := *out[i].Tool, ev.Tool
		if p.Kind != nil {
			folded.Kind = p.Kind
		}
		if p.Title != nil {
			folded.Title = p.Title
		}
		if p.Status != nil {
			folded.Status = p.Status
		}
		if p.Content != nil {
			folded.Content = p.Content
		}
		if p.Locations != nil {
			folded.Locations = p.Locations
		}
		out[i].Tool, out[i].Seq = &folded, ev.Seq
	}
	return out
}
