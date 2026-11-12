package hybrid

import (
	"fmt"
	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/conversation"
	"strconv"
	"strings"
)

func activityBlocks(blocks []acp.ContentBlock) string {
	var text strings.Builder
	for _, b := range blocks {
		if b.Type == acp.ContentText {
			text.WriteString(b.Text)
		}
	}
	return text.String()
}

func activityToolText(title string, contents []acp.ToolCallContent) string {
	var text strings.Builder
	if title != "" {
		text.WriteString(title)
		text.WriteByte('\n')
	}
	for _, c := range contents {
		switch c.Type {
		case acp.ToolContentContent:
			if c.Content != nil && c.Content.Type == acp.ContentText {
				text.WriteString(c.Content.Text)
			}
		case acp.ToolContentDiff:
			fmt.Fprintf(&text, "\nFile: %s\n", c.Path)
			if c.OldText != nil {
				text.WriteString("Before:\n")
				text.WriteString(*c.OldText)
			}
			text.WriteString("\nAfter:\n")
			text.WriteString(c.NewText)
		}
	}
	return text.String()
}

func recordedToolHeading(t recordedTool) string {
	heading := t.label
	if t.input != "" {
		heading += "\nCommand input: " + t.input
	}
	if t.exitCode != nil {
		heading += "\nExit code: " + strconv.Itoa(*t.exitCode)
	}
	if heading != "" {
		heading += "\n"
	}
	return heading
}

func activityAttachments(attachments []Attachment) []conversation.ActivityAttachment {
	metadata := make([]conversation.ActivityAttachment, 0, len(attachments))
	for _, v := range attachments {
		metadata = append(metadata, conversation.ActivityAttachment{Name: v.Name, MimeType: v.MimeType, Reference: v.Path, Size: v.Size})
	}
	return metadata
}
