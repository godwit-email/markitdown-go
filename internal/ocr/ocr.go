// Package ocr remembers one recognition result per image hash.
// It does not know about the root package's Recognizer.
package ocr

import (
	"crypto/sha256"
	"strings"
)

// Session caches recognition by sha256 of the image bytes.
type Session struct {
	cache map[[32]byte]string
	seen  map[[32]byte]bool
}

// New returns an empty session.
func New() *Session {
	return &Session{cache: map[[32]byte]string{}, seen: map[[32]byte]bool{}}
}

// Once calls recognize the first time data is seen. A later call with the
// same bytes returns the cached text and does not call recognize.
// ok is false when the cached or returned text is empty.
func (s *Session) Once(data []byte, recognize func([]byte) (string, error)) (text string, ok bool, err error) {
	if s == nil {
		return "", false, nil
	}
	key := sha256.Sum256(data)
	if s.seen[key] {
		text = s.cache[key]
		return text, text != "", nil
	}
	text, err = recognize(data)
	if err != nil {
		return "", false, err
	}
	text = strings.TrimSpace(text)
	s.seen[key] = true
	s.cache[key] = text
	return text, text != "", nil
}

// Block is the markdown wrapped around recognized text.
func Block(text string) string {
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
