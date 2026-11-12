package localclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// GetBounded reads a JSON response with a caller-selected hard byte limit.
// MCP history readers use this through the same authenticated local transport.
func (c *Client) GetBounded(ctx context.Context, path string, limit int64, out any) error {
	if limit < 1 {
		return errors.New("invalid local API response limit")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, BaseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return unwrapURLError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return apiError(resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return errors.New("local API response exceeds limit")
	}
	return json.Unmarshal(data, out)
}
