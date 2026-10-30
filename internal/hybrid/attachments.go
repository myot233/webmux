package hybrid

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/pathutil"
)

const (
	// MaxAttachments bounds one prompt's attachment count.
	MaxAttachments = 8
	// MaxAttachmentBytes bounds one attachment sent to the agent.
	MaxAttachmentBytes int64 = 10 << 20
	// MaxPromptAttachmentBytes bounds all raw attachment bytes of one
	// prompt, before base64 expansion.
	MaxPromptAttachmentBytes int64 = 20 << 20
	// MaxPromptWireBytes bounds the encoded prompt: base64 expands images
	// and resources by about a third, so this stays below ACP's protocol
	// line limit with room for JSON framing.
	MaxPromptWireBytes = acp.MaxLineBytes - (2 << 20)
)

// imageMimes are the image types sent as ACP image blocks. SVG is
// deliberately absent: agents must not execute uploaded markup.
var imageMimes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

// resolveAttachments validates uploaded files with check and fills their
// metadata. Only the request's Path is used; Name, MimeType, Size and Kind
// come from the stored file.
func resolveAttachments(check func(string) (string, error), input []Attachment) ([]Attachment, error) {
	if len(input) > MaxAttachments {
		return nil, fmt.Errorf("%w: 每条消息最多 %d 个附件", ErrInvalid, MaxAttachments)
	}
	if len(input) == 0 {
		return nil, nil
	}
	out := make([]Attachment, 0, len(input))
	var total int64
	for _, item := range input {
		path, err := check(item.Path)
		if err != nil {
			return nil, fmt.Errorf("%w: 附件路径无效: %v", ErrInvalid, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("%w: 读取附件失败", ErrInvalid)
		}
		if info.Size() > MaxAttachmentBytes {
			return nil, fmt.Errorf("%w: 附件 %s 超过 %d MiB", ErrTooLarge, filepath.Base(path), MaxAttachmentBytes>>20)
		}
		mimeType, err := sniffMime(path)
		if err != nil {
			return nil, fmt.Errorf("%w: 读取附件失败", ErrInvalid)
		}
		total += info.Size()
		if total > MaxPromptAttachmentBytes {
			return nil, fmt.Errorf("%w: 附件合计超过 %d MiB", ErrTooLarge, MaxPromptAttachmentBytes>>20)
		}
		kind := AttachmentFile
		if imageMimes[mimeType] {
			kind = AttachmentImage
		}
		out = append(out, Attachment{
			Path: path, Name: filepath.Base(path), MimeType: mimeType, Size: info.Size(), Kind: kind,
		})
	}
	return out, nil
}

// sniffMime detects a media type from the file's first bytes.
func sniffMime(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	return pathutil.SniffContentType(head[:n]), nil
}

// readAttachment reads one attachment again, keeping the size bound in case
// the file changed since it was validated.
func readAttachment(check func(string) (string, error), at Attachment) ([]byte, error) {
	path, err := check(at.Path)
	if err != nil {
		return nil, fmt.Errorf("%w: 附件路径无效: %v", ErrInvalid, err)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: 读取附件失败", ErrInvalid)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, MaxAttachmentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: 读取附件失败", ErrInvalid)
	}
	if int64(len(data)) > MaxAttachmentBytes {
		return nil, fmt.Errorf("%w: 附件 %s 超过 %d MiB", ErrTooLarge, at.Name, MaxAttachmentBytes>>20)
	}
	return data, nil
}

// promptBlocks builds the ACP prompt for text and attachments under caps,
// rechecking each attachment with check before reading it. Unsupported
// content degrades to the file's server path in the text, and notes
// describe every degradation for the timeline.
func promptBlocks(text string, attachments []Attachment, caps PromptCapabilities, check func(string) (string, error)) ([]acp.ContentBlock, []string, error) {
	if len(attachments) > MaxAttachments {
		return nil, nil, fmt.Errorf("%w: 每条消息最多 %d 个附件", ErrInvalid, MaxAttachments)
	}
	var blocks []acp.ContentBlock
	var notes []string
	var paths []string
	var rawBytes int64
	wireBytes := int64(len(text))
	add := func(block acp.ContentBlock, raw int64) error {
		rawBytes += raw
		if rawBytes > MaxPromptAttachmentBytes {
			return fmt.Errorf("%w: 附件合计超过 %d MiB", ErrTooLarge, MaxPromptAttachmentBytes>>20)
		}
		wireBytes += int64(len(block.Raw))
		if wireBytes > MaxPromptWireBytes {
			return fmt.Errorf("%w: 附件编码后超过 %d MiB", ErrTooLarge, MaxPromptWireBytes>>20)
		}
		blocks = append(blocks, block)
		return nil
	}
	for _, at := range attachments {
		if at.Kind == AttachmentImage && caps.Image {
			data, err := readAttachment(check, at)
			if err != nil {
				return nil, nil, err
			}
			if err := add(acp.ImageBlock(data, at.MimeType, fileURI(at.Path)), int64(len(data))); err != nil {
				return nil, nil, err
			}
			continue
		}
		if at.Kind != AttachmentImage && caps.EmbeddedContext {
			data, err := readAttachment(check, at)
			if err != nil {
				return nil, nil, err
			}
			block := acp.ResourceBlobBlock(fileURI(at.Path), at.MimeType, data)
			if utf8.Valid(data) && !bytes.ContainsRune(data, 0) {
				block = acp.ResourceTextBlock(fileURI(at.Path), at.MimeType, string(data))
			}
			if err := add(block, int64(len(data))); err != nil {
				return nil, nil, err
			}
			continue
		}
		paths = append(paths, at.Path)
		if at.Kind == AttachmentImage {
			notes = append(notes, "该 agent 不支持图片，已按路径发送："+at.Name)
		} else {
			notes = append(notes, "该 agent 不支持嵌入资源，已按路径发送："+at.Name)
		}
	}
	if len(paths) > 0 {
		if text != "" {
			text += "\n\n"
		}
		text += "附件路径：\n" + strings.Join(paths, "\n")
	}
	if text != "" || len(blocks) == 0 {
		blocks = append([]acp.ContentBlock{acp.TextBlock(text)}, blocks...)
	}
	wireBytes = int64(len(text))
	for _, block := range blocks {
		wireBytes += int64(len(block.Raw))
	}
	if wireBytes > MaxPromptWireBytes {
		return nil, nil, fmt.Errorf("%w: 提示编码后超过 %d MiB", ErrTooLarge, MaxPromptWireBytes>>20)
	}
	return blocks, notes, nil
}

// fileURI is the file:// URI of an absolute path.
func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}
