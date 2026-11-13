package hybrid

import (
	"testing"

	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/agentprofile/claudecode"
	"github.com/rainchan/webmux/internal/agentprofile/codex"
	"github.com/rainchan/webmux/internal/agentprofile/devin"
	"github.com/rainchan/webmux/internal/agentprofile/kimi"
	"github.com/rainchan/webmux/internal/agentprofile/qoder"
	"github.com/rainchan/webmux/internal/agentprofile/qodercn"
)

func TestDefaultChatPermissionSeeds(t *testing.T) {
	for _, tc := range []struct {
		agent agentprofile.Agent
		value string
	}{
		{claudecode.Agent, "bypassPermissions"}, {codex.Agent, "agent-full-access"},
		{devin.Agent, "bypass"}, {kimi.Agent, "yolo"},
		{qoder.Agent, "bypass_permissions"}, {qodercn.Agent, "bypass_permissions"},
	} {
		t.Run(tc.agent.Profile().Name, func(t *testing.T) {
			a := actor{cmd: agentprofile.Command{Agent: tc.agent, Profile: tc.agent.Profile()}}
			seeds := a.seedValues()
			if len(seeds) != 1 || seeds[0].id != "mode" || seeds[0].value != tc.value {
				t.Fatalf("seeds = %+v", seeds)
			}
		})
	}
}

func TestExplicitChatModeOverridesDefault(t *testing.T) {
	registry := agentprofile.NewRegistry(claudecode.Agent, codex.Agent, kimi.Agent, devin.Agent, qoder.Agent)
	for _, argv := range [][]string{
		{"claude", "--permission-mode", "plan"},
		{"codex", "--sandbox", "read-only"},
		{"kimi", "--plan"},
		{"devin", "--permission-mode", "smart"},
		{"qoder", "--permission-mode", "acceptEdits"},
	} {
		t.Run(argv[0], func(t *testing.T) {
			cmd, err := registry.Parse(argv)
			if err != nil {
				t.Fatal(err)
			}
			explicit, _ := cmd.ChatConfig()
			a := actor{cmd: cmd}
			seeds := a.seedValues()
			if len(seeds) != len(explicit) || len(seeds) != 1 || seeds[0].value != explicit[0].Value {
				t.Fatalf("default overrode explicit mode: %+v", seeds)
			}
		})
	}
}
