package lofter

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const (
	DefaultBaseURL = "https://api.lofter.com"
	DefaultDWRURL  = "https://www.lofter.com"
	DefaultUA      = "LOFTER-Android 8.2.36 (V2309A; Android 9; null) WIFI"
)

// DetailMode selects the cheap H5 detail or the full App detail.
type DetailMode int

const (
	// DetailH5 uses newweb/post/detail.json: fast, but no photoLinks.
	DetailH5 DetailMode = iota
	// DetailFull uses oldapi/post/detail.api: content, photoLinks and flags.
	DetailFull
)

// Options configures HTTP behavior for the LOFTER client.
type Options struct {
	BaseURL      string
	DWRURL       string
	UserAgent    string
	HTTPClient   *http.Client
	Rate         time.Duration
	Retries      int
	MaxBodyBytes int64
	Timeout      time.Duration
}

// Client is a read-only LOFTER HTTP client. It is safe for concurrent use.
type Client struct {
	baseURL   string
	dwrURL    string
	userAgent string
	http      *http.Client
	limiter   *rate.Limiter
	retries   int
	maxBody   int64
}

// New creates a client with sensible defaults for retries, timeout and rate.
func New(opts Options) (*Client, error) {
	if opts.BaseURL == "" {
		opts.BaseURL = DefaultBaseURL
	}
	if opts.DWRURL == "" {
		opts.DWRURL = DefaultDWRURL
	}
	if opts.UserAgent == "" {
		opts.UserAgent = DefaultUA
	}
	if opts.Rate <= 0 {
		opts.Rate = 350 * time.Millisecond
	}
	if opts.Retries < 0 {
		opts.Retries = 0
	}
	if opts.Retries == 0 {
		opts.Retries = 3
	}
	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = 16 << 20
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: opts.Timeout}
	}
	return &Client{
		baseURL:   strings.TrimRight(opts.BaseURL, "/"),
		dwrURL:    strings.TrimRight(opts.DWRURL, "/"),
		userAgent: opts.UserAgent,
		http:      hc,
		limiter:   rate.NewLimiter(rate.Every(opts.Rate), 1),
		retries:   opts.Retries,
		maxBody:   opts.MaxBodyBytes,
	}, nil
}

func (c *Client) do(ctx context.Context, method, rawURL string, form url.Values, referer string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		var body io.Reader
		if form != nil {
			body = strings.NewReader(form.Encode())
		}
		req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("Accept", "application/json, text/plain, */*")
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
		}
		if referer != "" {
			req.Header.Set("Referer", referer)
			req.Header.Set("Origin", "https://www.lofter.com")
			req.Header.Set("X-Requested-With", "XMLHttpRequest")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%w: %w", ErrNetwork, err)
			if attempt < c.retries {
				if err := sleep(ctx, backoff(attempt)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, lastErr
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, c.maxBody+1))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("%w: read body: %w", ErrNetwork, readErr)
			if attempt < c.retries {
				if err := sleep(ctx, backoff(attempt)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, lastErr
		}
		if int64(len(data)) > c.maxBody {
			return nil, fmt.Errorf("%w: response too large", ErrUpstream)
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			return data, nil
		case resp.StatusCode == http.StatusNotFound:
			return nil, ErrNotFound
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			lastErr = fmt.Errorf("%w: HTTP %d", ErrRateLimited, resp.StatusCode)
			if attempt < c.retries {
				wait := retryAfter(resp.Header.Get("Retry-After"))
				if wait <= 0 {
					wait = backoff(attempt)
				}
				if err := sleep(ctx, wait); err != nil {
					return nil, err
				}
				continue
			}
			return nil, lastErr
		case resp.StatusCode == http.StatusForbidden:
			return nil, fmt.Errorf("%w: HTTP 403", ErrUpstream)
		default:
			return nil, fmt.Errorf("%w: HTTP %d: %s", ErrUpstream, resp.StatusCode, snippet(data))
		}
	}
	return nil, lastErr
}

func backoff(attempt int) time.Duration {
	d := time.Second
	for range attempt {
		d *= 2
	}
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

func retryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
