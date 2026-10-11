package markitdown_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/godwit-email/markitdown-go"
)

func TestZIPRemaining(t *testing.T) {
	buf := zipEntries(t, []zipEntry{{"notes.txt", "First archived entry."}, {"notes.txt", "Second archived entry."}})
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(buf), MIME: "application/zip", Filename: "duplicate.zip", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	want := "Content from the zip file `duplicate.zip`:\n\n## File: notes.txt\n\nFirst archived entry.\n\n## File: notes.txt\n\nSecond archived entry."
	if res.Text != want {
		t.Fatalf("%q", res.Text)
	}
	hello := zipOf(t, map[string]string{"hello.txt": "Hello world"})
	res, err = markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(hello), MIME: "application/zip", Filename: "", MaxOut: 0})
	if err != nil || !strings.HasPrefix(res.Text, "Content from the zip file `(unknown)`:\n\n") || !strings.Contains(res.Text, "Hello world") {
		t.Fatalf("%q %v", res.Text, err)
	}
	docx := testFile(t, "test.docx")
	alone, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(docx), MIME: "", Filename: "test.docx", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	wrapped := zipEntries(t, []zipEntry{{"test.docx", string(docx)}})
	res, err = markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(wrapped), MIME: "", Filename: "archive.zip", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "## File: test.docx") || !strings.Contains(res.Text, strings.TrimSpace(alone.Text)) {
		t.Fatal("docx member mismatch")
	}
	if strings.Contains(res.Text, "word/document.xml") || strings.Contains(res.Text, "[Content_Types].xml") {
		t.Fatal("unpacked docx")
	}
	bundle, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, "test_files.zip")), MIME: "application/zip", Filename: "test_files.zip", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"314b0a30-5b04-470b-b9f7-eed2c2bec74a",
		"# Abstract",
		"## 09060124-b5e7-4717-9d07-3c046eb",
		"Microsoft entered the operating system (OS) business in 1980 with its own version of [Unix]",
		`[Bill Gates](/wiki/Bill_Gates "Bill Gates")`,
	} {
		if !strings.Contains(bundle.Text, s) {
			t.Fatalf("zip missing %s", s)
		}
	}
}
