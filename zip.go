package markitdown

import (
	"archive/zip"
	"errors"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
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
		if err != nil {
			partial = true
			if errors.Is(err, ErrTooLarge) {
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

func renderSlide(root *xnode) string {
	var b strings.Builder
	tree := findFirst(root, "spTree")
	if tree == nil {
		tree = root
	}
	writeShapes(&b, tree)
	return b.String()
}

func writeShapes(b *strings.Builder, n *xnode) {
	for _, c := range n.kids {
		switch c.local {
		case "sp", "cxnSp":
			writeShape(b, c)
		case "grpSp":
			writeShapes(b, c)
		case "graphicFrame":
			if tbl := findFirst(c, "tbl"); tbl != nil {
				if table := renderDrawTable(tbl); table != "" {
					b.WriteString(table)
					b.WriteByte('\n')
				}
			}
		}
	}
}

func writeShape(b *strings.Builder, sp *xnode) {
	text := strings.TrimSpace(localText(sp, "t"))
	switch phType(sp) {
	case "title", "ctrTitle":
		if text != "" {
			b.WriteString("# ")
			b.WriteString(text)
			b.WriteByte('\n')
		}
	default:
		if text != "" {
			b.WriteString(text)
			b.WriteByte('\n')
		}
	}
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
