package wire

import (
	"encoding/json"
	"testing"

	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/p2p"
)

func TestP2PUnknownAndZeroRTT(t *testing.T) {
	zero := 0
	status := P2PStatus(config.P2PConfig{}, "config", nil, p2p.Status{
		Supported: true,
		Peers:     []p2p.PeerInfo{{ID: "unknown"}, {ID: "zero", RTTMs: &zero}},
	})
	packet, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Peers []map[string]any `json:"peers"`
	}
	if err := json.Unmarshal(packet, &parsed); err != nil {
		t.Fatal(err)
	}
	if _, ok := parsed.Peers[0]["rttMs"]; ok {
		t.Fatal("unmeasured RTT was serialized")
	}
	if parsed.Peers[1]["rttMs"] != float64(0) {
		t.Fatal("measured zero RTT was omitted")
	}
	if status.UDPBindings == nil || status.Capabilities.ICEMode != "full" || status.Capabilities.TURN {
		t.Fatalf("capabilities: %+v", status)
	}
	unsupported := P2PStatus(config.P2PConfig{Enabled: true}, "config", nil, p2p.Status{})
	if unsupported.Enabled || unsupported.Capabilities.ICEMode != "unsupported" || unsupported.Capabilities.Networks == nil || len(unsupported.Capabilities.Networks) != 0 {
		t.Fatalf("nop2p: %+v", unsupported)
	}
}
