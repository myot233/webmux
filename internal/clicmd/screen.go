package clicmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/0magnet/xterm-go/vt"
	"github.com/coder/websocket"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

// screenView is the --json form of webmux screen.
type screenView struct {
	Cols      int      `json:"cols"`
	Rows      int      `json:"rows"`
	Cursor    position `json:"cursor"`
	AltScreen bool     `json:"altScreen"`
	Stable    bool     `json:"stable"`
	Lines     []string `json:"lines"`
}

type position struct {
	Row int `json:"row"`
	Col int `json:"col"`
}

// maxScreenScrollback bounds --scrollback: the emulator allocates its
// history up front.
const maxScreenScrollback = 100000

var screenCommand = &Command{
	Name: "screen", Group: GroupSessions, Args: "SESSION",
	Summary: "Print the text a terminal session shows right now, rendered on the client.",
	Description: "Connects as an observer, replays the session into a terminal emulator inside this command at the controller's size, " +
		"and prints the visible screen; the server does no terminal emulation. Use it to see a full-screen TUI's state, such as an " +
		"agent waiting on a permission prompt. A truncated replay may leave the screen incomplete; full-screen programs usually redraw.\n\n" +
		"The command does not wait forever for a quiet screen: it returns once the session has had no output for 150 ms, or after one second at the latest. " +
		"A program that keeps redrawing (an animated spinner) yields a snapshot flagged unstable; use logs or the chat view for those.\n\n" +
		"--scrollback N also prints up to N lines of scroll history above the screen. The history comes from the session's 2 MiB " +
		"output buffer, replayed at the controller's current width, so it holds only what that buffer still has and is cut off the same way " +
		"as a truncated replay. A program on the alternate screen (vim, less, most TUIs) has no scroll history: only the visible screen is printed.",
	Arguments: []Item{sessionArgument},
	Env:       []Item{sessionIDEnv},
	Output: "The visible lines without trailing blanks, preceded by the scroll history with --scrollback. With --json, " +
		"{cols, rows, cursor: {row, col}, altScreen, stable, lines}, where lines includes the history and stable tells whether the screen had settled. " +
		"--cursor writes the 1-based cursor position to standard error. If output is still arriving one second after the replay, the snapshot is printed anyway, " +
		"stable is false and a note goes to standard error; the exit status stays 0.",
	Exit: exitsCommon(ExitItem{Code: 1, Text: "No such session, or it is on its chat view."}),
	Examples: []Example{
		{Command: "webmux screen abcd2345", Text: "See what an agent's terminal shows."},
		{Command: "webmux screen abcd2345 --json | jq -r '.lines[-1]'", Text: "Read the last line of the screen."},
		{Command: "webmux screen abcd2345 --scrollback 200", Text: "See the last 200 lines that scrolled off the screen, then the screen."},
	},
	SeeAlso: []string{"webmux logs", "webmux send", "webmux chat status"},
	Bind: func(fs *flag.FlagSet) Action {
		asJSON := fs.Bool("json", false, "print size, cursor, alternate-screen state and lines as JSON")
		cursor := fs.Bool("cursor", false, "write the cursor position to standard error")
		scrollback := fs.Int("scrollback", 0, "also print up to `N` lines of scroll history above the screen (0-100000; none on the alternate screen)")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 1 {
				return e.Usagef("exactly one session ID is required")
			}
			id, code, ok := e.SessionID(args[0])
			if !ok {
				return code
			}
			if *scrollback < 0 || *scrollback > maxScreenScrollback {
				return e.Usagef("--scrollback must be 0-%d", maxScreenScrollback)
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			view, err := e.renderScreen(ctx, client, id, *scrollback)
			if err != nil {
				return e.Fail(err)
			}
			if *cursor {
				_, _ = fmt.Fprintf(e.Stderr, "cursor %d,%d\n", view.Cursor.Row, view.Cursor.Col)
			}
			if *asJSON {
				writeJSON(e.Stdout, view)
				return contract.ExitOK
			}
			w := bufio.NewWriter(e.Stdout)
			for _, line := range view.Lines {
				_, _ = w.WriteString(line + "\n")
			}
			_ = w.Flush()
			return contract.ExitOK
		}
	},
}

// screenQuiet is how long without output counts as a settled screen.
const screenQuiet = 150 * time.Millisecond

// screenSettleMax bounds how long screen keeps reading after the replay: a
// TUI that animates never goes quiet, so the screen is printed as received.
const screenSettleMax = time.Second

func (e Env) renderScreen(ctx context.Context, client *localclient.Client, id string, scrollback int) (screenView, error) {
	query := url.Values{"label": {e.scriptLabel()}}
	conn, err := client.DialWebSocket(ctx, "/ws/s/"+url.PathEscape(id)+"?"+query.Encode())
	if err != nil {
		return screenView{}, err
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(64 << 20)
	var pending [][]byte
	cols, rows, truncated := 0, 0, false
	gotSnapshot, stable := false, true
	var settleBy time.Time
	for {
		readCtx := ctx
		var cancel context.CancelFunc
		capped := false
		if gotSnapshot && cols > 0 {
			wait := screenQuiet
			if left := time.Until(settleBy); left <= 0 {
				stable = false
				break
			} else if left < wait {
				wait, capped = left, true
			}
			readCtx, cancel = context.WithTimeout(ctx, wait)
		}
		_, frame, err := conn.Read(readCtx)
		if cancel != nil {
			cancel()
		}
		if err != nil {
			if gotSnapshot && ctx.Err() == nil {
				// A deadline can surface as a closed-socket error.
				// Reaching the hard cap is not evidence of a quiet screen.
				stable = !capped || time.Now().Before(settleBy)
				break
			}
			switch websocket.CloseStatus(err) {
			case contract.CloseMoved:
				return screenView{}, errors.New("the session is on its chat side; use webmux chat status")
			case 4404:
				return screenView{}, errors.New("session not found")
			}
			return screenView{}, err
		}
		if len(frame) == 0 {
			continue
		}
		switch frame[0] {
		case contract.FrameSnapshot:
			pending = [][]byte{frame[1:]}
			if !gotSnapshot {
				gotSnapshot, settleBy = true, time.Now().Add(screenSettleMax)
			}
		case contract.FrameOutput:
			pending = append(pending, frame[1:])
		case contract.FramePosition:
			var p contract.StreamPosition
			if json.Unmarshal(frame[1:], &p) == nil && p.Truncated {
				truncated = true
			}
		case contract.FrameState:
			var info contract.SessionInfo
			if json.Unmarshal(frame[1:], &info) == nil && info.Cols > 0 {
				cols, rows = int(info.Cols), int(info.Rows)
			}
		case contract.FrameExit:
			if !gotSnapshot {
				gotSnapshot, settleBy = true, time.Now().Add(screenSettleMax)
			}
		}
	}
	if !stable {
		_, _ = fmt.Fprintf(e.Stderr, "webmux screen: output was still arriving after %s; this is a snapshot of a screen that kept changing, not a settled one\n", screenSettleMax)
	}
	if truncated {
		_, _ = fmt.Fprintln(e.Stderr, "webmux screen: the replay was truncated; the screen may be incomplete")
	}
	if cols == 0 {
		cols, rows = 80, 24
	}
	options := vt.NewOptions()
	options.Cols, options.Rows, options.Scrollback = cols, rows, scrollback
	term := vt.NewTerminal(options)
	for _, chunk := range pending {
		term.Write(chunk)
	}
	buffer := term.Buffer()
	view := screenView{
		Cols: cols, Rows: rows, AltScreen: buffer == term.Buffers().Alt(), Stable: stable,
		Cursor: position{Row: buffer.Y + 1, Col: buffer.X + 1},
	}
	// Without scrollback the screen is the last rows lines; with it the
	// history above them comes first.
	first := buffer.YBase
	if scrollback > 0 {
		first = 0
	}
	for y := first; y < buffer.YBase+rows; y++ {
		view.Lines = append(view.Lines, strings.TrimRight(buffer.TranslateBufferLineToString(y, true, 0, cols), " "))
	}
	for len(view.Lines) > 0 && view.Lines[len(view.Lines)-1] == "" {
		view.Lines = view.Lines[:len(view.Lines)-1]
	}
	if view.Lines == nil {
		view.Lines = []string{}
	}
	return view, nil
}

var logsCommand = &Command{
	Name: "logs", Group: GroupSessions, Args: "SESSION",
	Summary: "Print a session's terminal output; --follow keeps streaming as an observer.",
	Description: "Prints the session's retained output (GET /api/sessions/{id}/buffer) as raw bytes. --strip removes terminal control " +
		"sequences and keeps line text, which suits line-oriented programs but not full-screen TUIs; use webmux screen for those. " +
		"--follow connects as an observer, prints the replay and then new output, never taking control or resizing, until the session exits.",
	Arguments: []Item{sessionArgument},
	Env:       []Item{sessionIDEnv},
	Output:    "The output bytes, or plain text with --strip. With --follow, the exit code of the session goes to standard error.",
	Exit:      exitsCommon(ExitItem{Code: 1, Text: "No such session, or following was interrupted."}),
	Examples: []Example{
		{Command: "webmux logs abcd2345 --strip --tail 50", Text: "Show the last 50 lines as plain text."},
		{Command: "webmux logs self --follow --strip | grep --line-buffered ERROR", Text: "Watch a session's output for errors."},
	},
	SeeAlso: []string{"webmux screen", "webmux attach --observe"},
	Notes:   map[string]Note{"tail": {Conflicts: []string{"follow"}}, "follow": {Conflicts: []string{"tail"}}},
	Bind: func(fs *flag.FlagSet) Action {
		follow := fs.Bool("follow", false, "keep printing new output until the session exits")
		strip := fs.Bool("strip", false, "remove terminal control sequences")
		tail := fs.Int("tail", 0, "print only the last `N` lines")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 1 {
				return e.Usagef("exactly one session ID is required")
			}
			if *follow && *tail > 0 {
				return e.Usagef("--tail cannot be combined with --follow")
			}
			if *tail < 0 {
				return e.Usagef("--tail must not be negative")
			}
			id, code, ok := e.SessionID(args[0])
			if !ok {
				return code
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			out := e.Stdout
			var s *stripper
			if *strip {
				s = newStripper(out)
				out = s
			}
			if *follow {
				result, err := followTerminal(ctx, client, id, out, e.scriptLabel())
				if s != nil {
					_ = s.Flush()
				}
				if err != nil {
					return e.Fail(err)
				}
				if result.Exited {
					_, _ = fmt.Fprintf(e.Stderr, "webmux logs: session %s exited with %d\n", id, result.ExitCode)
				}
				return contract.ExitOK
			}
			data, err := client.Get(ctx, "/api/sessions/"+url.PathEscape(id)+"/buffer")
			if err != nil {
				return e.Fail(err)
			}
			if *tail == 0 {
				_, _ = out.Write(data)
				if s != nil {
					_ = s.Flush()
				}
				return contract.ExitOK
			}
			var text bytes.Buffer
			if s != nil {
				t := newStripper(&text)
				_, _ = t.Write(data)
				_ = t.Flush()
			} else {
				text.Write(data)
			}
			_, _ = e.Stdout.Write(lastLines(text.Bytes(), *tail))
			return contract.ExitOK
		}
	},
}

var eventsCommand = &Command{
	Name: "events", Group: GroupSessions,
	Summary: "Print session list events as NDJSON until interrupted.",
	Description: "Each /ws/sessions event (snapshot, created, updated, removed, forwards) is written as one JSON line, unchanged. " +
		"With --session, only events about those sessions are written and the snapshot keeps only them. After a lost connection " +
		"the command reconnects and writes a new snapshot.",
	Output: "One JSON object per line.",
	Exit:   exitsCommon(ExitItem{Code: 1, Text: "The connection failed."}),
	Examples: []Example{
		{Command: `webmux events | jq -c 'select(.type=="updated") | {id: .session.id, status: .session.status}'`, Text: "Print every status change."},
		{Command: "webmux events --session abcd2345", Text: "Follow one session's events."},
	},
	SeeAlso: []string{"webmux wait"},
	Notes:   map[string]Note{"session": {Repeatable: true}},
	Bind: func(fs *flag.FlagSet) Action {
		var only listFlag
		fs.Var(&only, "session", "only events about `SESSION` (or self)")
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) > 0 {
				return e.Usagef("unexpected argument %q", args[0])
			}
			ids := make([]string, 0, len(only))
			for _, arg := range only {
				id, code, ok := e.SessionID(arg)
				if !ok {
					return code
				}
				ids = append(ids, id)
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			for attempt := 0; ; attempt++ {
				err := e.streamEvents(ctx, client, ids)
				if ctx.Err() != nil {
					return contract.ExitOK
				}
				if attempt >= 5 {
					return e.Fail(err)
				}
				select {
				case <-time.After(time.Second):
				case <-ctx.Done():
					return contract.ExitOK
				}
			}
		}
	},
}

func (e Env) streamEvents(ctx context.Context, client *localclient.Client, ids []string) error {
	conn, err := client.DialWebSocket(ctx, "/ws/sessions")
	if err != nil {
		return err
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(64 << 20)
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		if len(ids) > 0 {
			var event contract.SessionEvent
			if json.Unmarshal(data, &event) != nil {
				continue
			}
			switch event.Type {
			case "snapshot":
				event.Sessions = slices.DeleteFunc(event.Sessions, func(s contract.SessionInfo) bool { return !slices.Contains(ids, s.ID) })
				data, _ = json.Marshal(event)
			case "created", "updated":
				if event.Session == nil || !slices.Contains(ids, event.Session.ID) {
					continue
				}
			case "removed":
				if !slices.Contains(ids, event.ID) {
					continue
				}
			default:
				continue
			}
		}
		_, _ = e.Stdout.Write(append(bytes.TrimSpace(data), '\n'))
	}
}
