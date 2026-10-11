package markitdown_test

import (
	"bytes"
	"context"
	"github.com/godwit-email/markitdown-go"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

func TestExplicitCharsetStillWins(t *testing.T) {
	prefix := []byte(`{"name":"résumé","notes":"`)
	data := append(append([]byte{}, prefix...), bytes.Repeat([]byte("a"), 65536-1-len(prefix))...)
	data = append(data, []byte("é"+`"}`)...)
	want, err := charmap.Windows1252.NewDecoder().Bytes(data)
	if err != nil {
		t.Fatal(err)
	}
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(data), MIME: "application/json; charset=cp1252", Filename: "a.json", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != string(want) {
		t.Fatal("cp1252 did not win")
	}
}

func TestLateNonASCIIIndex(t *testing.T) {
	data := testFile(t, "json_late_non_ascii.json")
	first := -1
	for i, b := range data {
		if b >= 128 {
			first = i
			break
		}
	}
	if first <= 8192 || first >= 65536 {
		t.Fatalf("first non-ascii at %d", first)
	}
}

func TestBinaryExtensionsAreNotTheWordNone(t *testing.T) {
	undecodable := undecodableBytes()
	for _, extn := range []string{".txt", ".md"} {
		res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(undecodable), MIME: "", Filename: "a" + extn, MaxOut: 0})
		if err != nil {
			t.Fatal(extn, err)
		}
		if res.Text == "None" {
			t.Fatalf("%s became None", extn)
		}
	}
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(undecodable), MIME: "", Filename: "a.csv", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text == "| None |\n| --- |" {
		t.Fatalf("%s", res.Text)
	}
}

func undecodableBytes() []byte {
	b := make([]byte, 4096)
	for i := range b {
		b[i] = byte((7*i*i + 251*i + 193) % 256)
	}
	return b
}
