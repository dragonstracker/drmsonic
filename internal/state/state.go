package state

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"drmsonic/internal/config"
)

type Channel struct {
	Provider string
	Stream   string

	Running    bool
	StartedAt  time.Time
	StoppedAt  time.Time
	LastError  string
	StatusLine string
	Info       string

	BwIn  float64
	BwOut float64

	Readers int

	GlobalChange uint64
	PChange      uint64
}

func (c *Channel) Uptime() time.Duration {
	if !c.Running || c.StartedAt.IsZero() {
		return 0
	}
	return time.Since(c.StartedAt)
}

func (c *Channel) StatusText() string {
	if !c.Running {
		switch c.StatusLine {
		case "", "Starting", "Running", "Waiting for segments":
			return "Stopped"
		}
		return c.StatusLine
	}
	if c.StatusLine == "" || c.StatusLine == "Running" || c.StatusLine == "Starting" {
		if c.StatusLine == "Starting" {
			return "Starting"
		}
		return fmt.Sprintf("Streaming\n%s", shortDuration(c.Uptime()))
	}
	return c.StatusLine
}

func (c *Channel) Failed() bool {
	return strings.HasPrefix(c.StatusLine, "Error")
}

func (c *Channel) Ended() bool {
	return strings.HasPrefix(c.StatusLine, "Broadcast ended")
}

func shortDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}

type Manager struct {
	mu       sync.RWMutex
	channels map[string]*Channel
	started  time.Time
	global   uint64
}

func NewManager() *Manager {
	return &Manager{
		channels: make(map[string]*Channel),
		started:  time.Now(),
	}
}

func key(provider, stream string) string {
	return provider + "/" + stream
}

func (m *Manager) Get(provider, stream string) *Channel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if c, ok := m.channels[key(provider, stream)]; ok {
		return c
	}
	return nil
}

func (m *Manager) Ensure(provider, stream string) *Channel {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(provider, stream)
	c, ok := m.channels[k]
	if !ok {
		c = &Channel{Provider: provider, Stream: stream}
		m.channels[k] = c
	}
	return c
}

func (m *Manager) Start(provider, stream string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(provider, stream)
	c, ok := m.channels[k]
	if !ok {
		c = &Channel{Provider: provider, Stream: stream}
		m.channels[k] = c
	}
	if c.Running {
		return
	}
	c.Running = true
	c.StartedAt = time.Now()
	c.LastError = ""
	c.PChange++
}

func (m *Manager) Stop(provider, stream string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.channels[key(provider, stream)]
	if !ok {
		return
	}
	c.Running = false
	c.StoppedAt = time.Now()
	c.Readers = 0
	c.BwIn = 0
	c.BwOut = 0
}

func (m *Manager) StopProvider(provider string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, c := range m.channels {
		if c.Provider == provider {
			c.Running = false
			c.StoppedAt = time.Now()
			c.Readers = 0
			delete(m.channels, k)
		}
	}
}

func (m *Manager) BumpGlobal() {
	m.mu.Lock()
	m.global++
	m.mu.Unlock()
}

func (m *Manager) Global() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.global
}

func (m *Manager) All() map[string]*Channel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]*Channel, len(m.channels))
	for k, v := range m.channels {
		out[k] = v
	}
	return out
}

func (m *Manager) StartedAt() time.Time {
	return m.started
}

type StreamStatus struct {
	Id                  string `json:"Id"`
	OnAirId             string `json:"OnAirId"`
	Name                string `json:"Name"`
	Mode                string `json:"Mode"`
	IsEvent             bool   `json:"IsEvent"`
	RunningMode         string `json:"RunningMode"`
	OutputMode          string `json:"OutputMode"`
	CurrentVideo        string `json:"CurrentVideo"`
	ManifestInfo        string `json:"ManifestInfo"`
	NameColor           string `json:"NameColor"`
	Status              string `json:"Status"`
	StatusColor         string `json:"StatusColor"`
	StreamInfo          string `json:"StreamInfo"`
	StreamErrors        string `json:"StreamErrors"`
	Running             bool   `json:"Running"`
	OnDemand            bool   `json:"OnDemand"`
	CurrentBind         string `json:"CurrentBind"`
	CurrentProxy        string `json:"CurrentProxy"`
	CurrentDoh          string `json:"CurrentDoh"`
	CurrentNetworkScope string `json:"CurrentNetworkScope"`
	StreamingUrl        string `json:"StreamingUrl"`
	PChangeCounter      uint64 `json:"PChangeCounter"`
}

type ProviderStatus struct {
	Id          string         `json:"Id"`
	Name        string         `json:"Name"`
	RunningMode string         `json:"RunningMode"`
	OutputMode  string         `json:"OutputMode"`
	Streams     []StreamStatus `json:"Streams"`
}

type StatusResponse struct {
	GlobalChangeCounter uint64           `json:"GlobalChangeCounter"`
	StreamsCount        int              `json:"StreamsCount"`
	Providers           []ProviderStatus `json:"Providers"`
	InternalUdpOnly     bool             `json:"InternalUdpOnly"`
}

func TrackText(list []config.Track) string {
	if len(list) == 0 {
		return ""
	}
	out := ""
	for i, t := range list {
		if i > 0 {
			out += "\n"
		}
		out += t.Desc
	}
	return out
}

func BuildStatus(store *config.Store, m *Manager, baseURL string) StatusResponse {
	resp := StatusResponse{GlobalChangeCounter: m.Global()}
	providers := store.Providers()
	resp.Providers = make([]ProviderStatus, 0, len(providers))
	total := 0

	for _, p := range providers {
		ps := ProviderStatus{
			Id:          p.Id,
			Name:        p.Name,
			RunningMode: p.EffectiveRunningMode(),
			OutputMode:  p.EffectiveOutputMode(),
			Streams:     make([]StreamStatus, 0, len(p.Streams)),
		}
		for i := range p.Streams {
			st := &p.Streams[i]
			total++
			ch := m.Get(p.Id, st.Id)
			ss := StreamStatus{
				Id:                  st.Id,
				Name:                st.Name,
				Mode:                st.Mode,
				IsEvent:             st.IsEvent,
				RunningMode:         st.EffectiveRunningMode(p),
				OutputMode:          st.EffectiveOutputMode(p),
				NameColor:           "white",
				StatusColor:         "red",
				Status:              "Stopped",
				OnDemand:            st.OnDemand,
				CurrentBind:         st.Bind,
				CurrentProxy:        st.Proxy,
				CurrentDoh:          st.Doh,
				CurrentNetworkScope: st.NetworkScope,
				StreamingUrl:        fmt.Sprintf("%s/stream/%s/%s/master.m3u8", baseURL, p.Id, st.Id),
			}
			if st.Video != "" {
				ss.CurrentVideo = videoInfo(st)
			}
			ss.ManifestInfo = videoInfo(st)
			ss.StreamInfo = streamInfo(st)
			if ch != nil {
				ss.Running = ch.Running
				ss.Status = ch.StatusText()
				ss.StreamErrors = ch.LastError
				ss.PChangeCounter = ch.PChange
				if ch.Info != "" {
					ss.StreamInfo = ch.Info
				}
				switch {
				case ch.Failed():
					ss.StatusColor = "red"
				case ch.Ended():
					ss.StatusColor = "orange"
				case ch.Running:
					ss.StatusColor = "green"
				default:
					ss.StatusColor = "white"
				}
			}
			ps.Streams = append(ps.Streams, ss)
		}
		resp.Providers = append(resp.Providers, ps)
	}
	resp.StreamsCount = total
	return resp
}

func videoInfo(st *config.Stream) string {
	kind := "dash"
	switch {
	case len(st.Manifest) > 0 && containsAny(st.Manifest, ".m3u8"):
		kind = "hls"
	case len(st.Manifest) > 0 && containsAny(st.Manifest, ".ism"):
		kind = "smooth"
	}
	best := "unknown"
	if len(st.VideoList) > 0 {
		best = st.VideoList[0].Desc
	}
	return fmt.Sprintf("Format: <i><b>%s (%s)</b></i><br>Best video: <i><b>%s</b></i><br>", kind, st.Mode, best)
}

func streamInfo(st *config.Stream) string {
	out := "Video:\n"
	if len(st.VideoList) > 0 {
		out += st.VideoList[0].Desc
	}
	out += "\n\nAudio:\n"
	if len(st.AudioList) > 0 {
		out += st.AudioList[0].Desc
	}
	out += "\n\nSubtitles:\n"
	return out
}

func containsAny(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
