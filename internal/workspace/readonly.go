package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/rainchan/webmux/internal/session"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// ReadGit is a bounded, noninteractive, read-only runner shared by code views.
// It never refreshes the index or invokes user diff/textconv helpers.
func ReadGit(ctx context.Context, cwd string, limit int, args ...string) ([]byte, error) {
	return ReadGitInput(ctx, cwd, limit, nil, args...)
}

func ReadGitInput(ctx context.Context, cwd string, limit int, input io.Reader, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	overrides := []string{}
	if len(args) > 0 && (args[0] == "diff" || args[0] == "status") {
		// Even raw/status comparisons can run clean/process filters. Disable
		// every configured driver for this command without changing config.
		names, e := readGitCommand(ctx, cwd, limit, nil, nil, "config", "--null", "--name-only", "--get-regexp", `^filter\..*\.(clean|smudge|process|required)$`)
		if e != nil {
			return nil, e
		}
		for _, key := range strings.Split(string(names), "\x00") {
			if key == "" {
				continue
			}
			value := ""
			if strings.HasSuffix(key, ".required") {
				value = "false"
			}
			overrides = append(overrides, "-c", key+"="+value)
		}
	}
	return readGitCommand(ctx, cwd, limit, input, overrides, args...)
}
func readGitCommand(ctx context.Context, cwd string, limit int, input io.Reader, overrides []string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	prefix := []string{"--no-pager", "--literal-pathspecs", "-c", "core.fsmonitor=false", "-C", cwd}
	if len(args) > 0 && (args[0] == "check-ignore" || args[0] == "check-attr") {
		prefix = []string{"--no-pager", "-c", "core.fsmonitor=false", "-C", cwd}
	}
	prefix = append(prefix, overrides...)
	cmd := exec.CommandContext(ctx, "git", append(prefix, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "LC_ALL=C")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.Stdin = input
	cmd.WaitDelay = time.Second
	out := &boundedOutput{limit: limit, cancel: cancel}
	stderr := &boundedOutput{limit: 8192, cancel: cancel}
	cmd.Stdout, cmd.Stderr = out, stderr
	err := session.RunCommand(cmd)
	if cmd.Process != nil && (ctx.Err() != nil || errors.Is(err, exec.ErrWaitDelay)) {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	if out.exceeded || stderr.exceeded {
		return nil, problem(413, "Git 输出超过读取限制")
	}
	if ctx.Err() != nil {
		return nil, problem(504, "Git 读取取消或超时")
	}
	if err != nil {
		var exit *exec.ExitError
		if len(args) > 0 && (args[0] == "diff" || args[0] == "check-ignore" || args[0] == "config") && errors.As(err, &exit) && exit.ExitCode() == 1 {
			return out.Bytes(), nil
		}
		return nil, fmt.Errorf("Git 读取失败: %s (%w)", strings.TrimSpace(stderr.String()), err)
	}
	return out.Bytes(), nil
}

type boundedOutput struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
	cancel   context.CancelFunc
}

func (b *boundedOutput) Bytes() []byte  { return b.buffer.Bytes() }
func (b *boundedOutput) String() string { return b.buffer.String() }
func (b *boundedOutput) Len() int       { return b.buffer.Len() }
func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		b.exceeded = true
		b.cancel()
		return 0, errors.New("output limit")
	}
	return b.buffer.Write(p)
}
