package zip

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"path"
	"strings"

	"github.com/godwit-email/markitdown-go"
	"github.com/godwit-email/markitdown-go/internal/budget"
	"github.com/godwit-email/markitdown-go/internal/run"
)

const Kind = "zip"

func init() { markitdown.Register(converter{}) }

type converter struct{}

func (converter) Kind() string { return Kind }
func (converter) MediaTypes() []string {
	return []string{"application/zip", "application/x-zip-compressed"}
}
func (converter) Extensions() []string { return []string{".zip"} }

func (converter) Extract(ctx context.Context, in markitdown.Input) (markitdown.Result, error) {
	if err := ctx.Err(); err != nil {
		return markitdown.Result{}, err
	}
	res, err := convertZIP(ctx, in)
	if err != nil {
		return markitdown.Result{}, err
	}
	res.Kind = Kind
	return res, nil
}

func Extract(ctx context.Context, req markitdown.Request) (markitdown.Result, error) {
	in, err := prepare(ctx, req)
	if err != nil {
		return markitdown.Result{}, err
	}
	if mismatch(in, converter{}.MediaTypes(), converter{}.Extensions()) {
		return markitdown.Result{}, markitdown.ErrCorrupt
	}
	res, err := converter{}.Extract(ctx, in)
	if err != nil {
		return markitdown.Result{}, err
	}
	return finish(res, in.MaxOut), nil
}

func convertZIP(ctx context.Context, in markitdown.Input) (markitdown.Result, error) {
	st := budget.Get(ctx)
	depth := 1
	if st != nil {
		depth = st.Depth + 1
	}
	if depth > 3 {
		return markitdown.Result{Partial: true}, nil
	}
	zr, err := zip.NewReader(bytes.NewReader(in.Data), int64(len(in.Data)))
	if err != nil {
		return markitdown.Result{}, run.ErrCorrupt
	}
	label := strings.TrimSpace(in.Filename)
	if label == "" {
		label = "(unknown)"
	}
	var b strings.Builder
	b.WriteString("Content from the zip file `")
	b.WriteString(label)
	b.WriteString("`:\n\n")
	partial := false
	wrote := false
	files, left := 100, int64(markitdown.MaxInput)
	if st != nil && st.Files != nil {
		files = *st.Files
	}
	if st != nil && st.Left != nil {
		left = *st.Left
	}
	for _, f := range zr.File {
		if err := ctx.Err(); err != nil {
			return markitdown.Result{}, err
		}
		if f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/") {
			continue
		}
		if files <= 0 || left <= 0 {
			partial = true
			break
		}
		if f.UncompressedSize64 > uint64(left) {
			partial = true
			continue
		}
		display, base := safeMember(f.Name)
		if depth >= 3 && run.Ext(base) == ".zip" {
			partial = true
			continue
		}
		files--
		rc, err := f.Open()
		if err != nil {
			partial = true
			continue
		}
		buf, tooBig, readErr := readMember(rc, &left)
		rc.Close()
		if readErr != nil {
			partial = true
			// Declared size was smaller than the bytes actually produced.
			if errors.Is(readErr, zip.ErrFormat) {
				break
			}
			continue
		}
		if tooBig {
			partial = true
			left = 0
			break
		}
		childCtx := budget.With(ctx, &budget.State{Depth: depth, Files: &files, Left: &left})
		child, err := markitdown.Extract(childCtx, markitdown.Request{
			R: bytes.NewReader(buf), Filename: base, MaxOut: in.MaxOut, OCR: in.OCR,
		})
		if err != nil {
			partial = true
			if errors.Is(err, run.ErrTooLarge) && left <= 0 {
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
	if st != nil {
		if st.Files != nil {
			*st.Files = files
		}
		if st.Left != nil {
			*st.Left = left
		}
	}
	text := ""
	if wrote {
		text = strings.TrimSpace(b.String())
	}
	return markitdown.Result{Text: text, Partial: partial}, nil
}

func readMember(r io.Reader, left *int64) ([]byte, bool, error) {
	limit := *left + 1
	if limit < 1 {
		return nil, true, nil
	}
	buf, err := io.ReadAll(io.LimitReader(r, limit))
	if err != nil {
		return nil, false, err
	}
	if int64(len(buf)) > *left {
		return nil, true, nil
	}
	*left -= int64(len(buf))
	return buf, false, nil
}

func safeMember(name string) (display, base string) {
	display = name
	base = path.Base(strings.ReplaceAll(name, "\\", "/"))
	return display, base
}
