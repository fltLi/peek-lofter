package config

import (
	"os"
	"strconv"
	"time"
)

// Global holds options shared by all commands.
type Global struct {
	Format      string
	Fields      []string
	Rate        time.Duration
	Timeout     time.Duration
	Retries     int
	Concurrency int
	Verbose     bool
	Quiet       bool
}

// Default returns the built-in CLI defaults.
func Default() Global {
	return Global{
		Format:      "auto",
		Rate:        350 * time.Millisecond,
		Timeout:     30 * time.Second,
		Retries:     3,
		Concurrency: 2,
	}
}

// FromEnv overlays PEEK_LOFTER_* variables on top of defaults.
func FromEnv(g *Global) {
	if v := os.Getenv("PEEK_LOFTER_FORMAT"); v != "" {
		g.Format = v
	}
	if v := os.Getenv("PEEK_LOFTER_RATE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			g.Rate = d
		}
	}
	if v := os.Getenv("PEEK_LOFTER_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			g.Timeout = d
		}
	}
	if v := os.Getenv("PEEK_LOFTER_RETRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			g.Retries = n
		}
	}
	if v := os.Getenv("PEEK_LOFTER_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			g.Concurrency = n
		}
	}
}
