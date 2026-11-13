package clicmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/localclient"
)

func TestMCPCancellationUnblocksIdleStdin(t *testing.T) {
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	var stdout, stderr bytes.Buffer
	e := Env{Stdin: input, Stdout: &stdout, Stderr: &stderr, Connect: func() (*localclient.Client, error) {
		return localclient.New(localclient.UnixDialer{Path: "/unused"}), nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() { done <- e.Run(ctx, mcpCommand, nil) }()
	// Let the stdin read enter the kernel before cancelling.
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case code := <-done:
		if code == 0 {
			t.Fatal("cancelled command reported normal EOF")
		}
	case <-time.After(time.Second):
		t.Fatal("idle MCP command did not stop while stdin remained open")
	}
}

func TestMCPCancellationUnblocksOSPipeWithoutClosingDescriptor(t *testing.T) {
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer writer.Close()
	var stdout, stderr bytes.Buffer
	e := Env{Stdin: input, Stdout: &stdout, Stderr: &stderr, Connect: func() (*localclient.Client, error) {
		return localclient.New(localclient.UnixDialer{Path: "/unused"}), nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() { done <- e.Run(ctx, mcpCommand, nil) }()
	// Let the stdin read enter the kernel before cancelling.
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case code := <-done:
		if code == 0 {
			t.Fatal("cancelled command reported EOF")
		}
	case <-time.After(time.Second):
		t.Fatal("OS stdin pipe stayed blocked")
	}
	if _, err := writer.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	var b [1]byte
	if _, err := input.Read(b[:]); err != nil || b[0] != 'x' {
		t.Fatalf("stdin descriptor no longer usable: %q %v", b, err)
	}
}
