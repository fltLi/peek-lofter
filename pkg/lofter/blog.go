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

type blogResponse struct {
	Meta struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
	} `json:"meta"`
	Response struct {
		Archives []struct {
			Year       int   `json:"year"`
			MonthCount []int `json:"monthCount"`
		} `json:"archives"`
		Offset int `json:"offset"`
		Posts  []struct {
			Post blogPost `json:"post"`
		} `json:"posts"`
	} `json:"response"`
}

type blogPost struct {
	ID          int64    `json:"id"`
	BlogID      int64    `json:"blogId"`
	Title       string   `json:"title"`
	PublishTime int64    `json:"publishTime"`
	Type        int      `json:"type"`
	Tag         string   `json:"tag"`
	TagList     []string `json:"tagList"`
	Permalink   string   `json:"permalink"`
	BlogPageURL string   `json:"blogPageUrl"`
	BlogInfo    struct {
		BlogID       int64  `json:"blogId"`
		BlogName     string `json:"blogName"`
		BlogNickName string `json:"blogNickName"`
	} `json:"blogInfo"`
	PublisherMainBlogInfo struct {
		BlogID       int64  `json:"blogId"`
		BlogName     string `json:"blogName"`
		BlogNickName string `json:"blogNickName"`
	} `json:"publisherMainBlogInfo"`
}

// BlogPage fetches one page of a user's posts together with archive counts.
func (c *Client) BlogPage(ctx context.Context, q model.BlogQuery) (model.BlogPage, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	form := url.Values{}
	form.Set("blogdomain", q.BlogName+".lofter.com")
	form.Set("method", "getPostLists")
	form.Set("checkpwd", "1")
	form.Set("following", "0")
	form.Set("limit", strconv.Itoa(limit))
	form.Set("offset", strconv.Itoa(q.Offset))
	form.Set("needgetpoststat", "1")
	form.Set("postdigestnew", "1")
	form.Set("supportposttypes", "1,2,3,4,5,6")

	raw, err := c.do(ctx, "POST", c.baseURL+"/v2.0/blogHomePage.api?product=lofter-iphone-10.0.0", form, "")
	if err != nil {
		return model.BlogPage{}, err
	}
	var resp blogResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return model.BlogPage{}, fmt.Errorf("%w: decode blog page: %w", ErrUpstream, err)
	}
	if resp.Meta.Status != 200 {
		if resp.Meta.Status == 4200 {
			return model.BlogPage{}, fmt.Errorf("%w: %s", ErrNotFound, resp.Meta.Msg)
		}
		return model.BlogPage{}, fmt.Errorf("%w: status=%d msg=%s", ErrUpstream, resp.Meta.Status, resp.Meta.Msg)
	}
	page := model.BlogPage{Next: resp.Response.Offset}
	for _, a := range resp.Response.Archives {
		var months [12]int
		for i := 0; i < len(a.MonthCount) && i < 12; i++ {
			months[i] = a.MonthCount[i]
		}
		page.Archives = append(page.Archives, model.Archive{Year: a.Year, Months: months})
	}
	for _, item := range resp.Response.Posts {
		p := item.Post
		author := p.BlogInfo
		if author.BlogName == "" {
			author = p.PublisherMainBlogInfo
		}
		url := p.BlogPageURL
		if url == "" {
			url = model.PostURL(author.BlogName, p.Permalink)
		}
		tags := p.TagList
		if len(tags) == 0 && p.Tag != "" {
			tags = splitTags(p.Tag)
		}
		ref := model.Ref{
			ID:          p.ID,
			URL:         url,
			Permalink:   p.Permalink,
			Title:       p.Title,
			Type:        p.Type,
			PublishedAt: model.FromUnixMilli(p.PublishTime),
			Author: model.Author{
				BlogID:   author.BlogID,
				BlogName: author.BlogName,
				Nickname: author.BlogNickName,
			},
		}
		_ = tags
		page.Refs = append(page.Refs, ref)
	}
	if len(page.Refs) == 0 {
		page.Next = -1
	} else if page.Next == 0 || page.Next <= q.Offset {
		page.Next = q.Offset + len(page.Refs)
	}
	return page, nil
}

func splitTags(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
