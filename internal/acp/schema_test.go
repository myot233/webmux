package acp

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

// schemaDoc is the pinned ACP schema (testdata/schema.json, tag
// schema-v1.27.0) reduced to what the guard needs.
type schemaDoc struct {
	defs map[string]any
}

func loadSchema(t *testing.T) *schemaDoc {
	t.Helper()
	b, err := os.ReadFile("testdata/schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var root struct {
		Defs map[string]any `json:"$defs"`
	}
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	return &schemaDoc{defs: root.Defs}
}

func (s *schemaDoc) resolve(node map[string]any) map[string]any {
	for {
		ref, ok := node["$ref"].(string)
		if !ok {
			return node
		}
		def, ok := s.defs[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any)
		if !ok {
			return map[string]any{}
		}
		node = def
	}
}

func jsonType(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		if x == math.Trunc(x) {
			return "integer"
		}
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	default:
		return "object"
	}
}

// typeFits reports whether v may match node by its declared type alone.
func typeFits(node map[string]any, v any) bool {
	got := jsonType(v)
	fits := func(want string) bool {
		return want == got || (want == "number" && got == "integer")
	}
	switch t := node["type"].(type) {
	case string:
		return fits(t)
	case []any:
		for _, want := range t {
			if s, ok := want.(string); ok && fits(s) {
				return true
			}
		}
		return false
	}
	if _, ok := node["properties"]; ok {
		return got == "object"
	}
	if _, ok := node["const"]; ok {
		return node["const"] == v
	}
	return true
}

// constMatch reports whether every const-valued property of alt matches v,
// and whether alt has any such discriminator.
func constMatch(alt map[string]any, v any) (match, discriminated bool) {
	props, _ := alt["properties"].(map[string]any)
	obj, _ := v.(map[string]any)
	match = true
	for k, p := range props {
		pm, _ := p.(map[string]any)
		c, ok := pm["const"]
		if !ok {
			continue
		}
		discriminated = true
		if obj == nil || obj[k] != c {
			match = false
		}
	}
	return match, discriminated
}

// candidates expands node into the schema objects that apply to v:
// itself, every allOf member, and the anyOf/oneOf alternatives v selects.
// strict reports whether the selection was unambiguous, so required keys
// can be enforced.
func (s *schemaDoc) candidates(node map[string]any, v any) (out []map[string]any, strict bool) {
	node = s.resolve(node)
	out = append(out, node)
	strict = true
	for _, m := range anyList(node["allOf"]) {
		sub, subStrict := s.candidates(m, v)
		out = append(out, sub...)
		strict = strict && subStrict
	}
	for _, key := range []string{"anyOf", "oneOf"} {
		alts := anyList(node[key])
		if len(alts) == 0 {
			continue
		}
		var byConst, plain []map[string]any
		for _, alt := range alts {
			alt = s.resolve(alt)
			if !typeFits(alt, v) {
				continue
			}
			match, disc := constMatch(alt, v)
			switch {
			case disc && match:
				byConst = append(byConst, alt)
			case !disc && alt["not"] == nil:
				plain = append(plain, alt)
			}
		}
		chosen := byConst
		if len(chosen) == 0 {
			chosen = plain
		}
		if len(chosen) != 1 {
			strict = false
		}
		for _, alt := range chosen {
			sub, subStrict := s.candidates(alt, v)
			out = append(out, sub...)
			strict = strict && subStrict
		}
	}
	return out, strict
}

func anyList(v any) []map[string]any {
	list, _ := v.([]any)
	var out []map[string]any
	for _, x := range list {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// check walks v against node and reports every key the schema does not
// define and every required key v lacks.
func (s *schemaDoc) check(path string, node map[string]any, v any, errs *[]string) {
	cands, strict := s.candidates(node, v)
	switch x := v.(type) {
	case map[string]any:
		props := map[string][]map[string]any{}
		var additional []map[string]any
		open, structured := false, false
		for _, c := range cands {
			if ps, ok := c["properties"].(map[string]any); ok {
				structured = true
				for k, p := range ps {
					if pm, ok := p.(map[string]any); ok {
						props[k] = append(props[k], pm)
					}
				}
			}
			switch ap := c["additionalProperties"].(type) {
			case bool:
				open = open || ap
			case map[string]any:
				additional = append(additional, ap)
			}
			if strict {
				required, _ := c["required"].([]any)
				for _, r := range required {
					if _, ok := x[r.(string)]; !ok {
						*errs = append(*errs, fmt.Sprintf("%s: missing required %q", path, r))
					}
				}
			}
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			switch {
			case len(props[k]) > 0:
				s.check(path+"."+k, map[string]any{"allOf": toAnySlice(props[k])}, x[k], errs)
			case len(additional) > 0:
				s.check(path+"."+k, map[string]any{"allOf": toAnySlice(additional)}, x[k], errs)
			case open || !structured:
			default:
				*errs = append(*errs, fmt.Sprintf("%s: field %q is not in the schema", path, k))
			}
		}
	case []any:
		var items []map[string]any
		for _, c := range cands {
			if it, ok := c["items"].(map[string]any); ok {
				items = append(items, it)
			}
		}
		if len(items) == 0 {
			return
		}
		for i, el := range x {
			s.check(fmt.Sprintf("%s[%d]", path, i), map[string]any{"anyOf": toAnySlice(items)}, el, errs)
		}
	}
}

func toAnySlice(ms []map[string]any) []any {
	out := make([]any, len(ms))
	for i, m := range ms {
		out[i] = m
	}
	return out
}

// guardSamples has a fully populated value of every type this package
// encodes or decodes, keyed by its schema definition. Every json tag in
// these types therefore appears in at least one sample.
func guardSamples() []struct {
	def   string
	value any
} {
	text := TextBlock("hi")
	link := ContentBlock{Type: ContentResourceLink, URI: "file:///a", Name: "a", Title: "A", Description: "d", MimeType: "text/plain", Size: new(int64(3))}
	content := []ToolCallContent{
		{Type: ToolContentContent, Content: &text},
		{Type: ToolContentDiff, Path: "/a", OldText: new("x"), NewText: "y"},
		{Type: ToolContentTerminal, TerminalID: "t1"},
	}
	locations := []ToolCallLocation{{Path: "/a", Line: new(3)}}
	selectOpt := ConfigOption{ID: "model", Name: "Model", Description: "d", Category: "model", Type: ConfigSelect, Current: ConfigValue{ID: "fast"},
		Options: []ConfigSelectOption{{Value: "fast", Name: "Fast", Description: "quick"}}}
	groupOpt := ConfigOption{ID: "model", Name: "Model", Type: ConfigSelect, Current: ConfigValue{ID: "fast"},
		Options: []ConfigSelectOption{{Value: "fast", Name: "Fast", Description: "quick", Group: "g", GroupName: "G"}}}
	boolOpt := ConfigOption{ID: "auto", Name: "Auto", Type: ConfigBoolean, Current: ConfigValue{Bool: new(true)}}
	modes := &SessionModeState{CurrentModeID: "code", AvailableModes: []SessionMode{{ID: "code", Name: "Code", Description: "d"}}}
	configs := []ConfigOption{selectOpt, groupOpt, boolOpt}
	meta := json.RawMessage(`{"x":1}`)
	toolUpdate := ToolCallUpdate{ToolCallID: "c1", Title: new("t"), Kind: new("read"), Status: new(ToolCompleted),
		Content: content, Locations: locations, RawInput: json.RawMessage(`{"a":1}`), RawOutput: json.RawMessage(`"o"`)}
	note := func(u Update) SessionNotification { return SessionNotification{SessionID: "s1", Update: u} }
	return []struct {
		def   string
		value any
	}{
		{"InitializeRequest", InitializeRequest{ProtocolVersion: 1, ClientCapabilities: clientCapabilities(), ClientInfo: &Implementation{Name: "webmux", Title: "WebMux", Version: "1"}, Meta: meta}},
		{"InitializeResponse", InitializeResponse{ProtocolVersion: 1, AgentCapabilities: AgentCapabilities{
			LoadSession:         true,
			PromptCapabilities:  PromptCapabilities{Image: true, Audio: true, EmbeddedContext: true},
			SessionCapabilities: SessionCapabilities{List: &Capability{Meta: meta}, Resume: &Capability{}, Close: &Capability{}, Delete: &Capability{}},
			Meta:                meta,
		}, AuthMethods: []AuthMethod{{ID: "login", Name: "Login", Description: "d"}}, AgentInfo: &Implementation{Name: "omp", Version: "18"}, Meta: meta}},
		{"ClientCapabilities", ClientCapabilities{
			Session:     &ClientSessionCapabilities{Notices: &Capability{}, Compaction: &Capability{}},
			Elicitation: &ElicitationCapabilities{Form: &Capability{}, URL: &Capability{}},
		}},
		{"NewSessionRequest", NewSessionRequest{Cwd: "/w", MCPServers: []json.RawMessage{}}},
		{"NewSessionResponse", NewSessionResponse{SessionID: "s1", Modes: modes, ConfigOptions: configs}},
		{"LoadSessionRequest", LoadSessionRequest{SessionID: "s1", Cwd: "/w", MCPServers: []json.RawMessage{}}},
		{"LoadSessionResponse", LoadSessionResponse{Modes: modes, ConfigOptions: configs}},
		{"ResumeSessionRequest", ResumeSessionRequest{SessionID: "s1", Cwd: "/w", MCPServers: []json.RawMessage{}}},
		{"ResumeSessionResponse", ResumeSessionResponse{Modes: modes, ConfigOptions: configs}},
		{"ListSessionsRequest", ListSessionsRequest{Cwd: "/w", Cursor: "c"}},
		{"ListSessionsResponse", ListSessionsResponse{Sessions: []SessionInfo{{SessionID: "s1", Cwd: "/w", Title: "t", UpdatedAt: "2026-10-28T00:00:00Z"}}, NextCursor: "c"}},
		{"CloseSessionRequest", CloseSessionRequest{SessionID: "s1"}},
		{"PromptRequest", PromptRequest{SessionID: "s1", Prompt: []ContentBlock{text, link}, Meta: meta}},
		{"PromptResponse", PromptResponse{StopReason: StopCancelled}},
		{"CancelNotification", CancelNotification{SessionID: "s1"}},
		{"CancelRequestNotification", CancelRequestNotification{RequestID: json.RawMessage(`7`)}},
		{"SetSessionConfigOptionRequest", SetConfigOptionRequest{SessionID: "s1", ConfigID: "model", Value: ConfigValue{ID: "fast"}}},
		{"SetSessionConfigOptionRequest", SetConfigOptionRequest{SessionID: "s1", ConfigID: "auto", Value: ConfigValue{Bool: new(false)}}},
		{"SetSessionConfigOptionResponse", SetConfigOptionResponse{ConfigOptions: configs}},
		{"RequestPermissionRequest", RequestPermissionRequest{SessionID: "s1", ToolCall: toolUpdate,
			Options: []PermissionOption{{OptionID: "a", Name: "Allow", Kind: PermissionAllowOnce}}}},
		{"RequestPermissionResponse", RequestPermissionResponse{Outcome: PermissionOutcome{Outcome: OutcomeSelected, OptionID: "a"}}},
		{"RequestPermissionResponse", RequestPermissionResponse{Outcome: PermissionOutcome{Outcome: OutcomeCancelled}}},
		{"CreateElicitationRequest", CreateElicitationRequest{Mode: ElicitationForm, Message: "m", SessionID: "s1", ToolCallID: "c1",
			RequestedSchema: &ElicitationSchema{Type: "object", Title: "t", Description: "d", Required: []string{"a"}, Properties: map[string]ElicitationProperty{
				"a": {Type: "string", Title: "A", Description: "d", MinLength: new(1), MaxLength: new(9), Pattern: "^x", Format: "email", Default: json.RawMessage(`"x"`),
					Enum: []string{"x"}, OneOf: []EnumOption{{Const: "x", Title: "X", Description: "d"}}},
				"n": {Type: "number", Minimum: new(0.5), Maximum: new(9.5), Default: json.RawMessage(`1.5`)},
				"i": {Type: "integer", Minimum: new(1.0), Maximum: new(9.0)},
				"b": {Type: "boolean", Default: json.RawMessage(`true`)},
				"m": {Type: "array", MinItems: new(1), MaxItems: new(2), Items: &MultiSelectItems{Type: "string", Enum: []string{"x", "y"}}},
				"t": {Type: "array", Items: &MultiSelectItems{AnyOf: []EnumOption{{Const: "x", Title: "X"}}}},
			}}}},
		{"CreateElicitationRequest", CreateElicitationRequest{Mode: ElicitationForm, Message: "m", RequestID: json.RawMessage(`3`),
			RequestedSchema: &ElicitationSchema{Type: "object"}}},
		{"CreateElicitationResponse", CreateElicitationResponse{Action: ElicitAccept, Content: map[string]any{"a": "x", "n": 2, "m": []string{"x"}, "b": true}}},
		{"CreateElicitationResponse", CreateElicitationResponse{Action: ElicitCancel}},
		{"SessionNotification", note(&UserMessageChunk{Content: text, MessageID: "m1"})},
		{"SessionNotification", note(&AgentMessageChunk{Content: link, MessageID: "m2"})},
		{"SessionNotification", note(&AgentThoughtChunk{Content: text})},
		{"SessionNotification", note(&ToolCall{ToolCallID: "c1", Title: "t", Kind: "read", Status: ToolPending, Content: content, Locations: locations,
			RawInput: json.RawMessage(`{"a":1}`), RawOutput: json.RawMessage(`{"b":2}`)})},
		{"SessionNotification", note(&toolUpdate)},
		{"SessionNotification", note(&Plan{Entries: []PlanEntry{{Content: "c", Priority: "high", Status: "pending"}}})},
		{"SessionNotification", note(&AvailableCommandsUpdate{AvailableCommands: []AvailableCommand{{Name: "n", Description: "d", Input: &AvailableCommandInput{Hint: "h"}}}})},
		{"SessionNotification", note(&CurrentModeUpdate{CurrentModeID: "code"})},
		{"SessionNotification", note(&ConfigOptionUpdate{ConfigOptions: configs})},
		{"SessionNotification", note(&SessionInfoUpdate{Title: StringPatch{Set: true, Value: new("t")}, UpdatedAt: StringPatch{Set: true}})},
		{"SessionNotification", note(&UsageUpdate{Used: 1, Size: 2, Cost: &Cost{Amount: 0.5, Currency: "USD"}})},
		{"SessionNotification", note(&Notice{Severity: NoticeError, Title: "t", Description: "d"})},
		{"SessionNotification", note(&CompactionUpdate{CompactionID: "k", Status: CompactionFailed, Summary: []ContentBlock{text}, Error: new("e")})},
		{"SessionNotification", note(&CompactionSummaryChunk{CompactionID: "k", Content: text})},
		{"Error", RPCError{Code: CodeAuthRequired, Message: "m", Data: json.RawMessage(`{"a":1}`)}},
	}
}

// TestSchemaGuard proves every method, update kind and field name used by
// this package exists in the pinned schema. Renaming or misspelling a json
// tag fails it.
func TestSchemaGuard(t *testing.T) {
	s := loadSchema(t)
	for _, sample := range guardSamples() {
		def, ok := s.defs[sample.def].(map[string]any)
		if !ok {
			t.Errorf("schema has no definition %s", sample.def)
			continue
		}
		b, err := json.Marshal(sample.value)
		if err != nil {
			t.Fatalf("%s: %v", sample.def, err)
		}
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatal(err)
		}
		var errs []string
		s.check(sample.def, def, v, &errs)
		for _, e := range errs {
			t.Errorf("%s (%T): %s", sample.def, sample.value, e)
		}
	}
}

// TestSchemaGuardCoversTypes keeps the samples complete: every exported
// struct reachable from them is sampled, so a new field cannot skip the
// guard.
func TestSchemaGuardCoversTypes(t *testing.T) {
	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type)
	walk = func(rt reflect.Type) {
		for rt.Kind() == reflect.Pointer || rt.Kind() == reflect.Slice || rt.Kind() == reflect.Map {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct || seen[rt] || rt.PkgPath() != reflect.TypeFor[Conn]().PkgPath() {
			return
		}
		seen[rt] = true
		for f := range rt.Fields() {
			walk(f.Type)
		}
	}
	for _, sample := range guardSamples() {
		walk(reflect.TypeOf(sample.value))
		if n, ok := sample.value.(SessionNotification); ok {
			walk(reflect.TypeOf(n.Update))
		}
	}
	want := []any{
		InitializeRequest{}, InitializeResponse{}, NewSessionRequest{}, NewSessionResponse{}, LoadSessionRequest{},
		LoadSessionResponse{}, ResumeSessionRequest{}, ResumeSessionResponse{}, ListSessionsRequest{}, ListSessionsResponse{},
		CloseSessionRequest{}, PromptRequest{}, PromptResponse{}, CancelNotification{}, CancelRequestNotification{},
		SetConfigOptionRequest{}, SetConfigOptionResponse{}, RequestPermissionRequest{}, RequestPermissionResponse{},
		CreateElicitationRequest{}, CreateElicitationResponse{}, ElicitationProperty{}, MultiSelectItems{}, EnumOption{},
		UserMessageChunk{}, AgentMessageChunk{}, AgentThoughtChunk{}, ToolCall{}, ToolCallUpdate{}, ToolCallContent{},
		ToolCallLocation{}, Plan{}, PlanEntry{}, AvailableCommandsUpdate{}, AvailableCommand{}, AvailableCommandInput{},
		CurrentModeUpdate{}, ConfigOptionUpdate{}, ConfigOption{}, SessionInfoUpdate{}, UsageUpdate{}, Cost{}, Notice{},
		CompactionUpdate{}, CompactionSummaryChunk{}, ContentBlock{}, SessionModeState{}, SessionMode{}, AuthMethod{},
		AgentCapabilities{}, PromptCapabilities{}, SessionCapabilities{}, Capability{}, Implementation{},
		ClientCapabilities{}, ClientSessionCapabilities{}, ElicitationCapabilities{}, ElicitationSchema{}, SessionInfo{},
	}
	for _, w := range want {
		if !seen[reflect.TypeOf(w)] {
			t.Errorf("%T is not reachable from guardSamples", w)
		}
	}
}

// TestSchemaMethods checks every method name against meta.json and the
// params types against the schema's x-method annotations.
func TestSchemaMethods(t *testing.T) {
	b, err := os.ReadFile("testdata/meta.json")
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		Version         int               `json:"version"`
		AgentMethods    map[string]string `json:"agentMethods"`
		ClientMethods   map[string]string `json:"clientMethods"`
		ProtocolMethods map[string]string `json:"protocolMethods"`
	}
	if err := json.Unmarshal(b, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Version != ProtocolVersion {
		t.Fatalf("meta.json version %d, want %d", meta.Version, ProtocolVersion)
	}
	var known []string
	for _, m := range []map[string]string{meta.AgentMethods, meta.ClientMethods, meta.ProtocolMethods} {
		for _, name := range m {
			known = append(known, name)
		}
	}
	s := loadSchema(t)
	tests := []struct {
		method string
		params string // schema definition whose x-method is method
	}{
		{MethodInitialize, "InitializeRequest"},
		{MethodSessionNew, "NewSessionRequest"},
		{MethodSessionLoad, "LoadSessionRequest"},
		{MethodSessionResume, "ResumeSessionRequest"},
		{MethodSessionList, "ListSessionsRequest"},
		{MethodSessionClose, "CloseSessionRequest"},
		{MethodSessionPrompt, "PromptRequest"},
		{MethodSessionCancel, "CancelNotification"},
		{MethodSessionSetConfigOption, "SetSessionConfigOptionRequest"},
		{MethodSessionUpdate, "SessionNotification"},
		{MethodRequestPermission, "RequestPermissionRequest"},
		{MethodElicitationCreate, "CreateElicitationRequest"},
		{MethodCancelRequest, "CancelRequestNotification"},
	}
	for _, tt := range tests {
		if !slices.Contains(known, tt.method) {
			t.Errorf("method %q is not in meta.json", tt.method)
		}
		def, _ := s.defs[tt.params].(map[string]any)
		if got := def["x-method"]; got != tt.method {
			t.Errorf("%s has x-method %v, want %q", tt.params, got, tt.method)
		}
	}
	var kinds []string
	for _, alt := range anyList(s.defs["SessionUpdate"].(map[string]any)["oneOf"]) {
		props, _ := alt["properties"].(map[string]any)
		kind, _ := props["sessionUpdate"].(map[string]any)["const"].(string)
		kinds = append(kinds, kind)
		if !isKnownUpdateKind(kind) {
			t.Errorf("schema update kind %q is not decoded", kind)
		}
	}
	for _, kind := range []string{KindUserMessageChunk, KindAgentMessageChunk, KindAgentThoughtChunk, KindToolCall, KindToolCallUpdate,
		KindPlan, KindAvailableCommandsUpdate, KindCurrentModeUpdate, KindConfigOptionUpdate, KindSessionInfoUpdate,
		KindUsageUpdate, KindNotice, KindCompactionUpdate, KindCompactionSummaryChunk} {
		if !slices.Contains(kinds, kind) {
			t.Errorf("update kind %q is not in the schema", kind)
		}
	}
}
