package web

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const (
	sessionCookie = "drmsonic_session"
	sessionTTL    = 12 * time.Hour
)

type sessionEntry struct {
	Username string
	Expires  time.Time
}

type sessionStore struct {
	mu   sync.Mutex
	byID map[string]sessionEntry
}

func newSessionStore() *sessionStore {
	return &sessionStore{byID: make(map[string]sessionEntry)}
}

func (s *sessionStore) create(username string) (string, time.Time) {
	var b [32]byte
	_, _ = rand.Read(b[:])
	id := hex.EncodeToString(b[:])
	exp := time.Now().Add(sessionTTL)
	s.mu.Lock()
	s.byID[id] = sessionEntry{Username: username, Expires: exp}
	s.mu.Unlock()
	return id, exp
}

func (s *sessionStore) get(id string) (string, bool) {
	if id == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.byID[id]
	if !ok || time.Now().After(e.Expires) {
		delete(s.byID, id)
		return "", false
	}
	return e.Username, true
}

func (s *sessionStore) revoke(id string) {
	s.mu.Lock()
	delete(s.byID, id)
	s.mu.Unlock()
}

type loginThrottle struct {
	mu   sync.Mutex
	fail map[string]failInfo
}

type failInfo struct {
	Count int
	Until time.Time
}

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{fail: make(map[string]failInfo)}
}

func (t *loginThrottle) blocked(ip string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	f, ok := t.fail[ip]
	if !ok {
		return false
	}
	return time.Now().Before(f.Until)
}

func (t *loginThrottle) record(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	f := t.fail[ip]
	f.Count++
	delay := time.Duration(1<<min(f.Count, 6)) * time.Second
	f.Until = time.Now().Add(delay)
	t.fail[ip] = f
}

func (t *loginThrottle) clear(ip string) {
	t.mu.Lock()
	delete(t.fail, ip)
	t.mu.Unlock()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func setSessionCookie(w http.ResponseWriter, id string, exp time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		Expires:  exp,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
