package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"drmsonic/internal/config"
	"drmsonic/internal/out"
	"drmsonic/internal/pipeline"
	"drmsonic/internal/state"
)

type channelRuntime struct {
	mu   sync.Mutex
	runs map[string]context.CancelFunc
}

func newRuntime() *channelRuntime {
	return &channelRuntime{runs: make(map[string]context.CancelFunc)}
}

func (s *Server) StopAll() {
	s.stopAll()
}

func (s *Server) Autostart() {
	now := time.Now().Unix()
	for _, p := range s.store.Providers() {
		for i := range p.Streams {
			st := &p.Streams[i]
			if !st.Autostart {
				continue
			}
			if st.IsEvent && st.Start > 0 {
				if now < st.Start || (st.End > 0 && now > st.End) {
					continue
				}
			}
			s.startChannel(p, st)
			log.Printf("autostart %s/%s", p.Id, st.Id)
		}
	}
}

func (s *Server) startChannel(prov *config.Provider, st *config.Stream) {
	key := prov.Id + "/" + st.Id

	s.run.mu.Lock()
	if _, ok := s.run.runs[key]; ok {
		s.run.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.run.runs[key] = cancel
	s.run.mu.Unlock()

	s.mgr.Start(prov.Id, st.Id)
	ch := s.mgr.Ensure(prov.Id, st.Id)
	ch.StatusLine = "Starting"
	ch.LastError = ""
	s.media.touch(prov.Id, st.Id, "")

	rends := renditionIDs(st)
	if len(rends) <= 1 {
		vid := ""
		if len(rends) == 1 {
			vid = rends[0]
		}
		go s.supervise(ctx, prov, st, ch, key, vid, true)
		return
	}
	var wg sync.WaitGroup
	for i, vid := range rends {
		vid := vid
		delay := time.Duration(i) * 250 * time.Millisecond
		wg.Add(1)
		go func() {
			defer wg.Done()
			if delay > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(delay):
				}
			}
			s.supervise(ctx, prov, st, ch, key, vid, false)
		}()
	}
	go func() {
		wg.Wait()
		s.run.mu.Lock()
		delete(s.run.runs, key)
		s.run.mu.Unlock()
		s.mgr.Stop(prov.Id, st.Id)
	}()
}

func renditionIDs(st *config.Stream) []string {
	sel := strings.TrimSpace(st.Video)
	if sel == "" || sel == "best" {
		return nil
	}
	if sel == "all" {
		out := make([]string, 0, len(st.VideoList))
		for _, t := range st.VideoList {
			if t.Id == "" || t.Id == "best" || t.Id == "all" {
				continue
			}
			out = append(out, t.Id)
		}
		return out
	}
	parts := strings.Split(sel, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "best" || part == "all" || part == "worst" || part == "none" {
			continue
		}
		if strings.ContainsAny(part, "<>=") || strings.HasPrefix(part, "codec=") || strings.HasPrefix(part, "bw") || strings.HasPrefix(part, "h") || strings.HasPrefix(part, "top:") {
			continue
		}
		for _, t := range st.VideoList {
			if t.Id == part && !seen[part] {
				seen[part] = true
				out = append(out, part)
				break
			}
		}
	}
	return out
}

func (s *Server) stopChannel(prov, stream string) {
	key := prov + "/" + stream
	s.run.mu.Lock()
	cancel, ok := s.run.runs[key]
	if ok {
		delete(s.run.runs, key)
	}
	s.run.mu.Unlock()
	if ok {
		cancel()
	}
	s.mgr.Stop(prov, stream)
}

func (s *Server) restartChannel(prov *config.Provider, st *config.Stream) {
	ch := s.mgr.Get(prov.Id, st.Id)
	if ch == nil || !ch.Running {
		return
	}
	s.stopChannel(prov.Id, st.Id)
	s.startChannel(prov, st)
}

func (s *Server) restartRunning(prov *config.Provider) {
	for i := range prov.Streams {
		st := &prov.Streams[i]
		ch := s.mgr.Get(prov.Id, st.Id)
		if ch != nil && ch.Running {
			s.stopChannel(prov.Id, st.Id)
			s.startChannel(prov, st)
		}
	}
}

func (s *Server) stopAll() {
	s.run.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.run.runs))
	for k, c := range s.run.runs {
		cancels = append(cancels, c)
		delete(s.run.runs, k)
	}
	s.run.mu.Unlock()
	for _, c := range cancels {
		c()
	}
}

func (s *Server) onDemandLoop() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.sweepOnDemand()
		}
	}
}

func (s *Server) sweepOnDemand() {
	for _, prov := range s.store.Providers() {
		for i := range prov.Streams {
			st := &prov.Streams[i]
			if !st.OnDemand || st.Autostart {
				continue
			}
			ch := s.mgr.Get(prov.Id, st.Id)
			if ch == nil || !ch.Running {
				continue
			}
			if s.media.idle(prov.Id, st.Id) < onDemandIdle(prov) {
				continue
			}
			s.stopChannel(prov.Id, st.Id)
			s.media.drop(prov.Id, st.Id)
			log.Printf("%s/%s: idle, stopped on demand stream", prov.Id, st.Id)
		}
	}
}

func onDemandIdle(prov *config.Provider) time.Duration {
	secs := prov.PlaylistDuration
	if secs <= 0 {
		secs = 60
	}
	limit := time.Duration(secs) * time.Second
	if limit < 60*time.Second {
		limit = 60 * time.Second
	}
	return limit
}

func (s *Server) supervise(ctx context.Context, prov *config.Provider, st *config.Stream, ch *state.Channel, key, rend string, ownKey bool) {
	defer func() {
		if ownKey {
			s.run.mu.Lock()
			delete(s.run.runs, key)
			s.run.mu.Unlock()
			s.mgr.Stop(prov.Id, st.Id)
		}
	}()

	var udp *out.UDPSender
	if rend == "" && st.OutputMode == config.OutUDPPush {
		sender, err := out.DialUDP(st.ProtoOutputParams)
		if err != nil {
			ch.LastError = err.Error()
			log.Printf("%s/%s: udp output: %v", prov.Id, st.Id, err)
		} else {
			udp = sender
			defer udp.Close()
		}
	}

	var inBytes int64
	stopTicker := make(chan struct{})
	defer close(stopTicker)
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-stopTicker:
				return
			case <-t.C:
				n := atomic.SwapInt64(&inBytes, 0)
				if rend == "" || rend == firstRendition(st) {
					ch.BwIn = float64(n) * 8 / 1e6
				}
			}
		}
	}()

	videoSel := st.Video
	if rend != "" {
		videoSel = rend
	}

	manifestURL := st.Manifest
	keys := append([]string{}, st.Keys...)
	if strings.TrimSpace(prov.Script) != "" {
		if st.SessionManifest {
			if u, err := s.resolveScriptManifest(prov, st); err != nil {
				log.Printf("%s/%s: manifest script: %v", prov.Id, st.Id, err)
				if ch.StatusLine == "Starting" {
					ch.LastError = err.Error()
					ch.StatusLine = "Manifest script failed"
				}
			} else if u != "" {
				manifestURL = u
			}
		}
		if st.UseCdm {
			if k, err := s.resolveScriptKeys(prov, st); err != nil {
				log.Printf("%s/%s: cdm script: %v", prov.Id, st.Id, err)
				if len(keys) == 0 && (rend == "" || rend == firstRendition(st)) {
					ch.LastError = err.Error()
					if ch.StatusLine == "Starting" || ch.StatusLine == "Manifest script failed" {
						ch.StatusLine = "CDM script failed"
					}
				}
			} else if len(k) > 0 {
				keys = k
			}
		}
	}

	cfg := pipeline.Config{
		Name:         st.Name,
		ManifestURL:  manifestURL,
		Mode:         st.Mode,
		Video:        videoSel,
		Audio:        st.Audio,
		Keys:         keys,
		UserAgent:    prov.UserAgent,
		Bind:         bindFor(prov, st),
		Proxy:        proxyFor(prov, st),
		Doh:          dohFor(prov, st),
		Headers:      headersFor(prov, st),
		Retries:      retriesFor(prov),
		FixIVSize:    st.FixIvSize,
		AllowPrivate: s.opts.AllowPrivate,
	}

	tracks := func(video, audio []pipeline.TrackInfo) {
		if s.applyTracks(prov, st, video, audio) {
			if err := s.store.Put(prov); err != nil {
				log.Printf("save tracks %s/%s: %v", prov.Id, st.Id, err)
			}
		}
	}

	cb := pipeline.Callbacks{
		Publish: func(seg []byte, dur float64) {
			if udp != nil {
				if err := udp.Write(seg); err != nil {
					log.Printf("%s/%s: udp write: %v", prov.Id, st.Id, err)
				}
			}
			s.media.push(prov.Id, st.Id, rend, seg, dur, windowFor(prov, st))
			if dur > 0 {
				s.media.setTarget(prov.Id, st.Id, rend, int(math.Ceil(dur)))
			}
			atomic.AddInt64(&inBytes, int64(len(seg)))
			if ch.StatusLine != "Running" {
				ch.StatusLine = "Running"
				ch.LastError = ""
			}
		},
		Status: func(text string) {
			if rend == "" || rend == firstRendition(st) {
				ch.StatusLine = text
			}
		},
		Info: func(text string) {
			if rend == "" || rend == firstRendition(st) {
				ch.Info = text
			}
		},
		Error: func(text string) {
			if ctx.Err() != nil {
				return
			}
			ch.LastError = text
			log.Printf("%s/%s: %s", prov.Id, st.Id, text)
		},
		Tracks: tracks,
	}

	for {
		err := pipeline.Run(ctx, cfg, cb)
		if ctx.Err() != nil {
			return
		}
		ended := errors.Is(err, pipeline.ErrBroadcastEnded)
		if err != nil {
			log.Printf("%s/%s: %v", prov.Id, st.Id, err)
			if ended {
				ch.LastError = ""
			} else {
				ch.LastError = pipeline.Describe(err)
				if ch.StatusLine == "" || ch.StatusLine == "Starting" || ch.StatusLine == "Running" {
					ch.StatusLine = pipeline.ErrorStatus(err)
				}
			}
		}
		if ended && !prov.RestartFinished {
			ch.StatusLine = "Broadcast ended"
			return
		}
		if st.NoRestartOnError || prov.NoRestartOnError {
			return
		}
		delay := prov.RestartDelay
		if delay <= 0 {
			delay = 5
		}
		switch {
		case ended:
			ch.StatusLine = fmt.Sprintf("Broadcast ended (autorestart in %ds)", delay)
		case err != nil:
			ch.StatusLine = fmt.Sprintf("%s (autorestart in %ds)", pipeline.ErrorStatus(err), delay)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(delay) * time.Second):
		}
	}
}

func firstRendition(st *config.Stream) string {
	ids := renditionIDs(st)
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func (s *Server) applyTracks(prov *config.Provider, st *config.Stream, video, audio []pipeline.TrackInfo) bool {
	if st.IgnoreUpdate {
		return false
	}
	changed := false
	wasSingle := len(renditionIDs(st)) <= 1

	if len(video) > 0 {
		list := make([]config.Track, 0, len(video))
		for _, t := range video {
			list = append(list, config.Track{
				Id:        t.ID,
				Desc:      t.Desc,
				Codecs:    t.Codecs,
				Width:     t.Width,
				Height:    t.Height,
				Bandwidth: t.Bandwidth,
				Kid:       t.Kid,
			})
		}
		if !sameTracks(st.VideoList, list) {
			st.VideoList = list
			changed = true
		}
		if st.Video == "" {
			if best := bestTrack(list); best != "" {
				st.Video = best
				changed = true
			}
		}
	}
	if len(audio) > 0 {
		list := make([]config.Track, 0, len(audio))
		for _, t := range audio {
			list = append(list, config.Track{
				Id:        t.ID,
				Desc:      t.Desc,
				Codecs:    t.Codecs,
				Bandwidth: t.Bandwidth,
				Language:  t.Lang,
				Kid:       t.Kid,
			})
		}
		if !sameTracks(st.AudioList, list) {
			st.AudioList = list
			changed = true
		}
		if st.Audio == "" {
			if best := bestTrack(list); best != "" {
				st.Audio = best
				changed = true
			}
		}
	}
	if changed && wasSingle && st.Video == "all" && len(renditionIDs(st)) > 1 {
		go func() {
			time.Sleep(200 * time.Millisecond)
			s.restartChannel(prov, st)
		}()
	}
	return changed
}

func sameTracks(a, b []config.Track) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Id != b[i].Id || a[i].Codecs != b[i].Codecs || a[i].Desc != b[i].Desc || a[i].Kid != b[i].Kid {
			return false
		}
	}
	return true
}

func bestTrack(list []config.Track) string {
	if len(list) == 0 {
		return ""
	}
	best := list[0]
	for _, t := range list {
		if t.Bandwidth > best.Bandwidth {
			best = t
		}
	}
	return best.Id
}

func bindFor(p *config.Provider, st *config.Stream) string {
	if st.NetworkOverride && st.Bind != "" {
		return st.Bind
	}
	return p.Bind
}

func proxyFor(p *config.Provider, st *config.Stream) string {
	if st.NetworkOverride && st.Proxy != "" {
		return st.Proxy
	}
	return p.Proxy
}

func dohFor(p *config.Provider, st *config.Stream) string {
	if st.NetworkOverride && st.Doh != "" {
		return st.Doh
	}
	return p.Doh
}

func headersFor(p *config.Provider, st *config.Stream) []string {
	out := append([]string{}, p.Headers...)
	out = append(out, st.Headers...)
	return out
}

func retriesFor(p *config.Provider) int {
	if p.HttpGetRetries > 0 {
		return p.HttpGetRetries
	}
	return 2
}
