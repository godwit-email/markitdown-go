package html

import (
	"context"

	"github.com/godwit-email/markitdown-go"
	"github.com/godwit-email/markitdown-go/internal/htmlmd"
)

const Kind = "html"

func init() { markitdown.Register(converter{}) }

type converter struct{}

func (converter) Kind() string         { return Kind }
func (converter) MediaTypes() []string { return []string{"text/html", "application/xhtml+xml"} }
func (converter) Extensions() []string { return []string{".html", ".htm"} }
func (converter) Extract(ctx context.Context, in markitdown.Input) (markitdown.Result, error) {
	return extract(ctx, in, Options{})
}

// Options are HTML-only. The registry path uses the zero value.
type Options struct {
	KeepDataURIs bool
	BaseURL      string
}

type htmlOpt struct{}

func Extract(ctx context.Context, req markitdown.Request) (markitdown.Result, error) {
	return ExtractOptions(ctx, req, Options{})
}

func ExtractOptions(ctx context.Context, req markitdown.Request, opt Options) (markitdown.Result, error) {
	in, err := prepare(ctx, req)
	if err != nil {
		return markitdown.Result{}, err
	}
	if mismatch(in, converter{}.MediaTypes(), converter{}.Extensions()) {
		return markitdown.Result{}, markitdown.ErrCorrupt
	}
	res, err := extract(ctx, in, opt)
	if err != nil {
		return markitdown.Result{}, err
	}
	return finish(res, in.MaxOut), nil
}

func extract(ctx context.Context, in markitdown.Input, opt Options) (markitdown.Result, error) {
	if err := ctx.Err(); err != nil {
		return markitdown.Result{}, err
	}
	text, err := htmlmd.Convert(in.Data, htmlmd.Options{KeepData: opt.KeepDataURIs, Base: opt.BaseURL})
	if err != nil {
		return markitdown.Result{}, err
	}
	return markitdown.Result{Text: text, Kind: Kind}, nil
}
