package handoff

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/hybrid"
)

func (s *Service) ownAttachments(ctx context.Context, p conversation.Preview, e conversation.Execution) ([]hybrid.Attachment, error) {
	if len(p.Request.Attachments) == 0 {
		return nil, nil
	}
	if s.options.Artifacts == nil {
		return nil, conversation.ErrUnavailable
	}
	saved := e.Attachments
	if len(saved) == 0 {
		directory, err := s.options.Artifacts.EnsureUploads(e.RuntimeID)
		if err != nil {
			return nil, err
		}
		for i, a := range p.Request.Attachments {
			file, err := os.OpenInRoot(filepath.Dir(a.Reference), filepath.Base(a.Reference))
			if err != nil {
				return nil, conversation.ErrInvalid
			}
			hash := sha256.New()
			n, copyErr := io.Copy(hash, io.LimitReader(file, hybrid.MaxAttachmentBytes+1))
			closeErr := file.Close()
			if copyErr != nil || closeErr != nil || n != a.Size {
				return nil, conversation.ErrInvalid
			}
			name := "handoff-" + e.DeliveryID + "-" + strconv.Itoa(i)
			saved = append(saved, conversation.DeliveryAttachment{SourceReference: a.Reference, Reference: filepath.Join(directory, name), Name: a.Name, MimeType: a.MimeType, Kind: a.Kind, Digest: hex.EncodeToString(hash.Sum(nil)), Size: a.Size})
		}
		if err := s.options.Conversations.SaveExecutionAttachments(ctx, e.ID, saved); err != nil {
			return nil, err
		}
	}
	if len(saved) != len(p.Request.Attachments) {
		return nil, conversation.ErrConflict
	}
	out := make([]hybrid.Attachment, 0, len(saved))
	for _, a := range saved {
		path, err := s.options.Artifacts.CopyUpload(e.RuntimeID, a.SourceReference, filepath.Base(a.Reference), a.Digest, a.Size)
		if err != nil {
			return nil, err
		}
		out = append(out, hybrid.Attachment{Path: path, Name: a.Name, MimeType: a.MimeType, Kind: a.Kind, Size: a.Size})
	}
	return out, nil
}
