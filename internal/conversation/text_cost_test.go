package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestEscapedTextCostMatchesJSON(t *testing.T) {
	for _, text := range []string{"", "中文🙂", "\"\\\n\t\b\f\r\x00<>&\u2028\u2029", "�", string([]byte{0xff, 0xe4, 0xb8})} {
		body, err := json.Marshal(text)
		if err != nil {
			t.Fatal(err)
		}
		if got := escapedTextBytes(text); got != int64(len(body)-2) {
			t.Fatalf("cost %d want %d", got, len(body)-2)
		}
	}
}

func TestObservedUsageFollowsNativeIdentityAndOptions(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	b := bindTest(t, s, "runtime")
	if err := s.SyncRuntime(ctx, RuntimeInput{RuntimeID: "runtime", AgentSessionID: "native-id", Config: map[string]string{"model": "one"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.ObserveUsage(ctx, b.ProviderThreadID, "other", 40, 100); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err := s.ObserveUsage(ctx, b.ProviderThreadID, "native-id", 4000, 32000); err != nil {
		t.Fatal(err)
	}
	if err := s.SyncRuntime(ctx, RuntimeInput{RuntimeID: "runtime", AgentSessionID: "native-id", Config: map[string]string{"model": "two"}}); err != nil {
		t.Fatal(err)
	}
	p, err := s.ProviderThread(ctx, b.ProviderThreadID)
	if err != nil || p.ContextUsage != 4000 || !p.UsageMeasured || p.ContextWindow != 0 || p.AutoCompactThreshold != 0 {
		t.Fatalf("invalidated occupancy: %+v %v", p, err)
	}
	if err := s.SyncRuntime(ctx, RuntimeInput{RuntimeID: "runtime", AgentSessionID: "fresh", Config: map[string]string{"model": "two"}}); err != nil {
		t.Fatal(err)
	}
	changed, err := s.Binding(ctx, "runtime")
	if err != nil || changed.ConversationID != b.ConversationID || changed.ProviderThreadID == b.ProviderThreadID {
		t.Fatalf("native change reused identity: %+v %v", changed, err)
	}
	p, err = s.ProviderThread(ctx, changed.ProviderThreadID)
	if err != nil || p.ContextUsage != 0 || p.UsageMeasured {
		t.Fatalf("fresh identity retained usage: %+v %v", p, err)
	}
}
