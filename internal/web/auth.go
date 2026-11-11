package web

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"

	"drmsonic/internal/config"
)

type ctxKey int

const ctxUserKey ctxKey = 1

func withUser(ctx context.Context, u config.User) context.Context {
	return context.WithValue(ctx, ctxUserKey, u)
}

func userFrom(ctx context.Context) (config.User, bool) {
	u, ok := ctx.Value(ctxUserKey).(config.User)
	return u, ok
}

func (s *Server) authEnabled() bool {
	if s.store.NoAuth() {
		return false
	}
	if s.opts.User != "" && s.opts.Password != "" {
		return true
	}
	for _, u := range s.store.Users() {
		if u.Password != "" {
			return true
		}
	}
	return false
}

func (s *Server) publicPath(path string) bool {
	if path == "/login" || strings.HasPrefix(path, "/static/") {
		return true
	}
	return false
}

func (s *Server) authWrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.publicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if u, ok := s.resolveUser(r); ok {
			next.ServeHTTP(w, r.WithContext(withUser(r.Context(), u)))
			return
		}
		if !s.authEnabled() {
			next.ServeHTTP(w, r.WithContext(withUser(r.Context(), config.User{
				Username: "no-auth", IsAdmin: true, HasWebAccess: true,
			})))
			return
		}
		if wantsHTML(r) && !isMediaPath(r.URL.Path) {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

func isMediaPath(path string) bool {
	parts := splitPath(path)
	if len(parts) == 0 {
		return false
	}
	if isUserPassPath(parts) || parts[0] == "t" {
		return true
	}
	switch parts[0] {
	case "stream", "app1.conf", "playlist.m3u", "live.m3u", "event.m3u", "replay.m3u":
		return true
	}
	return false
}

func wantsHTML(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/html") || accept == "" || strings.Contains(accept, "*/*")
}

func (s *Server) resolveUser(r *http.Request) (config.User, bool) {
	ip := clientIP(r, s.opts.TrustedProxies)
	if s.allowListed(ip) {
		return config.User{Username: "allow", IsAdmin: true, HasWebAccess: true}, true
	}
	if tok, ok := pathToken(r); ok {
		return s.userFromStreamToken(tok, ip)
	}
	if u, p, ok := pathUserPass(r); ok {
		return s.authenticate(u, p, ip)
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		if name, ok := s.sessions.get(c.Value); ok {
			if u, ok := s.userByName(name, ip); ok {
				return u, true
			}
		}
	}
	if u, p, ok := r.BasicAuth(); ok {
		return s.authenticate(u, p, ip)
	}
	return config.User{}, false
}

func (s *Server) userFromStreamToken(token, ip string) (config.User, bool) {
	if s.staticToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.staticToken)) == 1 {
		return config.User{
			Username: s.opts.User, IsAdmin: true, HasWebAccess: true, StreamToken: s.staticToken,
		}, true
	}
	u, ok := s.store.UserByToken(token)
	if !ok {
		return config.User{}, false
	}
	if u.Network != "" && !networkAllows(u.NetworkList(), ip) {
		return config.User{}, false
	}
	return u, true
}

func (s *Server) userByName(name, ip string) (config.User, bool) {
	if s.checkStatic(name, s.opts.Password) && name == s.opts.User {
		return config.User{
			Username: name, IsAdmin: true, HasWebAccess: true, StreamToken: s.staticToken, Password: s.opts.Password,
		}, true
	}
	for _, u := range s.store.Users() {
		if u.Username != name {
			continue
		}
		if u.Network != "" && !networkAllows(u.NetworkList(), ip) {
			return config.User{}, false
		}
		return u, true
	}
	return config.User{}, false
}

func (s *Server) authenticate(name, pass, ip string) (config.User, bool) {
	if name == "" || pass == "" {
		return config.User{}, false
	}
	if s.checkStatic(name, pass) {
		return config.User{
			Username: name, IsAdmin: true, HasWebAccess: true, StreamToken: s.staticToken, Password: pass,
		}, true
	}
	for _, u := range s.store.Users() {
		if u.Username != name {
			continue
		}
		if u.Password == "" {
			return config.User{}, false
		}
		if subtle.ConstantTimeCompare([]byte(pass), []byte(u.Password)) != 1 {
			return config.User{}, false
		}
		if u.Network != "" && !networkAllows(u.NetworkList(), ip) {
			return config.User{}, false
		}
		return u, true
	}
	return config.User{}, false
}

func (s *Server) checkStatic(user, pass string) bool {
	if s.opts.User == "" || s.opts.Password == "" {
		return false
	}
	u := subtle.ConstantTimeCompare([]byte(user), []byte(s.opts.User)) == 1
	p := subtle.ConstantTimeCompare([]byte(pass), []byte(s.opts.Password)) == 1
	return u && p
}

func (s *Server) currentUser(r *http.Request) (config.User, bool) {
	if u, ok := userFrom(r.Context()); ok {
		return u, true
	}
	return s.resolveUser(r)
}

func (s *Server) isAdmin(r *http.Request) bool {
	u, ok := s.currentUser(r)
	return ok && u.IsAdmin
}

func (s *Server) canUsePanel(r *http.Request) bool {
	u, ok := s.currentUser(r)
	return ok && (u.HasWebAccess || u.IsAdmin)
}

func (s *Server) canUsePlaylist(r *http.Request) bool {
	u, ok := s.currentUser(r)
	return ok && u.CanUsePlaylist()
}

func (s *Server) canAccessProvider(r *http.Request, providerID string) bool {
	u, ok := s.currentUser(r)
	if !ok {
		return !s.authEnabled()
	}
	if u.IsAdmin {
		return true
	}
	if len(u.ProviderIds) == 0 {
		return false
	}
	for _, id := range u.ProviderIds {
		if id == providerID || id == "*" {
			return true
		}
	}
	return false
}

func (s *Server) visibleProviders(r *http.Request) []*config.Provider {
	all := s.store.Providers()
	u, ok := s.currentUser(r)
	if !ok || u.IsAdmin || !s.authEnabled() {
		return all
	}
	if len(u.ProviderIds) == 0 {
		return nil
	}
	want := make(map[string]bool, len(u.ProviderIds))
	for _, id := range u.ProviderIds {
		want[id] = true
	}
	out := make([]*config.Provider, 0, len(all))
	for _, p := range all {
		if want[p.Id] || want["*"] {
			out = append(out, p)
		}
	}
	return out
}

func (s *Server) allowListed(ip string) bool {
	return networkAllows(splitCSV(s.opts.Allow), ip)
}

func networkAllows(list []string, ip string) bool {
	if len(list) == 0 {
		return false
	}
	target := net.ParseIP(ip)
	for _, raw := range list {
		if raw == ip {
			return true
		}
		if _, cidr, err := net.ParseCIDR(raw); err == nil && target != nil && cidr.Contains(target) {
			return true
		}
	}
	return false
}

func splitCSV(s string) []string {
	out := make([]string, 0, 4)
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func pathToken(r *http.Request) (string, bool) {
	if v := r.Header.Get("X-Drmsonic-Token"); v != "" {
		return v, true
	}
	return "", false
}

func pathUserPass(r *http.Request) (string, string, bool) {
	u := r.Header.Get("X-Drmsonic-User")
	p := r.Header.Get("X-Drmsonic-Pass")
	if u != "" && p != "" {
		return u, p, true
	}
	return "", "", false
}

var reservedPathHeads = map[string]bool{
	"static": true, "api": true, "login": true, "logout": true,
	"providers": true, "status": true, "events": true, "config": true,
	"users": true, "rec": true, "records": true, "monitoring": true,
	"help": true, "debug": true, "stream": true, "t": true,
	"playlist.m3u": true, "live.m3u": true, "event.m3u": true, "replay.m3u": true,
	"app1.conf": true,
}

func isUserPassPath(parts []string) bool {
	if len(parts) < 3 || reservedPathHeads[parts[0]] {
		return false
	}
	switch parts[2] {
	case "stream", "app1.conf", "playlist.m3u", "live.m3u", "event.m3u", "replay.m3u":
		return true
	default:
		return false
	}
}

func clientIP(r *http.Request, trustedProxies string) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if trustedProxies == "" || !networkAllows(splitCSV(trustedProxies), host) {
		return host
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	return host
}

func remoteIP(r *http.Request) string {
	return clientIP(r, "")
}

func tokenPrefix(token string) string {
	if token == "" {
		return ""
	}
	return "/t/" + url.PathEscape(token)
}

func deriveStaticToken(user, pass, secret string) string {
	if user == "" || pass == "" {
		return ""
	}
	key := []byte(secret)
	if len(key) == 0 {
		key = []byte("drmsonic-static-token")
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(user))
	mac.Write([]byte{0})
	mac.Write([]byte(pass))
	return hex.EncodeToString(mac.Sum(nil)[:24])
}

func (s *Server) pathAuthRewrite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := splitPath(r.URL.EscapedPath())
		if len(parts) >= 3 && parts[0] == "t" {
			tok, err := url.PathUnescape(parts[1])
			if err == nil && tok != "" {
				rest := "/" + strings.Join(parts[2:], "/")
				r2 := r.Clone(r.Context())
				r2.URL.Path = rest
				r2.Header.Set("X-Drmsonic-Token", tok)
				next.ServeHTTP(w, r2)
				return
			}
		}
		if isUserPassPath(parts) {
			user, err1 := url.PathUnescape(parts[0])
			pass, err2 := url.PathUnescape(parts[1])
			if err1 == nil && err2 == nil && user != "" && pass != "" {
				rest := "/" + strings.Join(parts[2:], "/")
				r2 := r.Clone(r.Context())
				r2.URL.Path = rest
				r2.Header.Set("X-Drmsonic-User", user)
				r2.Header.Set("X-Drmsonic-Pass", pass)
				next.ServeHTTP(w, r2)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) csrfOK(r *http.Request) bool {
	if !s.authEnabled() || sameOrigin(r) {
		return true
	}
	if r.Header.Get("Authorization") != "" || r.Header.Get("X-Drmsonic-Token") != "" {
		return true
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		if _, ok := s.sessions.get(c.Value); ok {
			return true
		}
	}
	return false
}

func userPassPrefix(user, pass string) string {
	if user == "" || pass == "" {
		return ""
	}
	return "/" + url.PathEscape(user) + "/" + url.PathEscape(pass)
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdmin(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (s *Server) requirePanel(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.canUsePanel(r) {
			next(w, r)
			return
		}
		if wantsHTML(r) {
			http.Redirect(w, r, "/logout", http.StatusFound)
			return
		}
		http.Error(w, "forbidden", http.StatusForbidden)
	}
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	referer := r.Header.Get("Referer")
	host := r.Host
	if origin != "" {
		u, err := url.Parse(origin)
		return err == nil && u.Host == host
	}
	if referer != "" {
		u, err := url.Parse(referer)
		return err == nil && u.Host == host
	}
	return false
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.authEnabled() {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if r.Method == http.MethodGet {
		s.renderLogin(w, "")
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ip := clientIP(r, s.opts.TrustedProxies)
	_ = r.ParseForm()
	user := strings.TrimSpace(r.FormValue("username"))
	pass := r.FormValue("password")
	u, ok := s.authenticate(user, pass, ip)
	if !ok {
		if s.throttle.blocked(ip) {
			s.renderLogin(w, "Too many attempts. Try again later.")
			return
		}
		s.throttle.record(ip)
		log.Printf("login failed for %q from %s", user, ip)
		s.renderLogin(w, "Invalid username or password.")
		return
	}
	if !u.HasWebAccess && !u.IsAdmin {
		s.renderLogin(w, "This account is for playlist access only.")
		return
	}
	s.throttle.clear(ip)
	id, exp := s.sessions.create(u.Username)
	setSessionCookie(w, id, exp)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.sessions.revoke(c.Value)
	}
	clearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) renderLogin(w http.ResponseWriter, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.tmpl.ExecuteTemplate(w, "login.html", map[string]string{"Error": errMsg})
}
