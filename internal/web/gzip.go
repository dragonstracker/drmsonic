package web

import (
	"compress/gzip"
	"net/http"
)

type gzipResponse struct {
	http.ResponseWriter
	compressor *gzip.Writer
	status     int
	decided    bool
}

func gzipDynamic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !acceptsGzip(r.Header.Get("Accept-Encoding")) || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipResponse{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(gw, r)
		gw.finish()
	})
}

func (w *gzipResponse) WriteHeader(status int) {
	w.status = status
}

func (w *gzipResponse) Write(p []byte) (int, error) {
	w.once()
	if w.compressor != nil {
		return w.compressor.Write(p)
	}
	return w.ResponseWriter.Write(p)
}

func (w *gzipResponse) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		w.once()
		if w.compressor != nil {
			w.compressor.Flush()
		}
		f.Flush()
	}
}

func (w *gzipResponse) once() {
	if w.decided {
		return
	}
	w.decided = true

	h := w.ResponseWriter.Header()
	if h.Get("Content-Encoding") != "" || !compressible(h.Get("Content-Type")) || w.status == http.StatusNotModified || w.status == http.StatusNoContent {
		w.ResponseWriter.WriteHeader(w.status)
		return
	}

	h.Set("Content-Encoding", "gzip")
	h.Add("Vary", "Accept-Encoding")
	h.Del("Content-Length")
	w.ResponseWriter.WriteHeader(w.status)
	w.compressor = gzip.NewWriter(w.ResponseWriter)
}

func (w *gzipResponse) finish() {
	w.once()
	if w.compressor != nil {
		w.compressor.Close()
	}
}
