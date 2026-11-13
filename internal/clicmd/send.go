package clicmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
	"github.com/rainchan/webmux/internal/sessionobserve"
)

type sendOptions struct {
	enter            bool
	keys             listFlag
	attachments      listFlag
	mode             string
	wait             string
	reply            bool
	autoWaitRequests bool
	timeout          time.Duration
	asJSON           bool
}

var sendCommand = &Command{
	Name: "send", Group: GroupSessions, Args: "SESSION [TEXT|-]",
	Summary: "Send input to a session: typed into its terminal, or a prompt on its chat view.",
	Description: "The server sends the input to the session's current view. On the terminal view TEXT is one acknowledged " +
		"submission (with Enter when --enter is given) and each --key follows; the command exits 0 once the server reports " +
		"everything written. Typing takes control of the terminal; the control indicator names this client \"script\" and, " +
		"inside a session, that session. On the chat view TEXT is a prompt, sent at once or queued while a turn runs; " +
		"--attach-file uploads files into the session and sends them with it, and --mode chooses how: auto (default) sends " +
		"now or queues, steer injects into the running turn, refuse fails instead of queueing, and now cancels the running " +
		"turn and sends next. A queued prompt reports its item and position on standard error. Options of the other view " +
		"are refused (wrong_view) and nothing is sent.\n\n" +
		"--wait STATE returns on the first change into STATE after the input (see webmux wait). --reply, for the chat view " +
		"only, follows the turn this input starts and prints the agent's final reply; a stop for a permission request or " +
		"question exits 1 unless --auto-wait-requests.",
	Arguments: []Item{sessionArgument, {Name: "TEXT", Text: "The text to send; - reads standard input."}},
	Env:       []Item{sessionIDEnv},
	Output: "Nothing; with --json, the SessionInputResult (view, queued, itemId, position, turnId, promptId). With --wait --json, the " +
		"SessionInfo that matched. With --reply, the reply text, or with --json {stopReason, error, reply, events, request}.",
	Exit: exitsCommon(
		ExitItem{Code: 1, Text: "No such session, it is not active or is changing views, an option belongs to the other view, " +
			"--mode refuse found the chat busy, the input was not (fully) written, an upload failed, or with --reply the turn " +
			"did not end with end_turn or waits for an answer."},
		ExitItem{Code: 124, Text: "--wait or --reply timed out; a turn goes on."},
	),
	Examples: []Example{
		{Command: "webmux send abcd2345 'make test' --enter --wait waiting,idle --timeout 10m", Text: "Run a command in a shell session and wait until its prompt is back (an idle shell prompt reads as waiting)."},
		{Command: `webmux send abcd2345 "fix the lint errors and summarize the change" --reply --timeout 20m`, Text: "Hand a task to an agent on its chat view and print its answer."},
		{Command: "webmux send abcd2345 'run the tests' --mode refuse", Text: "Fail with exit 1 instead of queueing when another turn is running."},
		{Command: "webmux send abcd2345 'stop and do this' --mode now", Text: "Cancel the running turn and send this prompt next."},
		{Command: "webmux send abcd2345 'what does this show?' --attach-file screenshot.png", Text: "Send a prompt with an image."},
	},
	SeeAlso: []string{"webmux interrupt", "webmux wait", "webmux screen", "webmux view", "webmux guide sessions"},
	Notes: map[string]Note{
		"key": {Repeatable: true}, "attach-file": {Repeatable: true},
		"wait":               {Conflicts: []string{"reply"}},
		"reply":              {Conflicts: []string{"wait"}},
		"auto-wait-requests": {Requires: []string{"reply"}},
	},
	Bind: func(fs *flag.FlagSet) Action {
		var o sendOptions
		fs.BoolVar(&o.enter, "enter", false, "terminal view: press Enter after the text, in the same submission")
		fs.Var(&o.keys, "key", "terminal view: send the named `KEY` after the text: "+keyNames())
		fs.Var(&o.attachments, "attach-file", "chat view: upload the file at `PATH` and send it with the prompt")
		fs.StringVar(&o.mode, "mode", "", "chat view: `MODE` auto (send now or queue, the default), steer (inject into the running turn), refuse (fail instead of queueing) or now (cancel the turn and send next)")
		fs.StringVar(&o.wait, "wait", "", "wait for the first change into `STATE` after sending (see webmux wait)")
		fs.BoolVar(&o.reply, "reply", false, "chat view: follow the turn this input starts and print the agent's final reply")
		fs.BoolVar(&o.autoWaitRequests, "auto-wait-requests", false, "with --reply, keep waiting while a permission request or question is open")
		fs.DurationVar(&o.timeout, "timeout", 0, "give up --wait or --reply after this `DURATION` (exit 124); 0 waits forever")
		fs.BoolVar(&o.asJSON, "json", false, "print the result as JSON")
		return func(ctx context.Context, e Env, args []string) int { return e.send(ctx, o, args) }
	},
}

// queuedNotice explains where a queued prompt sits and how to send it
// sooner; it goes to standard error so standard output stays script-safe.
func queuedNotice(r contract.SessionInputResult, mode string) string {
	item := ""
	if r.ItemID != "" {
		item = " as " + r.ItemID
	}
	position := ""
	if r.Position > 0 {
		position = fmt.Sprintf(" (position %d)", r.Position)
	}
	if mode == "now" {
		return fmt.Sprintf("webmux: queued%s%s; cancelling the running turn before sending it\n", item, position)
	}
	return fmt.Sprintf("webmux: queued%s%s; the chat is busy. Use --mode steer to inject it now, or --mode now to cancel the turn\n", item, position)
}

func (e Env) send(ctx context.Context, o sendOptions, args []string) int {
	if len(args) < 1 || len(args) > 2 {
		return e.Usagef("a session ID and at most one TEXT are required (quote text with spaces)")
	}
	for _, key := range o.keys {
		if _, ok := contract.InputKeys[key]; !ok {
			return e.Usagef("unknown key %q (keys: %s)", key, keyNames())
		}
	}
	var states []string
	switch {
	case o.wait != "" && o.reply:
		return e.Usagef("--wait and --reply cannot be combined")
	case o.mode != "" && !slices.Contains([]string{"auto", "steer", "refuse", "now"}, o.mode):
		return e.Usagef("--mode is auto, steer, refuse or now")
	case o.autoWaitRequests && !o.reply:
		return e.Usagef("--auto-wait-requests needs --reply")
	case o.timeout != 0 && o.wait == "" && !o.reply:
		return e.Usagef("--timeout needs --wait or --reply")
	case o.wait != "":
		var err error
		if states, err = parseStates(o.wait); err != nil {
			return e.Usagef("%v", err)
		}
	}
	id, code, ok := e.SessionID(args[0])
	if !ok {
		return code
	}
	text := ""
	if len(args) == 2 {
		text = args[1]
		if text == "-" {
			var err error
			if text, err = e.readStdin(); err != nil {
				return e.Fail(err)
			}
		}
	}
	if text == "" && !o.enter && len(o.keys) == 0 && len(o.attachments) == 0 {
		return e.Usagef("nothing to send: give TEXT, --enter, --key or --attach-file")
	}
	client, code := e.connect()
	if client == nil {
		return code
	}
	defer client.Close()
	base := "/api/sessions/" + url.PathEscape(id)
	request := contract.SessionInputRequest{ClientRequestID: newRequestID(), Text: text, Submit: o.enter, Keys: o.keys, Mode: o.mode}
	// Uploads and --reply need the chat view: check it before anything is
	// uploaded or sent.
	if o.reply || len(o.attachments) > 0 {
		var info contract.SessionInfo
		if err := client.Do(ctx, http.MethodGet, base, nil, &info); err != nil {
			return e.sessionFail(err)
		}
		if !onChat(info) {
			switch {
			case o.reply && hasChatView(info):
				_, _ = fmt.Fprintf(e.Stderr, "webmux send: session %s is on its terminal view, which has no reply to print; use --wait STATE, or webmux view %s chat first\n", id, id)
			case o.reply:
				_, _ = fmt.Fprintf(e.Stderr, "webmux send: session %s has no chat view, so there is no reply to print; use --wait STATE\n", id)
			default:
				_, _ = fmt.Fprintf(e.Stderr, "webmux send: session %s is not on its chat view; --attach-file is for the chat view (wrong_view)\n", id)
			}
			return contract.ExitError
		}
		for _, file := range o.attachments {
			path, err := e.uploadFile(ctx, client, base+"/upload", file)
			if err != nil {
				return e.Fail(fmt.Errorf("upload %s: %w; nothing was sent", file, err))
			}
			request.Attachments = append(request.Attachments, contract.ChatAttachment{Path: path})
		}
	}
	waitCtx := ctx
	if o.timeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, o.timeout)
		defer cancel()
	}
	if o.reply {
		return e.sendReply(ctx, waitCtx, client, id, request, o)
	}
	var watch *sessionWatch
	if o.wait != "" {
		var err error
		// Subscribe before the input: the change it causes must be seen.
		if watch, err = watchSessions(waitCtx, client); err != nil {
			return e.Fail(err)
		}
		defer watch.subscription.Close()
		if _, err := watch.subscription.Read(waitCtx); err != nil {
			return e.waitFailure(err)
		}
	}
	var result contract.SessionInputResult
	if err := client.Do(ctx, http.MethodPost, base+"/input", request, &result); err != nil {
		return e.sessionFail(err)
	}
	if !o.asJSON && result.Queued != nil && *result.Queued {
		_, _ = fmt.Fprint(e.Stderr, queuedNotice(result, o.mode))
	}
	if watch == nil {
		if o.asJSON {
			writeJSON(e.Stdout, result)
		}
		return contract.ExitOK
	}
	matched, err := watch.waitFor(waitCtx, waitOptions{ids: []string{id}, states: states, changed: true, primed: true})
	if err != nil {
		return e.waitFailure(err)
	}
	if o.asJSON {
		writeJSON(e.Stdout, matched[0])
	}
	return contract.ExitOK
}

// sendReply sends a prompt and follows the turn it starts: for a queued
// prompt the turn that later sends it, with steer the running turn.
func (e Env) sendReply(ctx, waitCtx context.Context, client *localclient.Client, id string, request contract.SessionInputRequest, o sendOptions) int {
	// Prime before input, preserving the replay as well as registering the
	// live subscription before a new or steered turn can end.
	stream, err := sessionobserve.OpenChat(waitCtx, client, id, 64<<20)
	if err != nil {
		return e.Fail(err)
	}
	defer stream.Close()
	if err := stream.Prime(waitCtx); err != nil {
		return e.waitFailure(err)
	}
	var result contract.SessionInputResult
	if err := client.Do(ctx, http.MethodPost, "/api/sessions/"+url.PathEscape(id)+"/input", request, &result); err != nil {
		return e.sessionFail(err)
	}
	if result.View != contract.ViewChat {
		// The session changed views between the check and the input.
		_, _ = fmt.Fprintf(e.Stderr, "webmux send: the input went to the %s view, which has no reply to print\n", result.View)
		return contract.ExitError
	}
	selector := sessionobserve.TurnSelector{}
	switch {
	case result.Queued != nil && *result.Queued:
		position := ""
		if result.Position > 0 {
			position = fmt.Sprintf(" (position %d)", result.Position)
		}
		_, _ = fmt.Fprintf(e.Stderr, "webmux: the prompt is queued as %s%s; waiting for its turn\n", result.ItemID, position)
		selector.QueueItemID = result.ItemID
	case result.TurnID != "":
		selector.TurnID = result.TurnID
	case result.PromptID != "":
		selector.PromptID = result.PromptID
	default:
		// Steering joins the running turn.
		if stream.State() == nil || !stream.State().Turn.Running {
			return e.Fail(errors.New("no turn is running to follow"))
		}
		selector.TurnID = stream.State().Turn.TurnID
	}
	turn, err := stream.WaitTurn(waitCtx, selector, sessionobserve.TurnPolicy{
		AttachPhase: sessionobserve.BeforeSubmit, StopAtRequest: !o.autoWaitRequests, CollectEvents: true,
	})
	if err != nil {
		return e.waitFailure(err)
	}
	return e.reportTurn(cliTurn(turn), o.asJSON)
}
