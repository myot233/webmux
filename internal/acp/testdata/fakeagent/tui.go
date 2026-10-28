package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const pluginVersion = "1.0.0"

// doubleEscWindow is how close two Esc presses must be for the opencode
// profile's interrupt.
const doubleEscWindow = time.Second

type tui struct {
	cfg config
	st  *store
	cwd string

	outMu sync.Mutex
	out   io.Writer

	cur      *session
	turn     chan struct{} // closed to interrupt the running turn; nil when idle
	turnDone chan bool     // receives interrupted when a turn ends
	pending  string        // the running turn's prompt
	line     []byte
	lastEsc  time.Time
}

func runTUI(cfg config, st *store, in io.Reader, out io.Writer) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	t := &tui{cfg: cfg, st: st, cwd: cwd, out: out, turnDone: make(chan bool, 1)}
	restore := rawMode()
	defer restore()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

	t.printf("fakeagent tui (%s)\r\n", cfg.profile)
	t.emit7501("state=idle:app=" + cfg.profile)
	switch {
	case cfg.resume != "":
		ss, err := st.load(cfg.resume)
		if err != nil {
			return fmt.Errorf("resume %s: %w", cfg.resume, err)
		}
		if err := t.open(ss); err != nil {
			return err
		}
	case cfg.session != "":
		ss, err := st.load(cfg.session)
		if errors.Is(err, fs.ErrNotExist) {
			ss = &session{ID: cfg.session, Cwd: cwd, Messages: []message{}, Config: defaultConfig()}
			err = st.save(ss)
		}
		if err != nil {
			return fmt.Errorf("session %s: %w", cfg.session, err)
		}
		if err := t.open(ss); err != nil {
			return err
		}
	case cfg.profile == "omp":
		if err := t.create(); err != nil {
			return err
		}
	}
	t.printf("prompt> ")

	keys := make(chan []byte)
	go func() {
		defer close(keys)
		buf := make([]byte, 4096)
		for {
			n, err := in.Read(buf)
			if n > 0 {
				keys <- append([]byte(nil), buf[:n]...)
			}
			if err != nil {
				return
			}
		}
	}()
	for {
		select {
		case <-sig:
			t.shutdown()
			return nil
		case chunk, ok := <-keys:
			if !ok {
				t.shutdown()
				return nil
			}
			if t.keys(chunk) {
				t.shutdown()
				return nil
			}
		case interrupted := <-t.turnDone:
			t.finishTurn(interrupted)
		}
	}
}

func (t *tui) printf(format string, args ...any) {
	t.outMu.Lock()
	defer t.outMu.Unlock()
	// A closed terminal ends the TUI through stdin EOF or SIGHUP; a failed
	// write has no one left to report to.
	_, _ = fmt.Fprintf(t.out, format, args...)
}

func (t *tui) emit7501(body string) { t.printf("\x1b]7501;%s\x1b\\", body) }

func (t *tui) warp(event string) {
	ev := map[string]any{"v": 1, "event": event, "agent": t.cfg.profile, "plugin_version": pluginVersion, "cwd": t.cwd}
	if t.cur != nil {
		ev["session_id"] = t.cur.ID
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	t.printf("\x1b]777;notify;warp://cli-agent;%s\x07", b)
}

func (t *tui) create() error {
	return t.open(t.st.draft(t.cwd))
}

func (t *tui) open(ss *session) error {
	if t.cur != nil {
		t.st.release(t.cur.ID)
	}
	ss, err := t.st.lease(ss)
	if err != nil {
		return err
	}
	t.cur = ss
	for _, m := range ss.Messages {
		mark := ">"
		if m.Role == "agent" {
			mark = "<"
		}
		t.printf("%s %s\r\n", mark, m.Text)
	}
	t.warp("session_start")
	// Like the real OpenCode TUI (baseline.md), the opencode profile names
	// its session only once a turn runs, even when it opened one with -s.
	if t.cfg.profile != "opencode" {
		t.emit7501("state=idle:id=" + ss.ID)
	}
	return nil
}

// keys handles input and reports whether the TUI should exit.
func (t *tui) keys(chunk []byte) bool {
	for i := 0; i < len(chunk); i++ {
		b := chunk[i]
		switch {
		case b == 0x1b:
			// A CSI/SS3 sequence in the same read is a key, not Esc.
			if i+1 < len(chunk) && (chunk[i+1] == '[' || chunk[i+1] == 'O') {
				i++
				for i+1 < len(chunk) && (chunk[i+1] < 0x40 || chunk[i+1] > 0x7e) {
					i++
				}
				i++
				continue
			}
			// Like the real OpenCode TUI, Esc Esc in one read is a single
			// Alt+Esc key, not two presses, so it does not interrupt.
			if t.cfg.profile == "opencode" && i+1 < len(chunk) && chunk[i+1] == 0x1b {
				i++
				continue
			}
			t.escape()
		case b == '\r' || b == '\n':
			line := strings.TrimSpace(string(t.line))
			t.line = t.line[:0]
			t.printf("\r\n")
			if t.submit(line) {
				return true
			}
		case b == 0x7f || b == 0x08:
			if len(t.line) > 0 {
				t.line = t.line[:len(t.line)-1]
			}
		case b >= 0x20:
			t.line = append(t.line, b)
		}
	}
	return false
}

func (t *tui) escape() {
	now := time.Now()
	double := now.Sub(t.lastEsc) <= doubleEscWindow
	t.lastEsc = now
	if t.turn == nil || t.cfg.ignoreInterrupt {
		return
	}
	if t.cfg.profile == "omp" || double {
		close(t.turn)
		t.turn = nil
		t.lastEsc = time.Time{}
	}
}

func (t *tui) submit(line string) bool {
	switch {
	case line == "":
		return false
	case line == "/exit" || line == "/quit":
		return true
	case t.turn != nil || t.pending != "":
		t.printf("busy\r\n")
		return false
	case line == "/new":
		if err := t.create(); err != nil {
			t.printf("error: %v\r\n", err)
		}
		t.printf("prompt> ")
		return false
	}
	if t.cur == nil {
		if err := t.create(); err != nil {
			t.printf("error: %v\r\n", err)
			return false
		}
	}
	t.cur.Messages = append(t.cur.Messages, message{ID: newMessageID(), Role: "user", Text: line})
	if err := t.st.save(t.cur); err != nil {
		t.printf("error: %v\r\n", err)
	}
	t.pending = line
	t.emit7501("state=working")
	t.emit7501("state=working:id=" + t.cur.ID)
	t.warp("prompt_submit")
	stop := make(chan struct{})
	t.turn = stop
	go t.stream("echo: "+line, stop)
	return false
}

func (t *tui) stream(reply string, stop chan struct{}) {
	t.printf("< ")
	for i, word := range strings.Fields(reply) {
		select {
		case <-stop:
			t.turnDone <- true
			return
		case <-time.After(t.cfg.turnDelay):
		}
		if i == 1 {
			crashAt(t.cfg, "tui-turn")
		}
		t.printf("%s ", word)
	}
	t.turnDone <- false
}

func (t *tui) finishTurn(interrupted bool) {
	reply := "echo: " + t.pending
	if interrupted {
		reply = "echo: (interrupted)"
		t.printf("[interrupted]")
	}
	t.turn = nil
	t.pending = ""
	t.cur.Messages = append(t.cur.Messages, message{ID: newMessageID(), Role: "agent", Text: reply})
	if err := t.st.save(t.cur); err != nil {
		t.printf("error: %v\r\n", err)
	}
	t.printf("\r\n")
	t.emit7501("state=idle")
	t.emit7501("state=idle:id=" + t.cur.ID)
	t.warp("stop")
	t.printf("prompt> ")
}

func (t *tui) shutdown() {
	if t.turn != nil {
		close(t.turn)
		t.turn = nil
		t.finishTurn(<-t.turnDone)
	}
	if t.cur != nil {
		t.st.release(t.cur.ID)
	}
	t.emit7501("state=clear")
}

// rawMode switches a terminal stdin to unbuffered input so Esc arrives on
// its own; it does nothing for pipes.
func rawMode() func() {
	get := exec.Command("stty", "-g")
	get.Stdin = os.Stdin
	saved, err := get.Output()
	if err != nil {
		return func() {}
	}
	set := exec.Command("stty", "-icanon", "-echo", "min", "1", "time", "0")
	set.Stdin = os.Stdin
	if set.Run() != nil {
		return func() {}
	}
	return func() {
		r := exec.Command("stty", strings.TrimSpace(string(saved)))
		r.Stdin = os.Stdin
		_ = r.Run()
	}
}
