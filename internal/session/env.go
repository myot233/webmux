package session

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/rainchan/webmux/internal/contract"
)

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
		if contract.ServerOwnedEnv(name) {
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
		contract.SessionIDEnv + "=" + id,
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

// ValidateEnv accepts portable variable names and NUL-free values. The
// session marker cannot be overridden: process cleanup depends on it.
func ValidateEnv(env map[string]string) error {
	for name, value := range env {
		if !contract.ValidEnvName(name) {
			return fmt.Errorf("invalid environment variable name %q", name)
		}
		if name == contract.SessionIDEnv {
			return errors.New(contract.SessionIDEnv + " is reserved")
		}
		if strings.ContainsRune(value, 0) {
			return fmt.Errorf("environment variable %s contains a NUL byte", name)
		}
	}
	return nil
}
