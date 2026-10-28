//go:build linux

package session

import (
	"testing"
	"time"
)

// The subreaper reaps every zombie child that is not a managed leader. A
// sidecar is registered through startChild, so its exit status stays with
// its own Wait even while reaping runs continuously.
func TestSubreaperLeavesSidecarExitStatus(t *testing.T) {
	t.Parallel()
	m := shellManager(t)
	s := createShell(t, m, nil)
	for range 20 {
		sidecar, err := s.StartSidecar([]string{"/bin/sh", "-c", "exit 17"}, false)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.After(10 * time.Second)
	reap:
		for {
			select {
			case <-sidecar.Done():
				break reap
			case <-deadline:
				t.Fatal("sidecar did not finish")
			default:
				reapAdopted(processes())
			}
		}
		if code := sidecar.ExitCode(); code != 17 {
			t.Fatalf("sidecar exit status stolen: %d", code)
		}
	}
}
