package session

import "os"

// ContainsProcess reports whether pid runs inside a WebMux session: on Linux
// it carries a WEBMUX_SESSION_ID marker, or one of its ancestors is a session
// leader, a member of a session's terminal session, or this server itself
// (the Linux subreaper adopts orphaned session descendants). macOS cannot
// read other processes' environments, so only the process tree counts there.
func (m *Manager) ContainsProcess(pid int) bool {
	if hasSessionMarker(pid) {
		return true
	}
	leaders := make(map[int]bool)
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.mu.Unlock()
	for _, s := range sessions {
		s.mu.Lock()
		if s.run != nil && s.run.cmd != nil && s.run.cmd.Process != nil {
			leaders[s.run.cmd.Process.Pid] = true
		}
		for sidecar := range s.sidecars {
			leaders[sidecar.pid] = true
		}
		s.mu.Unlock()
	}
	return insideTree(processes(), pid, os.Getpid(), leaders)
}
