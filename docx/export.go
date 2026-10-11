package docx

import (
	"context"

	"github.com/godwit-email/markitdown-go"
	"github.com/godwit-email/markitdown-go/internal/run"
)

// Kind is the result kind for Word documents.
const Kind = "docx"

func init() { markitdown.Register(converter{}) }

type converter struct{}

func (converter) Kind() string { return Kind }
func (converter) MediaTypes() []string {
	return []string{"application/vnd.openxmlformats-officedocument.wordprocessingml.document"}
}
func (converter) Extensions() []string { return []string{".docx"} }
func (converter) Extract(ctx context.Context, in markitdown.Input) (markitdown.Result, error) {
	res, err := convertDOCX(ctx, in)
	if err != nil {
		return markitdown.Result{}, err
	}
	res.Kind = Kind
	return res, nil
}

// Options are DOCX-only. The registry path uses the zero value.
type Options struct {
	StyleMap string
	KeepData bool
}

type docxOpt struct{}

// Extract reads a DOCX. Input that is not a DOCX is ErrCorrupt.
func Extract(ctx context.Context, req markitdown.Request) (markitdown.Result, error) {
	return ExtractOptions(ctx, req, Options{})
}

// ExtractOptions is Extract with a style map or kept image data URIs.
func ExtractOptions(ctx context.Context, req markitdown.Request, opt Options) (markitdown.Result, error) {
	in, err := prepare(ctx, req)
	if err != nil {
		return markitdown.Result{}, err
	}
	if mismatch(in, converter{}.MediaTypes(), converter{}.Extensions()) {
		return markitdown.Result{}, markitdown.ErrCorrupt
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, docxOpt{}, opt)
	res, err := convertDOCX(ctx, in)
	if err != nil {
		return markitdown.Result{}, err
	}
	res.Kind = Kind
	text, cut := run.Cut(res.Text, in.MaxOut)
	res.Text = text
	if cut {
		res.Partial = true
	}
	return res, nil
}
