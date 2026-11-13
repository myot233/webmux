package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/artifacts"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/session"
)

const quickTimeout = 30 * time.Second

var fakeAgent struct {
	once sync.Once
	base string
	env  []string
	dir  string
	err  error
}

// fakeAgentPath builds the ACP fake agent once, as executables named after
// the profiles agentprofile recognises.
func fakeAgentPath(t *testing.T, profile string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs the fake agent")
	}
	fakeAgent.once.Do(func() {
		fakeAgent.dir, fakeAgent.err = os.MkdirTemp(fakeAgent.base, "workspace-fakeagent-")
		if fakeAgent.err != nil {
			return
		}
		for _, name := range []string{"omp", "codex", "qoder"} {
			build := exec.Command("go", "build", "-o", filepath.Join(fakeAgent.dir, name), "../acp/testdata/fakeagent")
			build.Env = fakeAgent.env
			if out, err := build.CombinedOutput(); err != nil {
				fakeAgent.err = fmt.Errorf("go build: %w\n%s", err, out)
				return
			}
		}
	})
	if fakeAgent.err != nil {
		t.Fatal(fakeAgent.err)
	}
	return filepath.Join(fakeAgent.dir, profile)
}

type quickHarness struct {
	t        *testing.T
	s        *Service
	manager  *session.Manager
	launches *launch.Service
	hybrids  *hybrid.Service
	root     string
	store    string
	repo     Repository
	configID string
}

func newQuickHarness(t *testing.T, instruction string) *quickHarness {
	t.Helper()
	isolateGit(t)
	agent := fakeAgentPath(t, "omp")
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	gitTest(t, root, "init", "-b", "main")
	putFile(t, filepath.Join(root, "tracked"), "original\n")
	gitTest(t, root, "add", ".")
	gitTest(t, root, "commit", "-m", "initial")
	h := &quickHarness{t: t, root: root, store: t.TempDir()}
	h.manager = session.NewManager(session.Options{DefaultCwd: root})
	h.launches = launch.Open(t.TempDir())
	h.hybrids = hybrid.New(hybrid.Options{StateDir: t.TempDir(), Manager: h.manager})
	h.s = New(t.TempDir(), h.manager, h.launches, root)
	h.s.UseHybrid(h.hybrids)
	t.Cleanup(func() {
		h.hybrids.Close()
		h.s.Close()
		h.launches.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		h.manager.Shutdown(ctx)
	})
	repo, _, err := h.s.Register(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if instruction != "" {
		if repo, err = h.s.SetInitInstruction(context.Background(), repo.ID, instruction); err != nil {
			t.Fatal(err)
		}
	}
	h.repo = repo
	h.configID = h.config("agent", agent, map[string]string{"FAKEAGENT_STORE": h.store, "FAKEAGENT_TURN_DELAY": "5ms"}, true)
	return h
}

func (h *quickHarness) config(name, agent string, env map[string]string, hybridMode bool) string {
	h.t.Helper()
	c, err := h.launches.Create(launch.Input{Name: name, Cmd: []string{agent}, Env: env, Hybrid: hybridMode})
	if err != nil {
		h.t.Fatal(err)
	}
	return c.ID
}

// input is a chat started in a new worktree on branch, the quick start
// page's request.
func (h *quickHarness) input(requestID, branch string) CreateInput {
	configID := h.configID
	return CreateInput{
		ClientRequestID: requestID, ConfigID: &configID, Side: string(hybrid.SideACP),
		NewWorktree: &NewWorktree{RepositoryID: h.repo.ID, Mode: "new", Base: "HEAD", Branch: branch},
		Prompt:      "修复首页排序",
	}
}

func (h *quickHarness) start(in CreateInput) (CreateResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), quickTimeout)
	defer cancel()
	return h.s.Create(ctx, in)
}

// worktrees counts the repository's worktrees, the main one included.
func (h *quickHarness) worktrees() int {
	return strings.Count(gitTest(h.t, h.root, "worktree", "list", "--porcelain"), "worktree ")
}

func (h *quickHarness) events(id string) []hybrid.Event {
	h.t.Helper()
	sub, err := h.hybrids.Subscribe(id, nil)
	if err != nil {
		h.t.Fatal(err)
	}
	defer sub.Close()
	return sub.Events
}

// waitTurnEnd waits until session id's first turn ended.
func (h *quickHarness) waitTurnEnd(id string) {
	h.t.Helper()
	deadline := time.Now().Add(quickTimeout)
	for {
		for _, e := range h.events(id) {
			if e.Kind == hybrid.EventTurnEnd {
				return
			}
		}
		if time.Now().After(deadline) {
			h.t.Fatal("first turn did not end")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func userTexts(events []hybrid.Event) []string {
	var out []string
	for _, e := range events {
		if e.Kind == hybrid.EventMessage && e.Message != nil && e.Message.Role == hybrid.RoleUser {
			out = append(out, e.Message.Text)
		}
	}
	return out
}

func notices(events []hybrid.Event) []string {
	var out []string
	for _, e := range events {
		if e.Kind == hybrid.EventNotice && e.Notice != nil {
			out = append(out, e.Notice.Text)
		}
	}
	return out
}

// setConfigCalls counts the agent's session/set_config_option calls.
func (h *quickHarness) setConfigCalls() int {
	h.t.Helper()
	b, err := os.ReadFile(filepath.Join(h.store, "events.log"))
	if err != nil && !os.IsNotExist(err) {
		h.t.Fatal(err)
	}
	return strings.Count(string(b), " call "+acp.MethodSessionSetConfigOption+" ")
}

func quickFailure(t *testing.T, err error, step string, kind error) *CreateError {
	t.Helper()
	var failed *CreateError
	if !errors.As(err, &failed) || failed.Step != step || !errors.Is(err, kind) {
		t.Fatalf("error %v, want step %s kind %v", err, step, kind)
	}
	return failed
}

// A quick start creates the worktree next to the repository, starts the
// chat there, sets only the options that differ from the seeded ones,
// warns about an option the agent lacks, and sends the instruction and the
// user's text as the first prompt.
func TestQuickStartRunsEveryStep(t *testing.T) {
	h := newQuickHarness(t, "先读 AGENTS.md")
	in := h.input("quick-start-001", "wt/brisk-otter")
	in.UseInitInstruction = true
	in.AgentConfig = map[string]string{"mode": "code", "model": "smart", "effort": "high"}
	in.PermissionMode = "ask"
	result, err := h.start(in)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(h.root+".worktrees", "brisk-otter")
	if result.Worktree.Path != wantPath || result.Worktree.Branch == nil || *result.Worktree.Branch != "wt/brisk-otter" {
		t.Fatalf("worktree %+v", result.Worktree)
	}
	item, err := h.manager.Get(result.Session.Info().ID)
	if err != nil {
		t.Fatal(err)
	}
	info := item.Info()
	if info.Hybrid == nil || info.Hybrid.Side != string(hybrid.SideACP) || info.Cwd != wantPath || info.ConfigID != h.configID ||
		info.Workspace == nil || info.Workspace.RepositoryID != h.repo.ID || info.Workspace.WorktreeID != result.Worktree.ID {
		t.Fatalf("session %+v hybrid %+v", info, info.Hybrid)
	}
	if len(result.ConfigWarnings) != 1 || !strings.Contains(result.ConfigWarnings[0], "effort") {
		t.Fatalf("warnings %q", result.ConfigWarnings)
	}
	h.waitTurnEnd(result.Session.Info().ID)
	events := h.events(result.Session.Info().ID)
	if got := userTexts(events); len(got) != 1 || got[0] != "先读 AGENTS.md\n\n修复首页排序" {
		t.Fatalf("user messages %q", got)
	}
	if got := notices(events); !reflectContains(got, result.ConfigWarnings[0]) {
		t.Fatalf("notices %q lack the warning", got)
	}
	rec, err := h.hybrids.Record(result.Session.Info().ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Name != "" || rec.Config["model"] != "smart" || rec.Config["mode"] != "code" || rec.PermissionMode != hybrid.PermissionAsk {
		t.Fatalf("record %+v", rec)
	}
	if got := h.setConfigCalls(); got != 1 {
		t.Fatalf("set_config_option calls %d, want only the changed model", got)
	}
}

// The instruction goes first only when the request asks for it; its
// {{name}} references become quick command text, and {{dir}} in that text
// becomes the new session's artifacts folder. The user's own text is never
// rewritten.
func TestQuickStartInstructionTemplate(t *testing.T) {
	h := newQuickHarness(t, "规则：\n{{规则}}\n{{ 产物 }}")
	folders := artifacts.New(artifacts.Options{TempDir: t.TempDir(), StateDir: t.TempDir(), Manager: h.manager})
	t.Cleanup(folders.Close)
	h.s.UseArtifacts(folders)
	h.s.UseQuickCommands(func() []QuickCommand {
		return []QuickCommand{{Name: "规则", Text: "先读 AGENTS.md"}, {Name: "产物", Text: "文件放 {{dir}}，不要{{规则}}"}}
	})

	off := h.input("quick-start-off1", "wt/off")
	off.Prompt = "原样 {{dir}}"
	result, err := h.start(off)
	if err != nil {
		t.Fatal(err)
	}
	h.waitTurnEnd(result.Session.Info().ID)
	if got := userTexts(h.events(result.Session.Info().ID)); len(got) != 1 || got[0] != "原样 {{dir}}" {
		t.Fatalf("unticked instruction still sent: %q", got)
	}

	on := h.input("quick-start-on01", "wt/on")
	on.Prompt = "原样 {{dir}}"
	on.UseInitInstruction = true
	result, err = h.start(on)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := folders.Existing(result.Session.Info().ID)
	if err != nil {
		t.Fatalf("folder for {{dir}} not created: %v", err)
	}
	h.waitTurnEnd(result.Session.Info().ID)
	want := "规则：\n先读 AGENTS.md\n文件放 " + dir + "，不要{{规则}}\n\n原样 {{dir}}"
	if got := userTexts(h.events(result.Session.Info().ID)); len(got) != 1 || got[0] != want {
		t.Fatalf("first prompt %q, want %q", got, want)
	}
}

// {{webmux}} becomes the new session's brief, written once the session
// and its artifacts folder exist; without a brief writer the quick start
// stops before anything is created.
func TestQuickStartInstructionWebMuxBrief(t *testing.T) {
	h := newQuickHarness(t, "{{ webmux }}\n\n{{user}}")
	before := h.worktrees()
	in := h.input("quick-start-wmx1", "wt/wmx1")
	in.UseInitInstruction = true
	_, err := h.start(in)
	failed := quickFailure(t, err, StepPrecheck, ErrUnavailable)
	if !strings.Contains(failed.Error(), "WebMux 工具说明不可用") || h.worktrees() != before {
		t.Fatalf("without a brief: %v", err)
	}

	folders := artifacts.New(artifacts.Options{TempDir: t.TempDir(), StateDir: t.TempDir(), Manager: h.manager})
	t.Cleanup(folders.Close)
	h.s.UseArtifacts(folders)
	h.s.UseBrief(func(id, dir string) string { return "BRIEF " + id + " " + dir + " {{dir}}" })
	in = h.input("quick-start-wmx2", "wt/wmx2")
	in.Prompt = "修复 lint"
	in.UseInitInstruction = true
	result, err := h.start(in)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := folders.Existing(result.Session.Info().ID)
	if err != nil {
		t.Fatalf("artifacts folder not created for the brief: %v", err)
	}
	h.waitTurnEnd(result.Session.Info().ID)
	// The brief is inserted after {{dir}}, so nothing in it is replaced.
	want := "BRIEF " + result.Session.Info().ID + " " + dir + " {{dir}}\n\n修复 lint"
	if got := userTexts(h.events(result.Session.Info().ID)); len(got) != 1 || got[0] != want {
		t.Fatalf("first prompt %q, want %q", got, want)
	}
}

// {{user}} puts the user's message inside the instruction instead of after
// it, wherever it is written; the message is not read as a reference.
func TestQuickStartInstructionUserVariable(t *testing.T) {
	h := newQuickHarness(t, "任务：{{ user }}\n先读 AGENTS.md")
	h.s.UseQuickCommands(func() []QuickCommand { return []QuickCommand{{Name: "user", Text: "不应被使用"}} })
	in := h.input("quick-start-usr1", "wt/usr")
	in.Prompt = "改 {{规则}} 与 {{dir}}"
	in.UseInitInstruction = true
	result, err := h.start(in)
	if err != nil {
		t.Fatal(err)
	}
	h.waitTurnEnd(result.Session.Info().ID)
	want := "任务：改 {{规则}} 与 {{dir}}\n先读 AGENTS.md"
	if got := userTexts(h.events(result.Session.Info().ID)); len(got) != 1 || got[0] != want {
		t.Fatalf("first prompt %q, want %q", got, want)
	}
}

// ExpandPrompt fills the user's own {{dir}} and {{webmux}} once the session
// exists: {{dir}} first, the brief last so its text is never read again;
// other names stay as written, and without the flag nothing changes. The
// repository instruction's own expansion is unaffected, and a retry with the
// same request ID gets the first run's session and prompt.
func TestQuickStartExpandPrompt(t *testing.T) {
	h := newQuickHarness(t, "规则 {{dir}}\n{{user}}")
	folders := artifacts.New(artifacts.Options{TempDir: t.TempDir(), StateDir: t.TempDir(), Manager: h.manager})
	t.Cleanup(folders.Close)
	h.s.UseArtifacts(folders)
	h.s.UseBrief(func(id, dir string) string { return "BRIEF " + id + " {{dir}} {{user}}" })

	const prompt = "放 {{dir}}\n{{webmux}}\n{{user}} {{规则}}"
	off := h.input("quick-start-xoff", "wt/xoff")
	off.Prompt = prompt
	result, err := h.start(off)
	if err != nil {
		t.Fatal(err)
	}
	h.waitTurnEnd(result.Session.Info().ID)
	if got := userTexts(h.events(result.Session.Info().ID)); len(got) != 1 || got[0] != prompt {
		t.Fatalf("without ExpandPrompt %q, want the text as written", got)
	}
	if _, err := folders.Existing(result.Session.Info().ID); err == nil {
		t.Fatal("a folder was created without ExpandPrompt")
	}

	on := h.input("quick-start-xon1", "wt/xon")
	on.Prompt = prompt
	on.ExpandPrompt = true
	result, err = h.start(on)
	if err != nil {
		t.Fatal(err)
	}
	id := result.Session.Info().ID
	dir, err := folders.Existing(id)
	if err != nil {
		t.Fatalf("folder for {{dir}} not created: %v", err)
	}
	h.waitTurnEnd(id)
	want := "放 " + dir + "\nBRIEF " + id + " {{dir}} {{user}}\n{{user}} {{规则}}"
	if got := userTexts(h.events(id)); len(got) != 1 || got[0] != want {
		t.Fatalf("expanded prompt %q, want %q", got, want)
	}
	retry, err := h.start(on)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Session.Info().ID != id {
		t.Fatalf("retry made session %s, want %s", retry.Session.Info().ID, id)
	}
	if got := userTexts(h.events(id)); len(got) != 1 || got[0] != want {
		t.Fatalf("after retry %q, want the first run's prompt once", got)
	}

	// With the instruction, the expanded text lands at {{user}} and is not
	// scanned again.
	both := h.input("quick-start-xboth", "wt/xboth")
	both.Prompt = "{{webmux}}"
	both.ExpandPrompt = true
	both.UseInitInstruction = true
	result, err = h.start(both)
	if err != nil {
		t.Fatal(err)
	}
	id = result.Session.Info().ID
	dir, err = folders.Existing(id)
	if err != nil {
		t.Fatal(err)
	}
	h.waitTurnEnd(id)
	want = "规则 " + dir + "\nBRIEF " + id + " {{dir}} {{user}}"
	if got := userTexts(h.events(id)); len(got) != 1 || got[0] != want {
		t.Fatalf("instruction with expanded prompt %q, want %q", got, want)
	}
}

// An expanding prompt that needs the artifacts folder or the brief stops
// the quick start before anything is created when either is missing.
func TestQuickStartExpandPromptUnavailable(t *testing.T) {
	h := newQuickHarness(t, "")
	before := h.worktrees()
	for name, prompt := range map[string]string{"dir": "放 {{dir}}", "brief": "{{webmux}}"} {
		in := h.input("quick-start-xun-"+name, "wt/xun-"+name)
		in.Prompt = prompt
		in.ExpandPrompt = true
		_, err := h.start(in)
		quickFailure(t, err, StepPrecheck, ErrUnavailable)
	}
	if h.worktrees() != before {
		t.Fatal("a worktree was created for an unavailable placeholder")
	}
	plain := h.input("quick-start-xun-plain", "wt/xun-plain")
	plain.Prompt = "没有占位符 {{规则}}"
	plain.ExpandPrompt = true
	if _, err := h.start(plain); err != nil {
		t.Fatalf("a prompt without placeholders needs neither: %v", err)
	}
}

// A reference to a quick command that does not exist stops the quick start
// before anything is created.
func TestQuickStartInstructionUnknownReference(t *testing.T) {
	h := newQuickHarness(t, "{{缺失}}和{{也缺}}和{{缺失}}")
	h.s.UseQuickCommands(func() []QuickCommand { return nil })
	before := h.worktrees()
	in := h.input("quick-start-bad1", "wt/bad")
	in.UseInitInstruction = true
	_, err := h.start(in)
	failed := quickFailure(t, err, StepPrecheck, ErrInvalid)
	if !strings.Contains(failed.Error(), "缺失、也缺") {
		t.Fatalf("error %q does not name the missing commands", failed.Error())
	}
	if h.worktrees() != before {
		t.Fatal("a worktree was created for an invalid instruction")
	}
}

func reflectContains(list []string, item string) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}

// A value the agent does not offer keeps the agent's value, is reported
// and shown in the chat, and the first prompt is still sent.
func TestQuickStartInvalidConfigValueWarns(t *testing.T) {
	h := newQuickHarness(t, "")
	in := h.input("quick-start-002", "wt/calm-heron")
	in.AgentConfig = map[string]string{"model": "huge"}
	result, err := h.start(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ConfigWarnings) != 1 || !strings.Contains(result.ConfigWarnings[0], "Model") || !strings.Contains(result.ConfigWarnings[0], "huge") {
		t.Fatalf("warnings %q", result.ConfigWarnings)
	}
	h.waitTurnEnd(result.Session.Info().ID)
	events := h.events(result.Session.Info().ID)
	if got := userTexts(events); len(got) != 1 || got[0] != "修复首页排序" {
		t.Fatalf("user messages %q", got)
	}
	if !reflectContains(notices(events), result.ConfigWarnings[0]) {
		t.Fatalf("notices %q", notices(events))
	}
	if rec, err := h.hybrids.Record(result.Session.Info().ID); err != nil || rec.Config["model"] != "fast" {
		t.Fatalf("record %+v %v", rec, err)
	}
	if got := h.setConfigCalls(); got != 0 {
		t.Fatalf("set_config_option calls %d", got)
	}
}

// Whatever can be checked first is checked before anything is created;
// later failures name their step and keep what was created.
func TestQuickStartFailures(t *testing.T) {
	h := newQuickHarness(t, "")
	plain := h.config("plain", "/bin/sh", nil, false)
	codex := h.config("codex", fakeAgentPath(t, "codex"), map[string]string{"PATH": "/nonexistent", "FAKEAGENT_STORE": h.store}, true)
	broken := h.config("broken", fakeAgentPath(t, "omp"), nil, true)

	in := h.input("quick-start-101", "wt/plain")
	in.ConfigID = &plain
	_, err := h.start(in)
	quickFailure(t, err, StepPrecheck, ErrInvalid)

	in = h.input("quick-start-102", "wt/codex")
	in.ConfigID = &codex
	_, err = h.start(in)
	var missing *hybrid.ConnectorMissingError
	if !errors.As(err, &missing) || missing.Connector.Executable != "codex-acp" {
		t.Fatalf("connector: %v", err)
	}
	quickFailure(t, err, StepPrecheck, hybrid.ErrConnectorMissing)

	in = h.input("quick-start-103", "wt/missing")
	in.NewWorktree.RepositoryID = "missing"
	_, err = h.start(in)
	quickFailure(t, err, StepPrecheck, ErrNotFound)

	in = h.input("quick-start-104", "wt/blank")
	in.Prompt = " \n"
	in.Attachments = []string{"/tmp/x"}
	_, err = h.start(in)
	quickFailure(t, err, StepPrecheck, ErrInvalid)

	in = h.input("bad id", "wt/bad-id")
	_, err = h.start(in)
	quickFailure(t, err, StepPrecheck, ErrInvalid)

	if got := h.worktrees(); got != 1 || len(h.manager.List()) != 0 {
		t.Fatalf("a refused quick start created %d worktrees, sessions %+v", got, h.manager.List())
	}

	// The agent cannot start: the worktree stays, nothing is rolled back.
	in = h.input("quick-start-105", "wt/broken")
	in.ConfigID = &broken
	_, err = h.start(in)
	failed := quickFailure(t, err, StepSession, hybrid.ErrAgent)
	if failed.Worktree == nil || failed.SessionID != "" {
		t.Fatalf("session failure %+v", failed)
	}
	if stat, err := os.Stat(failed.Worktree.Path); err != nil || !stat.IsDir() {
		t.Fatalf("worktree removed: %v", err)
	}
	if got := h.worktrees(); got != 2 {
		t.Fatalf("worktrees %d", got)
	}

	// The branch's directory now exists: a conflict at the worktree step.
	in = h.input("quick-start-106", "wt/broken")
	_, err = h.start(in)
	failed = quickFailure(t, err, StepWorktree, ErrConflict)
	if failed.Worktree != nil || len(h.manager.List()) != 0 {
		t.Fatalf("worktree failure %+v", failed)
	}
}

// Concurrent requests with one ID share one run; a retry within the
// retention gets the same outcome; after it the ID runs again.
func TestQuickStartDedupesByRequestID(t *testing.T) {
	h := newQuickHarness(t, "")
	h.s.creates.retention = time.Second
	in := h.input("quick-start-201", "wt/shared")
	const callers = 4
	results := make([]CreateResult, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = h.start(in)
		}()
	}
	wg.Wait()
	for i := range callers {
		if errs[i] != nil || results[i].Session.Info().ID != results[0].Session.Info().ID || results[i].Worktree.ID != results[0].Worktree.ID {
			t.Fatalf("caller %d: %+v %v, first %+v", i, results[i], errs[i], results[0])
		}
	}
	if got := h.worktrees(); got != 2 || len(h.manager.List()) != 1 {
		t.Fatalf("duplicates created %d worktrees, %d sessions", got, len(h.manager.List()))
	}
	again, err := h.start(in)
	if err != nil || again.Session.Info().ID != results[0].Session.Info().ID {
		t.Fatalf("retry: %+v %v", again, err)
	}
	h.waitTurnEnd(results[0].Session.Info().ID)
	if got := userTexts(h.events(results[0].Session.Info().ID)); len(got) != 1 {
		t.Fatalf("user messages %q", got)
	}
	// Past the retention the same ID is a new request: its branch now
	// exists, so it fails at the worktree step.
	deadline := time.Now().Add(quickTimeout)
	for {
		_, err := h.start(in)
		if err != nil {
			quickFailure(t, err, StepWorktree, ErrConflict)
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("finished quick start never expired")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// A quick start leaves the same session and record as starting the chat by
// hand: CreateWorktree, CreateSession with startSide acp, then Prompt.
func TestQuickStartMatchesManualStart(t *testing.T) {
	h := newQuickHarness(t, "")
	quick, err := h.start(h.input("quick-start-301", "wt/quick"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), quickTimeout)
	defer cancel()
	tree, err := h.s.CreateWorktree(ctx, h.repo.ID, CreateRequest{Path: filepath.Join(h.root+".worktrees", "manual"), Mode: "new", Branch: "wt/manual", Base: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	configID := h.configID
	item, err := h.s.CreateSession(ctx, CreateInput{ConfigID: &configID, Side: "acp", Workspace: &Reference{RepositoryID: h.repo.ID, WorktreeID: tree.ID}})
	if err != nil {
		t.Fatal(err)
	}
	manual := item.Info().ID
	if _, err := h.hybrids.Prompt(ctx, manual, hybrid.PromptInput{ClientRequestID: "manual-prompt-1", Text: "修复首页排序"}); err != nil {
		t.Fatal(err)
	}
	h.waitTurnEnd(quick.Session.Info().ID)
	h.waitTurnEnd(manual)
	info := func(id string) session.Info {
		item, err := h.manager.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		i := item.Info()
		i.ID, i.Generation, i.Title, i.Cwd, i.CreatedAt, i.LastActiveAt, i.Preview = "", "", "", "", time.Time{}, time.Time{}, ""
		if i.Workspace != nil {
			w := *i.Workspace
			w.WorktreeID, w.WorktreePath = "", ""
			i.Workspace = &w
		}
		if i.Hybrid != nil {
			hy := *i.Hybrid
			hy.AgentSessionID = nil
			i.Hybrid = &hy
		}
		if i.Attention != nil {
			a := *i.Attention
			a.Since = time.Time{}
			i.Attention = &a
		}
		return i
	}
	if a, b := info(quick.Session.Info().ID), info(manual); !reflect.DeepEqual(a, b) {
		t.Fatalf("session info differs:\nquick  %+v\nmanual %+v", a, b)
	}
	record := func(id string) hybrid.Record {
		rec, err := h.hybrids.Record(id)
		if err != nil {
			t.Fatal(err)
		}
		rec.ID, rec.Title, rec.Cwd, rec.AgentSessionID, rec.PID, rec.Process = "", "", "", "", 0, nil
		rec.CreatedAt, rec.UpdatedAt = time.Time{}, time.Time{}
		if rec.Workspace != nil {
			rec.Workspace.WorktreeID = ""
		}
		return rec
	}
	if a, b := record(quick.Session.Info().ID), record(manual); !reflect.DeepEqual(a, b) {
		t.Fatalf("record differs:\nquick  %+v\nmanual %+v", a, b)
	}
	if a, b := userTexts(h.events(quick.Session.Info().ID)), userTexts(h.events(manual)); !reflect.DeepEqual(a, b) {
		t.Fatalf("first messages differ: %q %q", a, b)
	}
}

// A terminal start types the instruction and the user's message into the
// program once it is quiet, then presses Enter; the chat side gets them as
// its first prompt. The instruction needs a repository location.
func TestCreateSessionFirstMessage(t *testing.T) {
	h := newQuickHarness(t, "先读 AGENTS.md\n{{user}}")
	ref := &Reference{RepositoryID: h.repo.ID, WorktreeID: h.repo.Worktrees[0].ID}
	ctx := context.Background()

	if _, err := h.s.CreateSession(ctx, CreateInput{Cmd: []string{"/bin/cat"}, UseInitInstruction: true}); errorKind(err) != ErrInvalid {
		t.Fatalf("instruction without repository: %v", err)
	}

	item, err := h.s.CreateSession(ctx, CreateInput{
		Cmd: []string{"/bin/sh", "-c", "echo ready; exec cat"}, Workspace: ref,
		UseInitInstruction: true, Prompt: "修复排序",
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for strings.Count(string(item.Buffer()), "修复排序") < 2 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	// cat echoes each typed line once as echo and once as output; Enter ends
	// the line, so the whole message appears twice.
	if got := strings.Count(string(item.Buffer()), "修复排序"); got < 2 {
		t.Fatalf("terminal did not receive the message and Enter: %q", item.Buffer())
	}
	if !strings.Contains(string(item.Buffer()), "先读 AGENTS.md") {
		t.Fatalf("instruction missing: %q", item.Buffer())
	}

	chat, err := h.s.CreateSession(ctx, CreateInput{ConfigID: &h.configID, Side: "acp", Workspace: ref, UseInitInstruction: true, Prompt: "修复排序"})
	if err != nil {
		t.Fatal(err)
	}
	id := chat.Info().ID
	h.waitTurnEnd(id)
	if got := userTexts(h.events(id)); len(got) != 1 || got[0] != "先读 AGENTS.md\n修复排序" {
		t.Fatalf("first chat prompt %q", got)
	}
}

// A terminal start without a name, agent or plain shell, is titled by the
// user's first prompt alone: never the initialization instruction,
// placeholders or the directory; a given name wins.
func TestCreateSessionTerminalPromptTitle(t *testing.T) {
	h := newQuickHarness(t, "先读 AGENTS.md\n{{user}}")
	folders := artifacts.New(artifacts.Options{TempDir: t.TempDir(), StateDir: t.TempDir(), Manager: h.manager})
	t.Cleanup(folders.Close)
	h.s.UseArtifacts(folders)
	h.s.UseBrief(func(id, dir string) string { return "BRIEF" })
	ref := &Reference{RepositoryID: h.repo.ID, WorktreeID: h.repo.Worktrees[0].ID}
	dir := filepath.Base(h.root)
	shell := h.config("shell", "/bin/cat", nil, false)
	long := strings.Repeat("长", 100)
	for _, tc := range []struct {
		name string
		in   CreateInput
		want string
	}{
		{"prompt", CreateInput{Prompt: " 修复\n排序\t问题 "}, "修复 排序 问题"},
		{"instruction not in title", CreateInput{Prompt: "修复排序", UseInitInstruction: true}, "修复排序"},
		{"instruction only", CreateInput{UseInitInstruction: true}, dir},
		{"explicit name", CreateInput{Name: "我的名字", Prompt: "修复排序"}, "我的名字"},
		{"no prompt", CreateInput{}, dir},
		{"long", CreateInput{Prompt: long}, strings.Repeat("长", 79) + "…"},
		{"placeholder dropped", CreateInput{Prompt: "{{dir}} 修复排序 {{webmux}}", ExpandPrompt: true}, "修复排序"},
		{"only placeholder", CreateInput{Prompt: "{{dir}}", ExpandPrompt: true}, dir},
		{"plain shell", CreateInput{ConfigID: &shell, Prompt: " 修复\n排序 "}, "修复 排序"},
		{"plain shell without prompt", CreateInput{ConfigID: &shell}, dir},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in
			if in.ConfigID == nil {
				in.ConfigID = &h.configID
			}
			in.Side = "tui"
			in.Workspace = ref
			item, err := h.s.CreateSession(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}
			if got := item.Info().Title; got != tc.want {
				t.Fatalf("title %q, want %q", got, tc.want)
			}
		})
	}
}

// The chat side's temporary title comes from the user's text, not from the
// instruction prefixed to the first prompt.
func TestCreateSessionChatTitleIgnoresInstruction(t *testing.T) {
	h := newQuickHarness(t, "先读 AGENTS.md\n{{user}}")
	ref := &Reference{RepositoryID: h.repo.ID, WorktreeID: h.repo.Worktrees[0].ID}
	chat, err := h.s.CreateSession(context.Background(), CreateInput{ConfigID: &h.configID, Side: "acp", Workspace: ref, UseInitInstruction: true, Prompt: "修复排序"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for chat.Info().Title != "修复排序" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := chat.Info().Title; got != "修复排序" {
		t.Fatalf("title %q", got)
	}
}

// A terminal quick start creates the worktree and the agent's TUI, adopts
// staged attachments and appends their paths to the initial prompt (passed on
// the command line for migrated agents without persisting it in SessionInfo.Cmd
// or the hybrid record, or typed into the terminal once quiet for unmigrated
// agents), and rejects chat-only options.
func TestQuickStartTerminalSide(t *testing.T) {
	h := newQuickHarness(t, "")
	folders := artifacts.New(artifacts.Options{TempDir: t.TempDir(), StateDir: t.TempDir(), Manager: h.manager})
	t.Cleanup(folders.Close)
	h.s.UseArtifacts(folders)

	bad := h.input("quick-start-tui0", "wt/tui-bad")
	bad.Side = "tui"
	bad.AgentConfig = map[string]string{"model": "smart"}
	if _, err := h.start(bad); err == nil || errorKind(err) != ErrInvalid {
		t.Fatalf("agentConfig on a terminal start: %v", err)
	}
	staging, err := folders.EnsureStaging()
	if err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(staging, "shot.png")
	putFile(t, staged, "png")

	in := h.input("quick-start-tui1", "wt/tui")
	in.Side = "tui"
	in.Attachments = []string{staged}
	result, err := h.start(in)
	if err != nil {
		t.Fatal(err)
	}
	id := result.Session.Info().ID
	uploads, err := folders.EnsureUploads(id)
	if err != nil {
		t.Fatal(err)
	}
	adopted := filepath.Join(uploads, "shot.png")
	if _, err := os.Stat(adopted); err != nil {
		t.Fatalf("adopted attachment missing: %v", err)
	}
	item, err := h.manager.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for (!strings.Contains(string(item.Buffer()), "echo: "+in.Prompt) || !strings.Contains(string(item.Buffer()), adopted)) && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	// Migrated agent (omp) receives the prompt (with adopted attachment paths)
	// through argv rather than typed into stdin.
	if !strings.Contains(string(item.Buffer()), "echo: "+in.Prompt) || !strings.Contains(string(item.Buffer()), adopted) {
		t.Fatalf("terminal never ran initial prompt %q with %q: %q", in.Prompt, adopted, item.Buffer())
	}
	for _, arg := range item.Info().Cmd {
		if strings.Contains(arg, in.Prompt) || strings.Contains(arg, adopted) {
			t.Fatalf("session Info.Cmd leaked prompt: %q", item.Info().Cmd)
		}
	}
	rec, err := h.hybrids.Record(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range rec.Cmd {
		if strings.Contains(arg, in.Prompt) || strings.Contains(arg, adopted) {
			t.Fatalf("hybrid record Cmd leaked prompt: %q", rec.Cmd)
		}
	}
	if len(result.ConfigWarnings) != 0 {
		t.Fatalf("warnings %v", result.ConfigWarnings)
	}

	// Unmigrated agent (qoder) falls back to typing the prompt into the PTY
	// once the terminal is quiet.
	qoderCfg := h.config("qoder", fakeAgentPath(t, "qoder"), map[string]string{"FAKEAGENT_STORE": h.store, "FAKEAGENT_TURN_DELAY": "5ms"}, true)
	fallback := h.input("quick-start-tui2", "wt/tui-qoder")
	fallback.ConfigID = &qoderCfg
	fallback.Side = "tui"
	result2, err := h.start(fallback)
	if err != nil {
		t.Fatal(err)
	}
	item2, err := h.manager.Get(result2.Session.Info().ID)
	if err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(15 * time.Second)
	for !strings.Contains(string(item2.Buffer()), "echo: "+fallback.Prompt) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(string(item2.Buffer()), "echo: "+fallback.Prompt) {
		t.Fatalf("unmigrated terminal never received fallback prompt %q: %q", fallback.Prompt, item2.Buffer())
	}
}

// A new worktree without a branch gets wt/<date>-<time> next to the
// repository and a base from the repository's default.
func TestCreateGeneratesBranch(t *testing.T) {
	h := newQuickHarness(t, "")
	in := h.input("create-branch-1", "")
	in.NewWorktree.Base = ""
	result, err := h.start(in)
	if err != nil {
		t.Fatal(err)
	}
	tree := result.Worktree
	if tree == nil || tree.Branch == nil || !strings.HasPrefix(*tree.Branch, "wt/") {
		t.Fatalf("worktree %+v", tree)
	}
	if want := filepath.Join(h.root+".worktrees", filepath.Base(*tree.Branch)); tree.Path != want {
		t.Fatalf("path %s, want %s", tree.Path, want)
	}
	taken := func(name string) bool { return name == "wt/20260102-030405" }
	if got := generatedBranch(time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local), func(b string) bool { return !taken(b) }); got != "wt/20260102-030405-2" {
		t.Fatalf("generated %s", got)
	}
}

// Options of the chat view are refused for a terminal start before
// anything is created, as is a second location.
func TestCreateRefusesBeforeCreating(t *testing.T) {
	h := newQuickHarness(t, "")
	in := h.input("create-refuse-1", "wt/refuse")
	in.Side = string(hybrid.SideTUI)
	in.PermissionMode = "auto"
	_, err := h.start(in)
	quickFailure(t, err, StepPrecheck, ErrInvalid)
	in = h.input("create-refuse-2", "wt/refuse")
	in.Cwd = h.root
	_, err = h.start(in)
	quickFailure(t, err, StepPrecheck, ErrInvalid)
	in = h.input("create-refuse-3", "wt/refuse")
	in.NewWorktree.Mode = "other"
	_, err = h.start(in)
	quickFailure(t, err, StepPrecheck, ErrInvalid)
	if got := h.worktrees(); got != 1 || len(h.manager.List()) != 0 {
		t.Fatalf("a refused create made %d worktrees, sessions %+v", got, h.manager.List())
	}
}

func TestCreateInitialInputReceiptOnlyForSubmittedChat(t *testing.T) {
	tests := []struct {
		name, instruction, prompt string
		useInit, want             bool
	}{
		{name: "empty init and whitespace prompt", prompt: " \n ", useInit: true},
		{name: "whitespace prompt", prompt: " \t "},
		{name: "instruction only", instruction: "Read AGENTS.md", useInit: true, want: true},
		{name: "prompt", prompt: "hello", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newQuickHarness(t, tt.instruction)
			in := h.input("initial-receipt-test", "wt/receipt")
			in.Prompt = tt.prompt
			in.UseInitInstruction = tt.useInit
			result, err := h.start(in)
			if err != nil {
				t.Fatal(err)
			}
			if (result.InitialInput != nil) != tt.want {
				t.Fatalf("initial receipt %+v want submitted=%v", result.InitialInput, tt.want)
			}
			if tt.want && result.InitialInput.TurnID == "" {
				t.Fatalf("submitted prompt has no turn receipt: %+v", result.InitialInput)
			}
			if !tt.want && len(userTexts(h.events(result.Session.Info().ID))) != 0 {
				t.Fatal("empty launch submitted a user prompt")
			}
		})
	}
}
