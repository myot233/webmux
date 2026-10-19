package session

import "testing"

func TestPreviewDoesNotRetainHyperlinkHistory(t *testing.T) {
	var preview Preview
	for i := 0; i < 1000; i++ {
		preview.Feed([]byte("\x1b]8;;https://example.invalid/item\x1b"))
		preview.Feed([]byte("\\link\x1b]8;;\x1b\\\r\n"))
	}
	if _, ok := preview.screen.OscLinkService().GetLinkData(1); ok {
		t.Fatal("plain-text observer retained OSC 8 link metadata")
	}
	if preview.last != "link" {
		t.Fatalf("visible link label lost: %q", preview.last)
	}
}
