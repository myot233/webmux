package main

import (
	"context"
	"os"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/runtime"
)

// controlMCPServers supplies instance-local tools to actual ACP sessions. It is
// evaluated at session open, after the local socket has been started.
func controlMCPServers(executable string, socketPath func() string) func(context.Context) []acp.MCPServer {
	return func(context.Context) []acp.MCPServer {
		socket := socketPath()
		if !mcpAvailable(socket, executable) {
			return nil
		}
		return []acp.MCPServer{{Name: "webmux", Command: executable, Args: []string{"mcp", "--socket", socket}, Env: []acp.MCPEnv{}}}
	}
}

// Availability checks never launch a helper or expose instance paths to the
// browser. Both control and frozen-history descriptors use the running binary.
func mcpSocket(stateDir, executable string) (string, bool) {
	socket, err := runtime.SocketPath(stateDir, os.Getenv)
	if err != nil || !mcpAvailable(socket, executable) {
		return "", false
	}
	return socket, true
}

func mcpAvailable(socket, executable string) bool {
	if socket == "" || executable == "" {
		return false
	}
	bin, err := os.Stat(executable)
	if err != nil || !bin.Mode().IsRegular() || bin.Mode().Perm()&0111 == 0 {
		return false
	}
	info, err := os.Stat(socket)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return false
	}
	return true
}
