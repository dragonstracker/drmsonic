package manifest

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

type Key struct {
	Method string
	URI    string
	IV     string
}

type HLSSegment struct {
	URL           string
	Duration      float64
	Seq           int64
	Key           *Key
	Discontinuity bool
	Gap           bool
}

type Variant struct {
	URL        string
	Bandwidth  int
	Width      int
	Height     int
	Codecs     string
	FrameRate  float64
	AudioGroup string
}

type HLSPlaylist struct {
	Master         bool
	Variants       []Variant
	TargetDuration float64
	MediaSequence  int64
	Segments       []HLSSegment
	MapURL         string
	MapKey         *Key
	EndList        bool
}

func ParseHLS(data []byte, pageURL *url.URL) (*HLSPlaylist, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || !strings.HasPrefix(strings.TrimSpace(lines[0]), "#EXTM3U") {
		return nil, errors.New("hls: not a playlist")
	}

	pl := &HLSPlaylist{}
	var pendingVariant *Variant
	var pendingDur float64
	var pendingKey *Key
	var pendingDisc bool
	var pendingGap bool
	var mapKey *Key
	seq := int64(0)
	seqKnown := false

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		switch {
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			pl.Master = true
			pendingVariant = parseStreamInf(strings.TrimPrefix(line, "#EXT-X-STREAM-INF:"))

		case strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"):
			v, err := strconv.ParseInt(strings.TrimPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"), 10, 64)
			if err == nil {
				pl.MediaSequence = v
				seq = v
				seqKnown = true
			}

		case strings.HasPrefix(line, "#EXT-X-TARGETDURATION:"):
			v, err := strconv.ParseFloat(strings.TrimPrefix(line, "#EXT-X-TARGETDURATION:"), 64)
			if err == nil {
				pl.TargetDuration = v
			}

		case strings.HasPrefix(line, "#EXTINF:"):
			v := strings.TrimPrefix(line, "#EXTINF:")
			if idx := strings.Index(v, ","); idx >= 0 {
				v = v[:idx]
			}
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				pendingDur = f
			}

		case strings.HasPrefix(line, "#EXT-X-KEY:"):
			k := parseKeyAttrs(strings.TrimPrefix(line, "#EXT-X-KEY:"))
			if k.Method == "NONE" {
				pendingKey = nil
				mapKey = nil
			} else if k.Method != "" {
				pendingKey = &k
			}

		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			attrs := parseAttrs(strings.TrimPrefix(line, "#EXT-X-MAP:"))
			if u := attrs["URI"]; u != "" {
				pl.MapURL = resolveURL(pageURL, u).String()
			}
			if mapKey != nil {
				pl.MapKey = mapKey
				mapKey = nil
			}

		case strings.HasPrefix(line, "#EXT-X-DISCONTINUITY"):
			pendingDisc = true

		case strings.HasPrefix(line, "#EXT-X-GAP"):
			pendingGap = true

		case strings.HasPrefix(line, "#EXT-X-ENDLIST"):
			pl.EndList = true

		case strings.HasPrefix(line, "#"):
			continue

		default:
			if pendingVariant != nil {
				pendingVariant.URL = resolveURL(pageURL, line).String()
				pl.Variants = append(pl.Variants, *pendingVariant)
				pendingVariant = nil
				continue
			}
			if pendingDur > 0 {
				seg := HLSSegment{
					URL:           resolveURL(pageURL, line).String(),
					Duration:      pendingDur,
					Seq:           seq,
					Key:           pendingKey,
					Discontinuity: pendingDisc,
					Gap:           pendingGap,
				}
				pl.Segments = append(pl.Segments, seg)
				seq++
				pendingDur = 0
				pendingDisc = false
				pendingGap = false
			}
		}
	}

	_ = seqKnown
	return pl, nil
}

func (p *HLSPlaylist) BestVariant() *Variant {
	if len(p.Variants) == 0 {
		return nil
	}
	best := &p.Variants[0]
	for i := range p.Variants {
		if p.Variants[i].Bandwidth > best.Bandwidth {
			best = &p.Variants[i]
		}
	}
	return best
}

func parseStreamInf(v string) *Variant {
	attrs := parseAttrs(v)
	va := &Variant{
		Bandwidth:  atoi(attrs["BANDWIDTH"]),
		Codecs:     strings.Trim(attrs["CODECS"], `"`),
		AudioGroup: attrs["AUDIO"],
	}
	if rate := attrs["FRAME-RATE"]; rate != "" {
		if f, err := strconv.ParseFloat(rate, 64); err == nil {
			va.FrameRate = f
		}
	}
	res := attrs["RESOLUTION"]
	if idx := strings.Index(res, "x"); idx > 0 {
		va.Width = atoi(res[:idx])
		va.Height = atoi(res[idx+1:])
	}
	return va
}

func parseKeyAttrs(v string) Key {
	attrs := parseAttrs(v)
	return Key{
		Method: strings.Trim(attrs["METHOD"], `"`),
		URI:    strings.Trim(attrs["URI"], `"`),
		IV:     strings.Trim(attrs["IV"], `"`),
	}
}

func parseAttrs(v string) map[string]string {
	out := make(map[string]string, 8)
	for _, field := range splitAttrs(v) {
		idx := strings.Index(field, "=")
		if idx <= 0 {
			continue
		}
		out[strings.TrimSpace(field[:idx])] = strings.Trim(strings.TrimSpace(field[idx+1:]), `"`)
	}
	return out
}

func splitAttrs(v string) []string {
	var out []string
	start := 0
	inQuote := false
	for i := 0; i < len(v); i++ {
		switch v[i] {
		case '"':
			inQuote = !inQuote
		case ',':
			if !inQuote {
				out = append(out, v[start:i])
				start = i + 1
			}
		}
	}
	if start < len(v) {
		out = append(out, v[start:])
	}
	return out
}

func atoi(s string) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return v
}

func IVForSequence(seq int64) string {
	buf := make([]byte, 16)
	binary.BigEndian.PutUint64(buf[8:], uint64(seq))
	return hex.EncodeToString(buf)
}
