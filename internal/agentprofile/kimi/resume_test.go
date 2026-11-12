package kimi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ap "github.com/rainchan/webmux/internal/agentprofile"
)

func TestPrepareTUIResumeLocalizesACPBinding(t *testing.T) {
	home := t.TempDir()
	const sessionID = "session_test"
	wire := writeSessionWire(t, home, "work", sessionID, strings.Join([]string{
		`{"type":"environment.set_binding","environmentId":"local","agentId":"main"}`,
		`{"type":"environment.set_binding","environmentId":"acp:session_test","cwd":"/work/a&b","agentId":"main"}`,
		`{"type":"environment.set_binding","environmentId":"acp:session_other","agentId":"main"}`,
		`{"type":"context.append_message","message":{"role":"user","content":"unchanged"}}`,
	}, "\n")+"\n")
	before, err := os.ReadFile(wire)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := ap.PrepareTUIResume(Agent, sessionID, func(name string) string {
		if name == "KIMI_CODE_HOME" {
			return home
		}
		return ""
	}, t.TempDir())
	if err != nil || !changed {
		t.Fatalf("PrepareTUIResume changed=%v err=%v", changed, err)
	}
	after, err := os.ReadFile(wire)
	if err != nil {
		t.Fatal(err)
	}
	text := string(after)
	if strings.Contains(text, `"environmentId":"acp:session_test"`) || !strings.Contains(text, `"environmentId":"local"`) {
		t.Fatal("ACP binding was not replaced with local")
	}
	if !strings.Contains(text, `"/work/a&b"`) {
		t.Fatal("cwd was escaped or altered")
	}
	if !strings.Contains(text, "acp:session_other") {
		t.Fatal("another session's binding changed")
	}
	if !strings.Contains(text, `{"type":"context.append_message","message":{"role":"user","content":"unchanged"}}`) {
		t.Fatal("unrelated event changed")
	}
	backup, err := os.ReadFile(wire + ".before-local-binding")
	if err != nil || string(backup) != string(before) {
		t.Fatalf("backup does not match original: %v", err)
	}
	changed, err = ap.PrepareTUIResume(Agent, sessionID, func(string) string { return home }, "")
	if err != nil || changed {
		t.Fatalf("second PrepareTUIResume changed=%v err=%v", changed, err)
	}
}

func TestPrepareTUIResumeSkipsMissingWireAndBinding(t *testing.T) {
	home := t.TempDir()
	changed, err := ap.PrepareTUIResume(Agent, "session_missing", func(string) string { return home }, "")
	if err != nil || changed {
		t.Fatalf("missing wire changed=%v err=%v", changed, err)
	}
	wire := writeSessionWire(t, home, "work", "session_local", `{"type":"environment.set_binding","environmentId":"local"}`+"\n")
	changed, err = ap.PrepareTUIResume(Agent, "session_local", func(string) string { return home }, "")
	if err != nil || changed {
		t.Fatalf("local binding changed=%v err=%v", changed, err)
	}
	if _, err := os.Stat(wire + ".before-local-binding"); !os.IsNotExist(err) {
		t.Fatalf("unexpected backup: %v", err)
	}
}

func TestPrepareTUIResumeRejectsInvalidOrDuplicateWire(t *testing.T) {
	home := t.TempDir()
	wire := writeSessionWire(t, home, "one", "session_bad", "{not json}\n")
	before, _ := os.ReadFile(wire)
	if _, err := ap.PrepareTUIResume(Agent, "session_bad", func(string) string { return home }, ""); err == nil {
		t.Fatal("invalid wire succeeded")
	}
	after, _ := os.ReadFile(wire)
	if string(after) != string(before) {
		t.Fatal("invalid wire was modified")
	}
	writeSessionWire(t, home, "two", "session_bad", "{}\n")
	if _, err := ap.PrepareTUIResume(Agent, "session_bad", func(string) string { return home }, ""); err == nil {
		t.Fatal("duplicate session wires succeeded")
	}
	if _, err := ap.PrepareTUIResume(Agent, "../escape", func(string) string { return home }, ""); err == nil {
		t.Fatal("invalid session ID succeeded")
	}
	if _, err := ap.PrepareTUIResume(Agent, "session_bad", nil, ""); err == nil {
		t.Fatal("missing home succeeded")
	}
}

func writeSessionWire(t *testing.T, home, work, sessionID, content string) string {
	t.Helper()
	wire := filepath.Join(home, "sessions", work, sessionID, "agents", "main", "wire.jsonl")
	if err := os.MkdirAll(filepath.Dir(wire), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wire, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return wire
}
