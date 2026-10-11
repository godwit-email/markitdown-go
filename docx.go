package markitdown

import (
	"strconv"
	"strings"
)

func convertDOCX(data []byte) (Result, error) {
	zr, err := openZip(data)
	if err != nil {
		return Result{}, err
	}
	styles := map[string]int{}
	if b, err := readZip(zr, "word/styles.xml"); err == nil {
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
	text := renderDocx(root, styles)
	if b, err := readZip(zr, "word/comments.xml"); err == nil {
		if croot, err := parseXML(b); err == nil {
			if comments := strings.TrimSpace(renderDocx(croot, styles)); comments != "" {
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

func renderDocx(root *xnode, styles map[string]int) string {
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
			if table := renderDocxTable(n); table != "" {
				b.WriteString(table)
				b.WriteString("\n\n")
			}
		case "p":
			if line := renderParagraph(n, styles); line != "" {
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

func renderParagraph(p *xnode, styles map[string]int) string {
	text := inlineContent(p)
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

func paragraphText(p *xnode) string { return inlineContent(p) }

func inlineContent(n *xnode) string {
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
			collectTxbx(n, walk)
		case "txbxContent":
			for _, c := range n.kids {
				walk(c)
			}
		case "tbl":
			return
		case "t", "delText":
			b.WriteString(n.text)
		case "tab":
			b.WriteByte('\t')
		case "br", "cr":
			b.WriteByte('\n')
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

func renderDocxTable(tbl *xnode) string {
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
					if t := strings.TrimSpace(paragraphText(n)); t != "" {
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
