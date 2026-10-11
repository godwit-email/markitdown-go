package markitdown_test

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/godwit-email/markitdown-go"
	"github.com/xuri/excelize/v2"
)

func addPart(t *testing.T, z []byte, name string, body []byte) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(z), int64(len(z)))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestOOXMLDeclaredPartIsCorrupt(t *testing.T) {
	doc := zipEntries(t, []zipEntry{{
		"word/document.xml",
		`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>hello</w:t></w:r></w:p></w:body></w:document>`,
	}})
	raw := patchUncompressed(t, addPart(t, doc, "word/huge.bin", []byte{1}), 1, uint32(markitdown.MaxInput+1))
	_, err := markitdown.Extract(context.Background(), markitdown.Request{
		R: bytes.NewReader(raw), MIME: "", Filename: "a.docx",
	})
	if !errors.Is(err, markitdown.ErrCorrupt) {
		t.Fatal(err)
	}
}

func TestOOXMLActualPartIsCorrupt(t *testing.T) {
	doc := zipEntries(t, []zipEntry{{
		"word/document.xml",
		`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>hello</w:t></w:r></w:p></w:body></w:document>`,
	}})
	pad := bytes.Repeat([]byte{'Z'}, markitdown.MaxInput+1)
	raw := patchUncompressed(t, addPart(t, doc, "word/pad.bin", pad), uint32(len(pad)), 100)
	_, err := markitdown.Extract(context.Background(), markitdown.Request{
		R: bytes.NewReader(raw), MIME: "", Filename: "a.docx",
	})
	if !errors.Is(err, markitdown.ErrCorrupt) {
		t.Fatal(err)
	}
}

func TestXLSXAndPPTXHonorPartBudget(t *testing.T) {
	f := excelize.NewFile()
	if err := f.SetCellValue("Sheet1", "A1", "hello"); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	raw := patchUncompressed(t, addPart(t, buf.Bytes(), "xl/huge.bin", []byte{1}), 1, uint32(markitdown.MaxInput+1))
	_, err := markitdown.Extract(context.Background(), markitdown.Request{
		R: bytes.NewReader(raw), MIME: "", Filename: "a.xlsx",
	})
	if !errors.Is(err, markitdown.ErrCorrupt) {
		t.Fatal(err)
	}
	slide := buildPPTX(t, "Title", "Body", "")
	raw = patchUncompressed(t, addPart(t, slide, "ppt/huge.bin", []byte{1}), 1, uint32(markitdown.MaxInput+1))
	_, err = markitdown.Extract(context.Background(), markitdown.Request{
		R: bytes.NewReader(raw), MIME: "", Filename: "a.pptx",
	})
	if !errors.Is(err, markitdown.ErrCorrupt) {
		t.Fatal(err)
	}
}

func flatePDF(t *testing.T, plain []byte) []byte {
	t.Helper()
	var comp bytes.Buffer
	zw := zlib.NewWriter(&comp)
	if _, err := zw.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	stream := comp.Bytes()
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	var offsets []int
	write := func(s string) {
		offsets = append(offsets, b.Len())
		b.WriteString(s)
	}
	write("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	write("2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] >>\nendobj\n")
	write("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R >>\nendobj\n")
	write(fmt.Sprintf("4 0 obj\n<< /Length %d /Filter /FlateDecode >>\nstream\n", len(stream)))
	b.Write(stream)
	b.WriteString("\nendstream\nendobj\n")
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return b.Bytes()
}

func TestPDFContentStreamBudget(t *testing.T) {
	small, err := markitdown.Extract(context.Background(), markitdown.Request{
		R: bytes.NewReader(flatePDF(t, []byte("BT (A) Tj ET\n"))), MIME: "application/pdf", Filename: "a.pdf",
	})
	if err != nil || !strings.Contains(small.Text, "A") {
		t.Fatalf("fixture: %#v %v", small, err)
	}
	op := []byte("BT (A) Tj ET\n")
	plain := bytes.Repeat(op, (markitdown.MaxInput+1)/len(op)+1)
	_, err = markitdown.Extract(context.Background(), markitdown.Request{
		R: bytes.NewReader(flatePDF(t, plain)), MIME: "application/pdf", Filename: "big.pdf",
	})
	if !errors.Is(err, markitdown.ErrCorrupt) || errors.Is(err, markitdown.ErrTooLarge) {
		t.Fatal(err)
	}
}
