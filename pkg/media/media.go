package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fltLi/peek-lofter/pkg/model"
)

// Options configures the image downloader.
type Options struct {
	HTTPClient *http.Client
	MaxBytes   int64
	Timeout    time.Duration
	UserAgent  string
}

// Downloader fetches images and writes them into a bundle directory.
type Downloader struct {
	http      *http.Client
	maxBytes  int64
	userAgent string
}

// New creates a downloader with defaults for timeout and size limits.
func New(opts Options) *Downloader {
	if opts.Timeout <= 0 {
		opts.Timeout = 60 * time.Second
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = 100 << 20
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "Mozilla/5.0"
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: opts.Timeout}
	}
	return &Downloader{http: hc, maxBytes: opts.MaxBytes, userAgent: opts.UserAgent}
}

// MigrateURL rewrites nos.netease.com/imglfN paths to imglfN.lf127.net.
// The upstream nos host returns 403 for images.
func MigrateURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.Host != "nos.netease.com" {
		return raw
	}
	path := strings.TrimPrefix(u.Path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "imglf") {
		return raw
	}
	u.Scheme = "https"
	u.Host = parts[0] + ".lf127.net"
	u.Path = "/" + parts[1]
	return u.String()
}

// Download tries raw then orign, writes a .part file and renames it into place.
// base is the filename stem (for example 001).
func (d *Downloader) Download(ctx context.Context, img model.Image, dir, base string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	candidates := make([]string, 0, 2)
	if img.Raw != "" {
		candidates = append(candidates, MigrateURL(img.Raw))
	}
	if img.Orign != "" && img.Orign != img.Raw {
		candidates = append(candidates, img.Orign)
	}
	var lastErr error
	for _, rawURL := range candidates {
		path, err := d.downloadOne(ctx, rawURL, dir, base)
		if err == nil {
			return path, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("media: no url")
	}
	return "", lastErr
}

func (d *Downloader) downloadOne(ctx context.Context, rawURL, dir, base string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", d.userAgent)
	resp, err := d.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("media: HTTP %d for %s", resp.StatusCode, rawURL)
	}
	ext := extFromURL(rawURL)
	if ext == "" {
		ext = extFromContentType(resp.Header.Get("Content-Type"))
	}
	if ext == "" {
		ext = ".jpg"
	}
	path := filepath.Join(dir, base+ext)
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, d.maxBytes+1))
	closeErr := f.Close()
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	if closeErr != nil {
		os.Remove(tmp)
		return "", closeErr
	}
	if n > d.maxBytes {
		os.Remove(tmp)
		return "", fmt.Errorf("media: file too large: %s", rawURL)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(path)
		if err2 := os.Rename(tmp, path); err2 != nil {
			return "", err2
		}
	}
	return path, nil
}

func extFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(u.Path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
		return ext
	default:
		return ""
	}
}

func extFromContentType(ct string) string {
	ct = strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	switch ct {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	default:
		return ""
	}
}
