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
