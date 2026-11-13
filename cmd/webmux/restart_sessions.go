package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/rainchan/webmux/internal/workspace"
)

// restartResumeTimeout bounds resuming one session after a restart.
const restartResumeTimeout = time.Minute

// resumeRestarted resumes the hybrid sessions ids, one at a time, on their
// recorded side. One that cannot come back (its worktree was released, its
// launch config or agent session is gone) stays archived and is logged.
func resumeRestarted(ctx context.Context, w *workspace.Service, ids []string) {
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		rctx, cancel := context.WithTimeout(ctx, restartResumeTimeout)
		_, err := w.ResumeHybrid(rctx, id, workspace.HybridResumeInput{ClientRequestID: "restart-" + id})
		cancel()
		if err != nil {
			slog.Warn("session not resumed after restart; it stays archived", "id", id, "error", err)
			continue
		}
		slog.Info("session resumed after restart", "id", id)
	}
}
