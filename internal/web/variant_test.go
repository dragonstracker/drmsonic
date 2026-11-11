package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"drmsonic/internal/config"
)

func TestBitrateFromBuffer(t *testing.T) {
	prov := &config.Provider{Id: "channels", Name: "channels", RunningMode: config.RunInternalRemuxer, OutputMode: config.OutDirectHLS}
	prov.Streams = []config.Stream{{Id: "channel1", Name: "channel1", Mode: config.ModeLive}}
	s := testServer(t, Options{}, nil, prov)

	if got := s.media.bitrate("channels", "channel1", ""); got != 0 {
		t.Fatalf("empty buffer bitrate = %d, want 0", got)
	}

	for i := 0; i < 4; i++ {
		s.media.push("channels", "channel1", "", make([]byte, 600000), 2, 6)
	}

	got := s.media.bitrate("channels", "channel1", "")
	want := 2400000
	if got != want {
		t.Fatalf("bitrate = %d, want %d", got, want)
	}
}

func TestMasterPlaylistDeclaresMeasuredBandwidth(t *testing.T) {
	prov := &config.Provider{Id: "channels", Name: "channels", RunningMode: config.RunInternalRemuxer, OutputMode: config.OutDirectHLS}
	prov.Streams = []config.Stream{{
		Id:       "channel1",
		Name:     "channel1",
		Mode:     config.ModeLive,
		OnDemand: true,
		VideoList: []config.Track{{
			Id: "best", Codecs: "avc1.640028", Width: 1920, Height: 1080, Bandwidth: 100,
		}},
	}}
	s := testServer(t, Options{}, nil, prov)
	s.mgr.Start("channels", "channel1")

	for i := 0; i < 3; i++ {
		s.media.push("channels", "channel1", "", make([]byte, 900000), 2, 6)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream/channels/channel1/master.m3u8", nil)
	s.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "BANDWIDTH=3600000,") {
		t.Fatalf("master playlist did not use measured bandwidth:\n%s", body)
	}
}

func TestPlaylistsUseStandardHLSMIME(t *testing.T) {
	prov := &config.Provider{Id: "channels", Name: "channels", RunningMode: config.RunInternalRemuxer, OutputMode: config.OutDirectHLS}
	prov.Streams = []config.Stream{{Id: "channel1", Name: "channel1", Mode: config.ModeLive, OnDemand: true}}
	s := testServer(t, Options{}, nil, prov)
	s.mgr.Start("channels", "channel1")
	s.media.push("channels", "channel1", "", []byte("seg"), 2, 6)

	for _, path := range []string{
		"/stream/channels/channel1/master.m3u8",
		"/stream/channels/channel1/stream.m3u8",
	} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		s.Handler().ServeHTTP(rr, req)
		if got := rr.Header().Get("Content-Type"); got != "application/vnd.apple.mpegurl" {
			t.Fatalf("%s: Content-Type %q", path, got)
		}
	}
}
