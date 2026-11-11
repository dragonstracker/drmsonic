package web

import (
	"strings"
	"testing"
)

func TestStreamDomIdHelpers(t *testing.T) {
	cases := []struct {
		provider string
		stream   string
	}{
		{"channels", "tnt2fhd"},
		{"channelmultiple", "tnt_sports_2"},
		{"my_provider", "stream_with_underscores_1"},
	}
	for _, c := range cases {
		id := "_" + c.provider + "~" + c.stream
		if !strings.Contains(id, "~") {
			t.Fatalf("missing separator in %s", id)
		}
		if got := strings.SplitN(id[1:], "~", 2); got[0] != c.provider || got[1] != c.stream {
			t.Fatalf("split %q = %v", id, got)
		}
	}
}
