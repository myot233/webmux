package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/rainchan/webmux/internal/acp"
)

type mcpChild struct {
	pid int

	in     io.WriteCloser
	cancel context.CancelFunc
	done   chan struct{}
}

func (c *mcpChild) close() {
	_ = c.in.Close()
	select {
	case <-c.done:
	case <-time.After(time.Second):
		c.cancel()
		<-c.done
	}
	c.cancel()
}

// Real stdio client, deliberately independent of the server implementation.
// Configuration and returned text are fixture artifacts, never application logs.
func (a *agent) configureMCP(sid, method string, servers []acp.MCPServer) {
	for _, server := range servers {
		if server.Name != "webmux-history" && !(server.Name == "webmux" && os.Getenv("FAKEAGENT_CONTROL_MCP") == "1") {
			continue
		}
		child, result, err := runFixtureMCP(server)
		if err != nil {
			a.st.event("mcp unavailable", method)
			continue
		}
		a.mu.Lock()
		ls := a.live[sid]
		if ls != nil {
			ls.mcps = append(ls.mcps, child)
		}
		a.mu.Unlock()
		if ls == nil {
			child.close()
			continue
		}
		prefix := "mcp-"
		if server.Name == "webmux" {
			prefix = "mcp-control-"
		}
		if err := os.WriteFile(filepath.Join(a.cfg.store, prefix+"pid-"+filepath.Base(method)+"-"+sid), []byte(strconv.Itoa(child.pid)), 0600); err != nil {
			a.st.event("mcp pid fixture failed", method)
		}
		body, err := json.Marshal(result)
		if err == nil {
			err = os.WriteFile(filepath.Join(a.cfg.store, prefix+filepath.Base(method)+"-"+sid+".json"), body, 0600)
		}
		if err != nil {
			a.st.event("mcp fixture failed", method)
		}
	}
}

func runFixtureMCP(server acp.MCPServer) (*mcpChild, []json.RawMessage, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, server.Command, server.Args...)
	cmd.Env = os.Environ()
	for _, v := range server.Env {
		cmd.Env = append(cmd.Env, v.Name+"="+v.Value)
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		_ = in.Close()
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		_ = in.Close()
		return nil, nil, err
	}
	c := &mcpChild{pid: cmd.Process.Pid, in: in, cancel: cancel, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(c.done) }()
	timer := time.AfterFunc(8*time.Second, cancel)
	defer timer.Stop()
	scan := bufio.NewScanner(out)
	scan.Buffer(nil, 4<<20)
	enc := json.NewEncoder(in)
	id := 0
	call := func(method string, params any) (json.RawMessage, error) {
		id++
		if err := enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
			return nil, err
		}
		if !scan.Scan() {
			return nil, fmt.Errorf("MCP ended")
		}
		var resp struct {
			Result json.RawMessage
			Error  json.RawMessage
		}
		if err := json.Unmarshal(scan.Bytes(), &resp); err != nil {
			return nil, err
		}
		if len(resp.Error) > 0 {
			return nil, fmt.Errorf("MCP error")
		}
		return resp.Result, nil
	}
	fail := func(err error) (*mcpChild, []json.RawMessage, error) { c.close(); return nil, nil, err }
	if _, err := call("initialize", map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "fake-agent", "version": "1"}}); err != nil {
		return fail(err)
	}
	if err := enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); err != nil {
		return fail(err)
	}
	listed, err := call("tools/list", map[string]any{})
	if err != nil {
		return fail(err)
	}
	if server.Name == "webmux" {
		value, err := call("tools/call", map[string]any{"name": "capabilities", "arguments": map[string]any{}})
		if err != nil {
			return fail(err)
		}
		return c, []json.RawMessage{listed, value}, nil
	}
	var results []json.RawMessage
	after := "0"
	for range 10000 {
		value, err := call("tools/call", map[string]any{"name": "thread_read", "arguments": map[string]any{"view": "activity", "afterPosition": after, "limit": 20}})
		if err != nil {
			return fail(err)
		}
		results = append(results, value)
		var result struct {
			IsError bool `json:"isError"`
			Value   struct {
				Done bool   `json:"done"`
				Next string `json:"nextPosition"`
			} `json:"structuredContent"`
		}
		if err := json.Unmarshal(value, &result); err != nil {
			return fail(err)
		}
		if result.IsError || result.Value.Done {
			return c, results, nil
		}
		previous, _ := strconv.ParseInt(after, 10, 64)
		next, _ := strconv.ParseInt(result.Value.Next, 10, 64)
		if next <= previous {
			return fail(fmt.Errorf("MCP cursor did not advance"))
		}
		after = result.Value.Next
	}
	return fail(fmt.Errorf("MCP too many pages"))
}
