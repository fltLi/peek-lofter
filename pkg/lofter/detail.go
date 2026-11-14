package lofter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/fltLi/peek-lofter/pkg/model"
)

type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	switch s {
	case "true", "1":
		*b = true
	case "false", "0", "null", "":
		*b = false
	default:
		var n int
		if err := json.Unmarshal(data, &n); err == nil {
			*b = n != 0
			return nil
		}
		*b = false
	}
	return nil
}

type h5DetailResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		BlogInfo struct {
			BlogID       int64  `json:"blogId"`
			BlogName     string `json:"blogName"`
			BlogNickName string `json:"blogNickName"`
		} `json:"blogInfo"`
		PostData struct {
			PostView struct {
				ID                  int64    `json:"id"`
				BlogID              int64    `json:"blogId"`
				Title               string   `json:"title"`
				Type                int      `json:"type"`
				PublishTime         int64    `json:"publishTime"`
				Permalink           string   `json:"permalink"`
				Digest              string   `json:"digest"`
				TagList             []string `json:"tagList"`
				ForbidVideoDownload flexBool `json:"forbidVideoDownload"`
				FansVipPrivate      flexBool `json:"fansVipPrivate"`
				PayView             flexBool `json:"payView"`
			} `json:"postView"`
			PostCountView struct {
				ResponseCount int64 `json:"responseCount"`
				FavoriteCount int64 `json:"favoriteCount"`
				HotCount      int64 `json:"hotCount"`
			} `json:"postCountView"`
		} `json:"postData"`
	} `json:"data"`
}

type appDetailResponse struct {
	Meta struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
	} `json:"meta"`
	Response struct {
		Posts []struct {
			Post appPost `json:"post"`
		} `json:"posts"`
	} `json:"response"`
}

type appPost struct {
	ID          int64           `json:"id"`
	BlogID      int64           `json:"blogId"`
	Title       string          `json:"title"`
	Content     string          `json:"content"`
	Permalink   string          `json:"permalink"`
	PublishTime int64           `json:"publishTime"`
	Type        int             `json:"type"`
	Tag         string          `json:"tag"`
	TagList     []string        `json:"tagList"`
	PhotoLinks  json.RawMessage `json:"photoLinks"`
	Hot         int64           `json:"hot"`
	PostCount   struct {
		ResponseCount int64 `json:"responseCount"`
		FavoriteCount int64 `json:"favoriteCount"`
		HotCount      int64 `json:"hotCount"`
	} `json:"postCount"`
	ForbidVideoDownload flexBool `json:"forbidVideoDownload"`
	FansVipPrivate      flexBool `json:"fansVipPrivate"`
	PayView             flexBool `json:"payView"`
}

// PostDetail fetches a post. H5 is cheap and uses permalink; Full calls the
// App API and therefore needs numeric post and blog ids.
func (c *Client) PostDetail(ctx context.Context, ref model.Ref, mode DetailMode) (model.Post, error) {
	if ref.Permalink != "" {
		id, blogID := parsePermalinkIDs(ref.Permalink)
		if ref.ID == 0 {
			ref.ID = id
		}
		if ref.Author.BlogID == 0 {
			ref.Author.BlogID = blogID
		}
	}
	if mode == DetailFull || ref.Permalink == "" {
		return c.postDetailFull(ctx, ref)
	}
	return c.postDetailH5(ctx, ref)
}

func (c *Client) postDetailH5(ctx context.Context, ref model.Ref) (model.Post, error) {
	u := c.dwrURL + "/newweb/post/detail.json?permalink=" + url.QueryEscape(ref.Permalink)
	raw, err := c.do(ctx, "GET", u, nil, "")
	if err != nil {
		return model.Post{}, err
	}
	var resp h5DetailResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return model.Post{}, fmt.Errorf("%w: decode h5 detail: %w", ErrUpstream, err)
	}
	if resp.Code != 0 {
		return model.Post{}, fmt.Errorf("%w: code=%d msg=%s", ErrUpstream, resp.Code, resp.Msg)
	}
	pv := resp.Data.PostData.PostView
	post := model.Post{
		Ref:     ref,
		Tags:    pv.TagList,
		Content: pv.Digest,
		Stats: model.Stats{
			Hot:      resp.Data.PostData.PostCountView.HotCount,
			Favorite: resp.Data.PostData.PostCountView.FavoriteCount,
			Comments: resp.Data.PostData.PostCountView.ResponseCount,
		},
		Flags: model.Flags{
			PayView:             bool(pv.PayView),
			FansVipPrivate:      bool(pv.FansVipPrivate),
			ForbidVideoDownload: bool(pv.ForbidVideoDownload),
		},
	}
	if post.ID == 0 {
		post.ID = pv.ID
	}
	if post.Title == "" {
		post.Title = pv.Title
	}
	if post.Type == 0 {
		post.Type = pv.Type
	}
	if post.PublishedAt.IsZero() {
		post.PublishedAt = model.FromUnixMilli(pv.PublishTime)
	}
	if post.Permalink == "" {
		post.Permalink = pv.Permalink
	}
	if post.Author.BlogName == "" {
		post.Author.BlogName = resp.Data.BlogInfo.BlogName
	}
	if post.Author.BlogID == 0 {
		post.Author.BlogID = resp.Data.BlogInfo.BlogID
	}
	if post.Author.Nickname == "" {
		post.Author.Nickname = resp.Data.BlogInfo.BlogNickName
	}
	if post.URL == "" {
		post.URL = model.PostURL(post.Author.BlogName, post.Permalink)
	}
	post.ID = pv.ID
	post.Permalink = pv.Permalink
	post.Title = pv.Title
	post.Type = pv.Type
	post.PublishedAt = model.FromUnixMilli(pv.PublishTime)
	post.Author.BlogID = resp.Data.BlogInfo.BlogID
	post.Author.BlogName = resp.Data.BlogInfo.BlogName
	post.Author.Nickname = resp.Data.BlogInfo.BlogNickName
	if post.URL == "" {
		post.URL = model.PostURL(post.Author.BlogName, post.Permalink)
	}
	return post, nil
}

func (c *Client) postDetailFull(ctx context.Context, ref model.Ref) (model.Post, error) {
	// The App endpoint needs numeric post/blog ids. H5 is authoritative for
	// refs that only have a URL or permalink.
	h5, err := c.postDetailH5(ctx, ref)
	if err == nil {
		ref = h5.Ref
	}
	form := url.Values{}
	form.Set("targetblogid", strconv.FormatInt(ref.Author.BlogID, 10))
	form.Set("supportposttypes", "1,2,3,4,5,6")
	form.Set("tagFrom", "")
	form.Set("blogdomain", ref.Author.BlogName+".lofter.com")
	form.Set("offset", "0")
	form.Set("requestType", "0")
	form.Set("postdigestnew", "1")
	form.Set("postid", strconv.FormatInt(ref.ID, 10))
	form.Set("checkpwd", "1")
	form.Set("needgetpoststat", "1")

	raw, err := c.do(ctx, "POST", c.baseURL+"/oldapi/post/detail.api", form, "")
	if err != nil {
		return model.Post{}, err
	}
	var resp appDetailResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return model.Post{}, fmt.Errorf("%w: decode app detail: %w", ErrUpstream, err)
	}
	if resp.Meta.Status != 200 || len(resp.Response.Posts) == 0 {
		return model.Post{}, fmt.Errorf("%w: status=%d msg=%s", ErrUpstream, resp.Meta.Status, resp.Meta.Msg)
	}
	p := resp.Response.Posts[0].Post
	tags := p.TagList
	if len(tags) == 0 && p.Tag != "" {
		tags = splitTags(p.Tag)
	}
	post := model.Post{
		Ref:     ref,
		Tags:    tags,
		Content: p.Content,
		Stats: model.Stats{
			Hot:      p.PostCount.HotCount,
			Favorite: p.PostCount.FavoriteCount,
			Comments: p.PostCount.ResponseCount,
		},
		Images: decodeImages(p.PhotoLinks),
		Flags: model.Flags{
			PayView:             bool(p.PayView),
			FansVipPrivate:      bool(p.FansVipPrivate),
			ForbidVideoDownload: bool(p.ForbidVideoDownload),
		},
	}
	if post.ID == 0 {
		post.ID = p.ID
	}
	if post.Title == "" {
		post.Title = p.Title
	}
	if post.Type == 0 {
		post.Type = p.Type
	}
	if post.PublishedAt.IsZero() {
		post.PublishedAt = model.FromUnixMilli(p.PublishTime)
	}
	if post.Permalink == "" {
		post.Permalink = p.Permalink
	}
	if post.URL == "" {
		post.URL = model.PostURL(post.Author.BlogName, post.Permalink)
	}
	if post.Author.BlogID == 0 {
		post.Author.BlogID = p.BlogID
	}
	return post, nil
}

func decodeImages(raw json.RawMessage) []model.Image {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		raw = []byte(s)
	}
	var arr []struct {
		Raw   string `json:"raw"`
		Orign string `json:"orign"`
	}
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	out := make([]model.Image, 0, len(arr))
	for _, x := range arr {
		if x.Raw == "" && x.Orign == "" {
			continue
		}
		out = append(out, model.Image{Raw: x.Raw, Orign: x.Orign})
	}
	return out
}

func parsePermalinkIDs(p string) (postID int64, blogID int64) {
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
