package model

import (
	"encoding/json"
	"time"
)

// cst is UTC+8. China does not observe daylight saving time, so a fixed
// zone avoids loading tzdata on Windows.
var cst = time.FixedZone("UTC+8", 8*60*60)

// Timestamp wraps time.Time and serializes as RFC3339 in UTC+8.
// China has no daylight saving time, so a fixed zone avoids tzdata.
type Timestamp struct {
	time.Time
}

// FromUnixMilli converts a LOFTER millisecond timestamp to UTC+8.
func FromUnixMilli(ms int64) Timestamp {
	return Timestamp{Time: time.UnixMilli(ms).In(cst)}
}

// IsZero reports whether the timestamp is unset.
func (t Timestamp) IsZero() bool {
	return t.Time.IsZero()
}

// MarshalJSON emits RFC3339Nano in UTC+8; an unset timestamp becomes null.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	if t.Time.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.Time.In(cst).Format(time.RFC3339Nano))
}

// UnmarshalJSON accepts RFC3339/RFC3339Nano and normalizes to UTC+8.
func (t *Timestamp) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		t.Time = time.Time{}
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	tm, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return err
	}
	t.Time = tm.In(cst)
	return nil
}

// Author identifies the LOFTER blog that published a post.
type Author struct {
	BlogID   int64  `json:"blogId"`
	BlogName string `json:"blogName"`
	Nickname string `json:"nickname"`
}

// Source records where a Ref was discovered, for traceability.
type Source struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Window string `json:"window,omitempty"`
}

// Ref is the minimal post reference exchanged between discovery and fetch commands.
type Ref struct {
	ID          int64     `json:"id"`
	URL         string    `json:"url"`
	Permalink   string    `json:"permalink,omitempty"`
	Title       string    `json:"title,omitempty"`
	Author      Author    `json:"author"`
	PublishedAt Timestamp `json:"publishedAt"`
	Type        int       `json:"type,omitempty"`
	Source      *Source   `json:"source,omitempty"`
}

// Stats holds the public interaction counters shown on a post.
type Stats struct {
	Hot      int64 `json:"hot"`
	Favorite int64 `json:"favorite"`
	Comments int64 `json:"comments"`
}

// Image holds the raw and display URLs of one image.
type Image struct {
	Raw   string `json:"raw,omitempty"`
	Orign string `json:"orign,omitempty"`
}

// Flags marks paid, private or download-restricted posts.
type Flags struct {
	PayView             bool `json:"payView"`
	FansVipPrivate      bool `json:"fansVipPrivate"`
	ForbidVideoDownload bool `json:"forbidVideoDownload"`
}

// Post is the normalized work returned by post show and post fetch.
type Post struct {
	Ref
	Tags    []string `json:"tags,omitempty"`
	Stats   Stats    `json:"stats"`
	Content string   `json:"content,omitempty"`
	Images  []Image  `json:"images,omitempty"`
	Flags   Flags    `json:"flags"`
}

// Archive is one year of a user's monthly post counts.
type Archive struct {
	Year   int     `json:"year"`
	Months [12]int `json:"monthCount"`
}

// User is the public profile summary returned by user show.
type User struct {
	BlogID   int64     `json:"blogId"`
	BlogName string    `json:"blogName"`
	Nickname string    `json:"nickname"`
	Archives []Archive `json:"archives,omitempty"`
}

// TagPageQuery is one page request against the tag feed.
type TagPageQuery struct {
	Tag       string
	Type      string
	PostTypes string
	PostYm    string
	StartMs   int64
	EndMs     int64
	Offset    int
}

// TagPage is one page of tag refs; Next is -1 at the end.
type TagPage struct {
	Refs []Ref
	Next int
}

// BlogQuery is one page request against a user's blog.
type BlogQuery struct {
	BlogName string
	Offset   int
	Limit    int
}

// BlogPage is one page of user posts plus the archive month counts.
type BlogPage struct {
	Refs     []Ref
	Archives []Archive
	Next     int
}

// ReactionQuery requests a page of a user's likes or recommendations.
type ReactionQuery struct {
	BlogID int64
	Kind   string
	Limit  int
	Offset int
}

// RefPage is one page of refs; Next is -1 at the end.
type RefPage struct {
	Refs []Ref
	Next int
}

// PostURL builds the canonical public URL for a post.
func PostURL(blogName, permalink string) string {
	if blogName == "" {
		return "https://i.lofter.com/post/" + permalink
	}
	return "https://" + blogName + ".lofter.com/post/" + permalink
}
