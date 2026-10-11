package markitdown

import (
	"archive/zip"
	"encoding/base64"
	"strconv"
	"strings"
)

type docxCtx struct {
	zr   *zip.Reader
	hook ImageHook
	uTag string
	keep bool
	err  error
}

func convertDOCX(data []byte, lim budget) (Result, error) {
	zr, err := openZip(data)
	if err != nil {
		return Result{}, err
	}
	styles := map[string]int{}
	if b, err := readZip(zr, "word/styles.xml"); err == nil {
		b = repairStyles(b)
		if root, err := parseXML(b); err == nil {
			styles = headingStyles(root)
		}
	}
	doc, err := readZip(zr, "word/document.xml")
	if err != nil {
		return Result{}, ErrCorrupt
	}
	root, err := parseXML(doc)
	if err != nil {
		return Result{}, err
	}
	ctx := &docxCtx{
		zr:   zr,
		hook: lim.imageHook,
		uTag: underlineTag(lim.styleMap, embeddedStyleMap(zr)),
		keep: lim.keepData,
	}
	text := renderDocx(root, styles, ctx)
	if ctx.err != nil {
		return Result{}, ctx.err
	}
	if b, err := readZip(zr, "word/comments.xml"); err == nil {
		if croot, err := parseXML(b); err == nil {
			if comments := strings.TrimSpace(renderDocx(croot, styles, ctx)); comments != "" {
				text += "\n\n" + comments
			}
		}
	}
	return Result{Text: strings.TrimSpace(text)}, nil
}

func headingStyles(root *xnode) map[string]int {
	out := map[string]int{}
	var walk func(*xnode)
	walk = func(n *xnode) {
		if n.local == "style" {
			id := n.attr["styleId"]
			name := ""
			outline := -1
			for _, c := range n.kids {
				if c.local == "name" {
					name = c.attr["val"]
				}
				if c.local == "pPr" {
					for _, p := range c.kids {
						if p.local == "outlineLvl" {
							if v, err := strconv.Atoi(p.attr["val"]); err == nil {
								outline = v
							}
						}
					}
				}
			}
			if lvl := headingNameLevel(name); lvl > 0 {
				out[id] = lvl
			} else if outline >= 0 && outline < 6 && id != "" {
				out[id] = outline + 1
			}
		}
		for _, c := range n.kids {
			walk(c)
		}
	}
	walk(root)
	return out
}

func headingNameLevel(name string) int {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.ReplaceAll(n, " ", "")
	for _, prefix := range []string{"heading", "标题"} {
		if rest, ok := strings.CutPrefix(n, prefix); ok && len(rest) == 1 && rest[0] >= '1' && rest[0] <= '6' {
			return int(rest[0] - '0')
		}
	}
	return 0
}

func renderDocx(root *xnode, styles map[string]int, ctx *docxCtx) string {
	var b strings.Builder
	var walk func(*xnode)
	walk = func(n *xnode) {
		switch n.local {
		case "drawing", "pict", "object":
			collectTxbx(n, walk)
			return
		case "txbxContent":
			for _, c := range n.kids {
				walk(c)
			}
			return
		case "oMathPara":
			if s := mathLatex(n); s != "" {
				b.WriteString("$$")
				b.WriteString(s)
				b.WriteString("$$\n\n")
			}
			return
		case "oMath":
			if s := mathLatex(n); s != "" {
				b.WriteString("$")
				b.WriteString(s)
				b.WriteString("$\n\n")
			}
			return
		case "tbl":
			if table := renderDocxTable(n, ctx); table != "" {
				b.WriteString(table)
				b.WriteString("\n\n")
			}
		case "p":
			if line := renderParagraph(n, styles, ctx); line != "" {
				b.WriteString(line)
				b.WriteString("\n\n")
			}
		default:
			for _, c := range n.kids {
				walk(c)
			}
		}
	}
	walk(root)
	return b.String()
}

func renderParagraph(p *xnode, styles map[string]int, ctx *docxCtx) string {
	text := inlineContent(p, ctx)
	if strings.TrimSpace(text) == "" {
		return ""
	}
	if lvl := styles[paragraphStyle(p)]; lvl > 0 {
		return strings.Repeat("#", lvl) + " " + strings.TrimSpace(text)
	}
	return strings.TrimSpace(text)
}

func paragraphStyle(p *xnode) string {
	for _, c := range p.kids {
		if c.local != "pPr" {
			continue
		}
		for _, s := range c.kids {
			if s.local == "pStyle" {
				return s.attr["val"]
			}
		}
	}
	return ""
}

func paragraphText(p *xnode, ctx *docxCtx) string { return inlineContent(p, ctx) }

func inlineContent(n *xnode, ctx *docxCtx) string {
	var b strings.Builder
	var walk func(*xnode)
	walk = func(n *xnode) {
		switch n.local {
		case "oMathPara":
			if s := mathLatex(n); s != "" {
				b.WriteString("$$")
				b.WriteString(s)
				b.WriteString("$$")
			}
		case "oMath":
			if s := mathLatex(n); s != "" {
				b.WriteString("$")
				b.WriteString(s)
				b.WriteString("$")
			}
		case "drawing", "pict", "object":
			if s := docxDrawingImages(n, ctx); s != "" {
				b.WriteString(s)
			}
			collectTxbx(n, walk)
		case "txbxContent":
			for _, c := range n.kids {
				walk(c)
			}
		case "tbl":
			return
		case "t", "delText":
			b.WriteString(underlineText(n, n.text, ctx))
		case "tab":
			b.WriteString(underlineText(n, " ", ctx))
		case "br", "cr":
			b.WriteString(underlineText(n, "\n", ctx))
		default:
			for _, c := range n.kids {
				walk(c)
			}
		}
	}
	walk(n)
	return b.String()
}

func collectTxbx(n *xnode, walk func(*xnode)) {
	if n.local == "txbxContent" {
		for _, c := range n.kids {
			walk(c)
		}
		return
	}
	for _, c := range n.kids {
		collectTxbx(c, walk)
	}
}

func renderDocxTable(tbl *xnode, ctx *docxCtx) string {
	var rows [][]string
	for _, tr := range direct(tbl, "tr") {
		var row []string
		for _, tc := range direct(tr, "tc") {
			var parts []string
			var walk func(*xnode)
			walk = func(n *xnode) {
				switch n.local {
				case "tbl":
					return
				case "p":
					if t := strings.TrimSpace(paragraphText(n, ctx)); t != "" {
						parts = append(parts, t)
					}
				default:
					for _, c := range n.kids {
						walk(c)
					}
				}
			}
			walk(tc)
			row = append(row, strings.Join(parts, "\n"))
		}
		rows = append(rows, row)
	}
	return markdownTable(rows)
}

func underlineTag(caller, embedded string) string {
	for _, part := range []string{caller, embedded, "u => u"} {
		for _, line := range strings.Split(part, "\n") {
			line = strings.TrimSpace(line)
			if rest, ok := strings.CutPrefix(line, "u =>"); ok {
				return strings.TrimSpace(rest)
			}
		}
	}
	return "u"
}

func embeddedStyleMap(zr *zip.Reader) string {
	b, err := readZip(zr, "mammoth/style-map")
	if err != nil {
		return ""
	}
	return string(b)
}

func underlineText(n *xnode, text string, ctx *docxCtx) string {
	if !runHasUnderline(n) || blankRun(text) {
		return text
	}
	tag := "u"
	if ctx != nil && ctx.uTag != "" {
		tag = ctx.uTag
	}
	switch tag {
	case "em":
		return "*" + text + "*"
	case "strong":
		return "**" + text + "**"
	default:
		return "<u>" + text + "</u>"
	}
}

func blankRun(s string) bool {
	if s == "" {
		return true
	}
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' && r != '\u00a0' {
			return false
		}
	}
	return true
}

func runHasUnderline(n *xnode) bool {
	for p := n; p != nil; p = p.parent {
		if p.local != "r" {
			continue
		}
		for _, c := range p.kids {
			if c.local != "rPr" {
				continue
			}
			for _, prop := range c.kids {
				if prop.local == "u" {
					return true
				}
			}
		}
		return false
	}
	return false
}

func docxDrawingImages(n *xnode, ctx *docxCtx) string {
	if ctx == nil || ctx.err != nil {
		return ""
	}
	var b strings.Builder
	for _, blip := range findAll(n, "blip") {
		rid := blip.attr["embed"]
		if rid == "" {
			continue
		}
		part := docxRelTarget(ctx.zr, rid)
		if part == "" {
			if ctx.hook != nil {
				ctx.err = corruptf("missing image relationship %s", rid)
			}
			return b.String()
		}
		blob, err := readZip(ctx.zr, part)
		if err != nil {
			if ctx.hook != nil {
				ctx.err = err
			}
			return b.String()
		}
		defaults, overrides := contentTypes(ctx.zr)
		info := imageInfo(part, blob, defaults, overrides)
		alt := picAlt(n)
		if alt == "" {
			alt = info.Name
		}
		if ctx.hook == nil {
			b.WriteString(imageMarkdown(alt, info, ctx.keep))
			continue
		}
		fragment, err := ctx.hook(info)
		if err != nil {
			ctx.err = err
			return b.String()
		}
		inner, err := imageFragment(fragment)
		if err != nil {
			ctx.err = err
			return b.String()
		}
		if inner == "" {
			b.WriteString(imageMarkdown(alt, info, ctx.keep))
			continue
		}
		res, err := convertHTML([]byte(inner), htmlOpt{})
		if err != nil {
			ctx.err = err
			return b.String()
		}
		b.WriteString(res.Text)
	}
	return b.String()
}

func docxRelTarget(zr *zip.Reader, id string) string {
	for _, rel := range zipRels(zr, "word/_rels/document.xml.rels", "image") {
		if rel.id == id {
			return rel.target
		}
	}
	return ""
}

func picAlt(n *xnode) string {
	for _, pr := range findAll(n, "cNvPr") {
		if pr.attr["descr"] != "" {
			return pr.attr["descr"]
		}
		if pr.attr["name"] != "" {
			return pr.attr["name"]
		}
	}
	return ""
}

func imageMarkdown(alt string, img EmbeddedImage, keep bool) string {
	mime := img.MIME
	if mime == "" {
		mime = "application/octet-stream"
	}
	uri := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(img.Data)
	if !keep {
		if i := strings.Index(uri, ","); i >= 0 {
			uri = uri[:i] + "..."
		}
	}
	return "![" + alt + "](" + uri + ")"
}
