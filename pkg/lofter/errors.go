package lofter

import "errors"

var (
	ErrNotFound    = errors.New("lofter: not found")
	ErrRateLimited = errors.New("lofter: rate limited")
	ErrUpstream    = errors.New("lofter: upstream error")
	ErrNetwork     = errors.New("lofter: network error")
)
