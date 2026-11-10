package pipeline

import (
	"testing"

	"drmsonic/internal/manifest"
)

func TestSegmentSecondsFromTemplateDuration(t *testing.T) {
	s := &streamState{track: &manifest.Track{
		Timescale: 32000,
		Template:  &manifest.SegmentTemplate{Duration: 64000},
	}}
	if got := segmentSeconds(s, nil); got != 2 {
		t.Fatalf("segmentSeconds = %v, want 2", got)
	}
}

func TestSegmentSecondsFromTimeline(t *testing.T) {
	s := &streamState{track: &manifest.Track{
		Timescale: 32000,
		Template: &manifest.SegmentTemplate{
			Timeline: &manifest.SegmentTimeline{Items: []manifest.TimelineItem{
				{T: 0, D: 63488},
				{T: 63488, D: 64512},
			}},
		},
	}}
	got := segmentSeconds(s, nil)
	want := 64512.0 / 32000
	if got != want {
		t.Fatalf("segmentSeconds = %v, want %v", got, want)
	}
}

func TestSegmentSecondsPrefersVideo(t *testing.T) {
	v := &streamState{track: &manifest.Track{
		Timescale: 32000,
		Template:  &manifest.SegmentTemplate{Duration: 64000},
	}}
	a := &streamState{track: &manifest.Track{
		Timescale: 48000,
		Template:  &manifest.SegmentTemplate{Duration: 48000},
	}}
	if got := segmentSeconds(nil, a); got != 1 {
		t.Fatalf("audio-only = %v, want 1", got)
	}
	if got := segmentSeconds(v, a); got != 2 {
		t.Fatalf("video-first = %v, want 2", got)
	}
}

func TestSegmentSecondsUnknown(t *testing.T) {
	if got := segmentSeconds(nil, nil); got != 0 {
		t.Fatalf("segmentSeconds = %v, want 0", got)
	}
	s := &streamState{track: &manifest.Track{Template: &manifest.SegmentTemplate{}}}
	if got := segmentSeconds(s, nil); got != 0 {
		t.Fatalf("zero timescale = %v, want 0", got)
	}
}
