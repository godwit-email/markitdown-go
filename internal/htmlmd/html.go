package htmlmd

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/godwit-email/markitdown-go/internal/run"
	"golang.org/x/net/html"
)

type Options struct {
	KeepData      bool
	Base          string
	ClosedHeading bool
	PipeTables    bool
	Inline        bool
}

func Convert(data []byte, opt Options) (string, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return "", run.Wrap(err)
	}
	root := findHTMLTag(doc, "body")
	if root == nil {
		root = doc
	}
	return strings.TrimSpace(renderBlock(root, opt)), nil
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

func renderBlock(n *html.Node, opt Options) string {
	var parts []string
	var inline strings.Builder
	flush := func() {
		s := inline.String()
		inline.Reset()
		if strings.TrimSpace(s) == "" {
			return
		}
		parts = append(parts, s)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if isHTMLBlock(c) {
			flush()
			if s := renderNode(c, opt); s != "" {
				parts = append(parts, s)
			}
			continue
		}
		if c.Type == html.TextNode && strings.TrimSpace(c.Data) == "" && (isHTMLBlock(c.PrevSibling) || isHTMLBlock(c.NextSibling)) {
			continue
		}
		inline.WriteString(renderNode(c, opt))
	}
	flush()
	return joinMDChildren(parts)
}

func isHTMLBlock(n *html.Node) bool {
	if n == nil || n.Type != html.ElementNode {
		return false
	}
	switch n.Data {
	case "address", "article", "aside", "blockquote", "dd", "div", "dl", "dt", "fieldset", "figcaption", "figure", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "li", "main", "nav", "ol", "p", "pre", "section", "table", "tbody", "thead", "tfoot", "tr", "ul", "center":
		return true
	default:
		return false
	}
}

var (
	nlWS   = regexp.MustCompile(`[\t \r\n]*[\r\n][\t \r\n]*`)
	spWS   = regexp.MustCompile(`[\t ]+`)
	cellNL = regexp.MustCompile(`\n+`)
)

func normalizeText(s string) string {
	s = nlWS.ReplaceAllString(s, "\n")
	return spWS.ReplaceAllString(s, " ")
}

func wrapCode(text string) string {
	prefix, core, suffix := chompMD(text)
	if core == "" {
		return ""
	}
	max := 0
	run := 0
	for _, r := range core {
		if r == '`' {
			run++
			if run > max {
				max = run
			}
			continue
		}
		run = 0
	}
	delim := strings.Repeat("`", max+1)
	if max > 0 {
		core = " " + core + " "
	}
	return prefix + delim + core + delim + suffix
}

func renderNoscript(n *html.Node, opt Options) string {
	var raw strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			raw.WriteString(c.Data)
		}
	}
	if raw.Len() == 0 {
		return renderKids(n, opt)
	}
	doc, err := html.Parse(strings.NewReader("<html><body><div>" + raw.String() + "</div></body></html>"))
	if err != nil {
		return raw.String()
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				b.WriteString(renderNode(c, opt))
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return b.String()
}

func escapeMD(s string) string {
	s = strings.ReplaceAll(s, "*", `\*`)
	return strings.ReplaceAll(s, "_", `\_`)
}

func inPre(n *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && (p.Data == "pre" || p.Data == "code" || p.Data == "kbd" || p.Data == "samp") {
			return true
		}
	}
	return false
}

func renderNode(n *html.Node, opt Options) string {
	if n.Type == html.TextNode {
		if inPre(n) {
			return n.Data
		}
		s := normalizeText(n.Data)
		if isHTMLBlock(n.PrevSibling) || (n.PrevSibling == nil && isHTMLBlock(n.Parent)) {
			s = strings.TrimLeftFunc(s, unicode.IsSpace)
		}
		if isHTMLBlock(n.NextSibling) || (n.NextSibling == nil && isHTMLBlock(n.Parent)) {
			s = strings.TrimRightFunc(s, unicode.IsSpace)
		}
		return escapeMD(s)
	}
	if n.Type != html.ElementNode {
		return renderKids(n, opt)
	}
	if base := attr(n, "xml:base"); base != "" {
		opt.Base = resolveURL(opt.Base, base)
	}
	switch n.Data {
	case "noscript":
		return renderNoscript(n, opt)
	case "script", "style", "svg", "head":
		return ""
	case "input":
		if strings.EqualFold(attr(n, "type"), "checkbox") {
			if hasAttr(n, "checked") {
				return "[x] "
			}
			return "[ ] "
		}
		return ""
	case "sup", "sub":
		_, core, _ := chompMD(renderKids(n, opt))
		return core
	case "dt":
		text := strings.Join(strings.Fields(renderKids(n, opt)), " ")
		if opt.Inline {
			return " " + text + " "
		}
		if text == "" {
			return "\n"
		}
		return "\n\n" + text + "\n"
	case "dd":
		text := strings.TrimSpace(renderKids(n, opt))
		if opt.Inline {
			return " " + text + " "
		}
		if text == "" {
			return "\n"
		}
		var b strings.Builder
		for i, line := range strings.Split(text, "\n") {
			if i == 0 {
				b.WriteString(":   ")
				b.WriteString(line)
			} else if line != "" {
				b.WriteString("    ")
				b.WriteString(line)
			}
			b.WriteByte('\n')
		}
		return b.String()
	case "q":
		return `"` + renderKids(n, opt) + `"`
	case "hr":
		return "\n\n---\n\n"
	case "br":
		if opt.Inline {
			return " "
		}
		if insideMark(n) {
			return "\n"
		}
		return "  \n"
	case "code", "kbd", "samp":
		return wrapCode(renderKids(n, opt))
	case "pre":
		return "```\n" + strings.Trim(textContentHTML(n), "\n") + "\n```"
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(n.Data[1] - '0')
		inner := strings.TrimSpace(renderKids(n, opt))
		if opt.Inline {
			return inner
		}
		line := strings.Repeat("#", level)
		if inner != "" {
			line += " " + inner
		} else {
			line += " "
		}
		if opt.ClosedHeading && inner != "" {
			line += " " + strings.Repeat("#", level)
		}
		return "\n\n" + line + "\n\n"
	case "table":
		if opt.PipeTables {
			return renderHTMLTable(n, opt)
		}
		return renderBlock(n, opt)
	case "p", "div", "section", "article", "main", "center", "header", "footer", "nav", "figure", "figcaption", "tbody", "thead", "tfoot", "tr", "dl":
		inner := strings.TrimSpace(renderBlock(n, opt))
		if opt.Inline {
			return " " + inner + " "
		}
		if inner == "" {
			return ""
		}
		return "\n\n" + inner + "\n\n"
	case "blockquote":
		return prefixLines(strings.Trim(renderBlock(n, opt), " \t\r\n"), "> ")
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

func renderHTMLTable(n *html.Node, opt Options) string {
	type cell struct {
		text string
		span int
	}
	var rowCells [][]cell
	var firstTR *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			if firstTR == nil {
				firstTR = n
			}
			var cells []cell
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type != html.ElementNode || (c.Data != "td" && c.Data != "th") {
					continue
				}
				cellOpt := opt
				cellOpt.Inline = true
				text := strings.TrimSpace(strings.ReplaceAll(renderKids(c, cellOpt), "\n", " "))
				span := 1
				if v := attr(c, "colspan"); v != "" {
					if _, err := fmt.Sscanf(v, "%d", &span); err != nil || span < 1 {
						span = 1
					}
					if span > 1000 {
						span = 1000
					}
				}
				cells = append(cells, cell{text: text, span: span})
			}
			if len(cells) > 0 {
				rowCells = append(rowCells, cells)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "table" {
				continue
			}
			walk(c)
		}
	}
	var caption string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "caption" {
			caption = strings.TrimSpace(renderKids(c, opt))
		}
	}
	walk(n)
	if len(rowCells) == 0 {
		return ""
	}
	width := 0
	for _, row := range rowCells {
		n := 0
		for _, c := range row {
			n += c.span
		}
		if n > width {
			width = n
		}
	}
	writeRow := func(row []cell) string {
		var b strings.Builder
		b.WriteByte('|')
		used := 0
		for _, c := range row {
			b.WriteByte(' ')
			b.WriteString(c.text)
			b.WriteString(strings.Repeat(" |", c.span))
			used += c.span
		}
		for used < width {
			b.WriteString(" |")
			used++
		}
		return b.String()
	}
	header := false
	headerWidth := width
	if firstTR != nil {
		desc := descendantCells(firstTR)
		header = len(desc) > 0
		span := 0
		for _, c := range desc {
			if c.Data != "th" {
				header = false
			}
			span += cellSpan(c)
		}
		if span > headerWidth {
			headerWidth = span
		}
	}
	sep := "| " + strings.Join(repeat("---", headerWidth), " | ") + " |"
	var b strings.Builder
	if caption != "" {
		b.WriteString(caption)
		b.WriteString("\n\n")
	}
	if !header {
		b.WriteString("| " + strings.Join(repeat("", headerWidth), " | ") + " |")
		b.WriteByte('\n')
		b.WriteString(sep)
		b.WriteByte('\n')
	}
	for i, row := range rowCells {
		b.WriteString(writeRow(row))
		b.WriteByte('\n')
		if i == 0 && header {
			b.WriteString(sep)
			b.WriteByte('\n')
		}
	}
	return "\n\n" + strings.TrimRight(b.String(), "\n") + "\n\n"
}

func cellSpan(n *html.Node) int {
	span := 1
	if v := attr(n, "colspan"); v != "" {
		if _, err := fmt.Sscanf(v, "%d", &span); err != nil || span < 1 {
			return 1
		}
		if span > 1000 {
			return 1000
		}
	}
	return span
}

func descendantCells(tr *html.Node) []*html.Node {
	var cells []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
				cells = append(cells, c)
			}
			if c.Type == html.ElementNode {
				walk(c)
			}
		}
	}
	walk(tr)
	return cells
}

func repeat(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

func rowIsHeader(table *html.Node) bool {
	var first *html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if first != nil {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "tr" {
				first = c
				return
			}
			if c.Type == html.ElementNode && (c.Data == "thead" || c.Data == "tbody" || c.Data == "tfoot") {
				find(c)
			}
		}
	}
	find(table)
	if first == nil {
		return false
	}
	if first.Parent != nil && first.Parent.Data == "thead" {
		return true
	}
	saw := false
	for c := first.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || (c.Data != "td" && c.Data != "th") {
			continue
		}
		saw = true
		if c.Data != "th" {
			return false
		}
	}
	return saw
}

func renderKids(n *html.Node, opt Options) string {
	var parts []string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode && strings.TrimSpace(c.Data) == "" && (isHTMLBlock(c.PrevSibling) || isHTMLBlock(c.NextSibling)) {
			continue
		}
		if s := renderNode(c, opt); s != "" {
			parts = append(parts, s)
		}
	}
	return joinMDChildren(parts)
}

func joinMDChildren(parts []string) string {
	updated := []string{""}
	for _, child := range parts {
		leading, content, trailing := splitNL(child)
		if updated[len(updated)-1] != "" && leading != "" {
			prevTrail := updated[len(updated)-1]
			updated = updated[:len(updated)-1]
			n := len(prevTrail)
			if len(leading) > n {
				n = len(leading)
			}
			if n > 2 {
				n = 2
			}
			leading = strings.Repeat("\n", n)
		}
		updated = append(updated, leading, content, trailing)
	}
	return strings.Join(updated, "")
}

func splitNL(s string) (leading, content, trailing string) {
	i := 0
	for i < len(s) && s[i] == '\n' {
		i++
	}
	j := len(s)
	for j > i && s[j-1] == '\n' {
		j--
	}
	return s[:i], s[i:j], s[j:]
}

func renderList(n *html.Node, ordered bool, opt Options) string {
	var b strings.Builder
	i := 1
	if ordered {
		if v := attr(n, "start"); v != "" {
			if _, err := fmt.Sscanf(v, "%d", &i); err != nil || i < 1 {
				i = 1
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.Data != "li" {
			continue
		}
		item := strings.TrimSpace(renderKids(c, opt))
		if item == "" {
			if ordered {
				i++
			}
			continue
		}
		var bullet string
		if ordered {
			bullet = fmt.Sprintf("%d. ", i)
			i++
		} else {
			bullets := "*+-"
			depth := ulDepth(c)
			if depth < 0 {
				depth = 0
			}
			bullet = string(bullets[depth%len(bullets)]) + " "
		}
		lines := strings.Split(item, "\n")
		pad := strings.Repeat(" ", len(bullet))
		for j, line := range lines {
			if j == 0 {
				b.WriteString(bullet)
			} else if line != "" {
				b.WriteString(pad)
			}
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	body := strings.TrimRight(b.String(), "\n")
	if insideLi(n) {
		return "\n" + body
	}
	return "\n\n" + body
}

func ulDepth(n *html.Node) int {
	depth := -1
	for p := n; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && p.Data == "ul" {
			depth++
		}
	}
	return depth
}

func insideLi(n *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && p.Data == "li" {
			return true
		}
	}
	return false
}

func prefixLines(s, prefix string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			b.WriteByte('>')
		} else {
			b.WriteString(prefix)
			b.WriteString(line)
		}
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

func chompMD(s string) (prefix, core, suffix string) {
	if s != "" && s[0] == ' ' {
		prefix = " "
	}
	if s != "" && s[len(s)-1] == ' ' {
		suffix = " "
	}
	return prefix, strings.TrimSpace(s), suffix
}

func renderLink(n *html.Node, opt Options) string {
	prefix, text, suffix := chompMD(renderKids(n, opt))
	if text == "" {
		return ""
	}
	href := attr(n, "href")
	title := attr(n, "title")
	if href == "" {
		return prefix + text + suffix
	}
	quoted, ok := quoteHref(resolveURL(opt.Base, href))
	if !ok || quoted == "" || quoted == "#" {
		return prefix + text + suffix
	}
	if title == "" && strings.ReplaceAll(text, `\_`, "_") == quoted {
		return prefix + "<" + quoted + ">" + suffix
	}
	titlePart := ""
	if title != "" {
		titlePart = ` "` + strings.ReplaceAll(title, `"`, `\"`) + `"`
	}
	return prefix + "[" + text + "](" + quoted + titlePart + ")" + suffix
}

func renderImg(n *html.Node, opt Options) string {
	alt := strings.ReplaceAll(attr(n, "alt"), "\n", " ")
	src := attr(n, "src")
	dataSrc := attr(n, "data-src")
	if dataSrc != "" && (src == "" || (isDataURI(src) && !opt.KeepData)) {
		src = dataSrc
	}
	if src != "" && !isDataURI(src) {
		src = resolveURL(opt.Base, src)
	}
	if dataSrc != "" && !isDataURI(dataSrc) && src == dataSrc {
		src = resolveURL(opt.Base, src)
	}
	if isDataURI(src) && !opt.KeepData {
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

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
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
