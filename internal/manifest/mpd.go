package manifest

import (
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"drmsonic/internal/mp4"
)

type MPD struct {
	Type                      string   `xml:"type,attr"`
	MinimumUpdatePeriod       string   `xml:"minimumUpdatePeriod,attr"`
	AvailabilityStartTime     string   `xml:"availabilityStartTime,attr"`
	TimeShiftBufferDepth      string   `xml:"timeShiftBufferDepth,attr"`
	MediaPresentationDuration string   `xml:"mediaPresentationDuration,attr"`
	BaseURL                   []string `xml:"BaseURL"`
	Periods                   []Period `xml:"Period"`
}

type Period struct {
	Start          string          `xml:"start,attr"`
	Duration       string          `xml:"duration,attr"`
	BaseURL        []string        `xml:"BaseURL"`
	AdaptationSets []AdaptationSet `xml:"AdaptationSet"`
}

type AdaptationSet struct {
	ID              string              `xml:"id,attr"`
	MimeType        string              `xml:"mimeType,attr"`
	ContentType     string              `xml:"contentType,attr"`
	Lang            string              `xml:"lang,attr"`
	Label           string              `xml:"Label"`
	BaseURL         []string            `xml:"BaseURL"`
	Template        *SegmentTemplate    `xml:"SegmentTemplate"`
	List            *SegmentList        `xml:"SegmentList"`
	Base            *SegmentBase        `xml:"SegmentBase"`
	Protection      []ContentProtection `xml:"ContentProtection"`
	Representations []Representation    `xml:"Representation"`
}

type Representation struct {
	ID                string              `xml:"id,attr"`
	Bandwidth         int                 `xml:"bandwidth,attr"`
	Width             int                 `xml:"width,attr"`
	Height            int                 `xml:"height,attr"`
	FrameRate         string              `xml:"frameRate,attr"`
	Codecs            string              `xml:"codecs,attr"`
	MimeType          string              `xml:"mimeType,attr"`
	AudioSamplingRate int                 `xml:"audioSamplingRate,attr"`
	BaseURL           []string            `xml:"BaseURL"`
	Template          *SegmentTemplate    `xml:"SegmentTemplate"`
	List              *SegmentList        `xml:"SegmentList"`
	Base              *SegmentBase        `xml:"SegmentBase"`
	Protection        []ContentProtection `xml:"ContentProtection"`
}

type SegmentTemplate struct {
	Timescale              int64            `xml:"timescale,attr"`
	Duration               int64            `xml:"duration,attr"`
	StartNumber            int64            `xml:"startNumber,attr"`
	Media                  string           `xml:"media,attr"`
	Initialization         string           `xml:"initialization,attr"`
	PresentationTimeOffset int64            `xml:"presentationTimeOffset,attr"`
	Timeline               *SegmentTimeline `xml:"SegmentTimeline"`
}

type SegmentTimeline struct {
	Items []TimelineItem `xml:"S"`
}

type TimelineItem struct {
	T int64 `xml:"t,attr"`
	D int64 `xml:"d,attr"`
	R int   `xml:"r,attr"`
}

type SegmentList struct {
	Duration       int64            `xml:"duration,attr"`
	Timescale      int64            `xml:"timescale,attr"`
	Initialization *SegmentURL      `xml:"Initialization"`
	SegmentURLs    []SegmentURL     `xml:"SegmentURL"`
	Timeline       *SegmentTimeline `xml:"SegmentTimeline"`
}

type SegmentURL struct {
	Media      string `xml:"media,attr"`
	MediaRange string `xml:"mediaRange,attr"`
	Range      string `xml:"range,attr"`
}

type SegmentBase struct {
	Timescale              int64       `xml:"timescale,attr"`
	PresentationTimeOffset int64       `xml:"presentationTimeOffset,attr"`
	IndexRange             string      `xml:"indexRange,attr"`
	Initialization         *SegmentURL `xml:"Initialization"`
}

type ContentProtection struct {
	SchemeIDURI string `xml:"schemeIdUri,attr"`
	Value       string `xml:"value,attr"`
	DefaultKID  string `xml:"default_KID,attr"`
	Pssh        string `xml:"pssh"`
}

type Track struct {
	Kind        string
	ID          string
	Codecs      string
	Width       int
	Height      int
	FrameRate   string
	Bandwidth   int
	Lang        string
	Default     bool
	Rate        int
	Channels    int
	Timescale   int64
	InitURL     string
	Template    *SegmentTemplate
	List        *SegmentList
	Base        *url.URL
	SegBase     *SegmentBase
	FileURL     string
	InitRange   string
	IndexRange  string
	PTO         int64
	KID         string
	StartNumber int64
	StartedAt   time.Time
	Duration    time.Duration
}

type Manifest struct {
	URL          *url.URL
	Type         string
	UpdatePeriod time.Duration
	StartedAt    time.Time
	BufferDepth  time.Duration
	Static       bool
	Duration     time.Duration
	Video        []Track
	Audio        []Track
	Text         []Track
	All          []Track
}

func ParseMPD(data []byte, pageURL *url.URL) (*Manifest, error) {
	var mpd MPD
	if err := xml.Unmarshal(data, &mpd); err != nil {
		return nil, err
	}
	if len(mpd.Periods) == 0 {
		return nil, errors.New("dash: no periods")
	}

	m := &Manifest{
		URL:       pageURL,
		Type:      mpd.Type,
		Static:    mpd.Type == "static",
		StartedAt: parseTime(mpd.AvailabilityStartTime),
	}
	m.UpdatePeriod = parseDuration(mpd.MinimumUpdatePeriod)
	if m.UpdatePeriod == 0 {
		m.UpdatePeriod = 5 * time.Second
	}
	m.BufferDepth = parseDuration(mpd.TimeShiftBufferDepth)
	m.Duration = parseDuration(mpd.MediaPresentationDuration)

	root := resolveURL(pageURL, firstOf(mpd.BaseURL))

	for _, p := range mpd.Periods {
		pBase := resolveURL(root, firstOf(p.BaseURL))
		periodDuration := parseDuration(p.Duration)
		if periodDuration == 0 {
			periodDuration = m.Duration
		}

		startVideo, startAudio, startAll := len(m.Video), len(m.Audio), len(m.All)

		for _, as := range p.AdaptationSets {
			asBase := resolveURL(pBase, firstOf(as.BaseURL))
			kind := adaptationKind(as)
			for _, rep := range as.Representations {
				repBase := resolveURL(asBase, firstOf(rep.BaseURL))

				tmpl := mergeTemplate(as.Template, rep.Template)
				list := mergeList(as.List, rep.List)
				base := mergeBase(as.Base, rep.Base)

				tr := Track{
					Kind:      kind,
					ID:        rep.ID,
					Codecs:    rep.Codecs,
					Width:     rep.Width,
					Height:    rep.Height,
					FrameRate: rep.FrameRate,
					Bandwidth: rep.Bandwidth,
					Lang:      as.Lang,
					Rate:      rep.AudioSamplingRate,
					Base:      repBase,
					Timescale: 1000,
					StartedAt: m.StartedAt,
					Duration:  periodDuration,
				}
				if kind == "text" {
					continue
				}

				if tmpl == nil && list == nil && base == nil {
					continue
				}
				tr.Template = tmpl
				tr.List = list
				tr.SegBase = base

				timescale := int64(1000)
				pto := int64(0)
				initPattern := ""
				if tmpl != nil {
					if tmpl.Timescale > 0 {
						timescale = tmpl.Timescale
					}
					pto = tmpl.PresentationTimeOffset
					initPattern = tmpl.Initialization
					tr.StartNumber = tmpl.StartNumber
					if tr.StartNumber == 0 {
						tr.StartNumber = 1
					}
				} else if list != nil {
					if list.Timescale > 0 {
						timescale = list.Timescale
					}
					if list.Initialization != nil {
						initPattern = list.Initialization.Media
					}
				} else if base != nil {
					if base.Timescale > 0 {
						timescale = base.Timescale
					}
					pto = base.PresentationTimeOffset
					tr.FileURL = repBase.String()
					tr.IndexRange = strings.TrimSpace(base.IndexRange)
					if base.Initialization != nil && base.Initialization.Range != "" {
						tr.InitRange = base.Initialization.Range
					} else if tr.IndexRange != "" {
						tr.InitRange = initRangeBefore(tr.IndexRange)
					}
				}
				tr.Timescale = timescale
				tr.PTO = pto

				if initPattern != "" {
					tr.InitURL = resolveURL(repBase, expandTemplate(initPattern, rep.ID, rep.Bandwidth, tr.StartNumber, 0)).String()
				}

				if rep.Width > 0 {
					tr.FrameRate = normalizeFrameRate(tr.FrameRate)
				}

				protection := rep.Protection
				if len(protection) == 0 {
					protection = as.Protection
				}
				for _, cp := range protection {
					if cp.DefaultKID != "" {
						tr.KID = normaliseKID(cp.DefaultKID)
					}
				}

				switch kind {
				case "video":
					m.Video = append(m.Video, tr)
				case "audio":
					m.Audio = append(m.Audio, tr)
				}
				m.All = append(m.All, tr)
			}
		}

		if len(m.Video) == startVideo && len(m.Audio) == startAudio {
			continue
		}
		if m.Static {
			break
		}
		m.Video, m.Audio, m.All = m.Video[startVideo:], m.Audio[startAudio:], m.All[startAll:]
	}
	return m, nil
}

func (m *Manifest) BestVideo() *Track {
	if len(m.Video) == 0 {
		return nil
	}
	best := &m.Video[0]
	for i := range m.Video {
		if m.Video[i].Bandwidth > best.Bandwidth {
			best = &m.Video[i]
		}
	}
	return best
}

func (m *Manifest) BestAudio(pref string) *Track {
	if len(m.Audio) == 0 {
		return nil
	}
	if pref != "" && pref != "all" && pref != "none" {
		for i := range m.Audio {
			if m.Audio[i].ID == pref {
				return &m.Audio[i]
			}
		}
	}
	for i := range m.Audio {
		if m.Audio[i].Lang == "eng" {
			return &m.Audio[i]
		}
	}
	return &m.Audio[0]
}

type SegmentRef struct {
	Number   int64
	Time     int64
	Duration int64
	URL      string
	Range    string
}

func (t *Track) SegmentCount() int {
	if t.Template == nil {
		return 0
	}
	if t.Template.Timeline != nil {
		total := 0
		for _, it := range t.Template.Timeline.Items {
			total += it.R + 1
		}
		return total
	}
	return 0
}

func (t *Track) TailSegments(tail int) []SegmentRef {
	if t.Template != nil {
		return t.tailTemplate(tail)
	}
	if t.List != nil {
		return t.tailList(tail)
	}
	return nil
}

func (t *Track) tailTemplate(tail int) []SegmentRef {
	tmpl := t.Template
	refs := make([]SegmentRef, 0, tail)

	if tmpl.Timeline == nil || len(tmpl.Timeline.Items) == 0 {
		return t.tailDuration(tail)
	}

	type entry struct {
		time     int64
		duration int64
	}
	entries := make([]entry, 0, 64)

	var cursor int64
	for _, it := range tmpl.Timeline.Items {
		start := it.T
		if start == 0 {
			start = cursor
		}
		count := it.R + 1
		if it.R < 0 {
			count = 1
			if it.D > 0 && !t.DurationZero() {
				total := t.EndTicks()
				if total > start {
					count = int((total - start) / it.D)
				}
			}
			if count < 1 {
				count = 1
			}
		}
		for i := 0; i < count; i++ {
			entries = append(entries, entry{time: start + int64(i)*it.D, duration: it.D})
		}
		cursor = start + int64(count)*it.D
	}

	from := 0
	if tail > 0 {
		from = len(entries) - tail
		if from < 0 {
			from = 0
		}
	}
	for i := from; i < len(entries); i++ {
		e := entries[i]
		number := tmpl.StartNumber + int64(i)
		u := expandTemplate(tmpl.Media, t.ID, t.Bandwidth, number, e.time)
		refs = append(refs, SegmentRef{
			Number:   number,
			Time:     e.time,
			Duration: e.duration,
			URL:      resolveURL(t.Base, u).String(),
		})
	}
	return refs
}

func (t *Track) tailDuration(tail int) []SegmentRef {
	tmpl := t.Template
	if tmpl == nil || tmpl.Duration <= 0 || tmpl.Media == "" {
		return nil
	}
	ts := t.Timescale
	if ts <= 0 {
		ts = 1000
	}
	segTicks := tmpl.Duration
	segSeconds := float64(segTicks) / float64(ts)
	if segSeconds <= 0 {
		return nil
	}

	var last int64
	if t.StartedAt.IsZero() {
		if t.Duration <= 0 {
			return nil
		}
		total := int64(t.Duration.Seconds() / segSeconds)
		if total <= 0 {
			return nil
		}
		last = total - 1
	} else {
		elapsed := time.Since(t.StartedAt).Seconds()
		if elapsed < 0 {
			return nil
		}
		last = int64(elapsed/segSeconds) - 1
		if last < 0 {
			last = 0
		}
	}

	first := last - int64(tail) + 1
	if tail <= 0 || first < 0 {
		first = 0
	}
	refs := make([]SegmentRef, 0, last-first+1)
	for i := first; i <= last; i++ {
		number := tmpl.StartNumber + i
		if number == 0 {
			number = i + 1
		}
		u := expandTemplate(tmpl.Media, t.ID, t.Bandwidth, number, i*segTicks)
		refs = append(refs, SegmentRef{
			Number:   number,
			Time:     i * segTicks,
			Duration: segTicks,
			URL:      resolveURL(t.Base, u).String(),
		})
	}
	return refs
}

func mergeBase(parent, child *SegmentBase) *SegmentBase {
	if parent == nil {
		return child
	}
	if child == nil {
		out := *parent
		return &out
	}
	out := *parent
	if child.Timescale > 0 {
		out.Timescale = child.Timescale
	}
	if child.PresentationTimeOffset != 0 {
		out.PresentationTimeOffset = child.PresentationTimeOffset
	}
	if child.IndexRange != "" {
		out.IndexRange = child.IndexRange
	}
	if child.Initialization != nil {
		out.Initialization = child.Initialization
	}
	return &out
}

func mergeTemplate(parent, child *SegmentTemplate) *SegmentTemplate {
	if parent == nil {
		return child
	}
	if child == nil {
		out := *parent
		return &out
	}
	out := *parent
	if child.Timescale > 0 {
		out.Timescale = child.Timescale
	}
	if child.Duration > 0 {
		out.Duration = child.Duration
	}
	if child.StartNumber > 0 {
		out.StartNumber = child.StartNumber
	}
	if child.Media != "" {
		out.Media = child.Media
	}
	if child.Initialization != "" {
		out.Initialization = child.Initialization
	}
	if child.PresentationTimeOffset != 0 {
		out.PresentationTimeOffset = child.PresentationTimeOffset
	}
	if child.Timeline != nil && len(child.Timeline.Items) > 0 {
		out.Timeline = child.Timeline
	}
	return &out
}

func mergeList(parent, child *SegmentList) *SegmentList {
	if parent == nil {
		return child
	}
	if child == nil {
		out := *parent
		return &out
	}
	out := *parent
	if child.Duration > 0 {
		out.Duration = child.Duration
	}
	if child.Timescale > 0 {
		out.Timescale = child.Timescale
	}
	if child.Initialization != nil {
		out.Initialization = child.Initialization
	}
	if len(child.SegmentURLs) > 0 {
		out.SegmentURLs = child.SegmentURLs
	}
	if child.Timeline != nil {
		out.Timeline = child.Timeline
	}
	return &out
}

func (t *Track) tailList(tail int) []SegmentRef {
	list := t.List
	refs := make([]SegmentRef, 0, len(list.SegmentURLs))
	start := 0
	if tail > 0 && len(list.SegmentURLs) > tail {
		start = len(list.SegmentURLs) - tail
	}
	cursor := int64(0)
	ts := list.Timescale
	if ts == 0 {
		ts = t.Timescale
	}
	for i := start; i < len(list.SegmentURLs); i++ {
		u := list.SegmentURLs[i].Media
		refs = append(refs, SegmentRef{
			Number:   int64(i + 1),
			Time:     cursor,
			Duration: list.Duration,
			URL:      resolveURL(t.Base, u).String(),
			Range:    list.SegmentURLs[i].Range,
		})
		cursor += list.Duration
	}
	return refs
}

func (t *Track) DurationZero() bool {
	return t.Duration <= 0
}

func (t *Track) EndTicks() int64 {
	if t.Duration <= 0 {
		return 0
	}
	scale := t.Timescale
	if scale <= 0 {
		scale = 1000
	}
	return int64(t.Duration.Seconds()*float64(scale)) + t.PTO
}

func (t *Track) RangeSegments(sidx *mp4.Sidx, tail int) []SegmentRef {
	if sidx == nil || len(sidx.Refs) == 0 {
		return nil
	}
	total := int64(len(sidx.Refs))
	from := int64(0)
	if tail > 0 && total > int64(tail) {
		from = total - int64(tail)
	}
	refs := make([]SegmentRef, 0, total-from)
	cursor := sidx.End + sidx.FirstOffset
	ticks := sidx.Earliest
	for i := int64(0); i < total; i++ {
		ref := sidx.Refs[i]
		if i >= from {
			refs = append(refs, SegmentRef{
				Number:   i + 1,
				Time:     ticks,
				Duration: ref.Duration,
				URL:      t.FileURL,
				Range:    byteRange(cursor, ref.Size),
			})
		}
		cursor += ref.Size
		ticks += ref.Duration
	}
	return refs
}

func byteRange(start, size int64) string {
	if size <= 0 {
		return ""
	}
	return strconv.FormatInt(start, 10) + "-" + strconv.FormatInt(start+size-1, 10)
}

func initRangeBefore(indexRange string) string {
	parts := strings.SplitN(indexRange, "-", 2)
	if len(parts) == 0 {
		return ""
	}
	start, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || start <= 0 {
		return ""
	}
	return byteRange(0, start)
}

func (t *Track) SegmentDurationSeconds() float64 {
	if t.Template != nil && t.Template.Timeline != nil && len(t.Template.Timeline.Items) > 0 {
		d := t.Template.Timeline.Items[0].D
		if t.Timescale > 0 {
			return float64(d) / float64(t.Timescale)
		}
	}
	if t.Template != nil && t.Template.Duration > 0 && t.Timescale > 0 {
		return float64(t.Template.Duration) / float64(t.Timescale)
	}
	if t.List != nil && t.List.Duration > 0 && t.Timescale > 0 {
		return float64(t.List.Duration) / float64(t.Timescale)
	}
	return 2
}

func expandTemplate(pattern, repID string, bandwidth int, number, timeTicks int64) string {
	if pattern == "" {
		return ""
	}
	out := pattern
	out = strings.ReplaceAll(out, "$RepresentationID$", repID)

	out = replaceFormatted(out, "Bandwidth", int64(bandwidth))
	out = replaceFormatted(out, "Number", number)
	out = replaceFormatted(out, "Time", timeTicks)

	return out
}

func replaceFormatted(s, key string, value int64) string {
	plain := "$" + key + "$"
	if strings.Contains(s, plain) {
		s = strings.ReplaceAll(s, plain, strconv.FormatInt(value, 10))
	}
	open := "$" + key + "%0"
	for {
		idx := strings.Index(s, open)
		if idx < 0 {
			break
		}
		rest := s[idx+len(open):]
		end := strings.Index(rest, "d$")
		if end <= 0 {
			break
		}
		width, err := strconv.Atoi(rest[:end])
		if err != nil || width <= 0 || width > 20 {
			break
		}
		formatted := fmt.Sprintf("%0*d", width, value)
		s = s[:idx] + formatted + rest[end+2:]
	}
	return s
}

func adaptationKind(as AdaptationSet) string {
	ct := strings.ToLower(as.ContentType)
	if ct != "" {
		return ct
	}
	mt := strings.ToLower(as.MimeType)
	switch {
	case strings.HasPrefix(mt, "video"):
		return "video"
	case strings.HasPrefix(mt, "audio"):
		return "audio"
	case strings.HasPrefix(mt, "text"), strings.Contains(mt, "ttml"):
		return "text"
	}
	if len(as.Representations) > 0 {
		codec := strings.ToLower(as.Representations[0].Codecs)
		if strings.HasPrefix(codec, "mp4a") || strings.HasPrefix(codec, "ac-3") || strings.HasPrefix(codec, "ec-3") {
			return "audio"
		}
		if strings.HasPrefix(codec, "avc") || strings.HasPrefix(codec, "hvc") || strings.HasPrefix(codec, "hev") {
			return "video"
		}
	}
	return "other"
}

func resolveURL(base *url.URL, ref string) *url.URL {
	if ref == "" {
		return base
	}
	u, err := url.Parse(ref)
	if err != nil {
		return base
	}
	if base == nil {
		return u
	}
	return base.ResolveReference(u)
}

func firstOf(list []string) string {
	for _, s := range list {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

var durationRe = strings.NewReplacer("PT", "", "H", "h", "M", "m", "S", "s")

func parseDuration(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	s = strings.TrimPrefix(s, "P")
	if idx := strings.Index(s, "T"); idx >= 0 {
		s = s[idx+1:]
	}
	total := 0.0
	num := ""
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9' || r == '.':
			num += string(r)
		case r == 'H':
			total += atof(num) * 3600
			num = ""
		case r == 'M':
			total += atof(num) * 60
			num = ""
		case r == 'S':
			total += atof(num)
			num = ""
		case r == 'D':
			total += atof(num) * 86400
			num = ""
		}
	}
	if neg {
		total = -total
	}
	return time.Duration(total * float64(time.Second))
}

func atof(s string) float64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func normalizeFrameRate(s string) string {
	if s == "" {
		return ""
	}
	if strings.Contains(s, "/") {
		parts := strings.SplitN(s, "/", 2)
		num := atof(parts[0])
		den := atof(parts[1])
		if den == 0 {
			return ""
		}
		return strconv.FormatFloat(math.Round(num/den*100)/100, 'f', 2, 64)
	}
	return s
}

func normaliseKID(s string) string {
	parts := strings.Split(s, "-")
	if len(parts) == 5 {
		s = strings.Join(parts, "")
	}
	return strings.ToLower(s)
}
