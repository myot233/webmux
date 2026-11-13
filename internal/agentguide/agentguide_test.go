package agentguide

import (
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var link = regexp.MustCompile(`\]\((/llms[^)]*)\)`)

// testGuide has a stand-in command directory.
var testGuide = New("# 命令目录\n\n- `webmux ls [options]`: List sessions.\n")

func TestIndexFollowsLLMSTxt(t *testing.T) {
	index := testGuide.Index(nil, "")
	lines := strings.Split(index, "\n")
	if lines[0] != "# WebMux" || !strings.HasPrefix(lines[2], "> ") {
		t.Fatalf("index must start with an H1 and a blockquote summary:\n%s", index)
	}
	if !strings.Contains(index, "\n## Optional\n") {
		t.Fatal("index has no Optional section")
	}
	for _, topic := range Topics {
		if !strings.Contains(index, "](/llms/"+topic.Name+".md): ") {
			t.Errorf("index does not link topic %s", topic.Name)
		}
	}
}

func TestEveryLinkResolvesAndEveryPageIsListed(t *testing.T) {
	names := make([]string, 0, len(Topics))
	for _, topic := range Topics {
		names = append(names, topic.Name)
		if _, ok := testGuide.Page(topic.Name); !ok {
			t.Errorf("topic %s has no page", topic.Name)
		}
	}
	entries, err := fs.ReadDir(pages, "llms")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), ".md")
		if name == CommandsTopic {
			t.Errorf("llms/%s is rendered from the command table; it must not be embedded", entry.Name())
		}
		if !slices.Contains(names, name) {
			t.Errorf("page %s is not in the index", entry.Name())
		}
	}
	for _, m := range link.FindAllStringSubmatch(testGuide.Full(nil, ""), -1) {
		target := m[1]
		if target == "/llms.txt" || target == "/llms-full.txt" {
			continue
		}
		name, ok := strings.CutPrefix(target, "/llms/")
		name, isMarkdown := strings.CutSuffix(name, ".md")
		if _, found := testGuide.Page(name); !ok || !isMarkdown || !found {
			t.Errorf("broken link %s", target)
		}
	}
}

func TestPublicGuideCarriesNoInstanceFacts(t *testing.T) {
	instance := &Instance{Version: "9.9.9-secret", LocalURL: "http://10.1.2.3:7681", Bin: "/opt/private/webmux"}
	instance.SetSocket("/run/user/1000/webmux.sock")
	public := testGuide.Full(nil, "")
	for _, leak := range []string{"9.9.9-secret", "10.1.2.3", "/opt/private", "/run/user", "本实例："} {
		if strings.Contains(public, leak) {
			t.Errorf("public guide contains %q", leak)
		}
	}
	// The guide never names an installed path for the executable.
	if regexp.MustCompile("(/usr|/opt|/home|~)[^ `]*/webmux([ `\n]|$)").MatchString(public) {
		t.Error("public guide hard-codes an executable path")
	}
	local := testGuide.Index(instance, "")
	for _, fact := range []string{"9.9.9-secret", "/opt/private/webmux", "/run/user/1000/webmux.sock"} {
		if !strings.Contains(local, fact) {
			t.Errorf("local index lacks %q", fact)
		}
	}
}

func TestBrief(t *testing.T) {
	instance := &Instance{Version: "1.0", LocalURL: "http://127.0.0.1:7681/", Bin: "/usr/bin/webmux"}
	instance.SetSocket("/run/user/1000/webmux/webmux.sock")
	brief := instance.Brief(BriefInput{SessionID: "ab2cd3ef", Dir: "/tmp/webmux-artifacts-1000-x/ab2cd3ef"})
	for _, want := range []string{
		"ab2cd3ef", "/usr/bin/webmux", "/run/user/1000/webmux/webmux.sock", "http://127.0.0.1:7681/llms.txt",
		"/usr/bin/webmux guide", "/tmp/webmux-artifacts-1000-x/ab2cd3ef", "/llms/artifacts.md",
	} {
		if !strings.Contains(brief, want) {
			t.Errorf("brief lacks %q:\n%s", want, brief)
		}
	}
	if strings.Contains(brief, "{{") {
		t.Errorf("brief holds a reference: %s", brief)
	}
	bare := (&Instance{LocalURL: "http://127.0.0.1:7681"}).Brief(BriefInput{SessionID: "ab2cd3ef", Dir: "/d"})
	if strings.Contains(bare, "WebMux 命令：") || !strings.Contains(bare, "webmux guide") {
		t.Errorf("brief without an executable: %s", bare)
	}
}

func TestCommandsTopicIsTheInjectedDirectory(t *testing.T) {
	page, ok := testGuide.Page(CommandsTopic)
	if !ok || !strings.Contains(page, "`webmux ls [options]`") {
		t.Fatalf("commands topic %q", page)
	}
	if !strings.Contains(testGuide.Full(nil, ""), page) {
		t.Fatal("llms-full.txt lacks the command directory")
	}
	if page, ok := (Guide{}).Page(CommandsTopic); !ok || page != "" {
		t.Fatalf("zero guide commands topic: %q %v", page, ok)
	}
}

// TestHandWrittenTopicsDoNotRepeatHelp keeps the help sections out of the
// guide: options and examples live in --help.
func TestHandWrittenTopicsDoNotRepeatHelp(t *testing.T) {
	full := testGuide.Full(nil, "")
	for _, section := range []string{"Options:", "Examples:", "Exit status:", "\nUsage: "} {
		if strings.Contains(full, section) {
			t.Errorf("llms-full.txt contains the help section %q", section)
		}
	}
}

func TestLocalGuideNamesTheCallerSession(t *testing.T) {
	instance := &Instance{Version: "1.0", LocalURL: "http://127.0.0.1:7681"}
	for name, session := range map[string]string{"inside a session": "ab2cd3ef", "outside every session": ""} {
		for _, text := range []string{testGuide.Index(instance, session), testGuide.Full(instance, session)} {
			if got := strings.Contains(text, "- 本会话 ID：`ab2cd3ef`"); got != (session != "") {
				t.Errorf("%s: names the session = %v", name, got)
			}
		}
	}
	// The session ID is an instance fact: the unauthenticated listener never sees it.
	if strings.Contains(testGuide.Index(nil, "ab2cd3ef"), "ab2cd3ef") {
		t.Error("guide without an instance names a session")
	}
}
