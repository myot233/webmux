package config

// Field is safe diagnostic metadata. Never reflectively serialize Config:
// adding a field requires an explicit choice here and in the coverage test.
type Field struct {
	Value  any
	Source string
}

func (c *Config) SetSource(key, source string) {
	if c.Sources == nil {
		c.Sources = map[string]string{}
	}
	if source == "cli" {
		c.Sources["base:"+key] = c.Source(key)
	}
	c.Sources[key] = source
}
func (c Config) Source(key string) string {
	if s := c.Sources[key]; s != "" {
		return s
	}
	return "default"
}
func Fields(c Config) map[string]Field {
	values := map[string]any{
		"listen": c.Listen, "token": c.Token != "", "state_dir": c.StateDir, "default_cwd": c.DefaultCwd, "allowed_origins": append([]string{}, c.AllowedOrigins...),
		"history_capture": c.HistoryCapture, "handoff_token_cap": c.HandoffTokenCap, "scrollback_bytes": c.ScrollbackBytes, "idle_after_seconds": c.IdleAfterSeconds, "prompt_patterns": append([]string{}, c.PromptPatterns...), "agent_protocol": c.AgentProtocol, "bell_attention": c.BellAttention, "program_status": c.ProgramStatus, "agent_pretrust": c.AgentPretrust,
		"tunnel.enabled": c.Tunnel.Enabled, "tunnel.token": c.Tunnel.Token != "", "tunnel.hostname": c.Tunnel.Hostname, "tunnel.cloudflared": c.Tunnel.Cloudflared, "tunnel.download_mirrors": append([]string{}, c.Tunnel.DownloadMirrors...),
		"tailscale.mode": c.Tailscale.Mode, "tailscale.port": c.Tailscale.Port, "tailscale.provider": c.Tailscale.Provider, "tailscale.command": c.Tailscale.Command,
		"push.enabled": c.Push.Enabled, "push.subject": c.Push.Subject, "push.ttl_seconds": c.Push.TTLSeconds, "push.kinds": append([]string{}, c.Push.Kinds...), "push.include_title": c.Push.IncludeTitle,
		"voice.cmd": append([]string{}, c.Voice.Cmd...), "voice.env": len(c.Voice.Env) > 0,
		"p2p.enabled": c.P2P.Enabled, "p2p.port_mapping": c.P2P.PortMapping, "p2p.stun_servers": append([]string{}, c.P2P.STUNServers...),
	}
	out := map[string]Field{}
	for key, value := range values {
		out[key] = Field{Value: value, Source: c.Source(key)}
	}
	return out
}

func (c Config) BaseSource(key string) string {
	if s := c.Sources["base:"+key]; s != "" {
		return s
	}
	return c.Source(key)
}
