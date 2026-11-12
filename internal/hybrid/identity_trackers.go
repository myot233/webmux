package hybrid

import (
	"slices"
	"time"

	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/session"
)

// lockTracker follows the writer locks a terminal run holds open, for a
// profile that names its sessions by them. Codex keeps the lock of a
// session it left (/new) and, resuming, also holds a blank session's, so
// the current session is the one whose lock appeared last.
type lockTracker struct {
	pid     int
	seen    map[string]bool
	current string
	// unknown: the open files could not be read, or the locks do not tell
	// which session is current; the recently updated sessions decide.
	unknown bool
	read    time.Time
}

// track updates the current session from the locks held now. While the
// run is starting, or nothing is known, the run's resumed session wins,
// else a single lock; afterwards a single new lock is a new session.
func (lt *lockTracker) track(held []string, unreadable bool, resumed string, starting bool) {
	if unreadable {
		lt.current, lt.unknown = "", true
		return
	}
	var fresh []string
	for _, id := range held {
		if !lt.seen[id] {
			lt.seen[id] = true
			fresh = append(fresh, id)
		}
	}
	lt.unknown = false
	switch {
	case len(held) == 0:
		lt.current = ""
	case starting || lt.current == "":
		switch {
		case resumed != "" && slices.Contains(held, resumed):
			lt.current = resumed
		case len(held) == 1:
			lt.current = held[0]
		default:
			lt.current, lt.unknown = "", true
		}
	case len(fresh) == 1:
		lt.current = fresh[0]
	case len(fresh) > 1:
		lt.current, lt.unknown = "", true
	case !slices.Contains(held, lt.current):
		if len(held) == 1 {
			lt.current = held[0]
		} else {
			lt.current, lt.unknown = "", true
		}
	}
}

// lockedSession is the agent session a lock-tracking profile's terminal
// has open, read at most every lockReadEvery.
func (a *actor) lockedSession(id session.Identity) string {
	lt := &a.locks
	if id.PID == 0 {
		*lt = lockTracker{}
		return ""
	}
	if lt.pid != id.PID {
		*lt = lockTracker{pid: id.PID, seen: map[string]bool{}}
	}
	if !lt.read.IsZero() && time.Since(lt.read) < lockReadEvery {
		return lt.current
	}
	lt.read = time.Now()
	files := a.shell.RunOpenFiles()
	tracker, _ := a.cmd.Agent.(agentprofile.LockTracker)
	var held []string
	if tracker != nil {
		held = tracker.LockedSessions(files, a.shell.Getenv, a.svc.userHome)
	}
	lt.track(held, len(files) == 0, a.resumedSID, time.Since(a.runStart) < a.cmd.Profile.StartupQuiet)
	return lt.current
}

// processRegistryTracker bounds reads of the current process's registration
// record and retains only the selected native process and interpreted ID.
type processRegistryTracker struct {
	pid       int
	read      time.Time
	current   string
	unknown   bool
	process   agentprofile.ProcessFacts
	processOK bool
}

// registeredSession reads a process-registry profile's current ID at the
// same bounded cadence as writer locks. Missing or invalid records select
// the generic session/list candidate fallback.
func (a *actor) registeredSession(id session.Identity) string {
	r := &a.registry
	if id.PID == 0 {
		*r = processRegistryTracker{}
		return ""
	}
	if r.pid != id.PID {
		*r = processRegistryTracker{pid: id.PID}
	}
	if !r.read.IsZero() && time.Since(r.read) < lockReadEvery {
		return r.current
	}
	r.read = time.Now()
	r.process, r.processOK = a.agentProcess()
	r.current = ""
	r.unknown = !r.processOK
	if tracker, ok := a.cmd.Agent.(agentprofile.ProcessRegistry); ok && r.processOK {
		r.current, _ = tracker.RegisteredSession(r.process, a.shell.Getenv, a.svc.userHome)
		r.unknown = r.current == ""
	}
	return r.current
}

// agentProcess selects the native agent root, or exactly one native direct
// child of an allowed launcher. Process facts never leave the service.
func (a *actor) agentProcess() (agentprofile.ProcessFacts, bool) {
	processes := a.shell.RunProcesses()
	if len(processes) == 0 {
		return agentprofile.ProcessFacts{}, false
	}
	convert := func(p session.ProcessFacts) agentprofile.ProcessFacts {
		return agentprofile.ProcessFacts{PID: p.PID, Executable: p.Executable, Cwd: p.Cwd, Start: p.Start, Domain: p.Domain}
	}
	root := convert(processes[0])
	if a.cmd.Profile.RunsProcess(root.Executable) {
		return root, true
	}
	if a.cmd.Profile.Launcher == "" || root.Executable != a.cmd.Profile.Launcher {
		return agentprofile.ProcessFacts{}, false
	}
	var selected agentprofile.ProcessFacts
	for _, process := range processes[1:] {
		candidate := convert(process)
		if !a.cmd.Profile.RunsProcess(candidate.Executable) {
			continue
		}
		if selected.PID != 0 {
			return agentprofile.ProcessFacts{}, false
		}
		selected = candidate
	}
	return selected, selected.PID != 0
}

func (a *actor) tracksRegistry() bool {
	_, ok := a.cmd.Agent.(agentprofile.ProcessRegistry)
	return ok
}

func (a *actor) identityFallback() bool {
	// Agents with no session-ID source of their own (no LockTracker, no
	// ProcessRegistry, no OSC 7501 child record) always need the
	// session/list candidate probe.
	if !a.tracksLocks() && !a.tracksRegistry() {
		return true
	}
	return (a.tracksLocks() && a.locks.unknown) || (a.tracksRegistry() && a.registry.unknown)
}

// externallyHeld checks registry occupancy at most once per observation
// cadence. Operation boundaries force a fresh check before opening a writer.
func (a *actor) externallyHeld(sid string, force bool) bool {
	if sid == "" || !a.tracksRegistry() {
		return false
	}
	if !force && a.externalSID == sid && !a.externalRead.IsZero() && time.Since(a.externalRead) < lockReadEvery {
		return a.externalHeld
	}
	current := a.registry.process
	if force && a.side == SideTUI {
		if process, ok := a.agentProcess(); ok {
			current = process
		} else {
			current = agentprofile.ProcessFacts{}
		}
	}
	if current.Cwd == "" {
		current.Cwd = a.agentCwd
		if current.Cwd == "" {
			current.Cwd = a.cwd
		}
	}
	tracker, _ := a.cmd.Agent.(agentprofile.ProcessRegistry)
	a.externalSID, a.externalRead = sid, time.Now()
	a.externalHeld = a.registryExternalHeld(sid, current, tracker)
	return a.externalHeld
}

// registryExternalHeld performs one fresh scan without changing actor state,
// so ACP subtasks can also use it at operation boundaries.
func (a *actor) registryExternalHeld(sid string, current agentprofile.ProcessFacts, tracker agentprofile.ProcessRegistry) bool {
	if tracker == nil {
		return false
	}
	if current.Cwd == "" {
		current.Cwd = a.agentCwd
		if current.Cwd == "" {
			current.Cwd = a.cwd
		}
	}
	lookup := func(pid int) (agentprofile.ProcessFacts, bool) {
		process, ok := session.LookupProcess(pid)
		if !ok {
			return agentprofile.ProcessFacts{}, false
		}
		return agentprofile.ProcessFacts{PID: process.PID, Executable: process.Executable, Cwd: process.Cwd, Start: process.Start, Domain: process.Domain}, true
	}
	return tracker.ExternallyHeld(sid, current, lookup, a.shell.Getenv, a.svc.userHome)
}

func (a *actor) externalWriterNow(sid string) bool {
	tracker, _ := a.cmd.Agent.(agentprofile.ProcessRegistry)
	current := agentprofile.ProcessFacts{Cwd: a.agentCwd}
	return tracker != nil && a.registryExternalHeld(sid, current, tracker)
}

func (a *actor) agentIsRoot(id session.Identity) bool {
	if id.PID == 0 {
		return false
	}
	p := a.cmd.Profile
	if p.RunsProcess(id.Executable) {
		return true
	}
	return p.Launcher != "" && id.Executable == p.Launcher && slices.ContainsFunc(a.shell.RunChildren(), p.RunsProcess)
}

// tracksLocks reports that the agent's session IDs come from its writer
// locks rather than its terminal signals.
func (a *actor) tracksLocks() bool {
	_, ok := a.cmd.Agent.(agentprofile.LockTracker)
	return ok
}

func signals(id session.Identity) agentprofile.Signals {
	return agentprofile.Signals{
		WarpAgent: id.WarpAgent, WarpPluginVersion: id.WarpPluginVersion, WarpSessionID: id.WarpSessionID,
		WarpCwd: id.WarpCwd, ProgramApp: id.ProgramApp, ProgramChildID: id.ProgramChildID,
		RootExecutable: id.Executable, RootCwd: id.Cwd,
	}
}
