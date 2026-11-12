package pipeline

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"drmsonic/internal/cenc"
	"drmsonic/internal/fetch"
	"drmsonic/internal/manifest"
	"drmsonic/internal/mp4"
	"drmsonic/internal/mux"
	"drmsonic/internal/remux"
)

type avSample struct {
	dts   int64
	pts   int64
	end   int64
	key   bool
	video bool
	data  []byte
}

type streamState struct {
	track    *manifest.Track
	media    *mp4.Track
	video    bool
	keys     *KeySet
	fixIV    bool
	last     int64
	rate     int
	channels int
	hevc     bool
	sidx     *mp4.Sidx
}

func runDASH(ctx context.Context, cfg Config, cb Callbacks) error {
	pageURL, err := url.Parse(cfg.ManifestURL)
	if err != nil {
		return fail(cb, err)
	}
	cl := newClient(cfg)
	keys := ParseKeys(cfg.Keys)

	loadManifest := func() (*manifest.Manifest, error) {
		body, err := cl.Get(ctx, cfg.ManifestURL)
		if err != nil {
			return nil, err
		}
		return manifest.ParseMPD(body, pageURL)
	}

	m, err := loadManifest()
	if err != nil {
		return fail(cb, err)
	}

	videoTrack := chooseVideo(m.Video, cfg.Video)
	audioTrack := chooseAudioTrack(m.Audio, cfg.Audio)
	if videoTrack == nil && audioTrack == nil {
		return fail(cb, errors.New("no playable tracks in manifest"))
	}

	cb.Status("Loading")
	var vSrc, aSrc *streamState
	if videoTrack != nil {
		vSrc, err = openStream(ctx, cl, videoTrack, keys, cfg.FixIVSize, true)
		if err != nil {
			if audioTrack == nil {
				return fail(cb, err)
			}
			vSrc = nil
		}
	}
	if audioTrack != nil {
		aSrc, err = openStream(ctx, cl, audioTrack, keys, cfg.FixIVSize, false)
		if err != nil {
			if vSrc == nil {
				return fail(cb, err)
			}
			aSrc = nil
		}
	}
	if vSrc == nil && aSrc == nil {
		return fail(cb, errors.New("unable to load init segments"))
	}

	reportTracks(cb, m, vSrc, aSrc)

	live := cfg.Mode != "replay" && !m.Static
	update := m.UpdatePeriod
	if update < time.Second {
		update = time.Second
	}
	if update > 10*time.Second {
		update = 10 * time.Second
	}
	if seg := segmentSeconds(vSrc, aSrc); seg > 0 {
		cadence := time.Duration(seg * float64(time.Second))
		if cadence < time.Second {
			cadence = time.Second
		}
		if cadence < update {
			update = cadence
		}
	}

	var muxer *mux.Muxer
	if vSrc != nil {
		muxer = mux.New(true, aSrc != nil, vSrc.hevc)
	} else {
		muxer = mux.New(false, true, false)
		muxer.SetLanguage(aSrc.lang())
	}

	var (
		outDTS     int64
		pending    []avSample
		lastLoad   = time.Now()
		lastSwitch time.Time
		idleAt     time.Time
		loadFails  int
	)

	if !live {
		for _, s := range []*streamState{vSrc, aSrc} {
			if s != nil {
				s.last = 0
			}
		}
	}

	cb.Status("Running")
	cb.Info(fmt.Sprintf("DASH %s", m.Type))

	for ctx.Err() == nil {
		if time.Since(lastLoad) >= update {
			fresh, err := loadManifest()
			if err == nil {
				m = fresh
				live = cfg.Mode != "replay" && !m.Static
				if vSrc != nil {
					if t := findTrack(fresh.Video, vSrc.track.ID); t != nil {
						vSrc.track = t
					}
				}
				if aSrc != nil {
					if t := findTrack(fresh.Audio, aSrc.track.ID); t != nil {
						aSrc.track = t
					}
				}
				lastLoad = time.Now()
				loadFails = 0
			} else {
				loadFails++
				if loadFails >= 2 {
					if endedStatus(err) {
						return endBroadcast(cb)
					}
					return fail(cb, err)
				}
			}
		}

		var vRef manifest.SegmentRef
		haveVideo := false
		if vSrc != nil {
			if ref, ok := pickNextSrc(vSrc, live); ok {
				vRef = ref
				haveVideo = true
			}
		}
		videoAt := time.Now()

		if haveVideo {
			idleAt = time.Time{}
		} else {
			if idleAt.IsZero() {
				idleAt = time.Now()
			}
			if !live && time.Since(idleAt) > endGrace(update) {
				return endBroadcast(cb)
			}
		}

		if !haveVideo {
			if vSrc != nil {
				if live && time.Since(idleAt) > 15*time.Second {
					cb.Status("Waiting for segments")
				}
				wait(ctx, 100*time.Millisecond)
				continue
			}
			aRef, ok := pickNextSrc(aSrc, live)
			if !ok {
				if live && time.Since(idleAt) > 15*time.Second {
					cb.Status("Waiting for segments")
				}
				wait(ctx, 400*time.Millisecond)
				continue
			}
			samples, err := aSrc.fetch(ctx, cl, aRef)
			if err != nil {
				if errors.Is(err, ErrMissingKey) {
					return fail(cb, err)
				}
				cb.Error(Describe(err))
				wait(ctx, time.Second)
				continue
			}
			aSrc.last = aRef.Time
			if len(samples) == 0 {
				continue
			}
			base := samples[0].dts - outDTS
			dur := emitAudio(muxer, samples, base)
			if dur > 0 {
				outDTS += int64(dur * 90000)
				cb.Publish(muxer.Reset(), dur)
			}
			continue
		}

		videoSamples, err := vSrc.fetch(ctx, cl, vRef)
		elapsed := time.Since(videoAt)
		if err != nil {
			if errors.Is(err, ErrMissingKey) {
				return fail(cb, err)
			}
			cb.Error(Describe(err))
			wait(ctx, time.Second)
			continue
		}
		vSrc.last = vRef.Time

		segSeconds := float64(vRef.Duration) / float64(vSrc.track.Timescale)
		autoQuality := cfg.Video == "" || cfg.Video == "all"
		if autoQuality && segSeconds > 0 && elapsed.Seconds() > segSeconds*1.5 && time.Since(lastSwitch) > 30*time.Second {
			if lower := lowerVideo(m.Video, vSrc.track.Bandwidth); lower != nil {
				if next, oerr := openStream(ctx, cl, lower, keys, cfg.FixIVSize, true); oerr == nil {
					vSrc = next
					vSrc.last = -1
					pending = nil
					lastSwitch = time.Now()
					continue
				}
			}
		}

		if aSrc != nil {
			windowStart := float64(vRef.Time) / float64(vSrc.track.Timescale)
			windowEnd := windowStart + float64(vRef.Duration)/float64(vSrc.track.Timescale)
			for i := 0; i < 6; i++ {
				aRef, ok := pickNextSrc(aSrc, live)
				if !ok {
					break
				}
				start := float64(aRef.Time) / float64(aSrc.track.Timescale)
				length := float64(aRef.Duration) / float64(aSrc.track.Timescale)
				if start > windowEnd {
					break
				}
				samples, err := aSrc.fetch(ctx, cl, aRef)
				if err != nil {
					cb.Error(Describe(err))
					break
				}
				aSrc.last = aRef.Time
				if start+length >= windowStart {
					pending = append(pending, samples...)
				}
			}
		}

		if len(videoSamples) == 0 {
			continue
		}

		videoStart := videoSamples[0].dts
		videoEnd := videoSamples[len(videoSamples)-1].end
		base90 := videoStart - outDTS

		var audioPart []avSample
		keep := pending[:0]
		for _, s := range pending {
			switch {
			case s.pts < videoStart-9000:
			case s.pts <= videoEnd:
				audioPart = append(audioPart, s)
			default:
				keep = append(keep, s)
			}
		}
		pending = keep

		all := make([]avSample, 0, len(videoSamples)+len(audioPart))
		all = append(all, videoSamples...)
		all = append(all, audioPart...)
		sort.SliceStable(all, func(i, j int) bool { return all[i].dts < all[j].dts })

		muxer.Begin()
		minDTS := int64(-1)
		maxDTS := int64(0)
		for _, s := range all {
			pts := s.pts - base90
			dts := s.dts - base90
			end := s.end - base90
			if pts < 0 {
				pts = 0
			}
			if dts < 0 {
				dts = 0
			}
			if minDTS < 0 || (s.video && dts < minDTS) {
				minDTS = dts
			}
			if s.video && end > maxDTS {
				maxDTS = end
			}
			if s.video {
				muxer.Video(pts, dts, s.key, s.data)
			} else {
				muxer.Audio(pts, s.data)
			}
		}
		segment := muxer.Reset()
		if len(segment) == 0 {
			continue
		}
		dur := 0.0
		if minDTS >= 0 && maxDTS > minDTS {
			dur = float64(maxDTS-minDTS) / 90000.0
		}
		if dur <= 0 {
			dur = float64(vRef.Duration) / float64(vSrc.track.Timescale)
		}
		outDTS = videoEnd - base90
		cb.Publish(segment, dur)
	}

	return nil
}

func emitAudio(m *mux.Muxer, samples []avSample, base int64) float64 {
	if len(samples) == 0 {
		return 0
	}
	m.Begin()
	minDTS := int64(-1)
	maxDTS := int64(0)
	for _, s := range samples {
		pts := s.pts - base
		if pts < 0 {
			pts = 0
		}
		m.Audio(pts, s.data)
		d := s.dts - base
		if minDTS < 0 || d < minDTS {
			minDTS = d
		}
		if s.end-base > maxDTS {
			maxDTS = s.end - base
		}
	}
	if maxDTS > minDTS {
		return float64(maxDTS-minDTS) / 90000.0
	}
	return 0
}

func wait(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func pickNext(t *manifest.Track, last int64, live bool) (manifest.SegmentRef, bool) {
	tail := 0
	if live {
		tail = 6
	}
	return pickFromRefs(t.TailSegments(tail), last, live)
}

func segmentSeconds(v, a *streamState) float64 {
	for _, s := range []*streamState{v, a} {
		if s == nil || s.track == nil || s.track.Timescale <= 0 {
			continue
		}
		ts := float64(s.track.Timescale)
		if s.track.Template != nil {
			if s.track.Template.Duration > 0 {
				return float64(s.track.Template.Duration) / ts
			}
			items := s.track.Template.Timeline
			if items != nil && len(items.Items) > 0 {
				if d := items.Items[len(items.Items)-1].D; d > 0 {
					return float64(d) / ts
				}
			}
		}
	}
	return 0
}

func pickNextSrc(s *streamState, live bool) (manifest.SegmentRef, bool) {
	if s.track.SegBase == nil {
		return pickNext(s.track, s.last, live)
	}
	tail := 0
	if live {
		tail = 6
	}
	refs := s.track.RangeSegments(s.sidx, tail)
	if len(refs) == 0 {
		if s.sidx == nil && s.last < 0 {
			return manifest.SegmentRef{URL: s.track.FileURL}, true
		}
		return manifest.SegmentRef{}, false
	}
	return pickFromRefs(refs, s.last, live)
}

func pickFromRefs(refs []manifest.SegmentRef, last int64, live bool) (manifest.SegmentRef, bool) {
	if len(refs) == 0 {
		return manifest.SegmentRef{}, false
	}
	if last < 0 {
		idx := 0
		if live {
			idx = len(refs) - 2
			if idx < 0 {
				idx = 0
			}
		}
		return refs[idx], true
	}
	for i, r := range refs {
		if r.Time > last {
			if live && len(refs)-1-i >= 3 {
				i = len(refs) - 1
			}
			return refs[i], true
		}
	}
	return manifest.SegmentRef{}, false
}

func findTrack(tracks []manifest.Track, id string) *manifest.Track {
	for i := range tracks {
		if tracks[i].ID == id {
			return &tracks[i]
		}
	}
	return nil
}

func lowerVideo(tracks []manifest.Track, bandwidth int) *manifest.Track {
	var best *manifest.Track
	for i := range tracks {
		if tracks[i].Bandwidth >= bandwidth || tracks[i].Bandwidth == 0 {
			continue
		}
		if best == nil || tracks[i].Bandwidth > best.Bandwidth {
			best = &tracks[i]
		}
	}
	return best
}

func chooseAudioTrack(tracks []manifest.Track, want string) *manifest.Track {
	if len(tracks) == 0 {
		return nil
	}
	want = strings.TrimSpace(want)
	if want == "none" {
		return nil
	}
	if want != "" && want != "all" {
		for i := range tracks {
			if tracks[i].ID == want {
				return &tracks[i]
			}
		}
		pref := want
		switch {
		case strings.HasPrefix(want, "lang="):
			pref = strings.TrimPrefix(want, "lang=")
		case strings.HasPrefix(want, "desc:"):
			pref = strings.TrimPrefix(want, "desc:")
		}
		for _, raw := range strings.Split(pref, ",") {
			p := strings.ToLower(strings.TrimSpace(raw))
			if p == "" {
				continue
			}
			aliases := []string{p}
			if p == "en" {
				aliases = append(aliases, "eng")
			} else if p == "eng" {
				aliases = append(aliases, "en")
			}
			for i := range tracks {
				lang := strings.ToLower(tracks[i].Lang)
				id := strings.ToLower(tracks[i].ID)
				for _, a := range aliases {
					if lang == a || strings.Contains(id, a) {
						return &tracks[i]
					}
				}
			}
		}
	}
	for i := range tracks {
		lang := strings.ToLower(tracks[i].Lang)
		if lang == "eng" || lang == "en" {
			return &tracks[i]
		}
	}
	return &tracks[0]
}

func openStream(ctx context.Context, cl *fetch.Client, track *manifest.Track, keys *KeySet, fixIV, video bool) (*streamState, error) {
	var body []byte
	var err error
	var sidx *mp4.Sidx
	if track.SegBase != nil {
		initRange := track.InitRange
		if initRange == "" {
			initRange = "0-65535"
		}
		body, err = cl.GetRange(ctx, track.FileURL, initRange)
		if err != nil {
			return nil, err
		}
		sidx, err = loadSidx(ctx, cl, track)
		if err != nil {
			return nil, err
		}
	} else {
		if track.InitURL == "" {
			return nil, fmt.Errorf("missing init segment for representation %s", track.ID)
		}
		body, err = cl.Get(ctx, track.InitURL)
		if err != nil {
			return nil, err
		}
	}
	init, err := mp4.ParseInit(body)
	if err != nil {
		return nil, err
	}
	if len(init.Tracks) == 0 {
		return nil, errors.New("init segment has no tracks")
	}
	media := init.Tracks[0]
	for _, t := range init.Tracks {
		if (t.Handler == "vide") == video {
			media = t
			break
		}
	}

	s := &streamState{
		track:    track,
		media:    media,
		video:    video,
		keys:     keys,
		fixIV:    fixIV,
		last:     -1,
		rate:     media.Rate,
		channels: media.Channels,
		hevc:     strings.HasPrefix(strings.ToLower(track.Codecs), "hvc") || strings.HasPrefix(strings.ToLower(track.Codecs), "hev") || media.Codec == "hvc1",
		sidx:     sidx,
	}

	if rate, channels := mp4.ParseAudioSpecificConfig(media.AudioConfig); rate > 0 && !video {
		s.rate = rate
		if channels > 0 {
			s.channels = channels
		}
	}
	if s.channels == 0 {
		s.channels = 2
	}
	if s.rate == 0 {
		s.rate = 48000
	}
	if media.Encrypted && strings.EqualFold(media.Scheme, "cbcs") {
		return nil, errors.New("cbcs encryption is not supported")
	}
	if media.Encrypted && keys.Len() == 0 {
		return nil, ErrMissingKey
	}
	return s, nil
}

func loadSidx(ctx context.Context, cl *fetch.Client, track *manifest.Track) (*mp4.Sidx, error) {
	if track.IndexRange == "" {
		return nil, nil
	}
	body, err := cl.GetRange(ctx, track.FileURL, track.IndexRange)
	if err != nil {
		return nil, err
	}
	return mp4.ParseSidx(body, rangeStart(track.IndexRange))
}

func rangeStart(s string) int64 {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) == 0 {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func (s *streamState) lang() string {
	return s.track.Lang
}

func (s *streamState) fetch(ctx context.Context, cl *fetch.Client, ref manifest.SegmentRef) ([]avSample, error) {
	var body []byte
	var err error
	if ref.Range != "" {
		body, err = cl.GetRange(ctx, ref.URL, ref.Range)
	} else {
		body, err = cl.Get(ctx, ref.URL)
	}
	if err != nil {
		return nil, err
	}
	frag, err := mp4.ParseFragment(body, s.media.IVSize)
	if err != nil {
		return nil, err
	}

	out := make([]avSample, 0, len(frag.Samples))
	cursor := int64(frag.BaseTime)

	for _, sm := range frag.Samples {
		data, ok := frag.Data(sm)
		if !ok {
			cursor += int64(sm.Duration)
			continue
		}

		if s.media.Encrypted {
			key := s.keys.For(s.media.DefaultKID)
			if key == nil {
				return nil, ErrMissingKey
			}
			iv := sm.IV
			if s.fixIV && len(iv) == 16 {
				iv = iv[:8]
			}
			decoded, err := cenc.DecryptSample(key, iv, data, sm.Subsamples)
			if err != nil {
				return nil, err
			}
			data = decoded
		}

		dts := (cursor - s.track.PTO) * 90000 / s.track.Timescale
		pts := dts + int64(sm.CTS)*90000/s.track.Timescale
		end := dts + int64(sm.Duration)*90000/s.track.Timescale

		var es []byte
		if s.video {
			annex, keyframe := remux.ToAnnexB(data, s.media.VPS, s.media.SPS, s.media.PPS, s.hevc)
			out = append(out, avSample{dts: dts, pts: pts, end: end, key: keyframe, video: true, data: annex})
		} else {
			es = remux.ADTS(data, s.rate, s.channels)
			out = append(out, avSample{dts: dts, pts: pts, end: end, video: false, data: es})
		}
		cursor += int64(sm.Duration)
	}
	shiftNegativeCTS(out)
	return out, nil
}

func reportTracks(cb Callbacks, m *manifest.Manifest, vSrc, aSrc *streamState) {
	var video, audio []TrackInfo
	for _, t := range m.Video {
		video = append(video, TrackInfo{
			ID:        t.ID,
			Desc:      fmt.Sprintf("%dx%d", t.Width, t.Height),
			Codecs:    t.Codecs,
			Width:     t.Width,
			Height:    t.Height,
			Bandwidth: t.Bandwidth,
			Kid:       t.KID,
		})
	}
	for _, t := range m.Audio {
		audio = append(audio, TrackInfo{
			ID:        t.ID,
			Desc:      t.Lang,
			Codecs:    t.Codecs,
			Bandwidth: t.Bandwidth,
			Lang:      t.Lang,
			Kid:       t.KID,
		})
	}
	if vSrc != nil {
		cb.Info(fmt.Sprintf("%s %dx%d %s", vSrc.media.Codec, vSrc.media.Width, vSrc.media.Height, vSrc.track.Codecs))
	} else if aSrc != nil {
		cb.Info(fmt.Sprintf("%s %dHz %dch", aSrc.track.Codecs, aSrc.rate, aSrc.channels))
	}
	cb.Tracks(video, audio)
}
