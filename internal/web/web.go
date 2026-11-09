package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"drmsonic/internal/config"
	"drmsonic/internal/state"
	"drmsonic/ui"
)

type Options struct {
	Bind        string
	Port        int
	BaseURL     string
	Allow       string
	User        string
	Password    string
	Providers   string
	NoAutostart bool
	AccessLog   bool
}

type Server struct {
	opts   Options
	store  *config.Store
	mgr    *state.Manager
	tmpl   *template.Template
	mux    *http.ServeMux
	static http.Handler
	media  *mediaStore
	run    *channelRuntime
	start  time.Time
	stop   chan struct{}
	once   sync.Once
}

type Page struct {
	Title      string
	Version    string
	Tag        string
	BaseUrl    string
	LoggedUser string
	IsAdmin    bool
	LoggedIn   bool

	Providers       []*config.Provider
	Provider        *config.Provider
	ProviderId      string
	Streams         []config.Stream
	StreamsCount    int
	InternalUdpOnly bool
	Status          state.StatusResponse
	Users           []config.User
	Recordings      []config.Recording
	Replays         []config.Recording
	Events          []EventRow
	Timezone        string
	Entries         []EventRow
	Help            template.HTML
	Debug           string
	RunningModes    []string
	OutputModes     []string
	CdmTypes        []string
	ModeNames       []string
}

type EventRow struct {
	Title    string
	Channel  string
	StartFmt string
	EndFmt   string
	OnAir    bool
}

func (p Page) MenuProvider() string {
	if p.ProviderId != "" {
		return p.ProviderId
	}
	if p.Provider != nil {
		return p.Provider.Id
	}
	if len(p.Providers) > 0 {
		return p.Providers[0].Id
	}
	return ""
}

func (p Page) StreamCount() int {
	return len(p.Streams)
}

func New(opts Options, store *config.Store, mgr *state.Manager) (*Server, error) {
	if opts.Providers == "" {
		opts.Providers = "providers"
	}
	if opts.BaseURL == "" {
		opts.BaseURL = fmt.Sprintf("http://127.0.0.1:%d", opts.Port)
	}
	opts.BaseURL = strings.TrimRight(opts.BaseURL, "/")

	funcs := template.FuncMap{
		"join":      strings.Join,
		"upper":     strings.ToUpper,
		"lower":     strings.ToLower,
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,
		"contains":  strings.Contains,
		"trim":      strings.TrimSpace,
		"default": func(def, val string) string {
			if strings.TrimSpace(val) == "" {
				return def
			}
			return val
		},
		"checked": func(b bool) string {
			if b {
				return "checked"
			}
			return ""
		},
		"selected": func(a, b string) string {
			if a == b {
				return "selected"
			}
			return ""
		},
		"keysText": func(keys []string) string {
			return strings.Join(keys, "\n")
		},
		"headersText": func(h []string) string {
			return strings.Join(h, "\n")
		},
		"add": func(a, b int) int { return a + b },
		"each": func(n, i int) bool {
			return n > 0 && i%n == 0
		},
		"tablenotmod": func(v any, n int) bool {
			if n <= 0 {
				return false
			}
			switch list := v.(type) {
			case []config.Stream:
				return len(list)%n != 0
			case []config.User:
				return len(list)%n != 0
			case []*config.Provider:
				return len(list)%n != 0
			}
			return false
		},
		"fmtSize": func(n int) string { return fmt.Sprintf("%d", n) },
	}

	tmpl, err := template.New("root").Funcs(funcs).ParseFS(ui.Templates(), "*.html")
	if err != nil {
		return nil, err
	}

	s := &Server{
		opts:   opts,
		store:  store,
		mgr:    mgr,
		tmpl:   tmpl,
		mux:    http.NewServeMux(),
		static: http.FileServer(http.FS(ui.Static())),
		media:  newMediaStore(),
		run:    newRuntime(),
		start:  time.Now(),
		stop:   make(chan struct{}),
	}
	s.routes()
	go s.onDemandLoop()
	return s, nil
}

func (s *Server) requestShutdown() {
	s.once.Do(func() { close(s.stop) })
}

func (s *Server) ShutdownSignal() <-chan struct{} {
	return s.stop
}

func (s *Server) routes() {
	m := s.mux

	m.HandleFunc("GET /static/", func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/static/", s.static).ServeHTTP(w, r)
	})

	m.HandleFunc("POST /api/action", s.handleAction)

	m.HandleFunc("GET /{$}", s.handleIndex)
	m.HandleFunc("GET /providers", s.handleIndex)
	m.HandleFunc("GET /help", s.handleHelp)
	m.HandleFunc("GET /monitoring", s.handleMonitoring)
	m.HandleFunc("GET /users", s.handleUsers)
	m.HandleFunc("GET /rec", s.handleRecordings)
	m.HandleFunc("GET /records", s.handleRecordings)
	m.HandleFunc("GET /debug", s.handleDebug)
	m.HandleFunc("GET /debug/pprof/", s.handleDebug)

	m.HandleFunc("GET /status", s.handleStatusIndex)
	m.HandleFunc("GET /status/{provider}", s.handleStatus)
	m.HandleFunc("GET /events", s.handleEventsIndex)
	m.HandleFunc("GET /events/{provider}", s.handleEvents)
	m.HandleFunc("GET /config", s.handleConfigIndex)
	m.HandleFunc("GET /config/{provider}", s.handleConfig)

	m.HandleFunc("GET /playlist.m3u", s.handleM3U("all"))
	m.HandleFunc("GET /live.m3u", s.handleM3U("live"))
	m.HandleFunc("GET /event.m3u", s.handleM3U("event"))
	m.HandleFunc("GET /replay.m3u", s.handleM3U("replay"))
	m.HandleFunc("GET /app1.conf", s.handleApp1)

	m.HandleFunc("GET /stream/{provider}/{stream}/master.m3u8", s.handleMasterPlaylist)
	m.HandleFunc("GET /stream/{provider}/{stream}/stream.m3u8", s.handleMediaPlaylist)
	m.HandleFunc("GET /stream/{provider}/{stream}/{file}", s.handleSegment)

	m.HandleFunc("/", s.handleNotFound)
}

func (s *Server) Handler() http.Handler {
	return s.logWrap(s.authWrap(s.mux))
}

func (s *Server) logWrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.opts.AccessLog {
			log.Printf("%s %s %s", remoteIP(r), r.Method, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) ListenAndServe() error {
	addr := net.JoinHostPort(s.opts.Bind, fmt.Sprint(s.opts.Port))
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
	}
	return srv.ListenAndServe()
}

func (s *Server) basePage(r *http.Request) Page {
	u, ok := s.currentUser(r)
	p := Page{
		Version:      config.VersionName,
		Tag:          config.VersionTag,
		BaseUrl:      "/",
		RunningModes: config.RunningModes,
		OutputModes:  config.OutputModes,
		CdmTypes:     []string{config.CdmWidevine, config.CdmPlayReady, config.CdmVerimatrix},
		ModeNames:    []string{config.ModeLive, config.ModeReplay, config.ModeVOD},
		Providers:    s.store.Providers(),
	}
	for _, prov := range s.store.Providers() {
		p.StreamsCount += len(prov.Streams)
	}
	if ok {
		p.LoggedUser = u.Username
		p.IsAdmin = u.IsAdmin
		p.LoggedIn = true
	} else {
		p.LoggedUser = "no-auth"
		p.IsAdmin = true
		p.LoggedIn = true
	}
	return p
}

func (s *Server) render(w http.ResponseWriter, name string, p Page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, p); err != nil {
		log.Printf("template %s: %v", name, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) renderJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		log.Printf("json: %v", err)
	}
}

func (s *Server) baseFor(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	if host == "" {
		if s.opts.BaseURL != "" {
			return s.opts.BaseURL
		}
		host = fmt.Sprintf("127.0.0.1:%d", s.opts.Port)
	}
	return scheme + "://" + host
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		s.handleIndex(w, r)
		return
	}
	s.handleIndex(w, r)
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
