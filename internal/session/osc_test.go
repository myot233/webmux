package session

import (
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
