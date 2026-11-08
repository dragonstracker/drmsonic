package web

import (
	"context"
	"log"
	"math"
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
	for _, p := range s.store.Providers() {
		for i := range p.Streams {
			st := &p.Streams[i]
			if st.Autostart {
				s.startChannel(p, st)
				log.Printf("autostart %s/%s", p.Id, st.Id)
			}
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

	go s.supervise(ctx, prov, st, ch, key)
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

func (s *Server) supervise(ctx context.Context, prov *config.Provider, st *config.Stream, ch *state.Channel, key string) {
	defer func() {
		s.run.mu.Lock()
		delete(s.run.runs, key)
		s.run.mu.Unlock()
		s.mgr.Stop(prov.Id, st.Id)
	}()

	var udp *out.UDPSender
	if st.OutputMode == config.OutUDPPush {
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
				ch.BwIn = float64(n) * 8 / 1e6
			}
		}
	}()

	cfg := pipeline.Config{
		Name:        st.Name,
		ManifestURL: st.Manifest,
		Mode:        st.Mode,
		Video:       st.Video,
		Audio:       st.Audio,
		Keys:        st.Keys,
		UserAgent:   prov.UserAgent,
		Bind:        bindFor(prov, st),
		Proxy:       proxyFor(prov, st),
		Doh:         dohFor(prov, st),
		Headers:     headersFor(prov, st),
		Retries:     retriesFor(prov),
		FixIVSize:   st.FixIvSize,
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
			s.media.push(prov.Id, st.Id, seg, dur, windowFor(prov, st))
			if dur > 0 {
				s.media.setTarget(prov.Id, st.Id, int(math.Ceil(dur)))
			}
			atomic.AddInt64(&inBytes, int64(len(seg)))
			if ch.StatusLine != "Running" {
				ch.StatusLine = "Running"
			}
		},
		Status: func(text string) {
			ch.StatusLine = text
		},
		Info: func(text string) {
			ch.Info = text
		},
		Error: func(text string) {
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
		if err != nil {
			ch.LastError = err.Error()
			if ch.StatusLine == "" || ch.StatusLine == "Starting" || ch.StatusLine == "Running" {
				ch.StatusLine = "Error: " + err.Error()
			}
			log.Printf("%s/%s: %v", prov.Id, st.Id, err)
		}
		if st.NoRestartOnError || prov.NoRestartOnError {
			return
		}
		delay := prov.RestartDelay
		if delay <= 0 {
			delay = 5
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(delay) * time.Second):
		}
	}
}

func (s *Server) applyTracks(prov *config.Provider, st *config.Stream, video, audio []pipeline.TrackInfo) bool {
	if st.IgnoreUpdate {
		return false
	}
	changed := false

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
			})
		}
		if !sameTracks(st.VideoList, list) {
			st.VideoList = list
			changed = true
		}
		if st.Video == "" || st.Video == "all" {
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
			})
		}
		if !sameTracks(st.AudioList, list) {
			st.AudioList = list
			changed = true
		}
		if st.Audio == "" || st.Audio == "all" {
			if best := bestTrack(list); best != "" {
				st.Audio = best
				changed = true
			}
		}
	}
	return changed
}

func sameTracks(a, b []config.Track) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Id != b[i].Id || a[i].Codecs != b[i].Codecs || a[i].Desc != b[i].Desc {
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
