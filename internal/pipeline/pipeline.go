package pipeline

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"drmsonic/internal/fetch"
	"drmsonic/internal/manifest"
)

type TrackInfo struct {
	ID        string
	Desc      string
	Codecs    string
	Width     int
	Height    int
	Bandwidth int
	Lang      string
	Kid       string
}

type Callbacks struct {
	Publish func(seg []byte, dur float64)
	Status  func(string)
	Info    func(string)
	Error   func(string)
	BwIn    func(int64)
	Tracks  func(video, audio []TrackInfo)
}

type Config struct {
	Name         string
	ManifestURL  string
	Mode         string
	Video        string
	Audio        string
	Keys         []string
	UserAgent    string
	Bind         string
	Proxy        string
	Doh          string
	Headers      []string
	Retries      int
	FixIVSize    bool
	AllowPrivate bool
}

type KeySet struct {
	byKid  map[string][]byte
	single []byte
	lines  int
}

var ErrMissingKey = errors.New("Key(s) missing")

func ParseKeys(lines []string) *KeySet {
	ks := &KeySet{byKid: make(map[string][]byte, len(lines))}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.FieldsFunc(line, func(r rune) bool { return r == ':' || r == ' ' || r == ',' })
		switch len(fields) {
		case 2:
			kid, err1 := decodeHex(fields[0])
			key, err2 := decodeHex(fields[1])
			if err1 != nil || err2 != nil || len(key) == 0 {
				continue
			}
			ks.byKid[hex.EncodeToString(kid)] = key
			ks.single = key
			ks.lines++
		case 1:
			key, err := decodeHex(fields[0])
			if err != nil || len(key) == 0 {
				continue
			}
			ks.single = key
			ks.byKid[""] = key
			ks.lines++
		}
	}
	return ks
}

func (k *KeySet) Len() int {
	if k == nil {
		return 0
	}
	return k.lines
}

func (k *KeySet) For(kid []byte) []byte {
	if k == nil {
		return nil
	}
	if len(kid) > 0 {
		if key, ok := k.byKid[hex.EncodeToString(kid)]; ok {
			return key
		}
	}
	if key, ok := k.byKid[""]; ok {
		return key
	}
	if len(k.byKid) == 1 {
		for _, key := range k.byKid {
			return key
		}
	}
	return k.single
}

func decodeHex(s string) ([]byte, error) {
	clean := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '-' || s[i] == ' ' {
			continue
		}
		clean = append(clean, s[i])
	}
	return hex.DecodeString(string(clean))
}

func newClient(cfg Config) *fetch.Client {
	return fetch.New(fetch.Options{
		UserAgent:    cfg.UserAgent,
		Bind:         cfg.Bind,
		Proxy:        cfg.Proxy,
		Doh:          cfg.Doh,
		Headers:      cfg.Headers,
		Retries:      cfg.Retries,
		Timeout:      25 * time.Second,
		AllowPrivate: cfg.AllowPrivate,
	})
}

func Run(ctx context.Context, cfg Config, cb Callbacks) error {
	if cfg.ManifestURL == "" {
		return errors.New("empty manifest url")
	}
	cb.Status("Starting")
	if isHLSURL(cfg.ManifestURL) {
		return runHLS(ctx, cfg, cb)
	}
	return runDASH(ctx, cfg, cb)
}

func isHLSURL(u string) bool {
	lower := strings.ToLower(u)
	return strings.Contains(lower, ".m3u8") || strings.Contains(lower, "format=m3u8")
}

func fail(cb Callbacks, err error) error {
	cb.Status(ErrorStatus(err))
	cb.Error(Describe(err))
	return err
}

func ErrorStatus(err error) string {
	short := Describe(err)
	if raw := err.Error(); raw != short {
		return "Error: " + short + "\n" + raw
	}
	return "Error: " + short
}

func endBroadcast(cb Callbacks) error {
	cb.Status(ErrBroadcastEnded.Error())
	return ErrBroadcastEnded
}

func endGrace(update time.Duration) time.Duration {
	grace := 2 * update
	if grace > 10*time.Second {
		grace = 10 * time.Second
	}
	return grace
}

func shiftNegativeCTS(samples []avSample) {
	minCTS := int64(0)
	found := false
	for _, s := range samples {
		if !s.video {
			continue
		}
		cts := s.pts - s.dts
		if !found || cts < minCTS {
			minCTS = cts
			found = true
		}
	}
	if !found || minCTS >= 0 {
		return
	}
	for i := range samples {
		if samples[i].video {
			samples[i].dts += minCTS
			samples[i].end += minCTS
		}
	}
}

func chooseVideo(tracks []manifest.Track, want string) *manifest.Track {
	if len(tracks) == 0 {
		return nil
	}
	want = strings.TrimSpace(want)
	if want != "" && want != "all" && want != "best" && want != "max" {
		for i := range tracks {
			if tracks[i].ID == want {
				return &tracks[i]
			}
		}
	}
	best := &tracks[0]
	for i := range tracks {
		if tracks[i].Bandwidth > best.Bandwidth {
			best = &tracks[i]
		}
	}
	return best
}
