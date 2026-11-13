package session

import (
	"bytes"
	"errors"
	"testing"
)

func TestReadOutputRetainsTruncatedRawSuffix(t *testing.T) {
	ring := mustRing(t, 8)
	ring.Write([]byte("abcdefghijklmnop"))
	s := &Session{info: Info{Generation: "0123456789abcdef"}, run: &run{ring: ring}}
	first, err := s.ReadOutput(nil, 3)
	if err != nil {
		t.Fatal(err)
	}
	if string(first.Data) != "ijk" || first.Start != 8 || first.End != 16 || first.NextOffset != 11 || !first.HistoryTruncated || !first.Limited {
		t.Fatalf("first page %+v", first)
	}
	next, err := s.ReadOutput(&ResumePoint{Generation: first.Generation, Offset: first.NextOffset}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if string(next.Data) != "lmn" || next.Reset {
		t.Fatalf("next page %+v", next)
	}
	for _, resume := range []*ResumePoint{{Generation: first.Generation, Offset: 0}, {Generation: "fedcba9876543210", Offset: 11}, {Generation: first.Generation, Offset: 17}} {
		page, err := s.ReadOutput(resume, 1)
		if err != nil {
			t.Fatal(err)
		}
		if !page.Reset || string(page.Data) != "i" {
			t.Fatalf("reset page %+v", page)
		}
	}
	if s.control != nil || len(s.subs) != 0 {
		t.Fatal("raw read attached a client or took control")
	}
	first.Data[0] = 'x'
	page, err := s.ReadOutput(nil, 3)
	if err != nil {
		t.Fatal(err)
	}
	if string(page.Data) != "ijk" {
		t.Fatal("page aliases ring storage")
	}
}
func TestReadOutputBoundsPageRatherThanHistory(t *testing.T) {
	ring := mustRing(t, 8<<20)
	ring.Write(bytes.Repeat([]byte("x"), 5<<20))
	s := &Session{info: Info{Generation: "0123456789abcdef"}, run: &run{ring: ring}}
	page, err := s.ReadOutput(nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || cap(page.Data) != 1 || page.End != 5<<20 || !page.Limited {
		t.Fatalf("one-byte page allocated for full history: len=%d cap=%d end=%d", len(page.Data), cap(page.Data), page.End)
	}
	for _, n := range []int{0, 65537} {
		if _, err := s.ReadOutput(nil, n); !errors.Is(err, ErrInvalid) {
			t.Fatalf("limit %d: %v", n, err)
		}
	}
	if _, err := (&Session{}).ReadOutput(nil, 1); !errors.Is(err, ErrNoRun) {
		t.Fatalf("no run: %v", err)
	}
}
