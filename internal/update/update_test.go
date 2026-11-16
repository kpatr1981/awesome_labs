package update

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		cur, latest string
		want        bool
	}{
		{"v0.17.0", "v0.18.0", true},
		{"v0.18.0", "v0.18.0", false},
		{"v0.18.1", "v0.18.0", false},
		{"v0.9.0", "v0.10.0", true},
		{"v0.18.0-3-gabc123-dirty", "v0.18.0", false},
		{"v0.18.0-3-gabc123", "v0.19.0", true},
		{"dev", "v9.9.9", false},
		{"abc1234", "v1.0.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.cur, c.latest); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.cur, c.latest, got, c.want)
		}
	}
}

func TestDailyAsksOnceAndSendsNothingAboutTheUser(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if ua := r.Header.Get("User-Agent"); ua != "awesome_labs" {
			t.Errorf("User-Agent %q", ua)
		}
		if r.URL.RawQuery != "" || r.Header.Get("Cookie") != "" {
			t.Errorf("request carries extra data: %q", r.URL.String())
		}
		fmt.Fprint(w, `{"tag_name":"v0.19.0","html_url":"https://example.invalid/r"}`)
	}))
	defer srv.Close()
	old := API
	API = srv.URL
	defer func() { API = old }()
	dir := t.TempDir()
	for range 3 {
		r, err := Daily(context.Background(), dir)
		if err != nil || r.Tag != "v0.19.0" {
			t.Fatalf("Daily = %+v, %v", r, err)
		}
	}
	if calls != 1 {
		t.Errorf("GitHub asked %d times, want once a day", calls)
	}
}
