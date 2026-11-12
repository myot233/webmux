package hybrid

import (
	"errors"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/agentprofile/omp"
	"github.com/rainchan/webmux/internal/session"
)

// Input goes to the side the session is on: a prompt on the chat, an
// acknowledged submission on the terminal. Options of the other side are
// refused with ErrWrongSide before anything is written.
func TestInputDispatchesBySide(t *testing.T) {
	h := newHarness(t, 0)
	chat := h.create(h.input(omp.Name, SideACP)).Info().ID
	result, err := h.svc.Input(ctxT(t), chat, InputRequest{ClientRequestID: rid(), Text: "hello"})
	if err != nil || result.Side != SideACP || result.Prompt.PromptID == "" || result.Prompt.Queued {
		t.Fatalf("chat input %+v %v", result, err)
	}
	if _, err := h.svc.Input(ctxT(t), chat, InputRequest{ClientRequestID: rid(), Keys: [][]byte{[]byte("\x1b")}}); !errors.Is(err, ErrWrongSide) {
		t.Fatalf("keys on the chat: %v", err)
	}

	terminal := h.create(h.input(omp.Name, SideTUI))
	id := terminal.Info().ID
	for _, mode := range []string{ModeSteer, ModeRefuse, ModeNow} {
		if _, err := h.svc.Input(ctxT(t), id, InputRequest{ClientRequestID: rid(), Text: "x", Mode: mode}); !errors.Is(err, ErrWrongSide) {
			t.Fatalf("mode %s on the terminal: %v", mode, err)
		}
	}
	result, err = h.svc.Input(ctxT(t), id, InputRequest{ClientRequestID: rid(), Label: "script", Text: "typed", Submit: true})
	if err != nil || result.Side != SideTUI {
		t.Fatalf("terminal input %+v %v", result, err)
	}
	if err := h.svc.Interrupt(ctxT(t), id, rid(), "script"); err != nil {
		t.Fatalf("terminal interrupt: %v", err)
	}
	if err := h.svc.Interrupt(ctxT(t), chat, rid(), "script"); err != nil {
		t.Fatalf("idle chat interrupt: %v", err)
	}
}

// A switch in progress refuses input; a closed session is not active.
func TestInputRefusals(t *testing.T) {
	h := newHarness(t, 0)
	id := h.create(h.input(omp.Name, SideACP)).Info().ID
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call(ctxT(t), a, func() (struct{}, error) {
		a.side, a.switching = SideSwitching, &switchOp{from: SideACP, to: SideTUI}
		return struct{}{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Input(ctxT(t), id, InputRequest{ClientRequestID: rid(), Text: "x"}); !errors.Is(err, ErrSwitching) {
		t.Fatalf("input while switching: %v", err)
	}
	if err := h.svc.Interrupt(ctxT(t), id, rid(), "script"); !errors.Is(err, ErrSwitching) {
		t.Fatalf("interrupt while switching: %v", err)
	}
	if _, err := call(ctxT(t), a, func() (struct{}, error) {
		a.side, a.switching = SideACP, nil
		return struct{}{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	h.closeSession(id)
	if _, err := h.svc.Input(ctxT(t), id, InputRequest{ClientRequestID: rid(), Text: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("input after close: %v", err)
	}
}

// An ordinary session takes terminal input and Ctrl-C; one without a run
// fails with an input_failed receipt.
func TestTerminalInput(t *testing.T) {
	m := session.NewManager(session.Options{DefaultCwd: t.TempDir()})
	t.Cleanup(func() { m.Shutdown(ctxT(t)) })
	s, err := m.Create(session.CreateInput{Cmd: []string{"/bin/cat"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (*Service)(nil).TerminalInput(ctxT(t), s, InputRequest{ClientRequestID: rid(), Attachments: []Attachment{{Path: "/x"}}}); !errors.Is(err, ErrWrongSide) {
		t.Fatalf("attachments on a terminal: %v", err)
	}
	if _, err := (*Service)(nil).TerminalInput(ctxT(t), s, InputRequest{ClientRequestID: rid(), Text: "hi", Submit: true}); err != nil {
		t.Fatal(err)
	}
	if err := (*Service)(nil).TerminalInterrupt(ctxT(t), s, rid(), "script"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "cat exits on Ctrl-C", func() bool { return s.Info().Status == session.StatusExited })
	_, err = (*Service)(nil).TerminalInput(ctxT(t), s, InputRequest{ClientRequestID: rid(), Text: "late"})
	var failed *InputFailedError
	if !errors.As(err, &failed) || !errors.Is(err, ErrInputFailed) {
		t.Fatalf("input after exit: %v", err)
	}
}

func TestTerminalInputUsesSessionSubmitDelay(t *testing.T) {
	m := session.NewManager(session.Options{
		DefaultCwd:  t.TempDir(),
		SubmitDelay: func([]string) time.Duration { return 50 * time.Millisecond },
	})
	t.Cleanup(func() { m.Shutdown(ctxT(t)) })
	s, err := m.Create(session.CreateInput{Cmd: []string{"/bin/cat"}})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := (*Service)(nil).TerminalInput(ctxT(t), s, InputRequest{ClientRequestID: rid(), Text: "hi", Submit: true}); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Fatalf("ordinary terminal input was not delayed: %v", elapsed)
	}
}

// Forget closes an active session first, then deletes its record.
func TestForgetActive(t *testing.T) {
	h := newHarness(t, 0)
	id := h.create(h.input(omp.Name, SideACP)).Info().ID
	if err := h.svc.Forget(ctxT(t), id); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Record(id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("record after forget: %v", err)
	}
	waitFor(t, "shell removed", func() bool { _, err := h.manager.Get(id); return err != nil })
	if err := h.svc.Forget(ctxT(t), id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("forget twice: %v", err)
	}
}
