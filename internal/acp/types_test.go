package acp

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

// jsonEqual compares two JSON documents structurally.
func jsonEqual(t *testing.T, got, want []byte) bool {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("got invalid JSON %s: %v", got, err)
	}
	if err := json.Unmarshal(want, &w); err != nil {
		t.Fatalf("want invalid JSON %s: %v", want, err)
	}
	return reflect.DeepEqual(g, w)
}

func TestSessionUpdateDecodeEncode(t *testing.T) {
	tests := []struct {
		name string
		give string // the update object as an agent sends it
		want Update
	}{
		{
			name: "user message chunk",
			give: `{"sessionUpdate":"user_message_chunk","content":{"type":"text","text":"hi"},"messageId":"m1"}`,
			want: &UserMessageChunk{Content: TextBlock("hi"), MessageID: "m1"},
		},
		{
			name: "agent message chunk without message id",
			give: `{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":""}}`,
			want: &AgentMessageChunk{Content: TextBlock("")},
		},
		{
			name: "agent thought chunk with resource link",
			give: `{"sessionUpdate":"agent_thought_chunk","content":{"type":"resource_link","uri":"file:///a","name":"a","mimeType":"text/plain","size":3}}`,
			want: &AgentThoughtChunk{Content: ContentBlock{Type: ContentResourceLink, URI: "file:///a", Name: "a", MimeType: "text/plain", Size: new(int64(3))}},
		},
		{
			name: "tool call",
			give: `{"sessionUpdate":"tool_call","toolCallId":"c1","title":"Read","kind":"read","status":"pending",
				"content":[{"type":"content","content":{"type":"text","text":"x"}},{"type":"diff","path":"/a","newText":""},{"type":"terminal","terminalId":"t"}],
				"locations":[{"path":"/a","line":2}],"rawInput":{"path":"/a"}}`,
			want: &ToolCall{ToolCallID: "c1", Title: "Read", Kind: "read", Status: ToolPending,
				Content: []ToolCallContent{
					{Type: ToolContentContent, Content: new(TextBlock("x"))},
					{Type: ToolContentDiff, Path: "/a"},
					{Type: ToolContentTerminal, TerminalID: "t"},
				},
				Locations: []ToolCallLocation{{Path: "/a", Line: new(2)}},
				RawInput:  json.RawMessage(`{"path":"/a"}`)},
		},
		{
			name: "tool call update patch",
			give: `{"sessionUpdate":"tool_call_update","toolCallId":"c1","status":"completed","content":[]}`,
			want: &ToolCallUpdate{ToolCallID: "c1", Status: new(ToolCompleted), Content: []ToolCallContent{}},
		},
		{
			name: "plan",
			give: `{"sessionUpdate":"plan","entries":[{"content":"a","priority":"high","status":"in_progress"}]}`,
			want: &Plan{Entries: []PlanEntry{{Content: "a", Priority: "high", Status: "in_progress"}}},
		},
		{
			name: "available commands",
			give: `{"sessionUpdate":"available_commands_update","availableCommands":[{"name":"plan","description":"d","input":{"hint":"goal"}}]}`,
			want: &AvailableCommandsUpdate{AvailableCommands: []AvailableCommand{{Name: "plan", Description: "d", Input: &AvailableCommandInput{Hint: "goal"}}}},
		},
		{
			name: "current mode",
			give: `{"sessionUpdate":"current_mode_update","currentModeId":"plan"}`,
			want: &CurrentModeUpdate{CurrentModeID: "plan"},
		},
		{
			name: "config options",
			give: `{"sessionUpdate":"config_option_update","configOptions":[
				{"id":"model","name":"Model","category":"model","type":"select","currentValue":"b","options":[{"group":"g","name":"G","options":[{"value":"a","name":"A"},{"value":"b","name":"B"}]}]},
				{"id":"auto","name":"Auto","type":"boolean","currentValue":false}]}`,
			want: &ConfigOptionUpdate{ConfigOptions: []ConfigOption{
				{ID: "model", Name: "Model", Category: "model", Type: ConfigSelect, Current: ConfigValue{ID: "b"}, Options: []ConfigSelectOption{
					{Value: "a", Name: "A", Group: "g", GroupName: "G"}, {Value: "b", Name: "B", Group: "g", GroupName: "G"},
				}},
				{ID: "auto", Name: "Auto", Type: ConfigBoolean, Current: ConfigValue{Bool: new(false)}},
			}},
		},
		{
			name: "session info clears title",
			give: `{"sessionUpdate":"session_info_update","title":null,"updatedAt":"2026-10-28T00:00:00Z"}`,
			want: &SessionInfoUpdate{Title: StringPatch{Set: true}, UpdatedAt: StringPatch{Set: true, Value: new("2026-10-28T00:00:00Z")}},
		},
		{
			name: "session info leaves title",
			give: `{"sessionUpdate":"session_info_update"}`,
			want: &SessionInfoUpdate{},
		},
		{
			name: "usage",
			give: `{"sessionUpdate":"usage_update","used":53000,"size":200000,"cost":{"amount":0.25,"currency":"USD"}}`,
			want: &UsageUpdate{Used: 53000, Size: 200000, Cost: &Cost{Amount: 0.25, Currency: "USD"}},
		},
		{
			name: "notice",
			give: `{"sessionUpdate":"notice","severity":"warning","title":"slow","description":"d"}`,
			want: &Notice{Severity: NoticeWarning, Title: "slow", Description: "d"},
		},
		{
			name: "compaction",
			give: `{"sessionUpdate":"compaction_update","compactionId":"k","status":"completed","summary":[{"type":"text","text":"s"}]}`,
			want: &CompactionUpdate{CompactionID: "k", Status: CompactionCompleted, Summary: []ContentBlock{TextBlock("s")}},
		},
		{
			name: "compaction summary chunk",
			give: `{"sessionUpdate":"compaction_summary_chunk","compactionId":"k","content":{"type":"text","text":"s"}}`,
			want: &CompactionSummaryChunk{CompactionID: "k", Content: TextBlock("s")},
		},
		{
			name: "image content kept raw",
			give: `{"sessionUpdate":"agent_message_chunk","content":{"type":"image","data":"AAAA","mimeType":"image/png"}}`,
			want: &AgentMessageChunk{Content: ContentBlock{Type: ContentImage, MimeType: "image/png", Raw: json.RawMessage(`{"type":"image","data":"AAAA","mimeType":"image/png"}`)}},
		},
		{
			name: "unknown kind kept raw",
			give: `{"sessionUpdate":"state_update","state":{"running":true},"extra":[1,2]}`,
			want: &UnknownUpdate{Kind: "state_update", Raw: json.RawMessage(`{"sessionUpdate":"state_update","state":{"running":true},"extra":[1,2]}`)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var compact bytes.Buffer
			if err := json.Compact(&compact, []byte(tt.give)); err != nil {
				t.Fatal(err)
			}
			give := compact.Bytes()
			params := []byte(`{"sessionId":"s1","update":` + string(give) + `}`)
			var n SessionNotification
			if err := json.Unmarshal(params, &n); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if n.SessionID != "s1" {
				t.Errorf("sessionId = %q", n.SessionID)
			}
			if !reflect.DeepEqual(n.Update, tt.want) {
				t.Errorf("decoded\n%#v\nwant\n%#v", n.Update, tt.want)
			}
			if n.Update.UpdateKind() != tt.want.UpdateKind() {
				t.Errorf("kind %q, want %q", n.Update.UpdateKind(), tt.want.UpdateKind())
			}
			out, err := json.Marshal(n)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if !jsonEqual(t, out, params) {
				t.Errorf("re-encoded\n%s\nwant\n%s", out, params)
			}
		})
	}
}

func TestSessionUpdateRejects(t *testing.T) {
	tests := []struct {
		name string
		give string
	}{
		{"known kind with wrong field type", `{"sessionId":"s1","update":{"sessionUpdate":"usage_update","used":"many","size":1}}`},
		{"missing session id", `{"update":{"sessionUpdate":"plan","entries":[]}}`},
		{"missing update", `{"sessionId":"s1"}`},
		{"update not an object", `{"sessionId":"s1","update":7}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var n SessionNotification
			if err := json.Unmarshal([]byte(tt.give), &n); err == nil {
				t.Fatalf("decoded %#v, want error", n)
			}
		})
	}
}

func TestEncodeRequests(t *testing.T) {
	tests := []struct {
		name string
		give any
		want string
	}{
		{
			name: "initialize advertises notices, compaction and form only",
			give: InitializeRequest{ProtocolVersion: ProtocolVersion, ClientCapabilities: clientCapabilities(), ClientInfo: &Implementation{Name: "webmux", Version: "1.2.3"}},
			want: `{"protocolVersion":1,"clientCapabilities":{"session":{"notices":{},"compaction":{}},"elicitation":{"form":{}}},"clientInfo":{"name":"webmux","version":"1.2.3"}}`,
		},
		{
			name: "new session sends mcpServers",
			give: NewSessionRequest{Cwd: "/w", MCPServers: []json.RawMessage{}},
			want: `{"cwd":"/w","mcpServers":[]}`,
		},
		{
			name: "list without filter",
			give: ListSessionsRequest{},
			want: `{}`,
		},
		{
			name: "prompt",
			give: PromptRequest{SessionID: "s1", Prompt: []ContentBlock{TextBlock("hi")}},
			want: `{"sessionId":"s1","prompt":[{"type":"text","text":"hi"}]}`,
		},
		{
			name: "select config value",
			give: SetConfigOptionRequest{SessionID: "s1", ConfigID: "model", Value: ConfigValue{ID: "fast"}},
			want: `{"sessionId":"s1","configId":"model","value":"fast"}`,
		},
		{
			name: "boolean config value",
			give: SetConfigOptionRequest{SessionID: "s1", ConfigID: "auto", Value: ConfigValue{Bool: new(true)}},
			want: `{"sessionId":"s1","configId":"auto","type":"boolean","value":true}`,
		},
		{
			name: "permission selected",
			give: RequestPermissionResponse{Outcome: PermissionOutcome{Outcome: OutcomeSelected, OptionID: "allow"}},
			want: `{"outcome":{"outcome":"selected","optionId":"allow"}}`,
		},
		{
			name: "permission cancelled",
			give: RequestPermissionResponse{Outcome: PermissionOutcome{Outcome: OutcomeCancelled}},
			want: `{"outcome":{"outcome":"cancelled"}}`,
		},
		{
			name: "elicitation accepted",
			give: CreateElicitationResponse{Action: ElicitAccept, Content: map[string]any{"name": "Ann", "n": 2, "tags": []string{"a"}}},
			want: `{"action":"accept","content":{"name":"Ann","n":2,"tags":["a"]}}`,
		},
		{
			name: "elicitation cancelled",
			give: CreateElicitationResponse{Action: ElicitCancel},
			want: `{"action":"cancel"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.give)
			if err != nil {
				t.Fatal(err)
			}
			if !jsonEqual(t, got, []byte(tt.want)) {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestDecodeResponsesAndRequests(t *testing.T) {
	tests := []struct {
		name string
		give string
		into func() any
		want any
	}{
		{
			name: "initialize capabilities",
			give: `{"protocolVersion":1,"agentCapabilities":{"loadSession":true,"promptCapabilities":{"image":true},
				"sessionCapabilities":{"list":{},"resume":{},"close":{},"fork":{}},"_meta":{"x":1}},
				"agentInfo":{"name":"omp","version":"18.13.1"},"authMethods":[{"id":"login","name":"Log in"}],"_meta":{"steering":{"supported":true}}}`,
			into: func() any { return &InitializeResponse{} },
			want: &InitializeResponse{ProtocolVersion: 1, AgentCapabilities: AgentCapabilities{
				LoadSession:         true,
				PromptCapabilities:  PromptCapabilities{Image: true},
				SessionCapabilities: SessionCapabilities{List: &Capability{}, Resume: &Capability{}, Close: &Capability{}},
				Meta:                json.RawMessage(`{"x":1}`),
			}, AgentInfo: &Implementation{Name: "omp", Version: "18.13.1"}, AuthMethods: []AuthMethod{{ID: "login", Name: "Log in"}},
				Meta: json.RawMessage(`{"steering":{"supported":true}}`)},
		},
		{
			name: "null capability is unsupported",
			give: `{"protocolVersion":1,"agentCapabilities":{"sessionCapabilities":{"list":null}}}`,
			into: func() any { return &InitializeResponse{} },
			want: &InitializeResponse{ProtocolVersion: 1},
		},
		{
			name: "list page",
			give: `{"sessions":[{"sessionId":"s1","cwd":"/w","title":null,"updatedAt":"t"}],"nextCursor":"c2"}`,
			into: func() any { return &ListSessionsResponse{} },
			want: &ListSessionsResponse{Sessions: []SessionInfo{{SessionID: "s1", Cwd: "/w", UpdatedAt: "t"}}, NextCursor: "c2"},
		},
		{
			name: "load with modes and flat options",
			give: `{"modes":{"currentModeId":"code","availableModes":[{"id":"code","name":"Code"}]},
				"configOptions":[{"id":"mode","name":"Mode","type":"select","currentValue":"code","options":[{"value":"code","name":"Code","description":"d"}]},
				{"id":"future","name":"F","type":"slider","currentValue":3}]}`,
			into: func() any { return &LoadSessionResponse{} },
			want: &LoadSessionResponse{
				Modes: &SessionModeState{CurrentModeID: "code", AvailableModes: []SessionMode{{ID: "code", Name: "Code"}}},
				ConfigOptions: []ConfigOption{
					{ID: "mode", Name: "Mode", Type: ConfigSelect, Current: ConfigValue{ID: "code"}, Options: []ConfigSelectOption{{Value: "code", Name: "Code", Description: "d"}}},
					{ID: "future", Name: "F", Type: "slider"},
				},
			},
		},
		{
			name: "prompt result",
			give: `{"stopReason":"max_tokens"}`,
			into: func() any { return &PromptResponse{} },
			want: &PromptResponse{StopReason: StopMaxTokens},
		},
		{
			name: "permission request",
			give: `{"sessionId":"s1","toolCall":{"toolCallId":"c1","title":"Edit","kind":"edit"},
				"options":[{"optionId":"a","name":"Allow","kind":"allow_once"},{"optionId":"r","name":"Reject","kind":"reject_always"}]}`,
			into: func() any { return &RequestPermissionRequest{} },
			want: &RequestPermissionRequest{SessionID: "s1", ToolCall: ToolCallUpdate{ToolCallID: "c1", Title: new("Edit"), Kind: new("edit")},
				Options: []PermissionOption{{OptionID: "a", Name: "Allow", Kind: PermissionAllowOnce}, {OptionID: "r", Name: "Reject", Kind: PermissionRejectAlways}}},
		},
		{
			name: "form elicitation",
			give: `{"mode":"form","message":"Pick","sessionId":"s1","toolCallId":"c1","requestedSchema":{"type":"object",
				"properties":{"color":{"type":"string","oneOf":[{"const":"r","title":"Red"}]},"n":{"type":"integer","minimum":1}},"required":["color"]}}`,
			into: func() any { return &CreateElicitationRequest{} },
			want: &CreateElicitationRequest{Mode: ElicitationForm, Message: "Pick", SessionID: "s1", ToolCallID: "c1", RequestedSchema: &ElicitationSchema{
				Type: "object", Required: []string{"color"}, Properties: map[string]ElicitationProperty{
					"color": {Type: "string", OneOf: []EnumOption{{Const: "r", Title: "Red"}}},
					"n":     {Type: "integer", Minimum: new(1.0)},
				}}},
		},
		{
			name: "boolean config request",
			give: `{"sessionId":"s1","configId":"auto","type":"boolean","value":true}`,
			into: func() any { return &SetConfigOptionRequest{} },
			want: &SetConfigOptionRequest{SessionID: "s1", ConfigID: "auto", Value: ConfigValue{Bool: new(true)}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.into()
			if err := json.Unmarshal([]byte(tt.give), got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got\n%#v\nwant\n%#v", got, tt.want)
			}
		})
	}
}

func TestConfigOptionRoundTrip(t *testing.T) {
	tests := []ConfigOption{
		{ID: "m", Name: "M", Type: ConfigSelect, Current: ConfigValue{ID: "a"}, Options: []ConfigSelectOption{{Value: "a", Name: "A"}}},
		{ID: "m", Name: "M", Type: ConfigSelect, Current: ConfigValue{ID: "b"}, Options: []ConfigSelectOption{
			{Value: "a", Name: "A", Group: "g1", GroupName: "One"}, {Value: "b", Name: "B", Group: "g2", GroupName: "Two"},
		}},
		{ID: "t", Name: "T", Description: "d", Category: "thought_level", Type: ConfigBoolean, Current: ConfigValue{Bool: new(true)}},
	}
	for _, give := range tests {
		b, err := json.Marshal(give)
		if err != nil {
			t.Fatal(err)
		}
		var got ConfigOption
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, give) {
			t.Errorf("round trip of %s\ngot  %#v\nwant %#v", b, got, give)
		}
	}
}
