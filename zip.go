package markitdown

import (
	"archive/zip"
	"errors"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

func convertZIP(data []byte, filename string, lim budget) (Result, error) {
	if lim.depth <= 0 {
		return Result{Partial: true}, nil
	}
	zr, err := openZip(data)
	if err != nil {
		return Result{}, err
	}
	label := strings.TrimSpace(filename)
	if label == "" {
		label = "(unknown)"
	}
	var b strings.Builder
	b.WriteString("Content from the zip file `")
	b.WriteString(label)
	b.WriteString("`:\n\n")
	partial := false
	wrote := false
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/") {
			continue
		}
		if lim.files == nil || *lim.files <= 0 {
			partial = true
			break
		}
		if lim.uncompressed != nil && f.UncompressedSize64 > uint64(*lim.uncompressed) {
			partial = true
			break
		}
		*lim.files--
		rc, err := f.Open()
		if err != nil {
			partial = true
			continue
		}
		buf, tooBig, readErr := readMember(rc, lim.uncompressed)
		rc.Close()
		if readErr != nil {
			partial = true
			continue
		}
		if tooBig {
			partial = true
			break
		}
		display, base := safeMember(f.Name)
		child, err := convertBytes(buf, "", base, budget{
			depth:        lim.depth - 1,
			files:        lim.files,
			uncompressed: lim.uncompressed,
			keepData:     lim.keepData,
		})
		if err != nil || child.Kind == KindImage {
			partial = true
			if err != nil && errors.Is(err, ErrTooLarge) {
				break
			}
			continue
		}
		if child.Partial {
			partial = true
		}
		if wrote {
			b.WriteString("\n\n")
		}
		wrote = true
		b.WriteString("## File: ")
		b.WriteString(display)
		b.WriteString("\n\n")
		b.WriteString(child.Text)
	}
	return Result{Text: strings.TrimSpace(b.String()), Partial: partial}, nil
}

func readMember(r io.Reader, left *int64) ([]byte, bool, error) {
	limit := int64(maxZipUncompressed) + 1
	if left != nil {
		limit = *left + 1
	}
	buf, err := io.ReadAll(io.LimitReader(r, limit))
	if err != nil {
		return nil, false, err
	}
	if left != nil && int64(len(buf)) > *left {
		return nil, true, nil
	}
	if left != nil {
		*left -= int64(len(buf))
	}
	return buf, false, nil
}

func safeMember(name string) (display, base string) {
	name = strings.ReplaceAll(name, "\\", "/")
	var parts []string
	for _, p := range strings.Split(name, "/") {
		if p == "" || p == "." || p == ".." {
			continue
		}
		parts = append(parts, p)
	}
	display = strings.Join(parts, "/")
	base = path.Base(display)
	if base == "." || base == "/" || base == "" {
		base = "file"
	}
	if display == "" {
		display = base
	}
	return display, base
}

func slideNames(zr *zip.Reader) []string {
	var names []string
	for _, f := range zr.File {
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

func notesFor(zr *zip.Reader, slide string) string {
	rels := path.Dir(slide) + "/_rels/" + path.Base(slide) + ".rels"
	b, err := readZip(zr, rels)
	if err != nil {
		return ""
	}
	root, err := parseXML(b)
	if err != nil {
		return ""
	}
	target := ""
	var walk func(*xnode)
	walk = func(n *xnode) {
		if n.local == "Relationship" && strings.HasSuffix(n.attr["Type"], "/notesSlide") {
			target = n.attr["Target"]
		}
		for _, c := range n.kids {
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
	nb, err := readZip(zr, target)
	if err != nil {
		return ""
	}
	nroot, err := parseXML(nb)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(notesBody(nroot))
}

func notesBody(root *xnode) string {
	var b strings.Builder
	var walk func(*xnode)
	walk = func(n *xnode) {
		if n.local == "sp" {
			if phType(n) == "body" {
				b.WriteString(localText(n, "t"))
			}
			return
		}
		for _, c := range n.kids {
			walk(c)
		}
	}
	walk(root)
	return b.String()
}

type slideEnv struct {
	zr   *zip.Reader
	name string
	keep bool
	num  int
	ocr  *ocrSession
}

func renderSlide(root *xnode, env slideEnv) string {
	var b strings.Builder
	if env.num > 0 {
		b.WriteString("<!-- Slide number: ")
		b.WriteString(strconv.Itoa(env.num))
		b.WriteString(" -->\n")
	}
	tree := findFirst(root, "spTree")
	if tree == nil {
		tree = root
	}
	writeShapes(&b, tree, env)
	return b.String()
}

func writeShapes(b *strings.Builder, n *xnode, env slideEnv) {
	for _, c := range n.kids {
		switch c.local {
		case "sp", "cxnSp":
			writeShape(b, c)
		case "pic":
			writePic(b, c, env)
		case "grpSp":
			writeShapes(b, c, env)
		case "graphicFrame":
			if tbl := findFirst(c, "tbl"); tbl != nil {
				if table := renderDrawTable(tbl); table != "" {
					b.WriteString(table)
					b.WriteByte('\n')
				}
				continue
			}
			if chart := findFirst(c, "chart"); chart != nil {
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

func writePic(b *strings.Builder, pic *xnode, env slideEnv) {
	alt := cleanPictureAlt(picAlt(pic))
	rid := picEmbed(pic)
	if env.ocr != nil && rid != "" && env.zr != nil {
		for _, rel := range zipRels(env.zr, relsPath(env.name), "image") {
			if rel.id != rid {
				continue
			}
			blob, err := readZip(env.zr, rel.target)
			if err != nil {
				break
			}
			defaults, overrides := contentTypes(env.zr)
			info := imageInfo(rel.target, blob, defaults, overrides)
			md, ok := env.ocr.markdown(OCRImage{Data: blob, Name: info.Name, MIME: info.MIME})
			if ok {
				appendBlock(b, md)
				return
			}
			break
		}
	}
	if env.keep && rid != "" && env.zr != nil {
		for _, rel := range zipRels(env.zr, relsPath(env.name), "image") {
			if rel.id != rid {
				continue
			}
			blob, err := readZip(env.zr, rel.target)
			if err != nil {
				break
			}
			defaults, overrides := contentTypes(env.zr)
			info := imageInfo(rel.target, blob, defaults, overrides)
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

func pictureMarkdown(pic *xnode) string {
	name, descr := "", ""
	for _, pr := range findAll(pic, "cNvPr") {
		if name == "" {
			name = pr.attr["name"]
		}
		if descr == "" {
			descr = pr.attr["descr"]
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

func picEmbed(pic *xnode) string {
	for _, svg := range findAll(pic, "svgBlip") {
		if svg.attr["embed"] != "" {
			return svg.attr["embed"]
		}
	}
	for _, blip := range findAll(pic, "blip") {
		if blip.attr["embed"] != "" {
			return blip.attr["embed"]
		}
	}
	return ""
}

func writeShape(b *strings.Builder, sp *xnode) {
	text := strings.TrimSpace(localText(sp, "t"))
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

func phType(sp *xnode) string {
	ph := findFirst(sp, "ph")
	if ph == nil {
		return ""
	}
	return ph.attr["type"]
}

func renderDrawTable(tbl *xnode) string {
	var rows [][]string
	for _, tr := range direct(tbl, "tr") {
		var row []string
		for _, tc := range direct(tr, "tc") {
			row = append(row, strings.TrimSpace(localText(tc, "t")))
		}
		rows = append(rows, row)
	}
	return markdownTable(rows)
}
