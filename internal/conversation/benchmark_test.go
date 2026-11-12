package conversation

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func benchmarkStore(b *testing.B) (*Service, Binding) {
	b.Helper()
	s := Open(Options{StateDir: b.TempDir()})
	b.Cleanup(func() { _ = s.Close() })
	binding, err := s.EnsureRuntime(context.Background(), RuntimeInput{RuntimeID: "benchmark", Cwd: "/workspace", AgentName: "test", HistoryOrigin: "complete"})
	if err != nil {
		b.Fatal(err)
	}
	return s, binding
}

func BenchmarkActivityWriteTenThousand(b *testing.B) {
	s, binding := benchmarkStore(b)
	ctx := context.Background()
	text := strings.Repeat("history ", 250)
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	b.ResetTimer()
	for n := range b.N {
		if _, err := s.WriteActivity(ctx, ActivityInput{ProviderThreadID: binding.ProviderThreadID, LocalKey: fmt.Sprint(n), Kind: "message", Role: "user", Source: "local", Status: "completed", Text: text}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHistoryPageTenThousand(b *testing.B) {
	s, binding := benchmarkStore(b)
	ctx := context.Background()
	text := strings.Repeat("history ", 250)
	for n := range 10000 {
		if _, err := s.WriteActivity(ctx, ActivityInput{ProviderThreadID: binding.ProviderThreadID, LocalKey: fmt.Sprint(n), Kind: "message", Role: "user", Source: "local", Status: "completed", Text: text}); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		page, err := s.Activities(ctx, binding.ConversationID, 5000, 20, 4000)
		if err != nil || len(page.Items) != 20 || page.NextPosition != 5020 {
			b.Fatal("bounded history page", err)
		}
	}
}
