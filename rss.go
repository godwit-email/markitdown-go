package markitdown

import (
	"strings"
)

const (
	atomNS    = "http://www.w3.org/2005/Atom"
	xhtmlNS   = "http://www.w3.org/1999/xhtml"
	contentNS = "http://purl.org/rss/1.0/modules/content/"
)

func convertRSS(data []byte, base string) (Result, error) {
	root, err := parseXML(data)
	if err != nil {
		return Result{}, err
	}
	switch feedType(root) {
	case "rss":
		return parseRSS(root, base)
	case "atom":
		return parseAtom(root, base)
	default:
		return Result{}, ErrUnsupported
	}
}

func feedType(root *xnode) string {
	if root.local == "rss" {
		return "rss"
	}
	if root.local == "feed" && (root.space == "" || root.space == atomNS) && len(directNS(root, "entry", root.space)) > 0 {
		return "atom"
	}
	return ""
}

func directNS(n *xnode, local, space string) []*xnode {
	var out []*xnode
	for _, c := range n.kids {
		if c.local != local {
			continue
		}
		if space != "" && c.space != space {
			continue
		}
		if space == "" && c.space != "" && c.space != atomNS {
			continue
		}
		out = append(out, c)
	}
	return out
}

func firstNS(n *xnode, local, space string) *xnode {
	kids := directNS(n, local, space)
	if len(kids) == 0 {
		return nil
	}
	return kids[0]
}

func parseAtom(root *xnode, base string) (Result, error) {
	space := root.space
	title := flattenAtom(root, "title", space, base)
	subtitle := flattenAtom(root, "subtitle", space, base)
	var b strings.Builder
	if title != "" {
		b.WriteString("# ")
		b.WriteString(title)
		b.WriteByte('\n')
	}
	if subtitle != "" {
		b.WriteString(subtitle)
		b.WriteByte('\n')
	}
	for _, entry := range directNS(root, "entry", space) {
		entryTitle := flattenAtom(entry, "title", space, base)
		updated := flattenPlain(entry, "updated", space)
		summary, summaryMarkup := atomContent(entry, "summary", space)
		content, contentMarkup := atomContent(entry, "content", space)
		if entryTitle != "" {
			b.WriteString("\n## ")
			b.WriteString(entryTitle)
			b.WriteByte('\n')
		}
		if updated != "" {
			b.WriteString("Updated on: ")
			b.WriteString(updated)
			b.WriteByte('\n')
		}
		var parts []string
		if summary != "" {
			parts = append(parts, renderAtomBody(summary, summaryMarkup, fieldBase(entry, "summary", space, base)))
		}
		if content != "" {
			parts = append(parts, renderAtomBody(content, contentMarkup, fieldBase(entry, "content", space, base)))
		}
		body := strings.Join(nonEmpty(parts), "\n\n")
		if body != "" && b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteString("\n\n")
		}
		b.WriteString(body)
	}
	return Result{Text: strings.TrimSpace(b.String()), Title: title}, nil
}

func parseRSS(root *xnode, base string) (Result, error) {
	channel := firstNS(root, "channel", root.space)
	if channel == nil {
		channel = firstNS(root, "channel", "")
	}
	if channel == nil {
		return Result{}, ErrCorrupt
	}
	space := channel.space
	title := flattenPlain(channel, "title", space)
	description := readField(channel, "description", space, "text")
	var b strings.Builder
	if title != "" {
		b.WriteString("# ")
		b.WriteString(title)
		b.WriteByte('\n')
	}
	if description != "" {
		b.WriteString(description)
		b.WriteByte('\n')
	}
	for _, item := range directNS(channel, "item", space) {
		itemTitle := flattenPlain(item, "title", space)
		pub := flattenPlain(item, "pubDate", space)
		desc := readField(item, "description", space, "html")
		encoded := readEncoded(item)
		if itemTitle != "" {
			b.WriteString("\n## ")
			b.WriteString(itemTitle)
			b.WriteByte('\n')
		}
		if pub != "" {
			b.WriteString("Published on: ")
			b.WriteString(pub)
			b.WriteByte('\n')
		}
		var parts []string
		if desc != "" {
			parts = append(parts, htmlField(desc, fieldBase(item, "description", space, base)))
		}
		if encoded != "" {
			parts = append(parts, htmlField(encoded, fieldBaseEncoded(item, base)))
		}
		body := strings.Join(nonEmpty(parts), "\n\n")
		if body != "" && b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteString("\n\n")
		}
		b.WriteString(body)
	}
	return Result{Text: strings.TrimSpace(b.String()), Title: title}, nil
}

func flattenPlain(parent *xnode, local, space string) string {
	raw := readField(parent, local, space, "text")
	return squashLines(raw)
}

func flattenAtom(parent *xnode, local, space, base string) string {
	value, markup := atomContent(parent, local, space)
	if value == "" {
		return ""
	}
	if markup {
		res, err := convertHTML([]byte(value), htmlOpt{base: fieldBase(parent, local, space, base)})
		if err == nil {
			value = res.Text
		}
	}
	return squashLines(value)
}

func squashLines(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	var parts []string
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			parts = append(parts, line)
		}
	}
	return strings.Join(parts, " ")
}

func atomContent(parent *xnode, local, space string) (string, bool) {
	node := firstNS(parent, local, space)
	if node == nil {
		return "", true
	}
	kind := atomKind(node.attr["type"], local == "content")
	if kind == "binary" {
		return "", true
	}
	text := readNode(node, kind)
	if kind != "text" {
		if strings.TrimSpace(text) == "" {
			return "", true
		}
		return text, true
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 1 {
		text = lines[0] + "\n" + dedent(strings.Join(lines[1:], "\n"))
	}
	return strings.TrimSpace(text), false
}

func atomKind(contentType string, allowMedia bool) string {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	switch contentType {
	case "", "text":
		return "text"
	case "html", "xhtml":
		return contentType
	}
	if !allowMedia {
		return "text"
	}
	media := contentType
	if i := strings.Index(media, ";"); i >= 0 {
		media = strings.TrimSpace(media[:i])
	}
	switch {
	case media == "text/html":
		return "html"
	case strings.HasSuffix(media, "+xml") || strings.HasSuffix(media, "/xml"):
		return "xhtml"
	case strings.HasPrefix(media, "text/"):
		return "text"
	default:
		return "binary"
	}
}

func renderAtomBody(value string, markup bool, base string) string {
	if !markup {
		return value
	}
	return htmlField(value, base)
}

func htmlField(value, base string) string {
	res, err := convertHTML([]byte("<div>"+value+"</div>"), htmlOpt{base: base})
	if err != nil {
		return value
	}
	return strings.TrimSpace(res.Text)
}

func readField(parent *xnode, local, space, kind string) string {
	node := firstNS(parent, local, space)
	if node == nil && space != "" {
		node = firstNS(parent, local, "")
	}
	if node == nil {
		return ""
	}
	return readNode(node, kind)
}

func readEncoded(item *xnode) string {
	for _, c := range item.kids {
		if c.local == "encoded" && c.space == contentNS {
			return readNode(c, "html")
		}
	}
	return ""
}

func readNode(n *xnode, kind string) string {
	var b strings.Builder
	for _, c := range n.kids {
		writeNode(&b, n, c, kind)
	}
	if b.Len() == 0 {
		return n.text
	}
	return b.String()
}

func writeNode(b *strings.Builder, parent, n *xnode, kind string) {
	if n.local == "#text" {
		if kind == "text" || (kind == "html" && parent != nil) {
			b.WriteString(n.text)
			return
		}
		b.WriteString(escapeText(n.text))
		return
	}
	if kind == "text" {
		if isBreakElem(n.local) {
			b.WriteByte('\n')
		}
		for _, c := range n.kids {
			writeNode(b, n, c, kind)
		}
		if isBreakElem(n.local) {
			b.WriteByte('\n')
		}
		return
	}
	name := n.local
	if n.space != "" && n.space != xhtmlNS {
		name = n.local
	}
	var attrs []string
	for k, v := range n.attr {
		attrs = append(attrs, ` `+k+`="`+escapeText(v)+`"`)
	}
	// map iteration is random; stable enough for tests that check text not attr order
	if len(n.kids) == 0 {
		b.WriteString("<" + name + strings.Join(attrs, "") + "/>")
		return
	}
	b.WriteString("<" + name + strings.Join(attrs, "") + ">")
	for _, c := range n.kids {
		if c.local == "#text" && n.space == xhtmlNS {
			b.WriteString(escapeText(c.text))
			continue
		}
		writeNode(b, n, c, kind)
	}
	b.WriteString("</" + name + ">")
}

func isBreakElem(local string) bool {
	switch local {
	case "address", "article", "aside", "blockquote", "br", "dd", "div", "dl", "dt", "figcaption", "figure", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "li", "main", "nav", "ol", "p", "pre", "section", "table", "td", "th", "tr", "ul":
		return true
	default:
		return false
	}
}

func escapeText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func dedent(s string) string {
	lines := strings.Split(s, "\n")
	indent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		n := len(line) - len(strings.TrimLeft(line, " \t"))
		if indent < 0 || n < indent {
			indent = n
		}
	}
	if indent <= 0 {
		return s
	}
	for i, line := range lines {
		if len(line) >= indent {
			lines[i] = line[indent:]
		}
	}
	return strings.Join(lines, "\n")
}

func fieldBase(parent *xnode, local, space, docBase string) string {
	node := firstNS(parent, local, space)
	return xmlBase(node, docBase)
}

func fieldBaseEncoded(item *xnode, docBase string) string {
	for _, c := range item.kids {
		if c.local == "encoded" && c.space == contentNS {
			return xmlBase(c, docBase)
		}
	}
	return docBase
}

func xmlBase(node *xnode, docBase string) string {
	var chain []*xnode
	for n := node; n != nil; n = n.parent {
		chain = append(chain, n)
	}
	base := docBase
	for i := len(chain) - 1; i >= 0; i-- {
		if b, ok := chain[i].attr["xml:base"]; ok {
			base = resolveURL(base, b)
		}
	}
	return base
}

func nonEmpty(parts []string) []string {
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}
