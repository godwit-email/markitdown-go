package pdf

import (
	"bytes"
	"errors"
	"io"
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

// shortcut: glyph positions become lines, not pdfplumber tables. A grid
// matcher is a separate piece of work; bordered and borderless tables stay
// short of the expected markdown until then.
func plainText(p pdfdoc.Page) (text string, err error) {
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
	var b strings.Builder
	var prev *pdfdoc.Text
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
			if dy < 0 {
				dy = -dy
			}
			if dy > 2 {
				b.WriteByte('\n')
				// A baseline jump bigger than one line is a blank line in the text layer.
				line := prev.FontSize
				if line < 8 {
					line = 12
				}
				if dy > line*1.6 {
					b.WriteByte('\n')
				}
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
