package manifest

import (
	"net/url"
	"testing"

	"drmsonic/internal/mp4"
)

func TestSegmentBaseRanges(t *testing.T) {
	const mpd = `<?xml version="1.0"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" mediaPresentationDuration="PT12S">
  <Period>
    <AdaptationSet mimeType="video/mp4">
      <Representation id="v1" codecs="avc1.64001F" bandwidth="3000000" width="1280" height="720">
        <BaseURL>video.mp4</BaseURL>
        <SegmentBase timescale="1000" indexRange="800-1200">
          <Initialization range="0-799"/>
        </SegmentBase>
      </Representation>
    </AdaptationSet>
  </Period>
</MPD>`
	u, _ := url.Parse("https://cdn.example.com/vod/manifest.mpd")
	m, err := ParseMPD([]byte(mpd), u)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(m.Video) != 1 {
		t.Fatalf("video tracks = %d", len(m.Video))
	}
	tr := m.Video[0]
	if tr.FileURL != "https://cdn.example.com/vod/video.mp4" {
		t.Fatalf("file url = %s", tr.FileURL)
	}
	if tr.InitRange != "0-799" || tr.IndexRange != "800-1200" {
		t.Fatalf("ranges = %q %q", tr.InitRange, tr.IndexRange)
	}

	sidx := &mp4.Sidx{
		Timescale:   1000,
		FirstOffset: 0,
		End:         1201,
		Refs: []mp4.SidxRef{
			{Size: 1000, Duration: 2000},
			{Size: 2000, Duration: 2000},
		},
	}
	refs := tr.RangeSegments(sidx, 0)
	if len(refs) != 2 {
		t.Fatalf("range segments = %d", len(refs))
	}
	if refs[0].Range != "1201-2200" {
		t.Fatalf("first range = %s", refs[0].Range)
	}
	if refs[1].Range != "2201-4200" {
		t.Fatalf("second range = %s", refs[1].Range)
	}
	if refs[1].Time != 2000 {
		t.Fatalf("second time = %d", refs[1].Time)
	}
}

func TestTimelineRepeatToEnd(t *testing.T) {
	const mpd = `<?xml version="1.0"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" mediaPresentationDuration="PT10S">
  <Period>
    <AdaptationSet mimeType="video/mp4">
      <SegmentTemplate timescale="1000" initialization="init.mp4" media="seg-$Time$.m4s">
        <SegmentTimeline>
          <S t="0" d="2000" r="-1"/>
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
	refs := m.Video[0].TailSegments(0)
	if len(refs) != 5 {
		t.Fatalf("repeat segments = %d, want 5", len(refs))
	}
	if refs[4].Time != 8000 {
		t.Fatalf("last time = %d", refs[4].Time)
	}
}

func TestMultiPeriodFirstPlayable(t *testing.T) {
	const mpd = `<?xml version="1.0"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" mediaPresentationDuration="PT8S">
  <Period duration="PT4S">
    <AdaptationSet mimeType="video/mp4">
      <SegmentTemplate timescale="1000" duration="2000" initialization="init.mp4" media="p1-$Number$.m4s"/>
      <Representation id="v1" codecs="avc1.4d401f" bandwidth="1000"/>
    </AdaptationSet>
  </Period>
  <Period duration="PT4S">
    <AdaptationSet mimeType="video/mp4">
      <SegmentTemplate timescale="1000" duration="2000" initialization="init.mp4" media="p2-$Number$.m4s"/>
      <Representation id="v2" codecs="avc1.4d401f" bandwidth="2000"/>
    </AdaptationSet>
  </Period>
</MPD>`
	u, _ := url.Parse("https://cdn.example.com/x/manifest.mpd")
	m, err := ParseMPD([]byte(mpd), u)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(m.Video) != 1 || m.Video[0].ID != "v1" {
		t.Fatalf("tracks = %+v", m.Video)
	}
}
