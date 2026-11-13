package mcphttp

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestInjectedScannerHardExpiryCancelsAndJoins(t *testing.T) {
	entered := make(chan context.Context, 1)
	cleaned := make(chan struct{})
	base, clock := testHandler(t, func(ctx context.Context, _ json.RawMessage) (any, error) {
		entered <- ctx
		<-ctx.Done()
		close(cleaned)
		return nil, ctx.Err()
	})
	ticks := make(chan time.Time)
	stopped := make(chan struct{})
	h := newHandler(context.Background(), base.registry, clock.now, ticks, func() { close(stopped) })
	t.Cleanup(h.Close)
	session := initialize(t, h, "2025-11-25", true)
	done := startCall(h, session, `1`, context.Background())
	receive(t, entered)
	clock.advance(hardTimeout)
	ticks <- clock.now()
	requireError(t, receive(t, done), http.StatusOK, -32000)
	receive(t, cleaned)
	requireStatus(t, post(h, session, ping), http.StatusNotFound)
	h.Close()
	receive(t, stopped)
}

func TestParentCancellationJoinsAcceptedHTTPHandlers(t *testing.T) {
	entered := make(chan context.Context, 1)
	base, clock := testHandler(t, func(ctx context.Context, _ json.RawMessage) (any, error) {
		entered <- ctx
		<-ctx.Done()
		return nil, ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := newHandler(ctx, base.registry, clock.now, make(chan time.Time), func() {})
	t.Cleanup(h.Close)
	session := initialize(t, h, "2025-11-25", true)
	done := startCall(h, session, `1`, context.Background())
	receive(t, entered)
	cancel()
	receive(t, h.done)
	requireError(t, receive(t, done), http.StatusOK, -32000)
	h.Close()
}

func TestCancellationAndFinishRaceHasOneTerminalResponse(t *testing.T) {
	releases := make(chan chan struct{}, 1)
	entered := make(chan struct{}, 1)
	h, _ := testHandler(t, func(ctx context.Context, _ json.RawMessage) (any, error) {
		release := <-releases
		entered <- struct{}{}
		select {
		case <-release:
			return struct {
				Finished bool `json:"finished"`
			}{true}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	session := initialize(t, h, "2025-11-25", true)
	for range 32 {
		release := make(chan struct{})
		releases <- release
		done := startCall(h, session, `1`, context.Background())
		receive(t, entered)
		cancelled := make(chan struct{})
		go func() {
			cancelCall(h, session, `1`)
			close(cancelled)
		}()
		close(release)
		w := receive(t, done)
		receive(t, cancelled)
		requireStatus(t, w, http.StatusOK)
		var response struct {
			Error *struct {
				Code int `json:"code"`
			} `json:"error"`
			Result *struct {
				IsError bool `json:"isError"`
			} `json:"result"`
		}
		if json.Unmarshal(w.Body.Bytes(), &response) != nil {
			t.Fatal("race produced invalid response")
		}
		if response.Error != nil {
			if response.Error.Code != -32000 || response.Result != nil {
				t.Fatal("cancellation winner did not have one terminal response")
			}
		} else if response.Result == nil || response.Result.IsError {
			t.Fatal("completion winner was overwritten by a cancellation/tool error")
		}
	}
}
