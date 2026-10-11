package markitdown

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// OCRResult is text from one image. A non-empty Error keeps the native image.
type OCRResult struct {
	Text  string
	Error string
}

// OCRImage describes the image passed to an OCR service.
type OCRImage struct {
	Data []byte
	Name string
	MIME string
}

// OCRService reads text from an image. A returned error fails the conversion.
// OCRResult.Error keeps the native image instead.
type OCRService interface {
	ExtractText(img OCRImage) (OCRResult, error)
}

// VisionRequest is the chat completion a fake vision client receives.
type VisionRequest struct {
	Model   string
	Prompt  string
	DataURI string
}

// VisionClient is an OpenAI-compatible vision client. It is not called unless
// the caller supplies one.
type VisionClient interface {
	Complete(req VisionRequest) (string, error)
}

// LLMVisionOCR calls client with the image as a data URI.
type LLMVisionOCR struct {
	Client VisionClient
	Model  string
	Prompt string
}

func NewLLMVisionOCR(client VisionClient, model, prompt string) *LLMVisionOCR {
	if strings.TrimSpace(prompt) == "" {
		prompt = "Extract all text from this image. Return ONLY the extracted text, maintaining the original layout and order. Do not add any commentary or description."
	}
	return &LLMVisionOCR{Client: client, Model: model, Prompt: prompt}
}

func (s *LLMVisionOCR) ExtractText(img OCRImage) (OCRResult, error) {
	if s == nil || s.Client == nil {
		return OCRResult{Error: "LLM client not configured"}, nil
	}
	mime := img.MIME
	if mime == "" {
		mime = "image/png"
	}
	uri := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(img.Data)
	text, err := s.Client.Complete(VisionRequest{Model: s.Model, Prompt: s.Prompt, DataURI: uri})
	if err != nil {
		return OCRResult{Error: err.Error()}, nil
	}
	return OCRResult{Text: strings.TrimSpace(text)}, nil
}

type ocrSession struct {
	svc     OCRService
	cache   map[[32]byte]ocrHit
	err     error
	partial bool
}

type ocrHit struct {
	md string
	ok bool
}

func newOCRSession(svc OCRService) *ocrSession {
	if svc == nil {
		return nil
	}
	return &ocrSession{svc: svc, cache: map[[32]byte]ocrHit{}}
}

func (s *ocrSession) markdown(img OCRImage) (string, bool) {
	if s == nil || s.err != nil {
		return "", false
	}
	key := sha256.Sum256(img.Data)
	if hit, ok := s.cache[key]; ok {
		return hit.md, hit.ok
	}
	res, err := s.svc.ExtractText(img)
	if err != nil {
		// ExtractText's error skips this image. It does not fail the document.
		s.partial = true
		s.cache[key] = ocrHit{}
		return "", false
	}
	if res.Error != "" || strings.TrimSpace(res.Text) == "" {
		s.cache[key] = ocrHit{}
		return "", false
	}
	md := ocrMarkdown(res.Text)
	s.cache[key] = ocrHit{md: md, ok: true}
	return md, true
}

func ocrMarkdown(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range text {
		if r == '\\' || r == '_' || r == '*' {
			b.WriteByte('\\')
		}
		if r == '\n' {
			b.WriteString("  \n")
			continue
		}
		b.WriteRune(r)
	}
	return "*[Image OCR]  \n" + b.String() + "  \n[End OCR]*"
}
