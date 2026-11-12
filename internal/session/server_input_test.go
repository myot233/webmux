package session

import (
	"context"
	"testing"
	"time"
)

// Server input is one acknowledged submission plus keys. It takes control
// with its label while writing, gives it back after, and a repeated request
// ID is never written twice.
func TestSubmitServer(t *testing.T) {
	m := NewManager(Options{DefaultCwd: t.TempDir()})
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	}()
	s, err := m.Create(CreateInput{Cmd: []string{"/bin/cat"}})
	if err != nil {
		t.Fatal(err)
	}
	observer, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Detach(observer)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	in := ServerInput{RequestID: "req-00000001", Label: "script", Body: []byte("once"), Submit: []byte("\r"), Keys: [][]byte{[]byte("x\r")}}
	status, err := s.SubmitServer(ctx, in)
	if err != nil || status.State != SubmissionWritten {
		t.Fatalf("submit %+v %v", status, err)
	}
	waitForOutput(t, s, "once")
	waitForOutput(t, s, "x")
	time.Sleep(100 * time.Millisecond)
	written := countOutput(s.Buffer(), "once")
	if again, err := s.SubmitServer(ctx, in); err != nil || again != status {
		t.Fatalf("repeat %+v %v", again, err)
	}
	time.Sleep(100 * time.Millisecond)
	if n := countOutput(s.Buffer(), "once"); n != written {
		t.Fatalf("repeat wrote again: %d then %d", written, n)
	}
	labels := []string{}
	deadline := time.After(2 * time.Second)
	for len(labels) < 3 {
		select {
		case m := <-observer.Out:
			if m.Control != nil {
				label := ""
				if m.Control.Controller != nil {
					label = m.Control.Controller.Label
				}
				labels = append(labels, label)
			}
		case <-deadline:
			t.Fatalf("control frames %q", labels)
		}
	}
	// The observer took control on attach; the script took it over and
	// released it.
	if last := labels[len(labels)-1]; last != "" || !contains(labels, "script") {
		t.Fatalf("control labels %q", labels)
	}
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func countOutput(b []byte, text string) int {
	n := 0
	for i := 0; i+len(text) <= len(b); i++ {
		if string(b[i:i+len(text)]) == text {
			n++
		}
	}
	return n
}

func TestSubmitServerSeparatesBodyAndSubmit(t *testing.T) {
	m := NewManager(Options{DefaultCwd: t.TempDir(), SubmitDelay: func([]string) time.Duration { return 50 * time.Millisecond }})
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	}()
	s, err := m.Create(CreateInput{Cmd: []string{"/bin/cat"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	in := ServerInput{RequestID: "req-delay-1", Body: []byte("body"), Submit: []byte("\r")}
	start := time.Now()
	status, err := s.SubmitServer(ctx, in)
	if err != nil || status.State != SubmissionWritten {
		t.Fatalf("submit %+v %v", status, err)
	}
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Fatalf("body and Enter were not separated: %v", elapsed)
	}
	waitForOutput(t, s, "body")
}
