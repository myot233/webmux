package session

// Identity is what a session knows about the program in its current run:
// bounded, printable identity fields from its terminal output and its PTY's
// root process. It is agent-neutral; internal/agentprofile interprets it.
// It lives in memory only: it is never part of Info, logged or pushed, and
// it is cleared when a run ends or a new one starts.
type Identity struct {
	// Fields of Warp cli-agent events.
	WarpAgent         string
	WarpPluginVersion string
	WarpSessionID     string
	WarpCwd           string
	// ProgramApp is the OSC 7501 root record's app.
	ProgramApp string
	// ProgramChildID is the first segment of the latest OSC 7501 child
	// record id.
	ProgramChildID string
	// PID, Executable and Cwd describe the current run's root process. They
	// are empty without a live run; Executable and Cwd are also empty when
	// the process table cannot be read.
	PID        int
	Executable string
	Cwd        string
}

// mergeSignals keeps every signal field a report carried; the rest stay.
func (i *Identity) mergeSignals(report Identity) {
	set := func(to *string, from string) {
		if from != "" {
			*to = from
		}
	}
	set(&i.WarpAgent, report.WarpAgent)
	set(&i.WarpPluginVersion, report.WarpPluginVersion)
	set(&i.WarpSessionID, report.WarpSessionID)
	set(&i.WarpCwd, report.WarpCwd)
	set(&i.ProgramApp, report.ProgramApp)
	set(&i.ProgramChildID, report.ProgramChildID)
}

// Identity returns the identity of the session's current run. The root
// process is looked up on each call, outside the session lock.
func (s *Session) Identity() Identity {
	s.mu.Lock()
	identity := s.identity
	pid := 0
	if r := s.run; r != nil && !r.ended() {
		pid = r.cmd.Process.Pid
	}
	s.mu.Unlock()
	if pid > 0 {
		identity.PID = pid
		identity.Executable, identity.Cwd = rootProcess(pid)
	}
	return identity
}

// Identity returns the identity of session id's current run.
func (m *Manager) Identity(id string) (Identity, error) {
	s, err := m.Get(id)
	if err != nil {
		return Identity{}, err
	}
	return s.Identity(), nil
}
