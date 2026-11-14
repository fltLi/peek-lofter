package dwr

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/fltLi/peek-lofter/pkg/model"
)

// assignmentRe matches one assignment anywhere in the reply. DWR frequently
// puts several assignments on a single line, so line-based parsing is not enough.
var assignmentRe = regexp.MustCompile(
	`s([0-9]+)\.([A-Za-z_][A-Za-z0-9_]*)=("(?:[^"\\]|\\.)*"|s[0-9]+|-?[0-9]+|true|false|null);`,
)

type value struct {
	str   string
	ref   string
	num   int64
	isNum bool
	isStr bool
}

type object map[string]value

// ParseRefs scans DWR assignments and returns refs in the order their objects
// first appear. It never evaluates JavaScript.
func ParseRefs(raw []byte) ([]model.Ref, error) {
	objects := map[string]object{}
	var order []string
	for _, m := range assignmentRe.FindAllStringSubmatch(string(raw), -1) {
		name := "s" + m[1]
		field := m[2]
		v := parseValue(m[3])
		obj := objects[name]
		if obj == nil {
			obj = object{}
			objects[name] = obj
			order = append(order, name)
		}
		obj[field] = v
	}

	var refs []model.Ref
	for _, name := range order {
		obj := objects[name]
		pageURL, ok := obj["blogPageUrl"]
		if !ok || !pageURL.isStr || pageURL.str == "" {
			continue
		}
		ref := model.Ref{URL: pageURL.str, Permalink: permalinkFromURL(pageURL.str)}
		if ref.Permalink != "" {
			ref.ID, ref.Author.BlogID = parsePermalink(ref.Permalink)
		}
		// DWR carries the real post id; the permalink suffix is not a simple base36 id.
		if v, ok := obj["id"]; ok && v.isNum && v.num > 0 {
			ref.ID = v.num
		}
		if v, ok := obj["blogId"]; ok && v.isNum && v.num > 0 {
			ref.Author.BlogID = v.num
		}
		if v, ok := obj["publishTime"]; ok && v.isNum {
			ref.PublishedAt = model.FromUnixMilli(v.num)
		}
		if v, ok := obj["blogInfo"]; ok && v.ref != "" {
			if info := objects[v.ref]; info != nil {
				if x, ok := info["blogName"]; ok && x.isStr {
					ref.Author.BlogName = x.str
				}
				if x, ok := info["blogNickName"]; ok && x.isStr {
					ref.Author.Nickname = x.str
				}
				if x, ok := info["blogId"]; ok && x.isNum {
					ref.Author.BlogID = x.num
				}
			}
		}
		if ref.Author.BlogName == "" {
			ref.Author.BlogName = blogNameFromURL(pageURL.str)
		}
		if ref.URL == "" {
			ref.URL = model.PostURL(ref.Author.BlogName, ref.Permalink)
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func parseValue(s string) value {
	s = strings.TrimSpace(s)
	if s == "" {
		return value{}
	}
	if s[0] == '"' {
		str, err := strconv.Unquote(s)
		if err != nil {
			return value{str: strings.Trim(s, "\""), isStr: true}
		}
		return value{str: str, isStr: true}
	}
	if strings.HasPrefix(s, "s") {
		if _, err := strconv.Atoi(s[1:]); err == nil {
			return value{ref: s}
		}
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return value{num: n, isNum: true}
	}
	return value{}
}

func permalinkFromURL(raw string) string {
	i := strings.Index(raw, "/post/")
	if i < 0 {
		return ""
	}
	p := raw[i+len("/post/"):]
	if j := strings.IndexAny(p, "?#"); j >= 0 {
		p = p[:j]
	}
	return strings.Trim(p, "/")
}

func blogNameFromURL(raw string) string {
	if i := strings.Index(raw, "://"); i >= 0 {
		raw = raw[i+3:]
	}
	if i := strings.Index(raw, "/"); i >= 0 {
		raw = raw[:i]
	}
	if i := strings.Index(raw, "."); i >= 0 {
		return raw[:i]
	}
	return raw
}

func parsePermalink(p string) (postID int64, blogID int64) {
	parts := strings.SplitN(p, "_", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	if n, err := strconv.ParseInt(parts[0], 36, 64); err == nil {
		blogID = n
	}
	if n, err := strconv.ParseInt(parts[1], 36, 64); err == nil {
		postID = n
	}
	return postID, blogID
}

var ErrEmpty = errors.New("dwr: no refs found")
