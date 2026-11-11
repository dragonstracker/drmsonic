package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"drmsonic/internal/config"
)

func loginUser(t *testing.T, s *Server, user, pass string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"username": {user}, "password": {pass}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.Handler().ServeHTTP(rr, req)
	return rr
}

func TestLoginAcceptsSeededAdmin(t *testing.T) {
	prov := &config.Provider{Id: "channels", Name: "channels"}
	s := testServer(t, Options{}, nil, prov)

	rr := loginUser(t, s, "admin", "admin")
	if rr.Code != http.StatusFound || rr.Header().Get("Location") != "/" {
		t.Fatalf("login status %d location %q, want 302 /", rr.Code, rr.Header().Get("Location"))
	}
}

func TestCorrectPasswordAlwaysLogsIn(t *testing.T) {
	prov := &config.Provider{Id: "channels", Name: "channels"}
	s := testServer(t, Options{}, nil, prov)

	for i := 0; i < 6; i++ {
		loginUser(t, s, "admin", "wrong")
	}

	rr := loginUser(t, s, "admin", "admin")
	if rr.Code != http.StatusFound || rr.Header().Get("Location") != "/" {
		t.Fatalf("correct password refused while throttled: status %d body %q", rr.Code, rr.Body.String())
	}
}

func TestThrottleDecaysAfterQuietPeriod(t *testing.T) {
	th := newLoginThrottle()

	for i := 0; i < 6; i++ {
		th.record("10.0.0.1")
	}
	if !th.blocked("10.0.0.1") {
		t.Fatal("expected the address to be blocked")
	}

	th.mu.Lock()
	f := th.fail["10.0.0.1"]
	f.Last = time.Now().Add(-2 * throttleReset)
	th.fail["10.0.0.1"] = f
	th.mu.Unlock()

	if th.blocked("10.0.0.1") {
		t.Fatal("stale failure count still blocks")
	}
	th.mu.Lock()
	_, kept := th.fail["10.0.0.1"]
	th.mu.Unlock()
	if kept {
		t.Fatal("stale entry was not dropped")
	}
}

func TestThrottleDelayIsBounded(t *testing.T) {
	th := newLoginThrottle()
	for i := 0; i < 40; i++ {
		th.record("10.0.0.2")
	}
	th.mu.Lock()
	until := th.fail["10.0.0.2"].Until
	th.mu.Unlock()
	if wait := time.Until(until); wait > throttleMaxDelay {
		t.Fatalf("lockout %s exceeds cap %s", wait, throttleMaxDelay)
	}
}

func TestFirstFailureLocksBriefly(t *testing.T) {
	th := newLoginThrottle()
	th.record("10.0.0.3")
	th.mu.Lock()
	until := th.fail["10.0.0.3"].Until
	th.mu.Unlock()
	if wait := time.Until(until); wait > 2*time.Second {
		t.Fatalf("first failure locked out for %s", wait)
	}
}
