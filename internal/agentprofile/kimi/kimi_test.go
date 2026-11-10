package kimi

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/acp"
	ap "github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/agentprofile/agenttest"
)

func TestContract(t *testing.T) {
	agenttest.Run(t, Agent, agenttest.Fixture{
		Signals:  ap.Signals{ProgramApp: "kimi-code", RootCwd: "/p"},
		Identity: ap.Identity{Agent: Name, Cwd: "/p"},
	})
	p := Agent.Profile()
	if p.Icon != "kimi" || p.VerifiedVersion != "2.5.2" || !slices.Equal(p.HiddenCommands, []string{"login", "logout"}) {
		t.Fatalf("profile = %+v", p)
	}
	if got := ap.ReapplyOnLoad(Agent); !slices.Equal(got, []string{"mode"}) {
		t.Fatalf("reapply options = %q", got)
	}
	if _, ok := Agent.(ap.Steerer); ok {
		t.Fatal("Kimi unexpectedly implements steering")
	}
	if _, ok := Agent.(ap.LockTracker); ok {
		t.Fatal("Kimi unexpectedly implements lock tracking")
	}
	if _, ok := Agent.(ap.ProcessRegistry); ok {
		t.Fatal("Kimi unexpectedly implements process registry")
	}
}

func TestChatConfig(t *testing.T) {
	tests := []struct {
		name        string
		argv        []string
		seeds       []ap.Seed
		unsupported []string
	}{
		{
			name:  "model and auto",
			argv:  []string{"kimi", "--model", "provider/model", "--auto"},
			seeds: []ap.Seed{{ConfigID: "model", Value: "provider/model", Arg: "--model provider/model"}, {ConfigID: "mode", Value: "auto", Arg: "--auto"}},
		},
		{
			name:  "yolo",
			argv:  []string{"kimi", "--yolo"},
			seeds: []ap.Seed{{ConfigID: "mode", Value: "yolo", Arg: "--yolo"}},
		},
		{
			name:        "plan wins",
			argv:        []string{"kimi", "--auto", "--plan", "--yolo"},
			seeds:       []ap.Seed{{ConfigID: "mode", Value: "plan", Arg: "--plan"}},
			unsupported: []string{"--auto", "--yolo"},
		},
		{
			name:        "chat-only directories",
			argv:        []string{"kimi", "--add-dir", "/srv/lib", "--skills-dir=/srv/skills"},
			unsupported: []string{"--add-dir /srv/lib", "--skills-dir=/srv/skills"},
		},
	}
	registry := ap.NewRegistry(Agent)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := registry.Parse(tt.argv)
			if err != nil {
				t.Fatal(err)
			}
			seeds, unsupported := cmd.ChatConfig()
			if !slices.Equal(seeds, tt.seeds) || !slices.Equal(unsupported, tt.unsupported) {
				t.Fatalf("ChatConfig = %+v %q, want %+v %q", seeds, unsupported, tt.seeds, tt.unsupported)
			}
		})
	}
}

func TestCustomAgentDoesNotBreakResume(t *testing.T) {
	for _, argv := range [][]string{{"kimi", "--agent", "reviewer"}, {"kimi", "--agent-file", "/tmp/reviewer.md"}} {
		cmd, err := ap.NewRegistry(Agent).Parse(argv)
		if err != nil {
			t.Fatal(err)
		}
		resume, dropped := cmd.ResumeArgv("session_1")
		if !slices.Equal(resume, []string{"kimi", "--session", "session_1"}) {
			t.Fatalf("ResumeArgv(%q) = %q", argv, resume)
		}
		if len(dropped) != 2 {
			t.Fatalf("Dropped(%q) = %q", argv, dropped)
		}
	}
}

func TestSubmitDelay(t *testing.T) {
	if got := ap.SubmitDelay(Agent); got != 200*time.Millisecond {
		t.Fatalf("SubmitDelay = %v", got)
	}
}

func TestConfigErrorHandled(t *testing.T) {
	for _, tc := range []struct{ mode, details string }{{"plan", "Already in plan mode"}, {"auto", "Already in auto mode"}, {"yolo", "Already in YOLO mode"}, {"default", "Already in default mode"}} {
		rpc := &acp.RPCError{Code: acp.CodeInternalError, Data: json.RawMessage(`{"details":"` + tc.details + `"}`)}
		if !ap.ConfigErrorHandled(Agent, rpc, "mode", tc.mode) {
			t.Errorf("%q was not treated as idempotent", tc.details)
		}
	}
	rpc := &acp.RPCError{Code: acp.CodeInternalError, Data: json.RawMessage(`{"details":"Already in plan mode"}`)}
	if ap.ConfigErrorHandled(Agent, rpc, "model", "plan") || ap.ConfigErrorHandled(Agent, rpc, "mode", "auto") {
		t.Fatal("unrelated config error was accepted")
	}
}
