package session

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const (
	// sidecarStderrCapacity bounds the retained stderr tail.
	sidecarStderrCapacity = 64 << 10
	// sidecarStopGrace is how long Stop waits after closing stdin before it
	// hangs up the process group.
	sidecarStopGrace = 3 * time.Second
)

// Sidecar is a pipe-connected helper process of a shell, such as an ACP
// agent. It gets the shell's launch environment and directory, runs in its
// own session and process group, and is registered as a managed leader so
// the Linux subreaper never takes its exit status. Its stderr is kept only
// as a bounded tail for error details; it is never logged.
type Sidecar struct {
	cmd    *exec.Cmd
	pid    int
	stdin  *os.File
	stdout *os.File
	stderr *RingBuffer
	// stderrPipe is drained by one goroutine until stderrDone.
	stderrPipe  *os.File
	stderrDone  chan struct{}
	exited      chan struct{}
	cleanupDone chan struct{}
	done        chan struct{}
	code        int
	stopOnce    sync.Once
}

// StartSidecar starts argv beside the shell's runs. With protocolOnFD3 the
// child's stdout and stderr both go to the stderr tail and the protocol pipe
// is its fd 3, so a wrapper such as
// `bash -ic 'exec -- "$@" 1>&3 3>&-' webmux <agent>` keeps startup-file
// output away from the protocol.
func (s *Session) StartSidecar(argv []string, protocolOnFD3 bool) (*Sidecar, error) {
	if len(argv) == 0 || argv[0] == "" {
		return nil, fmt.Errorf("%w: command requires an executable", ErrInvalid)
	}
	if !s.shell {
		return nil, fmt.Errorf("%w: not a shell", ErrInvalid)
	}
	s.startMu.Lock()
	defer s.startMu.Unlock()
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil, ErrClosed
	}
	sidecar, err := startSidecar(s.info.ID, s.info.Cwd, argv, protocolOnFD3, s.options)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	s.mu.Lock()
	s.sidecars[sidecar] = struct{}{}
	s.mu.Unlock()
	go s.waitSidecar(sidecar)
	return sidecar, nil
}

func startSidecar(id, cwd string, argv []string, protocolOnFD3 bool, options startOptions) (*Sidecar, error) {
	var files []*os.File
	closeAll := func() {
		for _, f := range files {
			_ = f.Close()
		}
	}
	pipe := func() (r, w *os.File, err error) {
		if r, w, err = os.Pipe(); err == nil {
			files = append(files, r, w)
		}
		return r, w, err
	}
	stdinR, stdinW, err := pipe()
	if err != nil {
		return nil, err
	}
	stdoutR, stdoutW, err := pipe()
	if err != nil {
		closeAll()
		return nil, err
	}
	stderrR, stderrW, err := pipe()
	if err != nil {
		closeAll()
		return nil, err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = cwd
	cmd.Env = sessionEnvWithVersion(os.Environ(), id, options.agentProtocol, options.env, options.terminalVersion)
	cmd.Stdin = stdinR
	cmd.Stdout, cmd.Stderr = stdoutW, stderrW
	if protocolOnFD3 {
		cmd.Stdout = stderrW
		cmd.ExtraFiles = []*os.File{stdoutW}
	}
	// A new session is also a new process group, and its ID lets process
	// cleanup follow descendants without the shared session marker, which
	// would also match the shell's PTY run.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err = startChild(cmd); err != nil {
		closeAll()
		return nil, err
	}
	_ = stdinR.Close()
	_ = stdoutW.Close()
	_ = stderrW.Close()
	stderr, err := NewRingBuffer(sidecarStderrCapacity)
	if err != nil {
		panic(err) // constant positive capacity
	}
	sidecar := &Sidecar{
		cmd:         cmd,
		pid:         cmd.Process.Pid,
		stdin:       stdinW,
		stdout:      stdoutR,
		stderr:      stderr,
		stderrPipe:  stderrR,
		stderrDone:  make(chan struct{}),
		exited:      make(chan struct{}),
		cleanupDone: make(chan struct{}),
		done:        make(chan struct{}),
		code:        -1,
	}
	go sidecar.drainStderr()
	return sidecar, nil
}

func (c *Sidecar) drainStderr() {
	defer close(c.stderrDone)
	buf := make([]byte, 4096)
	for {
		n, err := c.stderrPipe.Read(buf)
		if n > 0 {
			c.stderr.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// waitSidecar reaps the leader, ends the rest of its process tree, drains
// stderr, and only then reports Done.
func (s *Session) waitSidecar(c *Sidecar) {
	err := c.cmd.Wait()
	releaseLeader(c.pid)
	c.code = exitCode(c.cmd, err)
	close(c.exited)
	c.terminate()
	<-c.cleanupDone
	drain := time.NewTimer(time.Second)
	select {
	case <-c.stderrDone:
		drain.Stop()
	case <-drain.C:
	}
	_ = c.stderrPipe.Close()
	<-c.stderrDone
	s.mu.Lock()
	delete(s.sidecars, c)
	s.mu.Unlock()
	close(c.done)
}

// terminate hangs up the process group and cleans up the process tree in
// the background; cleanupDone closes when that finishes.
func (c *Sidecar) terminate() {
	c.stopOnce.Do(func() {
		_ = syscall.Kill(-c.pid, syscall.SIGHUP)
		go func() {
			defer close(c.cleanupDone)
			cleanupProcesses(c.pid, "")
		}()
	})
}

// Stdin is the child's standard input.
func (c *Sidecar) Stdin() io.WriteCloser { return c.stdin }

// Stdout is the protocol output: the child's stdout, or its fd 3.
func (c *Sidecar) Stdout() io.Reader { return c.stdout }

// Stderr returns the retained stderr tail, at most 64 KiB.
func (c *Sidecar) Stderr() []byte { return c.stderr.Snapshot() }

// PID is the sidecar's leader process ID, also its process group ID.
func (c *Sidecar) PID() int { return c.pid }

// Done closes after the process tree is gone and stderr is drained.
func (c *Sidecar) Done() <-chan struct{} { return c.done }

// ExitCode is the leader's exit status (128+signal when killed), or -1
// before Done.
func (c *Sidecar) ExitCode() int {
	select {
	case <-c.done:
		return c.code
	default:
		return -1
	}
}

// Stop closes stdin, gives the process a grace period to exit on its own,
// then hangs up its process group and cleans up the tree (SIGKILL after the
// usual cleanup timeout). It releases the pipes once the sidecar is done, so
// call it even after an unexpected exit. When ctx expires first the process
// tree is killed and ctx's error returned.
func (c *Sidecar) Stop(ctx context.Context) error {
	_ = c.stdin.Close()
	grace := time.NewTimer(sidecarStopGrace)
	select {
	case <-c.exited:
	case <-grace.C:
	case <-ctx.Done():
	}
	grace.Stop()
	c.terminate()
	select {
	case <-c.done:
		_ = c.stdout.Close()
		return nil
	case <-ctx.Done():
		forceSession(c.pid, "")
		return ctx.Err()
	}
}
