package extractor

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

type htmlExtractor struct{}

func (e *htmlExtractor) Extract(r io.Reader) (string, error) {
	z := html.NewTokenizer(r)
	var buf strings.Builder
	inSkip := false

	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			err := z.Err()
			if err == io.EOF {
				return strings.TrimSpace(buf.String()), nil
			}
			return "", err
		case html.TextToken:
			if !inSkip {
				t := strings.TrimSpace(string(z.Text()))
				if t != "" {
					if buf.Len() > 0 {
						buf.WriteByte(' ')
					}
					buf.WriteString(t)
				}
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "script", "style", "noscript":
				inSkip = true
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "script", "style", "noscript":
				inSkip = false
			case "p", "br", "li", "div", "h1", "h2", "h3", "h4", "h5", "h6", "tr":
				buf.WriteByte('\n')
			}
		}
	}
}

func init() {
	Register(".html", &htmlExtractor{})
	Register(".htm", &htmlExtractor{})
}
