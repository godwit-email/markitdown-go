package markitdown_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"github.com/godwit-email/markitdown-go"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestJSONFixtures(t *testing.T) {
	for _, name := range []string{"test.json", "json_late_non_ascii.json"} {
		res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, name)), MIME: "application/json", Filename: name, MaxOut: 0})
		if err != nil {
			t.Fatal(name, err)
		}
		for _, s := range []string{
			"5b64c88c-b3c3-4510-bcb8-da0b200602d8",
			"9700dc99-6685-40b4-9a3a-5e406dcb37f3",
		} {
			if !strings.Contains(res.Text, s) {
				t.Fatalf("%s missing %s", name, s)
			}
		}
		if strings.Contains(res.Text, "\uFFFD") {
			t.Fatalf("%s has replacement char", name)
		}
	}
	late := string(testFile(t, "json_late_non_ascii.json"))
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: strings.NewReader(late), MIME: "application/json; charset=utf-8", Filename: "json_late_non_ascii.json", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, `"notes": "Late Unicode: café, 日本語, 😀"`) {
		t.Fatal("late unicode missing")
	}
	if strings.Contains(res.Text, "cafÃ©") {
		t.Fatal("mojibake")
	}
}

func TestMSKanjiCSV(t *testing.T) {
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, "test_mskanji.csv")), MIME: "text/csv; charset=cp932", Filename: "test_mskanji.csv", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"| 名前 | 年齢 | 住所 |",
		"| --- | --- | --- |",
		"| 佐藤太郎 | 30 | 東京 |",
		"| 三木英子 | 25 | 大阪 |",
		"| 髙橋淳 | 35 | 名古屋 |",
	} {
		if !strings.Contains(res.Text, s) {
			t.Fatalf("missing %s\n%s", s, res.Text)
		}
	}
}

func TestGB18030CSV(t *testing.T) {
	body, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte("名称,代码\n浦发银行,600000\n"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(body), MIME: "text/csv; charset=gb18030", Filename: "a.csv", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "浦发银行") {
		t.Fatalf("%s", res.Text)
	}
}

func TestXLSXFixture(t *testing.T) {
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, "test.xlsx")), MIME: "", Filename: "test.xlsx", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != "xlsx" || res.Partial {
		t.Fatalf("%#v", res)
	}
	for _, s := range []string{
		"## 09060124-b5e7-4717-9d07-3c046eb",
		"6ff4173b-42a5-4784-9b19-f49caff4d93d",
		"affc7dad-52dc-4b98-9b5d-51e65d8a8ad0",
	} {
		if !strings.Contains(res.Text, s) {
			t.Fatalf("missing %s\n%s", s, res.Text)
		}
	}
}

func TestXLSXDropsEmptySheetAndCapsCells(t *testing.T) {
	f := excelize.NewFile()
	f.SetCellValue("Sheet1", "A1", "keep")
	if _, err := f.NewSheet("empty"); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(buf.Bytes()), MIME: "", Filename: "a.xlsx", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Text, "empty") || !strings.Contains(res.Text, "keep") || res.Partial {
		t.Fatalf("%#v", res)
	}

	big := excelize.NewFile()
	row := make([]any, 1)
	for i := 1; i <= 50000+1; i++ {
		row[0] = "c"
		cell, _ := excelize.CoordinatesToCellName(1, i)
		if err := big.SetCellValue("Sheet1", cell, "c"); err != nil {
			t.Fatal(err)
		}
	}
	buf.Reset()
	if err := big.Write(&buf); err != nil {
		t.Fatal(err)
	}
	res, err = markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(buf.Bytes()), MIME: "", Filename: "big.xlsx", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Partial || !strings.Contains(res.Text, "c") {
		t.Fatalf("partial=%v len=%d", res.Partial, len(res.Text))
	}
}

func TestXLSXCorrupt(t *testing.T) {
	_, err := markitdown.Extract(context.Background(), markitdown.Request{R: strings.NewReader("not a workbook"), MIME: "", Filename: "a.xlsx", MaxOut: 0})
	if !errors.Is(err, markitdown.ErrCorrupt) {
		t.Fatal(err)
	}
}

func TestDOCXFixture(t *testing.T) {
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, "test.docx")), MIME: "", Filename: "test.docx", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != "docx" {
		t.Fatal(res.Kind)
	}
	for _, s := range []string{
		"314b0a30-5b04-470b-b9f7-eed2c2bec74a",
		"49e168b7-d2ae-407f-a055-2167576f39a1",
		"## d666f1f7-46cb-42bd-9a39-9a39cf2a509f",
		"# Abstract",
		"# Introduction",
		"AutoGen: Enabling Next-Gen LLM Applications via Multi-Agent Conversation",
	} {
		if !strings.Contains(res.Text, s) {
			t.Fatalf("missing %s", s)
		}
	}
}

func TestDOCXExternalEntity(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte("pwned"))
	}))
	defer srv.Close()
	xml := `<?xml version="1.0"?>` +
		`<!DOCTYPE foo [<!ENTITY xxe SYSTEM "` + srv.URL + `">]>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body><w:p><w:r><w:t>&xxe;</w:t></w:r></w:p></w:body></w:document>`
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(xml)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(buf.Bytes()), MIME: "", Filename: "a.docx", MaxOut: 0})
	if !errors.Is(err, markitdown.ErrCorrupt) {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Fatalf("entity fetch hits %d", hits)
	}
}

func TestPPTXFixtures(t *testing.T) {
	cases := []struct {
		name    string
		include []string
		exclude []string
	}{
		{"pptx_empty_title_run.pptx", []string{
			"AutoGen: Enabling Next-Gen LLM Applications",
			"Text after empty shape",
			"Preserved body text",
			"### Notes:\nPreserved speaker notes",
		}, []string{"Preserved slide title", "None"}},
		{"pptx_empty_body_run.pptx", []string{
			"AutoGen: Enabling Next-Gen LLM Applications",
			"Text after empty shape",
			"# Preserved slide title",
			"### Notes:\nPreserved speaker notes",
		}, []string{"Preserved body text", "None"}},
		{"pptx_notes_empty_run.pptx", []string{
			"AutoGen: Enabling Next-Gen LLM Applications",
			"Text after empty shape",
			"# Preserved slide title",
			"Preserved body text",
		}, []string{"Preserved speaker notes", "### Notes:", "None"}},
		{"pptx_notes_missing_body_frame.pptx", []string{
			"AutoGen: Enabling Next-Gen LLM Applications",
			"Text after empty shape",
			"# Preserved slide title",
			"Preserved body text",
		}, []string{"Preserved speaker notes", "### Notes:", "None"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, tc.name)), MIME: "", Filename: tc.name, MaxOut: 0})
			if err != nil {
				t.Fatal(err)
			}
			if res.Kind != "pptx" {
				t.Fatal(res.Kind)
			}
			for _, s := range tc.include {
				if !strings.Contains(res.Text, s) {
					t.Fatalf("missing %q\n%s", s, res.Text)
				}
			}
			for _, s := range tc.exclude {
				if strings.Contains(res.Text, s) {
					t.Fatalf("has %q\n%s", s, res.Text)
				}
			}
		})
	}
}

func TestPPTXTitleAndNotes(t *testing.T) {
	withTitle := buildPPTX(t, "Quarterly Results", "Body text", "Remember to mention the budget.")
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(withTitle), MIME: "", Filename: "a.pptx", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "# Quarterly Results") || !strings.Contains(res.Text, "Body text") {
		t.Fatalf("%s", res.Text)
	}
	if !strings.Contains(res.Text, "### Notes:\nRemember to mention the budget.") {
		t.Fatalf("%s", res.Text)
	}

	emptyTitle := buildPPTX(t, "   ", "Body text", "")
	res, err = markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(emptyTitle), MIME: "", Filename: "a.pptx", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Text, "#") || !strings.Contains(res.Text, "Body text") {
		t.Fatalf("%s", res.Text)
	}
	if strings.Contains(res.Text, "### Notes:") {
		t.Fatal(res.Text)
	}
}

func TestPDFTextLayer(t *testing.T) {
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, "test.pdf")), MIME: "application/pdf", Filename: "test.pdf", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != "pdf" || res.Partial {
		t.Fatalf("%#v", res.Kind)
	}
	if !strings.Contains(squash(res.Text), "While there is contemporaneous exploration of multi-agent approaches") {
		t.Fatalf("missing sentence\n%s", res.Text[:min(500, len(res.Text))])
	}

	plain, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, "pdf_cleanup_plain.pdf")), MIME: "", Filename: "pdf_cleanup_plain.pdf", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	flat := squash(plain.Text)
	if !strings.Contains(flat, "While there is contemporaneous exploration of multi-agent approaches") {
		t.Fatal("plain pdf missing sentence")
	}
	if strings.Contains(plain.Text, "ZAVA AUTO REPAIR") || strings.Contains(plain.Text, "| Customer Information |") {
		t.Fatal("plain pdf included form text")
	}
}

func TestPDFScannedIsPartial(t *testing.T) {
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, "MEDRPT-2024-PAT-3847_medical_report_scan.pdf")), MIME: "", Filename: "scan.pdf", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(res.Text) == "" && !res.Partial {
		t.Fatal("empty text layer should be partial")
	}
	if strings.TrimSpace(res.Text) == "" && res.Kind != "pdf" {
		t.Fatal(res.Kind)
	}
}

func TestPDFCorruptAndEncrypted(t *testing.T) {
	_, err := markitdown.Extract(context.Background(), markitdown.Request{R: strings.NewReader("%PDF-1.4\nnot really\n%%EOF\n"), MIME: "", Filename: "a.pdf", MaxOut: 0})
	if !errors.Is(err, markitdown.ErrCorrupt) {
		t.Fatal(err)
	}
	_, err = markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(encryptedPDF()), MIME: "", Filename: "secret.pdf", MaxOut: 0})
	if !errors.Is(err, markitdown.ErrUnsupported) {
		t.Fatal(err)
	}
}

func TestZIPMemberDOCXIsNotUnpacked(t *testing.T) {
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, "test_files.zip")), MIME: "application/zip", Filename: "test_files.zip", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != "zip" {
		t.Fatal(res.Kind)
	}
	if !strings.Contains(res.Text, "## File: test.docx") {
		t.Fatalf("%s", res.Text[:min(400, len(res.Text))])
	}
	if strings.Contains(res.Text, "[Content_Types].xml") {
		t.Fatal("docx was unpacked as zip")
	}
	if !res.Partial {
		t.Fatal("jpeg member has no converter in this version")
	}
	if strings.Contains(res.Text, "ImageSize:") {
		t.Fatal("jpeg member was converted")
	}
}

func TestZIPBudgets(t *testing.T) {
	ok := zipOf(t, map[string]string{"notes/hello.txt": "Hello world"})
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(ok), MIME: "", Filename: "a.zip", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if res.Partial || !strings.Contains(res.Text, "## File: notes/hello.txt") || !strings.Contains(res.Text, "Hello world") {
		t.Fatalf("%#v", res)
	}

	mixed := zipOf(t, map[string]string{"a.txt": "alpha", "b.bin": "nope"})
	res, err = markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(mixed), MIME: "", Filename: "m.zip", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Partial || !strings.Contains(res.Text, "alpha") || strings.Contains(res.Text, "nope") {
		t.Fatalf("%#v", res)
	}

	deep := nestZip(t, 4, "leaf.txt", "deep")
	res, err = markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(deep), MIME: "", Filename: "d.zip", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Partial || strings.Contains(res.Text, "deep") {
		t.Fatalf("depth partial=%v text=%q", res.Partial, res.Text)
	}
	three := nestZip(t, 3, "leaf.txt", "ok")
	res, err = markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(three), MIME: "", Filename: "d.zip", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if res.Partial || !strings.Contains(res.Text, "ok") {
		t.Fatalf("three partial=%v text=%q", res.Partial, res.Text)
	}

	var files []zipEntry
	for i := 0; i < 100+1; i++ {
		files = append(files, zipEntry{name: "f" + strconv.Itoa(i) + ".txt", body: "x"})
	}
	res, err = markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(zipEntries(t, files)), MIME: "", Filename: "many.zip", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Partial {
		t.Fatal("expected partial on file cap")
	}

	res, err = markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(zipBomb(t)), MIME: "", Filename: "bomb.zip", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Partial || strings.Contains(res.Text, strings.Repeat("0", 1000)) {
		t.Fatalf("bomb partial=%v len=%d", res.Partial, len(res.Text))
	}
	if !strings.Contains(res.Text, "small") {
		t.Fatalf("%s", res.Text)
	}

	_, err = markitdown.Extract(context.Background(), markitdown.Request{R: strings.NewReader("not a zip"), MIME: "", Filename: "bad.zip", MaxOut: 0})
	if !errors.Is(err, markitdown.ErrCorrupt) {
		t.Fatal(err)
	}
}

func TestDOCXNotKindZip(t *testing.T) {
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, "test.docx")), MIME: "application/octet-stream", Filename: "test.docx", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != "docx" {
		t.Fatal(res.Kind)
	}
}

func testFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func squash(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func encryptedPDF() []byte {
	o := strings.Repeat("O", 32)
	u := strings.Repeat("U", 32)
	id := strings.Repeat("I", 16)
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	xref := pdf.Len()
	pdf.WriteString("xref\n0 1\n0000000000 65535 f \n")
	pdf.WriteString("trailer\n<< /Size 1 /ID [(" + id + ")] /Encrypt << /Filter /Standard /V 1 /R 2 /Length 40 /O (" + o + ") /U (" + u + ") /P 0 >> >>\n")
	pdf.WriteString("startxref\n")
	pdf.WriteString(strconv.Itoa(xref))
	pdf.WriteString("\n%%EOF\n")
	return pdf.Bytes()
}

type zipEntry struct {
	name string
	body string
}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var entries []zipEntry
	for name, body := range files {
		entries = append(entries, zipEntry{name, body})
	}
	return zipEntries(t, entries)
}

func zipEntries(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func nestZip(t *testing.T, depth int, name, body string) []byte {
	t.Helper()
	payload := []byte(body)
	curName := name
	for i := 0; i < depth; i++ {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, err := zw.Create(curName)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(payload); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		payload = buf.Bytes()
		curName = "inner.zip"
	}
	return payload
}

func zipBomb(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("small.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("small")); err != nil {
		t.Fatal(err)
	}
	hdr := &zip.FileHeader{Name: "big.txt", Method: zip.Deflate}
	w, err = zw.CreateHeader(hdr)
	if err != nil {
		t.Fatal(err)
	}
	chunk := bytes.Repeat([]byte("0"), 1<<20)
	for i := 0; i < 26; i++ {
		if _, err := w.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func buildPPTX(t *testing.T, title, body, notes string) []byte {
	t.Helper()
	slide := `<?xml version="1.0" encoding="UTF-8"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
<p:cSld><p:spTree>
<p:sp><p:nvSpPr><p:cNvPr id="1" name="Title"/><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr><p:txBody><a:p><a:r><a:t>` + xmlEsc(title) + `</a:t></a:r></a:p></p:txBody></p:sp>
<p:sp><p:nvSpPr><p:cNvPr id="2" name="Body"/><p:nvPr><p:ph type="body"/></p:nvPr></p:nvSpPr><p:txBody><a:p><a:r><a:t>` + xmlEsc(body) + `</a:t></a:r></a:p></p:txBody></p:sp>
</p:spTree></p:cSld></p:sld>`
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZip(t, zw, "ppt/slides/slide1.xml", slide)
	if notes != "" {
		note := `<?xml version="1.0" encoding="UTF-8"?>
<p:notes xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
<p:cSld><p:spTree>
<p:sp><p:nvSpPr><p:cNvPr id="3" name="Notes"/><p:nvPr><p:ph type="body"/></p:nvPr></p:nvSpPr><p:txBody><a:p><a:r><a:t>` + xmlEsc(notes) + `</a:t></a:r></a:p></p:txBody></p:sp>
</p:spTree></p:cSld></p:notes>`
		writeZip(t, zw, "ppt/notesSlides/notesSlide1.xml", note)
		writeZip(t, zw, "ppt/slides/_rels/slide1.xml.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesSlide" Target="../notesSlides/notesSlide1.xml"/></Relationships>`)
	}
	writeZip(t, zw, "ppt/presentation.xml", `<p:presentation/>`)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeZip(t *testing.T, zw *zip.Writer, name, body string) {
	t.Helper()
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
}

func xmlEsc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	return s
}
