package localclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type responseCall struct {
	name string
	call func(*Client, context.Context, any) error
}

// Exercise both JSON requests and multipart uploads over the real transport.
var responseCalls = []responseCall{
	{"Do", func(c *Client, ctx context.Context, out any) error {
		return c.Do(ctx, http.MethodPost, "/response", nil, out)
	}},
	{"Upload", func(c *Client, ctx context.Context, out any) error {
		return c.Upload(ctx, "/response", "test.txt", strings.NewReader("upload"), out)
	}},
}

func TestDiscardedResponseReportsTruncation(t *testing.T) {
	for _, tt := range responseCalls {
		t.Run(tt.name, func(t *testing.T) {
			client := New(serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", "10")
				if _, err := io.WriteString(w, "short"); err != nil {
					t.Error(err)
				}
			})))
			t.Cleanup(client.Close)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := tt.call(client, ctx, nil); !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("truncated response error = %v, want unexpected EOF", err)
			}
		})
	}
}

func TestDiscardedResponseReusesConnection(t *testing.T) {
	for _, tt := range responseCalls {
		t.Run(tt.name, func(t *testing.T) {
			listener := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, err := io.WriteString(w, "ignored response"); err != nil {
					t.Error(err)
				}
			}))
			client := New(listener)
			t.Cleanup(client.Close)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			for range 2 {
				if err := tt.call(client, ctx, nil); err != nil {
					t.Fatal(err)
				}
			}
			if got := listener.dials.Load(); got != 1 {
				t.Fatalf("dials = %d, want 1 after draining responses", got)
			}
		})
	}
}

func TestJSONResponseDecoding(t *testing.T) {
	for _, tt := range responseCalls {
		t.Run(tt.name, func(t *testing.T) {
			client := New(serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, err := io.WriteString(w, `{"value":"decoded"}`); err != nil {
					t.Error(err)
				}
			})))
			t.Cleanup(client.Close)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var out struct{ Value string }
			if err := tt.call(client, ctx, &out); err != nil {
				t.Fatal(err)
			}
			if out.Value != "decoded" {
				t.Fatalf("decoded value = %q", out.Value)
			}
		})
	}
}

func TestResponseErrorsPreserveBody(t *testing.T) {
	calls := append(responseCalls[:len(responseCalls):len(responseCalls)],
		responseCall{"Get", func(c *Client, ctx context.Context, _ any) error {
			_, err := c.Get(ctx, "/response")
			return err
		}},
		responseCall{"GetBounded", func(c *Client, ctx context.Context, out any) error {
			return c.GetBounded(ctx, "/response", 1024, out)
		}},
	)
	tests := []struct {
		name        string
		give        string
		wantMessage string
	}{
		{"structured", `{"error":"conflict","revision":2}`, "conflict"},
		{"plain text", "server unavailable", ""},
		{"exact limit", `{"error":"conflict","padding":"` + strings.Repeat("x", 65503) + `"}`, "conflict"},
		{"oversized", `{"error":"` + strings.Repeat("x", 65536) + `"}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, call := range calls {
				t.Run(call.name, func(t *testing.T) {
					client := New(serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusConflict)
						// An oversized error may be closed before the server finishes writing.
						_, _ = io.WriteString(w, tt.give)
					})))
					t.Cleanup(client.Close)
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					var failure *APIError
					if err := call.call(client, ctx, nil); !errors.As(err, &failure) {
						t.Fatalf("error = %v, want APIError", err)
					}
					if failure.Status != http.StatusConflict || failure.Message != tt.wantMessage {
						t.Fatalf("error = %+v", failure)
					}
					wantBody := []byte(tt.give[:min(len(tt.give), 65536)])
					if !bytes.Equal(failure.Body, wantBody) {
						t.Fatalf("error body length = %d, want matching %d bytes", len(failure.Body), len(wantBody))
					}
				})
			}
		})
	}
}
