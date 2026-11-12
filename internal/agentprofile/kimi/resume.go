package kimi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	ap "github.com/rainchan/webmux/internal/agentprofile"
)

const maxSessionWireLine = 32 << 20

var _ ap.TUIResumePreparer = adapter{}

// PrepareTUIResume changes the persisted ACP environment binding to local.
// Kimi 2.5.2 otherwise restores only the ACP-facing tool registry in the TUI.
func (adapter) PrepareTUIResume(sessionID string, getenv func(string) string, userHome string) (bool, error) {
	if sessionID == "" || sessionID != filepath.Base(sessionID) {
		return false, fmt.Errorf("invalid Kimi session ID %q", sessionID)
	}
	home := ""
	if getenv != nil {
		home = getenv("KIMI_CODE_HOME")
	}
	if home == "" {
		if userHome == "" {
			return false, errors.New("missing Kimi home directory")
		}
		home = filepath.Join(userHome, ".kimi-code")
	}
	wire, err := sessionWire(home, sessionID)
	if err != nil || wire == "" {
		return false, err
	}
	changed, err := localizeBinding(wire, sessionID)
	if err != nil {
		return false, fmt.Errorf("update Kimi session %s: %w", sessionID, err)
	}
	return changed, nil
}

func sessionWire(home, sessionID string) (string, error) {
	entries, err := os.ReadDir(filepath.Join(home, "sessions"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read Kimi session store: %w", err)
	}
	var matches []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(home, "sessions", entry.Name(), sessionID, "agents", "main", "wire.jsonl")
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 0 {
		return "", nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("kimi session %s has %d event logs, want one", sessionID, len(matches))
	}
	return matches[0], nil
}

func localizeBinding(wire, sessionID string) (bool, error) {
	info, err := os.Stat(wire)
	if err != nil {
		return false, err
	}
	input, err := os.Open(wire)
	if err != nil {
		return false, err
	}
	defer func() { _ = input.Close() }()
	temporary, err := os.CreateTemp(filepath.Dir(wire), ".wire-local-binding-*")
	if err != nil {
		return false, err
	}
	temporaryName := temporary.Name()
	published := false
	defer func() {
		if !published {
			_ = temporary.Close()
			_ = os.Remove(temporaryName)
		}
	}()
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64<<10), maxSessionWireLine)
	writer := bufio.NewWriter(temporary)
	changed := false
	for scanner.Scan() {
		line := scanner.Bytes()
		var event struct {
			Type          string `json:"type"`
			EnvironmentID string `json:"environmentId"`
		}
		if err := json.Unmarshal(line, &event); err != nil {
			return false, fmt.Errorf("decode event log: %w", err)
		}
		if event.Type == "environment.set_binding" && event.EnvironmentID == "acp:"+sessionID {
			line, err = encodeLocalBinding(line)
			if err != nil {
				return false, err
			}
			changed = true
		}
		if _, err := writer.Write(line); err != nil {
			return false, err
		}
		if err := writer.WriteByte('\n'); err != nil {
			return false, err
		}
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("read event log: %w", err)
	}
	if err := writer.Flush(); err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		return false, err
	}
	if err := temporary.Sync(); err != nil {
		return false, err
	}
	if err := temporary.Close(); err != nil {
		return false, err
	}
	if err := backupWire(wire, info.Mode().Perm()); err != nil {
		return false, err
	}
	if err := os.Rename(temporaryName, wire); err != nil {
		return false, err
	}
	published = true
	return true, nil
}

func encodeLocalBinding(line []byte) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil {
		return nil, fmt.Errorf("decode environment binding: %w", err)
	}
	fields["environmentId"] = json.RawMessage(`"local"`)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(fields); err != nil {
		return nil, fmt.Errorf("encode environment binding: %w", err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func backupWire(wire string, mode os.FileMode) error {
	backup := wire + ".before-local-binding"
	if _, err := os.Lstat(backup); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	input, err := os.Open(wire)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	temporary, err := os.CreateTemp(filepath.Dir(wire), ".wire-backup-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
	}()
	if _, err := io.Copy(temporary, input); err != nil {
		return err
	}
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Link(temporaryName, backup); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return nil
}
