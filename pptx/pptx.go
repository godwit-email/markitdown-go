package pptx

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/godwit-email/markitdown-go"
	"github.com/godwit-email/markitdown-go/internal/ooxml"
	"github.com/godwit-email/markitdown-go/internal/run"
	"github.com/godwit-email/markitdown-go/internal/xmlsafe"
)

func convertPPTX(ctx context.Context, in markitdown.Input) (markitdown.Result, error) {
	opt, _ := ctx.Value(pptxOpt{}).(Options)
	zr, err := ooxml.Open(in.Data)
	if err != nil {
		return markitdown.Result{}, err
	}
	names := slideNames(zr)
	if len(names) == 0 {
		if _, err := zr.Read("ppt/presentation.xml"); err != nil {
			return markitdown.Result{}, run.ErrCorrupt
		}
		return markitdown.Result{}, nil
	}
	var b strings.Builder
	session := newOCRSession(ctx, in.OCR)
	for i, name := range names {
		body, err := zr.Read(name)
		if err != nil {
			return markitdown.Result{}, run.ErrCorrupt
		}
		root, err := xmlsafe.Parse(body)
		if err != nil {
			return markitdown.Result{}, err
		}
		slide := strings.TrimSpace(renderSlide(root, slideEnv{zr: zr, name: name, keep: opt.KeepData, num: i + 1, ocr: session}))
		if session != nil && session.err != nil {
			return markitdown.Result{}, session.err
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
	return markitdown.Result{Text: strings.TrimSpace(b.String())}, nil
}

func renderChart(env slideEnv, chartEl *xmlsafe.Node) string {
	rid := chartEl.Attr["id"]
	part := ""
	for _, rel := range ooxml.Rels(env.zr, ooxml.RelsPath(env.name), "chart") {
		if rel.ID == rid {
			part = rel.Target
			break
		}
	}
	if part == "" {
		return "\n\n[unsupported chart]\n"
	}
	body, err := env.zr.Read(part)
	if err != nil {
		return "\n\n[unsupported chart]\n"
	}
	root, err := xmlsafe.Parse(body)
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

func chartMarkdown(root *xmlsafe.Node) string {
	title := ""
	if t := xmlsafe.FindFirst(root, "title"); t != nil && xmlsafe.FindFirst(t, "rich") != nil {
		title = strings.TrimSpace(xmlsafe.LocalText(t, "t"))
	}
	heading := "### Chart"
	if title != "" {
		heading += ": " + title
	}
	var series []chartSeries
	for _, ser := range xmlsafe.FindAll(root, "ser") {
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

func readSeries(ser *xmlsafe.Node) chartSeries {
	var s chartSeries
	for _, c := range ser.Kids {
		switch c.Local {
		case "tx":
			s.name = strings.TrimSpace(xmlsafe.LocalText(c, "v"))
		case "cat":
			s.cats = pointValues(c)
		case "val":
			s.vals = pointValues(c)
		}
	}
	return s
}

func pointValues(n *xmlsafe.Node) []string {
	pts := xmlsafe.FindAll(n, "pt")
	sort.SliceStable(pts, func(i, j int) bool {
		return atoi(pts[i].Attr["idx"]) < atoi(pts[j].Attr["idx"])
	})
	out := make([]string, len(pts))
	for i, pt := range pts {
		out[i] = strings.TrimSpace(xmlsafe.LocalText(pt, "v"))
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
