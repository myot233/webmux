package handoff

import (
	"context"

	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/hybrid"
)

// attachmentSnapshot reuses the prompt validator, including its byte, count,
// aggregate and media-type rules. Client metadata never authorizes a file read.
func (s *Service) attachmentSnapshot(ctx context.Context, source string, input []conversation.PreviewAttachment) ([]conversation.PreviewAttachment, error) {
	out := make([]conversation.PreviewAttachment, 0, len(input))
	if len(input) == 0 {
		return out, nil
	}
	if s.options.Artifacts == nil || s.options.Hybrid == nil {
		return nil, conversation.ErrUnavailable
	}
	runtimeIDs, err := s.options.Conversations.RuntimeIDs(ctx, source)
	if err != nil {
		return nil, err
	}
	check := func(path string) (string, error) {
		clean, err := s.options.Artifacts.StagedPath(path)
		if err == nil {
			return clean, nil
		}
		for _, id := range runtimeIDs {
			if clean, err = s.options.Artifacts.UploadPath(id, path); err == nil {
				return clean, nil
			}
		}
		return "", conversation.ErrInvalid
	}
	attachments := make([]hybrid.Attachment, len(input))
	seen := map[string]bool{}
	for i, a := range input {
		if seen[a.Reference] {
			return nil, conversation.ErrInvalid
		}
		seen[a.Reference] = true
		attachments[i] = hybrid.Attachment{Path: a.Reference}
	}
	resolved, err := s.options.Hybrid.ResolveAttachments(check, attachments)
	if err != nil {
		return nil, conversation.ErrInvalid
	}
	for _, a := range resolved {
		// Only staging is ephemeral. Source uploads already belong to a
		// retained runtime and must not be recopied on review or delivery.
		if _, err := s.options.Artifacts.StagedPath(a.Path); err == nil {
			var owner string
			for _, id := range runtimeIDs {
				if _, err := s.options.Hybrid.Record(id); err == nil {
					owner = id
					break
				}
			}
			if owner == "" {
				return nil, conversation.ErrUnavailable
			}
			a.Path, err = s.options.Artifacts.CopyStagedUpload(owner, a.Path, a.Size)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, conversation.PreviewAttachment{Reference: a.Path, Name: a.Name, MimeType: a.MimeType, Size: a.Size, Kind: a.Kind})
	}
	return out, nil
}
