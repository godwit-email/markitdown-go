// Package markitdown extracts text from attachment bytes already in hand.
// The root package registers no formats. Import the format packages you need.
package markitdown

import (
	"context"
	"errors"
	"io"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/godwit-email/markitdown-go/internal/budget"
	"github.com/godwit-email/markitdown-go/internal/run"
)

const (
	// MaxInput is the maximum number of bytes read from Request.R.
	// Compressed packages are measured compressed. Past this, Extract
	// returns ErrTooLarge.
	MaxInput = 25 << 20
	// DefaultMaxOut is the output cap when Request.MaxOut is 0.
	DefaultMaxOut = 2 << 20
)

var (
	// ErrUnsupported means this binary does not extract the input.
	ErrUnsupported = run.ErrUnsupported
	// ErrCorrupt means the format was selected but the container is unreadable,
	// or a library error was wrapped as this sentinel.
	ErrCorrupt = run.ErrCorrupt
	// ErrTooLarge means the bytes read from Request.R exceeded MaxInput.
	ErrTooLarge = run.ErrTooLarge
)

// Request is one extraction. R is read up to MaxInput.
type Request struct {
	R        io.Reader
	MIME     string
	Filename string
	MaxOut   int
	OCR      Recognizer
}

// Image is one embedded image passed to Recognizer.
type Image struct {
	Data []byte
	MIME string
	Name string
}

// Recognizer reads text from one image. After it returns, the caller checks
// the context that was passed in. A non-nil ctx.Err() fails Extract.
// Any other error skips that image, sets Partial, and continues.
type Recognizer interface {
	Recognize(ctx context.Context, img Image) (string, error)
}

// Result is the extracted text. Partial is meaningful only when error is nil.
type Result struct {
	Text    string
	Kind    string
	Partial bool
}

// Input is what a format package receives after the shell has read and split
// the MIME type.
type Input struct {
	Data      []byte
	MediaType string
	Params    map[string]string
	Filename  string
	MaxOut    int
	OCR       Recognizer
}

// Converter is one registered format.
type Converter interface {
	Kind() string
	MediaTypes() []string
	Extensions() []string
	Extract(ctx context.Context, in Input) (Result, error)
}

var (
	regMu  sync.RWMutex
	byKind = map[string]Converter{}
	byMIME = map[string]Converter{}
	byExt  = map[string]Converter{}
)

// Register adds c. It panics on a nil converter, an empty Kind, a duplicate
// Kind, or a MediaType or Extension already claimed. Call it from init.
func Register(c Converter) {
	if c == nil || c.Kind() == "" {
		panic("markitdown: invalid converter")
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, ok := byKind[c.Kind()]; ok {
		panic("markitdown: duplicate kind " + c.Kind())
	}
	for _, mt := range c.MediaTypes() {
		mt = strings.ToLower(mt)
		if _, ok := byMIME[mt]; ok {
			panic("markitdown: duplicate media type " + mt)
		}
	}
	for _, ext := range c.Extensions() {
		ext = strings.ToLower(ext)
		if _, ok := byExt[ext]; ok {
			panic("markitdown: duplicate extension " + ext)
		}
	}
	byKind[c.Kind()] = c
	for _, mt := range c.MediaTypes() {
		byMIME[strings.ToLower(mt)] = c
	}
	for _, ext := range c.Extensions() {
		byExt[strings.ToLower(ext)] = c
	}
}

// Kinds returns the registered kind names, sorted.
func Kinds() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(byKind))
	for k := range byKind {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Extract reads req.R and runs the one registered format that claims it.
func Extract(ctx context.Context, req Request) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if budget.Get(ctx) == nil {
		files := 100
		left := int64(MaxInput)
		ctx = budget.With(ctx, &budget.State{Depth: 0, Files: &files, Left: &left})
	}
	data, err := run.Read(req.R, MaxInput)
	if cerr := ctx.Err(); cerr != nil {
		return Result{}, cerr
	}
	if err != nil {
		return Result{}, err
	}
	maxOut := req.MaxOut
	if maxOut <= 0 {
		maxOut = DefaultMaxOut
	}
	mt, params := run.Media(req.MIME)
	c := find(mt, req.Filename)
	if c == nil {
		return Result{}, ErrUnsupported
	}
	res, err := c.Extract(ctx, Input{
		Data: data, MediaType: mt, Params: params,
		Filename: req.Filename, MaxOut: maxOut, OCR: req.OCR,
	})
	if err != nil {
		if cerr := ctx.Err(); cerr != nil && errors.Is(err, cerr) {
			return Result{}, cerr
		}
		if errors.Is(err, ErrUnsupported) || errors.Is(err, ErrCorrupt) || errors.Is(err, ErrTooLarge) {
			return Result{}, err
		}
		return Result{}, run.Wrap(err)
	}
	if res.Kind == "" {
		res.Kind = c.Kind()
	}
	if text, cut := run.Cut(res.Text, maxOut); cut {
		res.Text = text
		res.Partial = true
	}
	return res, nil
}

func find(mt, filename string) Converter {
	regMu.RLock()
	defer regMu.RUnlock()
	if mt != "" && mt != "application/octet-stream" {
		return byMIME[mt]
	}
	return byExt[extOf(filename)]
}

func extOf(filename string) string {
	filename = strings.ReplaceAll(filename, "\\", "/")
	return strings.ToLower(path.Ext(path.Base(filename)))
}
