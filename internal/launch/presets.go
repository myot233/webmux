package launch

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/rainchan/webmux/internal/agentprofile"
)

// Preset describes a built-in launch config template that can be restored.
type Preset struct {
	Name     string   `json:"name"`
	Icon     string   `json:"icon,omitempty"`
	Cmd      []string `json:"cmd"`
	EnvNames []string `json:"envNames"`
	Hybrid   bool     `json:"hybrid"`
}

func (in Input) preset() Preset {
	names := slices.Sorted(maps.Keys(in.Env))
	if names == nil {
		names = []string{}
	}
	return Preset{
		Name:     in.Name,
		Icon:     in.Icon,
		Cmd:      slices.Clone(in.Cmd),
		EnvNames: names,
		Hybrid:   in.Hybrid,
	}
}

// Presets returns the launch configs seeded into a new installation: a login
// shell and common CLI agents. Agents load the startup file of the service's
// default shell; permission flags are explicit parts of the editable
// commands. Claude Code, Codex CLI, Antigravity CLI, OpenCode, omp, Qoder,
// QoderCN, Kimi Code CLI and Devin CLI start as hybrid sessions. They are ordinary configs the user may edit
// or delete. shell is the login shell path; empty means /bin/sh.
func Presets(shell string) []Input {
	login := shell
	if login == "" {
		login = "/bin/sh"
	}
	startup := agentprofile.DefaultShell(shell)
	command := func(program string, args ...string) []string {
		return agentprofile.StartupArgv(startup, program, args...)
	}
	return []Input{
		{Name: "Shell", Icon: "terminal", Cmd: []string{login, "-l", "-i"}},
		{Name: "Claude Code", Icon: "claudecode", Cmd: command("claude", "--dangerously-skip-permissions"), Hybrid: true},
		{Name: "Codex CLI", Icon: "codex", Cmd: command("codex", "--dangerously-bypass-approvals-and-sandbox"), Hybrid: true},
		{Name: "Antigravity CLI", Icon: "antigravity", Cmd: command("agy", "--dangerously-skip-permissions"), Hybrid: true},
		{Name: "Cursor CLI", Icon: "cursor", Cmd: command("agent")},
		{Name: "GitHub Copilot CLI", Icon: "githubcopilot", Cmd: command("copilot")},
		{Name: "Open Code", Icon: "opencode", Cmd: command("opencode", "--auto"), Hybrid: true},
		{Name: "Qwen Code", Icon: "qwen", Cmd: command("qwen")},
		{Name: "Kimi Code CLI", Icon: "kimi", Cmd: command("kimi", "--auto"), Hybrid: true},
		{Name: "CodeBuddy Code", Icon: "codebuddy", Cmd: command("codebuddy")},
		{Name: "omp", Icon: "pi", Cmd: command("omp", "--auto-approve"), Env: map[string]string{"PI_FORCE_IMAGE_PROTOCOL": "kitty"}, Hybrid: true},
		{Name: "Qoder", Icon: "qoder", Cmd: command("qoder", "--dangerously-skip-permissions"), Hybrid: true},
		{Name: "QoderCN", Icon: "qoder", Cmd: command("qodercn", "--dangerously-skip-permissions"), Hybrid: true},
		{Name: "Devin CLI", Icon: "devin", Cmd: command("devin", "--permission-mode", "dangerous"), Hybrid: true},
	}
}

func normalizePresets(ins []Input) ([]Input, error) {
	out := make([]Input, 0, len(ins))
	for _, in := range ins {
		in, err := normalize(in)
		if err != nil {
			return nil, err
		}
		if slices.ContainsFunc(out, func(p Input) bool { return p.Name == in.Name }) {
			return nil, ErrDuplicate
		}
		out = append(out, in)
	}
	return out, nil
}

func (s *Service) presetsLocked() ([]Input, error) {
	if s.presets != nil {
		return s.presets, nil
	}
	// Do not cache this fallback: Seed must always win so the shell-aware list
	// is used whenever it is available, regardless of call order.
	return normalizePresets(Presets(""))
}

// Presets returns the system launch presets configured on the service.
func (s *Service) Presets() ([]Preset, error) {
	if err := s.lockAvailable(); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	presets, err := s.presetsLocked()
	if err != nil {
		return nil, err
	}
	out := make([]Preset, 0, len(presets))
	for _, p := range presets {
		out = append(out, p.preset())
	}
	return out, nil
}

// RestorePresets adds missing system presets without overwriting existing
// configs of the same name. When name is non-empty, only that preset is
// restored (returning ErrNotFound for an unknown preset name or ErrDuplicate
// if a config with that name already exists). When name is empty, every
// missing preset is appended in preset order.
func (s *Service) RestorePresets(name string) ([]Summary, error) {
	name = strings.TrimSpace(name)
	if err := s.lockAvailable(); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	presets, err := s.presetsLocked()
	if err != nil {
		return nil, err
	}
	var toAdd []Input
	if name != "" {
		i := slices.IndexFunc(presets, func(p Input) bool { return p.Name == name })
		if i < 0 {
			return nil, ErrNotFound
		}
		if s.nameTakenLocked(name, "") {
			return nil, ErrDuplicate
		}
		toAdd = []Input{presets[i]}
	} else {
		for _, p := range presets {
			if !s.nameTakenLocked(p.Name, "") {
				toAdd = append(toAdd, p)
			}
		}
	}
	if len(toAdd) == 0 {
		return s.summariesLocked(), nil
	}
	if len(s.configs)+len(toAdd) > MaxConfigs {
		return nil, invalidf("最多保存 %d 条启动配置", MaxConfigs)
	}
	now := time.Now().UTC()
	next := slices.Clone(s.configs)
	for _, in := range toAdd {
		id, err := newID(func(id string) bool {
			return slices.ContainsFunc(next, func(c Config) bool { return c.ID == id })
		})
		if err != nil {
			return nil, err
		}
		c := Config{ID: id, Name: in.Name, Icon: in.Icon, Cmd: in.Cmd, Env: in.Env, Preview: in.Preview, Attention: in.Attention, Hybrid: in.Hybrid, CreatedAt: now, UpdatedAt: now}
		next = append(next, c.clone())
	}
	if err := s.saveLocked(next); err != nil {
		return nil, err
	}
	s.configs = next
	return s.summariesLocked(), nil
}
