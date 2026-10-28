package session

import (
	"bytes"
	"context"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func replySession(t *testing.T) (*Session, *os.File) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	s := &Session{run: inputRun(t, writer), programStatus: true}
	s.signals = s.signalHandler()
	return s, reader
}

// TestProbeReplyPrecedesLaterInput races the reply loop against a client
// reply sent after the probe was queued, as a browser answering the DA1 that
// follows the probe would.
func TestProbeReplyPrecedesLaterInput(t *testing.T) {
	const da1 = "\x1b[?62;22c"
	for range 200 {
		s, reader := replySession(t)
		go s.replyLoop(s.run)
		s.mu.Lock()
		s.osc.Feed([]byte("\x1b]7501;?\a\x1b[c"), s.signals)
		s.mu.Unlock()
		if err := s.WriteInput(context.Background(), []byte(da1), true); err != nil {
			t.Fatal(err)
		}
		s.run.endLifetime()
		<-s.run.repliesDone
		_ = s.run.terminal.Close()
		written, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if string(written) != programStatusReply+da1 {
			t.Fatalf("PTY input %q", written)
		}
	}
}

func TestProbeRepliesAreBoundedAndNotUserInput(t *testing.T) {
	s, reader := replySession(t)
	s.mu.Lock()
	s.info.Attention = &Attention{Kind: "question", Source: SourceAgent, Since: time.Now(), layer: layerProgram}
	s.osc.Feed(bytes.Repeat([]byte("\x1b]7501;?\x1b\\"), maxPendingReplies+3), s.signals)
	s.mu.Unlock()
	go s.replyLoop(s.run)
	// Wait for the bytes themselves: the loop takes the whole queue before
	// writing it, so an empty queue does not mean the replies were written.
	want := strings.Repeat(programStatusReply, maxPendingReplies)
	got := make(chan []byte, 1)
	go func() {
		buf := make([]byte, len(want))
		n, _ := io.ReadFull(reader, buf)
		got <- buf[:n]
	}()
	select {
	case written := <-got:
		if string(written) != want {
			t.Fatalf("wrote %q", written)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("replies were never written without a client")
	}
	s.run.endLifetime()
	<-s.run.repliesDone
	_ = s.run.terminal.Close()
	if extra, _ := io.ReadAll(reader); len(extra) != 0 {
		t.Fatalf("wrote more than %d replies: extra %q", maxPendingReplies, extra)
	}
	if s.info.Attention == nil || !s.lastInputAt.IsZero() {
		t.Fatal("a reply counted as user input")
	}
}

// TestCancelledWriterKeepsReplies has a client writer whose context is
// already cancelled take the gate: the reply it could not write must still
// reach the PTY through the reply loop.
func TestCancelledWriterKeepsReplies(t *testing.T) {
	s, reader := replySession(t)
	s.mu.Lock()
	s.queueProbeReplyLocked()
	s.mu.Unlock()
	<-s.run.replyWake // the loop is not running yet; drop its wake-up
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	// Bypass writeParts' gate select so the cancelled writer surely holds
	// the gate, as it may when both select cases are ready.
	<-s.run.inputGate
	if err := s.flushRepliesGated(cancelled, s.run); err == nil {
		t.Fatal("cancelled flush reported success")
	}
	s.run.inputGate <- struct{}{}
	s.mu.Lock()
	pending := len(s.replies)
	s.mu.Unlock()
	if pending != 1 {
		t.Fatalf("cancelled writer left %d replies", pending)
	}
	go s.replyLoop(s.run)
	select {
	case s.run.replyWake <- struct{}{}:
	default:
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		s.mu.Lock()
		pending = len(s.replies)
		s.mu.Unlock()
		if pending == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("reply loop never wrote the requeued reply")
		}
		time.Sleep(5 * time.Millisecond)
	}
	s.run.endLifetime()
	<-s.run.repliesDone
	_ = s.run.terminal.Close()
	if written, _ := io.ReadAll(reader); string(written) != programStatusReply {
		t.Fatalf("PTY input %q", written)
	}
}

func TestFailedReplyWriteRequeuesUnwrittenReplies(t *testing.T) {
	s, _ := replySession(t)
	s.mu.Lock()
	s.queueProbeReplyLocked()
	s.queueProbeReplyLocked()
	s.mu.Unlock()
	_ = s.run.terminal.Close() // every write now fails before writing a byte
	<-s.run.inputGate
	if err := s.flushRepliesGated(context.Background(), s.run); err == nil {
		t.Fatal("write to a closed terminal succeeded")
	}
	s.run.inputGate <- struct{}{}
	if len(s.replies) != 2 {
		t.Fatalf("unwritten replies lost: %d left", len(s.replies))
	}
	// Probes that arrive while a flush fails still respect the bound.
	<-s.run.inputGate
	s.mu.Lock()
	for range maxPendingReplies {
		s.replies = append(s.replies, []byte(programStatusReply))
	}
	s.mu.Unlock()
	_ = s.flushRepliesGated(context.Background(), s.run)
	s.run.inputGate <- struct{}{}
	if len(s.replies) != maxPendingReplies {
		t.Fatalf("requeue left %d replies, bound is %d", len(s.replies), maxPendingReplies)
	}
}

func TestReplyLoopEndsWithLifetime(t *testing.T) {
	s, _ := replySession(t)
	// Hold the gate as a stuck writer would: the loop must still end.
	<-s.run.inputGate
	s.mu.Lock()
	s.queueProbeReplyLocked()
	s.mu.Unlock()
	go s.replyLoop(s.run)
	time.Sleep(20 * time.Millisecond)
	s.run.endLifetime()
	select {
	case <-s.run.repliesDone:
	case <-time.After(2 * time.Second):
		t.Fatal("reply loop outlived the session")
	}
}

func TestOutputKeepsProgramStatus(t *testing.T) {
	s := attentionSession()
	feedSignals(s, programOutput("state=idle"))
	if got := s.outputStatusLocked(); got != StatusIdle {
		t.Fatalf("output during idle record: %s", got)
	}
	feedSignals(s, programOutput("state=working"))
	if got := s.outputStatusLocked(); got != StatusRunning {
		t.Fatalf("output during working record: %s", got)
	}
}

func TestRealSessionAnswersProbeWithoutClients(t *testing.T) {
	script := `stty raw -echo; printf '\033]7501;?\007\033[c'; ` +
		`reply=$(dd bs=1 count=` + strconv.Itoa(len(programStatusReply)) + ` 2>/dev/null); ` +
		`case "$reply" in *"7501;?:states=idle,working,done,blocked,error"*) echo GOT-REPLY;; *) echo NO-REPLY;; esac; ` +
		`printf '\033]7501;state=blocked:kind=permission\007'; sleep 30`
	m := NewManager(Options{DefaultCwd: t.TempDir()})
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	}()
	s, err := m.Create(CreateInput{Cwd: "", Cmd: []string{"/bin/bash", "-c", script}, Name: "", Env: nil, ConfigID: ""})
	if err != nil {
		t.Fatal(err)
	}
	waitForOutput(t, s, "GOT-REPLY")
	deadline := time.Now().Add(2 * time.Second)
	for {
		info := s.Info()
		if info.Attention != nil && info.Attention.Kind == "permission" && info.Status == StatusWaiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("permission record not reflected: %+v", info)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestReplayDoesNotAnswerProbeAgain(t *testing.T) {
	// perl, unlike timeout(1), is present on Linux and macOS alike.
	script := `stty raw -echo; printf '\033]7501;?\007READY'; ` +
		`perl -e 'use IO::Select; my $s = IO::Select->new(\*STDIN); my $b = ""; ` +
		`while ($s->can_read(2)) { sysread(STDIN, my $c, 4096) or last; $b .= $c } ` +
		`my $n = () = $b =~ /7501;\?:/g; print "\nCOUNT=$n\n"'; sleep 30`
	m := NewManager(Options{DefaultCwd: t.TempDir()})
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	}()
	s, err := m.Create(CreateInput{Cwd: "", Cmd: []string{"/bin/bash", "-c", script}, Name: "", Env: nil, ConfigID: ""})
	if err != nil {
		t.Fatal(err)
	}
	waitForOutput(t, s, "READY")
	for range 3 {
		s.Detach(attached(s.Attach()))
	}
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(string(s.Buffer()), "COUNT=") {
		if time.Now().After(deadline) {
			t.Fatalf("script never reported: %q", s.Buffer())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(string(s.Buffer()), "COUNT=1") {
		t.Fatalf("probe answered other than once: %q", s.Buffer())
	}
}
