package workspace

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Remote call outcomes, shared by fetch and push results.
const (
	remoteOK       = "ok"
	remoteAuth     = "auth"
	remoteNetwork  = "network"
	remoteTimeout  = "timeout"
	remoteRejected = "rejected"
	remoteError    = "error"
)

// maxRemoteMessage bounds a RemoteFailure message, in characters.
const maxRemoteMessage = 200

// batchSSHCommand makes ssh fail instead of waiting for a passphrase or a
// host key confirmation the server has no terminal for.
const batchSSHCommand = "GIT_SSH_COMMAND=ssh -o BatchMode=yes"

// RemoteFailure is a failed Git call that reached a remote: Status is one
// of auth, network, timeout, rejected or error, and Message is a fixed
// explanation followed by the redacted first line of Git's output.
type RemoteFailure struct {
	Status  string
	Message string
}

// remoteRule maps Git's fixed English output (LC_ALL=C) to a status. The
// first rule with a matching keyword wins.
type remoteRule struct {
	status   string
	text     string
	keywords []string
}

var remoteRules = []remoteRule{
	{remoteAuth, "主机密钥未受信任，请先在终端连接一次并确认指纹", []string{"Host key verification failed"}},
	{remoteAuth, "认证失败，请在终端完成认证后重试", []string{"Authentication failed", "could not read Username", "Permission denied (publickey)"}},
	{remoteNetwork, "网络不可达，请检查网络与远程地址", []string{"Could not resolve host", "Connection refused", "Network is unreachable"}},
	{remoteRejected, "远端拒绝了请求", []string{"rejected", "does not appear to be a git repository"}},
}

const (
	remoteTimeoutText = "远程操作超时，请稍后重试"
	remoteErrorText   = "Git 远程操作失败"
)

// urlPattern finds URLs in Git output; their credentials are redacted.
var urlPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://\S+`)

// remoteGit runs one Git call that talks to a remote. On top of the usual
// noninteractive environment it keeps ssh from prompting, and it turns a
// failure into a RemoteFailure that is safe to show and to log.
func (s *Service) remoteGit(ctx context.Context, cwd string, args ...string) ([]byte, *RemoteFailure) {
	stdout, _, failure := s.remoteGitOutput(ctx, cwd, args...)
	if failure != nil {
		return nil, failure
	}
	return stdout, nil
}

// remoteGitOutput is remoteGit that also returns, on failure too, what Git
// wrote, for callers that parse it further. Both outputs are raw: they can
// hold credentials and must not be shown or logged as they are.
func (s *Service) remoteGitOutput(ctx context.Context, cwd string, args ...string) ([]byte, string, *RemoteFailure) {
	if s.gitPath == "" {
		return nil, "", &RemoteFailure{Status: remoteError, Message: remoteMessage(remoteErrorText, s.Capability().GitError)}
	}
	stdout, run := s.runGit(ctx, cwd, s.remoteEnv(ctx, cwd), args...)
	if run.err == nil {
		return stdout.Bytes(), run.stderr, nil
	}
	failure := classifyRemote(run, stdout.exceeded)
	return stdout.Bytes(), run.stderr, &failure
}

// remoteEnv never asks an askpass program, and puts ssh in batch mode
// unless the user chose an ssh command: GIT_SSH_COMMAND, core.sshCommand
// and GIT_SSH (Git's order) each replace the injected one.
func (s *Service) remoteEnv(ctx context.Context, cwd string) []string {
	env := []string{"SSH_ASKPASS_REQUIRE=never"}
	for _, name := range []string{"GIT_SSH_COMMAND", "GIT_SSH"} {
		if _, set := os.LookupEnv(name); set {
			return env
		}
	}
	if s.sshCommandConfigured(ctx, cwd) {
		return env
	}
	return append(env, batchSSHCommand)
}

// sshCommandConfigured reports whether core.sshCommand is set for cwd. A
// read that fails for another reason than "not set" counts as set, so a
// user's ssh command is never overridden by mistake.
func (s *Service) sshCommandConfigured(ctx context.Context, cwd string) bool {
	_, run := s.runGit(ctx, cwd, nil, "config", "--get", "core.sshCommand")
	var exit *exec.ExitError
	unset := errors.As(run.err, &exit) && exit.ExitCode() == 1
	return !unset
}

func classifyRemote(run gitExit, stdoutExceeded bool) RemoteFailure {
	line := firstLine(run.stderr)
	switch {
	case stdoutExceeded || run.stderrExceeded:
		return RemoteFailure{Status: remoteError, Message: remoteMessage("Git 输出超过限制", "")}
	case run.ctxErr != nil:
		return RemoteFailure{Status: remoteTimeout, Message: remoteMessage(remoteTimeoutText, line)}
	}
	var launch *exec.Error
	if errors.As(run.err, &launch) || errors.Is(run.err, os.ErrNotExist) || errors.Is(run.err, os.ErrPermission) {
		return RemoteFailure{Status: remoteError, Message: remoteMessage("Git 无法执行", "")}
	}
	status, text := classifyRemoteOutput(run.stderr)
	return RemoteFailure{Status: status, Message: remoteMessage(text, line)}
}

// classifyRemoteOutput finds the status and fixed explanation of a failed
// remote call's output.
func classifyRemoteOutput(output string) (string, string) {
	for _, rule := range remoteRules {
		for _, keyword := range rule.keywords {
			if strings.Contains(output, keyword) {
				return rule.status, rule.text
			}
		}
	}
	return remoteError, remoteErrorText
}

// firstLine is the first non-blank line of Git's output; a progress line
// keeps only its last carriage-return segment.
func firstLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if i := strings.LastIndexByte(strings.TrimRight(line, "\r"), '\r'); i >= 0 {
			line = line[i+1:]
		}
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// remoteMessage joins text and the redacted Git line, at most
// maxRemoteMessage characters.
func remoteMessage(text, line string) string {
	message := text
	if line = redactText(line); line != "" {
		message += "：" + line
	}
	if runes := []rune(message); len(runes) > maxRemoteMessage {
		message = string(runes[:maxRemoteMessage-1]) + "…"
	}
	return message
}

// redactText hides the credentials of every URL in text.
func redactText(text string) string {
	return urlPattern.ReplaceAllStringFunc(text, redactURL)
}
