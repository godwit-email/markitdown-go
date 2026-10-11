package markitdown

import (
	"bytes"
	"errors"
	"strings"

	"github.com/ledongthuc/pdf"
)

func convertPDF(data []byte) (Result, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		if errors.Is(err, pdf.ErrInvalidPassword) {
			return Result{}, ErrUnsupported
		}
		return Result{}, wrapCorrupt(err)
	}
	var b strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		text, err := plainText(p)
		if err != nil || strings.TrimSpace(text) == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(text)
	}
	text := strings.TrimSpace(b.String())
	if text == "" {
		return Result{Partial: true}, nil
	}
	return Result{Text: text}, nil
}

func plainText(p pdf.Page) (text string, err error) {
	defer func() {
		if recover() != nil {
			text = ""
			err = ErrCorrupt
		}
	}()
	content := p.Content()
	if len(content.Text) == 0 {
		return p.GetPlainText(nil)
	}
	var b strings.Builder
	var prev *pdf.Text
	for i := range content.Text {
		t := &content.Text[i]
		if t.S == "" {
			continue
		}
		if t.S == "\n" {
			b.WriteByte('\n')
			prev = nil
			continue
		}
		if prev != nil {
			dy := t.Y - prev.Y
			if dy > 2 || dy < -2 {
				b.WriteByte('\n')
			} else if gap := t.X - (prev.X + prev.W); gap > spaceGap(prev.FontSize) {
				b.WriteByte(' ')
			}
		}
		b.WriteString(t.S)
		prev = t
	}
	return b.String(), nil
}

func spaceGap(fontSize float64) float64 {
	if fontSize < 1 {
		fontSize = 10
	}
	return fontSize * 0.15
}
