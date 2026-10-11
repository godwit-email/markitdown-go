package markitdown

import (
	"net/url"
	"path"
	"strings"
)

const dcNS = "http://purl.org/dc/elements/1.1/"

func convertEPUB(data []byte, keepData bool) (Result, error) {
	zr, err := openZip(data)
	if err != nil {
		return Result{}, err
	}
	container, err := readZip(zr, "META-INF/container.xml")
	if err != nil {
		return Result{}, ErrCorrupt
	}
	root, err := parseXML(container)
	if err != nil {
		return Result{}, err
	}
	opfPath := ""
	if rf := findLocal(root, "rootfile"); rf != nil {
		opfPath = rf.attr["full-path"]
	}
	if opfPath == "" {
		return Result{}, ErrCorrupt
	}
	opfBytes, err := readZip(zr, opfPath)
	if err != nil {
		return Result{}, ErrCorrupt
	}
	opf, err := parseXML(opfBytes)
	if err != nil {
		return Result{}, err
	}
	meta := epubMeta(opf)
	base := path.Dir(opfPath)
	if base == "." {
		base = ""
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	var chapters []string
	for _, idref := range epubSpine(opf) {
		href := epubManifest(opf)[idref]
		if href == "" {
			continue
		}
		name := resolveEPUBHref(href, base, names)
		body, err := readZip(zr, name)
		if err != nil {
			continue
		}
		html, err := convertHTML(body, htmlOpt{keepData: keepData})
		if err != nil {
			continue
		}
		if s := strings.TrimSpace(html.Text); s != "" {
			chapters = append(chapters, s)
		}
	}
	var parts []string
	if s := formatEPUBMeta(meta); s != "" {
		parts = append(parts, s)
	}
	parts = append(parts, chapters...)
	return Result{Text: strings.Join(parts, "\n\n"), Title: meta["title"]}, nil
}

func epubMeta(opf *xnode) map[string]string {
	out := map[string]string{}
	authors := epubDCTexts(opf, "creator")
	if len(authors) > 0 {
		out["authors"] = strings.Join(authors, ", ")
	}
	for _, key := range []string{"title", "language", "publisher", "date", "description", "identifier"} {
		if vals := epubDCTexts(opf, key); len(vals) > 0 {
			out[key] = vals[0]
		}
	}
	return out
}

func epubDCTexts(root *xnode, local string) []string {
	var out []string
	var walk func(*xnode)
	walk = func(n *xnode) {
		if n.local == local && (n.space == dcNS || n.space == "" || strings.Contains(n.space, "purl.org/dc")) {
			if t := strings.TrimSpace(plainNodeText(n)); t != "" {
				out = append(out, t)
			}
			return
		}
		for _, c := range n.kids {
			walk(c)
		}
	}
	walk(root)
	return out
}

func plainNodeText(n *xnode) string {
	var b strings.Builder
	if len(n.kids) == 0 {
		return n.text
	}
	for _, c := range n.kids {
		if c.local == "#text" {
			b.WriteString(c.text)
			continue
		}
		b.WriteString(plainNodeText(c))
	}
	return b.String()
}

func epubManifest(opf *xnode) map[string]string {
	out := map[string]string{}
	var walk func(*xnode)
	walk = func(n *xnode) {
		if n.local == "item" && n.attr["id"] != "" {
			out[n.attr["id"]] = n.attr["href"]
		}
		for _, c := range n.kids {
			walk(c)
		}
	}
	walk(opf)
	return out
}

func epubSpine(opf *xnode) []string {
	var out []string
	var walk func(*xnode)
	walk = func(n *xnode) {
		if n.local == "itemref" {
			if id := n.attr["idref"]; id != "" {
				out = append(out, id)
			}
			return
		}
		for _, c := range n.kids {
			walk(c)
		}
	}
	walk(opf)
	return out
}

func resolveEPUBHref(href, base string, names map[string]bool) string {
	decoded, _ := url.PathUnescape(href)
	var candidates []string
	for _, cand := range []string{decoded, href} {
		resolved := cand
		if base != "" {
			resolved = path.Clean(path.Join(base, cand))
		} else {
			resolved = path.Clean(cand)
		}
		if len(candidates) == 0 || candidates[len(candidates)-1] != resolved {
			candidates = append(candidates, resolved)
		}
	}
	for _, c := range candidates {
		if names[c] {
			return c
		}
	}
	if len(candidates) == 0 {
		return href
	}
	return candidates[0]
}

func formatEPUBMeta(meta map[string]string) string {
	var b strings.Builder
	for _, key := range []string{"title", "authors", "language", "publisher", "date", "description", "identifier"} {
		if meta[key] == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		label := key
		if label != "" {
			label = strings.ToUpper(label[:1]) + label[1:]
		}
		b.WriteString("**")
		b.WriteString(label)
		b.WriteString(":** ")
		b.WriteString(meta[key])
	}
	return b.String()
}

func findLocal(n *xnode, local string) *xnode {
	if n == nil {
		return nil
	}
	if n.local == local {
		return n
	}
	for _, c := range n.kids {
		if f := findLocal(c, local); f != nil {
			return f
		}
	}
	return nil
}
