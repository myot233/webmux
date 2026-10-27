package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rainchan/webmux/internal/client"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/runtime"
	"github.com/rainchan/webmux/internal/sessioncmd"
)

// startTimeout bounds how long `webmux start` waits for the child to publish
// its runtime record. A tunnel-enabled server may download cloudflared first.
const startTimeout = 3 * time.Minute

// startStopGrace gives a timed-out child time to shut its PTY sessions down
// before it is killed outright.
const startStopGrace = 10 * time.Second

// startCommand runs a detached server. The child is a normal `serve` process
// in a new session with its own log file, so `webmux stop` controls it.
func startCommand(args []string, stdout, stderr io.Writer) int {
	run, err := parseServerRun("webmux start", args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return contract.ExitOK
		}
		var parseErr flagParseError
		if errors.As(err, &parseErr) {
			return contract.ExitError
		}
		_, _ = fmt.Fprintln(stderr, "webmux start:", err)
		return contract.ExitError
	}
	// The settings stores hold their exclusive locks for their whole lifetime.
	// Close them before spawning `serve`, or the child would run with tunnel
	// and Tailscale settings unavailable for as long as this process waits.
	run.TunnelSettings.Close()
	run.TailscaleSettings.Close()

	target := client.Locate(run.ConfigPath, run.ConfigExplicit)
	if target.ConfigErr != nil {
		_, _ = fmt.Fprintln(stderr, "webmux start:", target.ConfigErr)
		return contract.ExitError
	}
	if target.Record != nil {
		status, code := client.Status(context.Background(), target, false, client.NetInterfaces)
		sessioncmd.PrintStatus(stdout, status)
		return code
	}

	logPath := run.LogFile
	if logPath == "" {
		logPath = filepath.Join(run.StateDir, "webmux.log")
	}
	before := statLog(logPath)
	record, err := launchDaemon(run, args, logPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "webmux start:", err)
		if before.changed(logPath) {
			if tail := readLogTail(logPath, 10); tail != "" {
				_, _ = fmt.Fprintln(stderr, tail)
			}
		}
		return contract.ExitError
	}
	printConfigNotice(stdout, run)
	_, _ = fmt.Fprintf(stdout, "WebMux started (PID %d)\n", record.PID)
	_, _ = fmt.Fprintln(stdout, "WebMux local:  "+client.LocalURL(record.Listen))
	_, _ = fmt.Fprintln(stdout, "Log file:      "+logPath)
	return contract.ExitOK
}

// launchDaemon starts a detached `serve` and waits for its runtime record.
func launchDaemon(run serverRun, args []string, logPath string) (runtime.Record, error) {
	executable, err := os.Executable()
	if err != nil {
		return runtime.Record{}, err
	}
	childArgs := append([]string{"serve"}, args...)
	childArgs = append(childArgs, "--log-file", logPath)
	cmd := exec.Command(executable, childArgs...)
	cmd.Env = daemonEnv(os.Environ())
	// Keep the launcher's working directory: relative --config and relative
	// TOML paths must resolve exactly as they did in this process.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// A nil writer would discard the errors `serve` reports before it opens
	// its log file. Capture the inherited stderr until redirectOutput replaces
	// it, then report those bytes if the child fails to start.
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		return runtime.Record{}, err
	}
	cmd.Stderr = writeEnd
	// Stdout and stdin stay nil, so the child never inherits the launcher's
	// terminal or input; stderr is captured only until --log-file takes over.
	if err := cmd.Start(); err != nil {
		_ = readEnd.Close()
		_ = writeEnd.Close()
		return runtime.Record{}, err
	}
	_ = writeEnd.Close()
	serverOutput := readStartupOutput(readEnd)
	record, waitErr := waitForDaemon(cmd, runtime.Dir(run.StateDir), logPath)
	if waitErr != nil {
		if output := <-serverOutput; output != "" {
			waitErr = fmt.Errorf("%w\n%s", waitErr, output)
		}
		return runtime.Record{}, waitErr
	}
	// The child redirected fd 2 before publishing its record, so closing the
	// read end also releases the capture goroutine.
	_ = readEnd.Close()
	return record, nil
}

// readStartupOutput drains the child's pre-log stderr without letting a stuck
// child grow the parent's buffer without bound.
func readStartupOutput(r *os.File) <-chan string {
	output := make(chan string, 1)
	go func() {
		defer func() { _ = r.Close() }()
		body, _ := io.ReadAll(io.LimitReader(r, 64<<10))
		output <- strings.TrimSpace(string(body))
	}()
	return output
}

// daemonEnv keeps a detached server independent of its launcher: the session
// marker would make the parent session's cleanup treat it as a descendant,
// and service markers would record the wrong launcher and require --force.
func daemonEnv(env []string) []string {
	drop := map[string]bool{
		"WEBMUX_SESSION_ID": true,
		"WEBMUX_LAUNCHER":   true,
		"INVOCATION_ID":     true,
		"XPC_SERVICE_NAME":  true,
	}
	kept := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if !drop[name] {
			kept = append(kept, entry)
		}
	}
	return kept
}

func waitForDaemon(cmd *exec.Cmd, runDir, logPath string) (runtime.Record, error) {
	var waitErr error
	exited := make(chan struct{})
	go func() {
		waitErr = cmd.Wait()
		close(exited)
	}()
	deadline := time.Now().Add(startTimeout)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		record, err := runtime.Read(runDir)
		if err == nil && record.PID == cmd.Process.Pid && record.Alive() == nil {
			return record, nil
		}
		select {
		case <-exited:
			return runtime.Record{}, fmt.Errorf("server exited before it published its runtime record: %v; see %s", waitErr, logPath)
		case <-ticker.C:
		}
		if time.Now().After(deadline) {
			// SIGTERM lets serve close its PTY sessions through its normal
			// shutdown path; those sessions are in their own process groups,
			// so killing this group alone would orphan them.
			terminateDaemon(cmd, exited)
			return runtime.Record{}, fmt.Errorf("server did not start within %s; see %s", startTimeout, logPath)
		}
	}
}

func terminateDaemon(cmd *exec.Cmd, exited <-chan struct{}) {
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	timer := time.NewTimer(startStopGrace)
	defer timer.Stop()
	select {
	case <-exited:
		return
	case <-timer.C:
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
	<-exited
}

// logSnapshot detects whether this start produced log output, so a failure
// report never presents a previous run's log as the cause.
type logSnapshot struct {
	exists  bool
	size    int64
	modTime time.Time
}

func statLog(path string) logSnapshot {
	info, err := os.Stat(path)
	if err != nil {
		return logSnapshot{}
	}
	return logSnapshot{exists: true, size: info.Size(), modTime: info.ModTime()}
}

func (s logSnapshot) changed(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !s.exists || info.Size() != s.size || !info.ModTime().Equal(s.modTime)
}

// readLogTail returns the last lines of a log file, bounded so a large log
// cannot flood the terminal.
func readLogTail(path string, lines int) string {
	const maxBytes = 64 << 10
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return ""
	}
	start := info.Size() - maxBytes
	if start < 0 {
		start = 0
	}
	body := make([]byte, info.Size()-start)
	if _, err := file.ReadAt(body, start); err != nil {
		return ""
	}
	text := strings.TrimRight(string(body), "\n")
	if start > 0 {
		if index := strings.IndexByte(text, '\n'); index >= 0 {
			text = text[index+1:]
		}
	}
	parts := strings.Split(text, "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, "\n")
}
