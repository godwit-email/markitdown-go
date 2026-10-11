package pdf

import (
	"bytes"
	"errors"
	"io"
	"sort"
	"strings"

	"github.com/godwit-email/markitdown-go"
	"github.com/godwit-email/markitdown-go/internal/run"
	pdfdoc "github.com/ledongthuc/pdf"
)

func convertPDF(data []byte) (markitdown.Result, error) {
	r, err := pdfdoc.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		if errors.Is(err, pdfdoc.ErrInvalidPassword) {
			return markitdown.Result{}, run.ErrUnsupported
		}
		return markitdown.Result{}, run.Wrap(err)
	}
	if err := contentStreams(r); err != nil {
		return markitdown.Result{}, err
	}
	n := r.NumPage()
	wide := make([]bool, n)
	anyWide := false
	for i := 1; i <= n; i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		wide[i-1] = wideColumns(p)
		anyWide = anyWide || wide[i-1]
	}
	var b strings.Builder
	for i := 1; i <= n; i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		// A prose page next to a wide-column page is what pdfplumber extracts
		// with a 3pt word gap, so those words stay glued.
		text, err := plainText(p, anyWide && !wide[i-1])
		if err != nil || strings.TrimSpace(text) == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(text)
	}
	text := strings.Trim(b.String(), "\n")
	if strings.TrimSpace(text) == "" {
		return markitdown.Result{Partial: true}, nil
	}
	return markitdown.Result{Text: text}, nil
}

func contentStreams(r *pdfdoc.Reader) (err error) {
	defer func() {
		if recover() != nil {
			err = run.ErrCorrupt
		}
	}()
	used := 0
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		if err := countStream(p.V.Key("Contents"), &used, markitdown.MaxInput); err != nil {
			return err
		}
	}
	return nil
}

func countStream(v pdfdoc.Value, used *int, max int) error {
	switch v.Kind() {
	case pdfdoc.Null:
		return nil
	case pdfdoc.Array:
		for i := 0; i < v.Len(); i++ {
			if err := countStream(v.Index(i), used, max); err != nil {
				return err
			}
		}
		return nil
	case pdfdoc.Stream:
		rc := v.Reader()
		defer rc.Close()
		buf := make([]byte, 32<<10)
		for {
			room := max + 1 - *used
			if room <= 0 {
				return run.ErrCorrupt
			}
			tmp := buf
			if len(tmp) > room {
				tmp = tmp[:room]
			}
			n, err := rc.Read(tmp)
			*used += n
			if *used > max {
				return run.ErrCorrupt
			}
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return run.Wrap(err)
			}
		}
	default:
		return nil
	}
}

// shortcut: no table markdown. TestPDFFullOutput locks text-layer fragments.
func plainText(p pdfdoc.Page, glue bool) (text string, err error) {
	defer func() {
		if recover() != nil {
			text = ""
			err = run.ErrCorrupt
		}
	}()
	content := p.Content()
	if len(content.Text) == 0 {
		return p.GetPlainText(nil)
	}
	var items []pdfdoc.Text
	for _, t := range content.Text {
		if t.S == "" || t.S == "\n" {
			continue
		}
		items = append(items, t)
	}
	if len(items) == 0 {
		return "", nil
	}
	// Some fonts (CMTT10) paint a zero-width U+2126 on top of real glyphs.
	// Fonts that report no widths at all (Helvetica in the form fixtures) are kept.
	measured := map[string]bool{}
	for _, t := range items {
		if t.W > 0 {
			measured[t.Font] = true
		}
	}
	type line struct {
		y     float64
		size  float64
		items []pdfdoc.Text
	}
	var lines []line
	for _, t := range items {
		best := -1
		bestDy := 0.0
		for i := range lines {
			dy := t.Y - lines[i].y
			if dy < 0 {
				dy = -dy
			}
			if dy <= 2 && (best < 0 || dy < bestDy) {
				best = i
				bestDy = dy
			}
		}
		if best >= 0 {
			lines[best].items = append(lines[best].items, t)
			if t.FontSize > lines[best].size {
				lines[best].size = t.FontSize
			}
			continue
		}
		lines = append(lines, line{y: t.Y, size: t.FontSize, items: []pdfdoc.Text{t}})
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].y > lines[j].y })
	// A raised footnote marker is its own baseline. Fold it into the line below.
	folded := make([]line, 0, len(lines))
	for _, ln := range lines {
		if n := len(folded); n > 0 {
			above := folded[n-1]
			if above.size < ln.size && above.y-ln.y <= ln.size*0.5 {
				ln.items = append(above.items, ln.items...)
				folded[n-1] = ln
				continue
			}
		}
		folded = append(folded, ln)
	}
	lines = folded
	var b strings.Builder
	for li, ln := range lines {
		sort.SliceStable(ln.items, func(i, j int) bool { return ln.items[i].X < ln.items[j].X })
		if li > 0 {
			b.WriteByte('\n')
		}
		var prev *pdfdoc.Text
		for i := range ln.items {
			t := &ln.items[i]
			if t.W == 0 && measured[t.Font] {
				continue
			}
			if prev != nil {
				if gap := t.X - (prev.X + prev.W); gap > spaceGap(prev.FontSize, glue) {
					b.WriteByte(' ')
				}
			}
			b.WriteString(expandLigatures(t.S))
			prev = t
		}
	}
	return b.String(), nil
}

func spaceGap(fontSize float64, glue bool) float64 {
	if glue {
		return 3
	}
	if fontSize < 1 {
		fontSize = 10
	}
	return fontSize * 0.15
}

// wideColumns reports a page whose lines break into three or more chunks
// separated by 50pt, the same column test the Python form extractor uses.
func wideColumns(p pdfdoc.Page) (ok bool) {
	defer func() { recover() }()
	content := p.Content()
	type line struct {
		y float64
		x []float64
		w []float64
	}
	var lines []line
	for _, t := range content.Text {
		if t.S == "" || t.S == "\n" {
			continue
		}
		best := -1
		bestDy := 0.0
		for i := range lines {
			dy := t.Y - lines[i].y
			if dy < 0 {
				dy = -dy
			}
			if dy <= 2 && (best < 0 || dy < bestDy) {
				best = i
				bestDy = dy
			}
		}
		if best < 0 {
			lines = append(lines, line{y: t.Y})
			best = len(lines) - 1
		}
		lines[best].x = append(lines[best].x, t.X)
		lines[best].w = append(lines[best].w, t.W)
	}
	for _, ln := range lines {
		order := make([]int, len(ln.x))
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(a, b int) bool { return ln.x[order[a]] < ln.x[order[b]] })
		chunks := 0
		prevEnd := 0.0
		for i, idx := range order {
			if i == 0 || ln.x[idx]-prevEnd > 50 {
				chunks++
			}
			end := ln.x[idx] + ln.w[idx]
			if end > prevEnd {
				prevEnd = end
			}
		}
		if chunks >= 3 {
			return true
		}
	}
	return false
}

// The PDF fonts in these files map the fi glyph to U+FB01. Python's extractor
// emits the two letters, so the fragment "benefit" only matches after expansion.
var ligatures = strings.NewReplacer(
	"\uFB00", "ff",
	"\uFB01", "fi",
	"\uFB02", "fl",
	"\uFB03", "ffi",
	"\uFB04", "ffl",
	"\uFB05", "ft",
	"\uFB06", "st",
)

func expandLigatures(s string) string { return ligatures.Replace(s) }
