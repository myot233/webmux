package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/rainchan/webmux/internal/agentguide"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rainchan/webmux/cmd/internal/assemble"
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/api"
	"github.com/rainchan/webmux/internal/artifacts"
	"github.com/rainchan/webmux/internal/clicmd"
	"github.com/rainchan/webmux/internal/codeview"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/files"
	"github.com/rainchan/webmux/internal/forward"
	"github.com/rainchan/webmux/internal/handoff"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/p2p"
	"github.com/rainchan/webmux/internal/p2psettings"
	"github.com/rainchan/webmux/internal/push"
	"github.com/rainchan/webmux/internal/runtime"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/settings"
	"github.com/rainchan/webmux/internal/tailscale"
	"github.com/rainchan/webmux/internal/tailscalesettings"
	"github.com/rainchan/webmux/internal/tunnel"
	"github.com/rainchan/webmux/internal/tunnelsettings"
	"github.com/rainchan/webmux/internal/update"
	"github.com/rainchan/webmux/internal/voice"
	"github.com/rainchan/webmux/internal/voicesettings"
	"github.com/rainchan/webmux/internal/workspace"
	"github.com/rainchan/webmux/internal/wsl"
)

var version = "dev"

// serverExecutable is the running executable as os.Executable reports it,
// or "" with a warning when it cannot be determined. On Linux it reads
// /proc/self/exe, which already names the real file and drops the
// " (deleted)" suffix an in-place upgrade leaves.
var serverExecutable = sync.OnceValue(func() string {
	path, err := os.Executable()
	if err != nil {
		slog.Warn("executable path unavailable; sessions get no WEBMUX_BIN", "error", err)
		return ""
	}
	return path
})

func main() {
	os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
}

// serverHelpFooter ends the top-level help. The Windows console client
// looks for "-log-file" in `webmux -h` to tell a webmux with the local API
// from an older one, so it must stay.
const serverHelpFooter = "Without a command, webmux runs serve. Server options: --config PATH, --listen HOST:PORT, " +
	"--token TOKEN, --tunnel, --log-file PATH. Services: Linux uses systemd --user and macOS a launchd LaunchAgent; " +
	"do not use sudo for them. Only \"update apply\" and \"update channel <stable|dev>\" run as root."

// dispatch routes by the first argument. No argument or a flag first keeps
// the original server invocation used by existing service templates.
func dispatch(args []string, stdout, stderr io.Writer) int {
	registry := commandRegistry()
	e := sessionEnv(stdout, stderr)
	e.Commands = registry
	e.HelpFooter = serverHelpFooter
	if len(args) > 0 {
		switch args[0] {
		case "help":
			return e.HelpCommand(registry, version, args[1:])
		case "--help", "-help", "-h":
			return e.HelpCommand(registry, version, nil)
		case "--version":
			_, _ = fmt.Fprintln(stdout, "webmux", version)
			return contract.ExitOK
		}
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runServe(args)
	}
	// The operations commands parse their own flags; help comes from the
	// command table for all of them.
	if c, rest := registry.Find(args); c != nil && isOps(c.Name) {
		if c.Bind == nil && len(rest) == 0 {
			return e.Run(context.Background(), c, nil)
		}
		if helpRequested(rest) && (c.Bind != nil || len(rest) == 1) {
			e.PrintHelp(c)
			return contract.ExitOK
		}
	}
	switch args[0] {
	case "serve":
		return runServe(args[1:])
	case "start":
		return startCommand(args[1:], stdout, stderr)
	case "version":
		return versionCommand(args[1:], stdout, stderr)
	case "status":
		return statusCommand(args[1:], stdout, stderr)
	case "stop":
		return stopCommand(args[1:], stdout, stderr)
	case "mcp-serve":
		return mcpCommand(args[1:], os.Stdin, stdout, stderr)
	case "bridge":
		return bridgeCommand(args[1:], os.Stdin, stdout, stderr)
	case "config":
		return configCommand(args[1:], stdout, stderr)
	case "service":
		return serviceCommand(args[1:], stdout, stderr)
	case "update":
		return updateCommand(args[1:], stdout, stderr)
	}
	if code, ok := sessionCommand(args, stdout, stderr); ok {
		return code
	}
	_, _ = fmt.Fprintf(stderr, "webmux: unknown command %q\nRun 'webmux help' for the list of commands.\n", args[0])
	return contract.ExitUsage
}

// isOps reports whether command name belongs to the operations commands,
// which keep their own parsers.
func isOps(name string) bool {
	first, _, _ := strings.Cut(name, " ")
	switch first {
	case "serve", "start", "version", "status", "stop", "bridge", "mcp-serve", "config", "service", "update", "help":
		return true
	}
	return false
}

// helpRequested reports whether args ask for help before any "--".
func helpRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "-h" || arg == "--help" || arg == "-help" {
			return true
		}
	}
	return false
}

func runServe(args []string) int {
	if err := run(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return contract.ExitOK
		}
		slog.Error("webmux failed", "error", err)
		return contract.ExitError
	}
	return contract.ExitOK
}

func run(args []string) error {
	c, err := loadConfig(args)
	if err != nil {
		return err
	}
	defer c.TunnelSettings.Close()
	defer c.TailscaleSettings.Close()
	defer c.P2PSettings.Close()
	if c.LogFile != "" {
		if err := redirectOutput(c.LogFile); err != nil {
			return fmt.Errorf("log file: %w", err)
		}
	}
	applyLaunchEnv()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serve(ctx, c, os.Stdout)
}

// serve runs until ctx ends. c must come from loadConfig, which generates or
// vets the token. Access URLs go to out, never to slog, so tokens stay out of
// structured logs.
func serve(ctx context.Context, c serverRun, out io.Writer) error {
	// Direct embedders/tests supply validated config rather than loadConfig.
	if c.TunnelSettings == nil {
		c.TunnelSettings = tunnelsettings.Open(c.StateDir, c.Tunnel)
		c.Tunnel, c.TunnelWebManaged = c.TunnelSettings.Effective()
	}
	defer c.TunnelSettings.Close()
	if err := c.TunnelSettings.Unavailable(); err != nil {
		c.Tunnel.Enabled = false
		slog.Warn("tunnel settings unavailable; public tunnel remains off", "error", err)
	}
	if c.TailscaleSettings == nil {
		c.TailscaleSettings = tailscalesettings.Open(c.StateDir, c.Tailscale)
		c.Tailscale, c.TailscaleWebManaged = c.TailscaleSettings.Effective()
	}
	defer c.TailscaleSettings.Close()
	if err := c.TailscaleSettings.Unavailable(); err != nil {
		slog.Warn("tailscale settings unavailable; managed Tailscale entry remains off", "error", err)
	}
	if c.P2PSettings == nil {
		c.P2PSettings = p2psettings.Open(c.StateDir, c.P2P)
		c.P2P, c.P2PWebManaged = c.P2PSettings.Effective()
	}
	defer c.P2PSettings.Close()
	if err := c.P2PSettings.Unavailable(); err != nil {
		slog.Warn("p2p settings unavailable; opportunistic P2P remains off", "error", err)
	}
	locator := tunnel.Locator{Explicit: c.Tunnel.Cloudflared, StateDir: c.StateDir, Mirrors: c.Tunnel.DownloadMirrors}
	var cloudflared string
	if c.Tunnel.Enabled && !c.TunnelWebManaged {
		path, err := locator.Find(ctx)
		if err != nil {
			return fmt.Errorf("cloudflare tunnel: %w", err)
		}
		cloudflared = path
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// The public origin registry always exists: it authorizes no host until
	// an entry point reports ready, so a server without one behaves as before.
	public := &api.PublicOrigins{}
	tsService := tailscale.New(assemble.TailscaleOptions(c.Config))
	tsCtl := newTailscaleController(tailscaleControllerOptions{
		Cfg: c.Tailscale, Store: c.TailscaleSettings, Service: tsService, StateDir: c.StateDir,
		Public: public, Token: c.Token, TokenGenerated: c.TokenGenerated, Out: out,
		Logger: slog.Default(), IsWSL: wsl.Detected, NetworkingMode: wsl.NetworkingMode,
	})
	defer tsCtl.Close()
	if err := tsCtl.Prepare(ctx, c.Listen); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return err
	}
	sessionOptions := assemble.SessionOptions(c.Config)
	// Keep the child terminal identity tied to the running WebMux build. The
	// session package still defaults to "dev" for embedders and tests.
	sessionOptions.TerminalVersion = version
	// Sessions find this very executable in WEBMUX_BIN, whatever PATH holds.
	sessionOptions.Executable = serverExecutable()
	// A configured port 0 is chosen when binding: exclude the bound port.
	if addr, ok := listener.Addr().(*net.TCPAddr); ok && !slices.Contains(sessionOptions.ExcludePorts, addr.Port) {
		sessionOptions.ExcludePorts = append(sessionOptions.ExcludePorts, addr.Port)
	}
	m := session.NewManager(sessionOptions)
	// Opening never fails: a locked, corrupt or unreadable store only
	// disables launch configs, and the endpoints answer 503 with the reason.
	launches := launch.Open(c.StateDir)
	defer launches.Close()
	if err := launches.Unavailable(); err != nil {
		slog.Warn("launch configs unavailable", "error", err)
	} else if err := launches.Seed(launch.Presets(os.Getenv("SHELL"))); err != nil {
		slog.Warn("seed default launch configs", "error", err)
	}
	preferences := settings.Open(c.StateDir)
	defer preferences.Close()
	// Session artifacts folders exist before hybrid sessions, which accept
	// only a session's own uploads as chat attachments.
	folders := artifacts.New(artifacts.Options{StateDir: c.StateDir, Manager: m, Logger: slog.Default()})
	defer folders.Close()
	// Like launch configs, a broken hybrid record store only disables
	// hybrid sessions; New logs why.
	conversations := conversation.Open(conversation.Options{StateDir: c.StateDir, DisableCapture: !c.HistoryCapture})
	defer func() {
		if err := conversations.Close(); err != nil {
			slog.Warn("conversation database close failed")
		}
	}()
	hybrids := hybrid.New(hybrid.Options{
		Conversations: conversations,
		HistoryReader: historyReader(c.StateDir, serverExecutable(), conversations),
		StateDir:      c.StateDir, Manager: m, ScrollbackBytes: sessionOptions.ScrollbackBytes,
		Version: version, Logger: slog.Default(), Uploads: folders.UploadPath,
		NextPromptCmd: c.NextPrompt.Cmd,
	})
	if err := hybrids.Unavailable(); err != nil {
		slog.Warn("hybrid records unreadable; session artifacts folders kept", "error", err)
	}
	// A deleted launch config's remembered agent options go with it.
	launches.OnDelete(func(id string) {
		if err := hybrids.ForgetAgentOptions(id); err != nil {
			slog.Warn("forget agent options of a deleted launch config", "error", err)
		}
	})
	// A recoverable hybrid session keeps its folder after leaving the
	// manager; an unreadable record store keeps every folder.
	folders.Start(func(id string) bool {
		_, err := hybrids.Record(id)
		return !errors.Is(err, hybrid.ErrNotFound)
	})
	// The shell guide and the {{webmux}} brief; the socket is added once the
	// local API listens.
	// The commands topic is the command directory, rendered once from this
	// build's command table.
	guide := &agentguide.Instance{
		Guide:   agentguide.New(clicmd.Directory(commandRegistry())),
		Version: version, LocalURL: localOrigin(listener.Addr()), Bin: serverExecutable(),
	}
	workspaces := workspace.New(c.StateDir, m, launches, c.DefaultCwd)
	workspaces.UseBrief(func(id, dir string) string {
		return guide.Brief(agentguide.BriefInput{SessionID: id, Dir: dir})
	})
	workspaces.UseHybrid(hybrids)
	workspaces.UseArtifacts(folders)
	// An archived record auto-evicted for a new session takes its artifacts
	// folder and private release ref with it, like forgetting it.
	hybrids.OnEvicted(func(ctx context.Context, rec hybrid.Record) {
		if rec.Released != nil {
			if err := workspaces.DropRelease(ctx, *rec.Released); err != nil {
				slog.Warn("released ref removal of an evicted record failed", "release", rec.Released.ID, "error", err)
			}
		}
		folders.Remove(rec.ID)
	})
	// Initialization instructions reference the saved quick commands by name.
	workspaces.UseQuickCommands(func() []workspace.QuickCommand {
		snapshot, err := preferences.Snapshot()
		if err != nil || snapshot.Values.QuickCommands == nil {
			return nil
		}
		list := make([]workspace.QuickCommand, 0, len(*snapshot.Values.QuickCommands))
		for _, c := range *snapshot.Values.QuickCommands {
			list = append(list, workspace.QuickCommand{Name: c.Name, Text: c.Command})
		}
		return list
	})
	defer workspaces.Close()
	codes := codeview.New(m, folders)
	defer codes.Close()
	pushes := push.New(c.Push, c.StateDir, m, slog.Default(), nil)
	// Forwards live in memory only; WebMux's own ports cannot be forwarded.
	forwards := forward.New(forward.Config{ExcludePorts: sessionOptions.ExcludePorts})
	defer forwards.Close()
	local := localOrigin(listener.Addr())
	tunnels := newTunnelController(tunnelControllerOptions{
		Cfg: c.Tunnel, Store: c.TunnelSettings, Local: local, Token: c.Token,
		Public: public, Locate: locator.Find, Out: out, Logger: slog.Default(),
	})
	tunnels.defaultEnabledSource = c.BaseSource("tunnel.enabled")
	tunnels.enabledSource = c.Source("tunnel.enabled")
	if c.TunnelWebManaged {
		tunnels.enabledSource = "web"
	}
	// The web override of [voice] wins over config.toml; a damaged store
	// turns recognition off rather than guessing.
	voiceStore := voicesettings.Open(c.StateDir, c.Voice)
	defer voiceStore.Close()
	if err := voiceStore.Unavailable(); err != nil {
		slog.Warn("voice settings unavailable; speech recognition stays off", "error", err)
	}
	voiceConfig := c.Config
	voiceConfig.Voice, _ = voiceStore.Effective()
	voiceService := voice.New(assemble.VoiceOptions(voiceConfig))
	var bridge api.Bridge
	p2pService := p2p.New(assemble.P2POptions(
		c.Config,
		version,
		func(sctx context.Context, path string, conn p2p.StreamConn) {
			if bridge.ServeStream != nil {
				bridge.ServeStream(sctx, path, conn)
			}
		},
		func(rctx context.Context, req p2p.RPCRequest) p2p.RPCResponse {
			if bridge.ServeRPC != nil {
				return bridge.ServeRPC(rctx, req)
			}
			return p2p.RPCResponse{Status: http.StatusServiceUnavailable, StatusText: http.StatusText(http.StatusServiceUnavailable)}
		},
	))
	defer p2pService.Close()
	p2pCtl := newP2PController(c.P2PSettings, p2pService)
	handoffs := handoff.New(handoff.Options{Workspace: workspaces, Conversations: conversations, Launch: launches, Artifacts: folders, Hybrid: hybrids, TokenCap: int64(c.HandoffTokenCap), MaxPromptBytes: hybrid.MaxPromptBytes, MaxPromptWireBytes: hybrid.MaxPromptWireBytes})
	defer handoffs.Close()
	deps := api.Deps{
		Handoffs: handoffs,
		Config:   c.Config, Manager: m, Workspace: workspaces, Code: codes,
		Conversations: conversations, Files: files.New(c.DefaultCwd), Artifacts: folders, Push: pushes, Launch: launches, Settings: preferences, Hybrid: hybrids,
		Forward:       forwards,
		Voice:         voiceService,
		VoiceSettings: newVoiceController(voiceStore, voiceService, voiceConfig),
		P2P:           p2pCtl,
		DefaultShell:  agentprofile.DefaultShell(os.Getenv("SHELL")),
		// The executable's identity is recorded now; later status and
		// configuration requests compare against it.
		InstalledVersion: update.NewInstalledVersion(version).Pending,
		LocalURL:         localOrigin(listener.Addr()),
		Guide:            guide,
		Launcher:         runtime.Launcher(os.Getenv),
	}
	deps.Public, deps.Tunnel, deps.TunnelSettings = public, tunnels.Status, tunnels
	deps.Tailscale = tsCtl
	bridge = api.NewBridge(deps)
	// Remote-access systems report their links through one interface; a new
	// forwarding system only needs to be appended here.
	rt := startRuntime(ctx, c, listener.Addr(), deps, cancel, tunnels, tsCtl, []accessProvider{tunnels, tsCtl})
	server := &http.Server{
		Handler:           bridge.HTTPHandler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       60 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	ended := make(chan error, 1)
	go func() { ended <- server.Serve(listener) }()
	rt.serve()
	slog.Info("webmux listening", "address", listener.Addr(), "version", version)
	// Sessions the previous server left running come back on their own;
	// only a close by the user archives a session.
	resumed := make(chan struct{})
	go func() {
		defer close(resumed)
		resumeRestarted(ctx, workspaces, hybrids.Restarted())
	}()
	printConfigNotice(out, c)
	_, _ = fmt.Fprintln(out, "WebMux local:  "+local)
	if c.Tunnel.Enabled {
		tunnels.startLocated(cloudflared)
	}
	tsCtl.Start(ctx, local)
	select {
	case <-ctx.Done():
	case err = <-ended:
	}
	cancel() // Cancel HTTP and hijacked WebSocket handlers before joining sessions.
	<-resumed
	tunnels.Close()
	tsCtl.Close()
	p2pService.Close()
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	pushes.Close() // Stop sending before sessions exit.
	// Hybrid sessions stop their ACP processes first; their records stay
	// active, so the next start marks them interrupted.
	hybrids.Close()
	m.Shutdown(shutdown)
	if shutdownErr := server.Shutdown(shutdown); shutdownErr != nil {
		_ = server.Close()
		slog.Warn("forced HTTP shutdown", "error", shutdownErr)
	}
	rt.close(shutdown)
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// restartResumeTimeout bounds resuming one session after a restart.
const restartResumeTimeout = time.Minute

// resumeRestarted resumes the hybrid sessions ids, one at a time, on their
// recorded side. One that cannot come back (its worktree was released, its
// launch config or agent session is gone) stays archived and is logged.
func resumeRestarted(ctx context.Context, w *workspace.Service, ids []string) {
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		rctx, cancel := context.WithTimeout(ctx, restartResumeTimeout)
		_, err := w.ResumeHybrid(rctx, id, workspace.HybridResumeInput{ClientRequestID: "restart-" + id})
		cancel()
		if err != nil {
			slog.Warn("session not resumed after restart; it stays archived", "id", id, "error", err)
			continue
		}
		slog.Info("session resumed after restart", "id", id)
	}
}

// printConfigNotice names the configuration in use and points at the local
// API for the current token without putting credentials in captured output.
func printConfigNotice(out io.Writer, c serverRun) {
	if c.Token != "" {
		_, _ = fmt.Fprintln(out, "Token: reveal it with: webmux status --json --reveal-token")
	}
	if c.ConfigPath == "" {
		return
	}
	if c.ConfigCreated {
		_, _ = fmt.Fprintln(out, "Created config:  "+c.ConfigPath)
	} else {
		_, _ = fmt.Fprintln(out, "Using config:    "+c.ConfigPath)
	}
}
