package media

import "testing"

func TestMigrateURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"http://nos.netease.com/imglf6/img/a.jpg", "https://imglf6.lf127.net/img/a.jpg"},
		{"https://imglf6.lf127.net/img/a.jpg", "https://imglf6.lf127.net/img/a.jpg"},
		{"http://example.com/a.jpg", "http://example.com/a.jpg"},
	}
	for _, c := range cases {
		if got := MigrateURL(c.in); got != c.want {
			t.Fatalf("MigrateURL(%q)=%q want %q", c.in, got, c.want)
		}
	}
}
