package web

import (
	"testing"

	"drmsonic/internal/config"
)

func TestMergeScriptStreamsRespectsIgnoreUpdate(t *testing.T) {
	existing := []config.Stream{
		{Id: "keep", Name: "Keep", IsEvent: true, IgnoreUpdate: true, Manifest: "old"},
		{Id: "upd", Name: "Upd", IsEvent: true, Manifest: "old"},
		{Id: "live", Name: "Live", IsEvent: false},
	}
	incoming := []config.Stream{
		{Id: "keep", Name: "Keep New", Manifest: "new"},
		{Id: "upd", Name: "Upd New", Manifest: "new"},
		{Id: "fresh", Name: "Fresh", Manifest: "x"},
	}
	got := mergeScriptStreams(existing, incoming, true, true)
	byID := map[string]config.Stream{}
	for _, st := range got {
		byID[st.Id] = st
	}
	if byID["keep"].Manifest != "old" {
		t.Fatalf("ignoreupdate overwritten")
	}
	if byID["upd"].Manifest != "new" {
		t.Fatalf("update missing")
	}
	if byID["fresh"].Name != "Fresh" || !byID["fresh"].IsEvent {
		t.Fatalf("fresh missing")
	}
	if byID["live"].Name != "Live" {
		t.Fatalf("non-event lost")
	}
}

func TestSyncSummaryNoChange(t *testing.T) {
	rows := []config.Stream{{Id: "a", Name: "A"}}
	if got := syncSummary("events", rows, rows); got != "No change in events" {
		t.Fatalf("got %q", got)
	}
}

func TestMergeScriptStreamsKeepsMediaOnBlankIncoming(t *testing.T) {
	existing := []config.Stream{{
		Id:              "ev1",
		Name:            "Match",
		IsEvent:         true,
		Manifest:        "https://cdn.example/a.mpd",
		ManifestScript:  "id=ev1",
		Cdm:             "id=ev1",
		Keys:            []string{"kid:key"},
		UseCdm:          true,
		SessionManifest: true,
		Autostart:       true,
		Video:           "max",
		Audio:           "lang=en",
	}}
	incoming := []config.Stream{{
		Id:   "ev1",
		Name: "Match",
	}}
	got := mergeScriptStreams(existing, incoming, true, true)
	byID := map[string]config.Stream{}
	for _, st := range got {
		byID[st.Id] = st
	}
	st := byID["ev1"]
	if st.Manifest != "https://cdn.example/a.mpd" {
		t.Fatalf("manifest lost: %q", st.Manifest)
	}
	if len(st.Keys) != 1 || st.Keys[0] != "kid:key" {
		t.Fatalf("keys lost: %#v", st.Keys)
	}
	if !st.UseCdm || !st.SessionManifest || !st.Autostart {
		t.Fatalf("flags cleared: usecdm=%v session=%v autostart=%v", st.UseCdm, st.SessionManifest, st.Autostart)
	}
	if st.Video != "max" || st.Audio != "lang=en" {
		t.Fatalf("tracks cleared: video=%q audio=%q", st.Video, st.Audio)
	}
}

func TestMergeScriptStreamsAllowsMediaUpgrade(t *testing.T) {
	existing := []config.Stream{{
		Id:       "ev1",
		IsEvent:  true,
		Manifest: "https://cdn.example/old.mpd",
		Keys:     []string{"old:key"},
	}}
	incoming := []config.Stream{{
		Id:       "ev1",
		Manifest: "https://cdn.example/new.mpd",
		Keys:     []string{"new:key"},
		UseCdm:   true,
	}}
	got := mergeScriptStreams(existing, incoming, true, true)
	st := got[0]
	if st.Manifest != "https://cdn.example/new.mpd" {
		t.Fatalf("manifest not upgraded: %q", st.Manifest)
	}
	if len(st.Keys) != 1 || st.Keys[0] != "new:key" {
		t.Fatalf("keys not upgraded: %#v", st.Keys)
	}
}
