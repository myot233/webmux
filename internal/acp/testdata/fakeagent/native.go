package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rainchan/webmux/internal/acp"
)

// A real stdio app-server peer: native thread persistence is shared with ACP,
// while the raw injected Responses items stay separately inspectable by tests.
func runNativeApp(cfg config, st *store, in io.Reader, out io.Writer) error {
	var opened *session
	var lock *os.File
	defer func() {
		if opened != nil {
			st.release(opened.ID)
		}
		if lock != nil {
			_ = lock.Close()
		}
	}()
	initialized, notified := false, false
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	for scanner.Scan() {
		var request rpcMessage
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			return err
		}
		if request.Method == "initialized" {
			notified = initialized
			continue
		}
		st.event("call "+request.Method, "")
		if delay := cfg.delays[request.Method]; delay > 0 {
			time.Sleep(delay)
		}
		response := rpcMessage{ID: request.ID}
		answer := func(result any, err error) error {
			if err != nil {
				response.Error = &acp.RPCError{Code: -32603, Message: "native operation failed"}
			} else {
				response.Result, _ = json.Marshal(result)
			}
			return json.NewEncoder(out).Encode(response)
		}
		if code, found := cfg.fails[request.Method]; found {
			response.Error = &acp.RPCError{Code: code, Message: "injected native failure"}
			if err := json.NewEncoder(out).Encode(response); err != nil {
				return err
			}
			continue
		}
		crashAt(cfg, request.Method)
		switch request.Method {
		case "initialize":
			initialized = true
			if err := answer(map[string]string{"userAgent": "codex/" + cfg.codexVersion}, nil); err != nil {
				return err
			}
		case "thread/resume":
			var p struct {
				ThreadID string `json:"threadId"`
			}
			err := json.Unmarshal(request.Params, &p)
			if !notified || err != nil || opened != nil {
				return io.ErrUnexpectedEOF
			}
			opened, err = st.load(p.ThreadID)
			if err == nil {
				lock, err = holdCodexLock(p.ThreadID)
			}
			if err == nil {
				opened, err = st.lease(opened)
				if err == nil && opened.ID != p.ThreadID {
					return io.ErrUnexpectedEOF
				}
			}
			if err := answer(map[string]any{"thread": map[string]string{"id": p.ThreadID}}, err); err != nil {
				return err
			}
		case "thread/inject_items":
			var p struct {
				ThreadID string `json:"threadId"`
				Items    []struct {
					Type, Role string
					Content    []struct{ Type, Text string }
				} `json:"items"`
			}
			if json.Unmarshal(request.Params, &p) != nil || opened == nil || p.ThreadID != opened.ID || len(p.Items) == 0 {
				return io.ErrUnexpectedEOF
			}
			var messages []message
			for _, item := range p.Items {
				if item.Type != "message" || len(item.Content) != 1 || (item.Role != "user" && item.Role != "assistant") {
					return io.ErrUnexpectedEOF
				}
				role, kind := "user", "input_text"
				if item.Role == "assistant" {
					role, kind = "agent", "output_text"
				}
				if item.Content[0].Type != kind {
					return io.ErrUnexpectedEOF
				}
				messages = append(messages, message{ID: st.newID(), Role: role, Text: item.Content[0].Text})
			}
			opened.Messages = append(opened.Messages, messages...)
			if err := st.save(opened); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(st.dir, "native-inject-"+opened.ID+".json"), request.Params, 0600); err != nil {
				return err
			}
			crashAt(cfg, "inject-saved")
			if err := answer(struct{}{}, nil); err != nil {
				return err
			}
		default:
			response.Error = &acp.RPCError{Code: -32601, Message: "method not found"}
			if err := json.NewEncoder(out).Encode(response); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}
