package p2psettings

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/rainchan/webmux/internal/config"
)

const fileName = "p2p-settings.json"

var tomlDefaults = config.DefaultP2PConfig()

func TestSaveOverridesTOMLAcrossRestartAndResetRestores(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir, tomlDefaults)
	if got, fromWeb := s.Effective(); !reflect.DeepEqual(got, tomlDefaults) || fromWeb {
		t.Fatalf("fresh store = %+v web=%v", got, fromWeb)
	}

	custom := config.P2PConfig{
		Enabled:     true,
		PortMapping: false,
		STUNServers: []string{"stun:stun.cloudflare.com:3478"},
	}
	if err := s.Save(custom); err != nil {
		t.Fatal(err)
	}
	s.Close()

	info, err := os.Stat(filepath.Join(dir, fileName))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("settings file %v %v", info, err)
	}

	inspected, fromWeb, err := Inspect(dir, tomlDefaults)
	if err != nil || !fromWeb || !reflect.DeepEqual(inspected, custom) {
		t.Fatalf("inspect = %+v web=%v err=%v", inspected, fromWeb, err)
	}

	s = Open(dir, tomlDefaults)
	defer s.Close()
	if got, fromWeb := s.Effective(); !reflect.DeepEqual(got, custom) || !fromWeb {
		t.Fatalf("reopened = %+v web=%v; want %+v", got, fromWeb, custom)
	}

	reset, err := s.Reset()
	if err != nil || !reflect.DeepEqual(reset, tomlDefaults) {
		t.Fatalf("reset = %+v %v", reset, err)
	}
	if _, err := os.Stat(filepath.Join(dir, fileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reset left override file: %v", err)
	}
}

func TestInvalidUpdatesAreRejectedWithoutChangingTheStore(t *testing.T) {
	s := Open(t.TempDir(), tomlDefaults)
	defer s.Close()

	bad := config.P2PConfig{
		Enabled:     true,
		PortMapping: true,
		STUNServers: []string{"https://not-stun.example.com"},
	}
	if err := s.Save(bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Save(%+v) = %v, want ErrInvalid", bad, err)
	}
	if got, fromWeb := s.Effective(); !reflect.DeepEqual(got, tomlDefaults) || fromWeb {
		t.Fatalf("rejected update changed store: %+v web=%v", got, fromWeb)
	}
}

func TestDamagedFileFailsClosedAndStaysUntouched(t *testing.T) {
	dir := t.TempDir()
	const damaged = `{"v":1,"enabled":true`
	path := filepath.Join(dir, fileName)
	if err := os.WriteFile(path, []byte(damaged), 0600); err != nil {
		t.Fatal(err)
	}

	s := Open(dir, tomlDefaults)
	defer s.Close()

	if got, _ := s.Effective(); got.Enabled || got.PortMapping {
		t.Fatalf("damaged store did not fail closed: %+v", got)
	}
	if err := s.Unavailable(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Unavailable() = %v", err)
	}
	if err := s.Save(tomlDefaults); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Save on damaged store = %v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != damaged {
		t.Fatalf("damaged file was modified: %s", b)
	}
}
