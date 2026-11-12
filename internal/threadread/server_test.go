package threadread

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/localclient"
)

func TestLegacyHandshakeAndModernDiscoveryFallback(t *testing.T) {
	r := Reader{Client: localclient.New(localclient.UnixDialer{Path: "/missing"}), ConversationID: strings.Repeat("a", 32), HandoffID: strings.Repeat("b", 32)}
	defer r.Client.Close()
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":"probe","method":"server/discover"}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"thread_read","arguments":{"view":"activity","conversationId":"escape"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"thread_read","arguments":{"view":"activity"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"ping"}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := Serve(context.Background(), strings.NewReader(input), &out, r); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 7 {
		t.Fatalf("responses %d", len(lines))
	}
	var first struct{ Error struct{ Code int } }
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil || first.Error.Code != -32601 {
		t.Fatal("discovery not safely declined")
	}
	if !strings.Contains(lines[3], `"thread_read"`) || !strings.Contains(lines[3], `"additionalProperties":false`) {
		t.Fatal("read-only tool missing")
	}
	for _, n := range []int{4, 5} {
		if !strings.Contains(lines[n], `"isError":true`) {
			t.Fatal("tool failure not isolated")
		}
	}
	if strings.Contains(out.String(), "/missing") {
		t.Fatal("local transport path leaked into tool error")
	}
}

func TestBoundedFramesAndStrictArguments(t *testing.T) {
	for _, raw := range []string{`{"view":"activity","afterPosition":"+1"}`, `{"view":"activity","afterPosition":"9223372036854775808"}`, `{"view":"activity","itemId":"../x"}`, `{"view":"activity","textOffset":1.5}`, `{"view":"activity","limit":0}`, `{"view":"activity","maxBytesPerItem":3}`, `{"view":"activity"} {}`, `null`} {
		if _, err := parseArguments(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	r := Reader{Client: localclient.New(localclient.UnixDialer{}), ConversationID: strings.Repeat("a", 32), HandoffID: strings.Repeat("b", 32)}
	defer r.Client.Close()
	if err := Serve(context.Background(), strings.NewReader(strings.Repeat("x", 70<<10)), &bytes.Buffer{}, r); err == nil {
		t.Fatal("unbounded frame accepted")
	}
}
