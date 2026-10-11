package pptx

import (
	"encoding/base64"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/godwit-email/markitdown-go/internal/ooxml"
	"github.com/godwit-email/markitdown-go/internal/table"
	"github.com/godwit-email/markitdown-go/internal/xmlsafe"
)

func slideNames(zr *ooxml.Package) []string {
	var names []string
	for _, f := range zr.Zip().File {
		base := path.Base(f.Name)
		if strings.HasPrefix(f.Name, "ppt/slides/slide") && strings.HasSuffix(base, ".xml") && !strings.Contains(f.Name, "/_rels/") {
			names = append(names, f.Name)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		return slideNum(names[i]) < slideNum(names[j])
	})
	return names
}

func slideNum(name string) int {
	base := strings.TrimSuffix(path.Base(name), ".xml")
	n, _ := strconv.Atoi(strings.TrimPrefix(base, "slide"))
	return n
}

func notesFor(zr *ooxml.Package, slide string) string {
	rels := path.Dir(slide) + "/_rels/" + path.Base(slide) + ".rels"
	b, err := zr.Read(rels)
	if err != nil {
		return ""
	}
	root, err := xmlsafe.Parse(b)
	if err != nil {
		return ""
	}
	target := ""
	var walk func(*xmlsafe.Node)
	walk = func(n *xmlsafe.Node) {
		if n.Local == "Relationship" && strings.HasSuffix(n.Attr["Type"], "/notesSlide") {
			target = n.Attr["Target"]
		}
		for _, c := range n.Kids {
			walk(c)
		}
	}
	walk(root)
	if target == "" {
		return ""
	}
	if strings.HasPrefix(target, "/") {
		target = strings.TrimPrefix(target, "/")
	} else {
		target = path.Clean(path.Join(path.Dir(slide), target))
	}
	nb, err := zr.Read(target)
	if err != nil {
		return ""
	}
	nroot, err := xmlsafe.Parse(nb)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(notesBody(nroot))
}

func notesBody(root *xmlsafe.Node) string {
	var b strings.Builder
	var walk func(*xmlsafe.Node)
	walk = func(n *xmlsafe.Node) {
		if n.Local == "sp" {
			if phType(n) == "body" {
				b.WriteString(xmlsafe.LocalText(n, "t"))
			}
			return
		}
		for _, c := range n.Kids {
			walk(c)
		}
	}
	walk(root)
	return b.String()
}

type slideEnv struct {
	zr   *ooxml.Package
	name string
	keep bool
	num  int
	ocr  *ocrSession
}

func renderSlide(root *xmlsafe.Node, env slideEnv) string {
	var b strings.Builder
	if env.num > 0 {
		b.WriteString("<!-- Slide number: ")
		b.WriteString(strconv.Itoa(env.num))
		b.WriteString(" -->\n")
	}
	tree := xmlsafe.FindFirst(root, "spTree")
	if tree == nil {
		tree = root
	}
	writeShapes(&b, tree, env)
	return b.String()
}

func writeShapes(b *strings.Builder, n *xmlsafe.Node, env slideEnv) {
	for _, c := range n.Kids {
		switch c.Local {
		case "sp", "cxnSp":
			writeShape(b, c)
		case "pic":
			writePic(b, c, env)
		case "grpSp":
			writeShapes(b, c, env)
		case "graphicFrame":
			if tbl := xmlsafe.FindFirst(c, "tbl"); tbl != nil {
				if table := renderDrawTable(tbl); table != "" {
					b.WriteString(table)
					b.WriteByte('\n')
				}
				continue
			}
			if chart := xmlsafe.FindFirst(c, "chart"); chart != nil {
				if md := renderChart(env, chart); md != "" {
					b.WriteString(md)
					b.WriteByte('\n')
				}
			}
		}
	}
}

func appendBlock(b *strings.Builder, block string) {
	s := strings.TrimRight(b.String(), "\n")
	tail := s
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		tail = s[i+1:]
	}
	b.Reset()
	b.WriteString(s)
	switch {
	case strings.HasPrefix(tail, "#"):
		b.WriteString("\n\n\n")
	case s != "":
		b.WriteString("\n\n")
	}
	b.WriteString(block)
}

func writePic(b *strings.Builder, pic *xmlsafe.Node, env slideEnv) {
	alt := cleanPictureAlt(picAlt(pic))
	rid := picEmbed(pic)
	if env.ocr != nil && rid != "" && env.zr != nil {
		for _, rel := range ooxml.Rels(env.zr, ooxml.RelsPath(env.name), "image") {
			if rel.ID != rid {
				continue
			}
			blob, err := env.zr.Read(rel.Target)
			if err != nil {
				break
			}
			defaults, overrides := ooxml.ContentTypes(env.zr)
			info := ooxml.ImageInfo(rel.Target, blob, defaults, overrides)
			md, ok := env.ocr.markdown(OCRImage{Data: blob, Name: info.Name, MIME: info.MIME})
			if ok {
				appendBlock(b, md)
				return
			}
			break
		}
	}
	if env.keep && rid != "" && env.zr != nil {
		for _, rel := range ooxml.Rels(env.zr, ooxml.RelsPath(env.name), "image") {
			if rel.ID != rid {
				continue
			}
			blob, err := env.zr.Read(rel.Target)
			if err != nil {
				break
			}
			defaults, overrides := ooxml.ContentTypes(env.zr)
			info := ooxml.ImageInfo(rel.Target, blob, defaults, overrides)
			if info.MIME == "" && strings.EqualFold(info.Ext, ".svg") {
				info.MIME = "image/svg+xml"
			}
			b.WriteString(imageMarkdown(alt, info, true))
			b.WriteByte('\n')
			return
		}
	}
	// Native caption is the alt text plus a sanitized shape name, matching
	// PptxConverter when keep_data_uris is off. The bytes stay out of the URI.
	b.WriteString(pictureMarkdown(pic))
	b.WriteByte('\n')
}

func pictureMarkdown(pic *xmlsafe.Node) string {
	name, descr := "", ""
	for _, pr := range xmlsafe.FindAll(pic, "cNvPr") {
		if name == "" {
			name = pr.Attr["name"]
		}
		if descr == "" {
			descr = pr.Attr["descr"]
		}
	}
	alt := descr
	if strings.TrimSpace(alt) == "" {
		alt = name
	}
	var file strings.Builder
	for _, r := range name {
		if r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			file.WriteRune(r)
		}
	}
	file.WriteString(".jpg")
	return "![" + cleanPictureAlt(alt) + "](" + file.String() + ")"
}

func cleanPictureAlt(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if r == '\r' || r == '\n' || r == '[' || r == ']' || unicode.IsSpace(r) {
			if b.Len() > 0 {
				space = true
			}
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	return b.String()
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
	mimeType := img.MIME
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	uri := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(img.Data)
	if !keep {
		if i := strings.Index(uri, ","); i >= 0 {
			uri = uri[:i] + "..."
		}
	}
	return "![" + alt + "](" + uri + ")"
}

func picEmbed(pic *xmlsafe.Node) string {
	for _, svg := range xmlsafe.FindAll(pic, "svgBlip") {
		if svg.Attr["embed"] != "" {
			return svg.Attr["embed"]
		}
	}
	for _, blip := range xmlsafe.FindAll(pic, "blip") {
		if blip.Attr["embed"] != "" {
			return blip.Attr["embed"]
		}
	}
	return ""
}

func writeShape(b *strings.Builder, sp *xmlsafe.Node) {
	text := strings.TrimSpace(xmlsafe.LocalText(sp, "t"))
	switch phType(sp) {
	case "title", "ctrTitle":
		if text != "" {
			writeSlideLine(b, "# "+text)
		}
	default:
		if text != "" {
			writeSlideLine(b, text)
		}
	}
}

func writeSlideLine(b *strings.Builder, line string) {
	if b.Len() > 0 && !strings.HasSuffix(b.String(), "-->\n") {
		if !strings.HasSuffix(b.String(), "\n\n") {
			b.WriteByte('\n')
		}
	}
	b.WriteString(line)
	b.WriteByte('\n')
}

func phType(sp *xmlsafe.Node) string {
	ph := xmlsafe.FindFirst(sp, "ph")
	if ph == nil {
		return ""
	}
	return ph.Attr["type"]
}

func renderDrawTable(tbl *xmlsafe.Node) string {
	var rows [][]string
	for _, tr := range xmlsafe.Direct(tbl, "tr") {
		var row []string
		for _, tc := range xmlsafe.Direct(tr, "tc") {
			row = append(row, strings.TrimSpace(xmlsafe.LocalText(tc, "t")))
		}
		rows = append(rows, row)
	}
	return table.Markdown(rows)
}
