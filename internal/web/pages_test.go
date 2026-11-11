package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"drmsonic/internal/config"
)

func statusPage(t *testing.T, target string, providers ...*config.Provider) string {
	t.Helper()
	s := testServer(t, Options{}, nil, providers...)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Host = "example.test:7779"
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("%s: status %d", target, rr.Code)
	}
	return rr.Body.String()
}

func TestStatusPageShowsOnlySelectedProvider(t *testing.T) {
	body := statusPage(t, "/status/example", sampleProvider("channels"), sampleProvider("example"))

	if strings.Contains(body, "name_channels~") {
		t.Fatal("other provider rows leaked into the page")
	}
	if !strings.Contains(body, "name_example~channel1") {
		t.Fatal("selected provider rows are missing")
	}
	if !strings.Contains(body, "1 stream(s)") {
		t.Fatal("stream total does not match the selected provider")
	}
}

func TestStatusPageAllRunningKeepsEveryProvider(t *testing.T) {
	body := statusPage(t, "/status/example?filter=allrunning", sampleProvider("channels"), sampleProvider("example"))

	if !strings.Contains(body, "name_channels~channel1") || !strings.Contains(body, "name_example~channel1") {
		t.Fatal("all running should span every provider")
	}
	if !strings.Contains(body, "2 stream(s)") {
		t.Fatal("all running should total every provider")
	}
}

func TestStatusPageKeepsProviderPicker(t *testing.T) {
	body := statusPage(t, "/status/example", sampleProvider("channels"), sampleProvider("example"))

	if !strings.Contains(body, `<option value="channels">channels</option>`) {
		t.Fatal("provider picker lost the other provider")
	}
	if !strings.Contains(body, `<option value="example">example</option>`) {
		t.Fatal("provider picker lost the selected provider")
	}
}
