package markitdown

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

func TestExtractTooLargeDoesNotReadPastCap(t *testing.T) {
	r := &countReader{left: maxInput + 100}
	_, err := Extract(r, "text/plain", "a.txt", 0)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
	if r.read > maxInput+1 {
		t.Fatalf("read %d bytes, cap is %d", r.read, maxInput+1)
	}
}

func TestExtractTruncatesOutput(t *testing.T) {
	res, err := Extract(strings.NewReader("abcdefghijklmnopqrstuvwxyz"), "text/plain", "a.txt", 4)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "abcd" || !res.Partial || res.Kind != KindText {
		t.Fatalf("%#v", res)
	}
}

func TestExtractDefaultMaxOut(t *testing.T) {
	res, err := Extract(strings.NewReader("hello"), "", "note.txt", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "hello" || res.Partial || res.Kind != KindText {
		t.Fatalf("%#v", res)
	}
}

func TestMIMEBeatsExtension(t *testing.T) {
	res, err := Extract(strings.NewReader("a,b\n1,2\n"), "text/plain", "a.csv", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindText || res.Text != "a,b\n1,2\n" {
		t.Fatalf("%#v", res)
	}
}

func TestOctetStreamUsesExtension(t *testing.T) {
	res, err := Extract(strings.NewReader("a,b\n1,2\n"), "application/octet-stream", "a.csv", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindCSV || res.Text != "| a | b |\n| --- | --- |\n| 1 | 2 |" {
		t.Fatalf("%#v", res)
	}
}

func TestUnsupported(t *testing.T) {
	cases := []struct{ mime, name string }{
		{"", "a.xls"},
		{"application/vnd.ms-excel", "a.xlsx"},
		{"text/html", "a.txt"},
		{"", "a.html"},
		{"application/octet-stream", "a.bin"},
	}
	for _, tc := range cases {
		_, err := Extract(strings.NewReader("x"), tc.mime, tc.name, 0)
		if !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s %s: %v", tc.mime, tc.name, err)
		}
	}
}

func TestOrdinaryTextAndNoneWord(t *testing.T) {
	res, err := Extract(strings.NewReader("hello, world\n"), "", "a.txt", 0)
	if err != nil || res.Text != "hello, world\n" {
		t.Fatalf("%#v %v", res, err)
	}
	res, err = Extract(strings.NewReader("value,note\nNone,missing\n"), "", "a.csv", 0)
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
	res, err := Extract(bytes.NewReader(body), "text/plain; charset=cp1252", "a.txt", 0)
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
