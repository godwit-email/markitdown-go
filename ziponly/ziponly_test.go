package ziponly_test

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/godwit-email/markitdown-go"
	_ "github.com/godwit-email/markitdown-go/zip"
)

func TestZipWithoutMemberFormatsIsEmptyPartial(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	res, err := markitdown.Extract(context.Background(), markitdown.Request{
		R: bytes.NewReader(buf.Bytes()), MIME: "application/zip", Filename: "a.zip",
	})
	if err != nil || res.Kind != "zip" || res.Text != "" || !res.Partial {
		t.Fatalf("%#v %v", res, err)
	}
	if strings.Contains(res.Text, "hello") || strings.Contains(res.Text, "Content from") {
		t.Fatalf("%q", res.Text)
	}
}
