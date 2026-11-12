// Package attach connects a native terminal to a WebMux session over
// /ws/s/{id}: raw input and output, control and resize, a detach key and
// the session's exit code. OS terminal adapters live in term_unix.go and
// term_windows.go.
package attach

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

const (
	// inputModeReset turns off what a session's program may have enabled in the
	// native terminal: win32-input-mode, modifyOtherKeys, kitty keyboard flags,
	// mouse and focus reporting, bracketed paste and colour scheme reports; and
	// shows the cursor.
	inputModeReset = "\x1b[?9001l\x1b[>4;0m\x1b[<99u\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1004l\x1b[?2004l\x1b[?2031l\x1b[?25h"

	// backgroundQuery asks the native terminal for its background colour (OSC 11),
	// which nearly every terminal answers, unlike CSI ? 996 n.
	backgroundQuery = "\x1b]11;?\x1b\\"

	// DetachKey is Ctrl+], which never reaches the session.
	DetachKey = 0x1d

	// TakeControlKey is Enter: while another client controls the session it
	// reclaims control and is not forwarded.
	TakeControlKey = '\r'

	// ResizeInterval is how often the terminal size is polled.
	ResizeInterval = 250 * time.Millisecond
)

// Frame types of docs/protocol/websocket.md.
const (
	frameOutput   = contract.FrameOutput
	frameSnapshot = contract.FrameSnapshot
	frameExit     = contract.FrameExit
	frameControl  = contract.FrameControl

	sendInput   = contract.FrameInput
	sendResize  = contract.FrameResize
	sendResync  = contract.FrameReplay
	sendControl = contract.FrameClaim
	sendReply   = contract.FrameReply
)

// Close codes of /ws/s/{id}.
const (
	closeNotFound = 4404
	closeSlow     = 4408
	closeMoved    = contract.CloseMoved
	closeExited   = 4410
)

// ErrNotFound means the session does not exist.
var ErrNotFound = errors.New("session not found")

// ErrMoved means the session is on its chat view (or changing views), so
// its terminal cannot be attached. Run wraps it in a *MovedError naming
// the view.
var ErrMoved = errors.New("会话不在终端视图")

// MovedError is a refused attachment: the server closed with 4409 before
// the replay. To is the view named in the close reason, empty when the
// reason names none.
type MovedError struct {
	To contract.SessionView
}

func (e *MovedError) Error() string {
	if e.To == contract.ViewChat {
		return "会话在聊天视图，请用 webmux view SESSION terminal 切回终端后重新附着，或在 Web 端操作"
	}
	return "会话正在切换视图，请稍后重新附着"
}

func (e *MovedError) Unwrap() error { return ErrMoved }

// movedPrefix starts the close reason of a 4409; the view follows it.
const movedPrefix = "session moved: "

// movedTo is the view a 4409 close reason names: terminal or chat.
func movedTo(err error) contract.SessionView {
	var closeErr websocket.CloseError
	if !errors.As(err, &closeErr) {
		return ""
	}
	view, ok := strings.CutPrefix(closeErr.Reason, movedPrefix)
	if !ok {
		return ""
	}
	switch v := contract.SessionView(strings.TrimSpace(view)); v {
	case contract.ViewTerminal, contract.ViewChat:
		return v
	}
	return ""
}

// Terminal is a native terminal in raw mode.
type Terminal interface {
	io.Writer
	// ReadContext must return when ctx ends, without closing the terminal.
	// Run owns input until it returns; no other reader may consume it.
	ReadContext(ctx context.Context, p []byte) (int, error)
	// Size reports columns and rows.
	Size() (cols, rows int, err error)
}

type Options struct {
	// Observe neither sends input nor resizes, and does not take control.
	Observe bool
	// Client identifies this terminal across reconnects; random if empty.
	Client string
	// Label names the client in the browser's control indicator.
	Label string
}

// Result is how an attachment ended without error.
type Result struct {
	// Detached reports the detach key; the session keeps running.
	Detached bool
	// Moved reports that the session changed to its chat view while
	// attached (close code 4409): a passive detach, the session keeps
	// running. MovedTo is the view the close reason names.
	Moved   bool
	MovedTo contract.SessionView
	// Exited reports that the session's process ended with ExitCode.
	Exited   bool
	ExitCode int
}

// Run attaches until detach, session exit, connection loss or ctx ends.
func Run(ctx context.Context, client *localclient.Client, id string, term Terminal, o Options) (Result, error) {
	if o.Client == "" {
		var raw [12]byte
		_, _ = rand.Read(raw[:])
		o.Client = "native-" + hex.EncodeToString(raw[:])
	}
	query := url.Values{"client": {o.Client}}
	if o.Label != "" {
		query.Set("label", o.Label)
	}
	conn, err := client.DialWebSocket(ctx, "/ws/s/"+url.PathEscape(id)+"?"+query.Encode())
	if err != nil {
		return Result{}, err
	}
	// Runs after the connection closes: the session's program may have left
	// the native terminal reporting keys, mouse, focus or pastes in its own
	// encodings, which the shell behind it cannot read.
	defer func() { _, _ = term.Write([]byte(inputModeReset)) }()
	conn.SetReadLimit(64 << 20)
	a := &attachment{conn: conn, term: term, observe: o.Observe, client: o.Client}
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() {
		cancel()
		// Cancel the terminal read and socket operations before joining.
		_ = conn.CloseNow()
		workers.Wait()
	}()
	if !o.Observe {
		// Until the server confirms this claim, control frames from the
		// replay describe the previous controller.
		a.controlled, a.claiming = true, true
		if err := a.send(ctx, []byte{sendControl}); err != nil {
			return Result{}, err
		}
		a.resize(ctx, true)
		workers.Go(func() { a.watchSize(ctx) })
	}
	detached := make(chan error, 1)
	workers.Go(func() { detached <- a.pumpInput(ctx) })
	output := make(chan readResult, 1)
	workers.Go(func() { output <- a.pumpOutput(ctx) })
	select {
	case err := <-detached:
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		if err != nil {
			return Result{}, err
		}
		_ = conn.Close(websocket.StatusNormalClosure, "detached")
		return Result{Detached: true}, nil
	case r := <-output:
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		return r.result, r.err
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

type readResult struct {
	result Result
	err    error
}

type attachment struct {
	conn    *websocket.Conn
	term    Terminal
	observe bool
	client  string
	// statusProbes is used only by the output pump.
	statusProbes probeFilter

	writeMu    sync.Mutex
	sizeMu     sync.Mutex
	cols, rows int

	// controlMu guards whether this terminal controls the session, an
	// unconfirmed claim, the controlling client's label, whether the
	// program asked for colour scheme reports, and unanswered background
	// queries.
	controlMu     sync.Mutex
	controlled    bool
	claiming      bool
	controller    string
	schemeReports bool
	probes        int
}

func (a *attachment) send(ctx context.Context, frame []byte) error {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	return a.conn.Write(ctx, websocket.MessageBinary, frame)
}

// resize sends the terminal size when it changed, or always when forced.
func (a *attachment) resize(ctx context.Context, force bool) {
	cols, rows, err := a.term.Size()
	if err != nil || cols <= 0 || rows <= 0 || cols > math.MaxUint16 || rows > math.MaxUint16 {
		return
	}
	a.sizeMu.Lock()
	changed := force || cols != a.cols || rows != a.rows
	a.cols, a.rows = cols, rows
	a.sizeMu.Unlock()
	if !changed {
		return
	}
	body, _ := json.Marshal(contract.TerminalResize{Cols: uint16(cols), Rows: uint16(rows)})
	_ = a.send(ctx, append([]byte{sendResize}, body...))
}

func (a *attachment) watchSize(ctx context.Context) {
	ticker := time.NewTicker(ResizeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.resize(ctx, false)
		}
	}
}

// pumpInput forwards raw input until the detach key; it returns nil on
// detach. Input never reaches the session while observing. While another
// client controls the session, input is dropped: the terminal's automatic
// replies (focus, device and colour reports) would otherwise claim control.
// Only TakeControlKey reclaims it.
func (a *attachment) pumpInput(ctx context.Context) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := a.term.ReadContext(ctx, buf)
		if n > 0 {
			chunk, report := a.takeBackground(buf[:n])
			if report != nil {
				if err := a.send(ctx, append([]byte{sendReply}, report...)); err != nil {
					return fmt.Errorf("connection lost: %w", err)
				}
			}
			at := detachIndex(chunk)
			if at >= 0 {
				chunk = chunk[:at]
			}
			if len(chunk) > 0 && !a.observe {
				if !a.isControlling() {
					if takeControlIndex(chunk) >= 0 {
						if err := a.takeControl(ctx); err != nil {
							return fmt.Errorf("connection lost: %w", err)
						}
					}
				} else if err := a.send(ctx, append([]byte{sendInput}, chunk...)); err != nil {
					return fmt.Errorf("connection lost: %w", err)
				}
			}
			if at >= 0 {
				return nil
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			// The terminal closed: behave as a detach, the session continues.
			return nil
		}
	}
}

func (a *attachment) isControlling() bool {
	a.controlMu.Lock()
	defer a.controlMu.Unlock()
	return a.controlled
}

// takeControl claims the session, applies this terminal's size and asks for
// a replay, which repaints over the banner and confirms the claim.
func (a *attachment) takeControl(ctx context.Context) error {
	a.controlMu.Lock()
	a.controlled, a.claiming = true, true
	a.controlMu.Unlock()
	if err := a.send(ctx, []byte{sendControl}); err != nil {
		return err
	}
	a.resize(ctx, true)
	return a.send(ctx, []byte{sendResync})
}

// applyControl records a control frame and reports whether this terminal
// controls the session afterwards, and whether it should query its
// background to announce its colour scheme: as in the browser, when it starts
// answering for a program that wants reports, or the program starts wanting
// them. The program may have asked before this terminal attached, or been
// answered with another device's theme.
func (a *attachment) applyControl(body []byte) (controlling, probe bool) {
	var state contract.ControlState
	if json.Unmarshal(body, &state) != nil {
		return a.isControlling(), false
	}
	a.controlMu.Lock()
	defer a.controlMu.Unlock()
	was := a.controlled && !a.claiming
	mine := state.Controller != nil && state.Controller.ClientID == a.client
	switch {
	case mine:
		a.controlled, a.claiming = true, false
	case a.claiming:
		// A frame queued before the claim reached the server.
	default:
		a.controlled = false
		a.controller = ""
		if state.Controller != nil {
			a.controller = state.Controller.Label
		}
	}
	// The mode belongs to the session, so it is current even mid-claim.
	probe = state.ColorSchemeReports && mine && (!was || !a.schemeReports)
	a.schemeReports = state.ColorSchemeReports
	if probe {
		a.probes++
	}
	return a.controlled, probe
}

// banner is drawn over the top-left corner while another client controls
// the session; cursor and attributes are saved and restored around it.
func (a *attachment) banner() []byte {
	a.controlMu.Lock()
	who := a.controller
	a.controlMu.Unlock()
	text := " Observing · Enter to take control · Ctrl+] to detach "
	if who != "" {
		text = " Controlled by " + who + " · Enter to take control · Ctrl+] to detach "
	}
	a.sizeMu.Lock()
	cols := a.cols
	a.sizeMu.Unlock()
	if runes := []rune(text); cols > 0 && len(runes) > cols {
		text = string(runes[:cols])
	}
	return []byte("\x1b7\x1b[1;1H\x1b[0;7m" + text + "\x1b[0m\x1b8")
}

func (a *attachment) pumpOutput(ctx context.Context) readResult {
	var exit *Result
	// A refused attachment closes before any frame; a session that moves
	// away closes after the replay.
	attached := false
	for {
		_, frame, err := a.conn.Read(ctx)
		if err != nil {
			switch websocket.CloseStatus(err) {
			case closeExited:
				if exit != nil {
					return readResult{result: *exit}
				}
				return readResult{result: Result{Exited: true}}
			case closeNotFound:
				return readResult{err: ErrNotFound}
			case closeMoved:
				if attached {
					return readResult{result: Result{Moved: true, MovedTo: movedTo(err)}}
				}
				return readResult{err: &MovedError{To: movedTo(err)}}
			case closeSlow:
				return readResult{err: errors.New("connection lost: the terminal could not keep up with the session output")}
			}
			if exit != nil {
				return readResult{result: *exit}
			}
			return readResult{err: fmt.Errorf("connection lost: %w", err)}
		}
		attached = true
		if len(frame) == 0 {
			continue
		}
		var out []byte
		switch frame[0] {
		case frameOutput:
			out = a.statusProbes.filter(frame[1:])
		case frameSnapshot:
			// A full replay: reset the terminal first, as xterm.js does.
			a.statusProbes.reset()
			out = append([]byte("\x1bc"), a.statusProbes.filter(frame[1:])...)
		case frameExit:
			var body contract.ExitFrame
			_ = json.Unmarshal(frame[1:], &body)
			exit = &Result{Exited: true, ExitCode: body.ExitCode}
		case frameControl:
			if a.observe {
				continue
			}
			controlling, probe := a.applyControl(frame[1:])
			if controlling {
				// Sent when this connection becomes the active one again.
				a.resize(ctx, true)
				if !probe {
					continue
				}
				out = []byte(backgroundQuery)
				break
			}
			out = []byte{}
		}
		if out == nil {
			continue
		}
		// Session output overwrites the banner; draw it again on top.
		if !a.observe && !a.isControlling() {
			out = append(out, a.banner()...)
		}
		if _, err := a.term.Write(out); err != nil {
			return readResult{err: err}
		}
	}
}
