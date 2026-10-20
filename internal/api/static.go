package api

import (
	"bytes"
	"compress/gzip"
	"io/fs"
	"math"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"

	assets "github.com/rainchan/webmux/web"
)

// The inventory is fixed by the embedded build. Request paths and query strings
// never add entries, and each file's representations are initialized once.
var staticBuild, staticBuildErr = embeddedStaticFiles()

type staticFiles struct {
	root  fs.FS
	files map[string]*staticAsset
}

type staticAsset struct {
	once        sync.Once
	data        []byte
	gzip        []byte
	contentType string
	err         error
}

func embeddedStaticFiles() (*staticFiles, error) {
	root, err := fs.Sub(assets.Assets, "dist")
	if err != nil {
		return nil, err
	}
	return newStaticFiles(root)
}

func newStaticFiles(root fs.FS) (*staticFiles, error) {
	files := &staticFiles{root: root, files: make(map[string]*staticAsset)}
	err := fs.WalkDir(root, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			files.files[name] = &staticAsset{contentType: staticContentType(name)}
		}
		return nil
	})
	return files, err
}

func (files *staticFiles) get(name string) (*staticAsset, error) {
	asset, ok := files.files[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	asset.once.Do(func() {
		asset.data, asset.err = fs.ReadFile(files.root, name)
		if asset.err != nil || len(asset.data) < 1024 || !compressibleStaticType(asset.contentType) {
			return
		}
		var buf bytes.Buffer
		zw, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
		if err != nil {
			return
		}
		if _, err = zw.Write(asset.data); err != nil {
			_ = zw.Close()
			return
		}
		if err = zw.Close(); err == nil {
			asset.gzip = buf.Bytes()
		}
	})
	return asset, asset.err
}

func staticContentType(name string) string {
	switch path.Ext(name) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".webmanifest":
		return "application/manifest+json"
	case ".svg":
		return "image/svg+xml"
	case ".woff2":
		return "font/woff2"
	case ".txt":
		return "text/plain; charset=utf-8"
	default:
		return ""
	}
}

func compressibleStaticType(contentType string) bool {
	return strings.HasPrefix(contentType, "text/") || contentType == "application/manifest+json" || contentType == "image/svg+xml"
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if staticBuildErr != nil {
		http.Error(w, "frontend build unavailable", http.StatusServiceUnavailable)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	if strings.HasPrefix(name, ".") || strings.Contains(name, "/.") {
		http.NotFound(w, r)
		return
	}
	asset, err := staticBuild.get(name)
	if err != nil {
		if strings.HasPrefix(name, "assets/") || path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
		name = "index.html"
		asset, err = staticBuild.get(name)
	}
	if err != nil {
		http.Error(w, "run make web before building", http.StatusServiceUnavailable)
		return
	}
	if asset.contentType != "" {
		w.Header().Set("Content-Type", asset.contentType)
	}
	// Font filenames include a content hash, so they can be cached as immutable.
	if strings.HasPrefix(name, "assets/") || path.Ext(name) == ".woff2" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	asset.write(w, r)
}

func (asset *staticAsset) write(w http.ResponseWriter, r *http.Request) {
	gzipQuality, identityQuality := staticEncodingQualities(strings.Join(r.Header.Values("Accept-Encoding"), ","))
	if len(asset.gzip) > 0 {
		w.Header().Add("Vary", "Accept-Encoding")
	}
	if len(asset.gzip) > 0 && gzipQuality > 0 && gzipQuality >= identityQuality {
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(asset.gzip)
		return
	}
	if identityQuality <= 0 {
		// A rejected representation must not inherit the immutable asset cache policy.
		w.Header().Del("Cache-Control")
		http.Error(w, "no acceptable content encoding", http.StatusNotAcceptable)
		return
	}
	_, _ = w.Write(asset.data)
}

func staticEncodingQualities(header string) (gzipQuality, identityQuality float64) {
	identityQuality = 1
	qualities := make(map[string]float64, 3)
	for _, item := range strings.Split(header, ",") {
		parts := strings.Split(item, ";")
		coding := strings.ToLower(strings.TrimSpace(parts[0]))
		if coding != "gzip" && coding != "identity" && coding != "*" {
			continue
		}
		quality := 1.0
		for _, parameter := range parts[1:] {
			key, value, ok := strings.Cut(parameter, "=")
			if strings.EqualFold(strings.TrimSpace(key), "q") {
				parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
				if !ok || err != nil || math.IsNaN(parsed) || parsed < 0 || parsed > 1 {
					quality = 0
				} else {
					quality = parsed
				}
			}
		}
		qualities[coding] = quality
	}
	if wildcard, ok := qualities["*"]; ok {
		gzipQuality = wildcard
		if wildcard == 0 {
			identityQuality = 0
		}
	}
	if quality, ok := qualities["gzip"]; ok {
		gzipQuality = quality
	}
	if quality, ok := qualities["identity"]; ok {
		identityQuality = quality
	}
	return gzipQuality, identityQuality
}
