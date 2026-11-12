package main

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/clicmd"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

// TestOpsCommandFlagsMatchTable runs each operations command with an
// undefined flag and compares the flags its own parser lists with the help
// table, so the help cannot drift from what the command accepts.
func TestOpsCommandFlagsMatchTable(t *testing.T) {
	impl := map[string]func(args []string, stderr io.Writer){
		"serve": func(args []string, stderr io.Writer) { _, _ = parseServerRun("webmux serve", args, stderr) },
		"start": func(args []string, stderr io.Writer) { _, _ = parseServerRun("webmux start", args, stderr) },
		"version": func(args []string, stderr io.Writer) {
			versionCommand(args, io.Discard, stderr)
		},
		"status":    func(args []string, stderr io.Writer) { statusCommand(args, io.Discard, stderr) },
		"stop":      func(args []string, stderr io.Writer) { stopCommand(args, io.Discard, stderr) },
		"mcp-serve": func(args []string, stderr io.Writer) { mcpCommand(args, strings.NewReader(""), io.Discard, stderr) },
		"bridge": func(args []string, stderr io.Writer) {
			bridgeCommand(args, strings.NewReader(""), io.Discard, stderr)
		},
	}
	for _, sub := range []string{"path", "show", "validate", "check", "init", "listen"} {
		impl["config "+sub] = func(args []string, stderr io.Writer) {
			configCommand(append([]string{sub}, args...), io.Discard, stderr)
		}
	}
	for _, sub := range []string{"install", "uninstall", "start", "stop", "restart", "status", "logs"} {
		impl["service "+sub] = func(args []string, stderr io.Writer) {
			serviceCommand(append([]string{sub}, args...), io.Discard, stderr)
		}
	}
	for _, sub := range []string{"check", "apply"} {
		impl["update "+sub] = func(args []string, stderr io.Writer) {
			runUpdate(append([]string{sub}, args...), io.Discard, stderr, updateEnv{})
		}
	}
	listed := regexp.MustCompile(`(?m)^  -([A-Za-z0-9-]+)`)
	e := sessionEnv(io.Discard, io.Discard)
	for _, c := range opsCommands() {
		run, ok := impl[c.Name]
		if !ok || c.Bind == nil {
			continue
		}
		var stderr bytes.Buffer
		run([]string{"--no-such-flag-in-webmux"}, &stderr)
		var got []string
		for _, m := range listed.FindAllStringSubmatch(stderr.String(), -1) {
			got = append(got, m[1])
		}
		var want []string
		fs, _ := e.FlagSet(c)
		fs.VisitAll(func(f *flag.Flag) { want = append(want, f.Name) })
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: implementation flags %v, help table %v\n%s", c.Name, got, want, stderr.String())
		}
	}
}

func TestHelpIsStdoutAndExitZero(t *testing.T) {
	for _, args := range [][]string{
		{"--help"}, {"-h"}, {"help"}, {"help", "get"}, {"get", "--help"}, {"run", "--help"},
		{"attach", "-h"}, {"ls", "--help"}, {"open", "--help"}, {"label", "--help"},
		{"status", "--help"}, {"config", "--help"}, {"config", "show", "--help"}, {"service", "logs", "-h"},
		{"update", "check", "--help"}, {"version", "--help"},
	} {
		var stdout, stderr bytes.Buffer
		code := dispatch(args, &stdout, &stderr)
		if code != contract.ExitOK || !strings.HasPrefix(stdout.String(), "Usage: webmux") || stderr.Len() != 0 {
			t.Errorf("%v: exit %d stdout %q stderr %q", args, code, firstLine(stdout.String()), stderr.String())
		}
	}
}

func TestTopLevelHelpKeepsServerOptions(t *testing.T) {
	// The Windows console client checks `webmux -h` for -log-file.
	var stdout bytes.Buffer
	dispatch([]string{"-h"}, &stdout, io.Discard)
	if !strings.Contains(stdout.String(), "-log-file") {
		t.Fatalf("top-level help lost the server options:\n%s", stdout.String())
	}
}

func TestGroupWithoutSubcommandIsUsageError(t *testing.T) {
	for _, group := range []string{"config", "service", "update"} {
		var stdout, stderr bytes.Buffer
		code := dispatch([]string{group}, &stdout, &stderr)
		if code != contract.ExitUsage || stdout.Len() != 0 || !strings.Contains(stderr.String(), "Commands:") {
			t.Errorf("%s: exit %d stdout %q stderr %q", group, code, stdout.String(), stderr.String())
		}
	}
}

func TestUsageErrorPointsToHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := dispatch([]string{"label", "abcdefgh"}, &stdout, &stderr)
	if code != contract.ExitUsage || stdout.Len() != 0 || !strings.Contains(stderr.String(), "Run 'webmux label --help' for details.") {
		t.Fatalf("exit %d stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
}

// TestCommandTableIsComplete holds every visible command to the help
// contract: a summary, two examples that its parser accepts, and a
// description for every flag.
func TestCommandTableIsComplete(t *testing.T) {
	e := sessionEnv(io.Discard, io.Discard)
	registry := commandRegistry()
	e.Commands = registry
	for _, c := range registry.Visible("unix") {
		if c.Summary == "" {
			t.Errorf("%s: no summary", c.Name)
		}
		if c.Bind == nil {
			if len(registry.Children(c.Name)) == 0 {
				t.Errorf("%s: a group without subcommands", c.Name)
			}
			continue
		}
		if len(c.Examples) < 2 {
			t.Errorf("%s: %d examples, want at least 2", c.Name, len(c.Examples))
		}
		for _, o := range e.Options(c) {
			if strings.TrimSpace(o.Description) == "" {
				t.Errorf("%s: --%s has no description", c.Name, o.Name)
			}
		}
		for _, x := range c.Examples {
			if x.Text == "" {
				t.Errorf("%s: example %q has no text", c.Name, x.Command)
			}
			args, ok := exampleArgs(x.Command, c.Name)
			if !ok {
				continue
			}
			var stderr bytes.Buffer
			probe := e
			probe.Stdout, probe.Stderr = io.Discard, &stderr
			if _, _, code, ok := probe.Parse(c, args); !ok && code != contract.ExitOK {
				t.Errorf("%s: example %q does not parse: %s", c.Name, x.Command, stderr.String())
			}
		}
	}
}

// exampleArgs extracts the arguments of command name from an example shell
// line, or false when the line runs something else.
func exampleArgs(line, name string) ([]string, bool) {
	prefix := "webmux " + name
	at := strings.Index(line, prefix)
	if at < 0 {
		return nil, false
	}
	rest := line[at+len(prefix):]
	for _, end := range []string{" | ", " && ", " > ", "; ", ")"} {
		if i := strings.Index(rest, end); i >= 0 {
			rest = rest[:i]
		}
	}
	return splitShell(rest), true
}

// splitShell splits words with single and double quotes.
func splitShell(s string) []string {
	var words []string
	var word strings.Builder
	quote := rune(0)
	inWord := false
	for _, r := range s {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			word.WriteRune(r)
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		words = append(words, word.String())
	}
	return words
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

var _ = clicmd.GroupHelp

func TestConnectLocalPrefersSocketThenConfigThenEnvironment(t *testing.T) {
	dir := t.TempDir()
	envSocket := dir + "/env.sock"
	getenv := func(name string) string {
		if name == contract.SocketEnv {
			return envSocket
		}
		return ""
	}
	var usage clicmd.UsageError
	if _, err := connectLocal(dir+"/a.sock", dir+"/config.toml", true, getenv); !errors.As(err, &usage) {
		t.Fatalf("--socket with --config: %v, want a usage error", err)
	}
	// A named socket nobody serves is "not running", never a fallback.
	for _, tc := range []struct {
		socket string
		want   string
	}{
		{dir + "/flag.sock", "flag.sock"},
		{"", "env.sock"},
	} {
		c, err := connectLocal(tc.socket, dir+"/missing.toml", false, getenv)
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Status(t.Context())
		if !errors.Is(err, localclient.ErrNotRunning) || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("socket %q: %v", tc.socket, err)
		}
	}
	// An explicit --config ignores WEBMUX_SOCKET.
	if _, err := connectLocal("", dir+"/missing.toml", true, getenv); err == nil || !strings.Contains(err.Error(), "configuration") {
		t.Fatalf("explicit config: %v", err)
	}
}

// TestHelpReferencesResolve keeps every "See also" target and every webmux
// command in an example pointing at a command this build has, and holds the
// hand-written guide pages and docs/usage/shell-control.md to the command
// table: each option they write after a command belongs to it, and only
// recipes repeats a command line of the table's examples.
func TestHelpReferencesResolve(t *testing.T) {
	registry := commandRegistry()
	e := sessionEnv(io.Discard, io.Discard)
	e.Commands = registry
	// A mention starts a line or follows a space, an opening bracket, a pipe,
	// a separator, a quote or a backtick (inline code in the guide pages).
	mention := regexp.MustCompile(`(?m)(?:^|[\s(|;&"\x60])webmux((?: [a-z][a-z0-9-]*)+)`)
	check := func(where, words string) {
		args := strings.Fields(words)
		c, rest := registry.Find(args)
		if c == nil {
			t.Errorf("%s: %q names no command", where, "webmux"+words)
			return
		}
		if c.Name == "help" && len(rest) > 0 {
			if sub, _ := registry.Find(rest); sub == nil {
				t.Errorf("%s: %q asks for help on no command", where, "webmux"+words)
			}
		}
	}
	examples := map[string]string{}
	for _, c := range registry {
		for _, see := range c.SeeAlso {
			for _, m := range mention.FindAllStringSubmatch(see, -1) {
				check(c.Name+" see also", m[1])
			}
		}
		for _, ex := range c.Examples {
			examples[strings.TrimSpace(ex.Command)] = c.Name
			for _, m := range mention.FindAllStringSubmatch(ex.Command, -1) {
				check(c.Name+" example", m[1])
			}
		}
	}
	pages, err := filepath.Glob("../../internal/agentguide/llms/*.md")
	if err != nil || len(pages) < 2 {
		t.Fatalf("guide pages: %v %v", pages, err)
	}
	for _, page := range append(pages, "../../docs/usage/shell-control.md") {
		data, err := os.ReadFile(page)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(page)
		for _, m := range mention.FindAllStringSubmatch(string(data), -1) {
			check(name, m[1])
		}
		for _, line := range commandLines(string(data)) {
			checkOptions(t, e, registry, name, line)
			// A bare command name is a mention, not a command line.
			if from, ok := examples[line]; ok && name != "recipes.md" && strings.HasPrefix(page, "../../internal/agentguide/") && !bareCommand(registry, line) {
				t.Errorf("%s repeats the %s example %q; point to webmux %s --help instead", name, from, line, from)
			}
		}
	}
	// The directory is rendered from the same table.
	directory := clicmd.Directory(registry)
	for _, c := range registry.Visible("unix") {
		if c.Bind != nil && !strings.Contains(directory, "- `"+c.Usage()+"`: ") {
			t.Errorf("the command directory lacks %s", c.Name)
		}
	}
	for _, section := range []string{"Options:", "Examples:", "Exit status:"} {
		if strings.Contains(directory, section) {
			t.Errorf("the command directory contains %q", section)
		}
	}
}

// commandLines lists the command lines of a Markdown page: each line of a
// fenced code block (continuations joined, comments dropped) and each
// inline code span.
func commandLines(page string) []string {
	var out []string
	inFence := false
	pending := ""
	for _, line := range strings.Split(page, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			line = pending + line
			pending = ""
			if strings.HasSuffix(line, "\\") {
				pending = strings.TrimSuffix(line, "\\")
				continue
			}
			if i := strings.Index(line, "  #"); i >= 0 {
				line = line[:i]
			}
			out = append(out, strings.TrimSpace(line))
			continue
		}
		for _, m := range codeSpan.FindAllStringSubmatch(line, -1) {
			out = append(out, strings.TrimSpace(m[1]))
		}
	}
	return out
}

var codeSpan = regexp.MustCompile("`([^`]+)`")

// checkOptions reports each --option after a webmux command in line that
// the command does not take.
func checkOptions(t *testing.T, e clicmd.Env, registry clicmd.Registry, where, line string) {
	t.Helper()
	words := splitShell(strings.ReplaceAll(line, ";", " ; "))
	for i := 0; i < len(words); i++ {
		w := words[i]
		if k := strings.LastIndexAny(w, "($`\"="); k >= 0 {
			w = w[k+1:]
		}
		if w != "webmux" {
			continue
		}
		var path []string
		j := i + 1
		for ; j < len(words) && commandWord.MatchString(words[j]); j++ {
			path = append(path, words[j])
		}
		c, _ := registry.Find(path)
		if c == nil {
			continue
		}
		valid := map[string]bool{"help": true}
		for _, o := range e.Options(c) {
			valid[o.Name] = true
		}
		for ; j < len(words); j++ {
			w := words[j]
			if w == "--" || w == "|" || w == "||" || w == "&&" || w == ";" || strings.HasPrefix(w, ">") || strings.HasPrefix(w, "|") {
				break
			}
			if !strings.HasPrefix(w, "--") {
				continue
			}
			name, _, _ := strings.Cut(strings.TrimPrefix(w, "--"), "=")
			name = strings.TrimRight(name, ");`'\"")
			if !valid[name] {
				t.Errorf("%s: webmux %s takes no --%s (in %q)", where, c.Name, name, line)
			}
		}
	}
}

var commandWord = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// bareCommand reports whether line is only "webmux" and a command path.
func bareCommand(registry clicmd.Registry, line string) bool {
	words := strings.Fields(line)
	if len(words) < 2 || words[0] != "webmux" {
		return false
	}
	c, rest := registry.Find(words[1:])
	return c != nil && len(rest) == 0
}
