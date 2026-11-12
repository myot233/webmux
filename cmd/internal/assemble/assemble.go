// Package assemble is shared wiring for the cmd/ assembly roots: it turns
// configuration into service options so no other layer depends on both.
package assemble

import (
	"net"
	"time"

	"github.com/rainchan/webmux/internal/agentprofile/agents"
	"github.com/rainchan/webmux/internal/agenttrust"
	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/p2p"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/tailscale"
	"github.com/rainchan/webmux/internal/voice"
	"github.com/rainchan/webmux/internal/wsl"
)

// SessionOptions converts the loaded configuration for session.NewManager.
func SessionOptions(c config.Config) session.Options {
	var preLaunch func(string, []string, map[string]string) map[string]string
	if c.AgentPretrust {
		preLaunch = agenttrust.New().Prepare
	}
	return session.Options{
		ScrollbackBytes: c.ScrollbackBytes,
		DefaultCwd:      c.DefaultCwd,
		IdleAfter:       time.Duration(c.IdleAfterSeconds) * time.Second,
		PromptPatterns:  c.PromptPatterns,
		ExcludePorts:    listenPorts(c.Listen),

		DisableAgentProtocol: !c.AgentProtocol,
		DisableBellAttention: !c.BellAttention,
		DisableProgramStatus: !c.ProgramStatus,
		PreLaunch:            preLaunch,
		SubmitDelay:          agents.Default().SubmitDelay,
	}
}

// listenPorts is the configured TCP port of WebMux's own listener. Port 0
// is chosen when binding, so the caller adds the bound port.
func listenPorts(listen string) []int {
	_, service, err := net.SplitHostPort(listen)
	if err != nil {
		return nil
	}
	port, err := net.LookupPort("tcp", service)
	if err != nil || port == 0 {
		return nil
	}
	return []int{port}
}

// TailscaleOptions converts [tailscale] for tailscale.New.
func TailscaleOptions(c config.Config) tailscale.Options {
	return tailscale.Options{
		Command:        c.Tailscale.Command,
		Provider:       c.Tailscale.Provider,
		IsWSL:          wsl.Detected,
		NetworkingMode: wsl.NetworkingMode,
	}
}

// VoiceOptions converts [voice] for voice.New.
func VoiceOptions(c config.Config) voice.Options {
	return voice.Options{Cmd: c.Voice.Cmd, Env: c.Voice.Env, Cwd: c.DefaultCwd}
}

// P2POptions converts [p2p] for p2p.New.
func P2POptions(c config.Config, appVersion string, stream p2p.StreamHandler, rpc p2p.RPCHandler) p2p.Options {
	return p2p.Options{
		AppVersion:    appVersion,
		Enabled:       c.P2P.Enabled,
		PortMapping:   c.P2P.PortMapping,
		STUNServers:   c.P2P.STUNServers,
		StreamHandler: stream,
		RPCHandler:    rpc,
	}
}
