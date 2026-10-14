//go:build linux

package session

import (
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

var leaderMu sync.Mutex
var managedLeaders = make(map[int]struct{})

// Registration and reaping share a lock so a very short-lived command cannot
// become an unregistered zombie between Start returning and publishing its PID.
func startPTY(cmd *exec.Cmd, size *pty.Winsize) (*os.File, error) {
	leaderMu.Lock()
	defer leaderMu.Unlock()
	f, err := pty.StartWithSize(cmd, size)
	if err == nil {
		managedLeaders[cmd.Process.Pid] = struct{}{}
	}
	return f, err
}

func releaseLeader(pid int) {
	leaderMu.Lock()
	delete(managedLeaders, pid)
	leaderMu.Unlock()
}

// A zombie has no usable environ. Reap adopted children regardless of terminal
// SID or environment marker, but never take a managed leader's Wait result.
func reapAdopted(table processTable) {
	leaderMu.Lock()
	defer leaderMu.Unlock()
	for pid, state := range table {
		if state.ppid != os.Getpid() || state.state != 'Z' {
			continue
		}
		if _, managed := managedLeaders[pid]; managed {
			continue
		}
		var status syscall.WaitStatus
		_, _ = syscall.Wait4(pid, &status, syscall.WNOHANG, nil)
	}
}

func startChild(cmd *exec.Cmd) error {
	leaderMu.Lock()
	defer leaderMu.Unlock()
	if err := cmd.Start(); err != nil {
		return err
	}
	managedLeaders[cmd.Process.Pid] = struct{}{}
	return nil
}
