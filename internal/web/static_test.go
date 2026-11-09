package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func staticTestFS() fstest.MapFS {
	js := strings.Repeat("console.log('drmsonic');\n", 200)
	files := fstest.MapFS{
		"drmsonic.js":    {Data: []byte(js)},
		"logo.png":       {Data: []byte("PNGDATA")},
		"nested/app.css": {Data: []byte(strings.Repeat("body{margin:0}\n", 200))},
	}
	for _, b := range bundles {
		for _, name := range b.files {
			if _, ok := files[name]; !ok {
				files[name] = &fstest.MapFile{Data: []byte(js)}
			}
		}
	}
	return files
}

func TestStaticHandlerServesWithTypeAndEtag(t *testing.T) {
	h := newStaticHandler(staticTestFS())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/drmsonic.js", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	if rec.Header().Get("ETag") == "" {
		t.Fatal("missing etag")
	}
	if rec.Body.Len() == 0 {
		t.Fatal("empty body")
	}
}

func TestStaticHandlerCompressesText(t *testing.T) {
	h := newStaticHandler(staticTestFS())

	req := httptest.NewRequest(http.MethodGet, "/static/drmsonic.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("content encoding = %q", got)
	}
	if rec.Body.Len() >= 200*23 {
		t.Fatalf("body not compressed: %d bytes", rec.Body.Len())
	}
}

func TestStaticHandlerLeavesImagesAlone(t *testing.T) {
	h := newStaticHandler(staticTestFS())

	req := httptest.NewRequest(http.MethodGet, "/static/logo.png", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") != "" {
		t.Fatal("image was compressed")
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("content type = %q", got)
	}
}

func TestStaticHandlerRevalidates(t *testing.T) {
	h := newStaticHandler(staticTestFS())

	first := httptest.NewRecorder()
	h.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/static/drmsonic.js", nil))
	etag := first.Header().Get("ETag")

	req := httptest.NewRequest(http.MethodGet, "/static/drmsonic.js", nil)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotModified {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body = %d bytes", rec.Body.Len())
	}
}

func TestStaticHandlerRejectsTraversal(t *testing.T) {
	h := newStaticHandler(staticTestFS())

	for _, target := range []string{"/static/", "/static/../web.go", "/static/missing.js"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d", target, rec.Code)
		}
	}
}

func TestStaticHandlerBundlesInOrder(t *testing.T) {
	fses := staticTestFS()
	fses["static/jquery.min.js"] = &fstest.MapFile{Data: []byte("jquery")}
	fses["static/drmsonic.js"] = &fstest.MapFile{Data: []byte("drmsonic")}

	h := newStaticHandler(fses)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/app.js", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Index(body, "jquery") > strings.Index(body, "drmsonic") {
		t.Fatal("bundle order is wrong")
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
}

func TestGzipDynamicCompressesJSON(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write([]byte(strings.Repeat(`{"Id":"channel"},`, 200)))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/action", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	gzipDynamic(next).ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("content encoding = %q", got)
	}
	if rec.Body.Len() >= 200*17 {
		t.Fatalf("body not compressed: %d bytes", rec.Body.Len())
	}
}

func TestGzipDynamicLeavesCompressedResponsesAlone(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Content-Encoding", "gzip")
		w.Write([]byte("already compressed"))
	})

	req := httptest.NewRequest(http.MethodGet, "/static/app.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	gzipDynamic(next).ServeHTTP(rec, req)

	if rec.Body.String() != "already compressed" {
		t.Fatalf("body was re-encoded: %q", rec.Body.String())
	}
}

func TestGzipDynamicSkipsBinary(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		w.Write([]byte("segment"))
	})

	req := httptest.NewRequest(http.MethodGet, "/media/segment.ts", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	gzipDynamic(next).ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") != "" {
		t.Fatal("segment was compressed")
	}
	if rec.Body.String() != "segment" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}
