package mcpcontrol

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/localclient"
)

type testDialer struct{ address string }

func (d testDialer) DialContext(ctx context.Context) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "tcp", d.address)
}
func websocketClient(t *testing.T, handler http.HandlerFunc) Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	api := localclient.New(testDialer{strings.TrimPrefix(server.URL, "http://")})
	t.Cleanup(api.Close)
	return Client{API: api}
}
