//go:build linux || darwin

package session

import (
	"fmt"
	"testing"
)

func TestProcessTreeWaiting(t *testing.T) {
	table := processTable{1: {ppid: 0, state: 'S'}, 2: {ppid: 1, state: 'S'}, 3: {ppid: 2, state: 'R'}}
	if newProcessSnapshot(table).foregroundBlocked(1) {
		t.Fatal("running descendant must prevent waiting")
	}
	table[3] = processState{ppid: 2, state: 'S'}
	if !newProcessSnapshot(table).foregroundBlocked(1) {
		t.Fatal("all sleeping leaves should wait")
	}
	delete(table, 1)
	if newProcessSnapshot(table).foregroundBlocked(1) {
		t.Fatal("missing root must not wait")
	}
}

func TestForegroundBlockedStates(t *testing.T) {
	tests := []struct {
		name  string
		table processTable
		want  bool
	}{
		{name: "missing root", table: processTable{2: {ppid: 1, state: 'S'}}},
		{name: "sleeping root", table: processTable{1: {state: 'S'}}, want: true},
		{name: "running root", table: processTable{1: {state: 'R'}, 2: {ppid: 1, state: 'S'}}},
		{name: "unknown root leaf", table: processTable{1: {state: '?'}}},
		{name: "unknown internal root", table: processTable{1: {state: '?'}, 2: {ppid: 1, state: 'S'}}, want: true},
		{name: "unknown internal descendant", table: processTable{1: {state: 'S'}, 2: {ppid: 1, state: '?'}, 3: {ppid: 2, state: 'S'}}, want: true},
		{name: "running internal descendant", table: processTable{1: {state: 'S'}, 2: {ppid: 1, state: 'R'}, 3: {ppid: 2, state: 'S'}}},
		{name: "all sleeping leaves", table: processTable{1: {state: 'S'}, 2: {ppid: 1, state: 'S'}, 3: {ppid: 1, state: 'S'}}, want: true},
		{name: "unknown leaf", table: processTable{1: {state: 'S'}, 2: {ppid: 1, state: 'S'}, 3: {ppid: 1, state: '?'}}},
		{name: "zombie leaf", table: processTable{1: {state: 'S'}, 2: {ppid: 1, state: 'Z'}}},
		{name: "unrelated running process", table: processTable{1: {state: 'S'}, 2: {ppid: 99, state: 'R'}}, want: true},
		{name: "self cycle", table: processTable{1: {ppid: 1, state: 'S'}}},
		{name: "root cycle without leaves", table: processTable{1: {ppid: 2, state: 'S'}, 2: {ppid: 1, state: 'S'}}},
		{name: "root cycle with sleeping leaf", table: processTable{1: {ppid: 2, state: 'S'}, 2: {ppid: 1, state: 'S'}, 3: {ppid: 2, state: 'S'}}, want: true},
		{name: "unrelated cycle", table: processTable{1: {state: 'S'}, 2: {ppid: 3, state: 'S'}, 3: {ppid: 2, state: 'S'}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newProcessSnapshot(tt.table).foregroundBlocked(1); got != tt.want {
				t.Fatalf("foregroundBlocked() = %v, want %v", got, tt.want)
			}
		})
	}
}

func BenchmarkForegroundBlocked(b *testing.B) {
	for _, shape := range []string{"chain", "wide", "unrelated"} {
		for _, size := range []int{128, 1024, 8192} {
			b.Run(fmt.Sprintf("%s/%d", shape, size), func(b *testing.B) {
				table := make(processTable, size)
				table[1] = processState{state: 'S'}
				for pid := 2; pid <= size; pid++ {
					parent := 1
					switch shape {
					case "chain":
						parent = pid - 1
					case "unrelated":
						parent = size + 1
					}
					table[pid] = processState{ppid: parent, state: 'S'}
				}
				snapshot := newProcessSnapshot(table)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if !snapshot.foregroundBlocked(1) {
						b.Fatal("sleeping process tree must be blocked")
					}
				}
			})
		}
	}
}

func BenchmarkProcessSnapshotManySessions(b *testing.B) {
	table := make(processTable, 5000)
	for pid := 1; pid <= 5000; pid++ {
		table[pid] = processState{ppid: 5000 + pid/3, state: 'S'}
	}
	b.ReportAllocs()
	for b.Loop() {
		snapshot := newProcessSnapshot(table)
		for pid := 1; pid <= 100; pid++ {
			if !snapshot.foregroundBlocked(pid) {
				b.Fatal("sleeping leaf must be blocked")
			}
		}
	}
}
