package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/push"
	"github.com/rainchan/webmux/internal/session"
)

func TestFailureSanitizesInternalErrors(t *testing.T) {
	w := httptest.NewRecorder()
	failure(w, http.StatusInternalServerError, errors.New("database path /secret leaked"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "/secret") {
		t.Fatalf("internal error leaked: %s", body)
	}
	if !strings.Contains(body, `"error":"Internal Server Error"`) {
		t.Fatalf("body = %s", body)
	}
}

func TestFailureKeepsUnavailableReason(t *testing.T) {
	w := httptest.NewRecorder()
	failure(w, http.StatusServiceUnavailable, errors.New("启动配置不可用: 服务未启用"))

	body := w.Body.String()
	if !strings.Contains(body, "启动配置不可用") {
		t.Fatalf("body = %s", body)
	}
	if strings.Contains(body, "Service Unavailable") {
		t.Fatalf("controlled 503 was replaced: %s", body)
	}
}

func TestSessionStatusDoesNotReportFailuresAsMissing(t *testing.T) {
	for _, row := range []struct {
		err  error
		want int
	}{
		{session.ErrNotFound, http.StatusNotFound},
		{fmt.Errorf("remove: %w", session.ErrRunning), http.StatusConflict},
		{session.ErrShuttingDown, http.StatusServiceUnavailable},
		{errors.New("driver close failed"), http.StatusInternalServerError},
	} {
		if got := sessionStatus(row.err); got != row.want {
			t.Errorf("sessionStatus(%v) = %d, want %d", row.err, got, row.want)
		}
	}
}

func TestPushStatus(t *testing.T) {
	if got := pushStatus(fmt.Errorf("register: %w", push.ErrFull)); got != http.StatusConflict {
		t.Fatalf("full store = %d", got)
	}
	if got := pushStatus(errors.New("disk failed")); got != http.StatusInternalServerError {
		t.Fatalf("save failure = %d", got)
	}
}
