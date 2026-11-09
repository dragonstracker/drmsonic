package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"drmsonic/internal/config"
)

const legacyProvider = `{
	"Name": "legacy",
	"Id": "legacy",
	"RunningMode": "internalremuxer",
	"OutputMode": "directhls",
	"XForwardedFor": "",
	"Headers": null,
	"Channels": [
		{
			"Name": "beiN SPORTS 1 FHD",
			"Id": "beinsports1fhd",
			"Mode": "live",
			"OnDemand": true,
			"Manifest": "https://example.test/cenc.mpd",
			"ManifestType": "dash",
			"Video": "best",
			"Audio": "7",
			"Keys": ["335dad778109954503dcbb21dc92015f:24bfd75d436cbf73168a2a2dccd40281"],
			"Headers": {
				"Manifest": null,
				"Media": null
			}
		}
	]
}`

func importRequest(t *testing.T, s *Server, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/action", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://example.test")
	req.Host = "example.test"
	req.SetBasicAuth("admin", "secret")
	s.Handler().ServeHTTP(rr, req)
	return rr
}

func TestImportLegacyProviderFile(t *testing.T) {
	s := testServer(t, Options{User: "admin", Password: "secret", SessionSecret: "x"}, nil, sampleProvider("channels"))

	rr := importRequest(t, s, map[string]any{
		"Action":               "import",
		"ProviderImportConfig": legacyProvider,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}

	p := s.store.Provider("legacy")
	if p == nil {
		t.Fatal("provider not stored")
	}
	if len(p.Streams) != 1 {
		t.Fatalf("streams: %d", len(p.Streams))
	}
	st := p.Streams[0]
	if st.Id != "beinsports1fhd" || st.Manifest != "https://example.test/cenc.mpd" {
		t.Fatalf("stream: %+v", st)
	}
	if len(st.Keys) != 1 {
		t.Fatalf("keys: %v", st.Keys)
	}
	if st.OnDemand != true || st.Mode != config.ModeLive {
		t.Fatalf("flags: %+v", st)
	}

	if err := s.store.Load(); err != nil {
		t.Fatal(err)
	}
	if reloaded := s.store.Provider("legacy"); reloaded == nil || len(reloaded.Streams) != 1 {
		t.Fatal("provider lost after reload")
	}
}

func TestImportUsesTypedProviderName(t *testing.T) {
	s := testServer(t, Options{User: "admin", Password: "secret", SessionSecret: "x"}, nil, sampleProvider("channels"))

	rr := importRequest(t, s, map[string]any{
		"Action":               "import",
		"ProviderName":         "my channels",
		"ProviderImportConfig": legacyProvider,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	if p := s.store.Provider("my_channels"); p == nil || p.Name != "my channels" {
		t.Fatalf("provider: %+v", p)
	}
}

func TestImportRoundTripsExport(t *testing.T) {
	s := testServer(t, Options{User: "admin", Password: "secret", SessionSecret: "x"}, nil, sampleProvider("channels"))
	src := s.store.Provider("channels")
	src.Streams[0].Headers = []string{"Cookie: a=b"}
	src.XForwardedFor = true
	src.Streams[0].VideoList = []config.Track{{Id: "1", Desc: "576p", Codecs: "avc1.64001F", Width: 1024, Height: 576, Bandwidth: 1400000}}

	body := providerToMap(src, true)
	body["Name"] = "copy"
	body["Id"] = "copy"
	body["Streams"] = []map[string]any{streamToMap(&src.Streams[0])}

	rr := importRequest(t, s, map[string]any{"Action": "import", "Body": body})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}

	p := s.store.Provider("copy")
	if p == nil {
		t.Fatal("provider not stored")
	}
	if !p.XForwardedFor {
		t.Fatal("XForwardedFor lost")
	}
	if len(p.Streams) != 1 || len(p.Streams[0].Headers) != 1 || p.Streams[0].Headers[0] != "Cookie: a=b" {
		t.Fatalf("headers: %+v", p.Streams)
	}
	if len(p.Streams[0].VideoList) != 1 || p.Streams[0].VideoList[0].Width != 1024 {
		t.Fatalf("tracks: %+v", p.Streams[0].VideoList)
	}
}

func TestImportRejectsGarbage(t *testing.T) {
	s := testServer(t, Options{User: "admin", Password: "secret", SessionSecret: "x"}, nil, sampleProvider("channels"))

	rr := importRequest(t, s, map[string]any{
		"Action":               "import",
		"ProviderImportConfig": "not json",
	})
	if rr.Code == http.StatusOK {
		t.Fatalf("garbage accepted: %s", rr.Body.String())
	}
}
