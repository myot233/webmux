package agenttrust

import (
	"os"
	"path/filepath"
)

// devinTrustKey is the array of trusted directories in Devin CLI's
// trusted_workspaces.json, which the CLI itself fills when the user answers
// its "Trust <dir>?" prompt.
const devinTrustKey = "trusted_paths"

// trustDevin appends dir to trusted_paths in
// $XDG_DATA_HOME/devin/cli/trusted_workspaces.json, defaulting to
// ~/.local/share/devin/cli/. Devin's TUI and ACP sides share this file. A
// missing file is created with the minimal record: the CLI's trust dialog
// does not gate any other first-run setup.
func (t *Trust) trustDevin(dir string, env map[string]string) error {
	base := t.env(env, "XDG_DATA_HOME")
	if base == "" {
		home, err := t.home()
		if err != nil {
			return err
		}
		base = filepath.Join(home, ".local", "share")
	}
	path := filepath.Join(base, "devin", "cli", "trusted_workspaces.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return editFile(path, true, func(data []byte) ([]byte, bool, error) {
		root, err := objectOf(data)
		if err != nil {
			return nil, false, err
		}
		changed, err := appendPath(root, devinTrustKey, dir)
		if err != nil || !changed {
			return nil, false, err
		}
		updated, err := indentJSON(root)
		return updated, err == nil, err
	})
}
