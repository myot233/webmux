package clicmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rainchan/webmux/internal/attach"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
	"github.com/rainchan/webmux/internal/sessionobserve"
)

// sessionStates are the values of --until and --wait.
var sessionStates = []string{"running", "waiting", "idle", "exited", "attention"}

// parseStates splits a comma-separated list of states.
func parseStates(value string) ([]string, error) {
	var out []string
	for part := range strings.SplitSeq(value, ",") {
		part = strings.TrimSpace(part)
		if !slices.Contains(sessionStates, part) {
			return nil, fmt.Errorf("unknown state %q (states: %s)", part, strings.Join(sessionStates, ", "))
		}
		out = append(out, part)
	}
	return out, nil
}

// inState reports whether s is in any of states.
func inState(s contract.SessionInfo, states []string, exited bool) bool {
	for _, state := range states {
		if state == "exited" {
			if exited {
				return true
			}
			continue
		}
		if state == "attention" && s.Attention != nil {
			return true
		}
		if string(s.Status) == state {
			return true
		}
	}
	return false
}

// errTimeout ends a wait that ran out of time.
var errTimeout = errors.New("timed out")

// sessionWatch owns CLI-only multi-session and timing policies; the shared
// subscription owns list facts, missing/removed targets and exit decisions.
type sessionWatch struct {
	subscription *sessionobserve.SessionSubscription
}

func watchSessions(ctx context.Context, client *localclient.Client) (*sessionWatch, error) {
	subscription, err := sessionobserve.OpenSessions(ctx, client, 64<<20)
	if err != nil {
		return nil, err
	}
	return &sessionWatch{subscription: subscription}, nil
}

// waitOptions say when waitFor returns.
type waitOptions struct {
	ids    []string
	states []string
	// changed ignores the state at subscription: only later events count.
	changed bool
	// stable requires the condition to hold this long.
	stable time.Duration
	// any returns when one session matches instead of all.
	any bool
	// primed means the subscription already read its snapshot: the
	// current sessions are the baseline.
	primed bool
}

// waitFor waits on an open subscription until opts hold, ctx ends (with
// errTimeout when its deadline passed) or a session is removed. It returns
// the matching sessions' latest info.
func (w *sessionWatch) waitFor(ctx context.Context, opts waitOptions) ([]contract.SessionInfo, error) {
	seenSnapshot := false
	var since time.Time
	baseline := map[string]string{}
	if opts.primed {
		for _, id := range opts.ids {
			s, err := w.subscription.Session(id)
			if err != nil {
				return nil, err
			}
			baseline[id] = stateKey(s)
		}
		seenSnapshot = true
	}
	for {
		readCtx := ctx
		var cancel context.CancelFunc
		if !since.IsZero() {
			readCtx, cancel = context.WithDeadline(ctx, since.Add(opts.stable))
		}
		event, err := w.subscription.Read(readCtx)
		if cancel != nil {
			cancel()
		}
		stableElapsed := err != nil && !since.IsZero() && ctx.Err() == nil && time.Since(since) >= opts.stable
		if err != nil && !stableElapsed {
			if ctx.Err() != nil {
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					return nil, errTimeout
				}
				return nil, ctx.Err()
			}
			return nil, err
		}
		if err == nil {
			if event.Type == "removed" && slices.Contains(opts.ids, event.ID) {
				_, err := w.subscription.Session(event.ID)
				return nil, err
			}
			if event.Type == "snapshot" {
				for _, id := range opts.ids {
					s, err := w.subscription.Session(id)
					if err != nil {
						return nil, err
					}
					baseline[id] = stateKey(s)
				}
				seenSnapshot = true
			}
		}
		if !seenSnapshot {
			continue
		}
		matched := w.matching(opts, baseline)
		done := len(matched) == len(opts.ids) || (opts.any && len(matched) > 0)
		switch {
		case !done:
			since = time.Time{}
		case opts.stable <= 0 || stableElapsed:
			return matched, nil
		case since.IsZero():
			since = time.Now()
		}
	}
}

// stateKey identifies the state a --changed wait must see change.
func stateKey(s contract.SessionInfo) string {
	key := string(s.Status)
	if s.Attention != nil {
		key += "+" + string(s.Attention.Kind)
	}
	return key
}

func (w *sessionWatch) matching(opts waitOptions, baseline map[string]string) []contract.SessionInfo {
	var out []contract.SessionInfo
	for _, id := range opts.ids {
		s, exited, err := w.subscription.Exit(id)
		if err != nil {
			continue
		}
		// --changed: the state must differ from the one at subscription.
		// Once any other state has been seen the baseline no longer applies,
		// so a round trip (waiting, running, waiting) counts as a change.
		if opts.changed && baseline[id] != "" && baseline[id] != stateKey(s) {
			baseline[id] = ""
		}
		if !inState(s, opts.states, exited) || opts.changed && baseline[id] == stateKey(s) {
			continue
		}
		if opts.changed {
			baseline[id] = ""
		}
		out = append(out, s)
	}
	return out
}

// observer is a terminal for attach.Run that never reads input and writes
// the session's output to w.
type observer struct {
	w io.Writer
}

func (o observer) ReadContext(ctx context.Context, _ []byte) (int, error) {
	<-ctx.Done()
	return 0, ctx.Err()
}
func (o observer) Write(p []byte) (int, error) { return o.w.Write(p) }
func (o observer) Size() (int, int, error)     { return 80, 24, nil }

// followTerminal streams session id's replay and output to w as an
// observer, without control or resizing, until the session exits or ctx
// ends.
func followTerminal(ctx context.Context, client *localclient.Client, id string, w io.Writer, label string) (attach.Result, error) {
	return attach.Run(ctx, client, id, observer{w: w}, attach.Options{Observe: true, Label: label})
}

// stripper removes terminal control sequences from a stream and keeps
// line text: CRLF becomes LF, a lone CR starts the line over, backspace
// deletes a rune. It is best effort and unsuitable for full-screen
// programs.
type stripper struct {
	w     io.Writer
	state int
	line  []byte
	cr    bool
	// skip counts the bytes of a charset designation still to drop.
	skip int
}

const (
	stripText = iota
	stripEscape
	stripCSI
	stripString // OSC, DCS, SOS, PM, APC: up to BEL or ESC \
	stripStringEscape
)

func newStripper(w io.Writer) *stripper { return &stripper{w: w} }

func (s *stripper) Write(p []byte) (int, error) {
	var out bytes.Buffer
	for _, b := range p {
		if s.skip > 0 {
			s.skip--
			continue
		}
		switch s.state {
		case stripEscape:
			switch b {
			case '[':
				s.state = stripCSI
			case ']', 'P', 'X', '^', '_':
				s.state = stripString
			case '(', ')', '*', '+', '-', '.', '/', '#', '%':
				s.state, s.skip = stripText, 1
			default:
				s.state = stripText
			}
			continue
		case stripCSI:
			if b >= 0x40 && b <= 0x7e {
				s.state = stripText
			}
			continue
		case stripString:
			switch b {
			case 0x07:
				s.state = stripText
			case 0x1b:
				s.state = stripStringEscape
			}
			continue
		case stripStringEscape:
			if b == '\\' {
				s.state = stripText
			} else {
				s.state = stripString
			}
			continue
		}
		if s.cr && b != '\n' && b != '\r' {
			// A lone CR: the line is written over from its start. The PTY
			// turns "\r\n" into "\r\r\n", so repeated CRs wait for what
			// follows them.
			s.line = s.line[:0]
		}
		s.cr = false
		switch {
		case b == 0x1b:
			s.state = stripEscape
		case b == '\r':
			s.cr = true
		case b == '\n':
			out.Write(s.line)
			out.WriteByte('\n')
			s.line = s.line[:0]
		case b == '\b':
			if len(s.line) > 0 {
				_, size := utf8.DecodeLastRune(s.line)
				s.line = s.line[:len(s.line)-size]
			}
		case b == '\t' || b >= 0x20 && b != 0x7f:
			s.line = append(s.line, b)
		}
	}
	if out.Len() > 0 {
		if _, err := s.w.Write(out.Bytes()); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// Flush writes the unfinished last line.
func (s *stripper) Flush() error {
	if len(s.line) == 0 {
		return nil
	}
	_, err := s.w.Write(s.line)
	s.line = s.line[:0]
	return err
}
