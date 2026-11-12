package api

import (
	"errors"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/wire"
	"github.com/rainchan/webmux/internal/workspace"
	"log/slog"
	"net/http"
)

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
