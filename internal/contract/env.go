package contract

import "strings"

// SessionIDEnv marks every session process; cleanup depends on it, so a
// request cannot set it.
const SessionIDEnv = "WEBMUX_SESSION_ID"

// Identity variables of the terminal or multiplexer that started a process,
// and the variables the server sets itself. Left in place they make programs
// target that host (for example DCS-wrapping output for tmux) instead of the
// browser terminal behind the session's PTY.
var serverOwnedEnvNames = map[string]bool{
	"TERM_PROGRAM": true, "TERM_PROGRAM_VERSION": true, "TMUX": true, "TMUX_PANE": true,
	"STY": true, "LC_TERMINAL": true, "LC_TERMINAL_VERSION": true, "WT_SESSION": true,
	"WT_PROFILE_ID": true, "VTE_VERSION": true,
	// Describes how the server was started, not the session's programs.
	"WEBMUX_LAUNCHER": true,
	// Set by the server for every session.
	"TERM": true, "COLORTERM": true, SessionIDEnv: true,
}

var serverOwnedEnvPrefixes = []string{"WARP_", "ZELLIJ", "KITTY_", "ITERM_", "WEZTERM_", "GHOSTTY_"}

// ServerOwnedEnv reports whether the server strips or sets name in every
// session environment. A client forwarding its own environment skips these,
// so the session keeps the WebMux terminal identity; an explicit request
// variable may still override them (except SessionIDEnv).
func ServerOwnedEnv(name string) bool {
	if serverOwnedEnvNames[name] {
		return true
	}
	for _, prefix := range serverOwnedEnvPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// ValidEnvName reports whether name is a portable variable name
// ([A-Za-z_][A-Za-z0-9_]*), the only names a create request accepts.
func ValidEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, c := range name {
		switch {
		case c == '_', c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}
