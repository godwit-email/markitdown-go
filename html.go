package markitdown

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

type htmlOpt struct {
	keepData      bool
	base          string
	closedHeading bool
	pipeTables    bool
}

func convertHTML(data []byte, opt htmlOpt) (Result, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return Result{}, wrapCorrupt(err)
	}
	if wiki := findHTMLID(doc, "mw-content-text"); wiki != nil {
		title := wikiTitle(doc)
		body := strings.TrimSpace(renderBlock(wiki, opt))
		text := body
		if title != "" {
			text = "# " + title + "\n\n" + body
		}
		return Result{Text: strings.TrimSpace(text), Title: title}, nil
	}
	if algos := findClassToken(doc, "b_algo"); len(algos) > 0 {
		var parts []string
		for _, algo := range algos {
			if hasAncestorClass(algo, "b_algo") {
				continue
			}
			rewriteBingLinks(algo)
			part := strings.TrimSpace(renderBlock(algo, opt))
			if part == "" {
				continue
			}
			var lines []string
			for _, line := range strings.Split(part, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					lines = append(lines, line)
				}
			}
			if len(lines) > 0 {
				parts = append(parts, strings.Join(lines, "\n"))
			}
		}
		return Result{Text: strings.TrimSpace(strings.Join(parts, "\n\n"))}, nil
	}
	root := findHTMLTag(doc, "body")
	if root == nil {
		root = doc
	}
	return Result{Text: strings.TrimSpace(renderBlock(root, opt))}, nil
}

func wikiTitle(doc *html.Node) string {
	if n := findClassTokenOne(doc, "mw-page-title-main"); n != nil {
		return cleanTitle(textContent(n))
	}
	if n := findHTMLID(doc, "firstHeading"); n != nil {
		return cleanTitle(textContent(n))
	}
	if n := findHTMLTag(doc, "title"); n != nil {
		return cleanTitle(textContent(n))
	}
	return ""
}

func cleanTitle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, " - Wikipedia")
	return strings.TrimSpace(s)
}

func renderBlock(n *html.Node, opt htmlOpt) string {
	var parts []string
	var inline strings.Builder
	flush := func() {
		s := inline.String()
		inline.Reset()
		if strings.TrimSpace(s) == "" {
			return
		}
		parts = append(parts, strings.TrimSpace(s))
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if isHTMLBlock(c) {
			flush()
			if s := strings.TrimSpace(renderNode(c, opt)); s != "" {
				parts = append(parts, s)
			}
			continue
		}
		inline.WriteString(renderNode(c, opt))
	}
	flush()
	return strings.Join(parts, "\n\n")
}

func isHTMLBlock(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	switch n.Data {
	case "address", "article", "aside", "blockquote", "div", "dl", "fieldset", "figcaption", "figure", "footer", "form", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "li", "main", "nav", "ol", "p", "pre", "section", "table", "tbody", "thead", "tfoot", "tr", "ul", "center":
		return true
	default:
		return false
	}
}

func renderNode(n *html.Node, opt htmlOpt) string {
	if n.Type == html.TextNode {
		return strings.ReplaceAll(n.Data, "\t", " ")
	}
	if n.Type != html.ElementNode {
		return renderKids(n, opt)
	}
	if base := attr(n, "xml:base"); base != "" {
		opt.base = resolveURL(opt.base, base)
	}
	switch n.Data {
	case "script", "style", "noscript", "svg", "head":
		return ""
	case "br":
		if insideMark(n) {
			return "\n"
		}
		return "  \n"
	case "pre":
		return "```\n" + strings.Trim(textContentHTML(n), "\n") + "\n```"
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(n.Data[1] - '0')
		inner := strings.TrimSpace(renderKids(n, opt))
		if inner == "" {
			return ""
		}
		line := strings.Repeat("#", level) + " " + inner
		if opt.closedHeading {
			line += " " + strings.Repeat("#", level)
		}
		return line
	case "table":
		if opt.pipeTables {
			return renderHTMLTable(n, opt)
		}
		return renderBlock(n, opt)
	case "p", "div", "section", "article", "main", "center", "header", "footer", "nav", "figure", "figcaption", "tbody", "thead", "tfoot", "tr", "dl":
		return renderBlock(n, opt)
	case "blockquote":
		return prefixLines(renderBlock(n, opt), "> ")
	case "ul":
		return renderList(n, false, opt)
	case "ol":
		return renderList(n, true, opt)
	case "li":
		return strings.TrimSpace(renderKids(n, opt))
	case "a":
		return renderLink(n, opt)
	case "img":
		return renderImg(n, opt)
	case "b", "strong":
		return wrapMark(renderKids(n, opt), "**", "**", false)
	case "i", "em":
		return wrapMark(renderKids(n, opt), "*", "*", false)
	case "u":
		return wrapUnderline(renderKids(n, opt))
	case "s", "del", "strike":
		return wrapMark(renderKids(n, opt), "~~", "~~", true)
	default:
		return renderKids(n, opt)
	}
}

func renderHTMLTable(n *html.Node, opt htmlOpt) string {
	var rows [][]string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			var cells []string
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type != html.ElementNode || (c.Data != "td" && c.Data != "th") {
					continue
				}
				cell := strings.TrimSpace(renderKids(c, opt))
				cell = strings.ReplaceAll(cell, "\n", " ")
				cells = append(cells, cell)
			}
			if len(cells) > 0 {
				rows = append(rows, cells)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimRight(markdownTable(rows), "\n")
}

func renderKids(n *html.Node, opt htmlOpt) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(renderNode(c, opt))
	}
	return b.String()
}

func renderList(n *html.Node, ordered bool, opt htmlOpt) string {
	var b strings.Builder
	i := 1
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.Data != "li" {
			continue
		}
		item := strings.TrimSpace(renderKids(c, opt))
		if item == "" {
			continue
		}
		if ordered {
			fmt.Fprintf(&b, "%d. %s\n", i, item)
			i++
		} else {
			b.WriteString("* ")
			b.WriteString(item)
			b.WriteByte('\n')
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func prefixLines(s, prefix string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		b.WriteString(prefix)
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func wrapUnderline(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	lead, core, trail := edgeSpace(text)
	if core == "" {
		return text
	}
	return lead + "<u>" + core + "</u>" + trail
}

func wrapMark(text, open, close string, dropEmpty bool) string {
	if strings.TrimSpace(text) == "" {
		if dropEmpty {
			return ""
		}
		return text
	}
	lead, core, trail := edgeSpace(text)
	core = trimLineEnds(core)
	lead = collapseSpaces(lead)
	trail = collapseSpaces(trail)
	if strings.TrimSpace(core) == "" {
		if dropEmpty {
			return ""
		}
		return lead + trail
	}
	return lead + open + core + close + trail
}

func edgeSpace(s string) (lead, core, trail string) {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	j := len(s)
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t') {
		j--
	}
	return s[:i], s[i:j], s[j:]
}

func insideMark(n *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		switch p.Data {
		case "u", "s", "del", "strike":
			return true
		}
	}
	return false
}

func trimLineEnds(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.Join(lines, "\n")
}

func textContentHTML(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func collapseSpaces(s string) string {
	if s == "" {
		return ""
	}
	return " "
}

func renderLink(n *html.Node, opt htmlOpt) string {
	text := renderKids(n, opt)
	if strings.TrimSpace(text) == "" {
		return ""
	}
	href := attr(n, "href")
	title := attr(n, "title")
	if href == "" {
		return text
	}
	quoted, ok := quoteHref(resolveURL(opt.base, href))
	if !ok {
		return text
	}
	titlePart := ""
	if title != "" {
		titlePart = ` "` + strings.ReplaceAll(title, `"`, `\"`) + `"`
	}
	return "[" + text + "](" + quoted + titlePart + ")"
}

func renderImg(n *html.Node, opt htmlOpt) string {
	alt := strings.ReplaceAll(attr(n, "alt"), "\n", " ")
	src := attr(n, "src")
	dataSrc := attr(n, "data-src")
	if dataSrc != "" && (src == "" || (isDataURI(src) && !opt.keepData)) {
		src = dataSrc
	}
	if src != "" && !isDataURI(src) {
		src = resolveURL(opt.base, src)
	}
	if dataSrc != "" && !isDataURI(dataSrc) && src == dataSrc {
		src = resolveURL(opt.base, src)
	}
	if isDataURI(src) && !opt.keepData {
		if i := strings.Index(src, ","); i >= 0 {
			src = src[:i] + "..."
		} else {
			src += "..."
		}
	}
	title := attr(n, "title")
	titlePart := ""
	if title != "" {
		titlePart = ` "` + strings.ReplaceAll(title, `"`, `\"`) + `"`
	}
	return "![" + alt + "](" + src + titlePart + ")"
}

func resolveURL(base, ref string) string {
	if ref == "" {
		return base
	}
	if base == "" {
		return ref
	}
	bu, err := url.Parse(base)
	if err != nil {
		return ref
	}
	ru, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return bu.ResolveReference(ru).String()
}

func isDataURI(s string) bool {
	return len(s) >= 5 && strings.EqualFold(s[:5], "data:")
}

func quoteHref(href string) (string, bool) {
	scheme := ""
	rest := href
	if i := strings.Index(href, ":"); i > 0 && !strings.ContainsAny(href[:i], "/?#") {
		scheme = href[:i]
		lower := strings.ToLower(scheme)
		if lower != "http" && lower != "https" && lower != "file" {
			return "", false
		}
	}
	prefix, path, tail := splitHref(rest, scheme)
	return prefix + quotePath(path) + tail, true
}

func splitHref(href, scheme string) (prefix, path, tail string) {
	rest := href
	if scheme != "" {
		i := strings.Index(href, "://")
		if i >= 0 {
			rest = href[i+3:]
			prefix = href[:i+3]
			slash := strings.Index(rest, "/")
			q := strings.IndexAny(rest, "?#")
			if slash < 0 || (q >= 0 && q < slash) {
				if q < 0 {
					return href, "", ""
				}
				prefix += rest[:q]
				return prefix, "", rest[q:]
			}
			prefix += rest[:slash]
			rest = rest[slash:]
		}
	}
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		return prefix, rest[:i], rest[i:]
	}
	return prefix, rest, ""
}

func quotePath(path string) string {
	var b strings.Builder
	for i := 0; i < len(path); {
		if path[i] == '%' && i+2 < len(path) && isHex(path[i+1]) && isHex(path[i+2]) {
			b.WriteString(path[i : i+3])
			i += 3
			continue
		}
		r, size := utf8.DecodeRuneInString(path[i:])
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&b, "%%%02X", path[i])
			i++
			continue
		}
		var buf [4]byte
		n := utf8.EncodeRune(buf[:], r)
		for _, c := range buf[:n] {
			if c == '/' || isUnreserved(c) {
				b.WriteByte(c)
			} else {
				fmt.Fprintf(&b, "%%%02X", c)
			}
		}
		i += size
	}
	return b.String()
}

func isHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func isUnreserved(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || strings.ContainsRune("-._~", rune(b))
}

func rewriteBingLinks(n *html.Node) {
	if n.Type == html.ElementNode && n.Data == "a" {
		for i, a := range n.Attr {
			if a.Key == "href" {
				n.Attr[i].Val = unwrapBing(a.Val)
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		rewriteBingLinks(c)
	}
}

func unwrapBing(href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	raw := u.Query().Get("u")
	if len(raw) < 3 {
		return href
	}
	payload := strings.TrimSpace(raw[2:])
	if payload == "" {
		return href
	}
	if m := len(payload) % 4; m != 0 {
		payload += strings.Repeat("=", 4-m)
	}
	b, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		b, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return href
		}
	}
	if !utf8.Valid(b) {
		return href
	}
	return string(b)
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func textContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			return
		}
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func findHTMLID(n *html.Node, id string) *html.Node {
	if n.Type == html.ElementNode && attr(n, "id") == id {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if f := findHTMLID(c, id); f != nil {
			return f
		}
	}
	return nil
}

func findHTMLTag(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if f := findHTMLTag(c, tag); f != nil {
			return f
		}
	}
	return nil
}

func classTokens(n *html.Node) []string {
	return strings.Fields(attr(n, "class"))
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range classTokens(n) {
		if c == class {
			return true
		}
	}
	return false
}

func findClassToken(n *html.Node, class string) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && hasClass(n, class) {
			out = append(out, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}

func findClassTokenOne(n *html.Node, class string) *html.Node {
	all := findClassToken(n, class)
	if len(all) == 0 {
		return nil
	}
	return all[0]
}

func hasAncestorClass(n *html.Node, class string) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if hasClass(p, class) {
			return true
		}
	}
	return false
}
