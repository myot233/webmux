package hybrid

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// trust accepts every path; which paths are uploads is the artifacts
// service's rule, tested there.
func trust(path string) (string, error) { return path, nil }

// upload writes data into a fresh directory standing in for uploads/.
func upload(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func pngBytes() []byte {
	return []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89")
}

func TestResolveAttachments(t *testing.T) {
	path := upload(t, "shot.png", pngBytes())
	list, err := resolveAttachments(trust, []Attachment{{Path: path}})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("attachments %v", list)
	}
	at := list[0]
	if at.Name != "shot.png" || at.Kind != AttachmentImage || at.MimeType != "image/png" || at.Size != int64(len(pngBytes())) {
		t.Fatalf("attachment %+v", at)
	}
	// Only the path is trusted: a client cannot claim another type.
	list, err = resolveAttachments(trust, []Attachment{{Path: path, Kind: AttachmentFile, MimeType: "text/plain", Name: "evil"}})
	if err != nil || list[0].Kind != AttachmentImage || list[0].Name != "shot.png" {
		t.Fatalf("client metadata used: %+v, %v", list, err)
	}
}

func TestResolveAttachmentsRejects(t *testing.T) {
	real := upload(t, "real.txt", []byte("hi"))
	refuse := func(string) (string, error) { return "", errors.New("not an upload") }
	if _, err := resolveAttachments(refuse, []Attachment{{Path: real}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("refused path: %v", err)
	}
	if _, _, err := promptBlocks("", []Attachment{{Path: real, Kind: AttachmentImage}}, PromptCapabilities{Image: true}, refuse); !errors.Is(err, ErrInvalid) {
		t.Fatalf("refused on reread: %v", err)
	}
	big := upload(t, "big.bin", make([]byte, MaxAttachmentBytes+1))
	if _, err := resolveAttachments(trust, []Attachment{{Path: big}}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized: %v", err)
	}
	total := make([]Attachment, 3)
	for i := range total {
		total[i] = Attachment{Path: sparseUpload(t, fmt.Sprintf("part%d.bin", i), MaxAttachmentBytes)}
	}
	if _, err := resolveAttachments(trust, total); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("total size: %v", err)
	}
}

// sparseUpload writes a file of size without allocating its bytes.
func sparseUpload(t *testing.T, name string, size int64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPromptBlocks(t *testing.T) {
	image := upload(t, "shot.png", pngBytes())
	text := upload(t, "note.txt", []byte("hello"))
	list, err := resolveAttachments(trust, []Attachment{{Path: image}, {Path: text}})
	if err != nil {
		t.Fatal(err)
	}

	blocks, notes, err := promptBlocks("look", list, PromptCapabilities{Image: true, EmbeddedContext: true}, trust)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 0 || len(blocks) != 3 {
		t.Fatalf("blocks %d notes %v", len(blocks), notes)
	}
	var raw map[string]any
	if err := json.Unmarshal(blocks[1].Raw, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["type"] != "image" || raw["data"] == "" {
		t.Fatalf("image block %v", raw)
	}

	blocks, notes, err = promptBlocks("look", list, PromptCapabilities{}, trust)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || blocks[0].Text == "" || !strings.Contains(blocks[0].Text, image) {
		t.Fatalf("degraded blocks %+v", blocks)
	}
	if len(notes) != 2 || !strings.Contains(notes[0], "路径") {
		t.Fatalf("degraded notes %v", notes)
	}

	blocks, notes, err = promptBlocks("", list, PromptCapabilities{EmbeddedContext: true}, trust)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 || len(notes) != 1 || !strings.Contains(notes[0], "按路径发送") {
		t.Fatalf("resource blocks %d notes %v", len(blocks), notes)
	}
	if blocks[0].Type != "text" || !strings.Contains(blocks[0].Text, image) || blocks[1].Type != "resource" {
		t.Fatalf("resource blocks %+v", blocks)
	}
}

func TestPromptBlocksCountLimit(t *testing.T) {
	list := make([]Attachment, MaxAttachments+1)
	for i := range list {
		list[i] = Attachment{Path: "/tmp/x", Kind: AttachmentFile}
	}
	if _, _, err := promptBlocks("", list, PromptCapabilities{}, trust); !errors.Is(err, ErrInvalid) {
		t.Fatalf("too many attachments: %v", err)
	}
}

func TestNonImageAttachmentsStayFiles(t *testing.T) {
	svg := upload(t, "icon.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`))
	list, err := resolveAttachments(trust, []Attachment{{Path: svg}})
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Kind != AttachmentFile {
		t.Fatalf("SVG kind %q", list[0].Kind)
	}
	empty := upload(t, "empty.txt", nil)
	list, err = resolveAttachments(trust, []Attachment{{Path: empty}})
	if err != nil {
		t.Fatal(err)
	}
	blocks, _, err := promptBlocks("", list, PromptCapabilities{EmbeddedContext: true}, trust)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || !strings.Contains(string(blocks[0].Raw), `"text":""`) {
		t.Fatalf("empty resource block %+v", blocks)
	}
}
