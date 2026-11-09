package web

import (
	"net/http"
	"net/http/httptest"
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

	for i := 0; i < 5; i++ {
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

func TestLiveEdgeSegmentWaits(t *testing.T) {
	prov := &config.Provider{Id: "channels", Name: "channels", RunningMode: config.RunInternalRemuxer, OutputMode: config.OutDirectHLS}
	prov.Streams = []config.Stream{{Id: "channel1", Name: "channel1", Mode: config.ModeLive, OnDemand: true}}
	s := testServer(t, Options{}, nil, prov)

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
