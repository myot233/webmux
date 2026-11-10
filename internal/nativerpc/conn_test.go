package nativerpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestNativeFramesOmitVersionAndConsumeAcknowledgementBeforeEOF(t *testing.T) {
	for range 30 {
		client, server := net.Pipe()
		c := New(client, client)
		peer := make(chan error, 1)
		go func() {
			defer func() { _ = server.Close() }()
			var request map[string]json.RawMessage
			if err := json.NewDecoder(server).Decode(&request); err != nil {
				peer <- err
				return
			}
			if _, present := request["jsonrpc"]; present {
				peer <- ErrProtocol
				return
			}
			peer <- json.NewEncoder(server).Encode(map[string]any{"id": request["id"], "result": struct{}{}})
		}()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if err := c.Call(ctx, "thread/inject_items", struct{}{}, &struct{}{}); err != nil {
			t.Fatal(err)
		}
		cancel()
		c.Close()
		if err := <-peer; err != nil {
			t.Fatal(err)
		}
	}
}

func TestNativeCancellationClosesAndJoinsBlockedTransport(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = server.Close() }()
	c := New(client, client)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	// The peer does not read, so even writing the request must be cancellable.
	if err := c.Call(ctx, "thread/inject_items", struct{}{}, &struct{}{}); err == nil {
		t.Fatal("blocked transport succeeded")
	}
	c.Close()
}

func TestNativeErrorsDoNotExposeProviderText(t *testing.T) {
	client, server := net.Pipe()
	c := New(client, client)
	defer c.Close()
	go func() {
		defer func() { _ = server.Close() }()
		line, _ := bufio.NewReader(server).ReadBytes('\n')
		var req struct{ ID int }
		_ = json.Unmarshal(line, &req)
		_ = json.NewEncoder(server).Encode(map[string]any{"id": req.ID, "error": map[string]any{"code": -32601, "message": "private source history"}})
	}()
	err := c.Call(context.Background(), "thread/inject_items", nil, &struct{}{})
	var rpc *Error
	if !errors.As(err, &rpc) || rpc.Code != -32601 || err.Error() != "native RPC error -32601" {
		t.Fatal("untyped or unsanitized native failure", err)
	}
}

func TestNativeRejectsOversizedFrames(t *testing.T) {
	client, server := net.Pipe()
	c := New(client, client)
	defer c.Close()
	go func() {
		defer func() { _ = server.Close() }()
		_, _ = bufio.NewReader(server).ReadBytes('\n')
		_ = json.NewEncoder(server).Encode(map[string]any{"id": 1, "result": strings.Repeat("x", 3<<20)})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Call(ctx, "thread/resume", nil, &struct{}{}); err == nil {
		t.Fatal("accepted unbounded native frame")
	}
}
