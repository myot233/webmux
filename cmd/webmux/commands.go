package main

import (
	"flag"

	"github.com/rainchan/webmux/internal/clicmd"
	"github.com/rainchan/webmux/internal/client"
	"github.com/rainchan/webmux/internal/config"
)

// The operations commands keep their own parsers; these entries are their
// help. TestOpsCommandFlagsMatchTable checks that each Bind declares exactly
// the flags its implementation accepts.

var configOption = func(fs *flag.FlagSet) {
	fs.String("config", config.DefaultPath(), "TOML configuration `PATH`")
}

func serverFlags(fs *flag.FlagSet) clicmd.Action {
	configOption(fs)
	fs.String("listen", "", "HTTP listen `HOST:PORT`, overriding the configuration")
	fs.String("token", "", "shared access `TOKEN`, overriding the configuration")
	fs.Bool("tunnel", false, "expose WebMux through a Cloudflare Tunnel")
	fs.String("log-file", "", "write stdout and stderr to this `PATH`, keeping the previous one as .1")
	return nil
}

func configFlagsOnly(fs *flag.FlagSet) clicmd.Action { configOption(fs); return nil }

func configJSON(text string) func(fs *flag.FlagSet) clicmd.Action {
	return func(fs *flag.FlagSet) clicmd.Action {
		configOption(fs)
		fs.Bool("json", false, text)
		return nil
	}
}

var (
	exitServer = []clicmd.ExitItem{
		{Code: 0, Text: "Stopped cleanly or --help."},
		{Code: 1, Text: "Configuration or startup failed."},
	}
	exitClient = []clicmd.ExitItem{
		{Code: 0, Text: "Done."},
		{Code: 1, Text: "Failed."},
		{Code: 2, Text: "Usage error."},
	}
	configEnv = []clicmd.Item{{Name: "HOME", Text: "Locates the default configuration path ~/.config/webmux/config.toml."}}
)

func opsCommands() clicmd.Registry {
	g := clicmd.GroupServer
	return clicmd.Registry{
		{
			Name: "serve", Group: g, NoGlobal: true, Bind: serverFlags,
			Summary: "Run the server in the foreground (the default without a command).",
			Description: "A missing default configuration file is created with a generated token. " +
				"The server stops on SIGINT or SIGTERM, which ends every terminal session.",
			Env:    configEnv,
			Output: "Log lines and the access links on standard output.",
			Exit:   exitServer,
			Examples: []clicmd.Example{
				{Command: "webmux serve", Text: "Serve with ~/.config/webmux/config.toml."},
				{Command: "webmux serve --listen 127.0.0.1:8080 --log-file ~/webmux.log", Text: "Serve on another port and log to a file."},
			},
			SeeAlso: []string{"webmux start", "webmux status", "webmux service install"},
		},
		{
			Name: "start", Group: g, NoGlobal: true, Bind: serverFlags,
			Summary: "Run the server in the background; stop it with webmux stop.",
			Description: "The child is a normal serve process in its own session with a log file. " +
				"When an instance already runs, start prints its status instead.",
			Env:    configEnv,
			Output: "The new instance's status, as webmux status prints it.",
			Exit:   exitClient,
			Examples: []clicmd.Example{
				{Command: "webmux start", Text: "Start a background server with the default configuration."},
				{Command: "webmux start --config ~/work/webmux.toml", Text: "Start an instance with another configuration."},
			},
			SeeAlso: []string{"webmux stop", "webmux status"},
		},
		{
			Name: "version", Group: g, NoGlobal: true,
			Summary: "Print the version; --json adds the control contract.",
			Bind: func(fs *flag.FlagSet) clicmd.Action {
				fs.Bool("json", false, "print the version and control contract as JSON")
				return nil
			},
			Output: "webmux VERSION, or the VersionV1 object with --json.",
			Exit:   exitClient,
			Examples: []clicmd.Example{
				{Command: "webmux version", Text: "Print the version."},
				{Command: `"$WEBMUX_BIN" version --json`, Text: "Check the version of the executable a session's server runs."},
			},
			SeeAlso: []string{"webmux status"},
		},
		{
			Name: "status", Group: g, NoGlobal: true,
			Summary: "Show server health, sessions and access links.",
			Bind: func(fs *flag.FlagSet) clicmd.Action {
				configOption(fs)
				fs.Bool("json", false, "print the status contract v1 as JSON")
				fs.Bool("reveal-token", false, "include the token in the JSON output")
				return nil
			},
			Env:    configEnv,
			Output: "A human-readable summary, or the StatusV1 object with --json.",
			Exit: []clicmd.ExitItem{
				{Code: 0, Text: "Running and healthy."},
				{Code: 1, Text: "Running but unhealthy, or the configuration is invalid."},
				{Code: 3, Text: "Not running."},
			},
			Examples: []clicmd.Example{
				{Command: "webmux status", Text: "Show whether WebMux runs and its links."},
				{Command: "webmux status --json | jq -r .urls.local", Text: "Print the local link for a script."},
			},
			SeeAlso: []string{"webmux start", "webmux stop", "webmux open"},
		},
		{
			Name: "stop", Group: g, NoGlobal: true,
			Summary: "Stop the server through its local control API (ends all terminal sessions).",
			Description: "A server managed by systemd or launchd is stopped only with --force; use webmux service stop instead. " +
				"Processes inside a WebMux session cannot stop the server.",
			Bind: func(fs *flag.FlagSet) clicmd.Action {
				configOption(fs)
				fs.Bool("json", false, "print the stop result as JSON")
				fs.Bool("force", false, "stop an instance managed by systemd or launchd")
				fs.Duration("timeout", client.DefaultStopTimeout, "wait this `DURATION` before SIGKILL")
				return nil
			},
			Env:    configEnv,
			Output: "One line describing the result, or the StopV1 object with --json.",
			Exit:   []clicmd.ExitItem{{Code: 0, Text: "Stopped or not running."}, {Code: 1, Text: "Refused or failed."}, {Code: 2, Text: "Usage error."}},
			Examples: []clicmd.Example{
				{Command: "webmux stop", Text: "Stop a server started with webmux start."},
				{Command: "webmux stop --timeout 30s --json", Text: "Allow sessions more time to exit and print the result as JSON."},
			},
			SeeAlso: []string{"webmux start", "webmux service stop"},
		},
		{
			Name: "bridge", Group: g, NoGlobal: true, Bind: configFlagsOnly,
			Summary: "Bridge the local API socket over standard input and output.",
			Description: "The Windows console client and SSH use it to reach the local API of a remote or WSL instance: " +
				"ssh host webmux bridge carries every session command without a token.",
			Env:    configEnv,
			Output: "The raw local API byte stream.",
			Exit:   []clicmd.ExitItem{{Code: 0, Text: "The connection ended."}, {Code: 1, Text: "Failed."}, {Code: 3, Text: "Not running."}},
			Examples: []clicmd.Example{
				{Command: "webmux bridge", Text: "Serve one local API connection over stdio."},
				{Command: "webmux bridge --config ~/work/webmux.toml", Text: "Bridge the instance of another configuration."},
			},
			SeeAlso: []string{"webmux status"},
		},
		{
			Name: "mcp-serve", Group: g, NoGlobal: true,
			Summary: "Serve scoped read-only handoff history over MCP stdio.",
			Bind: func(fs *flag.FlagSet) clicmd.Action {
				fs.String("socket", "", "local API socket `PATH`")
				fs.String("conversation", "", "fixed source Conversation `ID`")
				fs.String("handoff", "", "fixed handoff `ID`")
				return nil
			},
			Output: "MCP JSON-RPC only; diagnostics go to stderr.", Exit: exitClient,
			Description: "Started by a handoff target provider. Scope is fixed by argv; no file paths or SQL are accepted as tool input.",
			Examples:    []clicmd.Example{{Command: "webmux mcp-serve --socket /path/webmux.sock --conversation ID --handoff ID", Text: "Serve a fixed handoff source through the local API."}, {Command: "webmux help mcp-serve", Text: "Show the fixed scope options."}},
			SeeAlso:     []string{"webmux bridge"},
		},
		{Name: "config", Group: g, Summary: "Initialize, locate, show or validate configuration."},
		{
			Name: "config path", Group: g, NoGlobal: true, Bind: configJSON("print JSON"),
			Summary: "Print the configuration file path and whether it exists.",
			Env:     configEnv, Output: "The path, or a JSON object with --json.", Exit: exitClient,
			Examples: []clicmd.Example{
				{Command: "webmux config path", Text: "Print the default configuration path."},
				{Command: "webmux config path --json", Text: "Print the path and whether it exists as JSON."},
			},
			SeeAlso: []string{"webmux config show"},
		},
		{
			Name: "config show", Group: g, NoGlobal: true,
			Summary: "Show the effective configuration without secrets.",
			Bind: func(fs *flag.FlagSet) clicmd.Action {
				configOption(fs)
				fs.Bool("json", false, "print the safe configuration view as JSON")
				fs.Bool("live", false, "read the running process instead of disk")
				return nil
			},
			Env: configEnv, Output: "The effective settings and where each came from.", Exit: exitClient,
			Examples: []clicmd.Example{
				{Command: "webmux config show", Text: "Show the configuration on disk."},
				{Command: "webmux config show --live --json", Text: "Show what the running server uses, as JSON."},
			},
			SeeAlso: []string{"webmux config validate"},
		},
		{
			Name: "config validate", Group: g, NoGlobal: true, Bind: configJSON("print JSON"),
			Summary: "Validate the configuration file (alias: config check).",
			Env:     configEnv, Output: "OK or the problems found.", Exit: exitClient,
			Examples: []clicmd.Example{
				{Command: "webmux config validate", Text: "Check the default configuration."},
				{Command: "webmux config check --config ./webmux.toml --json", Text: "Check another file and print JSON."},
			},
			SeeAlso: []string{"webmux config show"},
		},
		{
			Name: "config check", Group: g, NoGlobal: true, Hidden: true, Bind: configJSON("print JSON"),
			Summary: "Alias of config validate.",
		},
		{
			Name: "config init", Group: g, NoGlobal: true,
			Summary: "Write a new configuration file.",
			Bind: func(fs *flag.FlagSet) clicmd.Action {
				configOption(fs)
				fs.String("listen", "", "listen address to write")
				fs.Bool("force", false, "keep an existing file as .bak and replace it")
				return nil
			},
			Env: configEnv, Output: "The written path.", Exit: exitClient,
			Examples: []clicmd.Example{
				{Command: "webmux config init", Text: "Create the default configuration."},
				{Command: "webmux config init --listen 0.0.0.0:7681 --force", Text: "Replace it, listening on every interface."},
			},
			SeeAlso: []string{"webmux config listen"},
		},
		{
			Name: "config listen", Group: g, NoGlobal: true, Args: "[ADDR]",
			Summary: "Change the listen address in the configuration file.",
			Bind: func(fs *flag.FlagSet) clicmd.Action {
				configOption(fs)
				fs.String("listen", "", "listen address to write")
				return nil
			},
			Arguments: []clicmd.Item{{Name: "ADDR", Text: "The HOST:PORT to listen on; the same as --listen."}},
			Env:       configEnv, Output: "The written path.", Exit: exitClient,
			Examples: []clicmd.Example{
				{Command: "webmux config listen 127.0.0.1:8080", Text: "Move the server to port 8080 after the next restart."},
				{Command: "webmux config listen --listen 0.0.0.0:7681", Text: "Listen on every interface."},
			},
			SeeAlso: []string{"webmux config init"},
		},
		{Name: "service", Group: g, Platform: "unix", Summary: "Install and control the user service (systemd --user or a launchd LaunchAgent)."},
		{
			Name: "service install", Group: g, Platform: "unix", NoGlobal: true,
			Summary: "Install and start a service for the current user.",
			Bind: func(fs *flag.FlagSet) clicmd.Action {
				fs.String("config", config.DefaultPath(), "TOML configuration path (default is created if missing)")
				fs.Bool("force", false, "replace the service definition and restart (ends terminal sessions)")
				fs.Bool("dry-run", false, "print the service definition without changing anything")
				return nil
			},
			Env: configEnv, Output: "Progress lines, or the definition with --dry-run.", Exit: exitClient,
			Examples: []clicmd.Example{
				{Command: "webmux service install", Text: "Install and start the user service."},
				{Command: "webmux service install --dry-run", Text: "Show the unit or plist without installing it."},
			},
			SeeAlso: []string{"webmux service status", "webmux service uninstall"},
		},
		serviceAction("uninstall", "Stop and remove the user service; keep configuration."),
		serviceAction("start", "Start the installed user service."),
		serviceAction("stop", "Stop the user service (ends all terminal sessions)."),
		serviceAction("restart", "Restart the user service (ends all terminal sessions)."),
		serviceAction("status", "Show the user service manager's status."),
		{
			Name: "service logs", Group: g, Platform: "unix", NoGlobal: true,
			Summary: "Show recent service logs; --follow streams them.",
			Bind: func(fs *flag.FlagSet) clicmd.Action {
				fs.Bool("follow", false, "stream log output until interrupted")
				fs.Bool("f", false, "stream log output until interrupted")
				return nil
			},
			Notes:  map[string]clicmd.Note{"f": {AliasOf: "follow"}},
			Output: "The service manager's log output.", Exit: exitClient,
			Examples: []clicmd.Example{
				{Command: "webmux service logs", Text: "Show recent logs."},
				{Command: "webmux service logs -f", Text: "Stream logs until Ctrl+C."},
			},
			SeeAlso: []string{"webmux service status"},
		},
		{Name: "update", Group: g, Summary: "Check for and install signed releases."},
		{
			Name: "update check", Group: g, NoGlobal: true,
			Summary: "Check for a newer signed release.",
			Bind: func(fs *flag.FlagSet) clicmd.Action {
				fs.Bool("json", false, "print the result as JSON")
				return nil
			},
			Output: "Whether an update is available, or the UpdateCheckV1 object with --json.", Exit: exitClient,
			Examples: []clicmd.Example{
				{Command: "webmux update check", Text: "Check the configured channel."},
				{Command: "webmux update check --json | jq .available", Text: "Check from a script."},
			},
			SeeAlso: []string{"webmux update apply", "webmux update channel"},
		},
		{
			Name: "update apply", Group: g, NoGlobal: true, Bind: func(*flag.FlagSet) clicmd.Action { return nil },
			Summary: "Install a newer deb or rpm release (root; used by webmux-update.timer).",
			Output:  "Progress lines.", Exit: exitClient,
			Examples: []clicmd.Example{
				{Command: "sudo webmux update apply", Text: "Install the latest release now."},
				{Command: "systemctl start webmux-update.service", Text: "Let the update service run it."},
			},
			SeeAlso: []string{"webmux update check"},
		},
		{
			Name: "update channel", Group: g, NoGlobal: true, Args: "[stable|dev]", Bind: func(*flag.FlagSet) clicmd.Action { return nil },
			Summary:   "Show the update channel, or set it to stable or dev (root).",
			Arguments: []clicmd.Item{{Name: "stable|dev", Text: "The channel to switch to; omit it to print the current one."}},
			Output:    "The channel.", Exit: exitClient,
			Examples: []clicmd.Example{
				{Command: "webmux update channel", Text: "Print the current channel."},
				{Command: "sudo webmux update channel dev", Text: "Follow development builds."},
			},
			SeeAlso: []string{"webmux update check"},
		},
		{
			Name: "help", Group: clicmd.GroupHelp, NoGlobal: true, Args: "[COMMAND...]",
			Summary: "Show help for all commands or one command; --json prints the command tree.",
			Bind: func(fs *flag.FlagSet) clicmd.Action {
				fs.Bool("json", false, "print the commands, options and examples as JSON")
				return nil
			},
			Arguments: []clicmd.Item{{Name: "COMMAND...", Text: "A command path such as ls or update check."}},
			Output:    "Help text, or the HelpTree object with --json.",
			Exit:      []clicmd.ExitItem{{Code: 0, Text: "Printed."}, {Code: 2, Text: "Unknown command."}},
			Examples: []clicmd.Example{
				{Command: "webmux help get", Text: "Show the help of webmux get."},
				{Command: "webmux help --json | jq -r '.commands[].path | join(\" \")'", Text: "List every command path."},
			},
			SeeAlso: []string{"webmux get", "webmux ls", "webmux guide"},
		},
	}
}

func serviceAction(action, summary string) *clicmd.Command {
	return &clicmd.Command{
		Name: "service " + action, Group: clicmd.GroupServer, Platform: "unix", NoGlobal: true,
		Bind:    func(*flag.FlagSet) clicmd.Action { return nil },
		Summary: summary, Output: "The service manager's output.", Exit: exitClient,
		Examples: []clicmd.Example{
			{Command: "webmux service " + action, Text: summary},
			{Command: "webmux service " + action + " && webmux status", Text: "Then show the server status."},
		},
		SeeAlso: []string{"webmux service status", "webmux status"},
	}
}

// commandRegistry is every command this client offers.
func commandRegistry() clicmd.Registry {
	return append(opsCommands(), clicmd.Commands()...)
}
