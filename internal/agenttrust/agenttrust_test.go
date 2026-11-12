package agenttrust

import (
	"strings"

	"encoding/json"
	"github.com/BurntSushi/toml"
	"os"
	"path/filepath"
	"testing"
)

func newTrust(home string, env map[string]string) *Trust {
	return &Trust{
		home:      func() (string, error) { return home, nil },
		getenv:    func(k string) string { return env[k] },
		lookupEnv: func(k string) (string, bool) { v, ok := env[k]; return v, ok },
	}
}

func canonical(t *testing.T, path string) string {
	t.Helper()
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func readProjects(t *testing.T, path string) map[string]map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Projects map[string]map[string]any `json:"projects"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON %q: %v", data, err)
	}
	return doc.Projects
}

func TestPrepareTrustsClaudeWorkdirKeepingOtherFields(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "proj")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(home, ".claude.json")
	seed := `{"numStartups": 7, "projects": {"/other": {"allowedTools": ["Bash"]}, "` + canonical(t, work) + `": {"lastCost": 1.5}}}`
	if err := os.WriteFile(config, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	newTrust(home, nil).Prepare(work, []string{"claude", "--dangerously-skip-permissions"}, nil)

	projects := readProjects(t, config)
	entry := projects[canonical(t, work)]
	if entry["hasTrustDialogAccepted"] != true || entry["lastCost"] != 1.5 {
		t.Fatalf("entry = %v", entry)
	}
	if projects["/other"]["allowedTools"] == nil {
		t.Fatalf("other project lost: %v", projects)
	}
	var top map[string]any
	data, _ := os.ReadFile(config)
	if err := json.Unmarshal(data, &top); err != nil || top["numStartups"] != float64(7) {
		t.Fatalf("top level = %v, %v", top, err)
	}
	info, err := os.Stat(config)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, %v", info.Mode(), err)
	}
}

func TestPrepareRecognisesWrappedAndPathedClaude(t *testing.T) {
	for _, argv := range [][]string{
		{"bash", "-ic", `exec -- "$@"`, "webmux", "claude", "--dangerously-skip-permissions"},
		{"/home/u/.local/bin/claude"},
		{"zsh", "-ic", `exec claude "$@"`, "claude"},
	} {
		home := t.TempDir()
		work := filepath.Join(home, "p")
		if err := os.Mkdir(work, 0o755); err != nil {
			t.Fatal(err)
		}
		config := filepath.Join(home, ".claude.json")
		if err := os.WriteFile(config, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		newTrust(home, nil).Prepare(work, argv, nil)
		if readProjects(t, config)[canonical(t, work)]["hasTrustDialogAccepted"] != true {
			t.Errorf("argv %q not trusted", argv)
		}
	}
}

func TestPrepareIgnoresOtherCommands(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "p")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(config, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	newTrust(home, nil).Prepare(work, []string{"/bin/sh", "-l", "-i"}, nil)
	newTrust(home, nil).Prepare(work, []string{"codex"}, nil)
	if data, _ := os.ReadFile(config); string(data) != "{}" {
		t.Fatalf("config changed: %s", data)
	}
}

func TestPrepareHonoursClaudeConfigDir(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "p")
	fromEnv := filepath.Join(home, "envcfg")
	fromServer := filepath.Join(home, "servercfg")
	for _, dir := range []string{work, fromEnv, fromServer} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	trust := newTrust(home, map[string]string{"CLAUDE_CONFIG_DIR": fromServer})
	trust.Prepare(work, []string{"claude"}, map[string]string{"CLAUDE_CONFIG_DIR": fromEnv})
	if readProjects(t, filepath.Join(fromEnv, ".claude.json"))[canonical(t, work)]["hasTrustDialogAccepted"] != true {
		t.Fatal("session CLAUDE_CONFIG_DIR not used")
	}
	trust.Prepare(work, []string{"claude"}, nil)
	if readProjects(t, filepath.Join(fromServer, ".claude.json"))[canonical(t, work)]["hasTrustDialogAccepted"] != true {
		t.Fatal("server CLAUDE_CONFIG_DIR not used")
	}
}

func TestPrepareNeverCreatesOrBreaksConfig(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "p")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	trust := newTrust(home, nil)
	trust.Prepare(work, []string{"claude"}, nil)
	if _, err := os.Stat(filepath.Join(home, ".claude.json")); !os.IsNotExist(err) {
		t.Fatalf("config created: %v", err)
	}
	config := filepath.Join(home, ".claude.json")
	for _, bad := range []string{"not json", `[]`, `{"projects": 3}`} {
		if err := os.WriteFile(config, []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		trust.Prepare(work, []string{"claude"}, nil)
		if data, _ := os.ReadFile(config); string(data) != bad {
			t.Fatalf("invalid config %q rewritten to %q", bad, data)
		}
	}
	entries, _ := os.ReadDir(home)
	for _, e := range entries {
		if e.Name() != "p" && e.Name() != ".claude.json" {
			t.Fatalf("temporary file left behind: %s", e.Name())
		}
	}
}

func TestPrepareRefusesBroadDirectories(t *testing.T) {
	parent := t.TempDir()
	home := filepath.Join(parent, "home")
	if err := os.Mkdir(home, 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(config, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	trust := newTrust(home, nil)
	for _, dir := range []string{home, parent, "/"} {
		trust.Prepare(dir, []string{"claude"}, nil)
	}
	if data, _ := os.ReadFile(config); string(data) != "{}" {
		t.Fatalf("broad directory trusted: %s", data)
	}
}

func TestPrepareIsIdempotentAndLeavesTrustedFileUntouched(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "p")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(home, ".claude.json")
	seed := `{"projects":{"` + canonical(t, work) + `":{"hasTrustDialogAccepted":true}}}`
	if err := os.WriteFile(config, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	newTrust(home, nil).Prepare(work, []string{"claude"}, nil)
	if data, _ := os.ReadFile(config); string(data) != seed {
		t.Fatalf("file rewritten: %s", data)
	}
}

func TestPrepareCodexAppendsProjectTableKeepingComments(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, `p "q"`)
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	codexDir := filepath.Join(home, ".codex")
	if err := os.Mkdir(codexDir, 0o700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(codexDir, "config.toml")
	seed := "# my settings\nmodel = \"gpt\"\n\n[projects.\"/other\"]\ntrust_level = \"untrusted\"\n"
	if err := os.WriteFile(config, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	trust := newTrust(home, nil)
	trust.Prepare(work, []string{"bash", "-ic", `exec -- "$@"`, "webmux", "codex", "--dangerously-bypass-approvals-and-sandbox"}, nil)

	data, _ := os.ReadFile(config)
	var doc struct {
		Model    string `toml:"model"`
		Projects map[string]struct {
			TrustLevel string `toml:"trust_level"`
		} `toml:"projects"`
	}
	if _, err := toml.Decode(string(data), &doc); err != nil {
		t.Fatalf("invalid TOML %q: %v", data, err)
	}
	if doc.Projects[canonical(t, work)].TrustLevel != "trusted" || doc.Projects["/other"].TrustLevel != "untrusted" || doc.Model != "gpt" {
		t.Fatalf("config = %s", data)
	}
	if !strings.HasPrefix(string(data), seed) {
		t.Fatalf("original text changed: %s", data)
	}
	trust.Prepare(work, []string{"codex"}, nil)
	if again, _ := os.ReadFile(config); string(again) != string(data) {
		t.Fatalf("second launch rewrote the file: %s", again)
	}
}

func TestPrepareCodexRespectsExplicitDecisionAndCodexHome(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "p")
	custom := filepath.Join(home, "custom")
	for _, dir := range []string{work, custom} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join(custom, "config.toml")
	seed := "[projects.\"" + canonical(t, work) + "\"]\ntrust_level = \"untrusted\"\n"
	if err := os.WriteFile(config, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	newTrust(home, nil).Prepare(work, []string{"codex"}, map[string]string{"CODEX_HOME": custom})
	if data, _ := os.ReadFile(config); string(data) != seed {
		t.Fatalf("explicit untrusted decision overwritten: %s", data)
	}
}

func TestPrepareCodexCreatesConfigOnlyWhenCodexDirExists(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "p")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	trust := newTrust(home, nil)
	trust.Prepare(work, []string{"codex"}, nil)
	if _, err := os.Stat(filepath.Join(home, ".codex")); !os.IsNotExist(err) {
		t.Fatalf("codex directory created: %v", err)
	}
	if err := os.Mkdir(filepath.Join(home, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	trust.Prepare(work, []string{"codex"}, nil)
	data, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil || !strings.Contains(string(data), `trust_level = "trusted"`) {
		t.Fatalf("config = %q, %v", data, err)
	}
	if info, _ := os.Stat(filepath.Join(home, ".codex", "config.toml")); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode())
	}
}

func TestPrepareCopilotAppendsTrustedFolder(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "p")
	dir := filepath.Join(home, ".copilot")
	for _, d := range []string{work, dir} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join(dir, "config.json")
	if err := os.WriteFile(config, []byte(`{"model":"x","trustedFolders":["/old"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	trust := newTrust(home, nil)
	trust.Prepare(work, []string{"copilot", "--yolo"}, nil)
	trust.Prepare(work, []string{"copilot", "--yolo"}, nil)

	data, _ := os.ReadFile(config)
	var doc struct {
		Model   string   `json:"model"`
		Folders []string `json:"trustedFolders"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON %q: %v", data, err)
	}
	want := []string{"/old", canonical(t, work)}
	if doc.Model != "x" || len(doc.Folders) != 2 || doc.Folders[0] != want[0] || doc.Folders[1] != want[1] {
		t.Fatalf("config = %s", data)
	}
}

func TestPrepareCopilotLeavesCommentedConfigUntouched(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "p")
	dir := filepath.Join(home, ".copilot")
	for _, d := range []string{work, dir} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join(dir, "config.json")
	seed := "// settings\n{\"trustedFolders\":[]}\n"
	if err := os.WriteFile(config, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	newTrust(home, nil).Prepare(work, []string{"copilot"}, nil)
	if data, _ := os.ReadFile(config); string(data) != seed {
		t.Fatalf("config rewritten: %s", data)
	}
}

func TestPrepareCopilotKeepsLegacyKeyOnlyWhenItIsTheOnlyOne(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "p")
	dir := filepath.Join(home, ".copilot")
	for _, d := range []string{work, dir} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join(dir, "config.json")
	if err := os.WriteFile(config, []byte(`{"trusted_folders":["/old"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	newTrust(home, nil).Prepare(work, []string{"copilot"}, nil)
	var doc map[string][]string
	data, _ := os.ReadFile(config)
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc["trusted_folders"]) != 2 || doc["trustedFolders"] != nil {
		t.Fatalf("config = %s", data)
	}
	if err := os.WriteFile(config, []byte(`{"trusted_folders":["/old"],"trustedFolders":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	newTrust(home, nil).Prepare(work, []string{"copilot"}, nil)
	doc = nil
	data, _ = os.ReadFile(config)
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc["trustedFolders"]) != 1 || len(doc["trusted_folders"]) != 1 {
		t.Fatalf("current key not preferred: %s", data)
	}
}

func TestPrepareAgyAppendsTrustedWorkspaceThroughShellWrapper(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "p")
	dir := filepath.Join(home, ".gemini", "antigravity-cli")
	for _, d := range []string{work, dir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(config, []byte(`{"model":"x","trustedWorkspaces":["/old"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	trust := newTrust(home, nil)
	argv := []string{"bash", "-ic", `exec -- "$@"`, "webmux", "agy", "--dangerously-skip-permissions"}
	trust.Prepare(work, argv, nil)
	trust.Prepare(work, argv, nil)

	data, _ := os.ReadFile(config)
	var doc struct {
		Model      string   `json:"model"`
		Workspaces []string `json:"trustedWorkspaces"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON %q: %v", data, err)
	}
	if doc.Model != "x" || len(doc.Workspaces) != 2 || doc.Workspaces[0] != "/old" || doc.Workspaces[1] != canonical(t, work) {
		t.Fatalf("config = %s", data)
	}
	if info, _ := os.Stat(config); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode())
	}
}

func TestPrepareAgyLeavesMissingConfigAloneAndAddsListToExistingOne(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "p")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	trust := newTrust(home, nil)
	trust.Prepare(work, []string{"agy"}, nil)
	if _, err := os.Stat(filepath.Join(home, ".gemini")); !os.IsNotExist(err) {
		t.Fatalf("agy config created: %v", err)
	}

	dir := filepath.Join(home, ".gemini", "antigravity-cli")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(config, []byte(`{"model":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	trust.Prepare(work, []string{"agy"}, nil)
	var doc map[string]any
	data, _ := os.ReadFile(config)
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if list, _ := doc["trustedWorkspaces"].([]any); len(list) != 1 || doc["model"] != "x" {
		t.Fatalf("config = %s", data)
	}
}

func TestPrepareQoderAndQoderCNAppendTrustDirectoriesKeepingOtherFields(t *testing.T) {
	cases := []struct {
		name   string
		dotDir string
		envVar string
		argvs  [][]string
	}{
		{
			name:   "qoder",
			dotDir: ".qoder",
			envVar: "QODER_CONFIG_DIR",
			argvs: [][]string{
				{"bash", "-ic", `exec -- "$@"`, "webmux", "qoder", "--dangerously-skip-permissions"},
				{"/opt/qoder/bin/qodercli"},
				{"/home/u/.qoder/bin/qodercli/qodercli-1.1.81"},
				{"qoder-2.0.0"},
			},
		},
		{
			name:   "qodercn",
			dotDir: ".qoder-cn",
			envVar: "QODERCN_CONFIG_DIR",
			argvs: [][]string{
				{"bash", "-ic", `exec -- "$@"`, "webmux", "qodercn", "--dangerously-skip-permissions"},
				{"qoderclicn"},
				{"qoder-cn"},
				{"/home/u/.qoder-cn/bin/qoderclicn/qoderclicn-1.1.84"},
			},
		},
	}
	for _, tc := range cases {
		for _, argv := range tc.argvs {
			home := t.TempDir()
			work := filepath.Join(home, "p")
			dir := filepath.Join(home, tc.dotDir)
			for _, d := range []string{work, dir} {
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			config := filepath.Join(dir, "settings.json")
			seed := `{"model":{"name":"auto"},"permissions":{"allow":["Read"],"trustDirectories":["/old"]}}`
			if err := os.WriteFile(config, []byte(seed), 0o600); err != nil {
				t.Fatal(err)
			}
			trust := newTrust(home, nil)
			trust.Prepare(work, argv, nil)
			first, err := os.ReadFile(config)
			if err != nil {
				t.Fatal(err)
			}
			trust.Prepare(work, argv, nil)
			second, _ := os.ReadFile(config)
			if string(second) != string(first) {
				t.Fatalf("%s (%v): not idempotent:\n%s", tc.name, argv, second)
			}

			var doc struct {
				Model       map[string]any `json:"model"`
				Permissions struct {
					Allow            []string `json:"allow"`
					TrustDirectories []string `json:"trustDirectories"`
				} `json:"permissions"`
			}
			if err := json.Unmarshal(first, &doc); err != nil {
				t.Fatalf("%s (%v): invalid JSON %q: %v", tc.name, argv, first, err)
			}
			want := canonical(t, work)
			if doc.Model["name"] != "auto" ||
				len(doc.Permissions.Allow) != 1 || doc.Permissions.Allow[0] != "Read" ||
				len(doc.Permissions.TrustDirectories) != 2 ||
				doc.Permissions.TrustDirectories[0] != "/old" ||
				doc.Permissions.TrustDirectories[1] != want {
				t.Fatalf("%s (%v): config = %s", tc.name, argv, first)
			}
			if info, _ := os.Stat(config); info.Mode().Perm() != 0o600 {
				t.Fatalf("%s (%v): mode = %v", tc.name, argv, info.Mode())
			}
		}

		// Missing file is not created; invalid JSON or non-object permissions are left untouched;
		// session and server environment variables override the default dotDir.
		home := t.TempDir()
		work := filepath.Join(home, "p")
		fromEnv := filepath.Join(home, "envcfg")
		fromServer := filepath.Join(home, "servercfg")
		for _, d := range []string{work, fromEnv, fromServer} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		trust := newTrust(home, map[string]string{tc.envVar: fromServer})
		trust.Prepare(work, tc.argvs[0], map[string]string{tc.envVar: fromEnv})
		if _, err := os.Stat(filepath.Join(fromEnv, "settings.json")); !os.IsNotExist(err) {
			t.Fatalf("%s: missing config created: %v", tc.name, err)
		}
		for _, cfgDir := range []string{fromEnv, fromServer} {
			if err := os.WriteFile(filepath.Join(cfgDir, "settings.json"), []byte(`{"model":{"name":"auto"}}`), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		trust.Prepare(work, tc.argvs[0], map[string]string{tc.envVar: fromEnv})
		trust.Prepare(work, tc.argvs[0], nil)
		for _, cfgDir := range []string{fromEnv, fromServer} {
			data, err := os.ReadFile(filepath.Join(cfgDir, "settings.json"))
			if err != nil {
				t.Fatal(err)
			}
			var doc struct {
				Permissions struct {
					TrustDirectories []string `json:"trustDirectories"`
				} `json:"permissions"`
			}
			if err := json.Unmarshal(data, &doc); err != nil || len(doc.Permissions.TrustDirectories) != 1 || doc.Permissions.TrustDirectories[0] != canonical(t, work) {
				t.Fatalf("%s (%s): config = %s, %v", tc.name, cfgDir, data, err)
			}
		}

		badFile := filepath.Join(fromServer, "settings.json")
		for _, bad := range []string{"// comment\n{}\n", `{"permissions": 3}`, `{"permissions": {"trustDirectories": "nope"}}`} {
			if err := os.WriteFile(badFile, []byte(bad), 0o600); err != nil {
				t.Fatal(err)
			}
			trust.Prepare(work, tc.argvs[0], nil)
			if got, _ := os.ReadFile(badFile); string(got) != bad {
				t.Fatalf("%s: bad config %q rewritten to %q", tc.name, bad, got)
			}
		}
	}
}

func TestPrepareWritesThroughSymlinkedConfig(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "p")
	dotfiles := filepath.Join(home, "dotfiles")
	for _, d := range []string{work, dotfiles} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(dotfiles, "claude.json")
	if err := os.WriteFile(target, []byte("{}"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o640); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".claude.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	newTrust(home, nil).Prepare(work, []string{"claude"}, nil)

	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink replaced: %v, %v", info, err)
	}
	if readProjects(t, target)[canonical(t, work)]["hasTrustDialogAccepted"] != true {
		t.Fatal("target not updated")
	}
	if info, _ := os.Stat(target); info.Mode().Perm() != 0o640 {
		t.Fatalf("target mode = %v", info.Mode())
	}
	entries, _ := os.ReadDir(dotfiles)
	if len(entries) != 1 {
		t.Fatalf("temporary file left in %d entries", len(entries))
	}
}

func TestPrepareLeavesDanglingSymlinkAlone(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "p")
	codex := filepath.Join(home, ".codex")
	for _, d := range []string{work, codex} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(codex, "config.toml")
	if err := os.Symlink(filepath.Join(home, "missing.toml"), link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	newTrust(home, nil).Prepare(work, []string{"codex"}, nil)
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("dangling link replaced: %v, %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(home, "missing.toml")); !os.IsNotExist(err) {
		t.Fatalf("link target created: %v", err)
	}
}
