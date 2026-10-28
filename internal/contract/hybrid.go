package contract

import "time"

// Hybrid sessions move one agent conversation between a terminal UI (side
// "tui") and an ACP chat view (side "acp"). See the add-acp-hybrid-sessions
// OpenSpec change for the behaviour behind these shapes.

// HybridSide is where a hybrid session currently runs.
type HybridSide string

const (
	HybridSideTUI       HybridSide = "tui"
	HybridSideACP       HybridSide = "acp"
	HybridSideSwitching HybridSide = "switching"
)

// HybridState is the lifecycle of a hybrid session record.
type HybridState string

const (
	HybridStateActive      HybridState = "active"
	HybridStateClosed      HybridState = "closed"
	HybridStateInterrupted HybridState = "interrupted"
)

// HybridPhase is the current step of a switch; empty outside a switch.
type HybridPhase string

const (
	HybridPhaseInterrupting HybridPhase = "interrupting"
	// HybridPhaseAwaitingChoice: the TUI did not return to idle within the
	// interrupt timeout; the user chooses wait, force or cancel.
	HybridPhaseAwaitingChoice HybridPhase = "awaiting_choice"
	HybridPhaseCancelling     HybridPhase = "cancelling"
	HybridPhaseStopping       HybridPhase = "stopping"
	HybridPhaseStarting       HybridPhase = "starting"
	HybridPhaseLoading        HybridPhase = "loading"
	HybridPhaseRollingBack    HybridPhase = "rolling_back"
)

// SessionHybrid is SessionInfo.hybrid; ordinary sessions omit it.
type SessionHybrid struct {
	Side    HybridSide  `json:"side"`
	Phase   HybridPhase `json:"phase,omitempty"`
	Dormant bool        `json:"dormant"`
	State   HybridState `json:"state"`
	Agent   string      `json:"agent"`
	// AgentSessionID is null until the agent's own session ID is known.
	AgentSessionID *string `json:"agentSessionId"`
}

// CloseMoved is the WebSocket close code for "the session moved to its
// other side". It is terminal: clients do not reconnect.
const CloseMoved = 4409

// HybridSwitchRequest is the body of POST /api/sessions/{id}/switch.
// Choice answers an interrupt timeout ("wait", "force" or "cancel") instead
// of starting a switch.
type HybridSwitchRequest struct {
	ClientRequestID string     `json:"clientRequestId"`
	To              HybridSide `json:"to,omitempty"`
	Choice          string     `json:"choice,omitempty"`
}

// HybridSwitchResult answers a completed switch request. Draft is the
// merged text of queued prompts returned to the requesting device when
// moving to the terminal; Dropped lists launch arguments the resume command
// did not keep; Unsupported lists arguments the chat side cannot apply.
type HybridSwitchResult struct {
	Side        HybridSide `json:"side"`
	Draft       string     `json:"draft,omitempty"`
	Dropped     []string   `json:"dropped"`
	Unsupported []string   `json:"unsupported"`
}

// HybridResumeRequest is the body of POST /api/sessions/{id}/resume.
// AgentSessionID picks a candidate after a 409 listing them;
// ConfirmWithoutEnv accepts resuming after the launch config was deleted.
type HybridResumeRequest struct {
	ClientRequestID   string     `json:"clientRequestId"`
	Side              HybridSide `json:"side"`
	AgentSessionID    string     `json:"agentSessionId,omitempty"`
	ConfirmWithoutEnv bool       `json:"confirmWithoutEnv,omitempty"`
}

// HybridRecord is one entry of GET /api/hybrid-sessions: the persisted link
// between a WebMux session ID and an agent session. It never carries
// environment values, messages or queued text.
type HybridRecord struct {
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	ConfigID       string      `json:"configId"`
	ConfigMissing  bool        `json:"configMissing"`
	Agent          string      `json:"agent"`
	Cwd            string      `json:"cwd"`
	Cmd            []string    `json:"cmd"`
	Side           HybridSide  `json:"side"`
	AgentSessionID *string     `json:"agentSessionId"`
	PreviousIDs    []string    `json:"previousIds"`
	State          HybridState `json:"state"`
	// Activity is what an interrupted session was doing: idle, turn or
	// switching; a turn or switch may not have finished.
	Activity string `json:"activity,omitempty"`
	// QueuedCount is how many queued prompts were lost when the session was
	// interrupted; the text itself is never stored.
	QueuedCount   int        `json:"queuedCount"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	InterruptedAt *time.Time `json:"interruptedAt"`
}

// HybridCandidate is an agent session offered when a record's agent
// session ID can no longer be found.
type HybridCandidate struct {
	AgentSessionID string    `json:"agentSessionId"`
	Title          string    `json:"title"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// HybridConflict is the 409 body of hybrid and chat commands. Reason is a
// stable code: switching, wrong_side, not_eligible, answered, sent,
// steering_unavailable, session_missing, config_missing or limit.
// Eligibility carries the not_eligible cause; Candidates the session_missing
// alternatives.
type HybridConflict struct {
	Error       string            `json:"error"`
	Reason      string            `json:"reason"`
	Eligibility string            `json:"eligibility,omitempty"`
	Candidates  []HybridCandidate `json:"candidates,omitempty"`
}
