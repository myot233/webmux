package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/rainchan/webmux/internal/session"
)

type Config struct {
	Listen           string           `toml:"listen"`
	Token            string           `toml:"token"`
	ScrollbackBytes  int              `toml:"scrollback_bytes"`
	DefaultCwd       string           `toml:"default_cwd"`
	StateDir         string           `toml:"state_dir"`
	Shell            string           `toml:"shell"`
	IdleAfterSeconds int              `toml:"idle_after_seconds"`
	PromptPatterns   []string         `toml:"prompt_patterns"`
	AllowedOrigins   []string         `toml:"allowed_origins"`
	Presets          []session.Preset `toml:"presets"`
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "webmux", "config.toml")
}
func DefaultStateDir() string {
	if base := os.Getenv("XDG_STATE_HOME"); base != "" {
		return filepath.Join(base, "webmux")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "webmux")
}
func Load(path string, required bool) (Config, error) {
	c := Config{
		Listen:           "127.0.0.1:7681",
		ScrollbackBytes:  2097152,
		DefaultCwd:       "~",
		IdleAfterSeconds: 30,
		PromptPatterns: []string{
			`[$#%>❯›?:]\s*$`,
			`(?i)\(y/n\)|\[y/n\]|yes.*no|press enter`,
		},
		Presets: []session.Preset{
			{Name: "shell", Label: "Shell", Cmd: []string{}},
			{Name: "claude", Label: "Claude Code", Cmd: []string{"claude"}, AllowArgs: true},
			{Name: "codex", Label: "Codex CLI", Cmd: []string{"codex"}, AllowArgs: true},
			{Name: "omp", Label: "omp", Cmd: []string{"omp"}, AllowArgs: true},
		},
	}
	if _, err := toml.DecodeFile(path, &c); err != nil {
		if required || !errors.Is(err, os.ErrNotExist) {
			return c, err
		}
	}
	return c, nil
}
func (c *Config) Validate() error {
	if _, _, err := net.SplitHostPort(c.Listen); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	if c.ScrollbackBytes < 1 || c.IdleAfterSeconds < 2 {
		return errors.New("scrollback_bytes must be positive and idle_after_seconds >= 2")
	}
	cwd, err := session.ExpandPath(c.DefaultCwd)
	if err != nil {
		return err
	}
	c.DefaultCwd = cwd
	if c.StateDir == "" {
		c.StateDir = DefaultStateDir()
	}
	// Storage accessibility is a workspace capability, not a startup prerequisite.
	if expanded, expandErr := session.ExpandPath(c.StateDir); expandErr == nil {
		c.StateDir = expanded
	}
	if c.Shell == "" {
		c.Shell = os.Getenv("SHELL")
	}
	if c.Shell == "" {
		c.Shell = "/bin/bash"
	}
	seen := map[string]bool{}
	for i := range c.Presets {
		p := &c.Presets[i]
		if p.Name == "" || seen[p.Name] {
			return errors.New("preset names must be nonempty and unique")
		}
		if p.Name == "custom" {
			return errors.New("preset name custom is reserved for custom commands")
		}
		seen[p.Name] = true
		if p.Name == "shell" && len(p.Cmd) == 0 {
			p.Cmd = []string{c.Shell, "-l", "-i"}
		}
		if len(p.Cmd) == 0 || p.Cmd[0] == "" {
			return fmt.Errorf("preset %s has no command", p.Name)
		}
	}
	for _, pattern := range c.PromptPatterns {
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("prompt pattern: %w", err)
		}
	}
	return nil
}
func (c Config) SessionOptions() session.Options {
	return session.Options{
		ScrollbackBytes: c.ScrollbackBytes,
		DefaultCwd:      c.DefaultCwd,
		Presets:         c.Presets,
		IdleAfter:       time.Duration(c.IdleAfterSeconds) * time.Second,
		PromptPatterns:  c.PromptPatterns,
	}
}
