// Package update finds out whether a newer awesome_labs release exists. It
// asks GitHub's public releases API and sends nothing about the user.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Repo is where releases are published.
const Repo = "kpatr1981/awesome_labs"

// API is the latest-release endpoint (tests point it elsewhere).
var API = "https://api.github.com/repos/" + Repo + "/releases/latest"

// Release is the part of GitHub's answer awesome_labs uses.
type Release struct {
	Tag string `json:"tag_name"`
	URL string `json:"html_url"`
}

// Latest asks GitHub for the newest release.
func Latest(ctx context.Context) (Release, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, API, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "awesome_labs")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var r Release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return Release{}, err
	}
	if r.Tag == "" {
		return Release{}, fmt.Errorf("no release found")
	}
	return r, nil
}

// parse turns "v1.2.3", "1.2.3" or a git describe like "v1.2.3-4-gabc-dirty"
// into its numbers; ok is false for anything else ("dev").
func parse(v string) (n [3]int, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return n, false
	}
	for i, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil {
			return n, false
		}
		n[i] = x
	}
	return n, true
}

// Newer reports whether latest is a newer release than current. A build
// that isn't a release ("dev") is never told to update.
func Newer(current, latest string) bool {
	c, ok1 := parse(current)
	l, ok2 := parse(latest)
	if !ok1 || !ok2 {
		return false
	}
	for i := range c {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

type cache struct {
	Checked time.Time `json:"checked"`
	Latest  Release   `json:"latest"`
}

// Daily returns the newest release, asking GitHub at most once a day: the
// answer is kept in dir/update-check.json.
func Daily(ctx context.Context, dir string) (Release, error) {
	p := filepath.Join(dir, "update-check.json")
	var c cache
	if b, err := os.ReadFile(p); err == nil && json.Unmarshal(b, &c) == nil && time.Since(c.Checked) < 24*time.Hour && c.Latest.Tag != "" {
		return c.Latest, nil
	}
	r, err := Latest(ctx)
	if err != nil {
		return Release{}, err
	}
	b, _ := json.Marshal(cache{Checked: time.Now(), Latest: r})
	_ = os.WriteFile(p, b, 0o644)
	return r, nil
}
