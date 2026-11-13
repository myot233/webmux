package hybrid

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/session"
)

const (
	// lookupTimeout bounds one connector check, startup file included.
	lookupTimeout = 10 * time.Second
	// versionTimeout bounds asking the agent executable for its version.
	versionTimeout = 5 * time.Second
	// lookupOutputBytes bounds the output a check keeps.
	lookupOutputBytes = 64 << 10
	// connectorRecheck is how often a missing connector is looked for again
	// while the terminal side runs.
	connectorRecheck = 30 * time.Second
	// exitNotFound is the shell's exit status for a command it cannot find.
	exitNotFound = 127
)

// ConnectorMissingError refuses an ACP start whose profile connector cannot
// be found where the start would look for it. No process was started.
type ConnectorMissingError struct {
	Connector *agentprofile.Connector
}

func (e *ConnectorMissingError) Error() string { return connectorText(e.Connector) }

func (e *ConnectorMissingError) Unwrap() error { return ErrConnectorMissing }

// connectorText is the user-facing text of a missing connector.
func connectorText(c *agentprofile.Connector) string {
	return "未安装 " + c.Title + "（" + c.Executable + "）"
}

// acpTools is what an ACP start needs besides its argv: the variables the
// profile adds below the launch config's, and the agent executable the
// terminal UI would run, which the connector is told to use.
type acpTools struct {
	env map[string]string
	// agentVersion is the agent executable's --version, when known.
	agentVersion string
	// agentMissing: the profile wanted the agent's path but it was not
	// found, so the connector uses its own.
	agentMissing bool
}

// prepareACP checks the profile's connector the way the ACP start will run
// it, and resolves the agent executable for the connector. A connector
// that cannot be found is a *ConnectorMissingError; a check that fails
// (timeout, broken startup file) is ErrAgent. It blocks: run it in subtasks.
func (a *actor) prepareACP(ctx context.Context) (acpTools, error) {
	return prepareACP(ctx, a.cmd, a.shell)
}

// prepareACP is the check of actor.prepareACP for command cmd run by shell,
// which may be a probe shell (session.Manager.ProbeShell).
func prepareACP(ctx context.Context, cmd agentprofile.Command, shell *session.Session) (acpTools, error) {
	p := cmd.Profile
	tools := acpTools{env: maps.Clone(p.ACPEnv)}
	if tools.env == nil {
		tools.env = make(map[string]string)
	}
	if p.Connector == nil && !p.VersionExecutable {
		return tools, nil
	}
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	agentPath := ""
	resolveAgent := p.AgentPathEnv != "" || p.VersionExecutable
	if p.AgentPathEnv != "" {
		agentPath = shell.Getenv(p.AgentPathEnv)
		resolveAgent = agentPath == ""
	}
	var connectorPath string
	if cmd.Form == agentprofile.FormStartup {
		var names []string
		if p.Connector != nil {
			names = append(names, p.Connector.Executable)
		}
		if resolveAgent {
			names = append(names, cmd.Executable)
		}
		out, _, err := shell.Output(ctx, cmd.Lookup(names...), lookupOutputBytes)
		paths, ok := lookupAnswers(out, len(names))
		if err != nil || !ok {
			if err == nil {
				err = errors.New("shell 启动文件未正常结束")
			}
			return tools, fmt.Errorf("%w: 检查 %s 失败: %w", ErrAgent, lookupTarget(p), err)
		}
		if p.Connector != nil {
			connectorPath = paths[0]
			paths = paths[1:]
		}
		if resolveAgent {
			agentPath = paths[0]
		}
	} else {
		if p.Connector != nil {
			var err error
			connectorPath, err = shell.LookPath(p.Connector.Executable)
			if err != nil && !errors.Is(err, session.ErrNoExecutable) {
				return tools, fmt.Errorf("%w: 检查 %s 失败: %w", ErrAgent, p.Connector.Executable, err)
			}
		}
		if resolveAgent {
			agentPath, _ = shell.LookPath(cmd.Executable)
		}
	}
	if p.Connector != nil && !strings.HasPrefix(connectorPath, "/") {
		return tools, &ConnectorMissingError{Connector: p.Connector}
	}
	if resolveAgent {
		if strings.HasPrefix(agentPath, "/") {
			if p.AgentPathEnv != "" {
				tools.env[p.AgentPathEnv] = agentPath
			}
		} else {
			if p.AgentPathEnv != "" {
				tools.agentMissing = true
			}
			agentPath = ""
		}
	}
	if agentPath != "" {
		tools.agentVersion = agentVersion(ctx, shell, agentPath)
	}
	return tools, nil
}

// lookupTarget names what a failed startup-file lookup was checking.
func lookupTarget(p *agentprofile.Profile) string {
	if p.Connector != nil {
		return p.Connector.Executable
	}
	return p.Name
}

// lookupAnswers reads the last n answer lines of a Lookup command; output
// the startup file printed comes before them. ok is false without n answers.
func lookupAnswers(out []byte, n int) ([]string, bool) {
	var answers []string
	for _, line := range bytes.Split(out, []byte("\n")) {
		if path, ok := bytes.CutPrefix(line, []byte(agentprofile.LookupMarker)); ok {
			answers = append(answers, string(path))
		}
	}
	if len(answers) < n {
		return nil, false
	}
	return answers[len(answers)-n:], true
}

// agentVersion asks the agent executable for its version, empty when it
// fails.
func agentVersion(ctx context.Context, shell *session.Session, path string) string {
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	out, code, err := shell.Output(ctx, []string{path, "--version"}, 4<<10)
	if err != nil || code != 0 {
		return ""
	}
	return versionField(string(out))
}

// versionField is the first version-shaped field of out's first line:
// "codex-cli 0.165.0" and "2.1.316 (Claude Code)" both name their version.
func versionField(out string) string {
	line, _, _ := strings.Cut(out, "\n")
	for _, field := range strings.Fields(line) {
		if _, ok := agentprofile.CompareVersions(field, field); ok {
			return field
		}
	}
	return ""
}

// startFailure classifies a failed ACP process start of cmd: an executable
// that cannot be found is a missing connector when the profile has one.
func startFailure(cmd agentprofile.Command, err error) *failure {
	if c := cmd.Profile.Connector; c != nil && errors.Is(err, session.ErrNoExecutable) {
		return &failure{err: &ConnectorMissingError{Connector: c}, public: connectorText(c)}
	}
	return &failure{err: fmt.Errorf("%w: %w", ErrAgent, err), public: "启动 agent 失败"}
}

// exitedNotFound reports a connector of cmd run through a startup file
// that ended with the shell's command-not-found status before answering:
// it was removed after the check.
func exitedNotFound(cmd agentprofile.Command, sidecar *session.Sidecar) *failure {
	c := cmd.Profile.Connector
	if c == nil || cmd.Form != agentprofile.FormStartup || sidecar.ExitCode() != exitNotFound {
		return nil
	}
	return &failure{err: &ConnectorMissingError{Connector: c}, public: connectorText(c)}
}

// externalWriter reports Codex refusing a session another client holds.
func externalWriter(err error) bool {
	var rpc *acp.RPCError
	if !errors.As(err, &rpc) || rpc.Code != acp.CodeInvalidRequest || len(rpc.Data) == 0 {
		return false
	}
	var data struct {
		Reason string `json:"reason"`
	}
	return json.Unmarshal(rpc.Data, &data) == nil && data.Reason == "thread_active_writer"
}

// connectorState is the eligibility check of a profile's connector while
// the terminal side runs (condition 2).
type connectorState struct {
	result  connectorResult
	running bool
	timer   *time.Timer
}

type connectorResult int

const (
	connectorUnknown connectorResult = iota
	connectorInstalled
	connectorMissing
)

// connectorReason is condition 2: connector_missing once a check found the
// profile's connector missing, else "". It starts a check when none is
// known; meanwhile the later conditions apply, and the session/list probe
// checks the connector again before it starts anything.
func (a *actor) connectorReason() string {
	if a.cmd.Profile.Connector == nil {
		return ""
	}
	switch a.connector.result {
	case connectorMissing:
		return ReasonConnectorMissing
	case connectorUnknown:
		a.checkConnector()
	}
	return ""
}

// checkConnector looks for the connector in a subtask.
func (a *actor) checkConnector() {
	if a.connector.running || a.closing || a.stopped {
		return
	}
	a.connector.running = true
	a.probes.Add(1)
	go func() {
		defer a.probes.Done()
		_, err := a.prepareACP(a.probeCtx)
		a.post(func() { a.connectorChecked(err) })
	}()
}

// connectorChecked records a check: a missing connector is looked for
// again every connectorRecheck; a check that failed lets the session
// list probe find out.
func (a *actor) connectorChecked(err error) {
	a.connector.running = false
	if errors.Is(err, context.Canceled) {
		return
	}
	a.setConnector(err)
	a.observe()
}

// setConnector applies what an ACP start or check found out about the
// connector.
func (a *actor) setConnector(err error) {
	if a.cmd.Profile.Connector == nil {
		return
	}
	if !errors.Is(err, ErrConnectorMissing) {
		if a.connector.timer != nil {
			a.connector.timer.Stop()
			a.connector.timer = nil
		}
		a.connector.result = connectorInstalled
		return
	}
	a.connector.result = connectorMissing
	if a.connector.timer == nil {
		a.connector.timer = time.AfterFunc(a.svc.timing.connectorRecheck, func() {
			a.post(func() {
				a.connector.timer = nil
				if a.connector.result == connectorMissing && a.side == SideTUI {
					a.checkConnector()
				}
			})
		})
	}
}
