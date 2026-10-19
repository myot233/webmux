package main

import "testing"

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
