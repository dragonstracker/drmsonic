package manifest

import (
	"net/url"
	"testing"
)

const sampleMPD = `<?xml version="1.0" encoding="UTF-8"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" mediaPresentationDuration="PT6S">
  <BaseURL>https://cdn.example.com/live/</BaseURL>
  <Period start="PT0S">
    <AdaptationSet mimeType="video/mp4" contentType="video">
      <SegmentTemplate timescale="1000" duration="2000" startNumber="10"
        initialization="$RepresentationID$/init.mp4"
        media="$RepresentationID$/seg-$Number$.m4s"/>
      <Representation id="v1080" codecs="avc1.640028" bandwidth="8000000" width="1920" height="1080"/>
      <Representation id="v720" codecs="avc1.64001F" bandwidth="3000000" width="1280" height="720"/>
    </AdaptationSet>
    <AdaptationSet mimeType="audio/mp4" contentType="audio" lang="eng">
      <SegmentTemplate timescale="48000" duration="96000" startNumber="10"
        initialization="$RepresentationID$/init.mp4"
        media="$RepresentationID$/seg-$Number$.m4s"/>
      <Representation id="a1" codecs="mp4a.40.2" bandwidth="128000"/>
    </AdaptationSet>
  </Period>
</MPD>`

func TestParseMPD(t *testing.T) {
	u, _ := url.Parse("https://cdn.example.com/live/manifest.mpd")
	m, err := ParseMPD([]byte(sampleMPD), u)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(m.Video) != 2 || len(m.Audio) != 1 {
		t.Fatalf("tracks = %d video, %d audio", len(m.Video), len(m.Audio))
	}
	best := m.BestVideo()
	if best == nil || best.ID != "v1080" {
		t.Fatal("best video not selected by bandwidth")
	}
	if best.InitURL != "https://cdn.example.com/live/v1080/init.mp4" {
		t.Fatalf("init url = %s", best.InitURL)
	}
	refs := best.TailSegments(2)
	if len(refs) != 2 {
		t.Fatalf("tail segments = %d", len(refs))
	}
	if refs[1].URL != "https://cdn.example.com/live/v1080/seg-12.m4s" {
		t.Fatalf("segment url = %s", refs[1].URL)
	}
	if refs[0].Duration != 2000 {
		t.Fatalf("segment duration = %d", refs[0].Duration)
	}
}

func TestSegmentTimeline(t *testing.T) {
	const mpd = `<?xml version="1.0"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="dynamic" minimumUpdatePeriod="PT1S">
  <Period>
    <AdaptationSet mimeType="video/mp4">
      <SegmentTemplate timescale="1000" initialization="init.mp4" media="seg-$Time$.m4s">
        <SegmentTimeline>
          <S t="1000" d="2000" r="2"/>
        </SegmentTimeline>
      </SegmentTemplate>
      <Representation id="v" codecs="avc1.4d401f" bandwidth="1000"/>
    </AdaptationSet>
  </Period>
</MPD>`
	u, _ := url.Parse("https://cdn.example.com/x/manifest.mpd")
	m, err := ParseMPD([]byte(mpd), u)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	refs := m.Video[0].TailSegments(8)
	if len(refs) != 3 {
		t.Fatalf("timeline segments = %d, want 3", len(refs))
	}
	if refs[0].Time != 1000 || refs[2].Time != 5000 {
		t.Fatalf("timeline times = %d..%d", refs[0].Time, refs[2].Time)
	}
	if refs[2].URL != "https://cdn.example.com/x/seg-5000.m4s" {
		t.Fatalf("segment url = %s", refs[2].URL)
	}
}
