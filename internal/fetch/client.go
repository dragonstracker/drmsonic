package fetch

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Options struct {
	UserAgent string
	Bind      string
	Proxy     string
	Doh       string
	Headers   []string
	Retries   int
	Timeout   time.Duration
}

type Client struct {
	http    *http.Client
	opts    Options
	headers map[string]string

	mu    sync.Mutex
	cache map[string]dnsEntry
}

type dnsEntry struct {
	addrs []net.IP
	at    time.Time
}

func New(opts Options) *Client {
	c := &Client{
		opts:    opts,
		headers: parseHeaders(opts.Headers),
		cache:   make(map[string]dnsEntry),
	}
	if opts.Timeout == 0 {
		opts.Timeout = 20 * time.Second
		c.opts.Timeout = opts.Timeout
	}
	if opts.Retries <= 0 {
		c.opts.Retries = 2
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	if opts.Bind != "" {
		if ip := net.ParseIP(opts.Bind); ip != nil {
			dialer.LocalAddr = &net.TCPAddr{IP: ip}
		}
	}

	tr := &http.Transport{
		DialContext:         c.dialContext(dialer),
		MaxIdleConns:        32,
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     60 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	if opts.Proxy != "" {
		if pu, err := url.Parse(opts.Proxy); err == nil {
			tr.Proxy = http.ProxyURL(pu)
		}
	}

	c.http = &http.Client{Transport: tr, Timeout: opts.Timeout}
	return c
}

func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var last error
	for attempt := 0; attempt <= c.opts.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 300 * time.Millisecond):
			}
		}
		body, err := c.get(ctx, rawURL, "")
		if err == nil {
			return body, nil
		}
		last = err
	}
	return nil, last
}

func (c *Client) GetRange(ctx context.Context, rawURL, byteRange string) ([]byte, error) {
	return c.get(ctx, rawURL, byteRange)
}

func (c *Client) get(ctx context.Context, rawURL, byteRange string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	ua := c.opts.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "*/*")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	if byteRange != "" {
		req.Header.Set("Range", "bytes="+byteRange)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("http %d for %s", resp.StatusCode, rawURL)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (c *Client) dialContext(dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	if c.opts.Doh == "" {
		return dialer.DialContext
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return dialer.DialContext(ctx, network, addr)
		}
		if ip := net.ParseIP(host); ip != nil {
			return dialer.DialContext(ctx, network, addr)
		}
		ips, err := c.resolve(ctx, host)
		if err != nil || len(ips) == 0 {
			return dialer.DialContext(ctx, network, addr)
		}
		var lastErr error
		for _, ip := range ips {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
}

func (c *Client) resolve(ctx context.Context, host string) ([]net.IP, error) {
	c.mu.Lock()
	if e, ok := c.cache[host]; ok && time.Since(e.at) < 5*time.Minute {
		c.mu.Unlock()
		return e.addrs, nil
	}
	c.mu.Unlock()

	ips, err := c.dohQuery(ctx, host)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.cache[host] = dnsEntry{addrs: ips, at: time.Now()}
	c.mu.Unlock()
	return ips, nil
}

func (c *Client) dohQuery(ctx context.Context, host string) ([]net.IP, error) {
	msg := buildQuery(host)
	endpoint := c.opts.Doh
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	if !strings.Contains(endpoint, "/") {
		endpoint += "/dns-query"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(msg))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("doh: %s", resp.Status)
	}
	body, err := io.ReadAll(http.MaxBytesReader(nil, resp.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	return parseAnswers(body), nil
}

func buildQuery(host string) []byte {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint16(buf[0:], 0)
	binary.BigEndian.PutUint16(buf[2:], 0x0100)
	binary.BigEndian.PutUint16(buf[4:], 1)
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		buf = append(buf, byte(len(label)))
		buf = append(buf, label...)
	}
	buf = append(buf, 0x00, 0x00, 0x01, 0x00, 0x01)
	return buf
}

func parseAnswers(msg []byte) []net.IP {
	if len(msg) < 12 {
		return nil
	}
	qd := int(binary.BigEndian.Uint16(msg[4:]))
	an := int(binary.BigEndian.Uint16(msg[6:]))
	off := 12
	for i := 0; i < qd; i++ {
		off = skipName(msg, off)
		if off < 0 || off+4 > len(msg) {
			return nil
		}
		off += 4
	}

	var out []net.IP
	for i := 0; i < an; i++ {
		off = skipName(msg, off)
		if off < 0 || off+10 > len(msg) {
			break
		}
		typ := binary.BigEndian.Uint16(msg[off:])
		class := binary.BigEndian.Uint16(msg[off+2:])
		rdlen := int(binary.BigEndian.Uint16(msg[off+8:]))
		off += 10
		if off+rdlen > len(msg) {
			break
		}
		if typ == 1 && class == 1 && rdlen == 4 {
			out = append(out, net.IP{msg[off], msg[off+1], msg[off+2], msg[off+3]})
		}
		off += rdlen
	}
	return out
}

func skipName(msg []byte, off int) int {
	for off < len(msg) {
		l := int(msg[off])
		if l == 0 {
			return off + 1
		}
		if l&0xC0 == 0xC0 {
			return off + 2
		}
		off += l + 1
	}
	return -1
}

func parseHeaders(raw []string) map[string]string {
	out := make(map[string]string, len(raw))
	for _, line := range raw {
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:idx])
		v := strings.TrimSpace(line[idx+1:])
		if k != "" && v != "" {
			out[k] = v
		}
	}
	return out
}

var ErrEmpty = errors.New("fetch: empty body")

func Int(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}
