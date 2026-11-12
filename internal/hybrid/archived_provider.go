package hybrid

import (
	"fmt"
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/session"
)

// Archived providers have no actor or shell whose registry can be checked.
// ProbeShell reconstructs only their launch environment; it starts no writer.
func (s *Service) archivedProviderHeld(rec Record, environment func() (map[string]string, error)) (bool, error) {
	if rec.AgentSessionID == "" {
		return false, nil
	}
	cmd, err := s.agents.Parse(rec.Cmd)
	if err != nil {
		return false, fmt.Errorf("%w: archived provider command: %w", ErrConflict, err)
	}
	tracker, ok := cmd.Agent.(agentprofile.ProcessRegistry)
	if !ok {
		return false, nil
	}
	var env map[string]string
	if environment != nil {
		env, err = environment()
		if err != nil {
			return false, err
		}
	}
	probe, err := s.manager.ProbeShell(session.CreateInput{Cwd: rec.Cwd, Cmd: cmd.TUIArgv(), Env: env, ConfigID: rec.ConfigID})
	if err != nil {
		return false, shellError(err)
	}
	held := registeredExternalWriter(rec.AgentSessionID, agentprofile.ProcessFacts{Cwd: rec.Cwd}, tracker, probe.Getenv, s.userHome)
	return held, nil
}
