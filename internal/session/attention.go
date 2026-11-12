package session

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rainchan/webmux/internal/attentionkind"
	"github.com/rainchan/webmux/internal/contract"
)

type AttentionSource = contract.AttentionSource

const (
	SourceAgent    = contract.AttentionSourceAgent
	SourceTerminal = contract.AttentionSourceTerminal
	SourceCLI      = contract.AttentionSourceCLI
)

// bellQuietPeriod ignores bells that answer a keystroke, such as shell completion.
const bellQuietPeriod = time.Second

// AttentionMode selects which automatic attention signals a session allows.
type AttentionMode string

const (
	AttentionModeAll    AttentionMode = "all"
	AttentionModeUrgent AttentionMode = "urgent"
	AttentionModeOff    AttentionMode = "off"
)

// AttentionPolicy overrides the server's global attention settings for one
// session. A nil *AttentionPolicy on CreateInput uses the manager's defaults.
type AttentionPolicy struct {
	Mode          AttentionMode
	ProgramStatus bool
	AgentProtocol bool
	Notify        bool
	Bell          bool
	MinTurn       time.Duration
	DoneDelay     time.Duration
}

// DefaultAttentionPolicy returns the default per-session attention policy with
// all signal sources enabled and no turn threshold or delay.
func DefaultAttentionPolicy() AttentionPolicy {
	return AttentionPolicy{
		Mode:          AttentionModeAll,
		ProgramStatus: true,
		AgentProtocol: true,
		Notify:        true,
		Bell:          true,
	}
}

func isNonUrgentKind(kind attentionkind.Kind) bool {
	switch kind {
	case attentionkind.Done, attentionkind.Notify, attentionkind.Bell:
		return true
	}
	return false
}

func (s *Session) attentionMode() AttentionMode {
	if s.attentionPolicy.Mode == "" {
		return AttentionModeAll
	}
	return s.attentionPolicy.Mode
}

func (s *Session) allowsAttentionKindLocked(kind attentionkind.Kind) bool {
	switch s.attentionMode() {
	case AttentionModeOff:
		return false
	case AttentionModeUrgent:
		return !isNonUrgentKind(kind)
	default:
		return true
	}
}

func (s *Session) startTurnLocked(now time.Time) {
	s.turnStartedAt = now
	s.cancelPendingAttentionLocked()
}

func (s *Session) cancelPendingAttentionLocked() {
	if s.doneDelayTimer != nil {
		s.doneDelayTimer.Stop()
		s.doneDelayTimer = nil
	}
	s.pendingAttention = nil
}

func (s *Session) passTurnThresholdLocked(kind attentionkind.Kind, now time.Time) bool {
	if !isNonUrgentKind(kind) {
		s.cancelPendingAttentionLocked()
		return true
	}
	if s.attentionPolicy.MinTurn > 0 && !s.turnStartedAt.IsZero() && now.Sub(s.turnStartedAt) < s.attentionPolicy.MinTurn {
		return false
	}
	return true
}

func (s *Session) commitOrScheduleAttentionLocked(next *Attention, now time.Time) {
	if !isNonUrgentKind(next.Kind) || s.attentionPolicy.DoneDelay <= 0 {
		s.cancelPendingAttentionLocked()
		s.info.Attention = next
		return
	}
	if s.pendingAttention != nil {
		if s.pendingAttention.layer > next.layer || attentionRank(next.Kind) < attentionRank(s.pendingAttention.Kind) {
			return
		}
		if s.pendingAttention.Kind == next.Kind {
			s.pendingAttention.Source, s.pendingAttention.layer, s.pendingAttention.Message = next.Source, next.layer, ""
			return
		}
	}
	if s.doneDelayTimer != nil {
		s.doneDelayTimer.Stop()
	}
	s.pendingAttention = next
	delay := s.attentionPolicy.DoneDelay
	var timer *time.Timer
	timer = time.AfterFunc(delay, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.doneDelayTimer != timer || s.pendingAttention == nil || s.info.ExitCode != nil {
			return
		}
		s.doneDelayTimer = nil
		pending := s.pendingAttention
		s.pendingAttention = nil
		pending.Since = time.Now()
		if s.program != nil && pending.layer < layerProgram {
			return
		}
		if current := s.info.Attention; current != nil {
			if current.layer > pending.layer || attentionRank(pending.Kind) < attentionRank(current.Kind) {
				return
			}
			if current.Kind == pending.Kind {
				current.Source, current.layer, current.Message = pending.Source, pending.layer, ""
				return
			}
		}
		s.info.Attention = pending
		if s.run != nil {
			s.info.Status = s.outputStatusLocked()
		}
		if len(s.subs) > 0 {
			s.broadcastLocked(s.stateLocked())
		}
	})
	s.doneDelayTimer = timer
}

// Attention latches a signal that the session needs the user. Output never
// clears it; only user input, an agent's own status or resume event, or exit
// does.
type Attention struct {
	Kind   attentionkind.Kind
	Source AttentionSource
	Since  time.Time
	// Message is the text a cli source attached; it lives only in memory.
	Message string
	// layer records which signal source raised it; it is merge policy only
	// and never leaves the session package.
	layer signalLayer
}

// signalLayer orders signal sources: a weaker layer never replaces or clears
// attention raised by a stronger one, and an OSC 7501 root record silences
// every other layer while it exists.
type signalLayer int

const (
	layerBell    signalLayer = iota + 1 // bare BEL
	layerNotify                         // OSC 9 / OSC 777 notifications
	layerAgent                          // Warp cli-agent events
	layerProgram                        // OSC 7501 root record
)

// programRecord is the OSC 7501 root record: only state and kind are kept.
type programRecord struct {
	state, kind string
}

func attentionRank(kind attentionkind.Kind) int {
	switch kind {
	case attentionkind.Question, attentionkind.Permission:
		return 4
	case attentionkind.Error:
		return 3
	case attentionkind.Done:
		return 2
	case attentionkind.Notify:
		return 1
	}
	return 0
}

// raiseAttentionLocked merges a signal below the program layer: it is ignored
// while a root record exists or when the current attention comes from a
// stronger layer, and otherwise keeps a stronger existing kind. Since changes
// only when the kind does.
func (s *Session) raiseAttentionLocked(kind attentionkind.Kind, source AttentionSource, layer signalLayer, now time.Time) {
	if s.program != nil {
		return
	}
	if !s.allowsAttentionKindLocked(kind) || !s.passTurnThresholdLocked(kind, now) {
		return
	}
	current := s.info.Attention
	if current != nil {
		if current.layer > layer || attentionRank(kind) < attentionRank(current.Kind) {
			return
		}
		if current.Kind == kind {
			current.Source, current.layer, current.Message = source, layer, ""
			return
		}
	}
	s.commitOrScheduleAttentionLocked(&Attention{Kind: kind, Source: source, Since: now, layer: layer}, now)
}

// programStatusLocked applies an OSC 7501 root record report. Only a change
// of the record moves attention, so a repeated report never revives attention
// the user already cleared; a change replaces attention whatever its rank.
func (s *Session) programStatusLocked(state, kind string, now time.Time) {
	s.structured = true
	if state == "clear" {
		if s.program != nil {
			s.program = nil
			s.info.Attention = nil
			s.cancelPendingAttentionLocked()
		}
		return
	}
	next := programRecord{state: state, kind: kind}
	if s.program != nil && *s.program == next {
		return
	}
	wasWorking := s.program != nil && s.program.state == "working"
	s.program = &next
	switch state {
	case "blocked":
		if kind == "permission" {
			s.setProgramAttentionLocked(attentionkind.Permission, now)
		} else {
			s.setProgramAttentionLocked(attentionkind.Question, now)
		}
	case "done":
		s.setProgramAttentionLocked(attentionkind.Done, now)
	case "error":
		s.setProgramAttentionLocked(attentionkind.Error, now)
	case "working":
		s.info.Attention = nil
		if !wasWorking {
			s.startTurnLocked(now)
		} else {
			s.cancelPendingAttentionLocked()
		}
	default: // idle
		s.info.Attention = nil
		s.cancelPendingAttentionLocked()
	}
}

func (s *Session) setProgramAttentionLocked(kind attentionkind.Kind, now time.Time) {
	if !s.allowsAttentionKindLocked(kind) || !s.passTurnThresholdLocked(kind, now) {
		s.info.Attention = nil
		return
	}
	if current := s.info.Attention; current != nil && current.Kind == kind {
		current.Source, current.layer, current.Message = SourceAgent, layerProgram, ""
		return
	}
	s.commitOrScheduleAttentionLocked(&Attention{Kind: kind, Source: SourceAgent, Since: now, layer: layerProgram}, now)
}

// dropProgramLocked forgets the root record and the attention it raised.
func (s *Session) dropProgramLocked() {
	s.program = nil
	if s.info.Attention != nil && s.info.Attention.layer == layerProgram {
		s.info.Attention = nil
	}
	if s.pendingAttention != nil && s.pendingAttention.layer == layerProgram {
		s.cancelPendingAttentionLocked()
	}
}

// promptLocked handles OSC 133;A: a new shell prompt ends a working or
// blocked program, while done and error outlive it.
func (s *Session) promptLocked() {
	if s.program != nil && (s.program.state == "working" || s.program.state == "blocked") {
		s.dropProgramLocked()
	}
}

func (s *Session) agentEventLocked(event string, now time.Time) {
	s.agentEventWithSessionLocked(event, "", now)
}

func (s *Session) agentEventWithSessionLocked(event, sessionID string, now time.Time) {
	switch event {
	case "prompt_submit":
		if sessionID != "" {
			s.warpRootSessionID = sessionID
		}
	case "session_start":
		if sessionID != "" && s.warpRootSessionID == "" {
			s.warpRootSessionID = sessionID
		}
	case "stop", "idle_prompt", "stop_failure", "subagent_start":
		if s.warpRootSessionID != "" && sessionID != "" && sessionID != s.warpRootSessionID {
			return
		}
	}
	s.structured = true
	switch event {
	case "question_asked":
		s.raiseAttentionLocked(attentionkind.Question, SourceAgent, layerAgent, now)
	case "permission_request":
		s.raiseAttentionLocked(attentionkind.Permission, SourceAgent, layerAgent, now)
	case "stop", "idle_prompt":
		s.raiseAttentionLocked(attentionkind.Done, SourceAgent, layerAgent, now)
	case "stop_failure":
		s.raiseAttentionLocked(attentionkind.Error, SourceAgent, layerAgent, now)
	case "prompt_submit":
		s.startTurnLocked(now)
		if s.program == nil {
			s.info.Attention = nil
		}
	case "permission_replied", "session_start":
		s.cancelPendingAttentionLocked()
		if s.program == nil {
			s.info.Attention = nil
		}
	default:
		// Ongoing work events (such as tool_call or subagent_start) cancel
		// any pending delayed done attention without clearing active urgent
		// questions/permissions.
		s.cancelPendingAttentionLocked()
	}
}

func (s *Session) bellLocked(now time.Time) {
	if s.bellAttention && now.Sub(s.lastInputAt) >= bellQuietPeriod {
		s.raiseAttentionLocked(attentionkind.Bell, SourceTerminal, layerBell, now)
	}
}

func (s *Session) signalHandler() *SignalHandler {
	hostname, _ := os.Hostname()
	h := &SignalHandler{
		Title: func(title string) {
			if s.userTitle == "" {
				s.info.PlaceholderTitle = title == ""
				if title == "" {
					title = filepath.Base(s.info.Cwd)
				}
				s.info.Title = title
			}
		},
		Bell: func() { s.bellLocked(time.Now()) },
		Cwd: func(path, host string) {
			remote := !reportedHostIsLocal(host, hostname)
			current := s.info.ReportedCwd
			if current != nil && current.Path == path && current.Host == host && current.Remote == remote {
				// A prompt in the same directory is not a state change.
				return
			}
			s.info.ReportedCwd = &ReportedCwd{Path: path, Host: host, Remote: remote, Since: time.Now()}
		},
		// Identity is collected for every session, whether or not it
		// answers OSC 7501; it never affects attention or status.
		Identity: s.identity.mergeSignals,
	}
	if s.attentionPolicy.Notify {
		h.Notify = func() { s.raiseAttentionLocked(attentionkind.Notify, SourceTerminal, layerNotify, time.Now()) }
	}
	if s.attentionPolicy.AgentProtocol {
		h.Event = func(event string) {
			s.agentEventWithSessionLocked(event, s.lastEventSessionID, time.Now())
			s.lastEventSessionID = ""
		}
		h.Identity = func(id Identity) {
			s.lastEventSessionID = id.WarpSessionID
			s.identity.mergeSignals(id)
		}
	}
	if s.programStatus {
		h.ProgramStatus = func(state, kind string) { s.programStatusLocked(state, kind, time.Now()) }
		h.Probe = s.queueProbeReplyLocked
		h.Prompt = s.promptLocked
		h.Reset = s.dropProgramLocked
	}
	return h
}

// userInputLocked records accepted input; terminal replies leave state alone.
func (s *Session) userInputLocked(p []byte, now time.Time) {
	if terminalReplyOnly(p) {
		return
	}
	s.recordInputLocked(now)
}

// recordInputLocked records input known to come from the user.
func (s *Session) recordInputLocked(now time.Time) {
	s.lastInputAt = now
	s.startTurnLocked(now)
	s.info.Attention = nil
	s.cliAttention = nil
}

// terminalReplyOnly reports input made solely of sequences a terminal emits on
// its own: OSC and DCS strings, and CSI reports ending in c, R, n, I, O, t or
// $y (device attributes, cursor position, status, focus, window size, mode
// state). Incomplete sequences and anything else, including mouse reports,
// count as user input.
func terminalReplyOnly(p []byte) bool {
	for i := 0; i < len(p); {
		if p[i] != 27 || i+1 >= len(p) {
			return false
		}
		switch p[i+1] {
		case '[':
			j := i + 2
			for j < len(p) && p[j] >= 0x20 && p[j] <= 0x3f {
				j++
			}
			if j >= len(p) {
				return false
			}
			// DECRQM replies end in "$y"; replayed history makes xterm.js
			// answer every mode query again whenever a browser attaches.
			report := strings.ContainsRune("cRnIOt", rune(p[j])) || p[j] == 'y' && p[j-1] == '$'
			if !report {
				return false
			}
			i = j + 1
		case ']', 'P':
			end := stringEnd(p, i+2, p[i+1] == ']')
			if end < 0 {
				return false
			}
			i = end
		default:
			return false
		}
	}
	return true
}

// stringEnd returns the index after an ST (or BEL for OSC) terminator, or -1.
func stringEnd(p []byte, from int, bel bool) int {
	for j := from; j < len(p); j++ {
		if bel && p[j] == 7 {
			return j + 1
		}
		if p[j] == 27 && j+1 < len(p) && p[j+1] == '\\' {
			return j + 2
		}
	}
	return -1
}
