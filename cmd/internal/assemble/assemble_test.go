package assemble

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/workspace"
)

func TestAttentionSignalOptions(t *testing.T) {
	dir := t.TempDir()
	for _, row := range []struct {
		name, body                                 string
		disableProtocol, disableBel, disableStatus bool
	}{
		{"defaults", "", false, false, false},
		{"legacy file", "idle_after_seconds = 30\n", false, false, false},
		{"disabled", "agent_protocol = false\nbell_attention = false\nprogram_status = false\n", true, true, true},
		{"protocol only off", "agent_protocol = false\n", true, false, false},
		{"program status only off", "program_status = false\n", false, false, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			path := filepath.Join(dir, row.name+".toml")
			if err := os.WriteFile(path, []byte(row.body), 0600); err != nil {
				t.Fatal(err)
			}
			c, err := config.Load(path, true)
			if err != nil {
				t.Fatal(err)
			}
			if err := c.Validate(); err != nil {
				t.Fatal(err)
			}
			o := SessionOptions(c)
			if o.DisableAgentProtocol != row.disableProtocol || o.DisableBellAttention != row.disableBel || o.DisableProgramStatus != row.disableStatus {
				t.Fatalf("options = %+v", o)
			}
		})
	}
}

func TestSessionOptionsExcludeListenPort(t *testing.T) {
	for _, row := range []struct {
		listen string
		want   []int
	}{
		{"127.0.0.1:7681", []int{7681}},
		{"[::1]:8443", []int{8443}},
		{":9000", []int{9000}},
		{"127.0.0.1:0", nil},
		{"no-port", nil},
	} {
		if got := SessionOptions(config.Config{Listen: row.listen}).ExcludePorts; !slices.Equal(got, row.want) {
			t.Errorf("listen %q: ExcludePorts = %v, want %v", row.listen, got, row.want)
		}
	}
}

func TestSavedLaunchConfigLoadsBashrcAndForwardsArguments(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WEBMUX_TEST_TOKEN", "")
	bashrc := "export WEBMUX_TEST_TOKEN=bashrc\nexport PATH='" + strings.ReplaceAll(bin, "'", "'\\''") + "':\"$PATH\"\n"
	if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte(bashrc), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(filepath.Join(home, "config.toml"), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	m := session.NewManager(SessionOptions(c))
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	}()
	launches := launch.Open(t.TempDir())
	defer launches.Close()
	service := workspace.New(t.TempDir(), m, launches, home)
	defer service.Close()
	agent := "opencode"
	script := "#!/bin/sh\nprintf 'agent=%s token=%s\\n' \"" + agent + "\" \"$WEBMUX_TEST_TOKEN\"\nfor arg in \"$@\"; do printf 'arg=<%s>\\n' \"$arg\"; done\n"
	if err := os.WriteFile(filepath.Join(bin, agent), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	saved, err := launches.Create(launch.Input{Name: "Open Code", Cmd: []string{"bash", "-ic", `exec -- "$@"`, "webmux", "opencode", "--resume", "two words", "", "$HOME"}})
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.Create(context.Background(), workspace.CreateInput{Cwd: home, ConfigID: &saved.ID})
	if err != nil {
		t.Fatal(err)
	}
	s := created.Session
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatalf("%s saved command did not exit", agent)
	}
	out := string(s.Buffer())
	if !strings.Contains(out, "agent="+agent+" token=bashrc") {
		t.Fatalf("~/.bashrc environment missing: %q", out)
	}
	if !strings.Contains(out, "arg=<--resume>") || !strings.Contains(out, "arg=<two words>") || !strings.Contains(out, "arg=<>") || !strings.Contains(out, "arg=<$HOME>") {
		t.Fatalf("saved command arguments missing or split: %q", out)
	}
}

func TestSessionOptionsSubmitDelay(t *testing.T) {
	delay := SessionOptions(config.Config{}).SubmitDelay
	for _, tc := range []struct {
		argv []string
		want time.Duration
	}{
		{argv: []string{"kimi"}, want: 200 * time.Millisecond},
		{argv: []string{"kimi", "--environment", "remote"}, want: 200 * time.Millisecond},
		{argv: []string{"bash", "-ic", `exec -- "$@"`, "webmux", "kimi", "--auto"}, want: 200 * time.Millisecond},
		{argv: []string{"opencode"}, want: 0},
		{argv: []string{"sh", "-c", "kimi"}, want: 0},
	} {
		if got := delay(tc.argv); got != tc.want {
			t.Errorf("SubmitDelay(%q) = %v, want %v", tc.argv, got, tc.want)
		}
	}
}

func TestAgentPretrustOption(t *testing.T) {
	dir := t.TempDir()
	for _, row := range []struct {
		body string
		want bool
	}{{"", true}, {"agent_pretrust = false\n", false}} {
		path := filepath.Join(dir, "c.toml")
		if err := os.WriteFile(path, []byte(row.body), 0600); err != nil {
			t.Fatal(err)
		}
		c, err := config.Load(path, true)
		if err != nil {
			t.Fatal(err)
		}
		if got := SessionOptions(c).PreLaunch != nil; got != row.want {
			t.Fatalf("%q: PreLaunch set = %v, want %v", row.body, got, row.want)
		}
	}
}
