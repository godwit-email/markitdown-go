package docx

import (
	"context"
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/godwit-email/markitdown-go"
	"github.com/godwit-email/markitdown-go/internal/ooxml"
	"github.com/godwit-email/markitdown-go/internal/run"
	"github.com/godwit-email/markitdown-go/internal/table"
	"github.com/godwit-email/markitdown-go/internal/xmlsafe"
)

type docxCtx struct {
	zr   *ooxml.Package
	uTag string
	keep bool
	ocr  *ocrSession
	err  error
}

func convertDOCX(ctx context.Context, in markitdown.Input) (markitdown.Result, error) {
	opt, _ := ctx.Value(docxOpt{}).(Options)
	zr, err := ooxml.Open(in.Data)
	if err != nil {
		return markitdown.Result{}, err
	}
	styles := map[string]int{}
	if b, err := zr.Read("word/styles.xml"); err == nil {
		b = repairStyles(b)
		if root, err := xmlsafe.Parse(b); err == nil {
			styles = headingStyles(root)
		}
	}
	doc, err := zr.Read("word/document.xml")
	if err != nil {
		return markitdown.Result{}, run.ErrCorrupt
	}
	root, err := xmlsafe.Parse(doc)
	if err != nil {
		return markitdown.Result{}, err
	}
	dctx := &docxCtx{
		zr:   zr,
		uTag: underlineTag(opt.StyleMap, embeddedStyleMap(zr)),
		keep: opt.KeepData,
		ocr:  newOCRSession(ctx, in.OCR),
	}
	text := renderDocx(root, styles, dctx)
	if dctx.err != nil {
		return markitdown.Result{}, dctx.err
	}
	if b, err := zr.Read("word/comments.xml"); err == nil {
		if croot, err := xmlsafe.Parse(b); err == nil {
			if comments := strings.TrimSpace(renderDocx(croot, styles, dctx)); comments != "" {
				text += "\n\n" + comments
			}
		}
	}
	return markitdown.Result{Text: strings.TrimSpace(text)}, nil
}

func headingStyles(root *xmlsafe.Node) map[string]int {
	out := map[string]int{}
	var walk func(*xmlsafe.Node)
	walk = func(n *xmlsafe.Node) {
		if n.Local == "style" {
			id := n.Attr["styleId"]
			name := ""
			outline := -1
			for _, c := range n.Kids {
				if c.Local == "name" {
					name = c.Attr["val"]
				}
				if c.Local == "pPr" {
					for _, p := range c.Kids {
						if p.Local == "outlineLvl" {
							if v, err := strconv.Atoi(p.Attr["val"]); err == nil {
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
		for _, c := range n.Kids {
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

func renderDocx(root *xmlsafe.Node, styles map[string]int, ctx *docxCtx) string {
	var b strings.Builder
	var walk func(*xmlsafe.Node)
	walk = func(n *xmlsafe.Node) {
		switch n.Local {
		case "drawing", "pict", "object":
			collectTxbx(n, walk)
			return
		case "txbxContent":
			for _, c := range n.Kids {
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
			for _, c := range n.Kids {
				walk(c)
			}
		}
	}
	walk(root)
	return b.String()
}

func renderParagraph(p *xmlsafe.Node, styles map[string]int, ctx *docxCtx) string {
	text := inlineContent(p, ctx)
	if strings.TrimSpace(text) == "" {
		return ""
	}
	if lvl := styles[paragraphStyle(p)]; lvl > 0 {
		return strings.Repeat("#", lvl) + " " + strings.TrimSpace(text)
	}
	return strings.TrimSpace(text)
}

func paragraphStyle(p *xmlsafe.Node) string {
	for _, c := range p.Kids {
		if c.Local != "pPr" {
			continue
		}
		for _, s := range c.Kids {
			if s.Local == "pStyle" {
				return s.Attr["val"]
			}
		}
	}
	return ""
}

func paragraphText(p *xmlsafe.Node, ctx *docxCtx) string { return inlineContent(p, ctx) }

func inlineContent(n *xmlsafe.Node, ctx *docxCtx) string {
	var b strings.Builder
	var walk func(*xmlsafe.Node)
	walk = func(n *xmlsafe.Node) {
		switch n.Local {
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
			for _, c := range n.Kids {
				walk(c)
			}
		case "tbl":
			return
		case "t", "delText":
			b.WriteString(underlineText(n, n.Text, ctx))
		case "tab":
			b.WriteString(underlineText(n, " ", ctx))
		case "br", "cr":
			b.WriteString(underlineText(n, "\n", ctx))
		default:
			for _, c := range n.Kids {
				walk(c)
			}
		}
	}
	walk(n)
	return b.String()
}

func collectTxbx(n *xmlsafe.Node, walk func(*xmlsafe.Node)) {
	if n.Local == "txbxContent" {
		for _, c := range n.Kids {
			walk(c)
		}
		return
	}
	for _, c := range n.Kids {
		collectTxbx(c, walk)
	}
}

func renderDocxTable(tbl *xmlsafe.Node, ctx *docxCtx) string {
	var rows [][]string
	for _, tr := range xmlsafe.Direct(tbl, "tr") {
		var row []string
		for _, tc := range xmlsafe.Direct(tr, "tc") {
			var parts []string
			var walk func(*xmlsafe.Node)
			walk = func(n *xmlsafe.Node) {
				switch n.Local {
				case "tbl":
					return
				case "p":
					if t := strings.TrimSpace(paragraphText(n, ctx)); t != "" {
						parts = append(parts, t)
					}
				default:
					for _, c := range n.Kids {
						walk(c)
					}
				}
			}
			walk(tc)
			row = append(row, strings.Join(parts, "\n"))
		}
		rows = append(rows, row)
	}
	if ctx != nil && ctx.ocr != nil && len(rows) > 0 {
		rows = append([][]string{make([]string, len(rows[0]))}, rows...)
	}
	return table.Markdown(rows)
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

func embeddedStyleMap(zr *ooxml.Package) string {
	b, err := zr.Read("mammoth/style-map")
	if err != nil {
		return ""
	}
	return string(b)
}

func underlineText(n *xmlsafe.Node, text string, ctx *docxCtx) string {
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

func runHasUnderline(n *xmlsafe.Node) bool {
	for p := n; p != nil; p = p.Parent {
		if p.Local != "r" {
			continue
		}
		for _, c := range p.Kids {
			if c.Local != "rPr" {
				continue
			}
			for _, prop := range c.Kids {
				if prop.Local == "u" {
					return true
				}
			}
		}
		return false
	}
	return false
}

func docxDrawingImages(n *xmlsafe.Node, ctx *docxCtx) string {
	if ctx == nil || ctx.err != nil {
		return ""
	}
	var b strings.Builder
	for _, blip := range xmlsafe.FindAll(n, "blip") {
		rid := blip.Attr["embed"]
		if rid == "" {
			continue
		}
		part := docxRelTarget(ctx.zr, rid)
		if part == "" {
			continue
		}
		blob, err := ctx.zr.Read(part)
		if err != nil {
			continue
		}
		defaults, overrides := ooxml.ContentTypes(ctx.zr)
		info := ooxml.ImageInfo(part, blob, defaults, overrides)
		if ctx.ocr != nil {
			md, ok := ctx.ocr.markdown(OCRImage{Data: blob, Name: info.Name, MIME: info.MIME})
			if ctx.ocr.err != nil {
				ctx.err = ctx.ocr.err
				return b.String()
			}
			if ok {
				b.WriteString(md)
				continue
			}
		}
		alt := picAlt(n)
		if alt == "" {
			alt = info.Name
		}
		b.WriteString(imageMarkdown(alt, info, ctx.keep))
	}
	return b.String()
}

func docxRelTarget(zr *ooxml.Package, id string) string {
	for _, rel := range ooxml.Rels(zr, "word/_rels/document.xml.rels", "image") {
		if rel.ID == id {
			return rel.Target
		}
	}
	return ""
}

func picAlt(n *xmlsafe.Node) string {
	for _, pr := range xmlsafe.FindAll(n, "cNvPr") {
		if pr.Attr["descr"] != "" {
			return pr.Attr["descr"]
		}
		if pr.Attr["name"] != "" {
			return pr.Attr["name"]
		}
	}
	return ""
}

func imageMarkdown(alt string, img ooxml.Image, keep bool) string {
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
