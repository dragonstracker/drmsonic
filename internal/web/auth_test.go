package web

import (
	"encoding/json"
	"io"
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
	if strings.Contains(body, "/admin/") {
		t.Fatal("credential prefix leaked onto open server")
	}
}

func TestStaticCredentials(t *testing.T) {
	s := testServer(t, Options{User: "admin", Password: "secret"}, nil, sampleProvider("channels"))
	h := s.Handler()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/providers", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/providers", nil)
	req.SetBasicAuth("admin", "wrong")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/providers", nil)
	req.SetBasicAuth("admin", "secret")
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("basic auth want 200 got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/admin/secret/app1.conf", nil)
	req.Host = "vps.example:7779"
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("path auth want 200 got %d body %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "hls://vps.example:7779/admin/secret/stream/channels/channel1/master.m3u8") {
		t.Fatalf("path prefix missing: %s", rr.Body.String())
	}
}

func TestEmptyPasswordCannotLogin(t *testing.T) {
	users := []config.User{{Username: "guest", Password: "", HasWebAccess: true, IsAdmin: false}}
	s := testServer(t, Options{User: "admin", Password: "secret"}, users, sampleProvider("channels"))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/providers", nil)
	req.SetBasicAuth("guest", "")
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rr.Code)
	}
}

func TestPasswordlessUsersDoNotEnableAuth(t *testing.T) {
	users := []config.User{{Username: "draft", Password: "", HasWebAccess: true}}
	s := testServer(t, Options{}, users, sampleProvider("channels"))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/providers", nil)
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("want open panel got %d", rr.Code)
	}
}

func TestProviderScope(t *testing.T) {
	users := []config.User{{
		Username: "viewer", Password: "pw", HasWebAccess: true, IsAdmin: false,
		ProviderIds: []string{"channels"},
	}}
	s := testServer(t, Options{}, users, sampleProvider("channels"), sampleProvider("hidden"))
	h := s.Handler()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/viewer/pw/app1.conf", nil)
	req.Host = "host:7779"
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "channels_channel1") {
		t.Fatal("expected granted provider")
	}
	if strings.Contains(body, "hidden_channel1") {
		t.Fatal("leaked ungranted provider")
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/viewer/pw/stream/hidden/channel1/master.m3u8", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", rr.Code)
	}
}

func TestUpdateUserKeepsPassword(t *testing.T) {
	users := []config.User{{Username: "bob", Password: "keepme", HasWebAccess: true, IsAdmin: true}}
	s := testServer(t, Options{User: "admin", Password: "secret"}, users, sampleProvider("channels"))
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

func TestCSRFRejectsPlainText(t *testing.T) {
	s := testServer(t, Options{User: "admin", Password: "secret"}, nil, sampleProvider("channels"))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/action", strings.NewReader(`{"Action":"getstatus"}`))
	req.Header.Set("Content-Type", "text/plain")
	req.SetBasicAuth("admin", "secret")
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("want 415 got %d", rr.Code)
	}
}

func TestAdminOnlyUsersPage(t *testing.T) {
	users := []config.User{{Username: "viewer", Password: "pw", HasWebAccess: true, IsAdmin: false}}
	s := testServer(t, Options{}, users, sampleProvider("channels"))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.SetBasicAuth("viewer", "pw")
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d", rr.Code)
	}
}

func TestSpecialCredentialChars(t *testing.T) {
	pass := "p@ss/w rd%"
	users := []config.User{{Username: "u1", Password: pass, HasWebAccess: true, IsAdmin: true, ProviderIds: []string{"*"}}}
	s := testServer(t, Options{}, users, sampleProvider("channels"))
	prefix := credPrefix("u1", pass)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, prefix+"/app1.conf", nil)
	req.Host = "h:7779"
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		b, _ := io.ReadAll(rr.Body)
		t.Fatalf("status %d body %s", rr.Code, b)
	}
	var probe map[string]any
	_ = json.Unmarshal([]byte("{}"), &probe)
	if !strings.Contains(rr.Body.String(), prefix+"/stream/") {
		t.Fatalf("escaped prefix missing in %s", rr.Body.String())
	}
}
