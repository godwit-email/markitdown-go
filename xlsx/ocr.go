package xlsx

import (
	"context"
	"errors"

	"github.com/godwit-email/markitdown-go"
	"github.com/godwit-email/markitdown-go/internal/ocr"
)

// OCRImage is the image passed into a recognition session.
type OCRImage struct {
	Data []byte
	Name string
	MIME string
}

type ocrSession struct {
	rec  markitdown.Recognizer
	sess *ocr.Session
	ctx  context.Context
	err  error
}

func newOCRSession(ctx context.Context, rec markitdown.Recognizer) *ocrSession {
	if rec == nil {
		return nil
	}
	return &ocrSession{rec: rec, sess: ocr.New(), ctx: ctx}
}

func (s *ocrSession) markdown(img OCRImage) (string, bool) {
	if s == nil || s.err != nil {
		return "", false
	}
	text, ok, err := s.sess.Once(img.Data, func(b []byte) (string, error) {
		if err := s.ctx.Err(); err != nil {
			return "", err
		}
		out, err := s.rec.Recognize(s.ctx, markitdown.Image{Data: b, MIME: img.MIME, Name: img.Name})
		if err != nil {
			return "", err
		}
		if err := s.ctx.Err(); err != nil {
			return "", err
		}
		return out, nil
	})
	if err != nil {
		if s.ctx.Err() != nil && errors.Is(err, s.ctx.Err()) {
			s.err = s.ctx.Err()
			return "", false
		}
		return "", false
	}
	if !ok {
		return "", false
	}
	return ocr.Block(text), true
}
