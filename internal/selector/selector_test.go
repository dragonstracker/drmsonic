package selector

import (
	"testing"

	"drmsonic/internal/manifest"
)

func tracks() []manifest.Track {
	return []manifest.Track{
		{ID: "1", Height: 1080, Bandwidth: 6000000, Codecs: "avc1.640028", Lang: "en"},
		{ID: "2", Height: 720, Bandwidth: 3000000, Codecs: "avc1.64001f", Lang: "en"},
		{ID: "3", Height: 480, Bandwidth: 1200000, Codecs: "avc1.4d401e", Lang: "en"},
	}
}

func TestBestWorstAll(t *testing.T) {
	v := tracks()
	p := Select(v, nil, nil, "best", "none", "")
	if len(p.Video) != 1 || p.Video[0].ID != "1" {
		t.Fatalf("best: %+v", p.Video)
	}
	p = Select(v, nil, nil, "worst", "none", "")
	if len(p.Video) != 1 || p.Video[0].ID != "3" {
		t.Fatalf("worst: %+v", p.Video)
	}
	p = Select(v, nil, nil, "all", "none", "")
	if len(p.Video) != 3 {
		t.Fatalf("all: %+v", p.Video)
	}
}

func TestIDsAndUnknown(t *testing.T) {
	v := tracks()
	p := Select(v, nil, nil, "2,nope", "none", "")
	if len(p.Video) != 1 || p.Video[0].ID != "2" {
		t.Fatalf("ids: %+v", p.Video)
	}
	if len(p.Notes) == 0 {
		t.Fatal("expected unknown id note")
	}
}

func TestFilters(t *testing.T) {
	v := tracks()
	p := Select(v, nil, nil, "h<=720,codec=avc1", "none", "")
	if len(p.Video) != 2 {
		t.Fatalf("filter: %+v notes=%v", p.Video, p.Notes)
	}
}
