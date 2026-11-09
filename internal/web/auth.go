package web

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"

	"drmsonic/internal/config"
)

func (s *Server) authWrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.authorized(r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="drmsonic"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

func (s *Server) authorized(r *http.Request) bool {
	if s.opts.User == "" && s.opts.Password == "" {
		return true
	}
	if s.allowListed(remoteIP(r)) {
		return true
	}
	if u, p, ok := r.BasicAuth(); ok {
		if s.checkStatic(u, p) {
			return true
		}
		if s.checkStoreUser(u, p) {
			return true
		}
	}
	if tok := r.URL.Query().Get("user"); tok != "" {
		if _, ok := s.storeUser(tok); ok {
			return true
		}
	}
	return false
}

func (s *Server) checkStatic(user, pass string) bool {
	if s.opts.User == "" {
		return false
	}
	u := subtle.ConstantTimeCompare([]byte(user), []byte(s.opts.User)) == 1
	p := subtle.ConstantTimeCompare([]byte(pass), []byte(s.opts.Password)) == 1
	return u && p
}

func (s *Server) storeUser(name string) (config.User, bool) {
	for _, u := range s.store.Users() {
		if u.Username == name && u.HasWebAccess {
			return u, true
		}
	}
	return config.User{}, false
}

func (s *Server) checkStoreUser(name, pass string) bool {
	u, ok := s.storeUser(name)
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(pass), []byte(u.Password)) == 1
}

func (s *Server) currentUser(r *http.Request) (config.User, bool) {
	if u, p, ok := r.BasicAuth(); ok {
		if s.checkStatic(u, p) {
			return config.User{Username: u, IsAdmin: true, HasWebAccess: true}, true
		}
		if s.checkStoreUser(u, p) {
			if su, ok := s.storeUser(u); ok {
				return su, true
			}
		}
	}
	if tok := r.URL.Query().Get("user"); tok != "" {
		if u, ok := s.storeUser(tok); ok {
			return u, true
		}
	}
	if s.opts.User == "" && s.opts.Password == "" {
		return config.User{Username: "no-auth", IsAdmin: true, HasWebAccess: true}, true
	}
	if s.allowListed(remoteIP(r)) {
		return config.User{Username: "no-auth", IsAdmin: true, HasWebAccess: true}, true
	}
	return config.User{}, false
}

func (s *Server) allowListed(ip string) bool {
	if s.opts.Allow == "" {
		return false
	}
	target := net.ParseIP(ip)
	for _, raw := range strings.Split(s.opts.Allow, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if raw == ip {
			return true
		}
		if _, cidr, err := net.ParseCIDR(raw); err == nil && target != nil && cidr.Contains(target) {
			return true
		}
	}
	return false
}

func (s *Server) isAdmin(r *http.Request) bool {
	u, ok := s.currentUser(r)
	return ok && u.IsAdmin
}
