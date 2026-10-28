package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rainchan/webmux/internal/acp"
)

const methodSteering = "_session/steering"

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *acp.RPCError   `json:"error,omitempty"`
}

// agent is the ACP side: an independent JSON-RPC peer, so the client
// package is tested against code that does not share its transport.
type agent struct {
	cfg config
	st  *store

	outMu sync.Mutex
	out   io.Writer

	mu       sync.Mutex
	nextID   int64
	pending  map[int64]chan rpcMessage
	inflight map[string]context.CancelFunc
	live     map[string]*liveSession
	wg       sync.WaitGroup
	gone     chan struct{} // closed when stdin ends
}

// liveSession is an open session. data may be a fork of the requested ID
// when another process held it: writes then land on the fork, as in omp.
type liveSession struct {
	data *session
	turn context.CancelFunc
}

func runACP(cfg config, st *store, in io.Reader, out io.Writer) error {
	a := &agent{cfg: cfg, st: st, out: out, pending: map[int64]chan rpcMessage{}, inflight: map[string]context.CancelFunc{}, live: map[string]*liveSession{}, gone: make(chan struct{})}
	if cfg.noise != "" {
		if _, err := fmt.Fprintln(out, cfg.noise); err != nil {
			return err
		}
	}
	br := bufio.NewReaderSize(in, 1<<20)
	for {
		line, err := br.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			var m rpcMessage
			if jerr := json.Unmarshal(line, &m); jerr != nil {
				a.send(rpcMessage{ID: json.RawMessage("null"), Error: &acp.RPCError{Code: acp.CodeParseError, Message: "parse error"}})
			} else {
				a.handle(m)
			}
		}
		if err != nil {
			// stdin closed: the client is gone. Stop turns and calls,
			// then release leases, like a real agent flushing on exit.
			close(a.gone)
			a.mu.Lock()
			for _, cancel := range a.inflight {
				cancel()
			}
			a.mu.Unlock()
			a.stopTurns()
			a.wg.Wait()
			a.releaseAll()
			return nil
		}
	}
}

func (a *agent) stopTurns() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, ls := range a.live {
		if ls.turn != nil {
			ls.turn()
		}
	}
}

func (a *agent) releaseAll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for id, ls := range a.live {
		a.st.release(ls.data.ID)
		delete(a.live, id)
	}
}

func (a *agent) send(m rpcMessage) {
	m.JSONRPC = "2.0"
	b, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	a.outMu.Lock()
	defer a.outMu.Unlock()
	_, _ = a.out.Write(append(b, '\n'))
}

func (a *agent) notify(method string, params any) {
	p, err := json.Marshal(params)
	if err != nil {
		panic(err)
	}
	a.send(rpcMessage{Method: method, Params: p})
}

func (a *agent) update(sessionID string, u acp.Update) {
	a.notify(acp.MethodSessionUpdate, acp.SessionNotification{SessionID: sessionID, Update: u})
}

// request calls the client and waits for its answer. Like a real agent it
// does not give up when the turn is cancelled: ACP has the client answer
// pending permission requests with "cancelled". Only stdin closing ends the
// wait.
func (a *agent) request(method string, params any, result any) error {
	p, err := json.Marshal(params)
	if err != nil {
		return err
	}
	ch := make(chan rpcMessage, 1)
	a.mu.Lock()
	a.nextID++
	id := a.nextID
	a.pending[id] = ch
	a.mu.Unlock()
	a.send(rpcMessage{ID: json.RawMessage(strconv.FormatInt(id, 10)), Method: method, Params: p})
	select {
	case m := <-ch:
		if m.Error != nil {
			return m.Error
		}
		if result == nil {
			return nil
		}
		return json.Unmarshal(m.Result, result)
	case <-a.gone:
		return io.EOF
	}
}

func (a *agent) handle(m rpcMessage) {
	switch {
	case m.Method == "" && m.ID != nil:
		id, err := strconv.ParseInt(string(m.ID), 10, 64)
		if err != nil {
			return
		}
		a.mu.Lock()
		ch := a.pending[id]
		delete(a.pending, id)
		a.mu.Unlock()
		if ch != nil {
			ch <- m
		}
	case m.ID == nil:
		a.notification(m)
	default:
		a.st.event("call "+m.Method, "-")
		ctx, cancel := context.WithCancel(context.Background())
		key := string(m.ID)
		a.mu.Lock()
		a.inflight[key] = cancel
		a.mu.Unlock()
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			defer func() {
				a.mu.Lock()
				delete(a.inflight, key)
				a.mu.Unlock()
				cancel()
			}()
			result, err := a.serve(ctx, m)
			resp := rpcMessage{ID: m.ID}
			var rpcErr *acp.RPCError
			switch {
			case err == nil:
				if resp.Result, err = json.Marshal(result); err != nil {
					panic(err)
				}
			case errors.As(err, &rpcErr):
				resp.Error = rpcErr
			case errors.Is(err, context.Canceled):
				resp.Error = &acp.RPCError{Code: acp.CodeRequestCancelled, Message: "request cancelled"}
			default:
				resp.Error = &acp.RPCError{Code: acp.CodeInternalError, Message: err.Error()}
			}
			a.send(resp)
			if resp.Error == nil {
				a.afterOpen(m, resp)
			}
		}()
	}
}

// afterOpen advertises commands once a session is open, after the answer,
// as OpenCode and omp do.
func (a *agent) afterOpen(m, resp rpcMessage) {
	var p struct {
		SessionID string `json:"sessionId"`
	}
	switch m.Method {
	case acp.MethodSessionNew:
		if json.Unmarshal(resp.Result, &p) != nil {
			return
		}
	case acp.MethodSessionLoad, acp.MethodSessionResume:
		if json.Unmarshal(m.Params, &p) != nil {
			return
		}
	default:
		return
	}
	a.commands(p.SessionID)
}

func (a *agent) commands(sid string) {
	a.update(sid, &acp.AvailableCommandsUpdate{AvailableCommands: []acp.AvailableCommand{
		{Name: "compact", Description: "Compact the context"},
		{Name: "plan", Description: "Show a plan", Input: &acp.AvailableCommandInput{Hint: "goal"}},
	}})
}

func (a *agent) notification(m rpcMessage) {
	switch m.Method {
	case acp.MethodCancelRequest:
		var p acp.CancelRequestNotification
		if json.Unmarshal(m.Params, &p) != nil {
			return
		}
		a.mu.Lock()
		cancel := a.inflight[string(p.RequestID)]
		a.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	case acp.MethodSessionCancel:
		var p acp.CancelNotification
		if json.Unmarshal(m.Params, &p) != nil {
			return
		}
		a.mu.Lock()
		if ls := a.live[p.SessionID]; ls != nil && ls.turn != nil {
			ls.turn()
		}
		a.mu.Unlock()
	}
}

// serve answers one client request after the configured fault injection.
func (a *agent) serve(ctx context.Context, m rpcMessage) (any, error) {
	if d, ok := a.cfg.delays[m.Method]; ok {
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if code, ok := a.cfg.fails[m.Method]; ok {
		return nil, &acp.RPCError{Code: code, Message: "injected failure in " + m.Method}
	}
	if m.Method != acp.MethodSessionPrompt {
		defer crashAt(a.cfg, m.Method)
	}
	switch m.Method {
	case acp.MethodInitialize:
		return a.initialize()
	case acp.MethodSessionNew:
		var p acp.NewSessionRequest
		if err := decodeParams(m.Params, &p); err != nil {
			return nil, err
		}
		ss, err := a.st.create(p.Cwd)
		if err != nil {
			return nil, err
		}
		if ss, err = a.open(ss.ID, ss); err != nil {
			return nil, err
		}
		return acp.NewSessionResponse{SessionID: ss.ID, ConfigOptions: configOptions(ss)}, nil
	case acp.MethodSessionLoad:
		var p acp.LoadSessionRequest
		if err := decodeParams(m.Params, &p); err != nil {
			return nil, err
		}
		ss, err := a.find(p.SessionID)
		if err != nil {
			return nil, err
		}
		if ss, err = a.open(p.SessionID, ss); err != nil {
			return nil, err
		}
		for _, msg := range ss.Messages {
			chunk := acp.ContentChunk{Content: acp.TextBlock(msg.Text), MessageID: msg.ID}
			if msg.Role == "user" {
				a.update(p.SessionID, (*acp.UserMessageChunk)(&chunk))
			} else {
				a.update(p.SessionID, (*acp.AgentMessageChunk)(&chunk))
			}
		}
		if a.cfg.profile == "omp" {
			// omp loads a session in its default mode (baseline D25).
			a.mu.Lock()
			ss.Config["mode"] = defaultConfig()["mode"]
			a.mu.Unlock()
		}
		a.mu.Lock()
		options := configOptions(ss)
		a.mu.Unlock()
		return acp.LoadSessionResponse{ConfigOptions: options}, nil
	case acp.MethodSessionResume:
		var p acp.ResumeSessionRequest
		if err := decodeParams(m.Params, &p); err != nil {
			return nil, err
		}
		ss, err := a.find(p.SessionID)
		if err != nil {
			return nil, err
		}
		if ss, err = a.open(p.SessionID, ss); err != nil {
			return nil, err
		}
		return acp.ResumeSessionResponse{ConfigOptions: configOptions(ss)}, nil
	case acp.MethodSessionList:
		var p acp.ListSessionsRequest
		if err := decodeParams(m.Params, &p); err != nil {
			return nil, err
		}
		return a.list(p)
	case acp.MethodSessionClose:
		var p acp.CloseSessionRequest
		if err := decodeParams(m.Params, &p); err != nil {
			return nil, err
		}
		a.mu.Lock()
		ls := a.live[p.SessionID]
		delete(a.live, p.SessionID)
		a.mu.Unlock()
		if ls == nil {
			return nil, &acp.RPCError{Code: acp.CodeResourceNotFound, Message: "session not open"}
		}
		if ls.turn != nil {
			ls.turn()
		}
		a.st.release(ls.data.ID)
		return struct{}{}, nil
	case acp.MethodSessionPrompt:
		var p acp.PromptRequest
		if err := decodeParams(m.Params, &p); err != nil {
			return nil, err
		}
		return a.prompt(p)
	case acp.MethodSessionSetConfigOption:
		var p acp.SetConfigOptionRequest
		if err := decodeParams(m.Params, &p); err != nil {
			return nil, err
		}
		return a.setConfig(p)
	case methodSteering:
		if a.cfg.profile != "omp" {
			break
		}
		var p acp.PromptRequest
		if err := decodeParams(m.Params, &p); err != nil {
			return nil, err
		}
		ls := a.session(p.SessionID)
		if ls == nil {
			return nil, &acp.RPCError{Code: acp.CodeResourceNotFound, Message: "session not open"}
		}
		a.mu.Lock()
		ls.data.Messages = append(ls.data.Messages, message{ID: newMessageID(), Role: "user", Text: promptText(p.Prompt)})
		err := a.st.save(ls.data)
		a.mu.Unlock()
		return struct{}{}, err
	}
	return nil, &acp.RPCError{Code: acp.CodeMethodNotFound, Message: "method not found"}
}

func decodeParams(params json.RawMessage, v any) error {
	if err := json.Unmarshal(params, v); err != nil {
		return &acp.RPCError{Code: acp.CodeInvalidParams, Message: "invalid params"}
	}
	return nil
}

func (a *agent) initialize() (any, error) {
	resp := acp.InitializeResponse{
		ProtocolVersion: a.cfg.protocolVersion,
		AgentCapabilities: acp.AgentCapabilities{
			LoadSession: true,
			SessionCapabilities: acp.SessionCapabilities{
				List:   &acp.Capability{},
				Resume: &acp.Capability{},
				Close:  &acp.Capability{},
			},
		},
		AgentInfo: &acp.Implementation{Name: a.cfg.profile, Version: pluginVersion},
	}
	if a.cfg.noResume {
		resp.AgentCapabilities.SessionCapabilities.Resume = nil
	}
	if a.cfg.profile == "omp" {
		resp.Meta = json.RawMessage(`{"steering":{"supported":true}}`)
	}
	return resp, nil
}

func (a *agent) find(id string) (*session, error) {
	ss, err := a.st.load(id)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, &acp.RPCError{Code: acp.CodeResourceNotFound, Message: "session not found"}
	}
	return ss, err
}

// open leases ss for this process under the client's ID key; a lease
// conflict forks it.
func (a *agent) open(key string, ss *session) (*session, error) {
	a.mu.Lock()
	if ls := a.live[key]; ls != nil {
		a.mu.Unlock()
		return ls.data, nil
	}
	a.mu.Unlock()
	ss, err := a.st.lease(ss)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.live[key] = &liveSession{data: ss}
	a.mu.Unlock()
	return ss, nil
}

func (a *agent) session(id string) *liveSession {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.live[id]
}

func (a *agent) list(p acp.ListSessionsRequest) (any, error) {
	all, err := a.st.list(p.Cwd)
	if err != nil {
		return nil, err
	}
	const page = 100
	start := 0
	if p.Cursor != "" {
		if start, err = strconv.Atoi(p.Cursor); err != nil || start < 0 || start > len(all) {
			return nil, &acp.RPCError{Code: acp.CodeInvalidParams, Message: "bad cursor"}
		}
	}
	resp := acp.ListSessionsResponse{Sessions: []acp.SessionInfo{}}
	for _, ss := range all[start:min(start+page, len(all))] {
		resp.Sessions = append(resp.Sessions, acp.SessionInfo{SessionID: ss.ID, Cwd: ss.Cwd, Title: ss.Title, UpdatedAt: ss.UpdatedAt})
	}
	if start+page < len(all) {
		resp.NextCursor = strconv.Itoa(start + page)
	}
	return resp, nil
}

func configOptions(ss *session) []acp.ConfigOption {
	return []acp.ConfigOption{
		{ID: "mode", Name: "Mode", Category: "mode", Type: acp.ConfigSelect, Current: acp.ConfigValue{ID: ss.Config["mode"]}, Options: []acp.ConfigSelectOption{
			{Value: "code", Name: "Code"}, {Value: "plan", Name: "Plan"},
		}},
		{ID: "model", Name: "Model", Category: "model", Type: acp.ConfigSelect, Current: acp.ConfigValue{ID: ss.Config["model"]}, Options: []acp.ConfigSelectOption{
			{Value: "fast", Name: "Fast", Group: "builtin", GroupName: "Built-in"},
			{Value: "smart", Name: "Smart", Group: "builtin", GroupName: "Built-in"},
		}},
	}
}

func (a *agent) setConfig(p acp.SetConfigOptionRequest) (any, error) {
	ls := a.session(p.SessionID)
	if ls == nil {
		return nil, &acp.RPCError{Code: acp.CodeResourceNotFound, Message: "session not open"}
	}
	valid := map[string][]string{"mode": {"code", "plan"}, "model": {"fast", "smart"}}[p.ConfigID]
	ok := false
	for _, v := range valid {
		ok = ok || v == p.Value.ID
	}
	if !ok || p.Value.Bool != nil {
		return nil, &acp.RPCError{Code: acp.CodeInvalidParams, Message: "unknown config value"}
	}
	a.mu.Lock()
	ls.data.Config[p.ConfigID] = p.Value.ID
	err := a.st.save(ls.data)
	opts := configOptions(ls.data)
	a.mu.Unlock()
	return acp.SetConfigOptionResponse{ConfigOptions: opts}, err
}

func promptText(blocks []acp.ContentBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.Type == acp.ContentText {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// prompt runs one turn; session/cancel or session/close ends it with
// stopReason cancelled.
func (a *agent) prompt(p acp.PromptRequest) (any, error) {
	ls := a.session(p.SessionID)
	if ls == nil {
		return nil, &acp.RPCError{Code: acp.CodeResourceNotFound, Message: "session not open"}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.mu.Lock()
	if ls.turn != nil {
		a.mu.Unlock()
		return nil, &acp.RPCError{Code: acp.CodeInvalidRequest, Message: "turn already running"}
	}
	ls.turn = cancel
	text := promptText(p.Prompt)
	ls.data.Messages = append(ls.data.Messages, message{ID: newMessageID(), Role: "user", Text: text})
	err := a.st.save(ls.data)
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		ls.turn = nil
		a.mu.Unlock()
	}()
	if err != nil {
		return nil, err
	}
	sid := p.SessionID
	reply, stop := a.turn(ctx, sid, text)
	if reply != "" {
		a.mu.Lock()
		ls.data.Messages = append(ls.data.Messages, message{ID: newMessageID(), Role: "agent", Text: reply})
		err = a.st.save(ls.data)
		a.mu.Unlock()
	}
	crashAt(a.cfg, acp.MethodSessionPrompt)
	return acp.PromptResponse{StopReason: stop}, err
}

// pause waits one streaming step and reports whether the turn may go on.
func (a *agent) pause(ctx context.Context) bool {
	select {
	case <-time.After(a.cfg.turnDelay):
		return true
	case <-ctx.Done():
		return false
	}
}

func (a *agent) turn(ctx context.Context, sid, text string) (reply, stop string) {
	thought := acp.ContentChunk{Content: acp.TextBlock("thinking about it")}
	a.update(sid, (*acp.AgentThoughtChunk)(&thought))
	crashAt(a.cfg, "prompt-mid")
	if !a.pause(ctx) {
		return "", acp.StopCancelled
	}
	if strings.Contains(text, "/plan") {
		a.update(sid, &acp.Plan{Entries: []acp.PlanEntry{
			{Content: "read", Priority: "high", Status: "completed"},
			{Content: "answer", Priority: "medium", Status: "in_progress"},
		}})
	}
	if strings.Contains(text, "/notice") {
		a.update(sid, &acp.Notice{Severity: acp.NoticeWarning, Title: "rate limited", Description: "retrying"})
	}
	if strings.Contains(text, "/title") {
		title := "fake title"
		a.update(sid, &acp.SessionInfoUpdate{Title: acp.StringPatch{Set: true, Value: &title}})
	}
	if strings.Contains(text, "/unknown") {
		a.update(sid, &acp.UnknownUpdate{Kind: "future_kind", Raw: json.RawMessage(`{"sessionUpdate":"future_kind","x":1}`)})
	}
	if strings.Contains(text, "/compact") {
		a.update(sid, &acp.CompactionUpdate{CompactionID: "cmp_1", Status: acp.CompactionInProgress})
		a.update(sid, &acp.CompactionUpdate{CompactionID: "cmp_1", Status: acp.CompactionCompleted, Summary: []acp.ContentBlock{acp.TextBlock("summary")}})
	}
	extra := ""
	if strings.Contains(text, "/fs") {
		err := a.request("fs/read_text_file", map[string]any{"sessionId": sid, "path": "/etc/hostname"}, nil)
		var rpcErr *acp.RPCError
		if errors.As(err, &rpcErr) {
			extra = fmt.Sprintf(" (fs error %d)", rpcErr.Code)
		}
	}

	title, inProgress, failed := "Read file", acp.ToolInProgress, acp.ToolFailed
	a.update(sid, &acp.ToolCall{ToolCallID: "call_1", Title: title, Kind: "read", Status: acp.ToolPending,
		Locations: []acp.ToolCallLocation{{Path: "README.md"}}})
	rejected := false
	if a.cfg.permission {
		var resp acp.RequestPermissionResponse
		err := a.request(acp.MethodRequestPermission, acp.RequestPermissionRequest{
			SessionID: sid,
			ToolCall:  acp.ToolCallUpdate{ToolCallID: "call_1", Title: &title},
			Options: []acp.PermissionOption{
				{OptionID: "allow", Name: "Allow", Kind: acp.PermissionAllowOnce},
				{OptionID: "reject", Name: "Reject", Kind: acp.PermissionRejectOnce},
			},
		}, &resp)
		outcome := resp.Outcome.Outcome
		if resp.Outcome.OptionID != "" {
			outcome = resp.Outcome.OptionID
		}
		if err != nil {
			outcome = "error"
		}
		a.st.event("permission "+outcome, sid)
		if err != nil || resp.Outcome.Outcome == acp.OutcomeCancelled || ctx.Err() != nil {
			a.update(sid, &acp.ToolCallUpdate{ToolCallID: "call_1", Status: &failed})
			return "", acp.StopCancelled
		}
		if resp.Outcome.OptionID == "reject" {
			a.update(sid, &acp.ToolCallUpdate{ToolCallID: "call_1", Status: &failed})
			rejected = true
			extra += " (rejected)"
		}
	}
	if !rejected {
		a.update(sid, &acp.ToolCallUpdate{ToolCallID: "call_1", Status: &inProgress})
		if !a.pause(ctx) {
			return "", acp.StopCancelled
		}
		done := acp.ToolCompleted
		out := acp.TextBlock("file contents")
		a.update(sid, &acp.ToolCallUpdate{ToolCallID: "call_1", Status: &done,
			Content: []acp.ToolCallContent{{Type: acp.ToolContentContent, Content: &out}}})
	}
	if a.cfg.elicit {
		var resp acp.CreateElicitationResponse
		err := a.request(acp.MethodElicitationCreate, acp.CreateElicitationRequest{
			Mode: acp.ElicitationForm, Message: "Who are you?", SessionID: sid,
			RequestedSchema: &acp.ElicitationSchema{Type: "object", Properties: map[string]acp.ElicitationProperty{
				"name": {Type: "string", Title: "Name"},
			}, Required: []string{"name"}},
		}, &resp)
		a.st.event("elicitation "+resp.Action, sid)
		switch {
		case err != nil || ctx.Err() != nil || resp.Action == acp.ElicitCancel:
			return "", acp.StopCancelled
		case resp.Action == acp.ElicitAccept:
			extra += fmt.Sprintf(" (hello %v)", resp.Content["name"])
		default:
			extra += " (declined)"
		}
	}

	reply = "echo: " + text + extra
	id := newMessageID()
	half := len(reply) / 2
	for _, piece := range []string{reply[:half], reply[half:]} {
		if !a.pause(ctx) {
			return "", acp.StopCancelled
		}
		chunk := acp.ContentChunk{Content: acp.TextBlock(piece), MessageID: id}
		a.update(sid, (*acp.AgentMessageChunk)(&chunk))
	}
	a.update(sid, &acp.UsageUpdate{Used: int64(len(reply)), Size: 200000, Cost: &acp.Cost{Amount: 0.01, Currency: "USD"}})
	return reply, acp.StopEndTurn
}
