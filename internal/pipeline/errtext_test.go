package pipeline

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"

	"drmsonic/internal/fetch"
)

func TestDescribe(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"broadcast ended", ErrBroadcastEnded, "Broadcast ended"},
		{"missing key", ErrMissingKey, "Key(s) missing"},
		{"forbidden", &fetch.StatusError{Code: 403, URL: "https://x"}, "Access denied (403)"},
		{"not found", &fetch.StatusError{Code: 404, URL: "https://x"}, "Not found (404)"},
		{"server error", &fetch.StatusError{Code: 503, URL: "https://x"}, "Server error (503)"},
		{"timeout", context.DeadlineExceeded, "Connection timeout"},
		{"url wrap", &url.Error{Op: "Get", URL: "https://x", Err: &fetch.StatusError{Code: 403}}, "Access denied (403)"},
		{"unknown", errors.New("empty manifest url"), "Empty manifest url"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Describe(tc.err); got != tc.want {
				t.Fatalf("Describe(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

func TestEndedStatus(t *testing.T) {
	for _, code := range []int{404, 410} {
		if !endedStatus(&fetch.StatusError{Code: code}) {
			t.Fatalf("http %d should end a broadcast", code)
		}
	}
	if endedStatus(fmt.Errorf("http 403")) {
		t.Fatal("403 is an error, not a finished broadcast")
	}
}
