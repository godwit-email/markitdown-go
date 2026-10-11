package markitdown_test

import (
	"bytes"
	"context"
	"errors"
	"github.com/godwit-email/markitdown-go"
	"io"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

func TestExtractTooLargeDoesNotReadPastCap(t *testing.T) {
	r := &countReader{left: markitdown.MaxInput + 100}
	_, err := markitdown.Extract(context.Background(), markitdown.Request{R: r, MIME: "text/plain", Filename: "a.txt", MaxOut: 0})
	if !errors.Is(err, markitdown.ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
	if r.read > markitdown.MaxInput+1 {
		t.Fatalf("read %d bytes, cap is %d", r.read, markitdown.MaxInput+1)
	}
}

func TestExtractTruncatesOutput(t *testing.T) {
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: strings.NewReader("abcdefghijklmnopqrstuvwxyz"), MIME: "text/plain", Filename: "a.txt", MaxOut: 4})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "abcd" || !res.Partial || res.Kind != "text" {
		t.Fatalf("%#v", res)
	}
}

func TestExtractDefaultMaxOut(t *testing.T) {
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: strings.NewReader("hello"), MIME: "", Filename: "note.txt", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "hello" || res.Partial || res.Kind != "text" {
		t.Fatalf("%#v", res)
	}
}

func TestMIMEBeatsExtension(t *testing.T) {
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: strings.NewReader("a,b\n1,2\n"), MIME: "text/plain", Filename: "a.csv", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != "text" || res.Text != "a,b\n1,2\n" {
		t.Fatalf("%#v", res)
	}
}

func TestOctetStreamUsesExtension(t *testing.T) {
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: strings.NewReader("a,b\n1,2\n"), MIME: "application/octet-stream", Filename: "a.csv", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != "csv" || res.Text != "| a | b |\n| --- | --- |\n| 1 | 2 |" {
		t.Fatalf("%#v", res)
	}
}

func TestUnsupported(t *testing.T) {
	cases := []struct{ mime, name string }{
		{"application/octet-stream", "a.bin"},
	}
	for _, tc := range cases {
		_, err := markitdown.Extract(context.Background(), markitdown.Request{R: strings.NewReader("x"), MIME: tc.mime, Filename: tc.name, MaxOut: 0})
		if !errors.Is(err, markitdown.ErrUnsupported) {
			t.Errorf("%s %s: %v", tc.mime, tc.name, err)
		}
	}
}

func TestOrdinaryTextAndNoneWord(t *testing.T) {
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: strings.NewReader("hello, world\n"), MIME: "", Filename: "a.txt", MaxOut: 0})
	if err != nil || res.Text != "hello, world\n" {
		t.Fatalf("%#v %v", res, err)
	}
	res, err = markitdown.Extract(context.Background(), markitdown.Request{R: strings.NewReader("value,note\nNone,missing\n"), MIME: "", Filename: "a.csv", MaxOut: 0})
	if err != nil || res.Text != "| value | note |\n| --- | --- |\n| None | missing |" {
		t.Fatalf("%#v %v", res, err)
	}
}

func TestDeclaredCharsetWins(t *testing.T) {
	body := []byte{0xC3, 0xA9}
	want, err := charmap.Windows1252.NewDecoder().String(string(body))
	if err != nil {
		t.Fatal(err)
	}
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(body), MIME: "text/plain; charset=cp1252", Filename: "a.txt", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != want || res.Text == "é" {
		t.Fatalf("%q", res.Text)
	}
}

type countReader struct {
	left int
	read int
}

func (c *countReader) Read(p []byte) (int, error) {
	if c.left <= 0 {
		return 0, io.EOF
	}
	n := len(p)
	if n > c.left {
		n = c.left
	}
	for i := 0; i < n; i++ {
		p[i] = 'x'
	}
	c.left -= n
	c.read += n
	return n, nil
}
