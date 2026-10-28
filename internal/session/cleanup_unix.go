//go:build linux || darwin

package session

import (
	"syscall"
	"time"
)

// Find every session process by terminal session, process tree, and the
// inherited environment marker, so separate job-control groups are covered too.
// An empty id skips the marker: a shell's sidecars share the marker with its
// PTY run, so stopping a sidecar must follow only its own process tree. A
// run's cleanup does match the marker and so also ends the shell's sidecars.
func cleanupProcesses(pid int, id string) {
	table := processes()
	reapAdopted(table)
	targets := make(map[int]uint64)
	var marker []byte
	if id != "" {
		marker = []byte("WEBMUX_SESSION_ID=" + id)
	}
	for child, state := range table {
		owned := state.sid == pid
		if !owned && marker != nil && state.state != 'Z' {
			owned = sessionMarked(child, marker)
		}
		if owned {
			targets[child] = state.start
		}
	}
	signalTargets(targets, table, syscall.SIGHUP)
	start := time.Now()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		table = processes()
		reapAdopted(table)
		// Shell job-control groups share a session ID; include new descendants too.
		for child, state := range table {
			if state.sid == pid {
				targets[child] = state.start
			}
		}
		for changed := true; changed; {
			changed = false
			for child, state := range table {
				if _, known := targets[child]; known {
					continue
				}
				if parentStart, owned := targets[state.ppid]; owned && table[state.ppid].start == parentStart {
					targets[child] = state.start
					changed = true
				}
			}
		}
		for child, born := range targets {
			state, exists := table[child]
			if !exists || state.start != born {
				delete(targets, child)
				continue
			}
		}
		if len(targets) == 0 {
			return
		}
		if time.Since(start) >= 5*time.Second {
			signalTargets(targets, table, syscall.SIGKILL)
		}
		if time.Since(start) >= 7*time.Second {
			return
		}
		<-ticker.C
	}
}
func signalTargets(targets map[int]uint64, table processTable, signal syscall.Signal) {
	for pid, born := range targets {
		if state, exists := table[pid]; exists && state.start == born && state.state != 'Z' {
			_ = syscall.Kill(pid, signal)
		}
	}
}
func forceSession(pid int, _ string) {
	table := processes()
	targets := make(map[int]uint64)
	for child, state := range table {
		if state.sid == pid {
			targets[child] = state.start
		}
	}
	signalTargets(targets, table, syscall.SIGKILL)
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
