package lofter

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/fltLi/peek-lofter/internal/dwr"
	"github.com/fltLi/peek-lofter/pkg/model"
)

// Reactions fetches one DWR page of a user's likes or recommendations.
func (c *Client) Reactions(ctx context.Context, q model.ReactionQuery) (model.RefPage, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	method := "queryLikePosts"
	referer := c.dwrURL + "/"
	if q.Kind == "share" {
		method = "querySharePosts"
	}
	form := url.Values{}
	form.Set("callCount", "1")
	form.Set("scriptSessionId", "A1B2C3D4E5F6187")
	form.Set("httpSessionId", "")
	form.Set("c0-scriptName", "BlogBean")
	form.Set("c0-methodName", method)
	form.Set("c0-id", "0")
	form.Set("c0-param0", "number:"+strconv.FormatInt(q.BlogID, 10))
	form.Set("c0-param1", "number:"+strconv.Itoa(limit))
	form.Set("c0-param2", "number:"+strconv.Itoa(q.Offset))
	form.Set("c0-param3", "string:")
	form.Set("batchId", "1")

	raw, err := c.do(ctx, "POST", c.dwrURL+"/dwr/call/plaincall/BlogBean."+method+".dwr", form, referer)
	if err != nil {
		return model.RefPage{}, err
	}
	refs, err := dwr.ParseRefs(raw)
	if err != nil {
		return model.RefPage{}, fmt.Errorf("%w: parse dwr: %w", ErrUpstream, err)
	}
	page := model.RefPage{Refs: refs, Next: q.Offset + len(refs)}
	if len(refs) == 0 {
		page.Next = -1
	}
	return page, nil
}
