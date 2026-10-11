// Package run is the read, MIME, and truncation primitives shared by the
// shell and the format packages. It does not import the root package.
package run

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"strings"
	"unicode/utf8"
)

const (
	ErrUnsupportedText = "markitdown: unsupported format"
	ErrCorruptText     = "markitdown: corrupt input"
	ErrTooLargeText    = "markitdown: input too large"
)

var (
	ErrUnsupported = errors.New(ErrUnsupportedText)
	ErrCorrupt     = errors.New(ErrCorruptText)
	ErrTooLarge    = errors.New(ErrTooLargeText)
)

// Read reads r until max bytes. One more byte is ErrTooLarge, and that extra
// byte is not kept.
func Read(r io.Reader, max int) ([]byte, error) {
	if r == nil {
		return []byte{}, nil
	}
	var buf bytes.Buffer
	tmp := make([]byte, 32<<10)
	for {
		room := max + 1 - buf.Len()
		if room <= 0 {
			return nil, ErrTooLarge
		}
		want := tmp
		if len(want) > room {
			want = want[:room]
		}
		n, err := r.Read(want)
		if n > 0 {
			buf.Write(want[:n])
			if buf.Len() > max {
				return nil, ErrTooLarge
			}
		}
		if err == io.EOF {
			return buf.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
	}
}

// Media splits a Content-Type. A type that does not parse yields an empty
// type. A type that parses with a broken parameter list keeps the type and
// returns no parameters.
func Media(v string) (string, map[string]string) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", map[string]string{}
	}
	mt, params, err := mime.ParseMediaType(v)
	if err != nil {
		// mime.ParseMediaType fails the whole value when a parameter is bad.
		// Keep a leading type/subtype if it looks like one.
		head, _, _ := strings.Cut(v, ";")
		head = strings.TrimSpace(head)
		if strings.Contains(head, "/") && !strings.ContainsAny(head, " \t") {
			return strings.ToLower(head), map[string]string{}
		}
		return "", map[string]string{}
	}
	if params == nil {
		params = map[string]string{}
	}
	return strings.ToLower(mt), params
}

// Cut truncates s to at most max bytes on a UTF-8 boundary.
func Cut(s string, max int) (string, bool) {
	if max < 0 || len(s) <= max {
		return s, false
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
}

// Wrap returns ErrCorrupt, keeping err in the chain.
func Wrap(err error) error {
	if err == nil || errors.Is(err, ErrCorrupt) {
		return ErrCorrupt
	}
	return fmt.Errorf("%w: %v", ErrCorrupt, err)
}
