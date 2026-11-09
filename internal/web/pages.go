package web

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"drmsonic/internal/config"
	"drmsonic/internal/state"
)

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	p := s.basePage(r)
	p.Title = "Providers"
	s.render(w, "providers.html", p)
}

func (s *Server) handleStatusIndex(w http.ResponseWriter, r *http.Request) {
	first := s.store.DefaultProvider()
	if first == nil {
		p := s.basePage(r)
		p.Title = "Streams"
		s.render(w, "streams.html", p)
		return
	}
	http.Redirect(w, r, "/status/"+first.Id, http.StatusFound)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("provider")
	p := s.basePage(r)
	prov := s.store.Provider(id)
	if prov == nil {
		http.Error(w, "invalid provider", http.StatusNotFound)
		return
	}
	p.Title = "Streams"
	p.Provider = prov
	p.ProviderId = prov.Id
	p.Streams = prov.Streams
	s.render(w, "streams.html", p)
}

func (s *Server) handleConfigIndex(w http.ResponseWriter, r *http.Request) {
	first := s.store.DefaultProvider()
	if first == nil {
		p := s.basePage(r)
		p.Title = "Config"
		s.render(w, "config.html", p)
		return
	}
	http.Redirect(w, r, "/config/"+first.Id, http.StatusFound)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("provider")
	p := s.basePage(r)
	prov := s.store.Provider(id)
	if prov == nil {
		http.Error(w, "invalid provider", http.StatusNotFound)
		return
	}
	p.Title = "Config"
	p.Provider = prov
	p.ProviderId = prov.Id
	p.Streams = prov.Streams
	s.render(w, "config.html", p)
}

func (s *Server) handleEventsIndex(w http.ResponseWriter, r *http.Request) {
	first := s.store.DefaultProvider()
	if first == nil {
		p := s.basePage(r)
		p.Title = "Events"
		s.render(w, "events.html", p)
		return
	}
	http.Redirect(w, r, "/events/"+first.Id, http.StatusFound)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("provider")
	p := s.basePage(r)
	prov := s.store.Provider(id)
	if prov == nil {
		http.Error(w, "invalid provider", http.StatusNotFound)
		return
	}
	p.Title = "Events"
	p.Provider = prov
	p.ProviderId = prov.Id
	p.Events = []EventRow{}
	p.Replays = []config.Recording{}
	s.render(w, "events.html", p)
}

func (s *Server) handleMonitoring(w http.ResponseWriter, r *http.Request) {
	p := s.basePage(r)
	p.Title = "Monitoring"
	s.render(w, "monitoring.html", p)
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	p := s.basePage(r)
	p.Title = "Users"
	p.Users = s.store.Users()
	s.render(w, "users.html", p)
}

func (s *Server) handleRecordings(w http.ResponseWriter, r *http.Request) {
	p := s.basePage(r)
	p.Title = "Recordings"
	p.Recordings = []config.Recording{}
	s.render(w, "recordings.html", p)
}

func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	p := s.basePage(r)
	p.Title = "Help"
	s.render(w, "help.html", p)
}

func (s *Server) handleDebug(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<html><pre>System info:\n")
	fmt.Fprintf(w, "\tMemory usage: %s\n", runtimeMemory())
	fmt.Fprintf(w, "\tGo runtime: %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(w, "\tUptime: %s\n", shortUptime(s.start))
	fmt.Fprintf(w, "\nProviders:\n")
	total, running := 0, 0
	for _, p := range s.store.Providers() {
		run := 0
		for i := range p.Streams {
			total++
			if ch := s.mgr.Get(p.Id, p.Streams[i].Id); ch != nil && ch.Running {
				run++
				running++
			}
		}
		fmt.Fprintf(w, "\t[%s/%s]\n\t\tTotal: %d\n\t\tRunning: %d\n", p.Id, p.Name, len(p.Streams), run)
	}
	fmt.Fprintf(w, "\tTotal: %d\n\tRunning: %d\n", total, running)
	fmt.Fprintf(w, "\n</pre></html>")
}

func (s *Server) handleApp1(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	var b strings.Builder
	for _, p := range s.store.Providers() {
		for i := range p.Streams {
			st := &p.Streams[i]
			fmt.Fprintf(&b, "stream %s_%s {\n", p.Id, st.Id)
			fmt.Fprintf(&b, "\turl hls://%s/stream/%s/%s/master.m3u8;\n", strings.TrimPrefix(strings.TrimPrefix(s.opts.BaseURL, "http://"), "https://"), p.Id, st.Id)
			fmt.Fprintf(&b, "\ttitle \"[%s] %s\";\n", p.Id, st.Name)
			fmt.Fprintf(&b, "}\n\n")
		}
	}
	fmt.Fprint(w, b.String())
}

func (s *Server) handleM3U(scope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-mpegurl")
		user := r.URL.Query().Get("user")
		suffix := ""
		if user != "" {
			suffix = "?user=" + user
		}

		var b strings.Builder
		b.WriteString("#EXTM3U x-tvg-url=\"\"\n\n")
		base := s.baseFor(r)

		for _, p := range s.store.Providers() {
			for i := range p.Streams {
				st := &p.Streams[i]
				if scope == "live" && st.Mode != config.ModeLive {
					continue
				}
				if scope == "event" && !st.IsEvent {
					continue
				}
				if scope == "replay" {
					continue
				}
				fmt.Fprintf(&b, "#EXTINF:1 group-title=\"%s\" tvg-id=\"%s/%s\" tvg-logo=\"\" provider=\"%s\" provider-logo=\"\" provider-type=\"iptv\",[%s] %s\n",
					p.Id, p.Id, st.Id, p.Id, p.Id, st.Name)
				fmt.Fprintf(&b, "%s/stream/%s/%s/master.m3u8%s\n\n", base, p.Id, st.Id, suffix)
			}
		}
		fmt.Fprint(w, b.String())
	}
}

func (s *Server) handleMasterPlaylist(w http.ResponseWriter, r *http.Request) {
	prov, st := s.streamFromPath(w, r)
	if prov == nil {
		return
	}
	w.Header().Set("Content-Type", "application/x-mpegurl")
	s.media.touch(prov.Id, st.Id)
	if err := s.ensureRunning(prov, st); err != nil {
		http.Error(w, "#EXTM3U\n", http.StatusServiceUnavailable)
		return
	}
	bandwidth := 0
	res := "1920x1080"
	fps := "30.00"
	codecs := "avc1.640028,mp4a.40.2"
	v := selectedVideo(st)
	if v != nil {
		bandwidth = v.Bandwidth
		if v.Width > 0 && v.Height > 0 {
			res = fmt.Sprintf("%dx%d", v.Width, v.Height)
		}
		if v.FrameRate != "" {
			fps = v.FrameRate
		}
		if v.Codecs != "" {
			codecs = v.Codecs
		}
	}
	if bandwidth == 0 {
		bandwidth = 8133000
	}
	if st.Audio != "none" && len(st.AudioList) > 0 {
		codec := st.AudioList[0].Codecs
		if codec == "" {
			codec = "mp4a.40.2"
		}
		codecs += "," + codec
	}
	fmt.Fprintf(w, "#EXTM3U\n")
	fmt.Fprintf(w, "#EXT-X-STREAM-INF:PROGRAM-ID=1,BANDWIDTH=%d,RESOLUTION=%s,FRAME-RATE=%s,CODECS=\"%s\"\n", bandwidth, res, fps, codecs)
	fmt.Fprintf(w, "stream.m3u8%s\n", queryString(r))
}

func (s *Server) handleMediaPlaylist(w http.ResponseWriter, r *http.Request) {
	prov, st := s.streamFromPath(w, r)
	if prov == nil {
		return
	}
	w.Header().Set("Content-Type", "application/x-mpegurl")
	s.media.touch(prov.Id, st.Id)
	if err := s.ensureRunning(prov, st); err != nil {
		http.Error(w, "stream unavailable", http.StatusServiceUnavailable)
		return
	}
	s.awaitSegments(r, prov, st, 2, 10*time.Second)
	buf := s.segments(prov, st)
	seq := buf.Seq
	if len(buf.Items) > 0 {
		seq = buf.Items[0].Seq
	}
	fmt.Fprintf(w, "#EXTM3U\n")
	fmt.Fprintf(w, "#EXT-X-VERSION:6\n")
	fmt.Fprintf(w, "#EXT-X-MEDIA-SEQUENCE:%d\n", seq)
	fmt.Fprintf(w, "#EXT-X-DISCONTINUITY-SEQUENCE:0\n")
	fmt.Fprintf(w, "#EXT-X-TARGETDURATION:%d\n", buf.Target)
	fmt.Fprintf(w, "#EXT-X-INDEPENDENT-SEGMENTS\n")
	for _, seg := range buf.Items {
		fmt.Fprintf(w, "#EXTINF:%.8f,\n", seg.Duration)
		fmt.Fprintf(w, "%s%s\n", seg.Name, queryString(r))
	}
}

func (s *Server) handleSegment(w http.ResponseWriter, r *http.Request) {
	prov, st := s.streamFromPath(w, r)
	if prov == nil {
		return
	}
	name := r.PathValue("file")
	if !strings.HasSuffix(name, ".ts") {
		http.NotFound(w, r)
		return
	}
	s.media.touch(prov.Id, st.Id)
	if err := s.ensureRunning(prov, st); err != nil {
		http.Error(w, "stream unavailable", http.StatusServiceUnavailable)
		return
	}
	if ch := s.mgr.Get(prov.Id, st.Id); ch != nil && ch.Running {
		ch.Readers++
	}
	s.awaitSegments(r, prov, st, 1, 5*time.Second)
	data, ok := s.segmentData(prov, st, name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	w.Write(data)
}

func (s *Server) awaitSegments(r *http.Request, prov *config.Provider, st *config.Stream, want int, limit time.Duration) {
	if want < 1 {
		want = 1
	}
	deadline := time.Now().Add(limit)
	for len(s.segments(prov, st).Items) < want {
		if time.Now().After(deadline) || r.Context().Err() != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func (s *Server) streamFromPath(w http.ResponseWriter, r *http.Request) (*config.Provider, *config.Stream) {
	pid := r.PathValue("provider")
	sid := r.PathValue("stream")
	prov := s.store.Provider(pid)
	if prov == nil {
		http.Error(w, "invalid provider", http.StatusNotFound)
		return nil, nil
	}
	st := prov.FindStream(sid)
	if st == nil {
		http.Error(w, "invalid stream", http.StatusNotFound)
		return nil, nil
	}
	return prov, st
}

func (s *Server) ensureRunning(prov *config.Provider, st *config.Stream) error {
	ch := s.mgr.Get(prov.Id, st.Id)
	if ch != nil && ch.Running {
		return nil
	}
	if !st.OnDemand {
		return fmt.Errorf("stream not started")
	}
	s.startChannel(prov, st)
	return nil
}

func queryString(r *http.Request) string {
	if uid := r.URL.Query().Get("uid"); uid != "" {
		return "?uid=" + uid
	}
	return ""
}

func shortUptime(start time.Time) string {
	d := time.Since(start)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}

func runtimeMemory() string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return fmt.Sprintf("alloc=%dMB, total=%dMB, sys=%dMB, num_garb=%d",
		m.Alloc/1024/1024, m.TotalAlloc/1024/1024, m.Sys/1024/1024, m.NumGC)
}

func providerOf(store *config.Store, id string) *config.Provider {
	return store.Provider(id)
}

func selectedVideo(st *config.Stream) *config.Track {
	if len(st.VideoList) == 0 {
		return nil
	}
	var best *config.Track
	for i := range st.VideoList {
		t := &st.VideoList[i]
		if t.Id == st.Video {
			return t
		}
		if best == nil || t.Bandwidth > best.Bandwidth {
			best = t
		}
	}
	return best
}

func statusFor(store *config.Store, mgr *state.Manager, baseURL, id string) state.StatusResponse {
	return state.BuildStatus(store, mgr, baseURL)
}
