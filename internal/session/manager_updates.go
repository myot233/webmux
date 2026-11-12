package session

import (
	"context"
	"fmt"
	"github.com/rainchan/webmux/internal/procpeek"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func (m *Manager) Subscribe() *ListSubscriber {
	m.mu.Lock()
	defer m.mu.Unlock()
	sub := &ListSubscriber{Out: make(chan Event, sessionListQueueCapacity), Slow: make(chan struct{})}
	sub.Out <- Event{Type: "snapshot", Sessions: m.listLocked()}
	m.subs[sub] = struct{}{}
	return sub
}
func (m *Manager) Unsubscribe(sub *ListSubscriber) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.subs, sub)
}
func (m *Manager) publishLocked(e Event) {
	for sub := range m.subs {
		if sub.failed {
			continue
		}
		select {
		case sub.Out <- e:
		default:
			sub.failed = true
			close(sub.Slow)
		}
	}
}
func (m *Manager) updates() {
	defer close(m.updatesDone)
	timer := time.NewTicker(500 * time.Millisecond)
	defer timer.Stop()
	previous := make(map[string]Info)
	previewSent := make(map[string]time.Time)
	var snapshot procpeek.Snapshot
	var ports procpeek.PortScanner
	probeTick, portTick := false, false
	for {
		select {
		case <-m.done:
			return
		case now := <-timer.C:
			probeTick = !probeTick
			if probeTick {
				table := procpeek.Processes()
				reapAdopted(table)
				snapshot = procpeek.NewSnapshot(table)
			}
			m.mu.Lock()
			list := make([]*Session, 0, len(m.sessions))
			for _, s := range m.sessions {
				list = append(list, s)
			}
			m.mu.Unlock()
			// Classify every 500 ms using the once-per-second process table.
			// This leaves headroom for delivering the 3-second waiting signal.
			// Classification reads /proc, so it runs without the manager lock.
			// Listening ports change rarely: every other process table.
			if probeTick {
				portTick = !portTick
				if portTick {
					m.scanPorts(&ports, snapshot, list)
				}
			}
			infos := make(map[*Session]Info, len(list))
			for _, s := range list {
				s.evaluate(now, snapshot, m.options.IdleAfter, m.patterns)
				infos[s] = s.Info()
			}
			m.mu.Lock()
			for id, s := range m.sessions {
				info, evaluated := infos[s]
				if !evaluated {
					// Created after classification; the next tick covers it.
					continue
				}
				old, known := previous[id]
				if known && now.Sub(previewSent[id]) < 2*time.Second {
					info.Preview = old.Preview
				}
				if !known || !sameDisplay(old, info) {
					m.publishLocked(Event{Type: "updated", Session: &info})
					s.publishState()
					if !known || old.Preview != info.Preview {
						previewSent[id] = now
					}
					previous[id] = info
				}
			}
			for id := range previous {
				if _, exists := m.sessions[id]; !exists {
					delete(previous, id)
					delete(previewSent, id)
				}
			}
			m.mu.Unlock()
		}
	}
}
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	m.stopped = true
	m.mu.Unlock()
	// Wait for an in-flight create to either register or clean up its process
	// before taking the list Shutdown will close.
	m.createMu.Lock()
	m.mu.Lock()
	list := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
	}
	m.mu.Unlock()
	m.createMu.Unlock()
	// Shells are closed without their drivers: their records must survive
	// for recovery after restart.
	for _, s := range list {
		s.Close()
	}
	for _, s := range list {
		s.awaitProcesses(ctx)
		if s.shell {
			s.retire(true)
		}
	}
	reapAdopted(procpeek.Processes())
	m.stopOnce.Do(func() { close(m.done) })
	<-m.updatesDone
}

// SetWorkspace commits a separately resolved association only for the same
// session and immutable launch directory. No filesystem or Git work runs here.
func (m *Manager) SetWorkspace(id, cwd string, workspace *Workspace) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return
	}
	s.mu.Lock()
	if s.info.Cwd != cwd || sameWorkspace(s.info.Workspace, workspace) {
		s.mu.Unlock()
		return
	}
	if workspace == nil {
		s.info.Workspace = nil
	} else {
		value := *workspace
		s.info.Workspace = &value
	}
	info := s.infoLocked()
	s.broadcastLocked(s.stateLocked())
	s.mu.Unlock()
	m.publishLocked(Event{Type: "updated", Session: &info})
}

// MaxNameBytes bounds a user-chosen session name, the same limit an OSC title
// is held to.
const MaxNameBytes = 512

// Rename gives session id the user-chosen name, which then outranks terminal
// and agent titles. An empty name hands the title back to them, showing the
// launch directory until one arrives. A name is trimmed and may not hold
// control characters or exceed MaxNameBytes.
func (m *Manager) Rename(id, name string) (Info, error) {
	name = strings.TrimSpace(name)
	if len(name) > MaxNameBytes || !utf8.ValidString(name) || strings.ContainsFunc(name, unicode.IsControl) {
		return Info{}, fmt.Errorf("%w: 名称不能含控制字符或超过 %d 字节", ErrInvalid, MaxNameBytes)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return Info{}, ErrNotFound
	}
	s.mu.Lock()
	s.userTitle = name
	s.info.Title = filepath.Base(s.info.Cwd)
	s.info.PlaceholderTitle = name == ""
	if name != "" {
		s.info.Title = name
	}
	info := s.infoLocked()
	s.broadcastLocked(s.stateLocked())
	s.mu.Unlock()
	m.publishLocked(Event{Type: "updated", Session: &info})
	return info, nil
}
