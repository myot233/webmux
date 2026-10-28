package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// message is one stored turn entry.
type message struct {
	ID   string `json:"id"`
	Role string `json:"role"` // user | agent
	Text string `json:"text"`
}

// session is the on-disk record both modes read and write.
type session struct {
	ID        string            `json:"id"`
	Cwd       string            `json:"cwd"`
	Title     string            `json:"title,omitempty"`
	UpdatedAt string            `json:"updatedAt"`
	Messages  []message         `json:"messages"`
	Config    map[string]string `json:"config"`
}

// store keeps sessions as <dir>/sessions/<id>.json, leases as
// <dir>/leases/<id> and an append-only <dir>/events.log of
// "<unixnano> <pid> <mode> <event> <id>" lines that tests use to observe
// writers.
type store struct {
	dir     string
	mode    string
	profile string
	mu      sync.Mutex
}

func openStore(dir, mode, profile string) (*store, error) {
	for _, sub := range []string{"sessions", "leases"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return nil, fmt.Errorf("store: %w", err)
		}
	}
	return &store{dir: dir, mode: mode, profile: profile}, nil
}

func (s *store) newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	if s.profile == "opencode" {
		return "ses_" + hex.EncodeToString(b[:])
	}
	return hex.EncodeToString(b[:])
}

func (s *store) path(id string) string { return filepath.Join(s.dir, "sessions", id+".json") }

func (s *store) create(cwd string) (*session, error) {
	ss := s.draft(cwd)
	return ss, s.save(ss)
}

// draft is a session not yet on disk. Like the real TUIs, a session started
// in the terminal is only saved, and so listed, once it has a message.
func (s *store) draft(cwd string) *session {
	return &session{ID: s.newID(), Cwd: cwd, Messages: []message{}, Config: defaultConfig()}
}

func defaultConfig() map[string]string { return map[string]string{"mode": "code", "model": "fast"} }

func (s *store) load(id string) (*session, error) {
	if id == "" || strings.ContainsAny(id, `/\`) {
		return nil, fs.ErrNotExist
	}
	b, err := os.ReadFile(s.path(id))
	if err != nil {
		return nil, err
	}
	var ss session
	if err := json.Unmarshal(b, &ss); err != nil {
		return nil, fmt.Errorf("store: %s: %w", id, err)
	}
	if ss.Config == nil {
		ss.Config = defaultConfig()
	}
	return &ss, nil
}

func (s *store) save(ss *session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	b, err := json.Marshal(ss)
	if err != nil {
		return err
	}
	tmp := s.path(ss.ID) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(ss.ID))
}

func (s *store) list(cwd string) ([]*session, error) {
	entries, err := os.ReadDir(filepath.Join(s.dir, "sessions"))
	if err != nil {
		return nil, err
	}
	var out []*session
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok {
			continue
		}
		ss, err := s.load(id)
		if err != nil {
			continue
		}
		if cwd == "" || ss.Cwd == cwd {
			out = append(out, ss)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}

// lease takes the single-writer lease on id. Like omp, opening a session
// another live process holds forks it: the copy gets a new ID and the
// conflict is logged as a violation for tests to assert on.
func (s *store) lease(ss *session) (*session, error) {
	for range 3 {
		p := filepath.Join(s.dir, "leases", ss.ID)
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_, werr := fmt.Fprintf(f, "%d %s\n", os.Getpid(), s.mode)
			cerr := f.Close()
			s.event("open", ss.ID)
			return ss, errors.Join(werr, cerr)
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		if holder := leaseHolder(p); holder > 0 && holder != os.Getpid() && alive(holder) {
			s.event("violation", ss.ID)
			fork := &session{ID: s.newID(), Cwd: ss.Cwd, Title: ss.Title, Messages: ss.Messages, Config: ss.Config}
			if err := s.save(fork); err != nil {
				return nil, err
			}
			s.event("fork", fork.ID)
			ss = fork
			continue
		}
		_ = os.Remove(p) // stale lease of a dead process
	}
	return nil, fmt.Errorf("store: cannot lease %s", ss.ID)
}

func (s *store) release(id string) {
	p := filepath.Join(s.dir, "leases", id)
	if leaseHolder(p) == os.Getpid() {
		_ = os.Remove(p)
		s.event("close", id)
	}
}

// event appends to events.log. Tests read it, so a failed write is reported
// on stderr rather than silently lost.
func (s *store) event(name, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(filepath.Join(s.dir, "events.log"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err == nil {
		_, err = fmt.Fprintf(f, "%d %d %s %s %s\n", time.Now().UnixNano(), os.Getpid(), s.mode, name, id)
		err = errors.Join(err, f.Close())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakeagent: events.log:", err)
	}
}

func leaseHolder(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.Fields(string(b) + " 0")[0])
	return pid
}

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func newMessageID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return "msg_" + hex.EncodeToString(b[:])
}
