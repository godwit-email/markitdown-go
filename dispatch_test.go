package markitdown_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/godwit-email/markitdown-go"
)

func TestConstantsAndSentinels(t *testing.T) {
	if markitdown.MaxInput != 25<<20 || markitdown.DefaultMaxOut != 2<<20 {
		t.Fatalf("caps %d %d", markitdown.MaxInput, markitdown.DefaultMaxOut)
	}
	if markitdown.ErrUnsupported.Error() != "markitdown: unsupported format" ||
		markitdown.ErrCorrupt.Error() != "markitdown: corrupt input" ||
		markitdown.ErrTooLarge.Error() != "markitdown: input too large" {
		t.Fatal("sentinel text changed")
	}
}

func TestKindsAreTheNineFormats(t *testing.T) {
	got := markitdown.Kinds()
	want := []string{"csv", "docx", "html", "pdf", "pptx", "text", "xls", "xlsx", "zip"}
	if len(got) != len(want) {
		t.Fatalf("Kinds = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Kinds = %v", got)
		}
	}
}

func TestDispatch(t *testing.T) {
	pdf := testFile(t, "test.pdf")
	xls := testFile(t, "test.xls")
	xlsx := testFile(t, "test.xlsx")
	ok := func(mime, name string, body []byte, kind string) {
		t.Helper()
		res, err := markitdown.Extract(context.Background(), markitdown.Request{
			R: bytes.NewReader(body), MIME: mime, Filename: name,
		})
		if err != nil || res.Kind != kind {
			t.Fatalf("%s %s: kind %q err %v", mime, name, res.Kind, err)
		}
	}
	unsup := func(mime, name string, body []byte) {
		t.Helper()
		_, err := markitdown.Extract(context.Background(), markitdown.Request{
			R: bytes.NewReader(body), MIME: mime, Filename: name,
		})
		if !errors.Is(err, markitdown.ErrUnsupported) {
			t.Fatalf("%s %s: %v", mime, name, err)
		}
	}
	ok("application/vnd.ms-excel", "sheet.xlsx", xls, "xls")
	ok("application/excel", "a.xls", xls, "xls")
	ok("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "a.xls", xlsx, "xlsx")
	unsup("application/x-pdf", "a.pdf", pdf)
	ok("application/pdf; name=\"a.pdf", "a.pdf", pdf, "pdf")
	ok("notatype", "a.csv", []byte("a,b\n1,2\n"), "csv")
	ok("text/plain; charset=", "a.txt", []byte("hello"), "text")
	ok("", "page.htm", []byte("<p>Hi</p>"), "html")
	ok("", "note.markdown", []byte("# Hi\n"), "text")
	unsup("", "page.xhtml", []byte("<p>Hi</p>"))
	unsup("", "noext", []byte("hello"))
	unsup("application/octet-stream", "noext", []byte("hello"))
	zip := zipEntries(t, []zipEntry{{"a.txt", "hi"}})
	ok("application/x-zip-compressed", "a.bin", zip, "zip")
}

type panicConv struct{ kind, mime, ext string }

func (p panicConv) Kind() string         { return p.kind }
func (p panicConv) MediaTypes() []string { return []string{p.mime} }
func (p panicConv) Extensions() []string { return []string{p.ext} }
func (panicConv) Extract(context.Context, markitdown.Input) (markitdown.Result, error) {
	return markitdown.Result{}, nil
}

func TestRegisterPanics(t *testing.T) {
	mustPanic := func(fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic")
			}
		}()
		fn()
	}
	mustPanic(func() { markitdown.Register(nil) })
	mustPanic(func() { markitdown.Register(panicConv{}) })
	mustPanic(func() { markitdown.Register(panicConv{kind: "pdf", mime: "application/x-test-pdf", ext: ".pdftest"}) })
	mustPanic(func() { markitdown.Register(panicConv{kind: "fresh-kind", mime: "application/pdf", ext: ".pdftest2"}) })
	mustPanic(func() {
		markitdown.Register(panicConv{kind: "fresh-kind-2", mime: "application/x-test-pdf-2", ext: ".pdf"})
	})
}
