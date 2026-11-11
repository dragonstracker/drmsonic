package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"drmsonic/internal/config"
	"drmsonic/internal/state"
	"drmsonic/ui"
)

type Options struct {
	Bind           string
	Port           int
	BaseURL        string
	Allow          string
	User           string
	Password       string
	SessionSecret  string
	TrustedProxies string
	Providers      string
	NoAutostart    bool
	AccessLog      bool
	AllowPrivate   bool
}

type Server struct {
	opts        Options
	store       *config.Store
	mgr         *state.Manager
	tmpl        *template.Template
	mux         *http.ServeMux
	media       *mediaStore
	run         *channelRuntime
	sessions    *sessionStore
	throttle    *loginThrottle
	staticToken string
	scriptsDir         string
	pendingMu          sync.Mutex
	pending            map[string]*pendingUpdate
	refreshMu          sync.Mutex
	lastEventsRefresh  map[string]time.Time
	refreshBusy        map[string]bool
	start              time.Time
	stop               chan struct{}
	once               sync.Once
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
	StreamTables    []*config.Provider
	Provider        *config.Provider
	ProviderId      string
	Streams         []config.Stream
	StreamsCount    int
	InternalUdpOnly bool
	Status          state.StatusResponse
	Users           []config.User
	NoAuth          bool
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
		"fmtSize":    func(n int) string { return fmt.Sprintf("%d", n) },
		"pathEscape": url.PathEscape,
	}

	tmpl, err := template.New("root").Funcs(funcs).ParseFS(ui.Templates(), "*.html")
	if err != nil {
		return nil, err
	}

	s := &Server{
		opts:        opts,
		store:       store,
		mgr:         mgr,
		tmpl:        tmpl,
		mux:         http.NewServeMux(),
		media:       newMediaStore(),
		run:         newRuntime(),
		sessions:    newSessionStore(),
		throttle:    newLoginThrottle(),
		staticToken: deriveStaticToken(opts.User, opts.Password, opts.SessionSecret),
		scriptsDir:        "scripts",
		pending:           map[string]*pendingUpdate{},
		lastEventsRefresh: map[string]time.Time{},
		refreshBusy:       map[string]bool{},
		start:             time.Now(),
		stop:              make(chan struct{}),
	}
	s.routes()
	go s.onDemandLoop()
	go s.eventsAutorefreshLoop()
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

	m.HandleFunc("GET /static/", newStaticHandler(ui.Static()).ServeHTTP)

	m.HandleFunc("GET /login", s.handleLogin)
	m.HandleFunc("POST /login", s.handleLogin)
	m.HandleFunc("POST /logout", s.handleLogout)
	m.HandleFunc("GET /logout", s.handleLogout)

	m.HandleFunc("POST /api/action", s.handleAction)

	m.HandleFunc("GET /{$}", s.requirePanel(s.handleIndex))
	m.HandleFunc("GET /providers", s.requirePanel(s.handleIndex))
	m.HandleFunc("GET /help", s.requirePanel(s.handleHelp))
	m.HandleFunc("GET /monitoring", s.requirePanel(s.handleMonitoring))
	m.HandleFunc("GET /users", s.requireAdmin(s.handleUsers))
	m.HandleFunc("GET /rec", s.requirePanel(s.handleRecordings))
	m.HandleFunc("GET /records", s.requirePanel(s.handleRecordings))
	m.HandleFunc("GET /debug", s.requireAdmin(s.handleDebug))
	m.HandleFunc("GET /debug/pprof/", s.requireAdmin(s.handleDebug))

	m.HandleFunc("GET /status", s.requirePanel(s.handleStatusIndex))
	m.HandleFunc("GET /status/{provider}", s.requirePanel(s.handleStatus))
	m.HandleFunc("GET /events", s.requirePanel(s.handleEventsIndex))
	m.HandleFunc("GET /events/{provider}", s.requirePanel(s.handleEvents))
	m.HandleFunc("GET /config", s.requireAdmin(s.handleConfigIndex))
	m.HandleFunc("GET /config/{provider}", s.requireAdmin(s.handleConfig))

	m.HandleFunc("GET /playlist.m3u", s.handleM3U("all"))
	m.HandleFunc("GET /live.m3u", s.handleM3U("live"))
	m.HandleFunc("GET /event.m3u", s.handleM3U("event"))
	m.HandleFunc("GET /replay.m3u", s.handleM3U("replay"))
	m.HandleFunc("GET /app1.conf", s.handleApp1)

	m.HandleFunc("GET /stream/{provider}/{stream}/v/{rend}/stream.m3u8", s.handleMediaPlaylist)
	m.HandleFunc("GET /stream/{provider}/{stream}/v/{rend}/{file}", s.handleSegment)
	m.HandleFunc("GET /stream/{provider}/{stream}/master.m3u8", s.handleMasterPlaylist)
	m.HandleFunc("GET /stream/{provider}/{stream}/stream.m3u8", s.handleMediaPlaylist)
	m.HandleFunc("GET /stream/{provider}/{stream}/{file}", s.handleSegment)

	m.HandleFunc("/", s.handleNotFound)
}

func (s *Server) Handler() http.Handler {
	return s.logWrap(gzipDynamic(s.pathAuthRewrite(s.authWrap(s.mux))))
}

func (s *Server) logWrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.opts.AccessLog {
			path := r.URL.Path
			if parts := splitPath(path); len(parts) >= 3 && parts[2] == "stream" {
				path = "/****/****/" + strings.Join(parts[2:], "/")
			}
			log.Printf("%s %s %s", clientIP(r, s.opts.TrustedProxies), r.Method, path)
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
	providers := s.visibleProviders(r)
	p := Page{
		Version:      config.VersionName,
		Tag:          config.VersionTag,
		BaseUrl:      "/",
		RunningModes: config.RunningModes,
		OutputModes:  config.OutputModes,
		CdmTypes:     []string{config.CdmWidevine, config.CdmPlayReady, config.CdmVerimatrix},
		ModeNames:    []string{config.ModeLive, config.ModeReplay, config.ModeVOD},
		Providers:    providers,
	}
	for _, prov := range providers {
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
