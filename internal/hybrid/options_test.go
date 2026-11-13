package hybrid

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/agentprofile/codex"
	"github.com/rainchan/webmux/internal/agentprofile/omp"
)

func (h *harness) agentOptions(configID string) []ConfigOption {
	h.t.Helper()
	options, err := h.svc.AgentOptions(configID)
	if err != nil {
		h.t.Fatal(err)
	}
	return options
}

func (h *harness) optionsFile() string {
	h.t.Helper()
	b, err := os.ReadFile(filepath.Join(h.state, "agent-options.json"))
	if err != nil {
		h.t.Fatal(err)
	}
	return string(b)
}

// Each chat session's reported options are remembered for its launch
// config, follow later changes, survive a restart and go with the config.
func TestAgentOptionsRememberedPerConfig(t *testing.T) {
	h := newHarness(t, 0)
	if got := h.agentOptions("cfgtest01"); got == nil || len(got) != 0 {
		t.Fatalf("options before any session: %#v", got)
	}
	id := h.create(h.input(omp.Name, SideACP)).Info().ID
	options := h.agentOptions("cfgtest01")
	if len(options) != 2 || configValue(options, "model") != "fast" || configValue(options, "mode") != "code" {
		t.Fatalf("reported options %+v", options)
	}
	for _, o := range options {
		if len(o.Options) != 2 || o.Name == "" || o.Category == "" {
			t.Fatalf("option without its choices %+v", o)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	smart := "smart"
	if err := h.svc.SetConfig(ctx, id, ConfigInput{ClientRequestID: rid(), ConfigID: "model", Value: &smart}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "changed option remembered", func() bool { return configValue(h.agentOptions("cfgtest01"), "model") == "smart" })
	if got := h.agentOptions("other-config"); len(got) != 0 {
		t.Fatalf("another config's options %+v", got)
	}

	h.svc.Close()
	file := h.optionsFile()
	if !strings.Contains(file, `"configId": "cfgtest01"`) && !strings.Contains(file, `"configId":"cfgtest01"`) {
		t.Fatalf("options file %s", file)
	}
	if strings.Contains(file, "secret-value") {
		t.Fatal("options file holds an environment value")
	}
	h.open(0)
	restored := h.agentOptions("cfgtest01")
	if configValue(restored, "model") != "smart" || len(restored) != 2 || len(restored[0].Options) != 2 {
		t.Fatalf("options after restart %+v", restored)
	}

	if err := h.svc.ForgetAgentOptions("cfgtest01"); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.ForgetAgentOptions("never-seen"); err != nil {
		t.Fatal(err)
	}
	if got := h.agentOptions("cfgtest01"); len(got) != 0 {
		t.Fatalf("options after forget %+v", got)
	}
	if strings.Contains(h.optionsFile(), "cfgtest01") {
		t.Fatal("forgotten options still on disk")
	}
}

// A corrupt agent-options.json is reported and left alone; hybrid
// sessions keep working.
func TestAgentOptionsCorruptFileReported(t *testing.T) {
	h := newHarness(t, 0)
	h.svc.Close()
	path := filepath.Join(h.state, "agent-options.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.open(0)
	if !strings.Contains(h.logs.String(), "agent options unavailable") {
		t.Fatalf("corrupt file not logged: %s", h.logs.String())
	}
	options, err := h.svc.AgentOptions("cfgtest01")
	if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "agent-options.json") || options == nil || len(options) != 0 {
		t.Fatalf("corrupt options: %+v %v", options, err)
	}
	h.create(h.input(omp.Name, SideACP))
	if err := h.svc.ForgetAgentOptions("cfgtest01"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("forget with a corrupt file: %v", err)
	}
	h.svc.Close()
	if b, _ := os.ReadFile(path); string(b) != "{broken" {
		t.Fatalf("corrupt options file rewritten: %q", b)
	}
}

// CheckCreate refuses what Create would refuse before it starts anything,
// a missing connector included, and registers nothing.
func TestCheckCreate(t *testing.T) {
	h := newHarness(t, 0)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if err := h.svc.CheckCreate(ctx, h.input(omp.Name, SideACP)); err != nil {
		t.Fatalf("omp: %v", err)
	}
	if err := h.svc.CheckCreate(ctx, h.codexInput(SideACP, "")); err != nil {
		t.Fatalf("codex with its connector: %v", err)
	}
	dir := t.TempDir()
	if err := os.Symlink(agentPath(t, codex.Name), filepath.Join(dir, "codex")); err != nil {
		t.Fatal(err)
	}
	err := h.svc.CheckCreate(ctx, h.codexInput(SideACP, dir))
	var missing *ConnectorMissingError
	if !errors.As(err, &missing) || missing.Connector.Executable != "codex-acp" {
		t.Fatalf("codex without its connector: %v", err)
	}
	if err := h.svc.CheckCreate(ctx, h.codexInput(SideTUI, dir)); err != nil {
		t.Fatalf("terminal-first codex: %v", err)
	}
	invalid := h.input(omp.Name, SideACP)
	invalid.Cmd = []string{"/bin/sh"}
	if err := h.svc.CheckCreate(ctx, invalid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unsupported command: %v", err)
	}
	if recs, _ := h.svc.Records(false); len(recs) != 0 || len(h.manager.List()) != 0 {
		t.Fatalf("checks left records %v sessions %v", recs, h.manager.List())
	}
	if log, _ := readEventsLog(h.store); strings.Contains(log, " acp ") {
		t.Fatal("a check started an ACP process")
	}
}

// ProbeAgentOptions reads the options the agent offers before any chat
// session exists, remembers them for the launch config, and leaves no
// session, record or process behind; a missing connector is reported.
func TestProbeAgentOptions(t *testing.T) {
	h := newHarness(t, 0)
	h.svc.mcpServers = func(context.Context) []acp.MCPServer {
		t.Error("options probe requested instance MCP tools")
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	options, err := h.svc.ProbeAgentOptions(ctx, h.input(omp.Name, SideTUI))
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 2 || configValue(options, "model") != "fast" || configValue(options, "mode") != "code" {
		t.Fatalf("probed options %+v", options)
	}
	if got := h.agentOptions("cfgtest01"); len(got) != 2 || configValue(got, "model") != "fast" {
		t.Fatalf("remembered options %+v", got)
	}
	if recs, _ := h.svc.Records(false); len(recs) != 0 || len(h.manager.List()) != 0 {
		t.Fatalf("probe left records %v sessions %v", recs, h.manager.List())
	}
	dir := t.TempDir()
	if err := os.Symlink(agentPath(t, codex.Name), filepath.Join(dir, "codex")); err != nil {
		t.Fatal(err)
	}
	_, err = h.svc.ProbeAgentOptions(ctx, h.codexInput(SideACP, dir))
	var missing *ConnectorMissingError
	if !errors.As(err, &missing) {
		t.Fatalf("codex without its connector: %v", err)
	}
}
