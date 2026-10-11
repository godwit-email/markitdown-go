package markitdown

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

const ocrMock = "MOCK_OCR_TEXT_12345"
const ocrBlock = "*[Image OCR]  \nMOCK\\_OCR\\_TEXT\\_12345  \n[End OCR]*"

type fixedOCR struct{ text string }

func (f fixedOCR) ExtractText(OCRImage) (OCRResult, error) {
	return OCRResult{Text: f.text}, nil
}

func TestOCROfficeFixtures(t *testing.T) {
	ocr := fixedOCR{text: ocrMock}
	cases := []struct{ name, want string }{
		{"docx_image_start.docx", "Document with Image at Start\n\n" + ocrBlock + "\n\nThis is the main content after the header image.\n\nMore text content here."},
		{"docx_image_middle.docx", "# Introduction\n\nThis is the introduction section.\n\nWe will see an image below.\n\n" + ocrBlock + "\n\n# Analysis\n\nThis section comes after the image."},
		{"docx_image_end.docx", "Report\n\nMain findings of the report.\n\nDetails and analysis.\n\nRecommendations.\n\n" + ocrBlock},
		{"docx_multiple_images.docx", "Multi-Image Document\n\nFirst section\n\n" + ocrBlock + "\n\nSecond section with another image\n\n" + ocrBlock + "\n\nConclusion"},
		{"docx_multipage.docx", "# Page 1 - Mixed Content\n\nThis is the first paragraph on page 1.\n\nBEFORE IMAGE: Important content appears here.\n\n" + ocrBlock + "\n\nAFTER IMAGE: This content follows the image.\n\nMore text on page 1.\n\n# Page 2 - Image at End\n\nContent on page 2.\n\nMultiple paragraphs of text.\n\nBuilding up to the image...\n\nFinal paragraph before image.\n\n" + ocrBlock + "\n\n# Page 3 - Image at Start\n\n" + ocrBlock + "\n\nContent that follows the header image.\n\nAFTER IMAGE: This text is after the image."},
		{"docx_complex_layout.docx", "Complex Document\n\n|  |  |\n| --- | --- |\n| Feature | Status |\n| Authentication | Active |\n| Encryption | Enabled |\n\nSecurity notice:\n\n" + ocrBlock},
		{"xlsx_image_start.xlsx", "## Sales Q1\n| Product | Sales |\n| --- | --- |\n| Widget A | 100 |\n| Widget B | 150 |\n\n### Images in this sheet:\n\n" + ocrBlock + "\n\n## Forecast Q2\n| Projected Sales | Unnamed: 1 |\n| --- | --- |\n| Widget A | 120 |\n| Widget B | 180 |\n\n### Images in this sheet:\n\n" + ocrBlock},
		{"xlsx_image_middle.xlsx", "## Revenue\n| Q1 Report | Unnamed: 1 |\n| --- | --- |\n| NaN | NaN |\n| Revenue | $50,000 |\n| NaN | NaN |\n| NaN | NaN |\n| NaN | NaN |\n| NaN | NaN |\n| Profit Margin | 40% |\n\n### Images in this sheet:\n\n" + ocrBlock + "\n\n## Expenses\n| Expense Breakdown | Unnamed: 1 |\n| --- | --- |\n| NaN | NaN |\n| Expenses | $30,000 |\n| NaN | NaN |\n| NaN | NaN |\n| NaN | NaN |\n| NaN | NaN |\n| Savings | $5,000 |\n\n### Images in this sheet:\n\n" + ocrBlock},
		{"xlsx_image_end.xlsx", "## Sheet\n| Financial Summary | Unnamed: 1 |\n| --- | --- |\n| Total Revenue | $500,000 |\n| Total Expenses | $300,000 |\n| Net Profit | $200,000 |\n| NaN | NaN |\n| NaN | NaN |\n| NaN | NaN |\n| NaN | NaN |\n| NaN | NaN |\n| Signature: | NaN |\n\n### Images in this sheet:\n\n" + ocrBlock + "\n\n## Budget\n| Budget Allocation | Unnamed: 1 |\n| --- | --- |\n| Marketing | $100,000 |\n| R&D | $150,000 |\n| Operations | $50,000 |\n| NaN | NaN |\n| NaN | NaN |\n| NaN | NaN |\n| NaN | NaN |\n| NaN | NaN |\n| Approved: | NaN |\n\n### Images in this sheet:\n\n" + ocrBlock},
		{"xlsx_multiple_images.xlsx", "## Overview\n| Dashboard |\n| --- |\n| Status: Active |\n| NaN |\n| NaN |\n| NaN |\n| NaN |\n| Performance Summary |\n\n### Images in this sheet:\n\n" + ocrBlock + "\n\n" + ocrBlock + "\n\n## Details\n| Detailed Metrics |\n| --- |\n| System Health |\n\n### Images in this sheet:\n\n" + ocrBlock + "\n\n## Summary\n| Quarter Summary |\n| --- |\n| Overall Performance |\n\n### Images in this sheet:\n\n" + ocrBlock},
		{"xlsx_complex_layout.xlsx", "## Complex Report\n| Annual Report 2024 | Unnamed: 1 |\n| --- | --- |\n| NaN | NaN |\n| Month | Sales |\n| Jan | 1000 |\n| Feb | 1200 |\n| NaN | NaN |\n| Total | 2200 |\n\n### Images in this sheet:\n\n" + ocrBlock + "\n\n" + ocrBlock + "\n\n## Customers\n| Customer Metrics | Unnamed: 1 |\n| --- | --- |\n| NaN | NaN |\n| New Customers | 250 |\n| Retention Rate | 92% |\n\n### Images in this sheet:\n\n" + ocrBlock + "\n\n## Regions\n| Regional Breakdown | Unnamed: 1 |\n| --- | --- |\n| NaN | NaN |\n| Region | Revenue |\n| North | $800K |\n| South | $600K |\n\n### Images in this sheet:\n\n" + ocrBlock},
		{"pptx_image_start.pptx", "<!-- Slide number: 1 -->\n# Welcome\n\n\n" + ocrBlock},
		{"pptx_image_middle.pptx", "<!-- Slide number: 1 -->\n# Introduction\n\n<!-- Slide number: 2 -->\n# Architecture\n\n\n" + ocrBlock + "\n\n<!-- Slide number: 3 -->\n# Conclusion"},
		{"pptx_image_end.pptx", "<!-- Slide number: 1 -->\n# Presentation\n\n<!-- Slide number: 2 -->\n# Thank You\n\n\n" + ocrBlock},
		{"pptx_multiple_images.pptx", "<!-- Slide number: 1 -->\n\n" + ocrBlock + "\n\n" + ocrBlock},
		{"pptx_complex_layout.pptx", "<!-- Slide number: 1 -->\n# Product Comparison\n\nOur products lead the market\n\n" + ocrBlock},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := ExtractOptions(bytes.NewReader(ocrFile(t, tc.name)), "", tc.name, 0, Options{OCR: ocr})
			if err != nil {
				t.Fatal(err)
			}
			if res.Text != tc.want {
				t.Fatalf("got:\n%s", res.Text)
			}
		})
	}
}

func TestOCRKeepsUnderline(t *testing.T) {
	body := `<w:r><w:t xml:space="preserve">plain </w:t></w:r><w:r><w:rPr><w:u w:val="single"/></w:rPr><w:t>underlined</w:t></w:r>`
	doc := docxWithBody(t, body)
	res, err := ExtractOptions(bytes.NewReader(doc), "", "a.docx", 0, Options{OCR: fixedOCR{text: ocrMock}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "plain <u>underlined</u>") {
		t.Fatalf("%s", res.Text)
	}
}

func TestOCRFixturesHaveNoTagsWithoutService(t *testing.T) {
	for _, name := range []string{"docx_image_middle.docx", "xlsx_image_middle.xlsx", "pptx_image_middle.pptx"} {
		res, err := Extract(bytes.NewReader(ocrFile(t, name)), "", name, 0)
		if err != nil {
			t.Fatal(name, err)
		}
		if strings.Contains(res.Text, "*[Image OCR]") || strings.Contains(res.Text, "[End OCR]*") {
			t.Fatalf("%s has ocr tags\n%s", name, res.Text)
		}
	}
}

func TestOCRVisionRequestUsesFakeClient(t *testing.T) {
	var got VisionRequest
	client := visionFunc(func(req VisionRequest) (string, error) {
		got = req
		return "Recognized_text", nil
	})
	svc := NewLLMVisionOCR(client, "vision-model", "Read the text")
	png := []byte{0x89, 'P', 'N', 'G'}
	res, err := svc.ExtractText(OCRImage{Data: png, MIME: "image/png", Name: "red.png"})
	if err != nil || res.Text != "Recognized_text" {
		t.Fatal(res, err)
	}
	if got.Model != "vision-model" || got.Prompt != "Read the text" || !strings.HasPrefix(got.DataURI, "data:image/png;base64,") {
		t.Fatalf("%+v", got)
	}
	doc := docxPackage(t, `<w:p><w:r><w:t>Before </w:t></w:r>`+onePNG()+`<w:r><w:t> After</w:t></w:r></w:p>`, map[string][]byte{"word/media/image.png": png},
		`  <Relationship Id="rIdPng" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image.png"/>`)
	out, err := ExtractOptions(bytes.NewReader(doc), "", "a.docx", 0, Options{OCR: svc})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Text, "*[Image OCR]  \nRecognized\\_text  \n[End OCR]*") {
		t.Fatalf("%s", out.Text)
	}
}

func TestOCRErrorKeepsNativeImage(t *testing.T) {
	name := "docx_image_start.docx"
	body := ocrFile(t, name)
	native, err := Extract(bytes.NewReader(body), "", name, 0)
	if err != nil {
		t.Fatal(err)
	}
	res, err := ExtractOptions(bytes.NewReader(body), "", name, 0, Options{OCR: errOCR{msg: "quota exceeded"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != native.Text || strings.Contains(res.Text, "*[Image OCR]") {
		t.Fatalf("native not kept\n%s", res.Text)
	}
	_, err = ExtractOptions(bytes.NewReader(body), "", name, 0, Options{OCR: panicOCR{err: errors.New("vision down")}})
	if err == nil || err.Error() != "vision down" {
		t.Fatal(err)
	}
}

func TestOCRCacheReusesIdenticalImages(t *testing.T) {
	var n int
	svc := countOCR{text: ocrMock, n: &n}
	_, err := ExtractOptions(bytes.NewReader(ocrFile(t, "docx_multiple_images.docx")), "", "a.docx", 0, Options{OCR: svc})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 && n != 1 {
		t.Fatalf("calls %d", n)
	}
}

type visionFunc func(VisionRequest) (string, error)

func (f visionFunc) Complete(req VisionRequest) (string, error) { return f(req) }

type errOCR struct{ msg string }

func (e errOCR) ExtractText(OCRImage) (OCRResult, error) {
	return OCRResult{Error: e.msg}, nil
}

type panicOCR struct{ err error }

func (p panicOCR) ExtractText(OCRImage) (OCRResult, error) { return OCRResult{}, p.err }

type countOCR struct {
	text string
	n    *int
}

func (c countOCR) ExtractText(OCRImage) (OCRResult, error) {
	*c.n++
	return OCRResult{Text: c.text}, nil
}

func ocrFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("packages/markitdown-ocr/tests/ocr_test_data/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func onePNG() string {
	return `<w:r><w:drawing><wp:inline><wp:docPr id="1" name="Image 1" descr="Image 1"/>` +
		`<a:graphic><a:graphicData><pic:pic><pic:nvPicPr><pic:cNvPr id="1" name="Image 1"/></pic:nvPicPr>` +
		`<pic:blipFill><a:blip r:embed="rIdPng"/></pic:blipFill></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`
}
