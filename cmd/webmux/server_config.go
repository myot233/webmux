package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/p2psettings"
	"github.com/rainchan/webmux/internal/pathutil"
	"github.com/rainchan/webmux/internal/tailscalesettings"
	"github.com/rainchan/webmux/internal/tunnelsettings"
	"github.com/rainchan/webmux/internal/wsl"
)

// serverRun is the validated server configuration plus how it was obtained.
type serverRun struct {
	config.Config
	ConfigPath     string
	ConfigExplicit bool
	// ConfigFound reports whether the configuration file was loaded from disk.
	ConfigFound bool
	// ConfigCreated reports whether this start created the missing default file.
	ConfigCreated bool
	LogFile       string
	// TokenGenerated marks a per-process token absent from the config.
	TokenGenerated   bool
	TunnelSettings   *tunnelsettings.Service
	TunnelWebManaged bool
	// TailscaleSettings holds the web override of [tailscale].
	TailscaleSettings   *tailscalesettings.Service
	TailscaleWebManaged bool
	// P2PSettings holds the web override of [p2p].
	P2PSettings   *p2psettings.Service
	P2PWebManaged bool
}

// flagParseError marks a flag error already reported by the flag set, so a
// caller does not print the same message twice.
type flagParseError struct{ err error }

func (e flagParseError) Error() string { return e.err.Error() }
func (e flagParseError) Unwrap() error { return e.err }

func loadConfig(args []string) (serverRun, error) {
	return parseServerRun("webmux serve", args, os.Stderr)
}

func parseServerRun(name string, args []string, stderr io.Writer) (serverRun, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	path := flags.String("config", config.DefaultPath(), "TOML configuration path")
	listen := flags.String("listen", "", "HTTP listen address")
	token := flags.String("token", "", "optional shared token")
	tunnelFlag := flags.Bool("tunnel", false, "expose webmux through a Cloudflare Tunnel")
	logFile := flags.String("log-file", "", "write stdout and stderr to this file, keeping the previous one as .1")
	if err := flags.Parse(args); err != nil {
		return serverRun{}, flagParseError{err}
	}
	if flags.NArg() != 0 {
		return serverRun{}, fmt.Errorf("unexpected argument %q; see webmux help", flags.Arg(0))
	}
	explicit := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	run := serverRun{ConfigExplicit: explicit["config"]}
	if !run.ConfigExplicit && !filepath.IsAbs(*path) {
		// An unset HOME made DefaultPath relative; never auto-write a token
		// into the current working directory (an untrusted repository).
		return run, fmt.Errorf("default configuration path %q is not absolute; set HOME or pass --config", *path)
	}
	var err error
	if run.ConfigPath, err = pathutil.Expand(*path); err != nil {
		return run, err
	}
	if *logFile != "" {
		if run.LogFile, err = pathutil.Expand(*logFile); err != nil {
			return run, err
		}
	}
	if !run.ConfigExplicit {
		// A missing default configuration is created once, then loaded like any
		// other file. An explicit --config is read-only: a missing path fails.
		run.ConfigCreated, err = ensureDefaultConfig(run.ConfigPath)
		if err != nil {
			return run, fmt.Errorf("create default configuration %s: %w", run.ConfigPath, err)
		}
	}
	_, statErr := os.Stat(run.ConfigPath)
	run.ConfigFound = statErr == nil
	// Both paths must now name a complete file: auto-creation either published
	// it or observed a concurrent winner, and an explicit path must exist.
	c, err := config.Load(run.ConfigPath, true)
	if err != nil {
		return run, err
	}
	if explicit["listen"] {
		c.Listen = *listen
		c.SetSource("listen", "cli")
	}
	if explicit["token"] {
		c.Token = *token
		c.SetSource("token", "cli")
	}
	if c.StateDir == "" {
		c.StateDir = config.DefaultStateDir()
	}
	run.TunnelSettings = tunnelsettings.Open(c.StateDir, c.Tunnel)
	var webManaged bool
	c.Tunnel, webManaged = run.TunnelSettings.Effective()
	if explicit["tunnel"] && run.TunnelSettings.Unavailable() == nil {
		c.Tunnel.Enabled = *tunnelFlag
		c.SetSource("tunnel.enabled", "cli")
	}
	run.TunnelWebManaged = webManaged && !explicit["tunnel"]
	closeSettings := func() {
		run.TunnelSettings.Close()
		run.TailscaleSettings.Close()
		run.P2PSettings.Close()
	}
	if err = c.Validate(); err != nil {
		run.TunnelSettings.Close()
		return run, err
	}
	run.TailscaleSettings = tailscalesettings.Open(c.StateDir, c.Tailscale)
	c.Tailscale, run.TailscaleWebManaged = run.TailscaleSettings.Effective()
	run.P2PSettings = p2psettings.Open(c.StateDir, c.P2P)
	c.P2P, run.P2PWebManaged = run.P2PSettings.Effective()
	if c.Tailscale.Provider == "windows" && !wsl.Detected() {
		closeSettings()
		return run, errors.New(`tailscale.provider "windows" is only valid inside WSL; use "linux" or "auto"`)
	}
	hadToken := c.Token != ""
	if err = ensureStartupToken(&c, run.TunnelWebManaged, run.TailscaleWebManaged); err != nil {
		closeSettings()
		return run, err
	}
	run.Config = c
	run.TokenGenerated = !hadToken && c.Token != ""
	return run, nil
}

var errMCPOwnerToken = errors.New("mcp.enabled requires a configured nonempty owner token")

// ensureStartupToken refuses a weak token only for TOML-enabled exposure.
// Web-managed exposure keeps local access and fails closed in its controller.
func ensureStartupToken(c *config.Config, tunnelWeb, tailscaleWeb bool) error {
	if c.MCP.Enabled && c.Token == "" {
		return errMCPOwnerToken
	}
	tomlExposure := (c.Tunnel.Enabled && !tunnelWeb) || (c.Tailscale.Enabled() && !tailscaleWeb)
	if !tomlExposure && c.Token != "" && len(c.Token) < minTunnelTokenLength {
		return nil
	}
	return ensureToken(c)
}

// ensureToken keeps every network entry point authenticated: an empty token
// is generated for this process unless the server listens only on loopback
// without a tunnel, and a tunnel token must not be weaker than that form.
func ensureToken(c *config.Config) error {
	if c.Token == "" {
		if c.Tunnel.Enabled || c.Tailscale.Enabled() || !loopbackListen(c.Listen) {
			c.Token = generateToken()
			c.SetSource("token", "generated")
		}
		return nil
	}
	if (c.Tunnel.Enabled || c.Tailscale.Enabled()) && len(c.Token) < minTunnelTokenLength {
		return fmt.Errorf("tunnel and Tailscale Serve/Funnel require the webmux token to be at least %d characters; leave it empty to generate one, or create one with openssl rand -base64 32", minTunnelTokenLength)
	}
	return nil
}
