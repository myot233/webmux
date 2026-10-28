package session

import (
	"os"
	"strings"
	"testing"
)

type signalLog []string

func (l *signalLog) handler() *SignalHandler {
	return &SignalHandler{
		Title:  func(s string) { *l = append(*l, "title:"+s) },
		Bell:   func() { *l = append(*l, "bell") },
		Notify: func() { *l = append(*l, "notify") },
		Event:  func(e string) { *l = append(*l, "event:"+e) },
		Cwd:    func(path, host string) { *l = append(*l, "cwd:"+path+"|"+host) },
		ProgramStatus: func(state, kind string) {
			*l = append(*l, "status:"+state+"/"+kind)
		},
		Probe:  func() { *l = append(*l, "probe") },
		Prompt: func() { *l = append(*l, "prompt") },
		Reset:  func() { *l = append(*l, "reset") },
	}
}

func scanSignals(chunks ...string) []string {
	var scanner OSCScanner
	var log signalLog
	h := log.handler()
	for _, chunk := range chunks {
		scanner.Feed([]byte(chunk), h)
	}
	return log
}

func TestOSCChunkBoundaries(t *testing.T) {
	for _, sequence := range []string{"\x1b]0;  标题  \a", "\x1b]2;  标题  \x1b\\"} {
		for split := 0; split <= len(sequence); split++ {
			got := scanSignals(sequence[:split], sequence[split:])
			if len(got) != 1 || got[0] != "title:标题" {
				t.Fatalf("split %d: %v", split, got)
			}
		}
	}
}
func TestOSCBoundsAndRecovery(t *testing.T) {
	got := scanSignals("\x1b]52;ignored\a\x1b]2;" + strings.Repeat("x", 513) + "\a\x1b]0;ok\a")
	if len(got) != 1 || got[0] != "title:ok" {
		t.Fatal(got)
	}
}

func TestSignalChunkBoundaries(t *testing.T) {
	sequence := "a\a\x1b]777;notify;warp://cli-agent;{\"v\":1,\"event\":\"question_asked\",\"summary\":\"问题\"}\x1b\\" +
		"\x1b]9;done\a\x1b]777;notify;omp;Complete\a"
	want := strings.Join(scanSignals(sequence), ",")
	if want != "bell,event:question_asked,notify,notify" {
		t.Fatal(want)
	}
	bytewise := make([]string, len(sequence))
	for i := range sequence {
		bytewise[i] = sequence[i : i+1]
	}
	if got := strings.Join(scanSignals(bytewise...), ","); got != want {
		t.Fatalf("bytewise %s, whole %s", got, want)
	}
	for split := 0; split <= len(sequence); split++ {
		if got := strings.Join(scanSignals(sequence[:split], sequence[split:]), ","); got != want {
			t.Fatalf("split %d: %s", split, got)
		}
	}
}

func TestOSC7DirectoryReports(t *testing.T) {
	for _, row := range []struct{ name, input, want string }{
		{"bel", "\x1b]7;file:///home/u/proj\a", "cwd:/home/u/proj|"},
		{"st localhost", "\x1b]7;file://localhost/home/u/my%20proj\x1b\\", "cwd:/home/u/my proj|localhost"},
		{"remote host", "\x1b]7;file://build.example/home/u\x07", "cwd:/home/u|build.example"},
		{"root", "\x1b]7;file:///\x07", "cwd:/|"},
	} {
		t.Run(row.name, func(t *testing.T) {
			if got := strings.Join(scanSignals(row.input), ","); got != row.want {
				t.Fatalf("got %q, want %q", got, row.want)
			}
		})
	}
}

func TestOSC7IgnoresInvalidReports(t *testing.T) {
	for _, row := range []struct{ name, input string }{
		{"other scheme", "\x1b]7;http://host/tmp\a"},
		{"relative path", "\x1b]7;file:relative\a"},
		{"missing path", "\x1b]7;file://host\a"},
		{"user info", "\x1b]7;file://user@host/tmp\a"},
		{"port", "\x1b]7;file://host:22/tmp\a"},
		{"query", "\x1b]7;file:///tmp?x\a"},
		{"fragment", "\x1b]7;file:///tmp#x\a"},
		{"control byte", "\x1b]7;file:///tmp/%00bad\a"},
		{"c1 control", "\x1b]7;file:///tmp/%c2%9bbad\a"},
		{"oversized", "\x1b]7;file:///tmp/" + strings.Repeat("x", maxSignalPayload) + "\a"},
	} {
		t.Run(row.name, func(t *testing.T) {
			if got := scanSignals(row.input); len(got) != 0 {
				t.Fatalf("invalid report produced %v", got)
			}
		})
	}
}

func TestReportedHostClassification(t *testing.T) {
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		name, host, local string
		want              bool
	}{
		{"empty", "", hostname, true},
		{"localhost", "localhost", hostname, true},
		{"loopback v4", "127.0.0.1", hostname, true},
		{"loopback v6", "::1", hostname, true},
		{"local name", hostname, hostname, true},
		{"local case", strings.ToUpper(hostname), hostname, true},
		{"trailing dot", hostname + ".", hostname, true},
		{"short name", shortHost(normalizeHost(hostname)), hostname, true},
		{"remote name", "build.example", hostname, false},
		{"different domains", "host.other.example", "host.local.example", false},
		{"bare remote", "webmux-remote", "webmux-local", false},
	} {
		t.Run(row.name, func(t *testing.T) {
			if got := reportedHostIsLocal(row.host, row.local); got != row.want {
				t.Fatalf("reportedHostIsLocal(%q, %q) = %v, want %v", row.host, row.local, got, row.want)
			}
		})
	}
}

func TestSignalClassification(t *testing.T) {
	long := "\x1b]777;notify;title;" + strings.Repeat("x", 5000) + "\a"
	for _, row := range []struct{ name, input, want string }{
		{"bare bell", "\a", "bell"},
		{"title terminator", "\x1b]0;t\a", "title:t"},
		{"progress", "\x1b]9;4;3\a\x1b]9;4;0;\a", ""},
		{"conemu subcommand", "\x1b]9;9;/tmp\a", ""},
		{"osc 9 notify", "\x1b]9;Waiting for input\a", "notify"},
		{"osc 9 numeric text", "\x1b]9;42 files\a", "notify"},
		{"osc 777 notify", "\x1b]777;notify;omp;Complete\x1b\\", "notify"},
		{"osc 777 other", "\x1b]777;preexec\a", ""},
		{"agent event", "\x1b]777;notify;warp://cli-agent;{\"v\":1,\"event\":\"stop\",\"response\":\"x;y\"}\a", "event:stop"},
		{"agent future version", "\x1b]777;notify;warp://cli-agent;{\"v\":2,\"event\":\"stop\"}\a", "event:stop"},
		{"invalid json", "\x1b]777;notify;warp://cli-agent;{not json\a", ""},
		{"missing v", "\x1b]777;notify;warp://cli-agent;{\"event\":\"stop\"}\a", ""},
		{"zero v", "\x1b]777;notify;warp://cli-agent;{\"v\":0,\"event\":\"stop\"}\a", ""},
		{"fractional v", "\x1b]777;notify;warp://cli-agent;{\"v\":1.5,\"event\":\"stop\"}\a", ""},
		{"non-string event", "\x1b]777;notify;warp://cli-agent;{\"v\":1,\"event\":3}\a", ""},
		{"json array", "\x1b]777;notify;warp://cli-agent;[1]\a", ""},
		{"oversized", long + "\a", "bell"},
		{"recovers after oversized", long + "\x1b]9;x\a", "notify"},
		{"bell inside dcs", "\x1bPtmux;\a\x1b\\", ""},
		{"bell inside apc", "\x1b_Gx\a\x1b\\\a", "bell"},
	} {
		t.Run(row.name, func(t *testing.T) {
			if got := strings.Join(scanSignals(row.input), ","); got != row.want {
				t.Fatalf("got %q, want %q", got, row.want)
			}
		})
	}
}

func TestProgramStatusClassification(t *testing.T) {
	long := "\x1b]7501;state=done:msg=" + strings.Repeat("x", 5000) + "\a"
	for _, row := range []struct{ name, input, want string }{
		{"blocked permission", "\x1b]7501;state=blocked:kind=permission:app=codex\x1b\\", "status:blocked/permission"},
		{"bel terminator", "\x1b]7501;state=idle:app=claude-code\a", "status:idle/"},
		{"whitespace", "\x1b]7501; state = blocked : kind = question \a", "status:blocked/question"},
		{"auth kind", "\x1b]7501;state=blocked:kind=auth\a", "status:blocked/auth"},
		{"unknown kind", "\x1b]7501;state=blocked:kind=maybe\a", "status:blocked/"},
		{"kind outside blocked", "\x1b]7501;state=done:kind=question\a", "status:done/"},
		{"repeated key", "\x1b]7501;state=working:state=done\a", "status:done/"},
		{"malformed pairs skipped", "\x1b]7501;junk:State=x:state=error:=y:msg=aGk=\a", "status:error/"},
		{"long key skipped", "\x1b]7501;abcdefghijklmnopq=1:state=idle\a", "status:idle/"},
		{"clear", "\x1b]7501;state=clear\x1b\\", "status:clear/"},
		{"unknown state", "\x1b]7501;state=thinking:kind=question\a", ""},
		{"missing state", "\x1b]7501;kind=question\a", ""},
		{"child record", "\x1b]7501;id=task/1:state=blocked:kind=question\a", ""},
		{"malformed id", "\x1b]7501;id=a b:state=done\a", ""},
		{"probe", "\x1b]7501;?\a\x1b[c", "probe"},
		{"probe st", "\x1b]7501;?\x1b\\", "probe"},
		{"reply is not a probe", "\x1b]7501;?:states=idle\x1b\\", ""},
		{"oversized", long, ""},
		{"recovers after oversized", long + "\x1b]7501;state=done\a", "status:done/"},
		{"semantic prompt", "\x1b]133;A\a\x1b]133;A;cl=m\x1b\\\x1b]133;B\a\x1b]133;D;0\a", "prompt,prompt"},
		{"full reset", "a\x1bcb", "reset"},
	} {
		t.Run(row.name, func(t *testing.T) {
			if got := strings.Join(scanSignals(row.input), ","); got != row.want {
				t.Fatalf("got %q, want %q", got, row.want)
			}
		})
	}
}

func TestProgramStatusChunkBoundaries(t *testing.T) {
	sequence := "\x1b]7501;?\a\x1b]7501;state=blocked:kind=question\x1b\\\x1b]133;A\a\x1bc"
	want := "probe,status:blocked/question,prompt,reset"
	bytewise := make([]string, len(sequence))
	for i := range sequence {
		bytewise[i] = sequence[i : i+1]
	}
	if got := strings.Join(scanSignals(bytewise...), ","); got != want {
		t.Fatalf("bytewise %s, want %s", got, want)
	}
}

// scanIdentity scans chunks with an Identity handler and returns the merged
// identity together with every other signal, in order.
func scanIdentity(chunks ...string) (Identity, []string) {
	var scanner OSCScanner
	var log signalLog
	var identity Identity
	h := log.handler()
	h.Identity = identity.mergeSignals
	for _, chunk := range chunks {
		scanner.Feed([]byte(chunk), h)
	}
	return identity, log
}

func warpEvent(fields string) string {
	return "\x1b]777;notify;warp://cli-agent;{\"v\":1,\"event\":\"session_start\"" + fields + "}\a"
}

func TestIdentityExtraction(t *testing.T) {
	long := strings.Repeat("a", maxIdentityName+1)
	deep := strings.Repeat("a/", maxProgramIDSegments) + "a"
	for _, row := range []struct {
		name, input string
		want        Identity
	}{
		{"omp session_start", warpEvent(`,"agent":"omp","plugin_version":"18.13.1","session_id":"01JX-abc","cwd":"/home/u/项目","query":"secret"`),
			Identity{WarpAgent: "omp", WarpPluginVersion: "18.13.1", WarpSessionID: "01JX-abc", WarpCwd: "/home/u/项目"}},
		{"root app", "\x1b]7501;state=idle:app=omp\a", Identity{ProgramApp: "omp"}},
		{"opencode child", "\x1b]7501;state=working:id=ses_abc:app=opencode:title=dGl0bGU=\x1b\\", Identity{ProgramChildID: "ses_abc"}},
		{"child first segment", "\x1b]7501;state=blocked:kind=question:id=ses_abc/task.1/x+y-z\a", Identity{ProgramChildID: "ses_abc"}},
		{"child clear", "\x1b]7501;state=clear:id=ses_abc\a", Identity{ProgramChildID: "ses_abc"}},
		{"later child wins", "\x1b]7501;state=working:id=ses_a\a\x1b]7501;state=working:id=ses_b\a", Identity{ProgramChildID: "ses_b"}},
		{"fields merge across reports", warpEvent(`,"agent":"omp","session_id":"one","cwd":"/p"`) + warpEvent(`,"session_id":"two"`) + "\x1b]7501;state=idle:app=omp\a",
			Identity{WarpAgent: "omp", WarpSessionID: "two", WarpCwd: "/p", ProgramApp: "omp"}},
		{"non-string fields ignored", warpEvent(`,"agent":["omp"],"session_id":7,"plugin_version":null,"cwd":"/ok"`), Identity{WarpCwd: "/ok"}},
		{"control characters ignored", warpEvent(`,"agent":"omp","session_id":"a\nb","cwd":"/a\u0007b"`), Identity{WarpAgent: "omp"}},
		{"oversized name ignored", warpEvent(`,"agent":"omp","session_id":"` + long + `"`), Identity{WarpAgent: "omp"}},
		{"name at the bound", warpEvent(`,"session_id":"` + long[1:] + `"`), Identity{WarpSessionID: long[1:]}},
		{"invalid envelope", "\x1b]777;notify;warp://cli-agent;{\"agent\":\"omp\",\"session_id\":\"x\",\"event\":\"stop\"}\a", Identity{}},
		{"other notify title", "\x1b]777;notify;omp;{\"v\":1,\"event\":\"stop\",\"session_id\":\"x\"}\a", Identity{}},
		{"oversized app ignored", "\x1b]7501;state=idle:app=" + long + "\a", Identity{}},
		{"non-printable app ignored", "\x1b]7501;state=idle:app=om\x01p\a", Identity{}},
		{"invalid utf-8 app ignored", "\x1b]7501;state=idle:app=om\xffp\a", Identity{}},
		{"app of unknown state", "\x1b]7501;state=thinking:app=omp\a", Identity{}},
		{"child app not kept", "\x1b]7501;state=working:id=ses_abc:app=opencode\a", Identity{ProgramChildID: "ses_abc"}},
		{"child of unknown state", "\x1b]7501;state=thinking:id=ses_abc\a", Identity{}},
		{"malformed id", "\x1b]7501;id=a b:state=done\a", Identity{}},
		{"empty id", "\x1b]7501;id=:state=done:app=omp\a", Identity{}},
		{"empty segment", "\x1b]7501;id=ses_abc//x:state=done\a", Identity{}},
		{"too many segments", "\x1b]7501;id=" + deep + ":state=done\a", Identity{}},
		{"long segment", "\x1b]7501;id=" + strings.Repeat("s", maxProgramIDSegment+1) + ":state=done\a", Identity{}},
		{"long id", "\x1b]7501;id=" + strings.Repeat(strings.Repeat("s", 30)+"/", 5) + "s:state=done\a", Identity{}},
		{"oversized sequence", warpEvent(`,"session_id":"x","query":"` + strings.Repeat("q", maxSignalPayload) + `"`), Identity{}},
	} {
		t.Run(row.name, func(t *testing.T) {
			got, signals := scanIdentity(row.input)
			if got != row.want {
				t.Fatalf("identity %+v, want %+v", got, row.want)
			}
			// Extraction changes no other signal.
			if want := scanSignals(row.input); strings.Join(signals, ",") != strings.Join(want, ",") {
				t.Fatalf("signals %v, want %v", signals, want)
			}
		})
	}
}

func TestIdentityChunkBoundaries(t *testing.T) {
	sequence := warpEvent(`,"agent":"omp","session_id":"s-1","cwd":"/w"`) + "\x1b]7501;state=working:id=ses_abc\x1b\\\x1b]7501;state=idle:app=omp\a"
	want := Identity{WarpAgent: "omp", WarpSessionID: "s-1", WarpCwd: "/w", ProgramApp: "omp", ProgramChildID: "ses_abc"}
	bytewise := make([]string, len(sequence))
	for i := range sequence {
		bytewise[i] = sequence[i : i+1]
	}
	got, signals := scanIdentity(bytewise...)
	if got != want || strings.Join(signals, ",") != "event:session_start,status:idle/" {
		t.Fatalf("bytewise %+v %v", got, signals)
	}
}
