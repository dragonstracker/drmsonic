package pipeline

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"drmsonic/internal/cenc"
	"drmsonic/internal/manifest"
	"drmsonic/internal/mp4"
	"drmsonic/internal/mux"
	"drmsonic/internal/remux"
)

func runHLS(ctx context.Context, cfg Config, cb Callbacks) error {
	cl := newClient(cfg)
	keys := ParseKeys(cfg.Keys)

	load := func(raw string) (*manifest.HLSPlaylist, *url.URL, error) {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, nil, err
		}
		body, err := cl.Get(ctx, raw)
		if err != nil {
			return nil, nil, err
		}
		pl, err := manifest.ParseHLS(body, u)
		if err != nil {
			return nil, nil, err
		}
		return pl, u, nil
	}

	pl, mediaURL, err := load(cfg.ManifestURL)
	if err != nil {
		return fail(cb, err)
	}

	var variants []TrackInfo
	if pl.Master {
		for _, v := range pl.Variants {
			variants = append(variants, TrackInfo{
				ID:        v.URL,
				Desc:      strconv.Itoa(v.Width) + "x" + strconv.Itoa(v.Height),
				Codecs:    v.Codecs,
				Width:     v.Width,
				Height:    v.Height,
				Bandwidth: v.Bandwidth,
			})
		}
		cb.Tracks(variants, nil)

		best := pl.BestVariant()
		if best == nil {
			return fail(cb, errors.New("no variants in master playlist"))
		}
		pl, mediaURL, err = load(best.URL)
		if err != nil {
			return fail(cb, err)
		}
	}

	cb.Status("Running")
	cb.Info("HLS")

	var muxer *mux.Muxer
	var hmedia *mp4.Track
	var hinit *mp4.Init
	var outDTS int64
	encrypted := false

	if pl.MapURL != "" {
		body, err := cl.Get(ctx, pl.MapURL)
		if err != nil {
			return fail(cb, err)
		}
		hinit, err = mp4.ParseInit(body)
		if err != nil {
			return fail(cb, err)
		}
		for _, t := range hinit.Tracks {
			if t.Handler == "vide" {
				hmedia = t
				break
			}
		}
		if hmedia == nil {
			hmedia = hinit.Tracks[0]
		}
		video := hmedia.Handler == "vide"
		muxer = mux.New(video, !video, hmedia.Codec == "hvc1")
		if !video {
			muxer.SetLanguage("eng")
		}
		encrypted = hmedia.Encrypted
	} else {
		muxer = mux.New(false, false, false)
	}

	lastSeq := int64(-1)
	update := time.Duration(pl.TargetDuration) * time.Second
	if update < 2*time.Second {
		update = 2 * time.Second
	}

	for ctx.Err() == nil {
		set := currentSegments(pl, cfg.Mode)
		if len(set) == 0 {
			wait(ctx, update)
			fresh, _, err := load(mediaURL.String())
			if err == nil {
				pl = fresh
			}
			continue
		}
		if lastSeq < 0 {
			lastSeq = set[0].Seq - 1
		}

		progressed := false
		for _, seg := range set {
			if seg.Seq <= lastSeq || seg.Gap {
				continue
			}
			lastSeq = seg.Seq
			progressed = true

			body, err := cl.Get(ctx, seg.URL)
			if err != nil {
				cb.Error(err.Error())
				continue
			}
			if seg.Key != nil && seg.Key.Method == "AES-128" {
				key, err := resolveHLSKey(cl, ctx, seg.Key, mediaURL)
				if err != nil {
					return fail(cb, err)
				}
				iv, err := resolveHLSIV(seg.Key, seg.Seq)
				if err != nil {
					return fail(cb, err)
				}
				body, err = decryptAES128(key, iv, body)
				if err != nil {
					cb.Error(err.Error())
					continue
				}
			}

			if hmedia == nil {
				cb.Publish(body, seg.Duration)
				continue
			}

			samples, err := hlsSamples(body, hmedia, encrypted, keys)
			if err != nil {
				cb.Error(err.Error())
				continue
			}
			if len(samples) == 0 {
				continue
			}
			sort.SliceStable(samples, func(i, j int) bool { return samples[i].dts < samples[j].dts })

			base90 := samples[0].dts - outDTS
			videoEnd := samples[len(samples)-1].end
			muxer.Begin()
			for _, s := range samples {
				pts := s.pts - base90
				dts := s.dts - base90
				if pts < 0 {
					pts = 0
				}
				if dts < 0 {
					dts = 0
				}
				if s.video {
					muxer.Video(pts, dts, s.key, s.data)
				} else {
					muxer.Audio(pts, s.data)
				}
			}
			outDTS = videoEnd - base90
			out := muxer.Reset()
			if len(out) > 0 {
				cb.Publish(out, seg.Duration)
			}
		}

		if !progressed {
			wait(ctx, update)
		}
		if fresh, _, err := load(mediaURL.String()); err == nil {
			pl = fresh
		}
	}
	return nil
}

func currentSegments(pl *manifest.HLSPlaylist, mode string) []manifest.HLSSegment {
	if len(pl.Segments) == 0 {
		return nil
	}
	if mode == "replay" || pl.EndList {
		return pl.Segments
	}
	if len(pl.Segments) > 4 {
		return pl.Segments[len(pl.Segments)-3:]
	}
	return pl.Segments
}

func hlsSamples(body []byte, media *mp4.Track, encrypted bool, keys *KeySet) ([]avSample, error) {
	frag, err := mp4.ParseFragment(body, media.IVSize)
	if err != nil {
		return nil, err
	}
	video := media.Handler == "vide"
	hevc := media.Codec == "hvc1" || media.Codec == "hev1"
	rate, channels := mp4.ParseAudioSpecificConfig(media.AudioConfig)
	if rate == 0 {
		rate = media.Rate
	}
	if channels == 0 {
		channels = media.Channels
	}
	if channels == 0 {
		channels = 2
	}

	out := make([]avSample, 0, len(frag.Samples))
	cursor := int64(frag.BaseTime)
	for _, sm := range frag.Samples {
		data, ok := frag.Data(sm)
		if !ok {
			cursor += int64(sm.Duration)
			continue
		}
		if encrypted {
			key := keys.For(media.DefaultKID)
			if key == nil {
				return nil, ErrMissingKey
			}
			decoded, err := cenc.DecryptSample(key, sm.IV, data, sm.Subsamples)
			if err != nil {
				return nil, err
			}
			data = decoded
		}
		dts := cursor * 90000 / media.Timescale
		pts := dts + int64(sm.CTS)*90000/media.Timescale
		end := dts + int64(sm.Duration)*90000/media.Timescale
		if video {
			annex, keyframe := remux.ToAnnexB(data, media.VPS, media.SPS, media.PPS, hevc)
			out = append(out, avSample{dts: dts, pts: pts, end: end, key: keyframe, video: true, data: annex})
		} else {
			out = append(out, avSample{dts: dts, pts: pts, end: end, video: false, data: remux.ADTS(data, rate, channels)})
		}
		cursor += int64(sm.Duration)
	}
	shiftNegativeCTS(out)
	return out, nil
}

func resolveHLSKey(cl interface {
	Get(context.Context, string) ([]byte, error)
}, ctx context.Context, key *manifest.Key, base *url.URL) ([]byte, error) {
	u, err := url.Parse(key.URI)
	if err != nil {
		return nil, err
	}
	raw, err := cl.Get(ctx, base.ResolveReference(u).String())
	if err != nil {
		return nil, err
	}
	if len(raw) < 16 {
		return nil, errors.New("invalid AES key")
	}
	return raw[:16], nil
}

func resolveHLSIV(key *manifest.Key, seq int64) ([]byte, error) {
	if key.IV == "" {
		return hex.DecodeString(manifest.IVForSequence(seq))
	}
	iv, err := hex.DecodeString(strings.TrimPrefix(key.IV, "0x"))
	if err != nil {
		return nil, err
	}
	if len(iv) != 16 {
		return nil, errors.New("invalid IV length")
	}
	return iv, nil
}

func decryptAES128(key, iv, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(data)%aes.BlockSize != 0 {
		return nil, errors.New("ciphertext is not a multiple of the block size")
	}
	out := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, data)
	if n := int(out[len(out)-1]); n > 0 && n <= aes.BlockSize && n <= len(out) {
		out = out[:len(out)-n]
	}
	return out, nil
}
