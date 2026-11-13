//go:build linux || darwin

package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/runtime"
)

func TestControlMCPServersUsesCurrentInstance(t *testing.T) {
	f := newFixture(t, "")
	socket, err := runtime.SocketPath(f.state, os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(f.dir, "webmux")
	if err := os.WriteFile(executable, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(executable, 0700); err != nil {
		t.Fatal(err)
	}
	ownedSocket := ""
	servers := controlMCPServers(executable, func() string { return ownedSocket })
	if got := servers(context.Background()); len(got) != 0 {
		t.Fatalf("injected without socket: %+v", got)
	}
	listener, err := runtime.ListenSocket(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	// An existing socket from another instance is never advertised.
	if got := servers(context.Background()); len(got) != 0 {
		t.Fatalf("injected unowned socket: %+v", got)
	}
	ownedSocket = socket
	want := []acp.MCPServer{{Name: "webmux", Command: executable, Args: []string{"mcp", "--socket", socket}, Env: []acp.MCPEnv{}}}
	if got := servers(context.Background()); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for _, invalid := range []string{"", filepath.Join(f.dir, "missing"), f.dir} {
		if got := controlMCPServers(invalid, func() string { return ownedSocket })(context.Background()); len(got) != 0 {
			t.Fatalf("injected invalid executable %q: %+v", invalid, got)
		}
	}
	if err := os.Chmod(executable, 0600); err != nil {
		t.Fatal(err)
	}
	if got := servers(context.Background()); len(got) != 0 {
		t.Fatalf("injected nonexecutable: %+v", got)
	}
	if err := os.Chmod(executable, 0700); err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(socket, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if got := servers(context.Background()); len(got) != 0 {
		t.Fatalf("injected regular file as socket: %+v", got)
	}
}
