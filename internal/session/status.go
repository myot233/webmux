package session

import (
	"regexp"
	"strings"
	"time"
)

func classify(now, last time.Time, line string, exited, blocked bool, idleAfter time.Duration, patterns []*regexp.Regexp) Status {
	if exited {
		return StatusExited
	}
	age := now.Sub(last)
	if age < 2*time.Second {
		return StatusRunning
	}
	for _, pattern := range patterns {
		if pattern.MatchString(strings.TrimSpace(line)) {
			return StatusWaiting
		}
	}
	if blocked {
		return StatusWaiting
	}
	if age > idleAfter {
		return StatusIdle
	}
	return StatusWaiting
}
func (s *Session) evaluate(now time.Time, table processTable, idleAfter time.Duration, patterns []*regexp.Regexp) {
	s.mu.Lock()
	defer s.mu.Unlock()
	blocked := false
	if s.info.ExitCode == nil && now.Sub(s.info.LastActiveAt) >= 2*time.Second {
		blocked = foregroundBlocked(s.cmd.Process.Pid, table)
	}
	s.info.Status = classify(now, s.info.LastActiveAt, s.info.LastLine, s.info.ExitCode != nil, blocked, idleAfter, patterns)
}
func (s *Session) publishState() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.subs) > 0 {
		s.broadcastLocked(s.stateLocked())
	}
}
func sameDisplay(a, b Info) bool {
	return a.Title == b.Title && a.Status == b.Status && a.LastLine == b.LastLine && a.LastActiveAt.Equal(b.LastActiveAt) && a.Cols == b.Cols && a.Rows == b.Rows && a.Clients == b.Clients && sameExit(a.ExitCode, b.ExitCode) && sameWorkspace(a.Workspace, b.Workspace)
}
func sameExit(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func sameWorkspace(a, b *Workspace) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
