package hybrid

import (
	"context"
	"reflect"
	"testing"

	"github.com/rainchan/webmux/internal/acp"
)

func TestDefaultMCPDoesNotRequireHistory(t *testing.T) {
	want := []acp.MCPServer{{Name: "webmux", Command: "/fixture/webmux"}}
	a := &actor{svc: &Service{mcpServers: func(context.Context) []acp.MCPServer { return want }}}
	got, scope := a.sessionMCPServers(context.Background())
	if !reflect.DeepEqual(got, want) || scope != "" {
		t.Fatalf("servers %+v scope %q", got, scope)
	}
}

func TestDefaultMCPKeepsHandoffScopeAndCallbackSlice(t *testing.T) {
	h, a, execution := readerRefreshCancellationTest(t)
	backing := []acp.MCPServer{{Name: "webmux"}, {Name: "sentinel"}}
	h.svc.mcpServers = func(context.Context) []acp.MCPServer { return backing[:1] }
	got, scope := a.sessionMCPServers(context.Background())
	if len(got) != 2 || got[0].Name != "webmux" || got[1].Name != "webmux-history" || scope != execution.HandoffID {
		t.Fatalf("servers %+v scope %q execution %+v", got, scope, execution)
	}
	if backing[1].Name != "sentinel" {
		t.Fatal("mutated descriptor callback's backing slice")
	}
}
