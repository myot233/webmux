package localclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// GetBounded reads a JSON response with a caller-selected hard byte limit.
// MCP history readers use this through the same authenticated local transport.
func (c *Client) GetBounded(ctx context.Context, path string, limit int64, out any) error {
	return c.DoBounded(ctx, http.MethodGet, path, nil, limit, out)
}

// DoBounded sends a typed JSON operation and bounds its response before decoding.
func (c *Client) DoBounded(ctx context.Context, method, path string, body any, limit int64, out any) error {
	if limit < 1 {
		return errors.New("invalid local API response limit")
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := c.request(ctx, method, path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
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
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}
