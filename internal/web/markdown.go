package web

import (
	"bytes"
	"encoding/json"
	"html"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// markdown renders exercise text to HTML. Raw HTML in the source is not
// passed through (goldmark's default), so the output is safe to insert.
type markdown struct {
	md    goldmark.Markdown
	cache sync.Map // source -> html
}

func newMarkdown() *markdown {
	return &markdown{md: goldmark.New(goldmark.WithExtensions(extension.GFM))}
}

func (m *markdown) render(src string) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	if v, ok := m.cache.Load(src); ok {
		return v.(string)
	}
	var b bytes.Buffer
	if err := m.md.Convert([]byte(src), &b); err != nil {
		return "<pre>" + html.EscapeString(src) + "</pre>"
	}
	m.cache.Store(src, b.String())
	return b.String()
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
