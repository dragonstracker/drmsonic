package pipeline

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type recorder struct {
	mu      sync.Mutex
	status  []string
	errors  []string
	publish int
}

func (r *recorder) callbacks() Callbacks {
	return Callbacks{
		Publish: func(seg []byte, dur float64) {
			r.mu.Lock()
			r.publish++
			r.mu.Unlock()
		},
		Status: func(text string) {
			r.mu.Lock()
			r.status = append(r.status, text)
			r.mu.Unlock()
		},
		Info: func(string) {},
		Error: func(text string) {
			r.mu.Lock()
			r.errors = append(r.errors, text)
			r.mu.Unlock()
		},
		BwIn:   func(int64) {},
		Tracks: func(video, audio []TrackInfo) {},
	}
}

func (r *recorder) lastStatus() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.status) == 0 {
		return ""
	}
	return r.status[len(r.status)-1]
}

func TestRunHLSBroadcastEnded(t *testing.T) {
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, ".m3u8") {
			fmt.Fprintf(w, "#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:2,\n%s/seg0.ts\n#EXTINF:2,\n%s/seg1.ts\n#EXT-X-ENDLIST\n", base, base)
			return
		}
		w.Write(make([]byte, 16))
	}))
	defer srv.Close()
	base = srv.URL

	rec := &recorder{}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := Run(ctx, Config{ManifestURL: srv.URL + "/live.m3u8", Mode: "live", AllowPrivate: true}, rec.callbacks())

	if !errors.Is(err, ErrBroadcastEnded) {
		t.Fatalf("err = %v, want ErrBroadcastEnded", err)
	}
	if got := rec.lastStatus(); got != "Broadcast ended" {
		t.Fatalf("last status = %q", got)
	}
	if rec.publish != 2 {
		t.Fatalf("published %d segments, want 2", rec.publish)
	}
}

func TestRunHLSManifestForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()

	rec := &recorder{}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := Run(ctx, Config{ManifestURL: srv.URL + "/live.m3u8", Mode: "live", AllowPrivate: true}, rec.callbacks())

	if err == nil {
		t.Fatal("expected an error for a forbidden manifest")
	}
	if got := rec.lastStatus(); got != "Error: Access denied (403)\nhttp 403 for "+srv.URL+"/live.m3u8" {
		t.Fatalf("status = %q", got)
	}
	if len(rec.errors) != 1 || rec.errors[0] != "Access denied (403)" {
		t.Fatalf("errors = %v", rec.errors)
	}
}

func TestRunDASHIgnoreStatic(t *testing.T) {
	const mpd = `<?xml version="1.0"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" mediaPresentationDuration="PT4S">
  <Period>
    <AdaptationSet mimeType="video/mp4">
      <SegmentTemplate timescale="1000" duration="2000" initialization="init.mp4" media="$Number$.m4s"/>
      <Representation id="v" codecs="avc1.4d401f" bandwidth="1000"/>
    </AdaptationSet>
  </Period>
</MPD>`
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		hits++
		fmt.Fprint(w, mpd)
	}))
	defer srv.Close()

	rec := &recorder{}
	cfg := Config{ManifestURL: srv.URL + "/a.mpd", Mode: "live", AllowPrivate: true, IgnoreStatic: true}
	err := Run(context.Background(), cfg, rec.callbacks())

	if !errors.Is(err, ErrBroadcastEnded) {
		t.Fatalf("err = %v, want ErrBroadcastEnded", err)
	}
	if hits != 1 || rec.publish != 0 {
		t.Fatalf("hits = %d, published = %d, want only the manifest fetch", hits, rec.publish)
	}
}
