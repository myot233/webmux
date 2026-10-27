package main

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/runtime"
)

// daemonTestEnv makes the detached `serve` child of an in-process `start`
// run this test binary with its real arguments through WEBMUX_TEST_EXEC=argv.
func daemonTestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("WEBMUX_TEST_EXEC", "argv")
}

// assertDetached checks the Linux-only evidence for a new session and a
// cleaned launcher environment; other platforms cannot inspect the child.
func assertDetached(t *testing.T, pid int) {
	t.Helper()
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	end := bytes.LastIndexByte(stat, ')')
	if end < 0 {
		t.Fatalf("stat %q", stat)
	}
	fields := strings.Fields(string(stat[end+1:]))
	if len(fields) < 4 || fields[3] != strconv.Itoa(pid) {
		t.Fatalf("session leader fields %v for PID %d", fields, pid)
	}
	environ, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"WEBMUX_SESSION_ID=", "WEBMUX_LAUNCHER=", "INVOCATION_ID=", "XPC_SERVICE_NAME="} {
		if bytes.Contains(environ, []byte(name)) {
			t.Fatalf("daemon inherited %s from its launcher", strings.TrimSuffix(name, "="))
		}
	}
}

func TestStartDetachesAndStops(t *testing.T) {
	f := newFixture(t, "")
	logPath := filepath.Join(f.state, "webmux.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("previous run\n"), 0600); err != nil {
		t.Fatal(err)
	}
	daemonTestEnv(t)
	t.Cleanup(func() { _, _, _ = f.run(t, "stop", "--timeout", "5s") })

	var stdout, stderr bytes.Buffer
	if code := dispatch([]string{"start", "--config", f.config}, &stdout, &stderr); code != contract.ExitOK {
		t.Fatalf("start %d stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
	record := f.record(t)
	if record.PID == os.Getpid() || record.Launcher != "manual" || record.LogFile != logPath {
		t.Fatalf("record %+v", record)
	}
	assertDetached(t, record.PID)
	if !strings.Contains(stdout.String(), "WebMux local:") || strings.Contains(stdout.String(), fixtureToken) {
		t.Fatalf("start output %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Using config:") {
		t.Fatalf("start did not report its configuration: %q", stdout.String())
	}
	if old, _ := os.ReadFile(logPath + ".1"); string(old) != "previous run\n" {
		t.Fatalf("rotated log %q", old)
	}
	if info, err := os.Stat(logPath); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("log file %v %v", info, err)
	}
	if data, _ := os.ReadFile(logPath); strings.Contains(string(data), "settings unavailable") {
		t.Fatalf("start held a settings lock across the spawn:\n%s", data)
	}

	stdout.Reset()
	stderr.Reset()
	if code := dispatch([]string{"start", "--config", f.config}, &stdout, &stderr); code != contract.ExitOK {
		t.Fatalf("second start %d stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "WebMux is running") {
		t.Fatalf("second start output %q", stdout.String())
	}
	if again := f.record(t); again.PID != record.PID {
		t.Fatalf("second start replaced PID %d with %d", record.PID, again.PID)
	}

	status, _, code := f.run(t, "status", "--json")
	if code != contract.ExitOK || decodeStatus(t, status).State != contract.StateRunning {
		t.Fatalf("status %d %q", code, status)
	}
	if _, _, code := f.run(t, "stop", "--json"); code != contract.ExitOK {
		t.Fatalf("stop %d", code)
	}
	runDir := runtime.Dir(f.state)
	if _, err := os.Stat(runtime.RecordPath(runDir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("record left behind: %v", err)
	}
	if _, err := os.Stat(record.Socket); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket left behind: %v", err)
	}
}

func TestStartClearsInheritedSessionAndServiceEnvironment(t *testing.T) {
	f := newFixture(t, "")
	daemonTestEnv(t)
	t.Setenv("WEBMUX_SESSION_ID", "session-1")
	t.Setenv("WEBMUX_LAUNCHER", "windows-tray")
	t.Setenv("INVOCATION_ID", "unit-1")
	t.Setenv("XPC_SERVICE_NAME", "com.webmux")
	t.Cleanup(func() { _, _, _ = f.run(t, "stop", "--timeout", "5s") })

	var stdout, stderr bytes.Buffer
	if code := dispatch([]string{"start", "--config", f.config}, &stdout, &stderr); code != contract.ExitOK {
		t.Fatalf("start %d stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
	record := f.record(t)
	if launcher := record.Launcher; launcher != "manual" {
		t.Fatalf("launcher %q", launcher)
	}
	assertDetached(t, record.PID)
	if _, _, code := f.run(t, "stop", "--json"); code != contract.ExitOK {
		t.Fatalf("stop without --force %d", code)
	}
}

func TestStartReportsChildFailure(t *testing.T) {
	f := newFixture(t, "")
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = occupied.Close() }()
	content := fmt.Sprintf("listen = %q\ntoken = %q\ndefault_cwd = %q\nstate_dir = %q\n", occupied.Addr().String(), fixtureToken, f.dir, f.state)
	if err := os.WriteFile(f.config, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(f.state, "webmux.log")
	daemonTestEnv(t)

	var stdout, stderr bytes.Buffer
	if code := dispatch([]string{"start", "--config", f.config}, &stdout, &stderr); code == contract.ExitOK {
		t.Fatalf("start succeeded on an occupied port: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "address already in use") || !strings.Contains(stderr.String(), logPath) {
		t.Fatalf("start error %q", stderr.String())
	}
}

func TestStartRejectsInvalidConfiguration(t *testing.T) {
	f := newFixture(t, "")
	daemonTestEnv(t)
	var stdout, stderr bytes.Buffer
	if code := dispatch([]string{"start", "--config", filepath.Join(f.dir, "missing.toml")}, &stdout, &stderr); code != contract.ExitError {
		t.Fatalf("start exit %d stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "missing.toml") {
		t.Fatalf("start error %q", stderr.String())
	}
}

func TestStartAcceptsRelativeConfig(t *testing.T) {
	f := newFixture(t, "")
	daemonTestEnv(t)
	t.Cleanup(func() { _, _, _ = f.run(t, "stop", "--timeout", "5s") })
	t.Chdir(f.dir)

	var stdout, stderr bytes.Buffer
	if code := dispatch([]string{"start", "--config", "config.toml"}, &stdout, &stderr); code != contract.ExitOK {
		t.Fatalf("start %d stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Using config:    "+f.config) {
		t.Fatalf("start resolved the wrong config: %q", stdout.String())
	}
	if record := f.record(t); record.Launcher != "manual" {
		t.Fatalf("record launcher %q", record.Launcher)
	}
}

func TestStartReportsPreLogFailure(t *testing.T) {
	f := newFixture(t, "")
	daemonTestEnv(t)
	blocker := filepath.Join(f.dir, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(blocker, "webmux.log")

	var stdout, stderr bytes.Buffer
	if code := dispatch([]string{"start", "--config", f.config, "--log-file", logPath}, &stdout, &stderr); code == contract.ExitOK {
		t.Fatalf("start succeeded with an unwritable log: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "log file") {
		t.Fatalf("start error lost the child diagnostic: %q", stderr.String())
	}
	if _, err := os.Stat(runtime.RecordPath(runtime.Dir(f.state))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed start left a runtime record: %v", err)
	}
}

func TestStartReplacesStaleRecord(t *testing.T) {
	f := newFixture(t, "")
	runDir := runtime.Dir(f.state)
	if err := runtime.EnsureDir(runDir); err != nil {
		t.Fatal(err)
	}
	stale := runtime.Record{
		PID: os.Getpid(), Identity: runtime.Identity{Start: "stale"},
		Listen: "127.0.0.1:1", Launcher: "manual",
	}
	if err := runtime.Write(runDir, stale); err != nil {
		t.Fatal(err)
	}
	daemonTestEnv(t)
	t.Cleanup(func() { _, _, _ = f.run(t, "stop", "--timeout", "5s") })

	var stdout, stderr bytes.Buffer
	if code := dispatch([]string{"start", "--config", f.config}, &stdout, &stderr); code != contract.ExitOK {
		t.Fatalf("start %d stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
	if record := f.record(t); record.PID == os.Getpid() || record.Listen == stale.Listen {
		t.Fatalf("record not replaced: %+v", record)
	}
}

func TestStartReportsUnresponsiveInstance(t *testing.T) {
	f := newFixture(t, "")
	standIn(t, f, "while :; do sleep 0.1; done")

	var stdout, stderr bytes.Buffer
	if code := dispatch([]string{"start", "--config", f.config}, &stdout, &stderr); code != contract.ExitError {
		t.Fatalf("start reported success for an unresponsive instance: %d stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "does not respond") {
		t.Fatalf("start output %q", stdout.String())
	}
}

func TestStartHonorsLogFile(t *testing.T) {
	f := newFixture(t, "")
	logPath := filepath.Join(f.dir, "custom.log")
	daemonTestEnv(t)
	t.Cleanup(func() { _, _, _ = f.run(t, "stop", "--timeout", "5s") })

	var stdout, stderr bytes.Buffer
	if code := dispatch([]string{"start", "--config", f.config, "--log-file", logPath}, &stdout, &stderr); code != contract.ExitOK {
		t.Fatalf("start %d stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
	if record := f.record(t); record.LogFile != logPath {
		t.Fatalf("record log file %q", record.LogFile)
	}
	if info, err := os.Stat(logPath); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("custom log %v %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(f.state, "webmux.log")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("default log was also written: %v", err)
	}
}
