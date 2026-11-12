package clicmd

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"strconv"
	"text/tabwriter"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

var forwardGroup = &Command{
	Name: "forward", Group: GroupWorkspace,
	Summary: "Forward loopback ports (dev servers) through WebMux so every device can open them.",
}

var portArgument = Item{Name: "PORT", Text: "A loopback TCP port, 1-65535."}

func parsePort(arg string) (int, bool) {
	port, err := strconv.Atoi(arg)
	return port, err == nil && port > 0 && port < 65536
}

var forwardListCommand = &Command{
	Name: "forward ls", Group: GroupWorkspace,
	Summary: "List forwarded ports with their status and source session.",
	Output:  "A table: PORT, STATUS, LABEL, SESSION. With --json, the Forward array.",
	Exit:    exitsCommon(ExitItem{Code: 1, Text: "Also: port forwarding is turned off on this server."}),
	Examples: []Example{
		{Command: "webmux forward ls", Text: "See which dev servers are reachable from other devices."},
		{Command: "webmux forward ls --json | jq -r '.[] | select(.status==\"listening\") | .port'", Text: "List the ports that answer."},
	},
	SeeAlso: []string{"webmux forward add", "webmux get"},
	Bind: func(fs *flag.FlagSet) Action {
		asJSON := fs.Bool("json", false, "print the Forward array")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 0 {
				return e.Usagef("forward ls takes no arguments")
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			var list []contract.Forward
			if err := client.Do(ctx, http.MethodGet, "/api/forwards", nil, &list); err != nil {
				return e.forwardFail(err)
			}
			if *asJSON {
				writeJSON(e.Stdout, list)
				return contract.ExitOK
			}
			w := tabwriter.NewWriter(e.Stdout, 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "PORT\tSTATUS\tLABEL\tSESSION")
			for _, f := range list {
				label, session := f.Label, f.Session
				if label == "" {
					label = "-"
				}
				if session == "" {
					session = "-"
				}
				_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", f.Port, f.Status, label, session)
			}
			_ = w.Flush()
			return contract.ExitOK
		}
	},
}

// forwardFail explains a 404 on the forward routes: forwarding is off.
func (e Env) forwardFail(err error) int {
	if isStatus(err, http.StatusNotFound) {
		if apiErr, ok := err.(*localclient.APIError); ok && apiErr.Message == "" {
			_, _ = fmt.Fprintln(e.Stderr, "webmux: port forwarding is turned off on this server")
			return contract.ExitError
		}
	}
	return e.Fail(err)
}

var forwardAddCommand = &Command{
	Name: "forward add", Group: GroupWorkspace, Args: "PORT",
	Summary: "Forward a loopback port.",
	Description: "The port appears in every browser's forward list and is served at /forward/PORT/ behind the WebMux login. " +
		"--session links it to the session that runs the server (self inside a session). The compatibility rewrites for " +
		"dev servers (Host, Origin, forwarded headers, response URLs) are on by default; --no-compat turns them all off.",
	Arguments: []Item{portArgument},
	Env:       []Item{sessionIDEnv},
	Output:    "The forwarded port; with --json, the Forward.",
	Exit:      exitsCommon(ExitItem{Code: 1, Text: "Also: the port is WebMux's own, already forwarded, or forwarding is off."}),
	Examples: []Example{
		{Command: "npm run dev & webmux forward add 5173 --session self --label web", Text: "Share a Vite dev server started by an agent."},
		{Command: "webmux forward add 8080 --no-compat", Text: "Forward a plain HTTP service without rewrites."},
	},
	SeeAlso: []string{"webmux forward ls", "webmux forward rm"},
	Bind: func(fs *flag.FlagSet) Action {
		label := fs.String("label", "", "show `TEXT` as the forward's name")
		session := fs.String("session", "", "link the forward to `SESSION` (self for the current one)")
		noCompat := fs.Bool("no-compat", false, "turn off every dev server compatibility rewrite")
		asJSON := fs.Bool("json", false, "print the Forward")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 1 {
				return e.Usagef("exactly one port is required")
			}
			port, ok := parsePort(args[0])
			if !ok {
				return e.Usagef("%q is not a port (1-65535)", args[0])
			}
			request := contract.ForwardRequest{Port: port, Label: *label}
			if *session != "" {
				id, code, ok := e.SessionID(*session)
				if !ok {
					return code
				}
				request.Session = id
			}
			if *noCompat {
				request.Options = &contract.ForwardOptions{Prefix: contract.ForwardPrefixKeep}
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			var created contract.Forward
			if err := client.Do(ctx, http.MethodPost, "/api/forwards", request, &created); err != nil {
				return e.forwardFail(err)
			}
			if *asJSON {
				writeJSON(e.Stdout, created)
			} else {
				_, _ = fmt.Fprintln(e.Stdout, created.Port)
			}
			return contract.ExitOK
		}
	},
}

var forwardRemoveCommand = &Command{
	Name: "forward rm", Group: GroupWorkspace, Args: "PORT",
	Summary:   "Stop forwarding a port; the server behind it keeps running.",
	Arguments: []Item{portArgument},
	Exit:      exitsCommon(ExitItem{Code: 1, Text: "Also: the port is not forwarded, or forwarding is off."}),
	Examples: []Example{
		{Command: "webmux forward rm 5173", Text: "Stop sharing a dev server."},
		{Command: "webmux forward ls --json | jq -r '.[].port' | xargs -n1 webmux forward rm", Text: "Remove every forward."},
	},
	SeeAlso: []string{"webmux forward ls"},
	Bind: func(fs *flag.FlagSet) Action {
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 1 {
				return e.Usagef("exactly one port is required")
			}
			port, ok := parsePort(args[0])
			if !ok {
				return e.Usagef("%q is not a port (1-65535)", args[0])
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			if err := client.Do(ctx, http.MethodDelete, "/api/forwards/"+strconv.Itoa(port), nil, nil); err != nil {
				return e.forwardFail(err)
			}
			return contract.ExitOK
		}
	},
}
