// Package hybrid runs hybrid sessions: one agent conversation that moves
// between the agent's terminal UI (a PTY run of a session shell) and an
// ACP chat (a sidecar process of the same shell), under one WebMux session
// ID. Each session is owned by an actor goroutine that drives the shell
// (session.Driver), translates ACP updates into normalized chat events,
// and keeps the session's record in hybrid-sessions.json.
//
// Content safety: nothing here logs prompts, messages, tool output,
// permission options, form content or agent stderr. Agent stderr only
// reaches the user who requested a failed start, in the returned error.
package hybrid

import (
	"context"
	"errors"
	"fmt"
	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/agentprofile/agents"
	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/session"
	"log/slog"
	"os"
	"slices"
	"sync"
	"time"
)

const (
	// defaultScrollbackBytes matches the session default when no limit is
	// configured.
	defaultScrollbackBytes = 2 << 20
	// shutdownTimeout bounds stopping the ACP processes in Close: actors
	// stop them in parallel and kill those still running after it.
	shutdownTimeout = 10 * time.Second
	// shutdownJoin bounds Close as a whole: the kill, the wait for killed
	// processes to go (goneTimeout), and a margin for the actors to end.
	shutdownJoin = shutdownTimeout + goneTimeout + time.Second
)

// Options configure a Service. Only cmd/ builds them.
type Options struct {
	// HistoryReader is assembled by cmd; nil disables the optional MCP tool.
	HistoryReader func(context.Context, conversation.ReaderScope) []acp.MCPServer

	// StateDir holds hybrid-sessions.json.
	StateDir string
	// Conversations is explicitly assembled by cmd; nil disables durable identity.
	Conversations *conversation.Service
	Manager       *session.Manager
	// ScrollbackBytes bounds each session's chat event log, like a
	// terminal's scrollback (config scrollback_bytes).
	ScrollbackBytes int
	// Version is the WebMux version sent as ACP clientInfo.
	Version string
	// Logger receives lifecycle events, method names and error codes only.
	Logger *slog.Logger
	// Uploads validates that path is one of session id's uploaded files and
	// returns it cleaned (artifacts.Service.UploadPath). Nil refuses every
	// attachment.
	Uploads func(id, path string) (string, error)
	// NextPromptCmd is the optional noninteractive coding agent argv.
	NextPromptCmd []string
}

// timing holds the service's waits; tests shorten them.
type timing struct {
	// interrupt is how long a switch waits for the terminal UI to go idle
	// after the interrupt key before it asks the user (D21).
	interrupt time.Duration
	// probeDelay debounces the eligibility probe after the terminal's
	// agent session or cwd changes.
	probeDelay time.Duration
	// observe is how often a terminal side's identity is read.
	observe time.Duration
	// connectorRecheck is how often a missing connector is looked for
	// again while the terminal side runs.
	connectorRecheck time.Duration
	// bgPoll is how often a finished turn with unfinished tool or background
	// work checks the agent's process tree before starting a status refresh.
	bgPoll time.Duration
}

var defaultTiming = timing{
	interrupt: 10 * time.Second, probeDelay: time.Second, observe: 500 * time.Millisecond,
	connectorRecheck: connectorRecheck, bgPoll: bgPoll,
}

// Service owns the hybrid sessions of one server.
type Service struct {
	historyReader     func(context.Context, conversation.ReaderScope) []acp.MCPServer
	conversations     *conversation.Service
	conversationError error
	manager           *session.Manager
	records           *records
	// options remembers each launch config's agent options for display
	// before a session starts (agent-options.json).
	options         *agentOptions
	capacity        int
	version         string
	log             *slog.Logger
	timing          timing
	uploads         func(id, path string) (string, error)
	nextPromptCmd   []string
	nextPromptSlots chan struct{}
	// agents recognises launch commands and terminal signals.
	agents *agentprofile.Registry
	// userHome is the service user's home directory, where agents keep
	// their state unless the session environment says otherwise.
	userHome string
	// probes holds a token per running eligibility probe (maxProbes).
	probes chan struct{}
	// resumes dedupes resume requests by client request ID.
	resumes dedupe
	// onEvicted cleans up after an auto-evicted record: its artifacts folder
	// and release ref. Nil keeps them.
	onEvicted func(context.Context, Record)

	mu       sync.Mutex
	actors   map[string]*actor
	closed   bool
	starting sync.WaitGroup
	// recovering holds the record IDs a resume or removal is working on.
	recovering map[string]bool
	// writerClaims serializes writer admission for a durable Conversation.
	writerClaims map[string]writerClaim
	// restarted holds the sessions this start found active and interrupted.
	restarted []string
}

// New opens the record store, marks sessions left active by an earlier
// server interrupted (ending their leftover processes), and reserves the
// recorded IDs in the manager. A store that cannot be opened leaves the
// service unavailable for hybrid creation; ordinary sessions keep working.
func New(o Options) *Service {
	log := o.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	capacity := o.ScrollbackBytes
	if capacity <= 0 {
		capacity = defaultScrollbackBytes
	}
	if o.Version == "" {
		o.Version = "dev"
	}
	userHome, _ := os.UserHomeDir()
	s := &Service{
		historyReader: o.HistoryReader, conversations: o.Conversations, manager: o.Manager, records: openRecords(o.StateDir), options: openAgentOptions(o.StateDir), capacity: capacity, version: o.Version, log: log,
		uploads: o.Uploads, userHome: userHome, agents: agents.Default(),
		timing: defaultTiming, probes: make(chan struct{}, maxProbes), actors: make(map[string]*actor),
		recovering:    make(map[string]bool),
		nextPromptCmd: slices.Clone(o.NextPromptCmd), nextPromptSlots: make(chan struct{}, maxNextPromptProcesses),
	}
	restarted, err := s.records.recoverAfterRestart(time.Now())
	if err != nil {
		log.Warn("hybrid sessions unavailable", "error", err)
	}
	s.conversationError = s.migrateConversations()
	if s.conversationError != nil {
		log.Warn("conversation identity migration unavailable")
	}
	s.restarted = restarted
	s.records.setLogger(log)
	s.options.setLogger(log)
	logAgentOptions(log, s.options)
	o.Manager.SetIDReserver(s.records.reserved)
	return s
}

// Unavailable reports why hybrid sessions cannot be created, or nil.
func (s *Service) Unavailable() error {
	if s.conversationError != nil {
		return fmt.Errorf("%w: conversation identity unavailable", ErrUnavailable)
	}
	return s.records.unavailable()
}

// Restarted returns the IDs of the sessions the previous server left
// running: the ones this start marked interrupted. A server that wants them
// back resumes each; sessions the user closed are not among them.
func (s *Service) Restarted() []string { return slices.Clone(s.restarted) }

// OnEvicted sets the cleanup of an auto-evicted record, called after its
// actor and shell ended. The server removes the record's artifacts folder
// and release ref there. Set it before serving.
func (s *Service) OnEvicted(fn func(context.Context, Record)) { s.onEvicted = fn }

// uploadCheck validates attachment paths against session id's uploads.
func (s *Service) uploadCheck(id string) func(string) (string, error) {
	return func(path string) (string, error) {
		if s.uploads == nil {
			return "", errors.New("上传不可用")
		}
		return s.uploads(id, path)
	}
}

func (s *Service) beginOperation() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("%w: 服务已关闭", ErrUnavailable)
	}
	s.starting.Add(1)
	return nil
}

// Close stops every actor and its ACP process without changing records,
// so a restart finds the sessions active and marks them interrupted, then
// releases the store once the actors ended (at most shutdownJoin).
// Terminal runs are left to Manager.Shutdown.
func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	actors := make([]*actor, 0, len(s.actors))
	for _, a := range s.actors {
		actors = append(actors, a)
	}
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	join, cancelJoin := context.WithTimeout(context.Background(), shutdownJoin)
	defer cancelJoin()
	for _, a := range actors {
		a.post(func() { a.shutdown(ctx) })
	}
	for _, a := range actors {
		select {
		case <-a.quit:
		case <-join.Done():
			select {
			case <-a.quit:
			default:
				s.log.Warn("hybrid session did not stop in time", "id", a.id)
			}
		}
	}
	s.starting.Wait()
	s.records.close()
	s.options.close()
}

func (s *Service) unregister(a *actor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.actors[a.id] == a {
		delete(s.actors, a.id)
	}
}

func (s *Service) actor(id string) (*actor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.actors[id]
	if !ok {
		return nil, ErrNotFound
	}
	return a, nil
}

// recoveringNow reports whether id is being resumed or removed right now, so
// auto-eviction must not drop it.
func (s *Service) recoveringNow(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recovering[id]
}

// Subscribe opens a gap-free view of session id's chat: a snapshot, the
// events after resume (or all cached events, with Reset when resume is no
// longer in the log), then live frames. A subscriber that falls behind is
// dropped with ErrSlow.
func (s *Service) Subscribe(id string, resume *ResumePoint) (*Subscription, error) {
	a, err := s.actor(id)
	if err != nil {
		return nil, err
	}
	return a.log.subscribe(resume)
}
