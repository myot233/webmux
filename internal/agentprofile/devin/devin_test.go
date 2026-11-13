package devin

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/rainchan/webmux/internal/acp"
	ap "github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/agentprofile/agenttest"
)

func TestContract(t *testing.T) {
	agenttest.Run(t, Agent, agenttest.Fixture{
		Signals:  ap.Signals{ProgramApp: "devin", RootCwd: "/p"},
		Identity: ap.Identity{Agent: Name, Cwd: "/p"},
	})
	p := Agent.Profile()
	if p.Icon != "devin" || p.VerifiedVersion != "3000.12.57" || !p.VersionExecutable {
		t.Fatalf("profile = %+v", p)
	}
	if !slices.Equal(p.HiddenCommands, []string{"login", "logout"}) {
		t.Fatalf("hidden commands = %q", p.HiddenCommands)
	}
	if ap.ResumesSessions(Agent) {
		t.Fatal("Devin must not declare session/resume: it answers Method not found")
	}
	if got := ap.ReapplyOnLoad(Agent); len(got) != 0 {
		t.Fatalf("Devin's session/load returns the stored config, reapply = %q", got)
	}
	if _, ok := Agent.(ap.Steerer); ok {
		t.Fatal("Devin unexpectedly implements steering")
	}
}

func TestClassify(t *testing.T) {
	registry := ap.NewRegistry(Agent)
	for _, tc := range []struct {
		name   string
		argv   []string
		reason ap.Reason
	}{
		{"bare", []string{"devin"}, ""},
		{"prompt after --", []string{"devin", "--", "修 bug"}, ""},
		{"directory", []string{"devin", "/home/u/proj"}, ap.ReasonDirectoryArg},
		{"resume", []string{"devin", "--resume", "old-session"}, ap.ReasonSessionArg},
		{"resume bare", []string{"devin", "-r"}, ap.ReasonSessionArg},
		{"continue", []string{"devin", "-c"}, ap.ReasonSessionArg},
		{"cloud", []string{"devin", "--cloud"}, ap.ReasonSessionStore},
		{"config", []string{"devin", "--config", "/tmp/x.json"}, ap.ReasonSessionStore},
		{"print", []string{"devin", "-p", "总结"}, ap.ReasonNonInteractive},
		{"print bare", []string{"devin", "--print"}, ap.ReasonNonInteractive},
		{"help", []string{"devin", "-h"}, ap.ReasonNonInteractive},
		{"version", []string{"devin", "-V"}, ap.ReasonNonInteractive},
		{"acp subcommand", []string{"devin", "acp"}, ap.ReasonNonInteractive},
		{"list subcommand", []string{"devin", "list"}, ap.ReasonNonInteractive},
		{"rm subcommand", []string{"devin", "rm", "old"}, ap.ReasonNonInteractive},
		{"ls alias", []string{"devin", "ls"}, ap.ReasonNonInteractive},
		{"auth subcommand", []string{"devin", "auth", "status"}, ap.ReasonNonInteractive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := registry.Check(tc.argv)
			if tc.reason == "" {
				if !s.Supported || s.Agent != Name {
					t.Fatalf("Check(%q) = %+v, want supported devin", tc.argv, s)
				}
				return
			}
			if s.Supported || s.Reason != tc.reason {
				t.Fatalf("Check(%q) = %+v, want reason %s", tc.argv, s, tc.reason)
			}
		})
	}
}

func TestResumeArgv(t *testing.T) {
	cmd, err := ap.NewRegistry(Agent).Parse([]string{"devin", "--permission-mode", "dangerous", "--model", "swe-2-high", "--sandbox", "--", "修 bug"})
	if err != nil {
		t.Fatal(err)
	}
	resume, dropped := cmd.ResumeArgv("calm-fjord")
	want := []string{"devin", "--permission-mode", "dangerous", "--model", "swe-2-high", "--sandbox", "--resume", "calm-fjord"}
	if !slices.Equal(resume, want) {
		t.Fatalf("ResumeArgv = %q, want %q", resume, want)
	}
	if !slices.Equal(dropped, []string{"修 bug"}) {
		t.Fatalf("dropped = %q", dropped)
	}
}

func TestResumeArgvKeepsOptionalFlags(t *testing.T) {
	cmd, err := ap.NewRegistry(Agent).Parse([]string{"devin", "--respect-workspace-trust=false", "--export"})
	if err != nil {
		t.Fatal(err)
	}
	resume, _ := cmd.ResumeArgv("sid")
	want := []string{"devin", "--respect-workspace-trust=false", "--export", "--resume", "sid"}
	if !slices.Equal(resume, want) {
		t.Fatalf("ResumeArgv = %q, want %q", resume, want)
	}
}

func TestACPArgv(t *testing.T) {
	cmd, err := ap.NewRegistry(Agent).Parse([]string{"devin", "--permission-mode", "dangerous", "--model", "swe-2-high", "--sandbox"})
	if err != nil {
		t.Fatal(err)
	}
	argv, unsupported := cmd.ACPArgv()
	if !slices.Equal(argv, []string{"devin", "acp", "--model", "swe-2-high"}) {
		t.Fatalf("ACPArgv = %q", argv)
	}
	if !slices.Equal(unsupported, []string{"--sandbox"}) {
		t.Fatalf("unsupported = %q", unsupported)
	}
}

func TestChatConfig(t *testing.T) {
	for _, tt := range []struct {
		name        string
		argv        []string
		seeds       []ap.Seed
		unsupported []string
	}{
		{
			name:  "preset",
			argv:  []string{"devin", "--permission-mode", "dangerous"},
			seeds: []ap.Seed{{ConfigID: "mode", Value: "bypass", Arg: "--permission-mode dangerous"}},
		},
		{
			name:  "model",
			argv:  []string{"devin", "--model", "swe-2-high"},
			seeds: []ap.Seed{{ConfigID: "model", Value: "swe-2-high", Arg: "--model swe-2-high"}},
		},
		{
			name:  "accept-edits and smart",
			argv:  []string{"devin", "--permission-mode", "smart"},
			seeds: []ap.Seed{{ConfigID: "mode", Value: "smart", Arg: "--permission-mode smart"}},
		},
		{
			name:        "auto has no chat mode",
			argv:        []string{"devin", "--permission-mode", "auto", "--sandbox"},
			unsupported: []string{"--permission-mode auto", "--sandbox"},
		},
		{
			name:        "export and verbosity unsupported",
			argv:        []string{"devin", "--verbosity", "2", "--export", "out.md"},
			unsupported: []string{"--verbosity 2", "--export out.md"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := ap.NewRegistry(Agent).Parse(tt.argv)
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

func TestLockHeldBy(t *testing.T) {
	data := json.RawMessage(`{"cognition.ai/errorKind":"session_locked","cognition.ai/retryable":true,"cognition.ai/lockHolderPid":51463}`)
	locked := &acp.RPCError{Code: -32015, Message: "Session 'x' is already open", Data: data}
	pid, ok := ap.LockHeldBy(Agent, locked)
	if !ok || pid != 51463 {
		t.Fatalf("LockHeldBy = %d %v, want 51463", pid, ok)
	}
	for _, err := range []error{
		errors.New("plain"),
		&acp.RPCError{Code: -32603, Message: "other"},
		&acp.RPCError{Code: -32015, Data: json.RawMessage(`{"cognition.ai/errorKind":"session_not_found"}`)},
		&acp.RPCError{Code: -32015, Data: json.RawMessage(`{"cognition.ai/errorKind":"session_locked","cognition.ai/lockHolderPid":0}`)},
		&acp.RPCError{Code: -32015, Data: json.RawMessage(`{"cognition.ai/errorKind":"session_locked"}`)},
	} {
		if pid, ok := ap.LockHeldBy(Agent, err); ok || pid != 0 {
			t.Fatalf("LockHeldBy(%v) = %d %v, want none", err, pid, ok)
		}
	}
}
