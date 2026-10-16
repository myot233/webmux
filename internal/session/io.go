package session

import (
	"context"
	"os"
	"syscall"
	"time"
)

// A nonblocking descriptor handed to NewFile participates in Go's poller.
// Duplicating transfers ownership without retaining two File finalizers on one fd.
func pollablePTY(original *os.File) (*os.File, error) {
	syscall.ForkLock.RLock()
	fd, err := syscall.Dup(int(original.Fd()))
	if err == nil {
		syscall.CloseOnExec(fd)
	}
	syscall.ForkLock.RUnlock()
	if err != nil {
		return nil, err
	}
	if err = syscall.SetNonblock(fd, true); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	name := original.Name()
	if err = original.Close(); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	f := os.NewFile(uintptr(fd), name)
	if err = f.SetReadDeadline(time.Time{}); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// WriteContext serializes input, but a disconnected client can abandon both
// the serialization wait and a full kernel input queue without ending the PTY.
func (s *Session) WriteContext(ctx context.Context, p []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return os.ErrClosed
	case <-s.inputGate:
	}
	defer func() { s.inputGate <- struct{}{} }()
	// Cancellation and an available gate can both be selected. Never deliver
	// input after cancellation merely because the gate happened to win.
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	if until, ok := ctx.Deadline(); ok && until.Before(deadline) {
		deadline = until
	}
	if err := s.terminal.SetWriteDeadline(deadline); err != nil {
		return err
	}
	cancelled := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = s.terminal.SetWriteDeadline(time.Now())
		close(cancelled)
	})
	_, err := s.terminal.Write(p)
	if !stop() {
		<-cancelled
	}
	_ = s.terminal.SetWriteDeadline(time.Time{})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err == nil {
		s.mu.Lock()
		s.userInputLocked(p, time.Now())
		s.mu.Unlock()
	}
	return err
}
