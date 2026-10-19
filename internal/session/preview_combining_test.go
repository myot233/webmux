package session

import (
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPreviewBoundsCombiningAllocation(t *testing.T) {
	var preview Preview
	preview.Feed([]byte("e"))
	input := []byte(strings.Repeat("\u0301", 10000))
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	preview.Feed(input)
	runtime.ReadMemStats(&after)
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("10000 combining marks allocated %d bytes", allocated)
	if allocated > 4<<20 {
		t.Fatalf("10000 combining marks allocated %d bytes", allocated)
	}
	assertPreviewCellBound(t, &preview)
	if got := preview.Feed([]byte("\r\ncontinued")); !strings.HasSuffix(got, "\ncontinued") {
		t.Fatalf("preview did not continue after combining marks: %q", got)
	}
}

func TestPreviewCombiningBudgetAcrossWritesAndRepeat(t *testing.T) {
	for _, base := range []string{"e", "界"} {
		t.Run(base, func(t *testing.T) {
			var preview Preview
			preview.Feed([]byte(base))
			// Each UTF-8 combining mark is split between separate Feed calls.
			for i := 0; i < 200; i++ {
				preview.Feed([]byte{0xcc})
				preview.Feed([]byte{0x81})
			}
			want := base + strings.Repeat("\u0301", 63)
			if preview.text != want {
				t.Fatalf("bounded cluster has %d runes, want 64", utf8.RuneCountInString(preview.text))
			}
			preview.Feed([]byte("\x1b[2b"))
			assertPreviewCellBound(t, &preview)
			buffer := preview.screen.Buffer()
			line := buffer.Lines.Get(buffer.YBase)
			for x := 0; x < buffer.X; x += max(1, line.GetWidth(x)) {
				if got := line.GetString(x); got != want {
					t.Fatalf("REP cell %d = %q, want bounded cluster", x, got)
				}
			}
			preview.Feed([]byte("\x1b[1;1Hb" + strings.Repeat("\u0301", 200)))
			assertPreviewCellBound(t, &preview)
			if got := line.GetString(0); got != "b"+strings.Repeat("\u0301", 63) {
				t.Fatalf("rewritten cluster has %d runes, want 64", utf8.RuneCountInString(got))
			}
		})
	}
}

func TestPreviewPreservesOrdinaryCombiningText(t *testing.T) {
	input := "Cafe\u0301 界\u0301 中文\r\nnext"
	want := "Cafe\u0301 界\u0301 中文\nnext"
	for split := 0; split <= len(input); split++ {
		var preview Preview
		preview.Feed([]byte(input[:split]))
		if got := preview.Feed([]byte(input[split:])); got != want {
			t.Fatalf("split %d: preview = %q, want %q", split, got, want)
		}
	}
}

func assertPreviewCellBound(t *testing.T, preview *Preview) {
	t.Helper()
	buffer := preview.screen.Buffer()
	for y := 0; y < preview.screen.Rows(); y++ {
		line := buffer.Lines.Get(buffer.YBase + y)
		for x := 0; x < line.Length; x++ {
			if n := utf8.RuneCountInString(line.GetString(x)); n > 64 {
				t.Fatalf("cell (%d, %d) retains %d runes, want at most 64", x, y, n)
			}
		}
	}
}
