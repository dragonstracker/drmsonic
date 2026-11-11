package config

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

const VersionName = "drmsonic"

var VersionTag = "v1.1.4"

const (
	ModeLive   = "live"
	ModeReplay = "replay"
	ModeVOD    = "vod"

	RunInternalRemuxer = "internalremuxer"
	RunFFmpegResident  = "ffmpegresident"
	RunFFmpegTSHLS     = "ffmpegtshls"
	RunFFmpegMultiTS   = "ffmpegmultitshls"
	RunFMP4HLS         = "fmp4hls"

	OutDirectHLS = "directhls"
	OutSRTServer = "srtserver"
	OutPipe      = "pipe"
	OutUDPPush   = "udppush"

	CdmWidevine   = "widevine"
	CdmPlayReady  = "playready"
	CdmVerimatrix = "verimatrix"

	CdmInternal = "internal"
	CdmExternal = "external"
)

var RunningModes = []string{
	RunInternalRemuxer,
}

var OutputModes = []string{
	OutDirectHLS,
	OutUDPPush,
}

type Track struct {
	Id        string `json:"Id"`
	Kid       string `json:"Kid"`
	Desc      string `json:"Desc"`
	Codecs    string `json:"Codecs"`
	Width     int    `json:"Width"`
	Height    int    `json:"Height"`
	FrameRate string `json:"FrameRate"`
	Bandwidth int    `json:"Bandwidth"`
	Language  string `json:"Language"`
	Index     int    `json:"Index"`
}

func (t Track) String() string {
	return t.Desc
}

type Stream struct {
	Name    string `json:"Name"`
	Id      string `json:"Id"`
	Mode    string `json:"Mode"`
	IsEvent bool   `json:"IsEvent"`

	Manifest        string   `json:"Manifest"`
	SessionManifest bool     `json:"SessionManifest"`
	ManifestScript  string   `json:"ManifestScript"`
	Headers         []string `json:"Headers"`

	Video         string  `json:"Video"`
	Audio         string  `json:"Audio"`
	Subtitles     string  `json:"Subtitles"`
	VideoList     []Track `json:"VideoList"`
	AudioList     []Track `json:"AudioList"`
	SubtitlesList []Track `json:"SubtitlesList"`

	Keys []string `json:"Keys"`

	UseCdm          bool   `json:"UseCdm"`
	CdmType         string `json:"CdmType"`
	CdmMode         string `json:"CdmMode"`
	Cdm             string `json:"Cdm"`
	PRClientVersion string `json:"PRClientVersion"`
	PRCustomData    string `json:"PRCustomData"`
	PRLAVersion     string `json:"PRLAVersion"`
	VmxUniqueId     string `json:"VmxUniqueId"`

	RunningMode  string `json:"RunningMode"`
	OutputMode   string `json:"OutputMode"`
	ModeOverride bool   `json:"ModeOverride"`
	MultiTS      bool   `json:"MultiTS"`

	Autostart bool `json:"Autostart"`
	OnDemand  bool `json:"OnDemand"`
	SpeedUp   bool `json:"SpeedUp"`

	NetworkOverride bool   `json:"NetworkOverride"`
	NetworkScope    string `json:"NetworkScope"`
	Bind            string `json:"Bind"`
	Proxy           string `json:"Proxy"`
	Doh             string `json:"Doh"`

	RecordEvent  bool `json:"RecordEvent"`
	IgnoreUpdate bool `json:"IgnoreUpdate"`
	FixIvSize    bool `json:"FixIvSize"`
	TimeRange    bool `json:"TimeRange"`

	RangeStartTime string `json:"RangeStartTime"`
	RangeEndTime   string `json:"RangeEndTime"`

	Start int64 `json:"Start"`
	End   int64 `json:"End"`

	Output                string `json:"Output"`
	PipeOutputParams      string `json:"PipeOutputParams"`
	PipeOutputCmdFormated string `json:"PipeOutputCmdFormated"`
	ProtoOutputParams     string `json:"ProtoOutputParams"`

	NoRestartOnError       bool `json:"NoRestartOnError"`
	NoRestartOnTrackChange bool `json:"NoRestartOnTrackChange"`
	NoWaitFullPlaylist     bool `json:"NoWaitFullPlaylist"`

	PlaylistDuration     float64 `json:"PlaylistDuration"`
	NbAnnouncedFragments int     `json:"NbAnnouncedFragments"`

	Status       string `json:"Status"`
	StatusColor  string `json:"StatusColor"`
	StreamInfo   string `json:"StreamInfo"`
	StreamErrors string `json:"StreamErrors"`

	Running      bool   `json:"Running"`
	OnAirId      string `json:"OnAirId"`
	ManifestInfo string `json:"ManifestInfo"`
	NameColor    string `json:"NameColor"`
}

func (s *Stream) EffectiveRunningMode(p *Provider) string {
	if s.ModeOverride && s.RunningMode != "" {
		return s.RunningMode
	}
	if p.RunningMode != "" {
		return p.RunningMode
	}
	return RunInternalRemuxer
}

func (s *Stream) EffectiveOutputMode(p *Provider) string {
	if s.ModeOverride && s.OutputMode != "" {
		return s.OutputMode
	}
	if p.OutputMode != "" {
		return p.OutputMode
	}
	return OutDirectHLS
}

func (s *Stream) KeyMap() map[string]string {
	m := make(map[string]string, len(s.Keys))
	for _, k := range s.Keys {
		parts := splitOnce(k, ":")
		if parts[0] == "" || parts[1] == "" {
			continue
		}
		m[normalizeKid(parts[0])] = parts[1]
	}
	return m
}

type Provider struct {
	Id   string `json:"Id"`
	Name string `json:"Name"`

	RunningMode string `json:"RunningMode"`
	OutputMode  string `json:"OutputMode"`

	Bind         string   `json:"Bind"`
	Proxy        string   `json:"Proxy"`
	Doh          string   `json:"Doh"`
	NetworkScope string   `json:"NetworkScope"`
	Headers      []string `json:"Headers"`

	UserAgent          string `json:"UserAgent"`
	Script             string `json:"Script"`
	VmxUniqueId        string `json:"VmxUniqueId"`
	XForwardedFor      bool   `json:"XForwardedFor"`
	AlwaysResetSession bool   `json:"AlwaysResetSession"`

	PipeOutputCmdFormated string `json:"PipeOutputCmdFormated"`

	AutorestartPeriod         int `json:"AutorestartPeriod"`
	RandomAutostartPeriod     int `json:"RandomAutostartPeriod"`
	SequencialAutostartPeriod int `json:"SequencialAutostartPeriod"`

	RestartDelay    int  `json:"RestartDelay"`
	RestartFinished bool `json:"RestartFinished"`

	PlaylistDuration     float64 `json:"PlaylistDuration"`
	HlsFragmentsDuration int     `json:"HlsFragmentsDuration"`
	NbAnnouncedFragments int     `json:"NbAnnouncedFragments"`

	PlaybackDelay      int  `json:"PlaybackDelay"`
	ContinuousPlayback bool `json:"ContinuousPlayback"`
	NoWaitFullPlaylist bool `json:"NoWaitFullPlaylist"`
	IgnoreStaticDash   bool `json:"IgnoreStaticDash"`
	UseDashDelay       bool `json:"UseDashDelay"`

	HttpGetRetries       int `json:"HttpGetRetries"`
	StallDetectTimeout   int `json:"StallDetectTimeout"`
	MaxConcurrentStreams int `json:"MaxConcurrentStreams"`

	NoRestartOnError       bool `json:"NoRestartOnError"`
	NoRestartOnTrackChange bool `json:"NoRestartOnTrackChange"`

	EventsAutorefresh   bool   `json:"EventsAutorefresh"`
	EventsAutoremove    bool   `json:"EventsAutoremove"`
	EventsRefreshPeriod int    `json:"EventsRefreshPeriod"`
	ChannelsAutoremove  bool   `json:"ChannelsAutoremove"`
	ReuseEventIndex     bool   `json:"ReuseEventIndex"`
	EpgTimezone         string `json:"EpgTimezone"`

	Streams []Stream `json:"Streams"`
}

func (p *Provider) FindStream(id string) *Stream {
	for i := range p.Streams {
		if p.Streams[i].Id == id {
			return &p.Streams[i]
		}
	}
	return nil
}

func (p *Provider) EffectiveRunningMode() string {
	if p.RunningMode != "" {
		return p.RunningMode
	}
	return RunInternalRemuxer
}

func (p *Provider) EffectiveOutputMode() string {
	if p.OutputMode != "" {
		return p.OutputMode
	}
	return OutDirectHLS
}

type User struct {
	Username     string   `json:"Username"`
	Password     string   `json:"Password"`
	StreamToken  string   `json:"StreamToken"`
	ProviderIds  []string `json:"ProviderIds"`
	Network      string   `json:"Network"`
	IsAdmin      bool     `json:"IsAdmin"`
	HasWebAccess bool     `json:"HasWebAccess"`
	NoPlaylist   bool     `json:"NoPlaylist,omitempty"`
}

func (u User) CanUsePlaylist() bool {
	return !u.NoPlaylist
}

func (u User) NetworkList() []string {
	out := make([]string, 0, 4)
	for _, part := range strings.Split(u.Network, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func NewStreamToken() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte("fallback-token-needs-entropy"))
	}
	return hex.EncodeToString(b[:])
}

func EnsureUserTokens(users []User) ([]User, bool) {
	changed := false
	for i := range users {
		if users[i].StreamToken == "" && users[i].Password != "" {
			users[i].StreamToken = NewStreamToken()
			changed = true
		}
	}
	return users, changed
}

func EnsureDefaultAdmin(users []User) ([]User, bool) {
	for _, u := range users {
		if u.Password != "" {
			return users, false
		}
	}
	for i := range users {
		if users[i].Username != "admin" {
			continue
		}
		users[i].Password = "admin"
		users[i].IsAdmin = true
		users[i].HasWebAccess = true
		if users[i].StreamToken == "" {
			users[i].StreamToken = NewStreamToken()
		}
		return users, true
	}
	return append(users, User{
		Username:     "admin",
		Password:     "admin",
		IsAdmin:      true,
		HasWebAccess: true,
		StreamToken:  NewStreamToken(),
	}), true
}

type Config struct {
	Users  []User `json:"Users"`
	NoAuth bool   `json:"NoAuth,omitempty"`
}

type Recording struct {
	Id           int    `json:"Id"`
	ProviderName string `json:"ProviderName"`
	StreamName   string `json:"StreamName"`
	StreamId     string `json:"StreamId"`
	Start        string `json:"Start"`
	End          string `json:"End"`
	Status       string `json:"Status"`
	StreamingUrl string `json:"StreamingUrl"`
}

func splitOnce(s, sep string) [2]string {
	for i := 0; i < len(s); i++ {
		if string(s[i]) == sep {
			return [2]string{s[:i], s[i+1:]}
		}
	}
	return [2]string{s, ""}
}

func normalizeKid(kid string) string {
	out := make([]byte, 0, len(kid))
	for i := 0; i < len(kid); i++ {
		c := kid[i]
		if c == '-' {
			continue
		}
		if c >= 'A' && c <= 'F' {
			c += 32
		}
		out = append(out, c)
	}
	return string(out)
}
