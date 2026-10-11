package markitdown_test

import (
	"archive/zip"
	"bytes"
	"context"
	"github.com/godwit-email/markitdown-go"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestXLSVector(t *testing.T) {
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, "test.xls")), MIME: "application/vnd.ms-excel", Filename: "test.xls", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != "xls" {
		t.Fatal(res.Kind)
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

func TestDOCXComments(t *testing.T) {
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, "test_with_comment.docx")), MIME: "", Filename: "test_with_comment.docx", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"314b0a30-5b04-470b-b9f7-eed2c2bec74a",
		"49e168b7-d2ae-407f-a055-2167576f39a1",
		"## d666f1f7-46cb-42bd-9a39-9a39cf2a509f",
		"# Abstract",
		"# Introduction",
		"AutoGen: Enabling Next-Gen LLM Applications via Multi-Agent Conversation",
	} {
		if !strings.Contains(strings.ReplaceAll(res.Text, `\`, ""), s) {
			t.Fatalf("missing %s", s)
		}
	}
}

func TestDOCXEquations(t *testing.T) {
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, "equations.docx")), MIME: "", Filename: "equations.docx", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "$m=1$") {
		t.Fatalf("inline equation missing\n%s", res.Text)
	}
	if !regexp.MustCompile(`(?s)\$\$(.+?)\$\$`).MatchString(res.Text) {
		t.Fatalf("no block equation\n%s", res.Text)
	}
}

func TestDOCXMalformedEquations(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math" xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>Normal text</w:t></w:r></w:p>
    <m:oMathPara><m:oMath><m:r><m:t>x+1</m:t></m:r></m:oMath></m:oMathPara>
    <w:p><w:r><w:t>After good equation</w:t></w:r></w:p>
    <m:oMathPara></m:oMathPara>
    <w:p><w:r><w:t>After empty oMathPara</w:t></w:r></w:p>
    <m:oMath></m:oMath>
    <w:p><w:r><w:t>After empty inline oMath</w:t></w:r></w:p>
    <oMathPara><oMath><r><t>y+2</t></r></oMath></oMathPara>
    <w:p><w:r><w:t>After unnamespaced oMathPara</w:t></w:r></w:p>
    <oMath><r><t>z+3</t></r></oMath>
    <w:p><w:r><w:t>After unnamespaced oMath</w:t></w:r></w:p>
  </w:body>
</w:document>`
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(zipDoc(t, "word/document.xml", xml)), MIME: "", Filename: "a.docx", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"Normal text",
		"After empty oMathPara",
		"After empty inline oMath",
		"After unnamespaced oMathPara",
		"After unnamespaced oMath",
	} {
		if !strings.Contains(res.Text, s) {
			t.Fatalf("missing %s\n%s", s, res.Text)
		}
	}
	if !strings.Contains(res.Text, "$x+1$") && !strings.Contains(res.Text, "$$x+1$$") {
		t.Fatalf("equation dropped\n%s", res.Text)
	}
}

func TestDOCXTextBoxAndRevision(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>Before</w:t></w:r></w:p>
    <w:p><w:r><w:drawing><w:txbxContent><w:p><w:r><w:t>Inside text box 9f3c</w:t></w:r></w:p></w:txbxContent></w:drawing></w:r></w:p>
    <w:p><w:ins w:author="a"><w:r><w:t>Inserted revision</w:t></w:r></w:ins><w:del w:author="a"><w:r><w:delText>Deleted revision</w:delText></w:r></w:del></w:p>
  </w:body>
</w:document>`
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(zipDoc(t, "word/document.xml", xml)), MIME: "", Filename: "a.docx", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"Before", "Inside text box 9f3c", "Inserted revision", "Deleted revision"} {
		if !strings.Contains(res.Text, s) {
			t.Fatalf("missing %s\n%s", s, res.Text)
		}
	}
}

func TestPDFVectorsAndMasterformat(t *testing.T) {
	check := func(name string, include, exclude []string) {
		t.Helper()
		res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, name)), MIME: "application/pdf", Filename: name, MaxOut: 0})
		if err != nil {
			t.Fatal(name, err)
		}
		for _, s := range include {
			if !strings.Contains(res.Text, s) {
				t.Errorf("%s missing %q", name, s)
			}
		}
		for _, s := range exclude {
			if strings.Contains(res.Text, s) {
				t.Errorf("%s has excluded %q", name, s)
			}
		}
	}
	check("pdf_cleanup_form.pdf", []string{"ZAVA AUTO REPAIR", "| Customer Information |", "Gabriel Diaz", "| GRAND TOTAL", "5,738"}, []string{"While there is contemporaneous exploration"})
	check("pdf_cleanup_mixed.pdf", []string{"ZAVA AUTO REPAIR", "| Customer Information |", "Gabriel Diaz", "| GRAND TOTAL", "5,738", "While there is contemporaneous exploration of multi-agent approaches"}, nil)
	check("movie-theater-booking-2024.pdf", []string{"Holiday Spectacular", "IMAX 3D", "$12,500.00"}, nil)
	check("SPARSE-2024-INV-1234_borderless_table.pdf", []string{"INVENTORY RECONCILIATION REPORT", "SKU-8847", "| Product Code |", "$45.00"}, nil)
	check("RECEIPT-2024-TXN-98765_retail_purchase.pdf", []string{"TOTAL $821.14", "Sarah Mitchell", "TXN-98765-2024"}, nil)
	check("REPAIR-2022-INV-001_multipage.pdf", []string{"ZAVA AUTO REPAIR", "5,738", "Bruce Wayne", "211,522", "|"}, nil)
	check("masterformat_partial_numbering.pdf", []string{"RFP for Construction Management Services", "Section 00 00 43", "Ken Sargent House"}, nil)

	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, "masterformat_partial_numbering.pdf")), MIME: "", Filename: "masterformat_partial_numbering.pdf", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(res.Text, "\n") {
		cleaned := strings.TrimSpace(strings.ReplaceAll(line, "|", ""))
		switch cleaned {
		case ".1", ".2", ".3", ".4", ".5", ".6", ".7", ".8", ".9", ".10":
			t.Errorf("isolated numbering %q", line)
		}
	}
	if !regexp.MustCompile(`\.1\s+The intent`).MatchString(res.Text) {
		t.Error("missing .1 The intent")
	}
	if !regexp.MustCompile(`\.2\s+Available information`).MatchString(res.Text) {
		t.Error("missing .2 Available information")
	}

	scan, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, "MEDRPT-2024-PAT-3847_medical_report_scan.pdf")), MIME: "", Filename: "scan.pdf", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(scan.Text) != "" {
		t.Errorf("scanned PDF should be empty, got %d bytes", len(scan.Text))
	}
}

func TestPDFFullOutput(t *testing.T) {
	// Byte-for-byte pdfplumber markdown. Left failing when the pure-Go text
	// layer cannot reproduce that layout.
	files := []string{
		"movie-theater-booking-2024",
		"SPARSE-2024-INV-1234_borderless_table",
		"REPAIR-2022-INV-001_multipage",
		"RECEIPT-2024-TXN-98765_retail_purchase",
	}
	for _, name := range files {
		res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, name+".pdf")), MIME: "", Filename: name + ".pdf", MaxOut: 0})
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile("testdata/expected_outputs/" + name + ".md")
		if err != nil {
			t.Fatal(err)
		}
		if res.Text != string(want) {
			t.Errorf("%s output != expected markdown (%d vs %d bytes)", name, len(res.Text), len(want))
		}
	}
}

func zipDoc(t *testing.T, name, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
