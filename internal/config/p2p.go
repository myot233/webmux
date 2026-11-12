package config

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
)

// P2PConfig controls opportunistic WebRTC DataChannel direct upgrades.
type P2PConfig struct {
	Enabled     bool     `toml:"enabled"`
	PortMapping bool     `toml:"port_mapping"`
	STUNServers []string `toml:"stun_servers"`
}

const MaxSTUNServers = 8

// DefaultSTUNServers returns the built-in public STUN servers.
func DefaultSTUNServers() []string {
	return []string{
		"stun:stun.miwifi.com:3478",
		"stun:stun.chat.bilibili.com:3478",
		"stun:stun.cloudflare.com:3478",
		"stun:stun.l.google.com:19302",
	}
}

// DefaultP2PConfig returns the default [p2p] configuration.
func DefaultP2PConfig() P2PConfig {
	return P2PConfig{
		Enabled:     true,
		PortMapping: true,
		STUNServers: DefaultSTUNServers(),
	}
}

// Validated returns a validated, normalized copy of p.
func (p P2PConfig) Validated() (P2PConfig, error) {
	out := P2PConfig{
		Enabled:     p.Enabled,
		PortMapping: p.PortMapping,
		STUNServers: slices.Clone(p.STUNServers),
	}
	if err := out.validate(); err != nil {
		return P2PConfig{}, err
	}
	return out, nil
}

func (p *P2PConfig) validate() error {
	if p.STUNServers == nil {
		p.STUNServers = DefaultSTUNServers()
	}
	if len(p.STUNServers) > MaxSTUNServers {
		return fmt.Errorf("p2p.stun_servers: at most %d servers are allowed", MaxSTUNServers)
	}
	normalized := make([]string, 0, len(p.STUNServers))
	seen := map[string]bool{}
	for _, raw := range p.STUNServers {
		u, err := normalizeSTUNURI(raw)
		if err != nil {
			return fmt.Errorf("p2p.stun_servers: %w", err)
		}
		if seen[u] {
			return fmt.Errorf("p2p.stun_servers: %q is listed twice", u)
		}
		seen[u] = true
		normalized = append(normalized, u)
	}
	p.STUNServers = normalized
	return nil
}

func normalizeSTUNURI(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("STUN server URI must not be empty")
	}
	if strings.ContainsAny(trimmed, "/?#@ \t\r\n") {
		return "", fmt.Errorf("%q must be stun:host[:port] or stuns:host[:port] without path, query or credentials", raw)
	}
	scheme, rest, ok := strings.Cut(trimmed, ":")
	if !ok {
		return "", fmt.Errorf("%q must use stun or stuns scheme", raw)
	}
	scheme = strings.ToLower(scheme)
	if scheme != "stun" && scheme != "stuns" {
		return "", fmt.Errorf("%q scheme must be stun or stuns", raw)
	}
	if rest == "" {
		return "", fmt.Errorf("%q is missing a host", raw)
	}
	host := rest
	port := ""
	if strings.HasPrefix(rest, "[") {
		if strings.HasSuffix(rest, "]") {
			host = strings.TrimSuffix(strings.TrimPrefix(rest, "["), "]")
		} else {
			h, p, err := net.SplitHostPort(rest)
			if err != nil {
				return "", fmt.Errorf("%q has an invalid host or port", raw)
			}
			host, port = h, p
		}
	} else if strings.Count(rest, ":") == 1 {
		h, p, err := net.SplitHostPort(rest)
		if err != nil {
			return "", fmt.Errorf("%q has an invalid host or port", raw)
		}
		host, port = h, p
	} else if strings.Count(rest, ":") > 1 {
		return "", fmt.Errorf("%q IPv6 host must be bracketed as [addr] or [addr]:port", raw)
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return "", fmt.Errorf("%q is missing a host", raw)
	}
	if ip := net.ParseIP(host); ip == nil {
		for _, r := range host {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '.' {
				return "", fmt.Errorf("%q has an invalid hostname", raw)
			}
		}
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("%q port must be between 1 and 65535", raw)
		}
		return scheme + ":" + net.JoinHostPort(host, strconv.Itoa(n)), nil
	}
	if strings.Contains(host, ":") {
		return scheme + ":[" + host + "]", nil
	}
	return scheme + ":" + host, nil
}
