package hybrid

import "errors"

// Error kinds; only internal/api/errors.go maps them to HTTP statuses.
var (
	// ErrNotFound: no hybrid session or record with that ID (404).
	ErrNotFound = errors.New("混合会话不存在")
	// ErrUnavailable: hybrid-sessions.json is locked, corrupt or unreadable,
	// or the service is shut down (503).
	ErrUnavailable = errors.New("混合会话不可用")
	// ErrLimit: the record limit is reached (409).
	ErrLimit = errors.New("混合会话记录已达上限，请先移除旧记录")
	// ErrConflict: the request conflicts with current state (409).
	ErrConflict = errors.New("混合会话状态冲突")
	// ErrInvalid: the request is malformed (400).
	ErrInvalid = errors.New("混合会话请求无效")
	// ErrTooLarge: an attachment exceeds the size limits (413).
	ErrTooLarge = errors.New("混合会话附件过大")
	// ErrWrongSide: the command needs the other side, e.g. a prompt while
	// the session is in the terminal (409 wrong_side).
	ErrWrongSide = errors.New("混合会话不在该端")
	// ErrAnswered: the request was already answered or withdrawn (409
	// answered).
	ErrAnswered = errors.New("该请求已被答复")
	// ErrSwitching: a switch is running; commands wait for it (409
	// switching).
	ErrSwitching = errors.New("混合会话正在切换")
	// ErrNotEligible: switching to chat is not possible now; the error is
	// an *IneligibleError with the reason code (409 not_eligible).
	ErrNotEligible = errors.New("现在不能切到聊天")
	// ErrSent: the queued prompt was already sent or removed (409 sent).
	ErrSent = errors.New("该排队消息已发送或已删除")
	// ErrSteeringUnavailable: steering needs a profile and agent that
	// support it and a running turn (409 steering_unavailable).
	ErrSteeringUnavailable = errors.New("当前不能插话")
	// ErrAgent: the agent failed to start or to open its session (502). The
	// message may carry the agent's stderr tail, for the requesting user
	// only: never log it.
	ErrAgent = errors.New("agent 启动失败")
	// ErrAgentCall: the agent refused or failed a command such as a
	// configuration change or steering (502).
	ErrAgentCall = errors.New("agent 未能完成请求")
	// ErrSessionMissing: the recorded agent session is no longer listed in
	// its directory; the error is a *SessionMissingError offering the
	// directory's other sessions (409 session_missing).
	ErrSessionMissing = errors.New("agent 会话已不存在，可以改选同一目录下的其他会话，或放弃恢复")
	// ErrConfigMissing: the record's launch config was deleted; resuming
	// again with confirmation runs without its environment (409
	// config_missing).
	ErrConfigMissing = errors.New("启动配置已删除，确认后将不带配置中的环境变量恢复")
)
