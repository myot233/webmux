package api

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"

	"github.com/rainchan/webmux/internal/artifacts"
	"github.com/rainchan/webmux/internal/codeview"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/files"
	"github.com/rainchan/webmux/internal/forward"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/push"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/settings"
	"github.com/rainchan/webmux/internal/tailscalesettings"
	"github.com/rainchan/webmux/internal/tunnelsettings"
	"github.com/rainchan/webmux/internal/voice"
	"github.com/rainchan/webmux/internal/voicesettings"
	"github.com/rainchan/webmux/internal/wire"
	"github.com/rainchan/webmux/internal/workspace"
)

// Services report transport-neutral error kinds; this file is the only place
// that turns them into HTTP statuses.

func cancelled(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

// sessionStatus maps a session close or removal error: a missing session is
// 404, a running one 409, and any other failure a server error.
func sessionStatus(err error) int {
	switch {
	case errors.Is(err, session.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, session.ErrRunning), errors.Is(err, session.ErrExited),
		errors.Is(err, session.ErrNoRun), errors.Is(err, session.ErrNotController):
		return http.StatusConflict
	case errors.Is(err, session.ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, session.ErrShuttingDown):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// pushStatus maps a push subscription store error: a full store is 409,
// anything else a failed save the caller logs.
func pushStatus(err error) int {
	if errors.Is(err, push.ErrFull) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

// uploadReadStatus classifies a failed request body read: the body limit is
// 413, a read deadline 408, anything else a malformed request.
func uploadReadStatus(err error) int {
	var tooLarge *http.MaxBytesError
	var networkError net.Error
	if errors.As(err, &tooLarge) {
		return http.StatusRequestEntityTooLarge
	} else if errors.As(err, &networkError) && networkError.Timeout() {
		return http.StatusRequestTimeout
	}
	return http.StatusBadRequest
}

// workspaceStatus maps workspace error kinds; cancellation is a timeout and
// anything else a server error.
func workspaceStatus(err error) int {
	switch {
	case errors.Is(err, workspace.ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, workspace.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, workspace.ErrConflict):
		return http.StatusConflict
	case errors.Is(err, workspace.ErrTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, workspace.ErrUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, workspace.ErrTimeout), cancelled(err):
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

// codeStatus maps code view error kinds. Missing files are 404, and the
// workspace size and timeout failures it surfaces keep their statuses.
func codeStatus(err error) int {
	switch {
	case errors.Is(err, codeview.ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, codeview.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, codeview.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, codeview.ErrConflict):
		return http.StatusConflict
	case errors.Is(err, codeview.ErrGone):
		return http.StatusGone
	case errors.Is(err, codeview.ErrTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, codeview.ErrBusy):
		return http.StatusTooManyRequests
	case errors.Is(err, codeview.ErrInternal):
		return http.StatusInternalServerError
	case errors.Is(err, codeview.ErrUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, codeview.ErrTimeout), cancelled(err):
		return http.StatusGatewayTimeout
	case errors.Is(err, os.ErrNotExist):
		return http.StatusNotFound
	case errors.Is(err, workspace.ErrTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, workspace.ErrTimeout):
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

// launchStatus maps launch store errors to HTTP statuses; anything else is a
// failed save.
func launchStatus(err error) int {
	switch {
	case errors.Is(err, launch.ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, launch.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, launch.ErrDuplicate):
		return http.StatusConflict
	case errors.Is(err, launch.ErrUnavailable):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// launchFailure answers err. Store errors name fields and paths, never
// environment values.
func launchFailure(w http.ResponseWriter, err error) {
	status := launchStatus(err)
	if status == http.StatusInternalServerError {
		slog.Error("launch config save failed", "error", err)
	}
	failure(w, status, err)
}

// filesFailure answers a files service error. A failed read of the upload
// body keeps the request-body classification.
func filesFailure(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, files.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, files.ErrTooLarge):
		status = http.StatusRequestEntityTooLarge
	case errors.Is(err, files.ErrRead):
		status = uploadReadStatus(err)
	}
	failure(w, status, err)
}

// artifactsFailure answers an artifacts folder error.
func artifactsFailure(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, artifacts.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, artifacts.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, artifacts.ErrTooLarge):
		status = http.StatusRequestEntityTooLarge
	case errors.Is(err, artifacts.ErrUnavailable):
		status = http.StatusServiceUnavailable
	}
	failure(w, status, err)
}

// hybridStatus maps hybrid session error kinds; ok is false for other
// errors. ErrAgent and ErrAgentCall are bad gateways, never 500s: their
// messages carry agent output for the requesting user and must not be
// logged.
func hybridStatus(err error) (status int, ok bool) {
	switch {
	case errors.Is(err, hybrid.ErrInvalid):
		return http.StatusBadRequest, true
	case errors.Is(err, hybrid.ErrTooLarge):
		return http.StatusRequestEntityTooLarge, true
	case errors.Is(err, hybrid.ErrNotFound):
		return http.StatusNotFound, true
	case errors.Is(err, hybrid.ErrLimit), errors.Is(err, hybrid.ErrConflict),
		errors.Is(err, hybrid.ErrWrongSide), errors.Is(err, hybrid.ErrAnswered),
		errors.Is(err, hybrid.ErrSwitching), errors.Is(err, hybrid.ErrSent),
		errors.Is(err, hybrid.ErrSteeringUnavailable), errors.Is(err, hybrid.ErrBusy),
		errors.Is(err, hybrid.ErrNotEligible),
		errors.Is(err, hybrid.ErrSessionMissing), errors.Is(err, hybrid.ErrConfigMissing),
		errors.Is(err, hybrid.ErrConnectorMissing), errors.Is(err, hybrid.ErrSessionAmbiguous),
		errors.Is(err, hybrid.ErrExternalWriter), errors.Is(err, hybrid.ErrNoChatView),
		errors.Is(err, hybrid.ErrNotActive), errors.Is(err, hybrid.ErrNotArchived),
		errors.Is(err, hybrid.ErrInputFailed):
		return http.StatusConflict, true
	case errors.Is(err, hybrid.ErrAgent), errors.Is(err, hybrid.ErrAgentCall):
		return http.StatusBadGateway, true
	case errors.Is(err, hybrid.ErrUnavailable):
		return http.StatusServiceUnavailable, true
	}
	return 0, false
}

// hybridReason is the contract.SessionConflict reason of a 409 hybrid
// error; empty for a plain conflict.
func hybridReason(err error) string {
	switch {
	case errors.Is(err, hybrid.ErrLimit):
		return "limit"
	case errors.Is(err, hybrid.ErrSwitching):
		return "switching"
	case errors.Is(err, hybrid.ErrWrongSide):
		return "wrong_view"
	case errors.Is(err, hybrid.ErrNoChatView):
		return "no_chat_view"
	case errors.Is(err, hybrid.ErrNotActive):
		return "not_active"
	case errors.Is(err, hybrid.ErrNotArchived):
		return "not_archived"
	case errors.Is(err, hybrid.ErrInputFailed):
		return "input_failed"
	case errors.Is(err, hybrid.ErrAnswered):
		return "answered"
	case errors.Is(err, hybrid.ErrSent):
		return "sent"
	case errors.Is(err, hybrid.ErrSteeringUnavailable):
		return "steering_unavailable"
	case errors.Is(err, hybrid.ErrBusy):
		return "busy"
	case errors.Is(err, hybrid.ErrNotEligible):
		return "not_eligible"
	case errors.Is(err, hybrid.ErrSessionMissing):
		return "session_missing"
	case errors.Is(err, hybrid.ErrConfigMissing):
		return "config_missing"
	case errors.Is(err, hybrid.ErrConnectorMissing):
		return "connector_missing"
	case errors.Is(err, hybrid.ErrSessionAmbiguous):
		return "session_ambiguous"
	case errors.Is(err, hybrid.ErrExternalWriter):
		return "occupied_external"
	}
	return ""
}

// hybridFailure answers a hybrid error kind, with a SessionConflict body
// for 409s that have a reason (not_eligible with its eligibility reason,
// session_missing and session_ambiguous with their candidates,
// connector_missing with its connector); false for other errors.
func hybridFailure(w http.ResponseWriter, err error) bool {
	status, ok := hybridStatus(err)
	if !ok {
		return false
	}
	if reason := hybridReason(err); reason != "" {
		conflict := contract.SessionConflict{Error: err.Error(), Reason: reason}
		var ineligible *hybrid.IneligibleError
		if errors.As(err, &ineligible) {
			conflict.Eligibility = ineligible.Reason
		}
		var missing *hybrid.SessionMissingError
		if errors.As(err, &missing) {
			conflict.Candidates = wire.AgentCandidates(missing.Candidates)
		}
		var ambiguous *hybrid.SessionAmbiguousError
		if errors.As(err, &ambiguous) {
			conflict.Candidates = wire.AgentCandidates(ambiguous.Candidates)
		}
		var connector *hybrid.ConnectorMissingError
		if errors.As(err, &connector) {
			conflict.Connector = wire.AgentConnector(connector.Connector)
		}
		var input *hybrid.InputFailedError
		if errors.As(err, &input) {
			submission := wire.SubmissionStatus(input.Status)
			conflict.Submission = &submission
		}
		reply(w, status, conflict)
		return true
	}
	failure(w, status, err)
	return true
}

// sessionCommandFailure answers a failed session or chat command: hybrid
// kinds with their SessionConflict, session errors with their statuses; a
// request that ended first is a timeout.
func sessionCommandFailure(w http.ResponseWriter, err error) {
	var bad *badRequest
	switch {
	case errors.As(err, &bad):
		failure(w, http.StatusBadRequest, bad.err)
	case hybridFailure(w, err):
	case errors.Is(err, session.ErrNotFound), errors.Is(err, session.ErrRunning), errors.Is(err, session.ErrExited),
		errors.Is(err, session.ErrNoRun), errors.Is(err, session.ErrNotController),
		errors.Is(err, session.ErrInvalid), errors.Is(err, session.ErrShuttingDown):
		failure(w, sessionStatus(err), err)
	case cancelled(err):
		failure(w, http.StatusGatewayTimeout, err)
	default:
		failure(w, http.StatusInternalServerError, err)
	}
}

// renameFailure answers a session rename: session errors, plus the hybrid
// statuses of a record that cannot be written.
func renameFailure(w http.ResponseWriter, err error) {
	if hybridFailure(w, err) {
		return
	}
	failure(w, sessionStatus(err), err)
}

// sessionFailure answers a session start: a saved launch config keeps its
// launch statuses and a hybrid session its hybrid statuses (a full record
// store is a 409 with reason limit); everything else is a workspace error.
func sessionFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, launch.ErrInvalid) || errors.Is(err, launch.ErrNotFound) || errors.Is(err, launch.ErrDuplicate) || errors.Is(err, launch.ErrUnavailable) {
		launchFailure(w, err)
		return
	}
	if hybridFailure(w, err) {
		return
	}
	failure(w, workspaceStatus(err), err)
}

func workspaceFailure(w http.ResponseWriter, err error) {
	failure(w, workspaceStatus(err), err)
}

func tunnelStatus(err error) int {
	switch {
	case errors.Is(err, tunnelsettings.ErrNotPersisted):
		return http.StatusServiceUnavailable
	case errors.Is(err, tunnelsettings.ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, tunnelsettings.ErrUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, tunnelsettings.ErrEnableBlocked):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func tunnelFailure(w http.ResponseWriter, err error) {
	status := tunnelStatus(err)
	message := err.Error()
	if status == http.StatusInternalServerError {
		message = "tunnel settings operation failed"
	}
	reply(w, status, contract.TunnelSettingsError{Error: message, NotPersisted: errors.Is(err, tunnelsettings.ErrNotPersisted)})
}

// localTunnelFailure preserves the local callback's refusal contract while
// keeping every service-error status decision in this file.
func localTunnelFailure(w http.ResponseWriter, err error) {
	if tunnelStatus(err) == http.StatusInternalServerError {
		failure(w, http.StatusConflict, err)
		return
	}
	tunnelFailure(w, err)
}

func forwardRedeemStatus(err error) int {
	if errors.Is(err, forward.ErrUnauthorized) {
		return http.StatusUnauthorized
	}
	return http.StatusInternalServerError
}

func voiceStatus(err error) int {
	if errors.Is(err, voice.ErrDisabled) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

// tailscaleStatus maps settings errors; a shutdown or other refusal is 409.
func tailscaleStatus(err error) int {
	switch {
	case errors.Is(err, tailscalesettings.ErrInvalid), errors.Is(err, tailscalesettings.ErrFunnelUnconfirmed):
		return http.StatusBadRequest
	case errors.Is(err, tailscalesettings.ErrUnavailable):
		return http.StatusServiceUnavailable
	default:
		return http.StatusConflict
	}
}

// voiceSettingsStatus maps recognizer settings errors; an unreadable store
// is 503, anything else a rejected value.
func voiceSettingsStatus(err error) int {
	switch {
	case errors.Is(err, voicesettings.ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, voicesettings.ErrUnavailable):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func settingsStatus(err error) int {
	switch {
	case errors.Is(err, settings.ErrInvalid):
		return 400
	case errors.Is(err, settings.ErrConflict):
		return 409
	case errors.Is(err, settings.ErrUnavailable):
		return 503
	default:
		return 500
	}
}

// forwardFailure maps forward registry errors: a bad port or option is 400,
// a port already forwarded 409, a missing forward 404.
func forwardFailure(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, forward.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, forward.ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, forward.ErrNotFound):
		status = http.StatusNotFound
	}
	failure(w, status, err)
}

// createStatus maps the error of a create step like the endpoint of that
// step does: launch configs and hybrid sessions keep their statuses, the
// rest are workspace errors.
func createStatus(err error) int {
	if errors.Is(err, launch.ErrInvalid) || errors.Is(err, launch.ErrNotFound) || errors.Is(err, launch.ErrDuplicate) || errors.Is(err, launch.ErrUnavailable) {
		return launchStatus(err)
	}
	if status, ok := hybridStatus(err); ok {
		return status
	}
	return workspaceStatus(err)
}

// createFailure answers a failed create with its step, what it had
// created, and the SessionConflict details of a hybrid conflict; Reason is
// "conflict" for an existing branch or path.
func createFailure(w http.ResponseWriter, err error) {
	body := contract.CreateSessionFailure{Step: workspace.StepPrecheck}
	var failed *workspace.CreateError
	if errors.As(err, &failed) {
		body.Step, body.Worktree, body.SessionID = failed.Step, wire.OptionalWorktree(failed.Worktree), failed.SessionID
		err = failed.Err
	}
	status := createStatus(err)
	body.Error = err.Error()
	if status == http.StatusInternalServerError {
		slog.Error("session create failed", "step", body.Step, "error", err)
		body.Error = http.StatusText(status)
	}
	if _, ok := hybridStatus(err); ok {
		body.Reason = hybridReason(err)
		var ineligible *hybrid.IneligibleError
		if errors.As(err, &ineligible) {
			body.Eligibility = ineligible.Reason
		}
		var connector *hybrid.ConnectorMissingError
		if errors.As(err, &connector) {
			body.Connector = wire.AgentConnector(connector.Connector)
		}
	} else if errors.Is(err, workspace.ErrConflict) {
		body.Reason = "conflict"
	}
	reply(w, status, body)
}

func conversationFailure(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, conversation.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, conversation.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, conversation.ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, conversation.ErrUnavailable):
		status = http.StatusServiceUnavailable
	case cancelled(err):
		status = http.StatusGatewayTimeout
	}
	failure(w, status, err)
}
