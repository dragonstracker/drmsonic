package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"drmsonic/internal/config"
)

func TestSegmentSeq(t *testing.T) {
	cases := map[string]uint64{
		"stream00001.ts": 1,
		"stream01234.ts": 1234,
		"stream123.ts":   123,
	}
	for name, want := range cases {
		got, ok := segmentSeq(name)
		if !ok || got != want {
			t.Fatalf("%s: got %d ok=%v want %d", name, got, ok, want)
		}
	}
	for _, bad := range []string{"stream.ts", "streamabc.ts", "stream1.ts.bak"} {
		if _, ok := segmentSeq(bad); ok {
			t.Fatalf("%s: expected parse failure", bad)
		}
	}
}

func TestExpiredSegmentFailsFast(t *testing.T) {
	prov := &config.Provider{Id: "channels", Name: "channels", RunningMode: config.RunInternalRemuxer, OutputMode: config.OutDirectHLS}
	prov.Streams = []config.Stream{{Id: "channel1", Name: "channel1", Mode: config.ModeLive, OnDemand: true}}
	s := testServer(t, Options{}, nil, prov)
	s.mgr.Start("channels", "channel1")

	for i := 0; i < 12; i++ {
		s.media.push("channels", "channel1", "", []byte("x"), 2, 3)
	}

	start := time.Now()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream/channels/channel1/stream00000.ts", nil)

	s.Handler().ServeHTTP(rr, req)
	elapsed := time.Since(start)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status %d", rr.Code)
	}
	if elapsed > time.Second {
		t.Fatalf("expired segment waited %s", elapsed)
	}
}

func TestPlaylistKeepsSpareSegments(t *testing.T) {
	prov := &config.Provider{Id: "channels", Name: "channels", RunningMode: config.RunInternalRemuxer, OutputMode: config.OutDirectHLS}
	prov.Streams = []config.Stream{{Id: "channel1", Name: "channel1", Mode: config.ModeLive, OnDemand: true, NbAnnouncedFragments: 4}}
	s := testServer(t, Options{}, nil, prov)
	s.mgr.Start("channels", "channel1")

	for i := 0; i < 10; i++ {
		s.media.push("channels", "channel1", "", []byte("x"), 2, 4)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream/channels/channel1/stream.m3u8", nil)
	s.Handler().ServeHTTP(rr, req)

	body := rr.Body.String()
	if n := strings.Count(body, ".ts"); n != 4 {
		t.Fatalf("announced %d segments, want 4\n%s", n, body)
	}
	if !strings.Contains(body, "stream00006.ts") {
		t.Fatalf("oldest announced segment should be 6:\n%s", body)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/stream/channels/channel1/stream00003.ts", nil)
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("spare segment status %d", rr.Code)
	}
}

func TestBufferedSegmentServesImmediately(t *testing.T) {
	prov := &config.Provider{Id: "channels", Name: "channels", RunningMode: config.RunInternalRemuxer, OutputMode: config.OutDirectHLS}
	prov.Streams = []config.Stream{{Id: "channel1", Name: "channel1", Mode: config.ModeLive, OnDemand: true}}
	s := testServer(t, Options{}, nil, prov)
	s.mgr.Start("channels", "channel1")

	s.media.push("channels", "channel1", "", []byte("seg"), 2, 6)

	start := time.Now()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream/channels/channel1/stream00000.ts", nil)
	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || rr.Body.String() != "seg" {
		t.Fatalf("status %d body %q", rr.Code, rr.Body.String())
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("buffered segment took %s", elapsed)
	}
}

func TestLiveEdgeSegmentWaits(t *testing.T) {
	prov := &config.Provider{Id: "channels", Name: "channels", RunningMode: config.RunInternalRemuxer, OutputMode: config.OutDirectHLS}
	prov.Streams = []config.Stream{{Id: "channel1", Name: "channel1", Mode: config.ModeLive, OnDemand: true}}
	s := testServer(t, Options{}, nil, prov)
	s.mgr.Start("channels", "channel1")

	for i := 0; i < 2; i++ {
		s.media.push("channels", "channel1", "", []byte("seg"), 2, 6)
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		s.media.push("channels", "channel1", "", []byte("next"), 2, 6)
	}()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream/channels/channel1/stream00002.ts", nil)

	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || rr.Body.String() != "next" {
		t.Fatalf("status %d body %q", rr.Code, rr.Body.String())
	}
}

func TestFutureSegmentFailsFast(t *testing.T) {
	prov := &config.Provider{Id: "channels", Name: "channels", RunningMode: config.RunInternalRemuxer, OutputMode: config.OutDirectHLS}
	prov.Streams = []config.Stream{{Id: "channel1", Name: "channel1", Mode: config.ModeLive, OnDemand: true}}
	s := testServer(t, Options{}, nil, prov)
	s.mgr.Start("channels", "channel1")

	for i := 0; i < 3; i++ {
		s.media.push("channels", "channel1", "", []byte("seg"), 2, 6)
	}

	start := time.Now()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream/channels/channel1/stream00050.ts", nil)
	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status %d", rr.Code)
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("future segment waited %s", elapsed)
	}
}

func TestPlaylistServesWhileFilling(t *testing.T) {
	prov := &config.Provider{Id: "channels", Name: "channels", RunningMode: config.RunInternalRemuxer, OutputMode: config.OutDirectHLS}
	prov.Streams = []config.Stream{{Id: "channel1", Name: "channel1", Mode: config.ModeLive, OnDemand: true}}
	s := testServer(t, Options{}, nil, prov)
	s.mgr.Start("channels", "channel1")
	s.media.push("channels", "channel1", "", []byte("seg"), 2, 6)

	start := time.Now()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream/channels/channel1/stream.m3u8", nil)
	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("playlist with one segment waited %s", elapsed)
	}
	if !strings.Contains(rr.Body.String(), "stream00000.ts") {
		t.Fatalf("single segment missing:\n%s", rr.Body.String())
	}
}

func TestMediaResponsesAllowCrossOrigin(t *testing.T) {
	prov := &config.Provider{Id: "channels", Name: "channels", RunningMode: config.RunInternalRemuxer, OutputMode: config.OutDirectHLS}
	prov.Streams = []config.Stream{{Id: "channel1", Name: "channel1", Mode: config.ModeLive, OnDemand: true}}
	s := testServer(t, Options{}, nil, prov)
	s.mgr.Start("channels", "channel1")
	s.media.push("channels", "channel1", "", []byte("seg"), 2, 6)

	for _, path := range []string{
		"/stream/channels/channel1/master.m3u8",
		"/stream/channels/channel1/stream.m3u8",
		"/stream/channels/channel1/stream00000.ts",
		"/playlist.m3u",
		"/app1.conf",
	} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Host = "host:7779"
		s.Handler().ServeHTTP(rr, req)
		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Fatalf("%s: Access-Control-Allow-Origin %q", path, got)
		}
	}
}
