package acp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
)

// Content block types.
const (
	ContentText         = "text"
	ContentImage        = "image"
	ContentAudio        = "audio"
	ContentResourceLink = "resource_link"
	ContentResource     = "resource"
)

// ContentBlock is message or tool content. Text is set for "text"; URI,
// Name, Title, Description, MimeType and Size for "resource_link". Other
// types (images, audio, embedded resources, future types) are kept whole
// in Raw and written back unchanged.
type ContentBlock struct {
	Type        string
	Text        string
	URI         string
	Name        string
	Title       string
	Description string
	MimeType    string
	Size        *int64
	Raw         json.RawMessage
}

// TextBlock returns a text content block.
func TextBlock(text string) ContentBlock { return ContentBlock{Type: ContentText, Text: text} }

type imageBlockJSON struct {
	Type     string `json:"type"`
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
	URI      string `json:"uri,omitempty"`
}

type textResourceJSON struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text"`
}

type blobResourceJSON struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Blob     string `json:"blob"`
}

type resourceBlockJSON[R any] struct {
	Type     string `json:"type"`
	Resource R      `json:"resource"`
}

// ImageBlock returns an "image" block carrying data base64-encoded.
func ImageBlock(data []byte, mimeType, uri string) ContentBlock {
	raw, _ := json.Marshal(imageBlockJSON{
		Type: ContentImage, Data: base64.StdEncoding.EncodeToString(data), MimeType: mimeType, URI: uri,
	})
	return ContentBlock{Type: ContentImage, MimeType: mimeType, Raw: raw}
}

// ResourceTextBlock returns an embedded "resource" block with text content.
func ResourceTextBlock(uri, mimeType, text string) ContentBlock {
	raw, _ := json.Marshal(resourceBlockJSON[textResourceJSON]{
		Type: ContentResource, Resource: textResourceJSON{URI: uri, MimeType: mimeType, Text: text},
	})
	return ContentBlock{Type: ContentResource, MimeType: mimeType, Raw: raw}
}

// ResourceBlobBlock returns an embedded "resource" block with binary
// content, base64-encoded.
func ResourceBlobBlock(uri, mimeType string, data []byte) ContentBlock {
	raw, _ := json.Marshal(resourceBlockJSON[blobResourceJSON]{
		Type: ContentResource, Resource: blobResourceJSON{URI: uri, MimeType: mimeType, Blob: base64.StdEncoding.EncodeToString(data)},
	})
	return ContentBlock{Type: ContentResource, MimeType: mimeType, Raw: raw}
}

type textBlockJSON struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type resourceLinkJSON struct {
	Type        string `json:"type"`
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
	Size        *int64 `json:"size,omitempty"`
}

// MarshalJSON writes the fields of the block's type.
func (c ContentBlock) MarshalJSON() ([]byte, error) {
	switch c.Type {
	case ContentText:
		return json.Marshal(textBlockJSON{Type: c.Type, Text: c.Text})
	case ContentResourceLink:
		return json.Marshal(resourceLinkJSON{Type: c.Type, URI: c.URI, Name: c.Name, Title: c.Title, Description: c.Description, MimeType: c.MimeType, Size: c.Size})
	}
	if len(c.Raw) > 0 {
		return c.Raw, nil
	}
	return json.Marshal(struct {
		Type string `json:"type"`
	}{c.Type})
}

// UnmarshalJSON reads text and resource links into fields and keeps other
// types raw.
func (c *ContentBlock) UnmarshalJSON(b []byte) error {
	var in struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		URI         string `json:"uri"`
		Name        string `json:"name"`
		Title       string `json:"title"`
		Description string `json:"description"`
		MimeType    string `json:"mimeType"`
		Size        *int64 `json:"size"`
	}
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}
	switch in.Type {
	case ContentText:
		*c = ContentBlock{Type: in.Type, Text: in.Text}
	case ContentResourceLink:
		*c = ContentBlock{Type: in.Type, URI: in.URI, Name: in.Name, Title: in.Title, Description: in.Description, MimeType: in.MimeType, Size: in.Size}
	default:
		*c = ContentBlock{Type: in.Type, MimeType: in.MimeType, Raw: bytes.Clone(b)}
	}
	return nil
}
