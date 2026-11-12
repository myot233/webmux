package session

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rainchan/webmux/internal/attentionkind"
)

// MaxAttentionMessageRunes bounds the text a script attaches to attention.
const MaxAttentionMessageRunes = 200

// ErrExited reports a change that needs a session that has not exited.
var ErrExited = errors.New("session has exited")

// RaiseAttention latches attention a script asked for on session id. It
// replaces whatever attention is latched, whatever its source, and is then
// cleared by the usual rules: user input, the program's next status report,
// or ClearAttention. message is kept only in memory.
func (m *Manager) RaiseAttention(id string, kind attentionkind.Kind, message string) (Info, error) {
	switch kind {
	case attentionkind.Notify, attentionkind.Done, attentionkind.Error, attentionkind.Question:
	default:
		return Info{}, fmt.Errorf("%w: kind 须为 notify、done、error 或 question", ErrInvalid)
	}
	message = strings.TrimSpace(message)
	if !utf8.ValidString(message) || utf8.RuneCountInString(message) > MaxAttentionMessageRunes ||
		strings.ContainsFunc(message, unicode.IsControl) {
		return Info{}, fmt.Errorf("%w: message 须不超过 %d 个字符且不含控制字符", ErrInvalid, MaxAttentionMessageRunes)
	}
	return m.changeAttention(id, func(s *Session) error {
		if s.infoLocked().Status == StatusExited {
			return ErrExited
		}
		s.cancelPendingAttentionLocked()
		// The agent layer lets the program's own later events replace or
		// clear it, while bells and OSC 9/777 notifications cannot.
		attention := &Attention{Kind: kind, Source: SourceCLI, Since: time.Now(), Message: message, layer: layerAgent}
		if s.run == nil && s.overlay != nil {
			s.cliAttention = attention
		} else {
			s.info.Attention = attention
		}
		return nil
	})
}

// ClearAttention clears session id's latched attention the way user input
// does, whatever raised it. A session without attention is left unchanged.
func (m *Manager) ClearAttention(id string) (Info, error) {
	return m.changeAttention(id, func(s *Session) error {
		s.cancelPendingAttentionLocked()
		s.info.Attention, s.cliAttention = nil, nil
		return nil
	})
}

// changeAttention applies change to a session and publishes the result
// when the visible attention moved.
func (m *Manager) changeAttention(id string, change func(*Session) error) (Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return Info{}, ErrNotFound
	}
	s.mu.Lock()
	before := s.infoLocked()
	if err := change(s); err != nil {
		s.mu.Unlock()
		return Info{}, err
	}
	// Show the status attention implies now instead of on the next tick;
	// /proc is not read, so a process blocked on input is caught then.
	if s.run != nil && s.info.ExitCode == nil {
		s.info.Status = classify(s.statusInputLocked(time.Now(), m.options.IdleAfter, m.patterns))
	}
	info := s.infoLocked()
	if sameAttention(before.Attention, info.Attention) {
		s.mu.Unlock()
		return info, nil
	}
	s.broadcastLocked(s.stateLocked())
	s.mu.Unlock()
	m.publishLocked(Event{Type: "updated", Session: &info})
	return info, nil
}

// attentionChanged reports whether the driver's next overlay carries other
// attention than the current one.
func attentionChanged(current, next *Overlay) bool {
	var a, b *Attention
	if current != nil {
		a = current.Attention
	}
	if next != nil {
		b = next.Attention
	}
	return !sameAttention(a, b)
}
