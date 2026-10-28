package agentprofile

import (
	"errors"
	"slices"
	"testing"
)

var wrapped = func(args ...string) []string {
	return append([]string{"bash", "-ic", `exec -- "$@"`, "webmux"}, args...)
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name   string
		argv   []string
		agent  string
		reason Reason
		arg    string
	}{
		{name: "preset bashrc omp", argv: wrapped("omp"), agent: OMP},
		{name: "preset bashrc opencode", argv: wrapped("opencode"), agent: OpenCode},
		{name: "direct path", argv: []string{"/usr/local/bin/omp", "--model", "haiku"}, agent: OMP},
		{name: "omp profile passes through", argv: []string{"omp", "--profile", "work"}, agent: OMP},
		{name: "custom shell wrapper", argv: []string{"sh", "-c", "omp"}, reason: ReasonUnsupportedForm},
		{name: "env prefix", argv: []string{"env", "A=1", "opencode"}, reason: ReasonUnsupportedForm},
		{name: "other bashrc script", argv: []string{"bash", "-ic", "exec omp", "webmux"}, reason: ReasonUnsupportedForm},
		{name: "plain shell", argv: []string{"/bin/bash", "-l"}, reason: ReasonUnknownAgent},
		{name: "other agent wrapped", argv: wrapped("claude"), reason: ReasonUnknownAgent},
		{name: "empty", argv: nil, reason: ReasonUnknownAgent},
		{name: "opencode directory", argv: []string{"opencode", "/home/u/proj"}, reason: ReasonDirectoryArg, arg: "/home/u/proj"},
		{name: "opencode server", argv: []string{"opencode", "--server", "http://x"}, reason: ReasonSessionStore, arg: "--server"},
		{name: "opencode server inline", argv: []string{"opencode", "--server=http://x"}, reason: ReasonSessionStore, arg: "--server"},
		{name: "opencode version", argv: []string{"opencode", "-v"}, reason: ReasonNonInteractive, arg: "-v"},
		{name: "omp session dir", argv: wrapped("omp", "--session-dir", "/tmp/s"), reason: ReasonSessionStore, arg: "--session-dir"},
		{name: "omp no session", argv: []string{"omp", "--no-session"}, reason: ReasonSessionStore, arg: "--no-session"},
		{name: "omp cwd", argv: []string{"omp", "--cwd=/srv"}, reason: ReasonDirectoryArg, arg: "--cwd"},
		{name: "omp print", argv: []string{"omp", "-p", "总结改动"}, reason: ReasonNonInteractive, arg: "-p"},
		{name: "omp subcommand", argv: []string{"omp", "commit"}, reason: ReasonNonInteractive, arg: "commit"},
		{name: "omp prompt equal to subcommand after prompt", argv: []string{"omp", "fix", "commit"}, agent: OMP},
		{name: "omp after double dash", argv: []string{"omp", "--", "commit"}, agent: OMP},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Check(tt.argv)
			want := Support{Supported: tt.agent != "", Agent: tt.agent, Reason: tt.reason}
			if got != want {
				t.Fatalf("Check(%q) = %+v, want %+v", tt.argv, got, want)
			}
			if tt.reason == "" {
				return
			}
			_, err := Parse(tt.argv)
			var unsupported *UnsupportedError
			if !errors.As(err, &unsupported) || unsupported.Arg != tt.arg {
				t.Fatalf("Parse(%q) error = %v, want arg %q", tt.argv, err, tt.arg)
			}
		})
	}
}

func TestResumeArgv(t *testing.T) {
	tests := []struct {
		name    string
		argv    []string
		id      string
		want    []string
		dropped []string
	}{
		{
			name:    "drop prompt and session options",
			argv:    []string{"omp", "--model", "haiku", "--continue", "修 bug"},
			id:      "01abc",
			want:    []string{"omp", "--model", "haiku", "--resume", "01abc"},
			dropped: []string{"--continue", "修 bug"},
		},
		{
			name:    "opencode standalone injected",
			argv:    []string{"opencode", "--prompt", "hi"},
			id:      "ses_1",
			want:    []string{"opencode", "--standalone", "-s", "ses_1"},
			dropped: []string{"--prompt hi"},
		},
		{
			name: "opencode standalone not duplicated",
			argv: []string{"opencode", "--auto", "--standalone", "--log-level=debug"},
			id:   "ses_2",
			want: []string{"opencode", "--auto", "--standalone", "--log-level=debug", "-s", "ses_2"},
		},
		{
			name:    "inline and separate values",
			argv:    []string{"omp", "--model=haiku", "--thinking", "low", "--resume=old", "-r", "--no-lsp"},
			id:      "01x",
			want:    []string{"omp", "--model=haiku", "--thinking", "low", "--no-lsp", "--resume", "01x"},
			dropped: []string{"--resume=old", "-r"},
		},
		{
			name:    "optional value consumed",
			argv:    []string{"omp", "-r", "01old", "--advisor"},
			id:      "01new",
			want:    []string{"omp", "--advisor", "--resume", "01new"},
			dropped: []string{"-r 01old"},
		},
		{
			name:    "unknown options dropped",
			argv:    []string{"omp", "--frobnicate", "x", "-e", "ext.ts"},
			id:      "01y",
			want:    []string{"omp", "-e", "ext.ts", "--resume", "01y"},
			dropped: []string{"--frobnicate", "x"},
		},
		{
			name: "bashrc form kept",
			argv: wrapped("omp", "--profile", "work"),
			id:   "01z",
			want: wrapped("omp", "--profile", "work", "--resume", "01z"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := Parse(tt.argv)
			if err != nil {
				t.Fatal(err)
			}
			got, dropped := cmd.ResumeArgv(tt.id)
			if !slices.Equal(got, tt.want) || !slices.Equal(dropped, tt.dropped) {
				t.Fatalf("ResumeArgv = %q dropped %q, want %q dropped %q", got, dropped, tt.want, tt.dropped)
			}
		})
	}
}

func TestTUIArgv(t *testing.T) {
	tests := []struct {
		argv []string
		want []string
	}{
		{[]string{"opencode", "--prompt", "hi"}, []string{"opencode", "--standalone", "--prompt", "hi"}},
		{[]string{"opencode", "--standalone"}, []string{"opencode", "--standalone"}},
		{wrapped("opencode"), wrapped("opencode", "--standalone")},
		{[]string{"omp", "--continue", "hi"}, []string{"omp", "--continue", "hi"}},
	}
	for _, tt := range tests {
		cmd, err := Parse(tt.argv)
		if err != nil {
			t.Fatal(err)
		}
		if got := cmd.TUIArgv(); !slices.Equal(got, tt.want) {
			t.Errorf("TUIArgv(%q) = %q, want %q", tt.argv, got, tt.want)
		}
	}
}

func TestACPArgv(t *testing.T) {
	tests := []struct {
		name        string
		argv        []string
		want        []string
		unsupported []string
		fd3         bool
	}{
		{
			name: "omp passes kept options",
			argv: []string{"omp", "--profile", "work", "--model", "haiku", "修 bug"},
			want: []string{"omp", "acp", "--profile", "work", "--model", "haiku"},
		},
		{
			name: "opencode accepts no options",
			argv: []string{"opencode", "--auto", "--log-level", "debug"},
			want: []string{"opencode", "acp"},
		},
		{
			name: "bashrc writes protocol to fd 3",
			argv: wrapped("omp", "--auto-approve"),
			want: []string{"bash", "-ic", `exec -- "$@" 1>&3 3>&-`, "webmux", "omp", "acp", "--auto-approve"},
			fd3:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := Parse(tt.argv)
			if err != nil {
				t.Fatal(err)
			}
			got, unsupported := cmd.ACPArgv()
			if !slices.Equal(got, tt.want) || !slices.Equal(unsupported, tt.unsupported) || cmd.ProtocolOnFD3() != tt.fd3 {
				t.Fatalf("ACPArgv = %q %q fd3=%v, want %q %q fd3=%v", got, unsupported, cmd.ProtocolOnFD3(), tt.want, tt.unsupported, tt.fd3)
			}
		})
	}
}

func TestIdentify(t *testing.T) {
	tests := []struct {
		name string
		in   Signals
		want Identity
		ok   bool
	}{
		{
			name: "omp from warp",
			in:   Signals{WarpAgent: "omp", WarpPluginVersion: "18.13.1", WarpSessionID: "01a", WarpCwd: "/p", ProgramApp: "omp", RootCwd: "/other"},
			want: Identity{Agent: OMP, Version: "18.13.1", AgentSessionID: "01a", Cwd: "/p"},
			ok:   true,
		},
		{
			name: "omp before first event",
			in:   Signals{ProgramApp: "omp"},
			want: Identity{Agent: OMP},
			ok:   true,
		},
		{
			name: "opencode child record and root cwd",
			in:   Signals{ProgramApp: "opencode", ProgramChildID: "ses_e7fb", RootCwd: "/p"},
			want: Identity{Agent: OpenCode, AgentSessionID: "ses_e7fb", Cwd: "/p"},
			ok:   true,
		},
		{
			name: "opencode ignores other child ids",
			in:   Signals{ProgramApp: "opencode", ProgramChildID: "task-7", RootCwd: "/p"},
			want: Identity{Agent: OpenCode, Cwd: "/p"},
			ok:   true,
		},
		{name: "unknown", in: Signals{ProgramApp: "claude-code", WarpSessionID: "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Identify(tt.in)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("Identify = %+v %v, want %+v %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestBelowVerified(t *testing.T) {
	ompProfile, _ := Lookup(OMP)
	opencodeProfile, _ := Lookup(OpenCode)
	tests := []struct {
		profile *Profile
		version string
		want    bool
	}{
		{ompProfile, "18.0.0", true},
		{ompProfile, "v18.13.1", false},
		{ompProfile, "18.13", true},
		{ompProfile, "18.13.1-beta.2", false},
		{ompProfile, "19", false},
		{ompProfile, "dev", false},
		{ompProfile, "", false},
		{opencodeProfile, "2.2.0", false},
		{opencodeProfile, "2.1.3", true},
	}
	for _, tt := range tests {
		if got := tt.profile.BelowVerified(tt.version); got != tt.want {
			t.Errorf("%s.BelowVerified(%q) = %v, want %v", tt.profile.Name, tt.version, got, tt.want)
		}
	}
}
