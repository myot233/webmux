package launch

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/rainchan/webmux/internal/agentprofile/agents"
)

func TestPresetsFollowDefaultShell(t *testing.T) {
	tests := []struct {
		name      string
		shell     string
		wantLogin []string
		wantOMP   []string
	}{
		{
			name:      "bash",
			shell:     "/bin/bash",
			wantLogin: []string{"/bin/bash", "-l", "-i"},
			wantOMP:   []string{"/usr/bin/env", "bash", "-ic", `exec -- "$@"`, "webmux", "omp", "--auto-approve"},
		},
		{
			name:      "zsh",
			shell:     "/bin/zsh",
			wantLogin: []string{"/bin/zsh", "-l", "-i"},
			wantOMP:   []string{"zsh", "-ic", `exec -- "$@"`, "webmux", "omp", "--auto-approve"},
		},
		{
			name:      "unset falls back to bash",
			shell:     "",
			wantLogin: []string{"/bin/sh", "-l", "-i"},
			wantOMP:   []string{"/usr/bin/env", "bash", "-ic", `exec -- "$@"`, "webmux", "omp", "--auto-approve"},
		},
		{
			name:      "other shell runs directly",
			shell:     "/usr/bin/fish",
			wantLogin: []string{"/usr/bin/fish", "-l", "-i"},
			wantOMP:   []string{"omp", "--auto-approve"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			presets := Presets(tt.shell)
			if len(presets) != 14 || presets[0].Name != "Shell" || !slices.Equal(presets[0].Cmd, tt.wantLogin) {
				t.Fatalf("login shell preset = %+v", presets[:min(1, len(presets))])
			}
			for _, preset := range presets {
				if preset.Name != "omp" {
					continue
				}
				if !slices.Equal(preset.Cmd, tt.wantOMP) {
					t.Fatalf("omp argv = %q, want %q", preset.Cmd, tt.wantOMP)
				}
				return
			}
			t.Fatal("omp preset missing")
		})
	}
}

func TestPresetsBeforeSeedPreferShellAwareList(t *testing.T) {
	s := open(t, t.TempDir())
	if _, err := s.Presets(); err != nil {
		t.Fatal(err)
	}
	if err := s.Seed(Presets("/bin/zsh")); err != nil {
		t.Fatal(err)
	}
	presets, err := s.Presets()
	if err != nil || len(presets) != 14 || !slices.Equal(presets[0].Cmd, []string{"/bin/zsh", "-l", "-i"}) {
		t.Fatalf("presets after seed = %+v, %v", presets, err)
	}
}

func TestRestorePresets(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)
	if err := s.Seed(Presets("/bin/zsh")); err != nil {
		t.Fatal(err)
	}
	presets, err := s.Presets()
	if err != nil || len(presets) != 14 || !slices.Equal(presets[0].Cmd, []string{"/bin/zsh", "-l", "-i"}) {
		t.Fatalf("presets = %+v, %v", presets, err)
	}
	// Editing a preset keeps its name; deleting others leaves them restorable.
	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	var claudeID string
	for _, c := range list {
		if c.Name == "Claude Code" {
			claudeID = c.ID
			if _, err := s.Update(c.ID, Input{Name: "Claude Code", Icon: "claudecode", Cmd: []string{"claude", "--resume"}}); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := s.Delete(c.ID); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()

	// Reopening an existing file and calling Seed keeps user edits while
	// remembering the shell's presets for later restoration.
	s = open(t, dir)
	if err := s.Seed(Presets("/bin/zsh")); err != nil {
		t.Fatal(err)
	}
	restored, err := s.RestorePresets("omp")
	if err != nil || len(restored) != 2 || restored[0].Name != "Claude Code" || restored[1].Name != "omp" {
		t.Fatalf("restore omp = %+v, %v", restored, err)
	}
	ompCfg, err := s.Get(restored[1].ID)
	if err != nil || ompCfg.Env["PI_FORCE_IMAGE_PROTOCOL"] != "kitty" || !ompCfg.Hybrid {
		t.Fatalf("restored omp config = %+v, %v", ompCfg, err)
	}
	if _, err := s.RestorePresets("Claude Code"); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("existing preset err = %v, want ErrDuplicate", err)
	}
	if _, err := s.RestorePresets("Unknown Preset"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown preset err = %v, want ErrNotFound", err)
	}

	all, err := s.RestorePresets("")
	if err != nil || len(all) != 14 {
		t.Fatalf("restore all = %+v, %v", all, err)
	}
	claudeCfg, err := s.Get(claudeID)
	if err != nil || !reflect.DeepEqual(claudeCfg.Cmd, []string{"claude", "--resume"}) {
		t.Fatalf("user-edited preset overwritten: %+v, %v", claudeCfg, err)
	}
	if !reflect.DeepEqual(all[2].Cmd, []string{"/bin/zsh", "-l", "-i"}) {
		t.Fatalf("restored Shell cmd = %q", all[2].Cmd)
	}
	// Restoring all when every preset already exists is a no-op.
	again, err := s.RestorePresets("")
	if err != nil || !reflect.DeepEqual(again, all) {
		t.Fatalf("restore all again = %+v, %v", again, err)
	}
}

func TestRestorePresetsRespectsMaxConfigs(t *testing.T) {
	s := open(t, t.TempDir())
	for i := range MaxConfigs - 1 {
		mustCreate(t, s, Input{Name: fmt.Sprintf("custom-%d", i), Cmd: []string{"sh"}})
	}
	if _, err := s.RestorePresets(""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("restore all over limit = %v, want ErrInvalid", err)
	}
	if _, err := s.RestorePresets("Shell"); err != nil {
		t.Fatalf("restore one at limit-1 = %v", err)
	}
	if _, err := s.RestorePresets("omp"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("restore one at limit = %v, want ErrInvalid", err)
	}
}

// The browser shows a hybrid session's agent with the profile's Icon from the
// launch-config icon catalog, so every agent needs a title and a brand glyph
// in that catalog, the same one its hybrid preset carries.
func TestAgentIconsComeFromLaunchCatalog(t *testing.T) {
	registry := agents.Default()
	for _, a := range registry.Agents() {
		p := a.Profile()
		if p.Title == "" || p.Icon == "" || p.Icon == "terminal" {
			t.Errorf("agent %q: title %q, icon %q; want a title and a brand glyph", p.Name, p.Title, p.Icon)
			continue
		}
		if _, err := normalize(Input{Name: p.Title, Icon: p.Icon, Cmd: []string{p.Name}}); err != nil {
			t.Errorf("agent %q icon %q is not a launch-config icon: %v", p.Name, p.Icon, err)
		}
	}
	for _, preset := range Presets("/bin/bash") {
		if !preset.Hybrid {
			continue
		}
		support := registry.Check(preset.Cmd)
		a, ok := registry.Lookup(support.Agent)
		if !ok {
			t.Errorf("hybrid preset %q runs no registered agent: %+v", preset.Name, support)
			continue
		}
		if got := a.Profile().Icon; got != preset.Icon {
			t.Errorf("agent %q icon %q, its preset %q has %q", support.Agent, got, preset.Name, preset.Icon)
		}
	}
}
