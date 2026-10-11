package pptx

import (
	"context"

	"github.com/godwit-email/markitdown-go"
)

const Kind = "pptx"

func init() { markitdown.Register(converter{}) }

type converter struct{}

func (converter) Kind() string { return Kind }
func (converter) MediaTypes() []string {
	return []string{"application/vnd.openxmlformats-officedocument.presentationml.presentation"}
}
func (converter) Extensions() []string { return []string{".pptx"} }
func (converter) Extract(ctx context.Context, in markitdown.Input) (markitdown.Result, error) {
	if err := ctx.Err(); err != nil {
		return markitdown.Result{}, err
	}
	res, err := convertPPTX(ctx, in)
	if err != nil {
		return markitdown.Result{}, err
	}
	res.Kind = Kind
	return res, nil
}

// Options are PPTX-only. The registry path uses the zero value.
type Options struct {
	KeepData bool
}

type pptxOpt struct{}

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
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, pptxOpt{}, opt)
	res, err := convertPPTX(ctx, in)
	if err != nil {
		return markitdown.Result{}, err
	}
	res.Kind = Kind
	return finish(res, in.MaxOut), nil
}
