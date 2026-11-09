package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"syscall"

	"drmsonic/internal/fetch"
)

var ErrBroadcastEnded = errors.New("Broadcast ended")

func Describe(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, ErrBroadcastEnded):
		return ErrBroadcastEnded.Error()
	case errors.Is(err, ErrMissingKey):
		return ErrMissingKey.Error()
	case errors.Is(err, context.Canceled):
		return "Cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "Connection timeout"
	}

	var se *fetch.StatusError
	if errors.As(err, &se) {
		return httpReason(se.Code)
	}
	var ue *url.Error
	if errors.As(err, &ue) && ue.Err != nil {
		return Describe(ue.Err)
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "Connection timeout"
	}

	switch {
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "Connection closed"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "Connection refused"
	case errors.Is(err, syscall.ECONNRESET):
		return "Connection reset"
	case errors.Is(err, syscall.EHOSTUNREACH):
		return "Host unreachable"
	case errors.Is(err, syscall.ENETUNREACH):
		return "Network unreachable"
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "no such host"):
		return "Host not found"
	case strings.Contains(msg, "connection refused"):
		return "Connection refused"
	case strings.Contains(msg, "connection reset"):
		return "Connection reset"
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "deadline exceeded"):
		return "Connection timeout"
	case strings.Contains(msg, "tls:"), strings.Contains(msg, "x509:"), strings.Contains(msg, "handshake"):
		return "TLS handshake failed"
	case strings.Contains(msg, "invalid character"), strings.Contains(msg, "XML syntax"),
		strings.Contains(msg, "not a playlist"), strings.Contains(msg, "no periods"):
		return "Invalid manifest"
	case strings.Contains(msg, "blocked address"):
		return "Address blocked"
	}
	return capitalize(msg)
}

func capitalize(s string) string {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return s
	}
	return string(s[0]-'a'+'A') + s[1:]
}

func endedStatus(err error) bool {
	var se *fetch.StatusError
	return errors.As(err, &se) && (se.Code == 404 || se.Code == 410)
}

func httpReason(code int) string {
	switch code {
	case 400:
		return "Bad request (400)"
	case 401:
		return "Unauthorized (401)"
	case 403:
		return "Access denied (403)"
	case 404:
		return "Not found (404)"
	case 410:
		return "Gone (410)"
	case 429:
		return "Too many requests (429)"
	}
	if code >= 500 {
		return fmt.Sprintf("Server error (%d)", code)
	}
	return fmt.Sprintf("HTTP error (%d)", code)
}
