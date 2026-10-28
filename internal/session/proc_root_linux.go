//go:build linux

package session

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// rootProcess returns the executable base name and working directory of pid.
// Either is empty when the process is gone or not readable by this user.
// The kernel reports the directory with symbolic links resolved.
func rootProcess(pid int) (exe, cwd string) {
	dir := "/proc/" + strconv.Itoa(pid)
	if link, err := os.Readlink(dir + "/exe"); err == nil {
		exe = filepath.Base(strings.TrimSuffix(link, " (deleted)"))
	}
	if link, err := os.Readlink(dir + "/cwd"); err == nil {
		cwd = strings.TrimSuffix(link, " (deleted)")
	}
	return exe, cwd
}
