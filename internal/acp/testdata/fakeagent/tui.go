package main

import (
	"bytes"
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
	cfg    config
	st     *store
	cwd    string
	codex  bool
	claude bool
	agy    bool
	kimi   bool
	devin  bool

	outMu sync.Mutex
	out   io.Writer

	cur          *session
	locks        []*os.File    // Codex: the writer locks of every thread opened
	turn         chan struct{} // closed to interrupt the running turn; nil when idle
	turnDone     chan bool     // receives interrupted when a turn ends
	pending      string        // the running turn's prompt
	line         []byte
	lastEsc      time.Time
	queryPending bool
	queryReply   []byte
}

func runTUI(cfg config, st *store, in io.Reader, out io.Writer) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	t := &tui{
		cfg: cfg, st: st, cwd: cwd, codex: cfg.profile == "codex", claude: cfg.profile == "claudecode", agy: cfg.profile == "agy", kimi: cfg.profile == "kimi", devin: cfg.profile == "devin",
		out: out, turnDone: make(chan bool, 1), queryPending: cfg.profile == "claudecode",
	}
	defer func() {
		if t.claude {
			removeClaudeRegistration()
		}
	}()
	restore := rawMode()
	defer restore()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

	t.printf("fakeagent tui (%s)\r\n", cfg.profile)
	if t.codex {
		// Codex reports working while it starts, and nothing but its root
		// record.
		t.emit7501("state=working:app=codex")
	} else if t.claude {
		// Claude waits for the capability answer before publishing idle.
		t.emit7501("?")
	} else if t.agy {
		t.emit7501("state=idle:app=antigravity")
	} else if t.kimi {
		t.emit7501("state=idle:app=kimi-code")
	} else if t.devin {
		t.emit7501("state=idle:app=devin")
	} else {
		t.emit7501("state=idle:app=" + cfg.profile)
	}
	switch {
	case cfg.resume != "":
		// Codex resume starts a blank thread before opening the resumed one.
		if t.codex {
			if err := t.holdLock(st.newID()); err != nil {
				return err
			}
		}
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
			ss = &session{ID: cfg.session, Cwd: cwd, Messages: []message{}, Config: defaultConfig(cfg.profile)}
			err = st.save(ss)
		}
		if err != nil {
			return fmt.Errorf("session %s: %w", cfg.session, err)
		}
		if err := t.open(ss); err != nil {
			return err
		}
	case cfg.profile == "omp" || t.codex || t.claude || t.kimi || t.devin:
		if err := t.create(); err != nil {
			return err
		}
	}
	if t.codex {
		time.Sleep(cfg.startupDelay)
		t.emit7501("state=idle:app=codex")
	}
	t.printf("prompt> ")
	if cfg.prompt != "" {
		t.submit(cfg.prompt)
	}

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

// warp sends a Warp cli-agent event; Codex, Claude, Antigravity and Kimi send none.
func (t *tui) warp(event string) {
	if t.codex || t.claude || t.agy || t.kimi || t.devin {
		return
	}
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
	if err := t.holdLock(ss.ID); err != nil {
		return err
	}
	if t.cur != nil {
		t.st.release(t.cur.ID)
	}
	requested := ss.ID
	var err error
	if t.claude {
		// Unlike omp, a second Claude TUI keeps the requested UUID and gets
		// its own PID registration instead of forking the session.
		t.st.event("open", ss.ID)
	} else {
		ss, err = t.st.lease(ss)
		if err != nil {
			return err
		}
	}
	if ss.ID != requested {
		if err := t.holdLock(ss.ID); err != nil {
			return err
		}
	}
	t.cur = ss
	if t.claude {
		if err := writeClaudeRegistration(t.cwd, ss.ID); err != nil {
			return err
		}
	}
	for _, m := range ss.Messages {
		mark := ">"
		if m.Role == "agent" {
			mark = "<"
		}
		t.printf("%s %s\r\n", mark, m.Text)
	}
	t.warp("session_start")
	// Like the real OpenCode TUI (baseline.md), the opencode profile names
	// its session only once a turn runs, even when it opened one with -s;
	// Codex and Claude never report it.
	if t.cfg.profile == "omp" {
		t.emit7501("state=idle:id=" + ss.ID)
	} else if t.agy && t.hasTitleHook() {
		t.emit7501("state=working:id=" + ss.ID)
	}
	return nil
}

func (t *tui) hasTitleHook() bool {
	return strings.Contains(t.cfg.settings, "conversation_id") || strings.Contains(t.cfg.settings, "7501")
}

// holdLock keeps a Codex thread's writer lock until the process exits, as
// Codex does even after /new.
func (t *tui) holdLock(id string) error {
	if !t.codex {
		return nil
	}
	f, err := holdCodexLock(id)
	if err != nil {
		return err
	}
	t.locks = append(t.locks, f)
	return nil
}

// keys handles input and reports whether the TUI should exit.
func (t *tui) keys(chunk []byte) bool {
	if t.claude && t.queryPending {
		chunk = t.consumeQueryReply(chunk)
	}
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

// consumeQueryReply waits for WebMux's OSC 7501 capability response. The
// response may share a read with subsequent keystrokes, so only its bytes are
// consumed. Input before the response is startup noise, not a prompt.
func (t *tui) consumeQueryReply(chunk []byte) []byte {
	t.queryReply = append(t.queryReply, chunk...)
	prefix := []byte("\x1b]7501;?:")
	start := bytes.Index(t.queryReply, prefix)
	if start < 0 {
		if len(t.queryReply) > 4096 {
			t.queryReply = append(t.queryReply[:0], t.queryReply[len(t.queryReply)-len(prefix)+1:]...)
		}
		return nil
	}
	body := t.queryReply[start+len(prefix):]
	end, width := bytes.Index(body, []byte("\x1b\\")), 2
	if bel := bytes.IndexByte(body, '\a'); bel >= 0 && (end < 0 || bel < end) {
		end, width = bel, 1
	}
	if end < 0 {
		return nil
	}
	rest := append([]byte(nil), body[end+width:]...)
	t.queryReply = nil
	t.queryPending = false
	t.emit7501("state=idle:app=claude-code")
	return rest
}

func (t *tui) escape() {
	now := time.Now()
	double := now.Sub(t.lastEsc) <= doubleEscWindow
	t.lastEsc = now
	if t.turn == nil || t.cfg.ignoreInterrupt {
		return
	}
	if t.cfg.profile != "opencode" || double {
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
	case line == "/login" && t.cfg.authFile != "":
		if err := os.WriteFile(t.cfg.authFile, []byte("authenticated\n"), 0o600); err != nil {
			t.printf("error: %v\r\n", err)
		} else {
			t.printf("authenticated\r\n")
		}
		t.printf("prompt> ")
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
	t.status("working")
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
	if (t.claude || t.agy || t.devin) && !interrupted {
		t.status("done")
	} else {
		t.status("idle")
	}
	t.warp("stop")
	t.printf("prompt> ")
}

// status reports a turn's state: Codex, Claude and Kimi in their root records
// only, Antigravity in its root record plus the child record when the title
// hook is active, and the others in the root and the session's child record.
func (t *tui) status(state string) {
	if t.codex {
		t.emit7501("state=" + state + ":app=codex")
		return
	}
	if t.claude {
		t.emit7501("state=" + state + ":app=claude-code")
		return
	}
	if t.agy {
		t.emit7501("state=" + state + ":app=antigravity")
		if t.cur != nil && t.hasTitleHook() {
			t.emit7501("state=working:id=" + t.cur.ID)
		}
		return
	}
	if t.kimi {
		t.emit7501("state=" + state + ":app=kimi-code")
		return
	}
	if t.devin {
		t.emit7501("state=" + state + ":app=devin")
		return
	}
	t.emit7501("state=" + state)
	t.emit7501("state=" + state + ":id=" + t.cur.ID)
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
	for _, f := range t.locks {
		_ = f.Close()
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
