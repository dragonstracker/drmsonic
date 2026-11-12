package selector

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"drmsonic/internal/manifest"
)

type Plan struct {
	Video []manifest.Track
	Audio []manifest.Track
	Text  []manifest.Track
	Notes []string
}

func Select(video, audio, text []manifest.Track, videoSel, audioSel, textSel string) Plan {
	p := Plan{}
	p.Video, p.Notes = pick(video, videoSel, true, p.Notes)
	p.Audio, p.Notes = pick(audio, audioSel, false, p.Notes)
	p.Text, p.Notes = pick(text, textSel, false, p.Notes)
	return p
}

func pick(tracks []manifest.Track, sel string, isVideo bool, notes []string) ([]manifest.Track, []string) {
	sel = strings.TrimSpace(sel)
	if sel == "" || sel == "best" || sel == "max" {
		return topN(tracks, 1), notes
	}
	if sel == "none" {
		if isVideo {
			notes = append(notes, "video none is invalid; using best")
			return topN(tracks, 1), notes
		}
		return nil, notes
	}
	if sel == "worst" {
		return bottomN(tracks, 1), notes
	}
	if sel == "all" {
		return dedupeSorted(tracks), notes
	}
	if strings.HasPrefix(sel, "top:") {
		n, _ := strconv.Atoi(strings.TrimPrefix(sel, "top:"))
		if n <= 0 {
			n = 1
		}
		return topN(tracks, n), notes
	}

	tokens := strings.Split(sel, ",")
	var filters []string
	var ids []string
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "" || tok == "best" || tok == "max" || tok == "all" || tok == "worst" || tok == "none" {
			continue
		}
		if _, ok := filterToken(tracks, tok, isVideo); ok {
			filters = append(filters, tok)
			continue
		}
		ids = append(ids, tok)
	}

	cand := tracks
	for _, f := range filters {
		cand, _ = filterToken(cand, f, isVideo)
	}

	out := make([]manifest.Track, 0, len(cand))
	seen := map[string]bool{}
	if len(ids) == 0 {
		for _, t := range cand {
			if !seen[t.ID] {
				seen[t.ID] = true
				out = append(out, t)
			}
		}
	} else {
		for _, id := range ids {
			found := false
			for _, t := range cand {
				if t.ID == id && !seen[t.ID] {
					seen[t.ID] = true
					out = append(out, t)
					found = true
					break
				}
			}
			if !found {
				notes = append(notes, fmt.Sprintf("unknown id %q dropped", id))
			}
		}
	}
	if len(out) == 0 && len(tracks) > 0 {
		notes = append(notes, "empty selection; falling back to best")
		return topN(tracks, 1), notes
	}
	return sortByBandwidth(out), notes
}

func filterToken(tracks []manifest.Track, tok string, isVideo bool) ([]manifest.Track, bool) {
	switch {
	case strings.HasPrefix(tok, "h<="):
		if !isVideo {
			return nil, true
		}
		n, _ := strconv.Atoi(strings.TrimPrefix(tok, "h<="))
		return filterHeight(tracks, func(h int) bool { return h <= n }), true
	case strings.HasPrefix(tok, "h>="):
		if !isVideo {
			return nil, true
		}
		n, _ := strconv.Atoi(strings.TrimPrefix(tok, "h>="))
		return filterHeight(tracks, func(h int) bool { return h >= n }), true
	case strings.HasPrefix(tok, "bw<="):
		n := parseBW(strings.TrimPrefix(tok, "bw<="))
		return filterBW(tracks, func(b int) bool { return b <= n }), true
	case strings.HasPrefix(tok, "bw>="):
		n := parseBW(strings.TrimPrefix(tok, "bw>="))
		return filterBW(tracks, func(b int) bool { return b >= n }), true
	case strings.HasPrefix(tok, "codec="):
		pref := strings.TrimPrefix(tok, "codec=")
		return filterCodec(tracks, pref), true
	case strings.HasPrefix(tok, "lang="):
		if isVideo {
			return nil, true
		}
		langs := strings.Split(strings.TrimPrefix(tok, "lang="), ",")
		return filterLang(tracks, langs), true
	case strings.HasPrefix(tok, "desc:"):
		if isVideo {
			return nil, true
		}
		return filterDesc(tracks, strings.TrimPrefix(tok, "desc:")), true
	case tok == "default":
		if isVideo {
			return nil, true
		}
		for _, t := range tracks {
			if t.Default {
				return []manifest.Track{t}, true
			}
		}
		return topN(tracks, 1), true
	}
	return nil, false
}

func parseBW(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	mult := 1
	if strings.HasSuffix(s, "k") {
		mult = 1000
		s = strings.TrimSuffix(s, "k")
	} else if strings.HasSuffix(s, "m") {
		mult = 1000000
		s = strings.TrimSuffix(s, "m")
	}
	n, _ := strconv.Atoi(s)
	return n * mult
}

func filterHeight(tracks []manifest.Track, ok func(int) bool) []manifest.Track {
	out := make([]manifest.Track, 0, len(tracks))
	for _, t := range tracks {
		if ok(t.Height) {
			out = append(out, t)
		}
	}
	return sortByBandwidth(out)
}

func filterBW(tracks []manifest.Track, ok func(int) bool) []manifest.Track {
	out := make([]manifest.Track, 0, len(tracks))
	for _, t := range tracks {
		if ok(t.Bandwidth) {
			out = append(out, t)
		}
	}
	return sortByBandwidth(out)
}

func filterCodec(tracks []manifest.Track, pref string) []manifest.Track {
	out := make([]manifest.Track, 0, len(tracks))
	for _, t := range tracks {
		if strings.HasPrefix(strings.ToLower(t.Codecs), strings.ToLower(pref)) {
			out = append(out, t)
		}
	}
	return sortByBandwidth(out)
}

func filterLang(tracks []manifest.Track, langs []string) []manifest.Track {
	want := map[string]bool{}
	for _, l := range langs {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == "" {
			continue
		}
		want[l] = true
		if l == "en" {
			want["eng"] = true
		} else if l == "eng" {
			want["en"] = true
		}
	}
	out := make([]manifest.Track, 0, len(tracks))
	for _, t := range tracks {
		if want[strings.ToLower(t.Lang)] {
			out = append(out, t)
		}
	}
	return sortByBandwidth(out)
}

func filterDesc(tracks []manifest.Track, needle string) []manifest.Track {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return sortByBandwidth(tracks)
	}
	langs := []string{needle}
	if needle == "en" {
		langs = append(langs, "eng")
	} else if needle == "eng" {
		langs = append(langs, "en")
	}
	out := make([]manifest.Track, 0, len(tracks))
	for _, t := range tracks {
		lang := strings.ToLower(t.Lang)
		id := strings.ToLower(t.ID)
		for _, a := range langs {
			if lang == a || strings.Contains(id, a) {
				out = append(out, t)
				break
			}
		}
	}
	return sortByBandwidth(out)
}

func topN(tracks []manifest.Track, n int) []manifest.Track {
	s := sortByBandwidth(tracks)
	if n > len(s) {
		n = len(s)
	}
	return append([]manifest.Track{}, s[:n]...)
}

func bottomN(tracks []manifest.Track, n int) []manifest.Track {
	s := sortByBandwidth(tracks)
	if len(s) == 0 {
		return nil
	}
	if n > len(s) {
		n = len(s)
	}
	return append([]manifest.Track{}, s[len(s)-n:]...)
}

func sortByBandwidth(tracks []manifest.Track) []manifest.Track {
	out := append([]manifest.Track{}, tracks...)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Bandwidth > out[j].Bandwidth
	})
	return out
}

func dedupeSorted(tracks []manifest.Track) []manifest.Track {
	s := sortByBandwidth(tracks)
	seen := map[string]bool{}
	out := make([]manifest.Track, 0, len(s))
	for _, t := range s {
		if seen[t.ID] {
			continue
		}
		seen[t.ID] = true
		out = append(out, t)
	}
	return out
}
