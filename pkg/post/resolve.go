package post

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/fltLi/peek-lofter/pkg/model"
)

// Resolver turns a URL, permalink or numeric id into a Ref.
type Resolver struct {
	Blog string
}

// Resolve accepts a full URL, a permalink (blogbase_postbase) or a numeric id
// with Blog set. Numeric ids need a blog for the full detail call.
func (r Resolver) Resolve(raw string) (model.Ref, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return model.Ref{}, errors.New("post: empty reference")
	}
	if u := firstURL(raw); u != "" {
		return r.fromURL(u)
	}
	if strings.Contains(raw, "/") {
		return model.Ref{}, fmt.Errorf("post: not a lofter url: %s", raw)
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		ref := model.Ref{ID: n, Author: model.Author{BlogName: r.Blog}}
		if r.Blog != "" {
			ref.URL = "https://" + r.Blog + ".lofter.com/"
		}
		return ref, nil
	}
	if strings.Contains(raw, "_") {
		ref := model.Ref{Permalink: raw, Author: model.Author{BlogName: r.Blog}}
		if r.Blog != "" {
			ref.URL = model.PostURL(r.Blog, raw)
		}
		return ref, nil
	}
	return model.Ref{}, fmt.Errorf("post: cannot resolve %q", raw)
}

func (r Resolver) fromURL(raw string) (model.Ref, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return model.Ref{}, err
	}
	ref := model.Ref{URL: raw, Author: model.Author{BlogName: r.Blog}}
	host := strings.ToLower(u.Hostname())
	if strings.HasSuffix(host, ".lofter.com") {
		prefix := strings.TrimSuffix(host, ".lofter.com")
		if prefix != "www" && prefix != "i" && prefix != "api" {
			ref.Author.BlogName = prefix
		}
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch {
	case len(parts) >= 2 && parts[0] == "post":
		ref.Permalink = parts[1]
	case len(parts) >= 4 && parts[0] == "front" && parts[1] == "blog" && parts[2] == "post":
		if n, err := strconv.ParseInt(parts[3], 10, 64); err == nil {
			ref.ID = n
		}
	}
	if v := u.Query().Get("permalink"); v != "" {
		ref.Permalink = v
	}
	if ref.ID == 0 && ref.Permalink == "" {
		return model.Ref{}, fmt.Errorf("post: unsupported url: %s", raw)
	}
	if ref.Author.BlogName != "" && ref.Permalink != "" {
		ref.URL = model.PostURL(ref.Author.BlogName, ref.Permalink)
	}
	return ref, nil
}

func firstURL(s string) string {
	for _, f := range strings.Fields(s) {
		if strings.HasPrefix(f, "http://") || strings.HasPrefix(f, "https://") {
			return strings.TrimRight(f, "。，,.!！?？)")
		}
	}
	return ""
}
