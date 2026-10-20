package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestFailedMeasurementRemainsInDenominator(t *testing.T) {
	m := measure("injected_failure", "ms", 2, func(i int) float64 {
		if i == 0 {
			panic("injected transport failure")
		}
		return 12
	})
	if !m.CorrectnessGate || m.Summary.Attempts != 2 || m.Summary.Passed != 1 || m.Summary.SuccessRate != .5 || m.Samples[0].Error != "injected transport failure" || m.Summary.P95 != 12 {
		t.Fatalf("failed sample was lost or included as latency: %+v", m)
	}
}

func TestCleanupTimeoutFailsDeferredSampleAndAborts(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	calls := 0
	defer func() {
		r := recover()
		abort, ok := r.(measurementAbort)
		if !ok {
			t.Fatalf("cleanup timeout did not abort: %v", r)
		}
		m := abort.Metric
		if calls != 1 || m.Summary.Attempts != 1 || m.Summary.Passed != 0 || m.Samples[0].OK || !strings.Contains(m.Samples[0].Error, "cleanup failed") {
			t.Fatalf("cleanup failure lost or later sample ran: calls=%d metric=%+v", calls, m)
		}
	}()
	measure("cleanup_timeout", "ms", 3, func(i int) float64 {
		calls++
		defer func() {
			if err := waitForCleanup(ctx, make(chan struct{})); err != nil {
				panic(cleanupFailure{ID: "fixture-session", Cause: err})
			}
		}()
		return 12
	})
	t.Fatal("cleanup failure was swallowed")
}

func TestCompletedCleanupSucceeds(t *testing.T) {
	done := make(chan struct{})
	close(done)
	if err := waitForCleanup(context.Background(), done); err != nil {
		t.Fatal(err)
	}
}
