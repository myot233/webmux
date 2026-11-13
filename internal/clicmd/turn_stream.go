package clicmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/sessionobserve"
)

// turnResult is the outcome of one tracked turn.
type turnResult struct {
	StopReason string               `json:"stopReason"`
	Error      string               `json:"error,omitempty"`
	Reply      string               `json:"reply"`
	Events     []contract.ChatEvent `json:"events"`
	// Retained internally until the guide/output layer exposes the field.
	Unfinished bool `json:"-"`
	// Request is an open permission request or form when the wait stopped
	// for it.
	Request *contract.ChatRequest `json:"request,omitempty"`
}

// cliTurn keeps the CLI's presentation contract separate from observation.
func cliTurn(observed sessionobserve.TurnObservation) turnResult {
	result := turnResult{
		StopReason: observed.StopReason,
		Error:      observed.Error,
		Reply:      strings.TrimSpace(observed.Reply),
		Events:     observed.Events,
		Unfinished: observed.Unfinished,
	}
	if len(observed.Requests) != 0 {
		result.Request = &observed.Requests[0]
	}
	return result
}

// reportTurn prints a tracked turn and returns the exit status:
// end_turn is 0, a stop for a request or any other reason is 1.
func (e Env) reportTurn(result turnResult, asJSON bool) int {
	if asJSON {
		writeJSON(e.Stdout, result)
	} else if result.Reply != "" {
		_, _ = fmt.Fprintln(e.Stdout, result.Reply)
	}
	if result.Request != nil {
		_, _ = fmt.Fprintf(e.Stderr, "webmux: the agent is waiting for an answer: request %s (%s) %s\n",
			result.Request.RequestID, result.Request.Kind, result.Request.Title)
		for _, option := range result.Request.Options {
			_, _ = fmt.Fprintf(e.Stderr, "webmux:   --option %s\t%s\n", option.OptionID, option.Name)
		}
		_, _ = fmt.Fprintln(e.Stderr, "webmux: answer with webmux chat answer SESSION", result.Request.RequestID, "(see webmux chat status), or in the Web UI")
		return contract.ExitError
	}
	if result.StopReason != "end_turn" {
		reason := result.StopReason
		if result.Error != "" {
			reason += ": " + result.Error
		}
		_, _ = fmt.Fprintln(e.Stderr, "webmux: the turn ended with", reason)
		return contract.ExitError
	}
	return contract.ExitOK
}

func writeJSON(w io.Writer, v any) {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(v)
}
