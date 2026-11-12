package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/session"
)

func configService(t *testing.T, launches *launch.Service) (*Service, string) {
	t.Helper()
	cwd := t.TempDir()
	m := session.NewManager(session.Options{DefaultCwd: cwd})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})
	s := New(t.TempDir(), m, launches, cwd)
	t.Cleanup(s.Close)
	return s, cwd
}

func TestConfigIDRulesRunBeforeDirectoryChecks(t *testing.T) {
	launches := launch.Open(t.TempDir())
	t.Cleanup(launches.Close)
	s, cwd := configService(t, launches)
	saved, err := launches.Create(launch.Input{Name: "saved", Cmd: []string{"/bin/sh", "-c", "printf SAVED"}, Env: map[string]string{"SAVED": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	empty, unknown, id := "", "zzzzzzzz", saved.ID
	// A missing cwd proves that each rejection happens before any
	// filesystem or Git work.
	missing := filepath.Join(cwd, "missing")
	for _, row := range []struct {
		name string
		in   CreateInput
		kind error
		text string
	}{
		{"empty", CreateInput{Cwd: missing, ConfigID: &empty}, ErrInvalid, "configId must not be empty"},
		{"with cmd", CreateInput{Cwd: missing, ConfigID: &id, Cmd: []string{"x"}}, ErrInvalid, "configId cannot be combined with cmd or env"},
		{"with env", CreateInput{Cwd: missing, ConfigID: &id, Env: map[string]string{"A": "1"}}, ErrInvalid, "configId cannot be combined with cmd or env"},
		{"unknown", CreateInput{Cwd: missing, ConfigID: &unknown}, launch.ErrNotFound, launch.ErrNotFound.Error()},
		{"forged env", CreateInput{Cwd: missing, Cmd: []string{"/bin/sh"}, Env: map[string]string{"WEBMUX_SESSION_ID": "x"}}, ErrInvalid, ""},
	} {
		_, err := s.CreateSession(context.Background(), row.in)
		if !errors.Is(err, row.kind) || (row.text != "" && err.Error() != row.text) {
			t.Errorf("%s: %v, want %v %q", row.name, err, row.kind, row.text)
		}
	}
	item, err := s.CreateSession(context.Background(), CreateInput{ConfigID: &id})
	if err != nil {
		t.Fatal(err)
	}
	info := item.Info()
	if !reflect.DeepEqual(info.Cmd, saved.Cmd) || info.Cwd != cwd || info.ConfigID != saved.ID {
		t.Fatalf("session from config: %+v", info)
	}
	direct, err := s.CreateSession(context.Background(), CreateInput{Cwd: cwd, Cmd: []string{"/bin/sh", "-c", "printf DIRECT"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := direct.Info().ConfigID; got != "" {
		t.Fatalf("explicit command recorded config %q", got)
	}
}

func TestConfigIDWithoutLaunchStoreIsUnavailable(t *testing.T) {
	s, _ := configService(t, nil)
	id := "abcdefgh"
	if _, err := s.CreateSession(context.Background(), CreateInput{ConfigID: &id}); !errors.Is(err, launch.ErrUnavailable) {
		t.Fatalf("nil store: %v", err)
	}
}

func TestConfigIDPassesAttentionSettingsToSession(t *testing.T) {
	launches := launch.Open(t.TempDir())
	t.Cleanup(launches.Close)
	s, _ := configService(t, launches)
	saved, err := launches.Create(launch.Input{
		Name: "quiet",
		Cmd:  []string{"/bin/sh", "-c", "printf '\\a'; sleep 2"},
		Attention: &launch.AttentionSettings{
			Mode:          launch.AttentionModeUrgent,
			ProgramStatus: true,
			AgentProtocol: true,
			Notify:        true,
			Bell:          true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := saved.ID
	item, err := s.CreateSession(context.Background(), CreateInput{ConfigID: &id})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if info := item.Info(); info.Attention != nil {
		t.Fatalf("urgent launch config did not suppress terminal bell: %+v", info.Attention)
	}
}
