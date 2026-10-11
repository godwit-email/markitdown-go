package xls

import (
	"context"

	"github.com/godwit-email/markitdown-go"
)

const Kind = "xls"

func init() { markitdown.Register(converter{}) }

type converter struct{}

func (converter) Kind() string { return Kind }
func (converter) MediaTypes() []string {
	return []string{"application/vnd.ms-excel", "application/excel"}
}
func (converter) Extensions() []string { return []string{".xls"} }
func (converter) Extract(ctx context.Context, in markitdown.Input) (markitdown.Result, error) {
	if err := ctx.Err(); err != nil {
		return markitdown.Result{}, err
	}
	res, err := convertXLS(in.Data)
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
