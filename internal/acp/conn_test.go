package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const testTimeout = 5 * time.Second

// secret marks content that must never reach a log.
const secret = "S3CRET-c0ntent"

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func testLogger() (*slog.Logger, *syncBuffer) {
	var b syncBuffer
	return slog.New(slog.NewTextHandler(&b, &slog.HandlerOptions{Level: slog.LevelDebug})), &b
}

// recorder is a Handler that records updates and delegates requests.
type recorder struct {
	updates    chan SessionNotification
	permission func(ctx context.Context, req RequestPermissionRequest) (RequestPermissionResponse, error)
	elicit     func(ctx context.Context, req CreateElicitationRequest) (CreateElicitationResponse, error)
}

var _ Handler = (*recorder)(nil)

func newRecorder() *recorder { return &recorder{updates: make(chan SessionNotification, 1024)} }

func (r *recorder) SessionUpdate(n SessionNotification) { r.updates <- n }

func (r *recorder) RequestPermission(ctx context.Context, req RequestPermissionRequest) (RequestPermissionResponse, error) {
	if r.permission == nil {
		return RequestPermissionResponse{Outcome: PermissionOutcome{Outcome: OutcomeCancelled}}, nil
	}
	return r.permission(ctx, req)
}

func (r *recorder) CreateElicitation(ctx context.Context, req CreateElicitationRequest) (CreateElicitationResponse, error) {
	if r.elicit == nil {
		return CreateElicitationResponse{Action: ElicitDecline}, nil
	}
	return r.elicit(ctx, req)
}

func (r *recorder) next(t *testing.T) SessionNotification {
	t.Helper()
	select {
	case n := <-r.updates:
		return n
	case <-time.After(testTimeout):
		t.Fatal("no session update")
		return SessionNotification{}
	}
}

// peer is the agent end of an in-memory connection.
type peer struct {
	t    *testing.T
	out  *io.PipeWriter // agent stdout
	msgs chan wireMessage
}

func newPair(t *testing.T, h Handler, log *slog.Logger) (*Conn, *peer) {
	t.Helper()
	clientIn, agentOut := io.Pipe()
	agentIn, clientOut := io.Pipe()
	c := NewConn(clientIn, clientOut, Options{Handler: h, Logger: log})
	p := &peer{t: t, out: agentOut, msgs: make(chan wireMessage, 64)}
	go func() {
		defer close(p.msgs)
		br := bufio.NewReader(agentIn)
		for {
			line, err := br.ReadBytes('\n')
			if err != nil {
				return
			}
			var m wireMessage
			if err := json.Unmarshal(line, &m); err != nil {
				t.Errorf("client wrote invalid JSON %q", line)
				return
			}
			p.msgs <- m
		}
	}()
	t.Cleanup(func() {
		c.Close()
		agentOut.Close()
		select {
		case <-c.Done():
		case <-time.After(testTimeout):
			t.Error("connection goroutines did not exit")
		}
	})
	return c, p
}

func (p *peer) read() wireMessage {
	p.t.Helper()
	select {
	case m, ok := <-p.msgs:
		if !ok {
			p.t.Fatal("client closed its output")
		}
		return m
	case <-time.After(testTimeout):
		p.t.Fatal("no message from client")
		return wireMessage{}
	}
}

// exitOnHangup waits for the client to close the agent's stdin, as it does
// on disconnect, and then ends the agent's stdout like an exiting agent.
func (p *peer) exitOnHangup() {
	p.t.Helper()
	deadline := time.After(testTimeout)
	for {
		select {
		case _, ok := <-p.msgs:
			if !ok {
				p.out.Close()
				return
			}
		case <-deadline:
			p.t.Fatal("client did not close the agent's stdin")
		}
	}
}

func waitDone(t *testing.T, c *Conn) {
	t.Helper()
	select {
	case <-c.Done():
	case <-time.After(testTimeout):
		t.Fatal("connection did not end")
	}
}

func (p *peer) expect(method string) wireMessage {
	p.t.Helper()
	m := p.read()
	if m.Method != method {
		p.t.Fatalf("got method %q, want %q", m.Method, method)
	}
	return m
}

func (p *peer) line(s string) {
	p.t.Helper()
	if _, err := io.WriteString(p.out, s+"\n"); err != nil {
		p.t.Fatal(err)
	}
}

func (p *peer) send(m wireMessage) {
	p.t.Helper()
	m.JSONRPC = "2.0"
	b, err := json.Marshal(m)
	if err != nil {
		p.t.Fatal(err)
	}
	p.line(string(b))
}

func (p *peer) respond(id json.RawMessage, result string) {
	p.t.Helper()
	p.send(wireMessage{ID: id, Result: json.RawMessage(result)})
}

func (p *peer) update(sessionID string, update string) {
	p.t.Helper()
	p.send(wireMessage{Method: MethodSessionUpdate, Params: json.RawMessage(`{"sessionId":"` + sessionID + `","update":` + update + `}`)})
}

type result[T any] struct {
	v   T
	err error
}

func async[T any](f func() (T, error)) chan result[T] {
	ch := make(chan result[T], 1)
	go func() {
		v, err := f()
		ch <- result[T]{v, err}
	}()
	return ch
}

func await[T any](t *testing.T, ch chan result[T]) (T, error) {
	t.Helper()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-time.After(testTimeout):
		t.Fatal("call did not return")
		var zero T
		return zero, nil
	}
}

func TestOutOfOrderResponses(t *testing.T) {
	c, p := newPair(t, newRecorder(), nil)
	ctx := context.Background()
	first := async(func() (ListSessionsResponse, error) { return c.ListSessions(ctx, ListSessionsRequest{Cwd: "/one"}) })
	m1 := p.expect(MethodSessionList)
	second := async(func() (ListSessionsResponse, error) { return c.ListSessions(ctx, ListSessionsRequest{Cwd: "/two"}) })
	m2 := p.expect(MethodSessionList)
	if string(m1.ID) == string(m2.ID) {
		t.Fatalf("both requests use id %s", m1.ID)
	}
	p.respond(m2.ID, `{"sessions":[{"sessionId":"b","cwd":"/two"}]}`)
	p.respond(m1.ID, `{"sessions":[{"sessionId":"a","cwd":"/one"}]}`)
	for _, tt := range []struct {
		ch   chan result[ListSessionsResponse]
		want string
	}{{first, "a"}, {second, "b"}} {
		resp, err := await(t, tt.ch)
		if err != nil {
			t.Fatal(err)
		}
		if len(resp.Sessions) != 1 || resp.Sessions[0].SessionID != tt.want {
			t.Errorf("got %+v, want session %s", resp, tt.want)
		}
	}
}

// endless is an agent stdout that never ends a line.
type endless struct{ pattern []byte }

func (e endless) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = e.pattern[i%len(e.pattern)]
	}
	return len(p), nil
}

type discardCloser struct{}

func (discardCloser) Write(p []byte) (int, error) { return len(p), nil }
func (discardCloser) Close() error                { return nil }

func TestOverlongLineDisconnects(t *testing.T) {
	log, logs := testLogger()
	// The agent keeps writing past the cap; the connection drains it to EOF.
	agentOut := io.LimitReader(endless{[]byte(secret)}, 2*MaxLineBytes)
	c := NewConn(agentOut, discardCloser{}, Options{Handler: newRecorder(), Logger: log})
	_, err := c.ListSessions(context.Background(), ListSessionsRequest{})
	var perr *ProtocolError
	if !errors.As(err, &perr) || perr.Reason != ReasonLineTooLong {
		t.Fatalf("call error = %v, want line too long", err)
	}
	waitDone(t, c)
	if !errors.Is(c.Err(), ErrProtocol) {
		t.Errorf("Err() = %v, want ErrProtocol", c.Err())
	}
	if got := logs.String(); strings.Contains(got, secret) || !strings.Contains(got, ReasonLineTooLong) {
		t.Errorf("log = %q, want the reason without content", got)
	}
}

func TestLineAtLimitIsAccepted(t *testing.T) {
	c, p := newPair(t, newRecorder(), nil)
	head, tail := `{"jsonrpc":"2.0","method":"_pad","params":"`, `"}`
	pad := strings.Repeat("a", MaxLineBytes-len(head)-len(tail))
	call := async(func() (ListSessionsResponse, error) {
		return c.ListSessions(context.Background(), ListSessionsRequest{})
	})
	m := p.expect(MethodSessionList)
	p.line(head + pad + tail)
	p.respond(m.ID, `{"sessions":[]}`)
	if _, err := await(t, call); err != nil {
		t.Fatalf("a %d byte line broke the connection: %v", MaxLineBytes, err)
	}
}

func TestInvalidLinesDisconnect(t *testing.T) {
	tests := []struct {
		name   string
		give   string
		reason string
	}{
		{"plain text", "hello " + secret, ReasonInvalidJSON},
		{"truncated json", `{"jsonrpc":"2.0","method":"` + secret, ReasonInvalidJSON},
		{"json array", `["` + secret + `"]`, ReasonInvalidMessage},
		{"wrong version", `{"jsonrpc":"1.0","method":"x","params":"` + secret + `"}`, ReasonInvalidMessage},
		{"response without result or error", `{"jsonrpc":"2.0","id":1}`, ReasonInvalidMessage},
		{"response with both", `{"jsonrpc":"2.0","id":1,"result":{},"error":{"code":1,"message":"` + secret + `"}}`, ReasonInvalidMessage},
		{"neither request nor response", `{"jsonrpc":"2.0","params":"` + secret + `"}`, ReasonInvalidMessage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, logs := testLogger()
			c, p := newPair(t, newRecorder(), log)
			call := async(func() (PromptResponse, error) {
				return c.Prompt(context.Background(), PromptRequest{SessionID: "s1", Prompt: []ContentBlock{TextBlock("hi")}})
			})
			p.expect(MethodSessionPrompt)
			p.line(tt.give)
			_, err := await(t, call)
			var perr *ProtocolError
			if !errors.As(err, &perr) || perr.Reason != tt.reason || perr.Bytes != len(tt.give) {
				t.Fatalf("call error = %v, want %s of %d bytes", err, tt.reason, len(tt.give))
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("error %q carries the line", err)
			}
			p.exitOnHangup()
			waitDone(t, c)
			if got := logs.String(); strings.Contains(got, secret) || !strings.Contains(got, tt.reason) {
				t.Errorf("log = %q, want the reason without content", got)
			}
		})
	}
}

func TestBlankLinesAndUnknownNotificationsAreIgnored(t *testing.T) {
	c, p := newPair(t, newRecorder(), nil)
	call := async(func() (ListSessionsResponse, error) {
		return c.ListSessions(context.Background(), ListSessionsRequest{})
	})
	m := p.expect(MethodSessionList)
	p.line("")
	p.line("  \r")
	p.send(wireMessage{Method: "_vendor/hint", Params: json.RawMessage(`{"a":1}`)})
	p.send(wireMessage{Method: "elicitation/complete", Params: json.RawMessage(`{"elicitationId":"e"}`)})
	p.send(wireMessage{ID: json.RawMessage(`"not-ours"`), Result: json.RawMessage(`{}`)})
	p.respond(m.ID, `{"sessions":[]}`)
	if _, err := await(t, call); err != nil {
		t.Fatal(err)
	}
}

func TestPromptCancelSendsSessionCancel(t *testing.T) {
	r := newRecorder()
	c, p := newPair(t, r, nil)
	ctx, cancel := context.WithCancel(context.Background())
	call := async(func() (PromptResponse, error) {
		return c.Prompt(ctx, PromptRequest{SessionID: "s1", Prompt: []ContentBlock{TextBlock("hi")}})
	})
	m := p.expect(MethodSessionPrompt)
	cancel()
	n := p.expect(MethodSessionCancel)
	if !jsonEqual(t, n.Params, []byte(`{"sessionId":"s1"}`)) {
		t.Errorf("session/cancel params %s", n.Params)
	}
	// Updates after the cancel are still delivered, before the answer.
	p.update("s1", `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"late"}}`)
	p.respond(m.ID, `{"stopReason":"cancelled"}`)
	resp, err := await(t, call)
	if err != nil || resp.StopReason != StopCancelled {
		t.Fatalf("Prompt = %+v, %v; want cancelled", resp, err)
	}
	select {
	case u := <-r.updates:
		if chunk, ok := u.Update.(*AgentMessageChunk); !ok || chunk.Content.Text != "late" {
			t.Errorf("update = %#v", u.Update)
		}
	default:
		t.Error("late update was not handled before Prompt returned")
	}
}

func TestRequestCancelSendsCancelRequest(t *testing.T) {
	c, p := newPair(t, newRecorder(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	call := async(func() (LoadSessionResponse, error) {
		return c.LoadSession(ctx, LoadSessionRequest{SessionID: "s1", Cwd: "/w"})
	})
	m := p.expect(MethodSessionLoad)
	cancel()
	n := p.expect(MethodCancelRequest)
	var params CancelRequestNotification
	if err := json.Unmarshal(n.Params, &params); err != nil || string(params.RequestID) != string(m.ID) {
		t.Fatalf("$/cancel_request params %s, want requestId %s", n.Params, m.ID)
	}
	if _, err := await(t, call); !errors.Is(err, context.Canceled) {
		t.Fatalf("LoadSession error = %v, want context.Canceled", err)
	}
	// The agent's late answer is dropped; the connection stays usable.
	p.send(wireMessage{ID: m.ID, Error: &RPCError{Code: CodeRequestCancelled, Message: "cancelled"}})
	next := async(func() (ListSessionsResponse, error) {
		return c.ListSessions(context.Background(), ListSessionsRequest{})
	})
	m2 := p.expect(MethodSessionList)
	p.respond(m2.ID, `{"sessions":[]}`)
	if _, err := await(t, next); err != nil {
		t.Fatal(err)
	}
}

func TestRequestDeadlineSendsCancelRequest(t *testing.T) {
	c, p := newPair(t, newRecorder(), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	call := async(func() (struct{}, error) { return struct{}{}, c.CloseSession(ctx, "s1") })
	m := p.expect(MethodSessionClose)
	n := p.expect(MethodCancelRequest)
	if !jsonEqual(t, n.Params, []byte(`{"requestId":`+string(m.ID)+`}`)) {
		t.Errorf("params %s", n.Params)
	}
	if _, err := await(t, call); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
}

func TestProcessExitFailsPendingCalls(t *testing.T) {
	c, p := newPair(t, newRecorder(), nil)
	ctx := context.Background()
	prompt := async(func() (PromptResponse, error) {
		return c.Prompt(ctx, PromptRequest{SessionID: "s1", Prompt: []ContentBlock{TextBlock("hi")}})
	})
	p.expect(MethodSessionPrompt)
	load := async(func() (LoadSessionResponse, error) {
		return c.LoadSession(ctx, LoadSessionRequest{SessionID: "s1", Cwd: "/w"})
	})
	p.expect(MethodSessionLoad)
	p.out.Close() // the agent exits
	if _, err := await(t, prompt); !errors.Is(err, ErrDisconnected) {
		t.Errorf("Prompt error = %v, want ErrDisconnected", err)
	}
	if _, err := await(t, load); !errors.Is(err, ErrDisconnected) {
		t.Errorf("LoadSession error = %v, want ErrDisconnected", err)
	}
	waitDone(t, c)
	if !errors.Is(c.Err(), ErrDisconnected) {
		t.Errorf("Err() = %v", c.Err())
	}
	if _, err := c.ListSessions(ctx, ListSessionsRequest{}); !errors.Is(err, ErrDisconnected) {
		t.Errorf("call after exit = %v, want ErrDisconnected", err)
	}
}

func TestResponseBeforeExitIsDelivered(t *testing.T) {
	c, p := newPair(t, newRecorder(), nil)
	call := async(func() (PromptResponse, error) {
		return c.Prompt(context.Background(), PromptRequest{SessionID: "s1"})
	})
	m := p.expect(MethodSessionPrompt)
	p.respond(m.ID, `{"stopReason":"end_turn"}`)
	p.out.Close()
	if resp, err := await(t, call); err != nil || resp.StopReason != StopEndTurn {
		t.Fatalf("Prompt = %+v, %v", resp, err)
	}
}

func TestCloseFailsPendingCalls(t *testing.T) {
	c, p := newPair(t, newRecorder(), nil)
	call := async(func() (ListSessionsResponse, error) {
		return c.ListSessions(context.Background(), ListSessionsRequest{})
	})
	p.expect(MethodSessionList)
	c.Close()
	if _, err := await(t, call); !errors.Is(err, ErrClosed) {
		t.Fatalf("error = %v, want ErrClosed", err)
	}
	if _, ok := <-p.msgs; ok {
		t.Error("agent stdin stayed open after Close")
	}
}

func TestUpdatesBeforeResponseAreHandledFirst(t *testing.T) {
	r := newRecorder()
	c, p := newPair(t, r, nil)
	call := async(func() (LoadSessionResponse, error) {
		return c.LoadSession(context.Background(), LoadSessionRequest{SessionID: "s1", Cwd: "/w"})
	})
	m := p.expect(MethodSessionLoad)
	for i := range 50 {
		p.update("s1", `{"sessionUpdate":"user_message_chunk","content":{"type":"text","text":"`+strconv.Itoa(i)+`"}}`)
	}
	p.respond(m.ID, `{}`)
	if _, err := await(t, call); err != nil {
		t.Fatal(err)
	}
	if len(r.updates) != 50 {
		t.Fatalf("%d updates handled before LoadSession returned, want 50", len(r.updates))
	}
	for i := range 50 {
		u := r.next(t)
		if got := u.Update.(*UserMessageChunk).Content.Text; got != strconv.Itoa(i) {
			t.Fatalf("update %d has text %q: out of order", i, got)
		}
	}
}

func TestUnknownUpdateKindIsDelivered(t *testing.T) {
	r := newRecorder()
	_, p := newPair(t, r, nil)
	p.update("s1", `{"sessionUpdate":"state_update","state":"idle"}`)
	p.update("s1", `{"sessionUpdate":"plan","entries":[]}`)
	u := r.next(t).Update.(*UnknownUpdate)
	if u.Kind != "state_update" || !jsonEqual(t, u.Raw, []byte(`{"sessionUpdate":"state_update","state":"idle"}`)) {
		t.Errorf("unknown update = %+v", u)
	}
	if _, ok := r.next(t).Update.(*Plan); !ok {
		t.Error("the update after an unknown kind was lost")
	}
}

func TestUnadvertisedClientMethodsAreNotFound(t *testing.T) {
	_, p := newPair(t, newRecorder(), nil)
	for i, method := range []string{"fs/read_text_file", "fs/write_text_file", "terminal/create", "terminal/output", "_vendor/x", "session/update"} {
		id := json.RawMessage(strconv.Itoa(100 + i))
		p.send(wireMessage{ID: id, Method: method, Params: json.RawMessage(`{"sessionId":"s1","path":"/etc/passwd"}`)})
		m := p.read()
		if string(m.ID) != string(id) || m.Error == nil || m.Error.Code != CodeMethodNotFound {
			t.Errorf("%s: got %+v, want method not found", method, m)
		}
	}
}

func TestPermissionRunsBesideUpdates(t *testing.T) {
	r := newRecorder()
	release := make(chan struct{})
	got := make(chan RequestPermissionRequest, 1)
	r.permission = func(ctx context.Context, req RequestPermissionRequest) (RequestPermissionResponse, error) {
		got <- req
		<-release
		return RequestPermissionResponse{Outcome: PermissionOutcome{Outcome: OutcomeSelected, OptionID: "allow"}}, nil
	}
	_, p := newPair(t, r, nil)
	p.update("s1", `{"sessionUpdate":"tool_call","toolCallId":"c1","title":"Edit"}`)
	p.send(wireMessage{ID: json.RawMessage(`"p1"`), Method: MethodRequestPermission, Params: json.RawMessage(
		`{"sessionId":"s1","toolCall":{"toolCallId":"c1"},"options":[{"optionId":"allow","name":"Allow","kind":"allow_once"}]}`)})
	if _, ok := r.next(t).Update.(*ToolCall); !ok {
		t.Fatal("tool call update not delivered first")
	}
	select {
	case req := <-got:
		if req.ToolCall.ToolCallID != "c1" || len(req.Options) != 1 {
			t.Errorf("request = %+v", req)
		}
	case <-time.After(testTimeout):
		t.Fatal("permission handler not called")
	}
	// The handler is blocked; updates keep flowing.
	p.update("s1", `{"sessionUpdate":"agent_thought_chunk","content":{"type":"text","text":"waiting"}}`)
	if _, ok := r.next(t).Update.(*AgentThoughtChunk); !ok {
		t.Fatal("update blocked behind a permission request")
	}
	close(release)
	m := p.read()
	if string(m.ID) != `"p1"` || !jsonEqual(t, m.Result, []byte(`{"outcome":{"outcome":"selected","optionId":"allow"}}`)) {
		t.Errorf("answer = %+v", m)
	}
}

func TestAgentCancelRequestCancelsHandler(t *testing.T) {
	r := newRecorder()
	r.elicit = func(ctx context.Context, req CreateElicitationRequest) (CreateElicitationResponse, error) {
		<-ctx.Done()
		return CreateElicitationResponse{}, ctx.Err()
	}
	_, p := newPair(t, r, nil)
	p.send(wireMessage{ID: json.RawMessage(`9`), Method: MethodElicitationCreate, Params: json.RawMessage(
		`{"mode":"form","message":"m","sessionId":"s1","requestedSchema":{"type":"object","properties":{}}}`)})
	p.send(wireMessage{Method: MethodCancelRequest, Params: json.RawMessage(`{"requestId": 9}`)})
	m := p.read()
	if string(m.ID) != "9" || m.Error == nil || m.Error.Code != CodeRequestCancelled {
		t.Fatalf("answer = %+v, want request cancelled", m)
	}
}

func TestElicitationModes(t *testing.T) {
	r := newRecorder()
	r.elicit = func(ctx context.Context, req CreateElicitationRequest) (CreateElicitationResponse, error) {
		if req.RequestedSchema == nil || req.RequestedSchema.Properties["name"].Type != "string" {
			return CreateElicitationResponse{}, &RPCError{Code: CodeInvalidParams, Message: "bad"}
		}
		return CreateElicitationResponse{Action: ElicitAccept, Content: map[string]any{"name": "Ann"}}, nil
	}
	_, p := newPair(t, r, nil)
	p.send(wireMessage{ID: json.RawMessage(`1`), Method: MethodElicitationCreate, Params: json.RawMessage(
		`{"mode":"form","message":"m","sessionId":"s1","requestedSchema":{"type":"object","properties":{"name":{"type":"string"}}}}`)})
	if m := p.read(); !jsonEqual(t, m.Result, []byte(`{"action":"accept","content":{"name":"Ann"}}`)) {
		t.Errorf("form answer = %+v", m)
	}
	p.send(wireMessage{ID: json.RawMessage(`2`), Method: MethodElicitationCreate, Params: json.RawMessage(
		`{"mode":"url","message":"m","sessionId":"s1","elicitationId":"e","url":"https://example.com"}`)})
	if m := p.read(); m.Error == nil || m.Error.Code != CodeInvalidParams {
		t.Errorf("url answer = %+v, want invalid params", m)
	}
}

func TestHandlerErrorsAreNotEchoed(t *testing.T) {
	r := newRecorder()
	r.permission = func(context.Context, RequestPermissionRequest) (RequestPermissionResponse, error) {
		return RequestPermissionResponse{}, errors.New(secret)
	}
	_, p := newPair(t, r, nil)
	p.send(wireMessage{ID: json.RawMessage(`1`), Method: MethodRequestPermission, Params: json.RawMessage(`{"sessionId":"s1","toolCall":{"toolCallId":"c"},"options":[]}`)})
	m := p.read()
	if m.Error == nil || m.Error.Code != CodeInternalError || strings.Contains(m.Error.Message, secret) {
		t.Errorf("answer = %+v", m.Error)
	}
}

func TestRPCErrorHidesMessage(t *testing.T) {
	c, p := newPair(t, newRecorder(), nil)
	call := async(func() (NewSessionResponse, error) {
		return c.NewSession(context.Background(), NewSessionRequest{Cwd: "/w"})
	})
	m := p.expect(MethodSessionNew)
	if !jsonEqual(t, m.Params, []byte(`{"cwd":"/w","mcpServers":[]}`)) {
		t.Errorf("params %s", m.Params)
	}
	p.send(wireMessage{ID: m.ID, Error: &RPCError{Code: CodeAuthRequired, Message: "login: " + secret}})
	_, err := await(t, call)
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != CodeAuthRequired || rpcErr.Message != "login: "+secret {
		t.Fatalf("error = %#v", err)
	}
	if strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "-32000") || !strings.Contains(err.Error(), MethodSessionNew) {
		t.Errorf("Error() = %q, want method and code only", err.Error())
	}
}

func TestInitialize(t *testing.T) {
	tests := []struct {
		name    string
		version int
		wantErr error
	}{
		{"version 1", 1, nil},
		{"version 2", 2, ErrUnsupportedVersion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, p := newPair(t, newRecorder(), nil)
			call := async(func() (InitializeResponse, error) {
				return c.Initialize(context.Background(), Implementation{Name: "webmux", Version: "1"}, json.RawMessage(`{"steering":{"supported":true}}`))
			})
			m := p.expect(MethodInitialize)
			want := `{"protocolVersion":1,"clientCapabilities":{"session":{"notices":{},"compaction":{}},"elicitation":{"form":{}}},
				"clientInfo":{"name":"webmux","version":"1"},"_meta":{"steering":{"supported":true}}}`
			if !jsonEqual(t, m.Params, []byte(want)) {
				t.Errorf("params %s", m.Params)
			}
			p.respond(m.ID, `{"protocolVersion":`+strconv.Itoa(tt.version)+`,"agentCapabilities":{"loadSession":true,"sessionCapabilities":{"list":{}}}}`)
			resp, err := await(t, call)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if !resp.AgentCapabilities.LoadSession || resp.AgentCapabilities.SessionCapabilities.List == nil {
				t.Errorf("capabilities = %+v", resp.AgentCapabilities)
			}
		})
	}
}

func TestCallExtensionRequiresUnderscore(t *testing.T) {
	c, p := newPair(t, newRecorder(), nil)
	if err := c.CallExtension(context.Background(), "session/prompt", nil, nil); err == nil {
		t.Fatal("non-extension method accepted")
	}
	call := async(func() (struct{}, error) {
		return struct{}{}, c.CallExtension(context.Background(), "_session/steering", map[string]string{"sessionId": "s1"}, nil)
	})
	m := p.expect("_session/steering")
	p.respond(m.ID, `{}`)
	if _, err := await(t, call); err != nil {
		t.Fatal(err)
	}
}

func TestNilHandler(t *testing.T) {
	_, p := newPair(t, nil, nil)
	p.update("s1", `{"sessionUpdate":"plan","entries":[]}`)
	p.send(wireMessage{ID: json.RawMessage(`1`), Method: MethodRequestPermission, Params: json.RawMessage(`{"sessionId":"s1","toolCall":{"toolCallId":"c"},"options":[]}`)})
	if m := p.read(); m.Error == nil || m.Error.Code != CodeMethodNotFound {
		t.Errorf("answer = %+v", m)
	}
}

// TestLogsCarryNoContent drives every logging path with content in each
// place an agent can put it and checks the log keeps none of it.
func TestLogsCarryNoContent(t *testing.T) {
	log, logs := testLogger()
	r := newRecorder()
	r.permission = func(context.Context, RequestPermissionRequest) (RequestPermissionResponse, error) {
		return RequestPermissionResponse{}, errors.New(secret)
	}
	c, p := newPair(t, r, log)
	call := async(func() (PromptResponse, error) {
		return c.Prompt(context.Background(), PromptRequest{SessionID: "s1", Prompt: []ContentBlock{TextBlock(secret)}})
	})
	m := p.expect(MethodSessionPrompt)
	p.update("s1", `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"`+secret+`"}}`)
	p.update("s1", `{"sessionUpdate":"`+secret+`","x":"`+secret+`"}`)
	p.update("s1", `{"sessionUpdate":"usage_update","used":"`+secret+`","size":1}`)
	p.update("s1", `{"sessionUpdate":"tool_call","toolCallId":"c1","title":"`+secret+`","rawInput":{"cmd":"`+secret+`"}}`)
	p.send(wireMessage{ID: json.RawMessage(`5`), Method: MethodRequestPermission, Params: json.RawMessage(
		`{"sessionId":"s1","toolCall":{"toolCallId":"c1"},"options":[{"optionId":"a","name":"` + secret + `","kind":"allow_once"}]}`)})
	p.send(wireMessage{ID: json.RawMessage(`6`), Method: "fs/read_text_file", Params: json.RawMessage(`{"path":"` + secret + `"}`)})
	for range 2 {
		p.read()
	}
	for range 3 {
		r.next(t)
	}
	p.send(wireMessage{ID: m.ID, Error: &RPCError{Code: CodeInternalError, Message: secret, Data: json.RawMessage(`"` + secret + `"`)}})
	if _, err := await(t, call); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("Prompt error = %v", err)
	}
	p.line(secret + " not json")
	p.exitOnHangup()
	waitDone(t, c)
	got := logs.String()
	if strings.Contains(got, secret) {
		t.Fatalf("log carries content:\n%s", got)
	}
	for _, want := range []string{"usage_update", ReasonInvalidJSON} {
		if !strings.Contains(got, want) {
			t.Errorf("log lacks %q:\n%s", want, got)
		}
	}
}
