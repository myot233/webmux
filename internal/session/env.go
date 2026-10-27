package session

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// Identity variables of the terminal or multiplexer that started webmux. Left
// in place they make programs target that host (for example DCS-wrapping
// output for tmux) instead of the browser terminal behind this PTY.
var hostTerminalNames = map[string]bool{
	"TERM_PROGRAM": true, "TERM_PROGRAM_VERSION": true, "TMUX": true, "TMUX_PANE": true,
	"STY": true, "LC_TERMINAL": true, "LC_TERMINAL_VERSION": true, "WT_SESSION": true,
	"WT_PROFILE_ID": true, "VTE_VERSION": true,
	// Describes how the server was started, not the session's programs.
	"WEBMUX_LAUNCHER": true,
	// Set explicitly below.
	"TERM": true, "COLORTERM": true, "WEBMUX_SESSION_ID": true,
}
var hostTerminalPrefixes = []string{"WARP_", "ZELLIJ", "KITTY_", "ITERM_", "WEZTERM_", "GHOSTTY_"}

// sessionEnv advertises the development WebMux terminal identity. Production
// callers should use sessionEnvWithVersion so TERM_PROGRAM_VERSION matches the
// running server; keeping this wrapper makes the environment helper useful to
// package tests and embedders that do not carry build metadata.
func sessionEnv(base []string, id string, agentProtocol bool, explicit map[string]string) []string {
	return sessionEnvWithVersion(base, id, agentProtocol, explicit, "dev")
}

// sessionEnvWithVersion constructs the child process environment. WebMux is a
// virtual terminal: host identities (including Kitty) are removed before the
// child starts, then the WebMux identity and xterm-compatible defaults are
// advertised. Explicit launch variables always win, including TERM and
// TERM_PROGRAM, so an application-specific preset can opt into a deliberate
// compatibility mode without being rewritten here.
func sessionEnvWithVersion(base []string, id string, agentProtocol bool, explicit map[string]string, version string) []string {
	env := make([]string, 0, len(base)+5+len(explicit))
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		if hostTerminalNames[name] || hasAnyPrefix(name, hostTerminalPrefixes) {
			continue
		}
		if _, replaced := explicit[name]; replaced {
			continue
		}
		env = append(env, entry)
	}
	for _, entry := range []string{
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"TERM_PROGRAM=webmux",
		"TERM_PROGRAM_VERSION=" + version,
		"WEBMUX_SESSION_ID=" + id,
	} {
		name, _, _ := strings.Cut(entry, "=")
		if _, replaced := explicit[name]; !replaced {
			env = append(env, entry)
		}
	}
	if _, replaced := explicit["WARP_CLI_AGENT_PROTOCOL_VERSION"]; agentProtocol && !replaced {
		env = append(env, "WARP_CLI_AGENT_PROTOCOL_VERSION=1")
	}
	for _, name := range slices.Sorted(maps.Keys(explicit)) {
		env = append(env, name+"="+explicit[name])
	}
	return env
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidateEnv accepts portable variable names and NUL-free values. The
// session marker cannot be overridden: process cleanup depends on it.
func ValidateEnv(env map[string]string) error {
	for name, value := range env {
		if !envName.MatchString(name) {
			return fmt.Errorf("invalid environment variable name %q", name)
		}
		if name == "WEBMUX_SESSION_ID" {
			return errors.New("WEBMUX_SESSION_ID is reserved")
		}
		if strings.ContainsRune(value, 0) {
			return fmt.Errorf("environment variable %s contains a NUL byte", name)
		}
	}
	return nil
}

func hasAnyPrefix(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
