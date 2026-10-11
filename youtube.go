package markitdown

import (
	"encoding/json"
	"mime"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// shortcut: no YouTube transcript client and no network fetch, so a transcript
// is never invented. Metadata and the HTML fallback only.

func htmlLike(mimeType, filename string) bool {
	mt, _, err := mime.ParseMediaType(strings.TrimSpace(mimeType))
	if err == nil && mt != "" && mt != "application/octet-stream" {
		return strings.HasPrefix(mt, "text/html") || strings.HasPrefix(mt, "application/xhtml")
	}
	switch ext(filename) {
	case ".html", ".htm":
		return true
	default:
		return false
	}
}

func youtubeVideoID(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	if un, err := url.PathUnescape(raw); err == nil {
		raw = un
	}
	raw = strings.ReplaceAll(raw, `\?`, "?")
	raw = strings.ReplaceAll(raw, `\=`, "=")
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	var parts []string
	for _, part := range strings.Split(u.Path, "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	switch host {
	case "youtu.be", "www.youtu.be":
		if len(parts) == 0 {
			return ""
		}
		return parts[0]
	case "youtube.com", "www.youtube.com", "m.youtube.com":
		if len(parts) == 0 {
			return ""
		}
		switch parts[0] {
		case "watch":
			return u.Query().Get("v")
		case "shorts", "embed":
			if len(parts) < 2 {
				return ""
			}
			return parts[1]
		}
	}
	return ""
}

var ytDataRE = regexp.MustCompile(`var ytInitialData = (\{.*?\});`)

func convertYouTube(data []byte, charset string) (Result, error) {
	text, err := decodeText(data, charset)
	if err != nil {
		return Result{}, err
	}
	doc, err := html.Parse(strings.NewReader(text))
	if err != nil {
		return Result{}, wrapCorrupt(err)
	}
	meta := map[string]string{}
	if title := titleString(doc); title != "" {
		meta["title"] = title
	}
	collectMeta(doc, meta)
	readYTDescription(doc, meta)

	title := metaGet(meta, "title", "og:title", "name")
	var b strings.Builder
	if title != "" {
		b.WriteString("\n## ")
		b.WriteString(title)
		b.WriteString("\n")
	}
	var stats strings.Builder
	if v := metaGet(meta, "interactionCount"); v != "" {
		stats.WriteString("- **Views:** ")
		stats.WriteString(v)
		stats.WriteByte('\n')
	}
	if v := metaGet(meta, "keywords"); v != "" {
		stats.WriteString("- **Keywords:** ")
		stats.WriteString(v)
		stats.WriteByte('\n')
	}
	if v := metaGet(meta, "duration"); v != "" {
		stats.WriteString("- **Runtime:** ")
		stats.WriteString(v)
		stats.WriteByte('\n')
	}
	if stats.Len() > 0 {
		b.WriteString("\n### Video Metadata\n")
		b.WriteString(stats.String())
		b.WriteByte('\n')
	}
	if d := metaGet(meta, "description", "og:description"); d != "" {
		b.WriteString("\n### Description\n")
		b.WriteString(d)
		b.WriteByte('\n')
	}
	if b.Len() == 0 {
		return convertHTML([]byte(text), htmlOpt{closedHeading: true, pipeTables: true})
	}
	return Result{Text: "# YouTube\n" + b.String(), Title: title}, nil
}

func metaGet(meta map[string]string, keys ...string) string {
	for _, key := range keys {
		if v, ok := meta[key]; ok {
			return v
		}
	}
	return ""
}

func titleString(doc *html.Node) string {
	n := findHTMLTag(doc, "title")
	if n == nil || n.FirstChild == nil || n.FirstChild.NextSibling != nil || n.FirstChild.Type != html.TextNode {
		return ""
	}
	return n.FirstChild.Data
}

func collectMeta(n *html.Node, meta map[string]string) {
	if n.Type == html.ElementNode && n.Data == "meta" {
		for _, a := range n.Attr {
			if a.Key != "itemprop" && a.Key != "property" && a.Key != "name" {
				continue
			}
			if a.Val != "" {
				if content := attr(n, "content"); content != "" {
					meta[a.Val] = content
				}
			}
			break
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectMeta(c, meta)
	}
}

func readYTDescription(doc *html.Node, meta map[string]string) {
	var walk func(*html.Node) bool
	walk = func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "script" {
			s := scriptString(n)
			if s == "" {
				return false
			}
			if !strings.Contains(s, "ytInitialData") {
				return false
			}
			if m := ytDataRE.FindStringSubmatch(s); m != nil {
				var data any
				if json.Unmarshal([]byte(m[1]), &data) == nil {
					if body, ok := findKey(data, "attributedDescriptionBodyText").(map[string]any); ok {
						content, _ := body["content"].(string)
						meta["description"] = content
					}
				}
			}
			return true
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if walk(c) {
				return true
			}
		}
		return false
	}
	walk(doc)
}

func scriptString(n *html.Node) string {
	if n.FirstChild == nil || n.FirstChild.NextSibling != nil || n.FirstChild.Type != html.TextNode {
		return ""
	}
	return n.FirstChild.Data
}

func findKey(v any, key string) any {
	switch t := v.(type) {
	case []any:
		for _, elm := range t {
			if ret := findKey(elm, key); ret != nil {
				return ret
			}
		}
	case map[string]any:
		if val, ok := t[key]; ok {
			return val
		}
		for _, val := range t {
			if ret := findKey(val, key); ret != nil {
				return ret
			}
		}
	}
	return nil
}
