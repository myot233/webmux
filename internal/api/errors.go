package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/rainchan/webmux/internal/codeview"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/files"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/settings"
	"github.com/rainchan/webmux/internal/tailscalesettings"
	"github.com/rainchan/webmux/internal/tunnelsettings"
	"github.com/rainchan/webmux/internal/wire"
	"github.com/rainchan/webmux/internal/workspace"
)

// Services report transport-neutral error kinds; this file is the only place
// that turns them into HTTP statuses.

func cancelled(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
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
		errors.Is(err, hybrid.ErrSteeringUnavailable), errors.Is(err, hybrid.ErrNotEligible),
		errors.Is(err, hybrid.ErrSessionMissing), errors.Is(err, hybrid.ErrConfigMissing):
		return http.StatusConflict, true
	case errors.Is(err, hybrid.ErrAgent), errors.Is(err, hybrid.ErrAgentCall):
		return http.StatusBadGateway, true
	case errors.Is(err, hybrid.ErrUnavailable):
		return http.StatusServiceUnavailable, true
	}
	return 0, false
}

// hybridReason is the contract.HybridConflict reason of a 409 hybrid
// error; empty for a plain conflict.
func hybridReason(err error) string {
	switch {
	case errors.Is(err, hybrid.ErrLimit):
		return "limit"
	case errors.Is(err, hybrid.ErrSwitching):
		return "switching"
	case errors.Is(err, hybrid.ErrWrongSide):
		return "wrong_side"
	case errors.Is(err, hybrid.ErrAnswered):
		return "answered"
	case errors.Is(err, hybrid.ErrSent):
		return "sent"
	case errors.Is(err, hybrid.ErrSteeringUnavailable):
		return "steering_unavailable"
	case errors.Is(err, hybrid.ErrNotEligible):
		return "not_eligible"
	case errors.Is(err, hybrid.ErrSessionMissing):
		return "session_missing"
	case errors.Is(err, hybrid.ErrConfigMissing):
		return "config_missing"
	}
	return ""
}

// hybridFailure answers a hybrid error kind, with a HybridConflict body
// for 409s that have a reason (not_eligible with its eligibility reason,
// session_missing with its candidates); false for other errors.
func hybridFailure(w http.ResponseWriter, err error) bool {
	status, ok := hybridStatus(err)
	if !ok {
		return false
	}
	if reason := hybridReason(err); reason != "" {
		conflict := contract.HybridConflict{Error: err.Error(), Reason: reason}
		var ineligible *hybrid.IneligibleError
		if errors.As(err, &ineligible) {
			conflict.Eligibility = ineligible.Reason
		}
		var missing *hybrid.SessionMissingError
		if errors.As(err, &missing) {
			conflict.Candidates = wire.HybridCandidates(missing.Candidates)
		}
		reply(w, status, conflict)
		return true
	}
	failure(w, status, err)
	return true
}

// chatFailure answers a failed chat command; a request that ended first is
// a timeout.
func chatFailure(w http.ResponseWriter, err error) {
	switch {
	case hybridFailure(w, err):
	case cancelled(err):
		failure(w, http.StatusGatewayTimeout, err)
	default:
		failure(w, http.StatusInternalServerError, err)
	}
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

func workspaceFailure(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	failure(w, workspaceStatus(err), err)
	return true
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
