package clicmd

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/rainchan/webmux/internal/contract"
)

var p2pGroup = &Command{
	Name: "p2p", Group: GroupServer,
	Summary: "Read running P2P diagnostics or probe STUN from this CLI.",
}

const p2pLimits = "UDP4/UDP6, browser offer and server full ICE answer; no TURN. " +
	"STUN and gateway mappings do not guarantee direct connectivity; failures fall back to HTTP/WS."

var p2pStatusCommand = p2pReadCommand(false)
var p2pPeersCommand = p2pReadCommand(true)

func p2pReadCommand(peers bool) *Command {
	name, summary := "status", "Show the running server's P2P configuration, UDP bindings and gateway mapping."
	if peers {
		name, summary = "peers", "Show active peers, selected candidate metadata, RTT and stream channel counts."
	}
	return &Command{
		Name: "p2p " + name, Group: GroupServer, Summary: summary,
		Description: "Reads the running instance over its local socket, including from a session; changes no settings. " + p2pLimits +
			" Candidate metadata and RTT may be browser-reported. Missing metrics are unknown, not a healthy zero. " +
			"Channels counts stream channels, excluding control and RPC channels.",
		Output: "Readable status or peer table; --json prints the P2PStatus object or its peers array. Missing RTT is omitted.",
		Exit:   exitsCommon(ExitItem{Code: 1, Text: "Request failed or the server is too old."}, ExitItem{Code: 124, Text: "Timed out."}),
		Examples: []Example{
			{Command: "webmux p2p " + name, Text: "Read live diagnostics from the local instance."},
			{Command: "webmux p2p " + name + " --json", Text: "Read diagnostics for a script."},
		},
		SeeAlso: []string{"webmux p2p probe", "webmux guide connect"},
		Bind: func(fs *flag.FlagSet) Action {
			asJSON := fs.Bool("json", false, "print JSON")
			timeout := fs.Duration("timeout", 10*time.Second, "overall request timeout `DURATION` (greater than 0, at most 1m)")
			return func(ctx context.Context, e Env, args []string) int {
				if len(args) != 0 || *timeout <= 0 || *timeout > time.Minute {
					return e.Usagef("no positional arguments; --timeout must be greater than 0 and at most 1m")
				}
				ctx, cancel := context.WithTimeout(ctx, *timeout)
				defer cancel()
				st, code := e.p2pStatus(ctx)
				if code != contract.ExitOK {
					return code
				}
				if *asJSON {
					var out any = st
					if peers {
						out = st.Peers
					}
					return e.p2pJSON(out)
				}
				if peers {
					e.printP2PPeers(st.Peers)
				} else {
					_, _ = fmt.Fprintf(e.Stdout, "Supported: %t\nEnabled: %t\nSource: %s\nConfigured UDP port: %d (0 = automatic)\n", st.Supported, st.Enabled, st.Source, st.UDPPort)
					if len(st.UDPBindings) == 0 {
						note := "per-peer sockets may still be used"
						if !st.Supported {
							note = "WebRTC unavailable in this build"
						}
						_, _ = fmt.Fprintf(e.Stdout, "Bound ICE UDP sockets: none (%s)\n", note)
					}
					for _, b := range st.UDPBindings {
						_, _ = fmt.Fprintf(e.Stdout, "Bound ICE UDP socket: %s port %d\n", b.Network, b.Port)
					}
					_, _ = fmt.Fprintf(e.Stdout, "STUN: %s\nPort mapping enabled: %t\nGateway mapping: %s", strings.Join(st.STUNServers, ", "), st.PortMapping, st.PortMap.State)
					if st.PortMap.State == "mapped" {
						_, _ = fmt.Fprintf(e.Stdout, " (%s, external UDP port %d)", st.PortMap.Protocol, st.PortMap.ExternalPort)
					}
					_, _ = fmt.Fprintf(e.Stdout, "\nActive peers: %d\nStorage available: %t\n", st.ActivePeers, st.StorageAvailable)
					if st.StorageError != "" {
						_, _ = fmt.Fprintln(e.Stdout, "Storage error:", st.StorageError)
					}
					_, _ = fmt.Fprintf(e.Stdout, "Compiled networks: %s; ICE: %s; TURN: %t\n%s\n", valueOr(strings.Join(st.Capabilities.Networks, ", "), "none"), st.Capabilities.ICEMode, st.Capabilities.TURN, p2pStatusLimits(st.Supported))
				}
				return contract.ExitOK
			}
		},
	}
}

func (e Env) p2pStatus(ctx context.Context) (contract.P2PStatus, int) {
	client, code := e.connect()
	if client == nil {
		return contract.P2PStatus{}, code
	}
	defer client.Close()
	var st contract.P2PStatus
	if err := client.Do(ctx, http.MethodGet, contract.PathP2PDiagnostics, nil, &st); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return st, e.p2pTimeout()
		}
		return st, e.failNew(ctx, client, err)
	}
	return st, contract.ExitOK
}

func (e Env) p2pTimeout() int {
	_, _ = fmt.Fprintln(e.Stderr, "webmux: P2P diagnostics timed out")
	return contract.ExitTimeout
}

func (e Env) p2pJSON(value any) int {
	enc := json.NewEncoder(e.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return e.Fail(err)
	}
	return contract.ExitOK
}

func (e Env) printP2PPeers(peers []contract.P2PPeerInfo) {
	if len(peers) == 0 {
		_, _ = fmt.Fprintln(e.Stdout, "No active peers.")
		return
	}
	w := tabwriter.NewWriter(e.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tSTATE\tCANDIDATES\tIP FAMILY\tRTT (ms)\tSTREAM CHANNELS\tCREATED")
	for _, p := range peers {
		rtt := "unknown"
		if p.RTTMs != nil {
			rtt = fmt.Sprint(*p.RTTMs)
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%s\n", p.ID, p.State, valueOr(p.CandidateType, "unknown"), valueOr(p.IPFamily, "unknown"), rtt, p.Channels, p.CreatedAt)
	}
	_ = w.Flush()
	_, _ = fmt.Fprintln(e.Stdout, "Candidate metadata and RTT may be browser-reported; missing metrics are unknown. RTT is not necessarily ICE RTT.")
}

func p2pStatusLimits(supported bool) string {
	if !supported {
		return "WebRTC P2P is compiled out (nop2p): no ICE or direct DataChannels. HTTP/WS remains available. CLI STUN probes use separate ephemeral sockets."
	}
	return p2pLimits
}
