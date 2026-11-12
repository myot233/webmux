// Package localclient speaks the webmux HTTP/WebSocket API over any byte
// stream: the Unix socket on Linux/macOS, or `webmux bridge` stdio from
// Windows. It depends only on the standard library and coder/websocket so it
// cross-compiles to every client platform.
package localclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"syscall"

	"github.com/coder/websocket"
	"github.com/rainchan/webmux/internal/contract"
)

const (
	// BaseURL is the authority used for every request; the dialer ignores it.
	BaseURL = "http://webmux"
	// WSProtocol is the WebSocket subprotocol the server selects.
	WSProtocol = "webmux"

	// Bound diagnostic bodies without buffering an unbounded server response.
	maxErrorBodyBytes = 64 << 10
)

// ErrNotRunning means no server accepts connections on the local API.
var ErrNotRunning = errors.New("webmux is not running")

// Dialer opens one connection to the local API.
type Dialer interface {
	DialContext(ctx context.Context) (net.Conn, error)
}

// UnixDialer connects to the server's socket. A missing socket or a refused
// connection is reported as ErrNotRunning.
type UnixDialer struct{ Path string }

func (d UnixDialer) DialContext(ctx context.Context) (net.Conn, error) {
	if d.Path == "" {
		return nil, fmt.Errorf("%w: local API unavailable", ErrNotRunning)
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", d.Path)
	if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) {
		return nil, fmt.Errorf("%w: %v", ErrNotRunning, err)
	}
	return conn, err
}

// Client reuses connections through HTTP/1.1 keep-alive.
type Client struct {
	HTTP *http.Client
}

func New(d Dialer) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return d.DialContext(ctx)
		},
		Proxy:               nil,
		DisableCompression:  true,
		MaxIdleConnsPerHost: 2,
	}
	return &Client{HTTP: &http.Client{Transport: transport}}
}

// Close drops idle connections; for a bridge dialer that ends its process.
func (c *Client) Close() { c.HTTP.CloseIdleConnections() }

// APIError is a non-success response with the server's error message.
type APIError struct {
	Status  int
	Message string
	// Body is the raw response body (up to 64 KiB), for endpoints whose
	// failures carry more than a message.
	Body []byte
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("webmux returned HTTP %d", e.Status)
	}
	return e.Message
}

// Do sends a JSON request and decodes a JSON response into out unless nil.
// Non-2xx responses become *APIError.
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, BaseURL+path, reader)
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
	return decodeResponse(resp, out)
}

// Raw sends body (JSON when not nil) and returns the status and raw body of
// any response; only transport failures are errors.
func (c *Client) Raw(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, BaseURL+path, reader)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, unwrapURLError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	return resp.StatusCode, data, err
}

// Get sends a GET and returns the raw body of a 2xx response; other
// statuses become *APIError.
func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, unwrapURLError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, apiError(resp)
	}
	return io.ReadAll(resp.Body)
}

// Upload posts one file as the multipart field "file", as the browser's
// upload endpoints expect, and decodes the JSON response into out.
func (c *Client) Upload(ctx context.Context, path, filename string, content io.Reader, out any) error {
	body, writer := io.Pipe()
	form := multipart.NewWriter(writer)
	go func() {
		part, err := form.CreateFormFile("file", filename)
		if err == nil {
			_, err = io.Copy(part, content)
		}
		if err == nil {
			err = form.Close()
		}
		_ = writer.CloseWithError(err)
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, BaseURL+path, body)
	if err != nil {
		_ = body.Close()
		return err
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return unwrapURLError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	return decodeResponse(resp, out)
}

// decodeResponse handles JSON requests and uploads alike. Even when the caller
// ignores the response, consuming it must succeed before reporting success.
func decodeResponse(resp *http.Response, out any) error {
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return apiError(resp)
	}
	if out == nil {
		_, err := io.Copy(io.Discard, resp.Body)
		return err
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// apiError reads a failed response into *APIError.
func apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	var failure contract.ErrorResponse
	_ = json.Unmarshal(body, &failure)
	return &APIError{Status: resp.StatusCode, Message: failure.Error, Body: body}
}

// unwrapURLError drops the fake "http://webmux" URL from messages while
// keeping ErrNotRunning reachable through errors.Is.
func unwrapURLError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

func (c *Client) Status(ctx context.Context) (contract.StatusV1, error) {
	var status contract.StatusV1
	err := c.Do(ctx, http.MethodGet, contract.PathStatus, nil, &status)
	return status, err
}

// Shutdown returns *APIError with status 409 for service-managed instances
// and 403 for callers inside a WebMux session.
func (c *Client) Shutdown(ctx context.Context, force bool) error {
	return c.Do(ctx, http.MethodPost, contract.PathShutdown, contract.ShutdownRequest{Force: force}, nil)
}

// Token returns "" without error when the server has no token.
func (c *Client) Token(ctx context.Context) (string, error) {
	var token contract.TokenResponse
	err := c.Do(ctx, http.MethodGet, contract.PathToken, nil, &token)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		return "", nil
	}
	return token.Token, err
}

// SetTunnel starts or stops the server's Cloudflare Tunnel. It returns
// *APIError with 409 when the token is too weak and 403 for callers inside
// a WebMux session.
func (c *Client) SetTunnel(ctx context.Context, enabled bool) (contract.TunnelStatus, error) {
	var status contract.TunnelStatus
	err := c.Do(ctx, http.MethodPost, contract.PathTunnel, contract.TunnelRequest{Enabled: &enabled}, &status)
	return status, err
}

// Tailscale returns the full Tailscale view, including detection and enable
// preconditions; refresh bypasses the server's detection cache.
func (c *Client) Tailscale(ctx context.Context, refresh bool) (contract.TailscaleStatus, error) {
	path := "/api/tailscale"
	if refresh {
		path += "?refresh=1"
	}
	var status contract.TailscaleStatus
	err := c.Do(ctx, http.MethodGet, path, nil, &status)
	return status, err
}

// SetTailscale saves and applies Tailscale settings like the web settings
// page. It returns *APIError with 400 for invalid settings, 409 when the
// entry point cannot be enabled and 403 for callers inside a WebMux session.
func (c *Client) SetTailscale(ctx context.Context, update contract.TailscaleSettingsUpdate) (contract.TailscaleStatus, error) {
	var status contract.TailscaleStatus
	err := c.Do(ctx, http.MethodPut, "/api/tailscale/settings", update, &status)
	return status, err
}

// DialWebSocket opens /ws/* through the same dialer; path includes any query.
func (c *Client) DialWebSocket(ctx context.Context, path string) (*websocket.Conn, error) {
	conn, resp, err := websocket.Dial(ctx, "ws://webmux"+path, &websocket.DialOptions{
		HTTPClient:   c.HTTP,
		Subprotocols: []string{WSProtocol},
	})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, unwrapURLError(err)
	}
	return conn, nil
}
