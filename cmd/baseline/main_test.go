package main

import (
	"math"
	"testing"
)

func TestSummariesIncludeFailuresAndUseNearestRank(t *testing.T) {
	s := summarize([]sample{{Value: 4, OK: true}, {Value: 1, OK: true}, {Error: "timeout"}, {Value: 2, OK: true}, {Value: 3, OK: true}})
	if s.Attempts != 5 || s.Passed != 4 || s.SuccessRate != .8 || s.Mean != 2.5 || s.P50 != 2 || s.P95 != 4 || s.Max != 4 {
		t.Fatalf("summary: %+v", s)
	}
	if math.Abs(s.Wilson95[0]-.3755) > .0001 || math.Abs(s.Wilson95[1]-.9638) > .0001 {
		t.Fatalf("Wilson interval: %v", s.Wilson95)
	}
}
func TestAllPassDoesNotClaimCertainty(t *testing.T) {
	s := summarize([]sample{{OK: true}, {OK: true}})
	if s.Wilson95[0] < .3423 || s.Wilson95[0] > .3424 || s.Wilson95[1] != 1 {
		t.Fatalf("interval: %v", s.Wilson95)
	}
	if summarize(nil).Attempts != 0 {
		t.Fatal("empty summary")
	}
}
