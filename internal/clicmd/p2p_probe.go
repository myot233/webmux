package clicmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/contract"
)

var p2pProbeCommand = &Command{
	Name: "p2p probe", Group: GroupServer,
	Summary: "Send bounded STUN/UDP binding requests from ephemeral CLI sockets.",
	Description: "Uses the running server's configured STUN list, or one explicit --server without contacting the server. " +
		"Each target gets one UDP datagram and at most 3s, within the overall timeout. " +
		"Probes this CLI's network and ephemeral ports, not the running server's ICE sockets or gateway mapping. " +
		"A valid response only verifies a STUN binding exchange with that target; it neither classifies NAT nor guarantees WebRTC traversal. " +
		"Supports stun: over UDP; stuns: (TLS/TCP) is reported unsupported. No settings are changed.",
	Output: "One success/failure per target with actual local, remote and STUN-observed mapped endpoints; --json prints a P2PProbe object even on probe failure.",
	Exit:   exitsCommon(ExitItem{Code: 1, Text: "A probe failed, was cancelled, or no targets were configured."}, ExitItem{Code: 124, Text: "The overall timeout expired."}),
	Examples: []Example{
		{Command: "webmux p2p probe --json", Text: "Probe the configured STUN targets from this CLI."},
		{Command: "webmux p2p probe --server stun:stun.cloudflare.com:3478 --network udp6 --timeout 5s", Text: "Probe one explicit STUN target using IPv6."},
	},
	SeeAlso: []string{"webmux p2p status", "webmux p2p peers", "webmux guide connect"},
	Bind: func(fs *flag.FlagSet) Action {
		asJSON := fs.Bool("json", false, "print JSON, including failed probe results")
		server := fs.String("server", "", "one explicit STUN `URI`; otherwise read the running server's configured list")
		network := fs.String("network", "udp4", "IP family: udp4 or udp6")
		timeout := fs.Duration("timeout", 10*time.Second, "overall timeout `DURATION`, including config lookup and DNS (greater than 0, at most 1m)")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 0 || *timeout <= 0 || *timeout > time.Minute || (*network != "udp4" && *network != "udp6") {
				return e.Usagef("no positional arguments; --network must be udp4 or udp6; --timeout must be greater than 0 and at most 1m")
			}
			if *server != "" {
				if _, err := stunTarget(*server); err != nil {
					return e.Usagef("--server: %v", err)
				}
			}
			ctx, cancel := context.WithTimeout(ctx, *timeout)
			defer cancel()
			servers := []string{*server}
			if *server == "" {
				st, code := e.p2pStatus(ctx)
				if code != contract.ExitOK {
					return code
				}
				servers = st.STUNServers
			}
			// Bound even responses from a mismatched/newer server.
			if len(servers) > config.MaxSTUNServers {
				return e.Fail(fmt.Errorf("at most %d STUN targets are supported", config.MaxSTUNServers))
			}
			out := contract.P2PProbe{
				Scope: "cli-ephemeral-udp",
				Limitations: []string{
					"Probes CLI network and ephemeral ports, not server ICE sockets or gateway mapping.",
					"STUN binding success does not determine NAT type or guarantee WebRTC traversal; no TURN.",
				},
				Results: make([]contract.P2PProbeResult, 0, len(servers)),
			}
			code := contract.ExitOK
			if len(servers) == 0 {
				code = contract.ExitError
			}
			for _, server := range servers {
				result := probeSTUN(ctx, server, *network)
				out.Results = append(out.Results, result)
				if !result.Success {
					code = contract.ExitError
				}
			}
			if *asJSON {
				if c := e.p2pJSON(out); c != contract.ExitOK {
					return c
				}
			} else {
				_, _ = fmt.Fprintln(e.Stdout, "Scope:", out.Scope)
				for _, line := range out.Limitations {
					_, _ = fmt.Fprintln(e.Stdout, line)
				}
				if len(servers) == 0 {
					_, _ = fmt.Fprintln(e.Stdout, "No STUN targets configured; use --server.")
				}
				for _, r := range out.Results {
					_, _ = fmt.Fprintf(e.Stdout, "%s (%s): success=%t\n", r.Server, r.Network, r.Success)
					if r.LocalAddress != "" {
						_, _ = fmt.Fprintf(e.Stdout, "  local=%s remote=%s\n", r.LocalAddress, r.RemoteAddress)
					}
					if r.Success {
						_, _ = fmt.Fprintf(e.Stdout, "  mapped=%s RTT=%.3f ms\n", r.MappedAddress, *r.RTTMs)
					} else {
						_, _ = fmt.Fprintln(e.Stdout, "  error:", r.Error)
					}
				}
			}
			// Socket deadlines can fire before the context timer publishes Err.
			// The overall deadline, not timer scheduling, determines exit 124.
			if deadline, _ := ctx.Deadline(); !time.Now().Before(deadline) {
				return e.p2pTimeout()
			}
			return code
		}
	},
}

// stunTarget reuses configuration validation, but only UDP stun: is probed.
func stunTarget(raw string) (string, error) {
	cfg, err := (config.P2PConfig{STUNServers: []string{raw}}).Validated()
	if err != nil {
		return "", err
	}
	uri := cfg.STUNServers[0]
	_, host, _ := strings.Cut(uri, ":")
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = net.JoinHostPort(strings.Trim(host, "[]"), "3478")
	} else if !strings.Contains(host, ":") {
		host = net.JoinHostPort(host, "3478")
	}
	return host, nil
}

func probeSTUN(parent context.Context, server, network string) contract.P2PProbeResult {
	out := contract.P2PProbeResult{Server: server, Network: network}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	err := func() error {
		target, err := stunTarget(server)
		if err != nil {
			return err
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(server)), "stuns:") {
			return errors.New("stuns: requires TLS/TCP; this probe supports only stun: over UDP")
		}
		conn, err := (&net.Dialer{}).DialContext(ctx, network, target)
		if err != nil {
			return fmt.Errorf("dial STUN: %w", err)
		}
		defer func() { _ = conn.Close() }()
		out.LocalAddress, out.RemoteAddress = conn.LocalAddr().String(), conn.RemoteAddr().String()
		deadline, _ := ctx.Deadline()
		if err := conn.SetDeadline(deadline); err != nil {
			return err
		}
		// Wake blocked I/O on cancellation; Close is safe concurrently and no
		// goroutine outlives a socket operation with unbounded work.
		closed := make(chan struct{})
		stop := context.AfterFunc(ctx, func() { _ = conn.Close(); close(closed) })
		defer func() {
			if !stop() {
				<-closed
			}
		}()
		request, err := stunBindingRequest()
		if err != nil {
			return err
		}
		start := time.Now()
		if _, err := conn.Write(request); err != nil {
			return fmt.Errorf("send STUN: %w", err)
		}
		response := make([]byte, 2048)
		n, err := conn.Read(response)
		if err != nil {
			return fmt.Errorf("read STUN: %w", err)
		}
		mapped, err := stunMappedAddress(response[:n], request)
		if err != nil {
			return err
		}
		rtt := float64(time.Since(start)) / float64(time.Millisecond)
		out.MappedAddress, out.RTTMs, out.Success = mapped, &rtt, true
		return nil
	}()
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		out.Error = err.Error()
	}
	return out
}
