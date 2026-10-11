package docx

import (
	"context"

	"github.com/godwit-email/markitdown-go"
	"github.com/godwit-email/markitdown-go/internal/run"
)

func prepare(ctx context.Context, req markitdown.Request) (markitdown.Input, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return markitdown.Input{}, err
	}
	data, err := run.Read(req.R, markitdown.MaxInput)
	if err != nil {
		return markitdown.Input{}, err
	}
	mt, params := run.Media(req.MIME)
	maxOut := req.MaxOut
	if maxOut <= 0 {
		maxOut = markitdown.DefaultMaxOut
	}
	return markitdown.Input{Data: data, MediaType: mt, Params: params, Filename: req.Filename, MaxOut: maxOut, OCR: req.OCR}, nil
}

func mismatch(in markitdown.Input, mimes, exts []string) bool {
	if in.MediaType != "" && in.MediaType != "application/octet-stream" {
		return !has(mimes, in.MediaType)
	}
	ext := run.Ext(in.Filename)
	if ext != "" {
		return !has(exts, ext)
	}
	return false
}

func has(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func finish(res markitdown.Result, maxOut int) markitdown.Result {
	text, cut := run.Cut(res.Text, maxOut)
	res.Text = text
	if cut {
		res.Partial = true
	}
	return res
}
