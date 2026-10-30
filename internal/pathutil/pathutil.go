// Package pathutil holds path helpers shared by the server, its
// configuration and the local clients.
package pathutil

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// SniffContentType names a media type from a file's first bytes, without
// parameters.
func SniffContentType(head []byte) string {
	mediaType, _, err := mime.ParseMediaType(http.DetectContentType(head))
	if err != nil {
		return "application/octet-stream"
	}
	return mediaType
}

// Expand expands a leading "~/" and makes the path absolute.
func Expand(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	return filepath.Abs(path)
}
