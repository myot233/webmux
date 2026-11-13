package hybrid

import (
	"context"
	"slices"
	"strconv"
	"time"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/session"
)

// optionsProbeTimeout bounds one agent options probe, from the connector
// check to session/new: agents can take seconds to open a session.
const optionsProbeTimeout = 45 * time.Second

// ProbeAgentOptions starts a temporary ACP process for a chat-first start
// of in, opens a new agent session in in.Cwd only to read the options the
// agent offers, then closes it (session/close when supported) and ends the
// process. No WebMux session or record is created; the options are
// remembered for in.ConfigID as a chat session's report would be. It
// shares the eligibility probes' limit (maxProbes). A missing connector is
// a *ConnectorMissingError, a failed agent call ErrAgent.
func (s *Service) ProbeAgentOptions(ctx context.Context, in CreateInput) ([]ConfigOption, error) {
	in.StartSide = SideACP
	cmd, _, cwd, err := s.admit(in)
	if err != nil {
		return nil, err
	}
	select {
	case s.probes <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-s.probes }()
	ctx, cancel := context.WithTimeout(ctx, optionsProbeTimeout)
	defer cancel()
	shell, err := s.manager.ProbeShell(session.CreateInput{Cwd: cwd, Cmd: cmd.TUIArgv(), Env: in.Env, ConfigID: in.ConfigID})
	if err != nil {
		return nil, shellError(err)
	}
	tools, err := prepareACP(ctx, cmd, shell)
	if err != nil {
		return nil, err
	}
	argv, _ := cmd.ACPArgv()
	sidecar, err := shell.StartSidecar(argv, cmd.ProtocolOnFD3(), tools.env)
	if err != nil {
		return nil, startFailure(cmd, err).err
	}
	link := &acpLink{sidecar: sidecar}
	link.conn = acp.NewConn(sidecar.Stdout(), sidecar.Stdin(), acp.Options{Logger: s.log})
	defer func() { _ = link.release() }()
	init, err := link.conn.Initialize(ctx, acp.Implementation{Name: "webmux", Title: "WebMux", Version: s.version}, nil)
	if err != nil {
		s.log.Warn("hybrid options probe failed", "config", in.ConfigID, "method", acp.MethodInitialize, "error", err)
		_ = link.stop()
		if missing := exitedNotFound(cmd, sidecar); missing != nil {
			return nil, missing.err
		}
		return nil, agentFailure(acp.MethodInitialize, err, sidecar).err
	}
	link.caps = init.AgentCapabilities.SessionCapabilities
	res, err := link.conn.NewSession(ctx, acp.NewSessionRequest{Cwd: cwd})
	if err != nil {
		s.log.Warn("hybrid options probe failed", "config", in.ConfigID, "method", acp.MethodSessionNew, "error", err)
		return nil, agentFailure(acp.MethodSessionNew, err, sidecar).err
	}
	link.sessionID = res.SessionID
	options := configOptions(res.ConfigOptions, cmd.Profile.ModelChoiceValues)
	applySeeds(options, cmd)
	s.options.remember(in.ConfigID, options, time.Now())
	return options, nil
}

// applySeeds folds the chat-side values derived from the launch arguments
// into the probed options, so a picker that has no session yet defaults to
// what a fresh chat of this config would open with.
func applySeeds(options []ConfigOption, cmd agentprofile.Command) {
	seeds, _ := cmd.ChatConfig()
	for _, seed := range seeds {
		for i := range options {
			option := &options[i]
			if option.ID != seed.ConfigID {
				continue
			}
			if option.Enabled != nil {
				if enabled, err := strconv.ParseBool(seed.Value); err == nil {
					*option.Enabled = enabled
				}
				break
			}
			if slices.ContainsFunc(option.Options, func(c ConfigChoice) bool {
				return c.Value == seed.Value
			}) {
				option.Value = seed.Value
			}
			break
		}
	}
}
