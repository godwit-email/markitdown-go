package markitdown

import (
	"sort"
	"strconv"
	"strings"
)

func convertPPTX(data []byte, lim budget) (Result, error) {
	zr, err := openZip(data)
	if err != nil {
		return Result{}, err
	}
	names := slideNames(zr)
	if len(names) == 0 {
		if _, err := readZip(zr, "ppt/presentation.xml"); err != nil {
			return Result{}, ErrCorrupt
		}
		return Result{}, nil
	}
	var b strings.Builder
	ocr := newOCRSession(lim.ocr)
	for i, name := range names {
		body, err := readZip(zr, name)
		if err != nil {
			return Result{}, ErrCorrupt
		}
		root, err := parseXML(body)
		if err != nil {
			return Result{}, err
		}
		slide := strings.TrimSpace(renderSlide(root, slideEnv{zr: zr, name: name, keep: lim.keepData, num: i + 1, ocr: ocr}))
		if ocr != nil && ocr.err != nil {
			return Result{}, ocr.err
		}
		notes := notesFor(zr, name)
		if slide == "" && notes == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(slide)
		if notes != "" {
			if slide != "" {
				b.WriteString("\n\n")
			}
			b.WriteString("### Notes:\n")
			b.WriteString(notes)
		}
	}
	return Result{Text: strings.TrimSpace(b.String())}, nil
}

func renderChart(env slideEnv, chartEl *xnode) string {
	rid := chartEl.attr["id"]
	part := ""
	for _, rel := range zipRels(env.zr, relsPath(env.name), "chart") {
		if rel.id == rid {
			part = rel.target
			break
		}
	}
	if part == "" {
		return "\n\n[unsupported chart]\n"
	}
	body, err := readZip(env.zr, part)
	if err != nil {
		return "\n\n[unsupported chart]\n"
	}
	root, err := parseOfficeXML(body)
	if err != nil {
		return "\n\n[unsupported chart]\n"
	}
	return chartMarkdown(root)
}

type chartSeries struct {
	name string
	cats []string
	vals []string
}

func chartMarkdown(root *xnode) string {
	title := ""
	if t := findFirst(root, "title"); t != nil && findFirst(t, "rich") != nil {
		title = strings.TrimSpace(localText(t, "t"))
	}
	heading := "### Chart"
	if title != "" {
		heading += ": " + title
	}
	var series []chartSeries
	for _, ser := range findAll(root, "ser") {
		series = append(series, readSeries(ser))
	}
	if len(series) == 0 {
		return "\n\n" + heading + "\n"
	}
	n := 0
	for _, s := range series {
		if len(s.cats) > n {
			n = len(s.cats)
		}
		if len(s.vals) > n {
			n = len(s.vals)
		}
	}
	header := []string{"Category"}
	for _, s := range series {
		header = append(header, s.name)
	}
	var rows []string
	rows = append(rows, "| "+strings.Join(header, " | ")+" |")
	sep := "|" + strings.Repeat("---|", len(header))
	rows = append(rows, sep)
	for i := 0; i < n; i++ {
		cat := ""
		if len(series) > 0 && i < len(series[0].cats) {
			cat = series[0].cats[i]
		}
		line := []string{cat}
		for _, s := range series {
			if i < len(s.vals) {
				line = append(line, pyNum(s.vals[i]))
			} else {
				line = append(line, "")
			}
		}
		rows = append(rows, "| "+strings.Join(line, " | ")+" |")
	}
	return "\n\n" + heading + "\n\n" + strings.Join(rows, "\n")
}

func readSeries(ser *xnode) chartSeries {
	var s chartSeries
	for _, c := range ser.kids {
		switch c.local {
		case "tx":
			s.name = strings.TrimSpace(localText(c, "v"))
		case "cat":
			s.cats = pointValues(c)
		case "val":
			s.vals = pointValues(c)
		}
	}
	return s
}

func pointValues(n *xnode) []string {
	pts := findAll(n, "pt")
	sort.SliceStable(pts, func(i, j int) bool {
		return atoi(pts[i].attr["idx"]) < atoi(pts[j].attr["idx"])
	})
	out := make([]string, len(pts))
	for i, pt := range pts {
		out[i] = strings.TrimSpace(localText(pt, "v"))
	}
	return out
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func pyNum(s string) string {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return s
	}
	out := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.ContainsAny(out, ".eE") {
		out += ".0"
	}
	return out
}
