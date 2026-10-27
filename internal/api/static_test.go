package api

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"

	assets "github.com/rainchan/webmux/web"
)

func staticStylesheet(t *testing.T) (string, []byte) {
	t.Helper()
	var name string
	var data []byte
	err := fs.WalkDir(assets.Assets, "dist", func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(p, ".css") {
			return nil
		}
		b, err := fs.ReadFile(assets.Assets, p)
		if err != nil {
			return err
		}
		if len(b) > len(data) {
			name, data = strings.TrimPrefix(p, "dist/"), b
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 1024 {
		t.Fatal("embedded build needs a stylesheet of at least 1024 bytes")
	}
	return name, data
}

func TestStaticEncodingNegotiation(t *testing.T) {
	name, _ := staticStylesheet(t)
	for _, tc := range []struct {
		accept, encoding string
		status           int
	}{
		{"", "", 200},
		{"gzip", "gzip", 200},
		{"gzip;q=0", "", 200},
		{"gzip;q=0, *;q=1", "", 200},
		{"*", "gzip", 200},
		{"GZIP", "gzip", 200},
		{"xgzip", "", 200},
		{"*;q=0, gzip;q=1", "gzip", 200},
		{"*;q=0, identity;q=1", "", 200},
		{"gzip;q=2", "", 200},
		{"gzip;q=0.5, identity;q=0.1", "gzip", 200},
		{"gzip;q=0.5, identity;q=1", "", 200},
		{"gzip;q=0, identity;q=0", "", 406},
		{"*;q=0", "", 406},
		{"br, identity;q=0", "", 406},
		{"gzip;q=invalid", "", 200},
		{"gzip;q=NaN", "", 200},
	} {
		t.Run(tc.accept, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/"+name, nil)
			req.Header.Set("Accept-Encoding", tc.accept)
			w := httptest.NewRecorder()
			(&Server{}).static(w, req)
			if w.Code != tc.status || w.Header().Get("Content-Encoding") != tc.encoding {
				t.Fatalf("status=%d encoding=%q; want %d %q", w.Code, w.Header().Get("Content-Encoding"), tc.status, tc.encoding)
			}
			if !strings.Contains(w.Header().Get("Vary"), "Accept-Encoding") {
				t.Fatalf("Vary=%q", w.Header().Get("Vary"))
			}
		})
	}
}

func TestStaticConcurrentRepresentations(t *testing.T) {
	name, want := staticStylesheet(t)
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/%s?v=%d", name, i), nil)
			if i%2 == 0 {
				req.Header.Set("Accept-Encoding", "gzip")
			}
			w := httptest.NewRecorder()
			(&Server{}).static(w, req)
			if w.Code != 200 {
				t.Errorf("status=%d", w.Code)
				return
			}
			data := w.Body.Bytes()
			if i%2 == 0 {
				zr, err := gzip.NewReader(bytes.NewReader(data))
				if err != nil {
					t.Error(err)
					return
				}
				defer func() {
					if err := zr.Close(); err != nil {
						t.Error(err)
					}
				}()
				data, err = io.ReadAll(zr)
				if err != nil {
					t.Error(err)
					return
				}
			} else if w.Header().Get("Content-Encoding") != "" {
				t.Error("identity response compressed")
			}
			if !bytes.Equal(data, want) {
				t.Error("response does not match embedded stylesheet")
			}
			if w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
				t.Error("lost immutable cache policy")
			}
		}()
	}
	wg.Wait()
}

// Implement ReadFileFS so reads count file content loads rather than inventory
// directory scans. The underlying map is immutable throughout the test.
type countedStaticFS struct {
	fs.FS
	reads atomic.Int32
}

func (files *countedStaticFS) ReadFile(name string) ([]byte, error) {
	files.reads.Add(1)
	return fs.ReadFile(files.FS, name)
}

func TestStaticCacheReadsKnownAssetsOnce(t *testing.T) {
	want := []byte(strings.Repeat("body { color: green; }\n", 100))
	root := &countedStaticFS{FS: fstest.MapFS{
		"assets/app.css": {Data: want},
		"index.html":     {Data: []byte("<!doctype html>")},
	}}
	cache, err := newStaticFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if root.reads.Load() != 0 {
		t.Fatal("inventory eagerly read asset contents")
	}
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			asset, err := cache.get("assets/app.css")
			if err != nil {
				t.Error(err)
				return
			}
			req := httptest.NewRequest(http.MethodGet, "/assets/app.css", nil)
			if i%2 == 0 {
				req.Header.Set("Accept-Encoding", "gzip")
			}
			w := httptest.NewRecorder()
			asset.write(w, req)
			data := w.Body.Bytes()
			if i%2 == 0 {
				reader, err := gzip.NewReader(bytes.NewReader(data))
				if err != nil {
					t.Error(err)
					return
				}
				data, err = io.ReadAll(reader)
				_ = reader.Close()
				if err != nil {
					t.Error(err)
					return
				}
			}
			if !bytes.Equal(data, want) {
				t.Error("cached contents changed")
			}
		}()
	}
	wg.Wait()
	if got := root.reads.Load(); got != 1 {
		t.Fatalf("read asset %d times, want 1", got)
	}
	first, _ := cache.get("assets/app.css")
	again, _ := cache.get("assets/app.css")
	if first != again || &first.data[0] != &again.data[0] || &first.gzip[0] != &again.gzip[0] {
		t.Fatal("requests do not share cached raw and gzip representations")
	}
	for i := range 1000 {
		if _, err := cache.get(fmt.Sprintf("missing-%d", i)); err != fs.ErrNotExist {
			t.Fatalf("unknown asset error=%v", err)
		}
	}
	if len(cache.files) != 2 || root.reads.Load() != 1 {
		t.Fatal("unknown paths expanded the fixed asset inventory")
	}
}

func TestStaticUncompressedAssetNegotiation(t *testing.T) {
	asset := &staticAsset{data: []byte("small body")}
	for _, tc := range []struct {
		accept string
		status int
	}{
		{"gzip", 200},
		{"*;q=0, identity;q=1", 200},
		{"gzip, identity;q=0", 406},
	} {
		req := httptest.NewRequest(http.MethodGet, "/small.txt", nil)
		req.Header.Set("Accept-Encoding", tc.accept)
		w := httptest.NewRecorder()
		asset.write(w, req)
		if w.Code != tc.status {
			t.Fatalf("Accept-Encoding=%q: status=%d want=%d", tc.accept, w.Code, tc.status)
		}
		if w.Header().Get("Content-Encoding") != "" {
			t.Fatal("compressed unavailable representation")
		}
	}
}

func BenchmarkStaticCachedGzip(b *testing.B) {
	root := fstest.MapFS{"app.js": {Data: []byte(strings.Repeat("console.log('embedded asset');\n", 10000))}}
	cache, err := newStaticFiles(root)
	if err != nil {
		b.Fatal(err)
	}
	asset, err := cache.get("app.js")
	if err != nil {
		b.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		w := httptest.NewRecorder()
		asset.write(w, req)
	}
}

// The installability checks browsers run: the manifest is served with its
// media type and fresh, and every icon it lists decodes at its declared size.
func TestStaticWebAppManifest(t *testing.T) {
	get := func(t *testing.T, target string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		(&Server{}).static(w, httptest.NewRequest(http.MethodGet, target, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d", target, w.Code)
		}
		return w
	}
	w := get(t, "/manifest.webmanifest")
	if got := w.Header().Get("Content-Type"); got != "application/manifest+json" {
		t.Fatalf("Content-Type=%q", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control=%q", got)
	}
	var manifest struct {
		ID, Lang, Scope, Display string
		StartURL                 string `json:"start_url"`
		Icons                    []struct{ Src, Sizes, Type string }
	}
	if err := json.Unmarshal(w.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ID != "/" || manifest.Lang != "zh-CN" || manifest.StartURL != "/" || manifest.Scope != "/" || manifest.Display != "standalone" {
		t.Fatalf("manifest=%+v", manifest)
	}
	sizes := map[string]bool{}
	for _, icon := range manifest.Icons {
		w := get(t, icon.Src)
		if got := w.Header().Get("Content-Type"); got != icon.Type {
			t.Fatalf("%s Content-Type=%q; want %q", icon.Src, got, icon.Type)
		}
		config, err := png.DecodeConfig(w.Body)
		if err != nil {
			t.Fatalf("%s: %v", icon.Src, err)
		}
		if got := fmt.Sprintf("%dx%d", config.Width, config.Height); got != icon.Sizes {
			t.Fatalf("%s is %s; manifest says %s", icon.Src, got, icon.Sizes)
		}
		sizes[icon.Sizes] = true
	}
	if !sizes["192x192"] || !sizes["512x512"] {
		t.Fatalf("icon sizes=%v; installability needs 192x192 and 512x512", sizes)
	}
}
