package session

import (
	"encoding/json"
	"net"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxSignalPayload = 4096
	maxTitlePayload  = 512
	maxCwdPath       = 4096
	maxCwdHost       = 255
	warpAgentTitle   = "warp://cli-agent"
	// Identity field bounds: IDs and names, and directories.
	maxIdentityName = 128
	maxIdentityCwd  = 4096
	// OSC 7501 id bounds: segments, their length, and the whole id.
	maxProgramIDSegments = 8
	maxProgramIDSegment  = 32
	maxProgramID         = 128
)

// SignalHandler receives terminal side-channel signals. Nil fields are ignored.
// Notification text and event bodies are deliberately not passed on: only
// their presence and the structured event name may influence session state.
type SignalHandler struct {
	Title  func(string)
	Bell   func()
	Notify func()
	Event  func(string)
	Cwd    func(path, host string)
	// ProgramStatus receives an OSC 7501 root record's state and kind; kind
	// is empty unless it is permission, question or auth. Probe receives the
	// OSC 7501 capability query. Prompt receives OSC 133;A and Reset a full
	// reset (RIS); both may end a program's records.
	ProgramStatus func(state, kind string)
	Probe         func()
	Prompt        func()
	Reset         func()
	// Identity receives bounded, printable identity fields of a valid Warp
	// cli-agent event or OSC 7501 report; only the signal fields that report
	// carried are set. It runs alongside, never instead of, the handlers
	// above, and the fields must not influence attention or status.
	Identity func(Identity)
}

// OSCScanner is incremental and bounded, including across split ST terminators.
// Besides OSC titles and notifications it tracks DCS/SOS/PM/APC strings so a
// BEL inside them is not mistaken for a bare bell.
type OSCScanner struct {
	state    byte
	buf      []byte
	overflow bool
}

const (
	scanGround byte = iota
	scanEscape
	scanOSC
	scanOSCEscape
	scanString
	scanStringEscape
)

func (s *OSCScanner) Feed(p []byte, h *SignalHandler) {
	for _, b := range p {
		switch s.state {
		case scanGround:
			switch b {
			case 27:
				s.state = scanEscape
			case 7:
				if h.Bell != nil {
					h.Bell()
				}
			}
		case scanEscape:
			switch b {
			case ']':
				s.state = scanOSC
				s.buf = s.buf[:0]
				s.overflow = false
			case 'P', 'X', '^', '_':
				s.state = scanString
			case 'c':
				s.state = scanGround
				if h.Reset != nil {
					h.Reset()
				}
			case 27:
			case 7:
				s.state = scanGround
				if h.Bell != nil {
					h.Bell()
				}
			default:
				s.state = scanGround
			}
		case scanOSC:
			switch b {
			case 7:
				s.finish(h)
			case 27:
				s.state = scanOSCEscape
			default:
				s.append(b)
			}
		case scanOSCEscape:
			if b == '\\' {
				s.finish(h)
			} else {
				s.append(27)
				s.append(b)
				s.state = scanOSC
			}
		case scanString:
			if b == 27 {
				s.state = scanStringEscape
			}
		case scanStringEscape:
			switch b {
			case '\\':
				s.state = scanGround
			case 27:
			default:
				s.state = scanString
			}
		}
	}
}
func (s *OSCScanner) append(b byte) {
	if len(s.buf) < maxSignalPayload {
		s.buf = append(s.buf, b)
	} else {
		s.overflow = true
	}
}
func (s *OSCScanner) finish(h *SignalHandler) {
	s.state = scanGround
	if s.overflow {
		return
	}
	text := string(s.buf)
	switch {
	case strings.HasPrefix(text, "0;") || strings.HasPrefix(text, "2;"):
		if len(text) <= maxTitlePayload && h.Title != nil {
			h.Title(strings.TrimSpace(text[2:]))
		}
	case strings.HasPrefix(text, "7;"):
		if path, host, ok := parseOSC7(text[2:]); ok && h.Cwd != nil {
			h.Cwd(path, host)
		}
	case strings.HasPrefix(text, "9;"):
		if !numericSubcommand(text[2:]) && h.Notify != nil {
			h.Notify()
		}
	case text == "7501;?":
		if h.Probe != nil {
			h.Probe()
		}
	case strings.HasPrefix(text, "7501;"):
		report, ok := parseProgramReport(text[len("7501;"):])
		if !ok {
			return
		}
		if report.childID != "" {
			// A child record never changes the root record.
			if h.Identity != nil {
				h.Identity(Identity{ProgramChildID: report.childID})
			}
			return
		}
		if h.ProgramStatus != nil {
			h.ProgramStatus(report.state, report.kind)
		}
		if report.app != "" && h.Identity != nil {
			h.Identity(Identity{ProgramApp: report.app})
		}
	case text == "133;A" || strings.HasPrefix(text, "133;A;"):
		if h.Prompt != nil {
			h.Prompt()
		}
	case strings.HasPrefix(text, "777;notify;"):
		title, body, _ := strings.Cut(text[len("777;notify;"):], ";")
		if title != warpAgentTitle {
			if h.Notify != nil {
				h.Notify()
			}
			return
		}
		event, identity, ok := agentEvent(body)
		if !ok {
			return
		}
		if identity != (Identity{}) && h.Identity != nil {
			h.Identity(identity)
		}
		if h.Event != nil {
			h.Event(event)
		}
	}
}

// parseOSC7 accepts the file URL shape used by shell integrations. Only the
// path and host are retained; credentials, ports, queries and fragments are
// rejected so a report cannot smuggle a different location into the UI.
func parseOSC7(raw string) (path, host string, ok bool) {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "file") || u.Opaque != "" ||
		u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return "", "", false
	}
	path, host = u.Path, u.Hostname()
	if len(path) > maxCwdPath || len(host) > maxCwdHost || !strings.HasPrefix(path, "/") {
		return "", "", false
	}
	if !utf8.ValidString(path) || !utf8.ValidString(host) || containsControl(path) || containsControl(host) {
		return "", "", false
	}
	return path, host, true
}

func containsControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return true
		}
	}
	return false
}

// reportedHostIsLocal treats loopback names and the service host's own name as
// local. A host with a domain only matches a local hostname with the same
// domain; a bare name may match the local short name.
func reportedHostIsLocal(host, hostname string) bool {
	h := normalizeHost(host)
	if h == "" || h == "localhost" || h == "localhost.localdomain" || h == "ip6-localhost" {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	local := normalizeHost(hostname)
	if local == "" {
		return false
	}
	if h == local {
		return true
	}
	if !strings.Contains(h, ".") {
		return h == shortHost(local)
	}
	if !strings.Contains(local, ".") {
		return shortHost(h) == local
	}
	return false
}

func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
}

func shortHost(host string) string {
	if i := strings.IndexByte(host, '.'); i > 0 {
		return host[:i]
	}
	return host
}

// programReport is a valid OSC 7501 report. childID is the first segment of
// a child record's id and empty for the root record; app is the root
// record's app when it is a valid identity name.
type programReport struct {
	state, kind, app, childID string
}

// parseProgramReport parses an OSC 7501 report body. Reports with an unknown
// state or a malformed id are rejected; a valid id makes the report a child
// record, which never changes the root. Besides state and kind only the
// root's app and the child's first id segment are kept, as identity fields;
// msg, title and every other value are never retained.
func parseProgramReport(body string) (report programReport, ok bool) {
	var app, id string
	hasID := false
	for _, pair := range strings.Split(body, ":") {
		key, value, found := strings.Cut(pair, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !found || !programStatusKey(key) {
			continue
		}
		switch key {
		case "state":
			report.state = value
		case "kind":
			report.kind = value
		case "app":
			app = value
		case "id":
			id, hasID = value, true
		}
	}
	switch report.state {
	case "idle", "working", "done", "blocked", "error", "clear":
	default:
		return programReport{}, false
	}
	if hasID {
		first, valid := programIDFirstSegment(id)
		if !valid {
			return programReport{}, false
		}
		return programReport{state: report.state, childID: first}, true
	}
	// kind means something only for blocked; keeping it elsewhere would make
	// an unchanged report look like a new one.
	switch report.kind {
	case "permission", "question", "auth":
		if report.state != "blocked" {
			report.kind = ""
		}
	default:
		report.kind = ""
	}
	report.app, _ = identityValue(app, maxIdentityName)
	return report, true
}

// programIDFirstSegment validates an OSC 7501 id, slash-separated segments of
// [A-Za-z0-9_.+-]{1,32}, at most 8 deep and 128 bytes, and returns its first
// segment.
func programIDFirstSegment(id string) (string, bool) {
	if len(id) > maxProgramID {
		return "", false
	}
	segments := strings.Split(id, "/")
	if len(segments) > maxProgramIDSegments {
		return "", false
	}
	for _, segment := range segments {
		if segment == "" || len(segment) > maxProgramIDSegment {
			return "", false
		}
		for i := range len(segment) {
			if !programIDByte(segment[i]) {
				return "", false
			}
		}
	}
	return segments[0], true
}

func programIDByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	return c == '_' || c == '.' || c == '+' || c == '-'
}

// identityValue accepts a non-empty identity field of at most limit bytes
// made only of printable characters.
func identityValue(value string, limit int) (string, bool) {
	if value == "" || len(value) > limit || !utf8.ValidString(value) {
		return "", false
	}
	for _, r := range value {
		if !unicode.IsPrint(r) {
			return "", false
		}
	}
	return value, true
}

// identityString decodes an identity field of a Warp event. A field of
// another JSON type, or an invalid value, is ignored without voiding the event.
func identityString(raw json.RawMessage, limit int) string {
	var value string
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return ""
	}
	value, _ = identityValue(value, limit)
	return value
}

func programStatusKey(key string) bool {
	if key == "" || len(key) > 16 {
		return false
	}
	for i := 0; i < len(key); i++ {
		if key[i] < 'a' || key[i] > 'z' {
			return false
		}
	}
	return true
}

// numericSubcommand reports ConEmu-style OSC 9 subcommands such as 9;4 progress.
func numericSubcommand(text string) bool {
	digits := 0
	for digits < len(text) && text[digits] >= '0' && text[digits] <= '9' {
		digits++
	}
	return digits > 0 && digits < len(text) && text[digits] == ';'
}

// agentEvent accepts only a JSON object with an integer v >= 1 and a string
// event. Besides event only the bounded identity fields agent,
// plugin_version, session_id and cwd are kept; every other field is ignored
// and never retained.
func agentEvent(body string) (event string, identity Identity, ok bool) {
	var envelope struct {
		V             *int64          `json:"v"`
		Event         *string         `json:"event"`
		Agent         json.RawMessage `json:"agent"`
		PluginVersion json.RawMessage `json:"plugin_version"`
		SessionID     json.RawMessage `json:"session_id"`
		Cwd           json.RawMessage `json:"cwd"`
	}
	if !strings.HasPrefix(strings.TrimSpace(body), "{") {
		return "", Identity{}, false
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return "", Identity{}, false
	}
	if envelope.V == nil || *envelope.V < 1 || envelope.Event == nil {
		return "", Identity{}, false
	}
	identity = Identity{
		WarpAgent:         identityString(envelope.Agent, maxIdentityName),
		WarpPluginVersion: identityString(envelope.PluginVersion, maxIdentityName),
		WarpSessionID:     identityString(envelope.SessionID, maxIdentityName),
		WarpCwd:           identityString(envelope.Cwd, maxIdentityCwd),
	}
	return *envelope.Event, identity, true
}
