package dwr

import "testing"

func TestParseRefsScansWholeLine(t *testing.T) {
	raw := []byte(
		"//#DWR-REPLY\nvar s1={};s1.blogPageUrl=\"https://example-user.lofter.com/post/1_2\";s1.publishTime=1714900572387;s1.id=123;s1.blogId=456;s1.blogInfo=s0;s0.blogName=\"example-user\";s0.blogNickName=\"Example User\";s0.blogId=456;\n",
	)
	refs, err := ParseRefs(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Fatalf("got %d refs", len(refs))
	}
	r := refs[0]
	if r.ID != 123 || r.Author.BlogID != 456 {
		t.Fatalf("unexpected ids: %+v", r)
	}
	if r.Permalink != "1_2" || r.Author.BlogName != "example-user" || r.Author.Nickname != "Example User" {
		t.Fatalf("unexpected ref: %+v", r)
	}
	if r.PublishedAt.UnixMilli() != 1714900572387 {
		t.Fatalf("unexpected time: %d", r.PublishedAt.UnixMilli())
	}
}
