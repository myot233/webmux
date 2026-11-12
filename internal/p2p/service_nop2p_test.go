//go:build nop2p

package p2p

import (
	"context"
	"errors"
	"testing"
)

func TestNoP2PStubRejectsSignaling(t *testing.T) {
	svc := New(Options{Enabled: true, PortMapping: true})
	defer svc.Close()
	if svc.Supported() || svc.Enabled() {
		t.Fatalf("expected nop2p stub to report Supported=false Enabled=false")
	}
	st := svc.Status()
	if st.Supported || st.ActivePeers != 0 || st.PortMap.State != "off" {
		t.Fatalf("unexpected nop2p status: %+v", st)
	}
	if _, err := svc.Signal(context.Background(), SignalInput{Action: "offer", SDP: "v=0"}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
}
