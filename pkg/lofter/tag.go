package lofter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/fltLi/peek-lofter/pkg/model"
)

type tagResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		List []struct {
			PostData struct {
				PostView struct {
					ID          int64    `json:"id"`
					BlogID      int64    `json:"blogId"`
					Title       string   `json:"title"`
					Type        int      `json:"type"`
					Permalink   string   `json:"permalink"`
					PublishTime int64    `json:"publishTime"`
					TagList     []string `json:"tagList"`
					PhotoCount  int      `json:"photoCount"`
				} `json:"postView"`
				PostCount struct {
					ResponseCount int64 `json:"responseCount"`
					FavoriteCount int64 `json:"favoriteCount"`
					HotCount      int64 `json:"hotCount"`
				} `json:"postCount"`
			} `json:"postData"`
			BlogInfo struct {
				BlogName     string `json:"blogName"`
				BlogNickName string `json:"blogNickName"`
				BlogID       int64  `json:"blogId"`
			} `json:"blogInfo"`
		} `json:"list"`
		Offset int `json:"offset"`
	} `json:"data"`
}

// TagPage fetches one page of a tag feed. Empty PostTypes is normalized to
// 1,2,4. Sending 1,2,3,4,5,6 makes the endpoint return only one item.
func (c *Client) TagPage(ctx context.Context, q model.TagPageQuery) (model.TagPage, error) {
	postTypes := q.PostTypes
	if postTypes == "" {
		postTypes = "1,2,4"
	}
	kind := q.Type
	if kind == "" {
		kind = "total"
	}
	form := url.Values{}
	form.Set("product", "lofter-android-8.2.36")
	form.Set("postTypes", postTypes)
	form.Set("offset", strconv.Itoa(q.Offset))
	form.Set("postYm", q.PostYm)
	if q.StartMs > 0 {
		form.Set("postYmdSt", strconv.FormatInt(q.StartMs, 10))
		form.Set("postYmdEt", strconv.FormatInt(q.EndMs, 10))
	}
	form.Set("returnGiftCombination", "")
	form.Set("recentDay", "0")
	form.Set("protectedFlag", "0")
	form.Set("range", "0")
	form.Set("firstpermalink", "null")
	form.Set("style", "0")
	form.Set("tag", q.Tag)
	form.Set("type", kind)

	raw, err := c.do(ctx, "POST", c.baseURL+"/newapi/tagPosts.json", form, "")
	if err != nil {
		return model.TagPage{}, err
	}
	var resp tagResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return model.TagPage{}, fmt.Errorf("%w: decode tag page: %w", ErrUpstream, err)
	}
	if resp.Code != 0 {
		return model.TagPage{}, fmt.Errorf("%w: code=%d msg=%s", ErrUpstream, resp.Code, resp.Msg)
	}
	page := model.TagPage{Next: resp.Data.Offset}
	for _, item := range resp.Data.List {
		pv := item.PostData.PostView
		bi := item.BlogInfo
		ref := model.Ref{
			ID:          pv.ID,
			URL:         model.PostURL(bi.BlogName, pv.Permalink),
			Permalink:   pv.Permalink,
			Title:       pv.Title,
			Type:        pv.Type,
			PublishedAt: model.FromUnixMilli(pv.PublishTime),
			Author: model.Author{
				BlogID:   bi.BlogID,
				BlogName: bi.BlogName,
				Nickname: bi.BlogNickName,
			},
		}
		page.Refs = append(page.Refs, ref)
	}
	if len(page.Refs) == 0 {
		page.Next = -1
	} else if page.Next == 0 || page.Next <= q.Offset {
		page.Next = q.Offset + len(page.Refs)
	}
	return page, nil
}
