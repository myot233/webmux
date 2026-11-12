package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/agentprofile/agents"
)

// agentNeutralGo are the production Go directories that must name no
// registered agent: per-agent behaviour belongs to an adapter under
// internal/agentprofile/<agent>. Subdirectories other than testdata are
// covered too, except the adapters and their registry.
var agentNeutralGo = []string{"internal/acp", "internal/hybrid", "internal/agentprofile"}

// agentNeutralWebExempt are frontend files allowed to name agents: the
// launch-config icon catalog, whose IDs are icon names shared with
// internal/launch, not agent identifiers.
var agentNeutralWebExempt = map[string]bool{
	"web/src/domains/settings/launch-configs.ts":          true,
	"web/src/components/settings/LaunchConfigIcon.svelte": true,
}

// agentWords are the identifiers of every registered agent: names and
// executables, including connectors'.
func agentWords() map[string]bool {
	words := map[string]bool{}
	for _, a := range agents.Default().Agents() {
		p := a.Profile()
		words[p.Name] = true
		for _, exe := range p.Executables {
			words[exe] = true
		}
		if p.Connector != nil {
			words[p.Connector.Executable] = true
		}
	}
	return words
}

// TestGenericCodeNamesNoAgent keeps agent differences behind the adapter
// interface: adding an agent must not need changes to ACP, hybrid, the
// adapter core or the frontend.
func TestGenericCodeNamesNoAgent(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	words := agentWords()
	var bad []string
	for _, dir := range agentNeutralGo {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if d.IsDir() {
				if d.Name() == "testdata" || (strings.HasPrefix(rel, "internal/agentprofile/") && rel != "internal/agentprofile") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			bad = append(bad, goAgentLiterals(t, path, rel, words)...)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	bad = append(bad, webAgentLiterals(t, root, words)...)
	if len(bad) > 0 {
		t.Fatalf("agent names in agent-neutral code (move the behaviour into an adapter in internal/agentprofile/<agent>, or send it from the backend):\n%s", strings.Join(bad, "\n"))
	}
}

func goAgentLiterals(t *testing.T, path, rel string, words map[string]bool) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var bad []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if v, err := strconv.Unquote(lit.Value); err == nil && words[v] {
			bad = append(bad, rel+":"+strconv.Itoa(fset.Position(lit.Pos()).Line)+": "+lit.Value)
		}
		return true
	})
	return bad
}

var webQuoted = regexp.MustCompile(`["'` + "`" + `]([a-z][a-z0-9-]*)["'` + "`" + `]`)

func webAgentLiterals(t *testing.T, root string, words map[string]bool) []string {
	t.Helper()
	var bad []string
	err := filepath.WalkDir(filepath.Join(root, "web/src"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if agentNeutralWebExempt[rel] || (!strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".svelte")) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(data), "\n") {
			for _, m := range webQuoted.FindAllStringSubmatch(line, -1) {
				if words[m[1]] {
					bad = append(bad, rel+":"+strconv.Itoa(i+1)+": "+m[0])
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return bad
}
