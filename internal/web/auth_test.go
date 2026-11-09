package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"drmsonic/internal/config"
	"drmsonic/internal/state"
)

func testServer(t *testing.T, opts Options, users []config.User, providers ...*config.Provider) *Server {
	t.Helper()
	dir := t.TempDir()
	provDir := filepath.Join(dir, "providers")
	if err := os.MkdirAll(provDir, 0o755); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(dir, "drmsonic.json")
	store, err := config.NewStore(provDir, conf)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range providers {
		if err := store.Put(p); err != nil {
			t.Fatal(err)
		}
	}
	if users != nil {
		store.SetUsers(users)
		if err := store.SaveUsers(); err != nil {
			t.Fatal(err)
		}
	}
	opts.Port = 7779
	if opts.Providers == "" {
		opts.Providers = provDir
	}
	srv, err := New(opts, store, state.NewManager())
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func sampleProvider(id string) *config.Provider {
	return &config.Provider{
		Id:          id,
		Name:        id,
		RunningMode: config.RunInternalRemuxer,
		OutputMode:  config.OutDirectHLS,
		Streams: []config.Stream{{
			Id:   "channel1",
			Name: "channel1",
			Mode: config.ModeLive,
		}},
	}
}

func TestOpenServerPlainURLs(t *testing.T) {
	s := testServer(t, Options{}, nil, sampleProvider("channels"))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/app1.conf", nil)
	req.Host = "example.test:7779"
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "hls://example.test:7779/stream/channels/channel1/master.m3u8") {
		t.Fatalf("unexpected body: %s", body)
	}
	if strings.Contains(body, "/t/") {
		t.Fatal("token prefix leaked onto open server")
	}
}

func TestLoginAndTokenPlaylist(t *testing.T) {
	tok := config.NewStreamToken()
	users := []config.User{{
		Username: "viewer", Password: "pw", StreamToken: tok,
		HasWebAccess: true, IsAdmin: false, ProviderIds: []string{"channels"},
	}}
	s := testServer(t, Options{SessionSecret: "test"}, users, sampleProvider("channels"), sampleProvider("hidden"))
	h := s.Handler()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/providers", nil)
	req.Header.Set("Accept", "text/html")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound || rr.Header().Get("Location") != "/login" {
		t.Fatalf("want redirect login got %d %s", rr.Code, rr.Header().Get("Location"))
	}

	rr = httptest.NewRecorder()
	form := strings.NewReader("username=viewer&password=pw")
	req = httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("login status %d", rr.Code)
	}
	cookie := rr.Result().Cookies()
	if len(cookie) == 0 {
		t.Fatal("missing session cookie")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/t/"+tok+"/app1.conf", nil)
	req.Host = "host:7779"
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("token playlist %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "/viewer/pw/stream/channels/channel1/master.m3u8") {
		t.Fatalf("want user/pass stream urls got %s", body)
	}
	if strings.Contains(body, "hidden_channel1") {
		t.Fatal("ungranted provider leaked")
	}
}

func TestEmptyPasswordCannotLogin(t *testing.T) {
	users := []config.User{{Username: "guest", Password: "", HasWebAccess: true}}
	s := testServer(t, Options{User: "admin", Password: "secret", SessionSecret: "x"}, users, sampleProvider("channels"))
	rr := httptest.NewRecorder()
	form := strings.NewReader("username=guest&password=")
	req := httptest.NewRequest(http.MethodPost, "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "Invalid") {
		t.Fatalf("want login error page got %d", rr.Code)
	}
}

func TestUserPassPlaylistPath(t *testing.T) {
	tok := config.NewStreamToken()
	users := []config.User{{Username: "viewer", Password: "secret", StreamToken: tok, HasWebAccess: true, ProviderIds: []string{"channels"}}}
	s := testServer(t, Options{SessionSecret: "x"}, users, sampleProvider("channels"))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/viewer/secret/app1.conf", nil)
	req.Host = "host:7779"
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("user/pass app1 status %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "/viewer/secret/stream/channels/channel1/master.m3u8") {
		t.Fatalf("want user/pass stream urls got %s", body)
	}
}

func TestCSRFRejectsPlainText(t *testing.T) {
	s := testServer(t, Options{User: "admin", Password: "secret", SessionSecret: "x"}, nil, sampleProvider("channels"))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/action", strings.NewReader(`{"Action":"getstatus"}`))
	req.Header.Set("Content-Type", "text/plain")
	req.SetBasicAuth("admin", "secret")
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("want 415 got %d", rr.Code)
	}
}

func TestUpdateUserKeepsPassword(t *testing.T) {
	tok := config.NewStreamToken()
	users := []config.User{{Username: "bob", Password: "keepme", StreamToken: tok, HasWebAccess: true, IsAdmin: true}}
	s := testServer(t, Options{User: "admin", Password: "secret", SessionSecret: "x"}, users, sampleProvider("channels"))
	body := `{"Action":"updateuser","Users":[{"Username":"bob","Password":"","Network":"","IsAdmin":true,"HasWebAccess":true,"ProviderIds":[]}]}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://example.test")
	req.Host = "example.test"
	req.SetBasicAuth("admin", "secret")
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	got := s.store.Users()
	if len(got) != 1 || got[0].Password != "keepme" {
		t.Fatalf("password changed: %+v", got)
	}
}

func TestSaveUsersOnly(t *testing.T) {
	tok := config.NewStreamToken()
	users := []config.User{{Username: "bob", Password: "x", StreamToken: tok, HasWebAccess: true, IsAdmin: true}}
	s := testServer(t, Options{User: "admin", Password: "secret", SessionSecret: "x"}, users, sampleProvider("channels"))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/action", strings.NewReader(`{"Action":"save","ProviderId":"users_only"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://example.test")
	req.Host = "example.test"
	req.SetBasicAuth("admin", "secret")
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}

func TestSaveUsersAction(t *testing.T) {
	tok := config.NewStreamToken()
	users := []config.User{{Username: "bob", Password: "old", StreamToken: tok, HasWebAccess: true, IsAdmin: true, ProviderIds: []string{"channels"}}}
	s := testServer(t, Options{User: "admin", Password: "secret", SessionSecret: "x"}, users, sampleProvider("channels"))
	body := `{"Action":"saveusers","NoAuth":true,"Users":[{"Username":"bob","Password":"newpass","Network":"10.0.0.0/8","IsAdmin":true,"HasWebAccess":true,"ProviderIds":["channels"]}]}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://example.test")
	req.Host = "example.test"
	req.SetBasicAuth("admin", "secret")
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	got := s.store.Users()
	if len(got) != 1 || got[0].Password != "newpass" || got[0].Network != "10.0.0.0/8" {
		t.Fatalf("users: %+v", got)
	}
	if !s.store.NoAuth() {
		t.Fatal("NoAuth not set")
	}
	if err := s.store.Load(); err != nil {
		t.Fatal(err)
	}
	if !s.store.NoAuth() {
		t.Fatal("NoAuth lost after reload")
	}
	got = s.store.Users()
	if len(got) != 1 || got[0].Password != "newpass" {
		t.Fatalf("password lost after reload: %+v", got)
	}
}

func TestRenameUserNewUserName(t *testing.T) {
	tok := config.NewStreamToken()
	users := []config.User{{Username: "bob", Password: "x", StreamToken: tok, HasWebAccess: true, IsAdmin: true}}
	s := testServer(t, Options{User: "admin", Password: "secret", SessionSecret: "x"}, users, sampleProvider("channels"))
	body := `{"Action":"renameuser","Users":[{"Username":"bob"}],"NewUserName":"alice"}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://example.test")
	req.Host = "example.test"
	req.SetBasicAuth("admin", "secret")
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	got := s.store.Users()
	if len(got) != 1 || got[0].Username != "alice" {
		t.Fatalf("rename failed: %+v", got)
	}
}

func TestAddUserDefaultsWebAccess(t *testing.T) {
	s := testServer(t, Options{User: "admin", Password: "secret", SessionSecret: "x"}, nil, sampleProvider("channels"))
	body := `{"Action":"adduser","Users":[{"Username":"viewer"}]}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/action", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://example.test")
	req.Host = "example.test"
	req.SetBasicAuth("admin", "secret")
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	got := s.store.Users()
	if len(got) != 1 || !got[0].HasWebAccess || got[0].IsAdmin {
		t.Fatalf("defaults: %+v", got)
	}
}
