package session

import (
	"runtime"
	"strings"
	"testing"
)

func TestPreviewBoundsRepeatWork(t *testing.T) {
	var preview Preview
	preview.Feed([]byte("x"))
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	preview.Feed([]byte("\x1b[1000000b"))
	runtime.ReadMemStats(&after)
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 1<<20 {
		t.Fatalf("short REP allocated %d bytes", allocated)
	}
	preview.Feed([]byte("\r\ncontinued"))
	if !strings.Contains(preview.text, "continued") {
		t.Fatal("preview stopped processing after REP")
	}
}

func TestPreviewPreservesOrdinarySplitRepeat(t *testing.T) {
	var preview Preview
	for _, part := range []string{"x\x1b[", "3", "b"} {
		preview.Feed([]byte(part))
	}
	if preview.text != "xxxx" {
		t.Fatalf("ordinary REP = %q", preview.text)
	}
}
