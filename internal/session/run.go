package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// run is one PTY process of a session: its output ring, generation, input
// gate, reader, reply loop and exit flow. An ordinary session has exactly one
// run; a shell starts a new run, under a new generation, after the previous
// one ended.
type run struct {
	cmd      *exec.Cmd
	terminal *os.File
	ring     *RingBuffer
	// inputGate serializes PTY input; replyWake wakes the reply loop.
	inputGate chan struct{}
	replyWake chan struct{}
	// lifetime ends when the process exits; it cancels pending replies.
	lifetime    context.Context
	endLifetime context.CancelFunc
	// done closes once exit (or the moved message) is published, after the
	// process group is cleaned up and the PTY drained.
	done        chan struct{}
	readerDone  chan struct{}
	cleanupDone chan struct{}
	repliesDone chan struct{}
	closeOnce   sync.Once
	// moved and killed are owned by Session.mu. moved: EndRun ends the run
	// for a switch, so subscribers get Moved instead of an exit. killed: the
	// run ended through Session.Close, which is not a natural exit.
	moved  bool
	killed bool
}

// launchRun starts argv on a new PTY of the given grid. It runs fork/exec,
// so callers never hold Session.mu around it.
func launchRun(id, cwd string, argv []string, cols, rows uint16, options startOptions) (*run, string, error) {
	ring, err := NewRingBuffer(options.capacity)
	if err != nil {
		return nil, "", err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = cwd
	cmd.Env = sessionEnvWithVersion(os.Environ(), id, options.agentProtocol, options.env, options.terminalVersion)
	f, err := startPTY(cmd, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		return nil, "", err
	}
	pollable, err := pollablePTY(f)
	if err != nil {
		_ = f.Close()
		abortRun(cmd, id)
		return nil, "", err
	}
	f = pollable
	var raw [8]byte
	if _, err = rand.Read(raw[:]); err != nil {
		_ = f.Close()
		abortRun(cmd, id)
		return nil, "", err
	}
	r := &run{
		cmd:         cmd,
		terminal:    f,
		ring:        ring,
		inputGate:   make(chan struct{}, 1),
		replyWake:   make(chan struct{}, 1),
		done:        make(chan struct{}),
		readerDone:  make(chan struct{}),
		cleanupDone: make(chan struct{}),
		repliesDone: make(chan struct{}),
	}
	r.lifetime, r.endLifetime = context.WithCancel(context.Background())
	r.inputGate <- struct{}{}
	return r, hex.EncodeToString(raw[:]), nil
}

func abortRun(cmd *exec.Cmd, id string) {
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Wait()
	releaseLeader(cmd.Process.Pid)
	cleanupProcesses(cmd.Process.Pid, id)
}

// installRunLocked makes r the current run. Run-scoped terminal state starts
// fresh: offsets, replies and status signals belong to one generation.
func (s *Session) installRunLocked(r *run, generation string) {
	s.run = r
	s.info.Generation = generation
	s.info.Status = StatusRunning
	s.info.LastActiveAt = time.Now()
}

// startRun begins the run's reader, reply loop and exit flow.
func (s *Session) startRun(r *run) {
	go s.read(r)
	go s.replyLoop(r)
	go s.wait(r)
}

// close hangs up the run's process group and cleans up every process of the
// session in the background; cleanupDone closes when that finishes.
func (r *run) close(id string) {
	r.closeOnce.Do(func() {
		_ = syscall.Kill(-r.cmd.Process.Pid, syscall.SIGHUP)
		go func() {
			defer close(r.cleanupDone)
			cleanupProcesses(r.cmd.Process.Pid, id)
		}()
	})
}

func (r *run) ended() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

func (s *Session) read(r *run) {
	defer close(r.readerDone)
	buf := make([]byte, 32768)
	for {
		n, err := r.terminal.Read(buf)
		if n > 0 {
			p := buf[:n]
			s.mu.Lock()
			r.ring.Write(p)
			s.osc.Feed(p, s.signals)
			reports := s.preview.colorSchemeReports
			s.info.Preview = s.preview.FeedThrottled(p)
			s.info.LastActiveAt = time.Now()
			// Latched attention and a program's own status outlive redraws
			// such as spinners and timers.
			s.info.Status = s.outputStatusLocked()
			if len(s.subs) > 0 {
				s.broadcastLocked(outputMessage(p))
				// After the output that toggled it, so the mode applies from there.
				if s.preview.colorSchemeReports != reports {
					s.broadcastLocked(s.controlLocked())
				}
			}
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func exitCode(cmd *exec.Cmd, err error) int {
	if err == nil {
		return 0
	}
	if cmd.ProcessState == nil {
		// Wait failed without a status (e.g. ECHILD).
		return -1
	}
	code := cmd.ProcessState.ExitCode()
	if code < 0 {
		if status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
			code = 128 + int(status.Signal())
		}
	}
	return code
}

func (s *Session) wait(r *run) {
	err := r.cmd.Wait()
	releaseLeader(r.cmd.Process.Pid)
	code := exitCode(r.cmd, err)
	// Drain final PTY bytes before publishing exit.
	r.close(s.info.ID)
	<-r.cleanupDone
	drain := time.NewTimer(time.Second)
	select {
	case <-r.readerDone:
		drain.Stop()
	case <-drain.C:
		_ = r.terminal.Close()
		<-r.readerDone
	}
	_ = r.terminal.Close()
	s.mu.Lock()
	if s.repaintTimer != nil {
		s.repaintTimer.Stop()
		s.repaintTimer = nil
	}
	if r.moved {
		s.releaseRunLocked()
		r.endLifetime()
		close(r.done)
		s.mu.Unlock()
		<-r.repliesDone
		slog.Info("session run ended", "id", s.info.ID, "exitCode", code)
		return
	}
	s.info.Status = StatusExited
	s.info.Attention = nil
	// Publish the final screen, including output throttled before exit.
	if s.preview.dirty {
		s.info.Preview = s.preview.refresh(true)
	}
	s.program = nil
	s.identity = Identity{}
	// A finished program can no longer stand behind its last directory report.
	s.info.ReportedCwd = nil
	s.info.ExitCode = &code
	s.broadcastLocked(s.stateLocked())
	s.broadcastLocked(exitMessage(code))
	r.endLifetime()
	close(r.done)
	if !s.shell {
		close(s.done)
	}
	notify := s.driver != nil && !r.killed
	s.mu.Unlock()
	<-r.repliesDone
	slog.Info("session exited", "id", s.info.ID, "exitCode", code)
	if notify {
		s.driver.RunExited(code)
	}
}

// Close ends the session's processes. An ordinary session hangs up its run;
// a shell also stops its sidecars and refuses to start new ones. Neither
// calls the shell's driver: Manager.Close does that.
func (s *Session) Close() {
	if !s.shell {
		s.mu.Lock()
		r := s.run
		s.mu.Unlock()
		r.close(s.info.ID)
		return
	}
	// startMu waits out a run or sidecar being forked, so none escapes.
	s.startMu.Lock()
	s.mu.Lock()
	s.closed = true
	r := s.run
	if r != nil {
		r.killed = true
	}
	sidecars := make([]*Sidecar, 0, len(s.sidecars))
	for sidecar := range s.sidecars {
		sidecars = append(sidecars, sidecar)
	}
	s.mu.Unlock()
	s.startMu.Unlock()
	if r != nil {
		r.close(s.info.ID)
	}
	for _, sidecar := range sidecars {
		sidecar.terminate()
	}
}
