package archtest

import (
	"strings"
	"testing"
)

// webmux is the layer manifest. Every package under internal/ appears here;
// see docs/architecture.md for what each layer may depend on. Exceptions
// are listed with their reason and are visible in review.
var webmux = manifest{
	module: "github.com/rainchan/webmux",
	layers: map[string]layer{
		// HTTP and WebSocket transport.
		"internal/api":  controller,
		"internal/hub":  controller,
		"internal/wire": controller,

		// Business rules, state, filesystem and processes.
		"internal/session":           service,
		"internal/hybrid":            service,
		"internal/handoff":           service,
		"internal/conversation":      service,
		"internal/workspace":         service,
		"internal/settings":          service,
		"internal/launch":            service,
		"internal/agenttrust":        service,
		"internal/push":              service,
		"internal/codeview":          service,
		"internal/files":             service,
		"internal/forward":           service,
		"internal/artifacts":         service,
		"internal/tunnelsettings":    service,
		"internal/tailscale":         service,
		"internal/tailscalesettings": service,
		"internal/voice":             service,
		"internal/voicesettings":     service,
		"internal/p2p":               service,
		"internal/p2psettings":       service,

		// Wire contracts and infrastructure every layer may use.
		"internal/acp":       shared,
		"internal/nativerpc": shared,
		// The shell guide served at /llms.txt and the {{webmux}} brief.
		"internal/agentguide":   shared,
		"internal/agentprofile": shared,
		// Agent adapters and the registry that lists them.
		"internal/agentprofile/agents":     shared,
		"internal/agentprofile/agy":        shared,
		"internal/agentprofile/claudecode": shared,
		"internal/agentprofile/codex":      shared,
		"internal/agentprofile/devin":      shared,
		"internal/agentprofile/kimi":       shared,
		"internal/agentprofile/omp":        shared,
		"internal/agentprofile/opencode":   shared,
		"internal/agentprofile/qoder":      shared,
		"internal/agentprofile/qodercn":    shared,
		"internal/contract":                shared,
		"internal/config":                  shared,
		"internal/runtime":                 shared,
		"internal/hoststat":                shared,
		"internal/jsonutil":                shared,
		"internal/statefile":               shared,
		"internal/settingsstore":           shared,
		"internal/fetch":                   shared,
		"internal/update":                  shared,
		"internal/pathutil":                shared,
		"internal/attentionkind":           shared,
		"internal/procpeek":                shared,
		"internal/vtpreview":               shared,
		"internal/wsl":                     shared,

		// Programs that run outside the server and talk to it over the wire.
		"internal/client":       client,
		"internal/localclient":  client,
		"internal/threadread":   client,
		"internal/mcpstdio":     client,
		"internal/mcpcontrol":   client,
		"internal/attach":       client,
		"internal/clicmd":       client,
		"internal/tray":         client,
		"internal/tray/forward": client,
		"internal/tray/traysys": client,
		"internal/tray/winui":   client,
		"internal/wslclient":    client,
		"internal/tunnel":       client,
		"internal/service":      client,

		"internal/archtest":               testOnly,
		"internal/agentprofile/agenttest": testOnly,
		"internal/docguard":               testOnly,
	},
	outbound: map[string]string{
		"internal/push": "delivers Web Push messages to browser push services",
		"internal/p2p":  "queries local UPnP IGD router XML and SOAP port-mapping control endpoints",
	},
	pure: map[string]string{
		"internal/push.Validate":    "validates subscription strings without service state or I/O, before the controller checks Origin",
		"internal/hybrid.FormValue": "decodes one form answer value of a chat request body without service state or I/O",
	},
	contract:  "internal/contract",
	wireScope: []string{"internal/api", "internal/wire", "internal/hub"},
	// The controller's JSON boundary: their values are checked where they
	// are called.
	sinks: map[string]sink{
		"internal/api.reply":          {arg: 2},
		"internal/api.decode":         {arg: 2},
		"internal/api.decodeOptional": {arg: 2},
		"internal/api.decodeStrict":   {arg: 2},
		"internal/api.serveCode":      {typeArgs: []int{0}},
		"internal/api.serve":          {typeArgs: []int{0}},
		"internal/api.serveIn":        {typeArgs: []int{0, 1}},
		"internal/api.serveInDone":    {typeArgs: []int{0}},
		"internal/hub.jsonFrame":      {arg: 1},
	},
	encoders: map[string]string{
		"internal/handoff.executionHash":                "hashes typed execution requests for durable idempotence",
		"internal/push.payload":                         "Web Push message body, read by the service worker",
		"internal/push.testPayload":                     "Web Push test message body",
		"internal/push.vapidHeader":                     "VAPID JWT header and claims sent to push services",
		"internal/codeview.version":                     "content digest of a snapshot, never sent as JSON",
		"internal/agenttrust.encodeJSON":                "rewrites an agent's own config file (Claude, Copilot, Antigravity, Qoder, QoderCN) to record folder trust; Kimi uses a process env instead, never sent to clients",
		"internal/p2p.encodeControlJSON":                "encodes internal WebRTC DataChannel control handshake and RTT ping/pong frames",
		"internal/p2p.encodeRPCResponseJSON":            "encodes internal WebRTC DataChannel RPC response metadata envelopes",
		"internal/conversation.encodePreviewRequest":    "encodes a private immutable SQLite preview input snapshot, never a client payload",
		"internal/conversation.encodePreviewBudget":     "encodes private SQLite budget facts, never a client payload",
		"internal/conversation.encodePreviewItems":      "encodes private preview selection IDs for set-based SQLite insertion, never a client payload",
		"internal/handoff.requestHash":                  "hashes a typed command for durable idempotence, never returns encoded client JSON",
		"internal/handoff.configFingerprint":            "hashes launch argv/env without retaining credentials or emitting client JSON",
		"internal/handoff.jsonBytes":                    "calculates internal handoff budget costs, never client-visible wire values",
		"internal/conversation.encodeInlineAttachments": "encodes target-owned SQLite replay attachment references, never a client payload",
	},
	bodyReaders: map[string]string{
		"internal/api.decode":                  "reads JSON bodies with a size limit and unknown fields rejected",
		"internal/api.decodeOptional":          "reads a JSON body that may be empty, then delegates to decode",
		"internal/api.decodeStrict":            "reads JSON bodies that must also reject duplicate and case-folded keys",
		"internal/api.decodeCreate":            "reads the session create JSON body like decode, answering its failure as a create failure",
		"internal/api.receiveUpload":           "streams one multipart file upload within the upload size limit",
		"internal/api.(*Server).redeemForward": "reads one small URL-encoded form (ticket code and gateway path) that a top-level browser form posts",
	},
}

func TestArchitecture(t *testing.T) {
	p, err := load("../..", webmux)
	if err != nil {
		t.Fatal(err)
	}
	if violations := p.check(); len(violations) > 0 {
		lines := make([]string, len(violations))
		for i, v := range violations {
			lines[i] = v.String()
		}
		t.Errorf("%d architecture violations (docs/architecture.md):\n%s", len(violations), strings.Join(lines, "\n"))
	}
}
