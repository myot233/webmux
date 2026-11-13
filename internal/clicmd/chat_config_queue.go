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
	"strconv"
	"strings"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

type stringsFlag []string

func (f *stringsFlag) String() string     { return strings.Join(*f, ",") }
func (f *stringsFlag) Set(v string) error { *f = append(*f, v); return nil }

// oneLine shortens text to its first line and at most n runes.
func oneLine(text string, n int) string {
	text, _, _ = strings.Cut(strings.TrimSpace(text), "\n")
	if runes := []rune(text); len(runes) > n {
		return string(runes[:n-1]) + "…"
	}
	return text
}

var chatAnswerCommand = &Command{
	Name: "chat answer", Group: GroupChat, Args: "SESSION REQUEST",
	Summary: "Answer an agent's pending permission request or question.",
	Description: "A permission request takes --option with one of its option IDs, or --cancel. A form (a question) takes " +
		"--content with a JSON object of field values, --decline or --cancel. Every device closes the request card at once. " +
		"webmux chat status lists pending requests with their options and fields. Check the current request and the user's authorization; use optionId, not kind.",
	Arguments: []Item{chatSessionArgument, {Name: "REQUEST", Text: "The request ID from chat status, chat watch or a stopped send --reply."}},
	Env:       []Item{sessionIDEnv},
	Exit:      chatExits(ExitItem{Code: 1, Text: "Also: no such pending request, or it was answered already."}),
	Examples: []Example{
		{Command: "webmux chat answer abcd2345 r3 --option OPTION_ID", Text: "Use the current authorized request's actual option ID from chat status."},
		{Command: `webmux chat answer abcd2345 r4 --content '{"plan":"B"}'`, Text: "Answer a question form."},
		{Command: "webmux chat answer abcd2345 r3 --cancel", Text: "Dismiss a request."},
	},
	SeeAlso: []string{"webmux chat status", "webmux chat permission", "webmux guide chat"},
	Notes: map[string]Note{
		"option":  {Conflicts: []string{"cancel", "content", "decline"}},
		"cancel":  {Conflicts: []string{"option", "content", "decline"}},
		"content": {Conflicts: []string{"option", "cancel", "decline"}},
		"decline": {Conflicts: []string{"option", "cancel", "content"}},
	},
	Bind: func(fs *flag.FlagSet) Action {
		option := fs.String("option", "", "select the permission option `ID`")
		cancelIt := fs.Bool("cancel", false, "cancel the request")
		content := fs.String("content", "", "submit a form with this `JSON` object of field values")
		decline := fs.Bool("decline", false, "decline a form")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 2 {
				return e.Usagef("a session and a request ID are required")
			}
			if countTrue(*option != "", *cancelIt, *content != "", *decline) != 1 {
				return e.Usagef("give exactly one of --option, --cancel, --content or --decline")
			}
			var fields map[string]json.RawMessage
			if *content != "" {
				if err := json.Unmarshal([]byte(*content), &fields); err != nil {
					return e.Usagef("--content must be a JSON object: %v", err)
				}
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			info, code, ok := e.chatSession(ctx, client, args[0], true)
			if !ok {
				return code
			}
			state, err := chatSnapshot(ctx, client, info.ID)
			if err != nil {
				return e.Fail(err)
			}
			i := slices.IndexFunc(state.Requests, func(r contract.ChatRequest) bool { return r.RequestID == args[1] })
			if i < 0 {
				_, _ = fmt.Fprintf(e.Stderr, "webmux: no pending request %s (answered already, or see webmux chat status)\n", args[1])
				return contract.ExitError
			}
			request := contract.ChatAnswerRequest{ClientRequestID: newRequestID()}
			form := state.Requests[i].Kind == "form"
			switch {
			case *option != "" && form, (*content != "" || *decline) && !form:
				return e.Usagef("request %s is a %s; see webmux chat answer --help", args[1], state.Requests[i].Kind)
			case *option != "":
				request.Outcome, request.OptionID = "selected", *option
			case *cancelIt && form:
				request.Outcome = "cancel"
			case *cancelIt:
				request.Outcome = "cancelled"
			case *decline:
				request.Outcome = "decline"
			default:
				request.Outcome, request.Content = "accept", fields
			}
			if err := client.Do(ctx, http.MethodPost, "/api/sessions/"+url.PathEscape(info.ID)+"/chat/requests/"+url.PathEscape(args[1]), request, nil); err != nil {
				return e.sessionFail(err)
			}
			return contract.ExitOK
		}
	},
}

// confirm asks question on a terminal. Without one it fails with a usage
// error naming --yes; a refusal exits 1.
func (e Env) confirm(question string) (int, bool) {
	if e.Interactive == nil || !e.Interactive() || e.Stdin == nil {
		return e.Usagef("not a terminal: repeat with --yes to confirm"), false
	}
	_, _ = fmt.Fprintf(e.Stderr, "%s [y/N] ", question)
	answer, _ := bufio.NewReader(e.Stdin).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(answer)); a == "y" || a == "yes" {
		return contract.ExitOK, true
	}
	_, _ = fmt.Fprintln(e.Stderr, "webmux: cancelled")
	return contract.ExitError, false
}

var chatSetCommand = &Command{
	Name: "chat set", Group: GroupChat, Args: "SESSION ID=VALUE...",
	Summary: "Change the agent's settings, such as its model or mode, on the chat view.",
	Description: "Each ID is one of the agent's settings as webmux chat status lists them. A select setting takes one of its " +
		"values; a boolean setting takes true or false. Settings are applied in order; when one fails it is reported, the " +
		"command exits 1, and the ones already applied stay. Every device shows the change.",
	Arguments: []Item{chatSessionArgument, {Name: "ID=VALUE", Text: "A setting and its new value."}},
	Env:       []Item{sessionIDEnv},
	Exit:      chatExits(ExitItem{Code: 1, Text: "Also: an unknown setting or value."}),
	Examples: []Example{
		{Command: "webmux chat status abcd2345", Text: "List the settings and their values first."},
		{Command: "webmux chat set abcd2345 SETTING_ID=VALUE", Text: "Apply a setting ID and allowed value discovered with chat status."},
	},
	SeeAlso: []string{"webmux chat status", "webmux chat permission", "webmux launch-config options"},
	Bind: func(fs *flag.FlagSet) Action {
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) < 2 {
				return e.Usagef("a session and at least one ID=VALUE are required")
			}
			type change struct{ id, value string }
			var changes []change
			for _, arg := range args[1:] {
				id, value, ok := strings.Cut(arg, "=")
				if !ok || id == "" {
					return e.Usagef("expected ID=VALUE, got %q", arg)
				}
				changes = append(changes, change{id, value})
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			info, code, ok := e.chatSession(ctx, client, args[0], true)
			if !ok {
				return code
			}
			state, err := chatSnapshot(ctx, client, info.ID)
			if err != nil {
				return e.Fail(err)
			}
			for _, c := range changes {
				if err := e.setChatConfig(ctx, client, info.ID, state.Config, c.id, c.value); err != nil {
					_, _ = fmt.Fprintf(e.Stderr, "webmux: %s=%s: ", c.id, c.value)
					return e.sessionFail(err)
				}
			}
			return contract.ExitOK
		}
	},
}

func (e Env) setChatConfig(ctx context.Context, client *localclient.Client, id string, config []contract.ChatConfig, key, value string) error {
	i := slices.IndexFunc(config, func(c contract.ChatConfig) bool { return c.ID == key })
	if i < 0 {
		ids := make([]string, 0, len(config))
		for _, c := range config {
			ids = append(ids, c.ID)
		}
		return fmt.Errorf("unknown setting (the agent offers: %s)", strings.Join(ids, ", "))
	}
	request := contract.ChatConfigRequest{ClientRequestID: newRequestID()}
	if config[i].Type == "boolean" {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return errors.New("a boolean setting takes true or false")
		}
		request.Enabled = &enabled
	} else {
		values := make([]string, 0, len(config[i].Options))
		for _, o := range config[i].Options {
			values = append(values, o.Value)
		}
		if len(values) > 0 && !slices.Contains(values, value) {
			return fmt.Errorf("unknown value (choose from: %s)", strings.Join(values, ", "))
		}
		request.Value = &value
	}
	return client.Do(ctx, http.MethodPut, "/api/sessions/"+url.PathEscape(id)+"/chat/config/"+url.PathEscape(key), request, nil)
}

var chatPermissionCommand = &Command{
	Name: "chat permission", Group: GroupChat, Args: "SESSION ask|auto",
	Summary: "Choose whether the agent's permission requests wait for an answer (ask) or are approved by the server (auto).",
	Description: "auto approves each permission request on the server with its first allow-once option (else allow-always); " +
		"questions (forms) always wait for an answer. The mode applies to the chat view and every device shows it.",
	Arguments: []Item{chatSessionArgument, {Name: "MODE", Text: "ask or auto."}},
	Env:       []Item{sessionIDEnv},
	Exit:      chatExits(),
	Examples: []Example{
		{Command: "webmux chat permission abcd2345 ask", Text: "Review every tool call before it runs."},
		{Command: "webmux chat permission self auto", Text: "Let the current agent run tools without asking."},
	},
	SeeAlso: []string{"webmux chat answer", "webmux chat status"},
	Bind: func(fs *flag.FlagSet) Action {
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 2 || args[1] != "ask" && args[1] != "auto" {
				return e.Usagef("a session and a mode (ask or auto) are required")
			}
			return e.chatCommand(ctx, args, http.MethodPut, "permission-mode",
				contract.ChatPermissionModeRequest{ClientRequestID: newRequestID(), Mode: contract.ChatPermissionMode(args[1])})
		}
	},
}

var chatQueueGroup = &Command{
	Name: "chat queue", Group: GroupChat,
	Summary: "Inspect and edit the prompts waiting in a chat session's queue.",
}

var queueItemArgument = Item{Name: "ITEM", Text: "A queued prompt's ID from webmux chat queue ls."}

var chatQueueListCommand = &Command{
	Name: "chat queue ls", Group: GroupChat, Args: "SESSION",
	Summary:   "List the queued prompts in the order they will be sent.",
	Arguments: []Item{chatSessionArgument},
	Env:       []Item{sessionIDEnv},
	Output:    "Lines \"N<TAB>ITEM<TAB>TEXT\" (the text shortened to one line); with --json, the ChatQueue {paused, items}.",
	Exit:      chatExits(),
	Examples: []Example{
		{Command: "webmux chat queue ls abcd2345", Text: "See what the agent will do next."},
		{Command: "webmux chat queue ls abcd2345 --json | jq -r '.items[].id'", Text: "List queued prompt IDs."},
	},
	SeeAlso: []string{"webmux send", "webmux chat status"},
	Bind: func(fs *flag.FlagSet) Action {
		asJSON := fs.Bool("json", false, "print the ChatQueue object")
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
			if state.Queue.Items == nil {
				state.Queue.Items = []contract.ChatQueueItem{}
			}
			if *asJSON {
				writeJSON(e.Stdout, state.Queue)
				return contract.ExitOK
			}
			if state.Queue.Paused {
				_, _ = fmt.Fprintln(e.Stderr, "webmux: the queue is paused; webmux chat queue resume sends it again")
			}
			for i, item := range state.Queue.Items {
				_, _ = fmt.Fprintf(e.Stdout, "%d\t%s\t%s\n", i+1, item.ID, oneLine(item.Text, 80))
			}
			return contract.ExitOK
		}
	},
}

// queueCommand builds a queue write that takes SESSION and, when item is
// set, ITEM.
func queueCommand(name, summary, method, path string, item bool, examples ...Example) *Command {
	args, arguments := "SESSION", []Item{chatSessionArgument}
	if item {
		args, arguments = "SESSION ITEM", append(arguments, queueItemArgument)
	}
	return &Command{
		Name: "chat queue " + name, Group: GroupChat, Args: args,
		Summary: summary, Arguments: arguments, Env: []Item{sessionIDEnv},
		Exit:     chatExits(ExitItem{Code: 1, Text: "Also: the item was sent or removed already."}),
		Examples: examples,
		SeeAlso:  []string{"webmux chat queue ls"},
		Bind: func(fs *flag.FlagSet) Action {
			return func(ctx context.Context, e Env, a []string) int {
				want := 1
				if item {
					want = 2
				}
				if len(a) != want {
					return e.Usagef("usage: webmux chat queue %s %s", name, args)
				}
				p := path
				if item {
					p = strings.Replace(p, "{qid}", url.PathEscape(a[1]), 1)
				}
				return e.chatCommand(ctx, a, method, p, contract.ChatQueueRequest{ClientRequestID: newRequestID()})
			}
		},
	}
}

var (
	chatQueueRemoveCommand = queueCommand("rm", "Remove a queued prompt.", http.MethodDelete, "queue/{qid}", true,
		Example{Command: "webmux chat queue rm abcd2345 q3", Text: "Drop one queued prompt."},
		Example{Command: "webmux chat queue ls abcd2345 && webmux chat queue rm abcd2345 q1", Text: "Look up an item, then drop it."})
	chatQueueSendNowCommand = queueCommand("send-now", "Send a queued prompt now, cancelling the running turn.", http.MethodPost, "queue/{qid}/send-now", true,
		Example{Command: "webmux chat queue send-now abcd2345 q3", Text: "Interrupt the agent with a queued prompt."},
		Example{Command: "webmux chat queue send-now self q1", Text: "Promote the next prompt of the current session."})
	chatQueueClearCommand = queueCommand("clear", "Remove every queued prompt.", http.MethodPost, "queue/clear", false,
		Example{Command: "webmux chat queue clear abcd2345", Text: "Empty the queue on every device."},
		Example{Command: "webmux chat queue clear abcd2345 && webmux interrupt abcd2345", Text: "Stop all pending and current work."})
	chatQueueResumeCommand = queueCommand("resume", "Resume a paused queue, sending the next prompt when the agent is idle.", http.MethodPost, "queue/resume", false,
		Example{Command: "webmux chat queue resume abcd2345", Text: "Continue after a failed send paused the queue."},
		Example{Command: "webmux chat queue ls abcd2345 && webmux chat queue resume abcd2345", Text: "Check the queue, then send it again."})
)

var chatQueueEditCommand = &Command{
	Name: "chat queue edit", Group: GroupChat, Args: "SESSION ITEM [TEXT|-]",
	Summary:   "Replace a queued prompt's text, or move it up one place with --up.",
	Arguments: []Item{chatSessionArgument, queueItemArgument, {Name: "TEXT", Text: "The new text; - reads standard input."}},
	Env:       []Item{sessionIDEnv},
	Exit:      chatExits(ExitItem{Code: 1, Text: "Also: the item was sent or removed already."}),
	Examples: []Example{
		{Command: `webmux chat queue edit abcd2345 q3 "run the tests first"`, Text: "Reword a queued prompt."},
		{Command: "webmux chat queue edit abcd2345 q3 --up", Text: "Send a prompt one place earlier."},
	},
	SeeAlso: []string{"webmux chat queue ls"},
	Bind: func(fs *flag.FlagSet) Action {
		up := fs.Bool("up", false, "move the item up one place")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) < 2 || len(args) > 3 || len(args) == 2 && !*up {
				return e.Usagef("a session, an item and new TEXT (or --up) are required")
			}
			request := contract.ChatQueueRequest{ClientRequestID: newRequestID()}
			if *up {
				request.Move = "up"
			}
			if len(args) == 3 {
				text := args[2]
				if text == "-" {
					read, err := e.readStdin()
					if err != nil {
						return e.Fail(err)
					}
					text = read
				}
				request.Text = &text
			}
			return e.chatCommand(ctx, args, http.MethodPatch, "queue/"+url.PathEscape(args[1]), request)
		}
	},
}
