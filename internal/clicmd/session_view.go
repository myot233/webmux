package clicmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"github.com/rainchan/webmux/internal/attach"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

// The view commands act on any session by ID; the server dispatches by the
// session's current view.

var interruptCommand = &Command{
	Name: "interrupt", Group: GroupSessions, Args: "SESSION",
	Summary: "Interrupt what a session is doing: cancel the chat turn, or press the terminal's interrupt keys.",
	Description: "On the chat view the running turn is cancelled; with no turn running nothing happens. On the terminal view " +
		"the keys the agent's profile uses to interrupt it are typed, or Ctrl-C for any other program, as acknowledged input " +
		"that takes control of the terminal like webmux send.",
	Arguments: []Item{sessionArgument},
	Env:       []Item{sessionIDEnv},
	Exit:      exitsCommon(ExitItem{Code: 1, Text: "No such session, it is not active, it is changing views, or the keys were not written."}),
	Examples: []Example{
		{Command: "webmux interrupt abcd2345", Text: "Stop an agent's turn or a shell's running command."},
		{Command: "webmux interrupt abcd2345 && webmux send abcd2345 'start over with the smaller fix'", Text: "Redirect an agent."},
	},
	SeeAlso: []string{"webmux send", "webmux kill", "webmux guide sessions"},
	Bind: func(fs *flag.FlagSet) Action {
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 1 {
				return e.Usagef("exactly one session ID is required")
			}
			id, code, ok := e.SessionID(args[0])
			if !ok {
				return code
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			request := contract.SessionCommandRequest{ClientRequestID: newRequestID()}
			if err := client.Do(ctx, http.MethodPost, "/api/sessions/"+url.PathEscape(id)+"/interrupt", request, nil); err != nil {
				return e.sessionFail(err)
			}
			return contract.ExitOK
		}
	},
}

// viewNamed parses a view name.
func viewNamed(name string) (contract.SessionView, bool) {
	switch v := contract.SessionView(name); v {
	case contract.ViewTerminal, contract.ViewChat:
		return v, true
	}
	return "", false
}

var viewCommand = &Command{
	Name: "view", Group: GroupSessions, Args: "SESSION terminal|chat",
	Summary: "Move a session with a chat view to its terminal or chat view and wait until the change is over.",
	Description: "The same agent session continues on the other view. Terminal clients (webmux attach, browsers) are detached " +
		"when the session moves to chat, and chat views close when it moves to the terminal. When the terminal UI does not " +
		"become idle within its interrupt timeout, the change needs a choice: --force interrupts it; without --force the " +
		"change is cancelled and the command exits 1. When the agent's session cannot be identified, the candidates are " +
		"listed; repeat with --agent-session. A session already on the view is left as it is.",
	Arguments: []Item{sessionArgument, {Name: "VIEW", Text: "terminal or chat."}},
	Env:       []Item{sessionIDEnv},
	Output:    "Nothing; with --json, the ViewChangeResult (view, draft of queued prompts returned to the terminal, dropped and unsupported arguments).",
	Exit: exitsCommon(ExitItem{Code: 1, Text: "No such session, it has no chat view, the change needed a choice without --force, " +
		"the agent session is ambiguous, or the server refused."}),
	Examples: []Example{
		{Command: "webmux view abcd2345 chat && webmux send abcd2345 continue", Text: "Move to chat, then prompt."},
		{Command: "webmux view abcd2345 terminal --force", Text: "Return to the terminal UI even if it is busy."},
	},
	SeeAlso: []string{"webmux chat status", "webmux attach", "webmux guide chat"},
	Bind: func(fs *flag.FlagSet) Action {
		force := fs.Bool("force", false, "interrupt a busy terminal UI instead of cancelling the change")
		agentSession := fs.String("agent-session", "", "moving to chat, resume this agent session `ID` when the server lists candidates")
		asJSON := fs.Bool("json", false, "print the ViewChangeResult")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 2 {
				return e.Usagef("a session and a view (terminal or chat) are required")
			}
			to, ok := viewNamed(args[1])
			if !ok {
				return e.Usagef("the view is terminal or chat, not %q", args[1])
			}
			if *agentSession != "" && to != contract.ViewChat {
				return e.Usagef("--agent-session works only when moving to chat")
			}
			id, code, ok := e.SessionID(args[0])
			if !ok {
				return code
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			var info contract.SessionInfo
			if err := client.Do(ctx, http.MethodGet, "/api/sessions/"+url.PathEscape(id), nil, &info); err != nil {
				return e.sessionFail(err)
			}
			if !hasChatView(info) {
				_, _ = fmt.Fprintf(e.Stderr, "webmux: session %s has no chat view (no_chat_view); only a launch config with the chat view starts one\n", id)
				return contract.ExitError
			}
			if info.View == to && info.Transition == nil {
				if *asJSON {
					writeJSON(e.Stdout, contract.ViewChangeResult{View: to, Dropped: []string{}, Unsupported: []string{}})
				}
				return contract.ExitOK
			}
			result, chose, err := e.changeView(ctx, client, id, contract.ViewChangeRequest{
				ClientRequestID: newRequestID(), To: to, AgentSessionID: *agentSession,
			}, *force)
			if err != nil {
				return e.sessionFail(err)
			}
			if chose == contract.ViewChoiceCancel {
				_, _ = fmt.Fprintln(e.Stderr, "webmux: the terminal UI stayed busy, so the change was cancelled; repeat with --force to interrupt it")
				return contract.ExitError
			}
			if result.Draft != "" && !*asJSON {
				_, _ = fmt.Fprintln(e.Stderr, "webmux: queued prompts were returned as a draft (see --json)")
			}
			if *asJSON {
				writeJSON(e.Stdout, result)
			}
			return contract.ExitOK
		}
	},
}

// changeView runs a view change and answers its choice, when it pauses for
// one, with force or cancel. It reports the choice it made.
func (e Env) changeView(ctx context.Context, client *localclient.Client, id string, request contract.ViewChangeRequest, force bool) (contract.ViewChangeResult, string, error) {
	watch, err := watchSessions(ctx, client)
	if err != nil {
		return contract.ViewChangeResult{}, "", err
	}
	defer watch.close()
	path := "/api/sessions/" + url.PathEscape(id) + "/view"
	type outcome struct {
		result contract.ViewChangeResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		var result contract.ViewChangeResult
		err := client.Do(ctx, http.MethodPost, path, request, &result)
		done <- outcome{result, err}
	}()
	choiceCtx, stop := context.WithCancel(ctx)
	defer stop()
	choices := make(chan string, 1)
	go func() {
		awaiting := func(s contract.SessionInfo) bool {
			return s.ID == id && s.Transition != nil && s.Transition.Phase == contract.PhaseAwaitingChoice
		}
		for {
			event, err := watch.next(choiceCtx)
			if err != nil {
				return
			}
			if (event.Session != nil && awaiting(*event.Session)) || slices.ContainsFunc(event.Sessions, awaiting) {
				choice := contract.ViewChoiceCancel
				if force {
					choice = contract.ViewChoiceForce
				}
				choices <- choice
				return
			}
		}
	}()
	chose := ""
	for {
		select {
		case o := <-done:
			return o.result, chose, o.err
		case choice := <-choices:
			chose = choice
			answer := contract.ViewChangeRequest{ClientRequestID: newRequestID(), Choice: choice}
			if err := client.Do(ctx, http.MethodPost, path, answer, nil); err != nil {
				return contract.ViewChangeResult{}, chose, err
			}
		}
	}
}

var resumeCommand = &Command{
	Name: "resume", Group: GroupSessions, Args: "SESSION",
	Summary: "Resume a closed or interrupted session under its old ID.",
	Description: "Only a session with a chat view keeps an archive after it ends; webmux ls --archived lists them. The agent's " +
		"own session is loaded again, on the view the session was archived on unless --view says otherwise. Like webmux run, " +
		"a session resumed on its terminal view is attached when there is a terminal; otherwise, or with --detach, its ID " +
		"is printed. When the launch config it was started from was deleted, resuming needs --without-env (the " +
		"saved environment is gone). When the agent's session cannot be found, the candidates are listed; repeat with " +
		"--agent-session.",
	Arguments: []Item{sessionArgument},
	Env:       []Item{sessionIDEnv},
	Output:    "Attached output, or the session ID; with --json, {session}.",
	Exit: exitsCommon(
		ExitItem{Code: 1, Text: "No such session, it is still active (not_archived), its config was deleted without --without-env, or the agent session is ambiguous."},
	),
	Examples: []Example{
		{Command: "webmux resume abcd2345 --view chat --detach", Text: "Bring back yesterday's interrupted agent on its chat view."},
		{Command: "webmux resume abcd2345 --agent-session ses_42", Text: "Resume a chosen agent session after a lookup failed."},
	},
	SeeAlso: []string{"webmux ls", "webmux rm", "webmux attach"},
	Bind: func(fs *flag.FlagSet) Action {
		view := fs.String("view", "", "resume on `VIEW`: terminal or chat (default: the view it was archived on)")
		agentSession := fs.String("agent-session", "", "resume this agent session `ID` when the server lists candidates")
		withoutEnv := fs.Bool("without-env", false, "resume although its launch config (and environment) was deleted")
		detach := fs.Bool("detach", false, "print the session ID instead of attaching")
		asJSON := fs.Bool("json", false, "print {session} instead of attaching")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 1 {
				return e.Usagef("exactly one session ID is required")
			}
			request := contract.ResumeRequest{ClientRequestID: newRequestID(), AgentSessionID: *agentSession, ConfirmWithoutEnv: *withoutEnv}
			if *view != "" {
				v, ok := viewNamed(*view)
				if !ok {
					return e.Usagef("--view is terminal or chat")
				}
				request.View = v
			}
			id, code, ok := e.SessionID(args[0])
			if !ok {
				return code
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			var info contract.SessionInfo
			if err := client.Do(ctx, http.MethodGet, "/api/sessions/"+url.PathEscape(id), nil, &info); err != nil {
				return e.sessionFail(err)
			}
			to := request.View
			if to == "" {
				to = info.View
			}
			var console Console
			if to == contract.ViewTerminal && !*detach && !*asJSON && e.OpenConsole != nil {
				c, err := e.OpenConsole()
				switch {
				case errors.Is(err, attach.ErrNotTerminal):
					_, _ = fmt.Fprintln(e.Stderr, "webmux resume: no terminal; detaching")
				case err != nil:
					return e.Fail(err)
				default:
					console = c
				}
			}
			var resumed contract.SessionInfo
			if err := client.Do(ctx, http.MethodPost, "/api/sessions/"+url.PathEscape(id)+"/resume", request, &resumed); err != nil {
				if console != nil {
					_ = console.Restore()
				}
				if conflict, ok := sessionConflict(err); ok && conflict.Reason == "config_missing" {
					_, _ = fmt.Fprintf(e.Stderr, "webmux: %s (config_missing); repeat with --without-env to resume without the deleted config's environment\n", conflict.Error)
					return contract.ExitError
				}
				return e.sessionFail(err)
			}
			if console != nil {
				_, _ = fmt.Fprintf(e.Stderr, "webmux: session %s · Ctrl+] to detach\r\n", resumed.ID)
				return e.attach(ctx, client, resumed.ID, false, console)
			}
			return e.detached(ctx, client, resumed, runOptions{asJSON: *asJSON}, nil)
		}
	},
}
