package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"drmsonic/internal/config"
	"drmsonic/internal/fetch"
	"drmsonic/internal/manifest"
	"drmsonic/internal/pipeline"
)

func (s *Server) acRefreshStreams(w http.ResponseWriter, req actionRequest) {
	p := s.providerOrError(w, req.ProviderId)
	if p == nil {
		return
	}
	ids := requestedIDs(req)
	payloadByID := map[string]map[string]any{}
	for _, payload := range req.Streams {
		id := str(mapGet(payload, "Id"))
		if id == "" || id == "all" || strings.Contains(id, ",") {
			continue
		}
		payloadByID[id] = payload
	}

	dirty := false
	var firstErr error
	for i := range p.Streams {
		st := &p.Streams[i]
		if !matchIDs(ids, st.Id) {
			continue
		}
		if payload, ok := payloadByID[st.Id]; ok {
			applyStreamPayload(st, payload)
		}
		if err := s.probeStreamTracks(p, st); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		dirty = true
	}
	if dirty {
		if err := s.store.Put(p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	s.mgr.BumpGlobal()
	if !dirty && firstErr != nil {
		http.Error(w, firstErr.Error(), http.StatusInternalServerError)
		return
	}
	s.renderJSON(w, "ok")
}

func (s *Server) probeStreamTracks(p *config.Provider, st *config.Stream) error {
	rawURL := strings.TrimSpace(st.Manifest)
	if rawURL == "" {
		return fmt.Errorf("empty manifest url")
	}
	page, err := url.Parse(rawURL)
	if err != nil || page.Scheme == "" || page.Host == "" {
		return fmt.Errorf("invalid manifest url")
	}

	cl := fetch.New(fetch.Options{
		UserAgent:    p.UserAgent,
		Bind:         bindFor(p, st),
		Proxy:        proxyFor(p, st),
		Doh:          dohFor(p, st),
		Headers:      headersFor(p, st),
		Retries:      retriesFor(p),
		Timeout:      30 * time.Second,
		AllowPrivate: s.opts.AllowPrivate,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	body, err := cl.Get(ctx, rawURL)
	if err != nil {
		return err
	}

	var video, audio []pipeline.TrackInfo
	path := strings.ToLower(page.Path)
	if strings.Contains(path, ".m3u8") || looksLikeHLS(body) {
		pl, err := manifest.ParseHLS(body, page)
		if err != nil {
			return err
		}
		for _, v := range pl.Variants {
			desc := strconv.Itoa(v.Width) + "x" + strconv.Itoa(v.Height)
			if v.Height > 0 {
				desc = fmt.Sprintf("%dp (%dx%d)", v.Height, v.Width, v.Height)
			}
			video = append(video, pipeline.TrackInfo{
				ID:        v.URL,
				Desc:      desc,
				Bandwidth: v.Bandwidth,
				Width:     v.Width,
				Height:    v.Height,
				Codecs:    v.Codecs,
			})
		}
	} else {
		m, err := manifest.ParseMPD(body, page)
		if err != nil {
			return err
		}
		for _, t := range m.Video {
			desc := fmt.Sprintf("%dx%d", t.Width, t.Height)
			if t.Height > 0 {
				desc = fmt.Sprintf("%dp (%dx%d)", t.Height, t.Width, t.Height)
			}
			video = append(video, pipeline.TrackInfo{
				ID:        t.ID,
				Desc:      desc,
				Codecs:    t.Codecs,
				Width:     t.Width,
				Height:    t.Height,
				Bandwidth: t.Bandwidth,
				Kid:       t.KID,
			})
		}
		for _, t := range m.Audio {
			desc := t.Lang
			if desc == "" {
				desc = t.ID
			}
			audio = append(audio, pipeline.TrackInfo{
				ID:        t.ID,
				Desc:      desc,
				Codecs:    t.Codecs,
				Bandwidth: t.Bandwidth,
				Lang:      t.Lang,
				Kid:       t.KID,
			})
		}
	}
	if len(video) == 0 && len(audio) == 0 {
		return fmt.Errorf("no tracks in manifest")
	}
	s.applyTracks(p, st, video, audio)
	return nil
}

func looksLikeHLS(body []byte) bool {
	s := strings.TrimSpace(string(body))
	return strings.HasPrefix(s, "#EXTM3U")
}
