package web

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
)

var staticTypes = map[string]string{
	".css":  "text/css; charset=utf-8",
	".html": "text/html; charset=utf-8",
	".ico":  "image/x-icon",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".js":   "text/javascript; charset=utf-8",
	".png":  "image/png",
	".svg":  "image/svg+xml",
}

type staticAsset struct {
	body   []byte
	zipped []byte
	etag   string
	mime   string
}

type bundle struct {
	files []string
	mime  string
	join  string
}

var bundles = map[string]bundle{
	"app.js": {
		files: []string{
			"jquery.min.js",
			"jquery.dataTables.min.js",
			"bootstrap.bundle.min.js",
			"bootstrap-select.min.js",
			"dataTables.bootstrap5.min.js",
			"bootbox.min.js",
			"drmsonic.js",
		},
		mime: "text/javascript; charset=utf-8",
		join: "\n;\n",
	},
	"app.css": {
		files: []string{
			"bootstrap.min.css",
			"bootstrap-select.min.css",
			"drmsonic.css",
		},
		mime: "text/css; charset=utf-8",
		join: "\n",
	},
	"tz.js": {
		files: []string{
			"moment.min.js",
			"moment-timezone.min.js",
		},
		mime: "text/javascript; charset=utf-8",
		join: "\n;\n",
	},
}

type staticHandler struct {
	fs     fs.FS
	mu     sync.Mutex
	assets map[string]*staticAsset
}

func newStaticHandler(fsys fs.FS) *staticHandler {
	return &staticHandler{fs: fsys, assets: make(map[string]*staticAsset)}
}

func (h *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a := h.asset(strings.TrimPrefix(r.URL.Path, "/static/"))
	if a == nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", a.etag)
	w.Header().Set("Vary", "Accept-Encoding")

	if etagMatch(r.Header.Get("If-None-Match"), a.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", a.mime)
	if a.zipped != nil && acceptsGzip(r.Header.Get("Accept-Encoding")) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Write(a.zipped)
		return
	}

	w.Write(a.body)
}

func (h *staticHandler) asset(name string) *staticAsset {
	name = strings.TrimPrefix(path.Clean("/"+name), "/")
	if name == "" || name == "." {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if a, ok := h.assets[name]; ok {
		return a
	}

	var a *staticAsset
	if b, ok := bundles[name]; ok {
		a = h.combine(b)
	} else {
		body, err := fs.ReadFile(h.fs, name)
		if err != nil {
			return nil
		}
		a = newAsset(body, staticType(name))
	}

	h.assets[name] = a
	return a
}

func (h *staticHandler) combine(b bundle) *staticAsset {
	var buf bytes.Buffer
	for _, name := range b.files {
		body, err := fs.ReadFile(h.fs, name)
		if err != nil {
			return nil
		}
		buf.Write(body)
		buf.WriteString(b.join)
	}
	return newAsset(buf.Bytes(), b.mime)
}

func newAsset(body []byte, mime string) *staticAsset {
	sum := sha256.Sum256(body)
	a := &staticAsset{
		body: body,
		etag: `"` + hex.EncodeToString(sum[:12]) + `"`,
		mime: mime,
	}
	if len(body) > 1024 && compressible(mime) {
		var buf bytes.Buffer
		zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if err == nil {
			zw.Write(body)
			zw.Close()
			if buf.Len() < len(body) {
				a.zipped = buf.Bytes()
			}
		}
	}
	return a
}

func staticType(name string) string {
	if t, ok := staticTypes[strings.ToLower(path.Ext(name))]; ok {
		return t
	}
	return "application/octet-stream"
}

func compressible(mime string) bool {
	return strings.HasPrefix(mime, "text/") || strings.HasPrefix(mime, "image/svg") || strings.HasPrefix(mime, "application/json")
}

func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		coding := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if coding == "gzip" {
			return true
		}
	}
	return false
}

func etagMatch(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		if strings.TrimSpace(candidate) == etag {
			return true
		}
	}
	return false
}
