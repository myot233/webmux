package api

import (
	"compress/gzip"
	"context"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/session"
	assets "github.com/rainchan/webmux/web"
)

func TestAuthenticationAndOriginBoundaries(t *testing.T) {
	c := config.Config{Token: "secret", AllowedOrigins: []string{"https://trusted.example"}}
	m := session.NewManager(session.Options{})
	defer m.Shutdown(context.Background())
	handler := New(c, m, testWorkspace(t, m, t.TempDir()))
	for _, row := range []struct {
		name, authorization, cookie, query, origin string
		want                                       int
	}{
		{"bearer", "Bearer secret", "", "", "http://webmux.example", 200},
		{"bearer case insensitive", "bearer secret", "", "", "", 200},
		{"cookie", "", "secret", "", "", 200},
		{"HTTP query forbidden", "", "", "?token=secret", "", 401},
		{"incorrect bearer", "Bearer wrong", "secret", "", "", 401},
		{"allowed cross origin", "Bearer secret", "", "", "https://trusted.example", 200},
		{"untrusted cross origin", "Bearer secret", "", "", "https://evil.example", 403},
		{"different scheme", "Bearer secret", "", "", "https://webmux.example", 403},
		{"opaque origin", "Bearer secret", "", "", "null", 403},
		{"origin query", "Bearer secret", "", "", "http://webmux.example?ignored", 403},
		{"empty origin query", "Bearer secret", "", "", "http://webmux.example?", 403},
		{"origin fragment", "Bearer secret", "", "", "http://webmux.example#ignored", 403},
		{"empty origin fragment", "Bearer secret", "", "", "http://webmux.example#", 403},
		{"origin credentials", "Bearer secret", "", "", "http://user@webmux.example", 403},
	} {
		t.Run(row.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "http://webmux.example/api/health"+row.query, nil)
			r.Header.Set("Authorization", row.authorization)
			r.Header.Set("Origin", row.origin)
			if row.cookie != "" {
				r.AddCookie(&http.Cookie{Name: "webmux_token", Value: row.cookie})
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != row.want {
				t.Fatalf("HTTP status %d, want %d: %s", w.Code, row.want, w.Body.String())
			}
			if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Referrer-Policy") != "no-referrer" {
				t.Fatal("missing security response headers")
			}
		})
	}
}

func TestWebSocketHandshakeBoundaries(t *testing.T) {
	m := session.NewManager(session.Options{})
	defer m.Shutdown(context.Background())
	server := httptest.NewServer(New(config.Config{Token: "secret"}, m, testWorkspace(t, m, t.TempDir())))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, row := range []struct {
		query, cookie, origin string
		want                  int
	}{
		{"", "", "", 401},
		{"?token=wrong", "", "", 401},
		{"?token=secret", "", "https://evil.example", 403},
		{"?token=secret", "", server.URL, 101},
		{"", "webmux_token=secret", server.URL, 101},
	} {
		header := make(http.Header)
		header.Set("Cookie", row.cookie)
		header.Set("Origin", row.origin)
		conn, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws/s/missing"+row.query, &websocket.DialOptions{HTTPHeader: header})
		if response == nil || response.StatusCode != row.want {
			t.Fatalf("handshake status: response=%v err=%v want=%d", response, err, row.want)
		}
		if row.want != 101 {
			if err == nil {
				_ = conn.CloseNow()
				t.Fatal("unauthorized websocket upgraded")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = conn.Read(ctx)
		_ = conn.CloseNow()
		if websocket.CloseStatus(err) != 4404 {
			t.Fatalf("missing session close: %v", err)
		}
	}
}

func TestStaticFontFilesAreTypedAndImmutable(t *testing.T) {
	root, err := fs.Sub(assets.Assets, "dist")
	if err != nil {
		t.Fatal(err)
	}
	var name string
	err = fs.WalkDir(root, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || name != "" || entry.IsDir() || !strings.HasSuffix(path, ".woff2") {
			return err
		}
		name = path
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if name == "" {
		t.Skip("frontend build has no font files")
	}
	m := session.NewManager(session.Options{})
	defer m.Shutdown(context.Background())
	handler := New(config.Config{}, m, testWorkspace(t, m, t.TempDir()))
	r := httptest.NewRequest(http.MethodGet, "/"+name, nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d for %s", w.Code, name)
	}
	if w.Header().Get("Content-Type") != "font/woff2" {
		t.Fatalf("content type %q", w.Header().Get("Content-Type"))
	}
	if w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("cache control %q", w.Header().Get("Cache-Control"))
	}
	if w.Header().Get("Content-Encoding") != "" {
		t.Fatal("woff2 responses are already compressed")
	}
	if w.Body.Len() < 100 {
		t.Fatalf("font response is %d bytes", w.Body.Len())
	}
}

func TestStaticStylesheetGzip(t *testing.T) {
	root, err := fs.Sub(assets.Assets, "dist")
	if err != nil {
		t.Fatal(err)
	}
	var name string
	var largest int64
	err = fs.WalkDir(root, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".css") {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > largest {
			largest = info.Size()
			name = path
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if name == "" {
		t.Skip("frontend build has no stylesheet")
	}
	m := session.NewManager(session.Options{})
	defer m.Shutdown(context.Background())
	handler := New(config.Config{}, m, testWorkspace(t, m, t.TempDir()))
	r := httptest.NewRequest(http.MethodGet, "/"+name, nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d for %s", w.Code, name)
	}
	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("content encoding %q", w.Header().Get("Content-Encoding"))
	}
	reader, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) < 100 {
		t.Fatalf("decoded stylesheet is %d bytes", len(body))
	}
}

func TestStaticFallbackDoesNotMaskMissingResources(t *testing.T) {
	m := session.NewManager(session.Options{})
	defer m.Shutdown(context.Background())
	handler := New(config.Config{Token: "secret"}, m, testWorkspace(t, m, t.TempDir()))
	for _, row := range []struct {
		path string
		want int
	}{
		{"/", 200},
		{"/s/example", 200},
		{"/assets/missing.js", 404},
		{"/assets/missing", 404},
		{"/missing.css", 404},
		{"/.env", 404},
		{"/nested/.env", 404},
		{"/api/missing", 404},
		{"/ws/missing", 404},
	} {
		r := httptest.NewRequest(http.MethodGet, row.path, nil)
		if strings.HasPrefix(row.path, "/api/") || strings.HasPrefix(row.path, "/ws/") {
			r.Header.Set("Authorization", "Bearer secret")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != row.want {
			t.Errorf("%s: status=%d want=%d", row.path, w.Code, row.want)
		}
		if row.want == 200 && !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
			t.Errorf("%s: fallback content type %q", row.path, w.Header().Get("Content-Type"))
		}
	}
}
