package api

import (
	"errors"
	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/launch"
	"net/http"
)

func conversationFailure(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, conversation.ErrInvalid), errors.Is(err, launch.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, conversation.ErrNotFound), errors.Is(err, launch.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, conversation.ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, conversation.ErrUnavailable), errors.Is(err, launch.ErrUnavailable):
		status = http.StatusServiceUnavailable
	case cancelled(err):
		status = http.StatusGatewayTimeout
	}
	failure(w, status, err)
}
