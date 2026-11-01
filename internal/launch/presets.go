package launch

// Presets returns the launch configs seeded into a new installation: a login
// shell and common CLI agents. Agents load ~/.bashrc; permission flags are
// explicit parts of the editable commands. OpenCode and omp start as hybrid
// sessions. They are ordinary configs the user may edit or delete.
// shell is the login shell path; empty means /bin/sh.
func Presets(shell string) []Input {
	if shell == "" {
		shell = "/bin/sh"
	}
	return []Input{
		{Name: "Shell", Icon: "terminal", Cmd: []string{shell, "-l", "-i"}},
		{Name: "Claude Code", Icon: "claudecode", Cmd: bashrcCommand("claude", "--dangerously-skip-permissions")},
		{Name: "Codex CLI", Icon: "codex", Cmd: bashrcCommand("codex", "--dangerously-bypass-approvals-and-sandbox")},
		{Name: "Gemini CLI", Icon: "geminicli", Cmd: bashrcCommand("gemini", "--yolo")},
		{Name: "Cursor CLI", Icon: "cursor", Cmd: bashrcCommand("agent")},
		{Name: "GitHub Copilot CLI", Icon: "githubcopilot", Cmd: bashrcCommand("copilot")},
		{Name: "Open Code", Icon: "opencode", Cmd: bashrcCommand("opencode", "--auto"), Hybrid: true},
		{Name: "Qwen Code", Icon: "qwen", Cmd: bashrcCommand("qwen")},
		{Name: "Kimi Code CLI", Icon: "kimi", Cmd: bashrcCommand("kimi")},
		{Name: "CodeBuddy Code", Icon: "codebuddy", Cmd: bashrcCommand("codebuddy")},
		{Name: "omp", Icon: "pi", Cmd: bashrcCommand("omp", "--auto-approve"), Env: map[string]string{"PI_FORCE_IMAGE_PROTOCOL": "kitty"}, Hybrid: true},
	}
}

// bashrcCommand runs program with args through an interactive bash so it sees
// the PATH and environment from ~/.bashrc, which a service manager never
// loads. It is the argv the settings editor saves for "加载 ~/.bashrc"
// (BASHRC_PREFIX in web/src/launchConfigs.ts), so the editor shows the plain
// command checked.
func bashrcCommand(program string, args ...string) []string {
	return append([]string{"bash", "-ic", `exec -- "$@"`, "webmux", program}, args...)
}
