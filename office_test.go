package markitdown_test

import (
	"archive/zip"
	"bytes"
	"context"
	"github.com/godwit-email/markitdown-go"
	"github.com/godwit-email/markitdown-go/docx"
	"github.com/godwit-email/markitdown-go/pptx"
	"io"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestXLSXNativeMarkdown(t *testing.T) {
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "Second")
	f.SetCellValue("Second", "A1", "Header_one")
	f.SetCellValue("Second", "A2", "<native> & *value*")
	f.SetCellValue("Second", "B2", 12)
	f.NewSheet("First")
	f.SetCellValue("First", "A1", "Other")
	f.SetCellValue("First", "A2", "End")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(buf.Bytes()), MIME: "", Filename: "a.xlsx", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	want := "## Second\n| Header\\_one | Unnamed: 1 |\n| --- | --- |\n| <native> & \\*value\\* | 12 |\n\n## First\n| Other |\n| --- |\n| End |"
	if res.Text != want {
		t.Fatalf("got:\n%s", res.Text)
	}
}

func TestDOCXUnderlineAndStyleMap(t *testing.T) {
	plain := docxWithBody(t, `<w:r><w:t xml:space="preserve">plain </w:t></w:r><w:r><w:rPr><w:u w:val="single"/></w:rPr><w:t>underlined</w:t></w:r>`)
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(plain), MIME: "", Filename: "a.docx", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "plain <u>underlined</u>") {
		t.Fatalf("%s", res.Text)
	}
	cases := []struct{ run, want string }{
		{`<w:t xml:space="preserve"> </w:t>`, "First Last"},
		{`<w:tab/>`, "First Last"},
		{`<w:t>&#160;</w:t>`, "First\u00a0Last"},
		{`<w:br/>`, "First\nLast"},
	}
	for _, tc := range cases {
		body := `<w:r><w:t>First</w:t></w:r><w:r><w:rPr><w:u w:val="single"/></w:rPr>` + tc.run + `</w:r><w:r><w:t>Last</w:t></w:r>`
		res, err = markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(docxWithBody(t, body)), MIME: "", Filename: "a.docx", MaxOut: 0})
		if err != nil || res.Text != tc.want {
			t.Fatalf("%s got %q err %v", tc.run, res.Text, err)
		}
	}
	embedded := docxWithBody(t, `<w:r><w:t xml:space="preserve">plain </w:t></w:r><w:r><w:rPr><w:u w:val="single"/></w:rPr><w:t>underlined</w:t></w:r>`)
	embedded = addZipFile(t, embedded, "mammoth/style-map", []byte("u => em"))
	res, err = markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(embedded), MIME: "", Filename: "a.docx", MaxOut: 0})
	if err != nil || !strings.Contains(res.Text, "plain *underlined*") || strings.Contains(res.Text, "<u>") {
		t.Fatalf("%s %v", res.Text, err)
	}
	res, err = docx.ExtractOptions(context.Background(), markitdown.Request{R: bytes.NewReader(embedded), Filename: "a.docx"}, docx.Options{StyleMap: "u => strong"})
	if err != nil || !strings.Contains(res.Text, "plain **underlined**") {
		t.Fatalf("%s %v", res.Text, err)
	}
}

type errImage string

func (e errImage) Error() string { return string(e) }

type zipExtra struct {
	name string
	body []byte
}

func TestPPTXSVGAndChart(t *testing.T) {
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, "test_svg_no_fallback.pptx")), MIME: "", Filename: "test_svg_no_fallback.pptx", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "Red square SVG") {
		t.Fatalf("%s", res.Text)
	}
	kept, err := pptx.ExtractOptions(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, "test_svg_no_fallback.pptx")), Filename: "a.pptx"}, pptx.Options{KeepData: true})
	if err != nil || !strings.Contains(kept.Text, "data:image/svg+xml;base64,") {
		t.Fatalf("%s %v", kept.Text, err)
	}
	chart := pptxWithChart(t)
	res, err = markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(chart), MIME: "", Filename: "chart.pptx", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"### Chart: Revenue", "Series A", "Series B", "| C0 |", "| C199 |", "| C10 | 10.0 | 20.0 |"} {
		if !strings.Contains(res.Text, s) {
			t.Fatalf("missing %s\n%s", s, res.Text)
		}
	}
	plain := rewriteZip(t, chart, func(name string, content []byte) []byte {
		if name == "ppt/charts/chart1.xml" {
			return bytes.Replace(content, []byte("<c:title><c:tx><c:rich><a:p><a:r><a:t>Revenue</a:t></a:r></a:p></c:rich></c:tx></c:title>"), []byte("<c:title/>"), 1)
		}
		return content
	})
	res, err = markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(plain), MIME: "", Filename: "chart.pptx", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "### Chart\n") || strings.Contains(res.Text, "### Chart:") {
		t.Fatalf("%s", res.Text)
	}
}

func xlsxWithImages(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "Second")
	f.SetCellValue("Second", "A1", "Header_one")
	f.SetCellValue("Second", "A2", "<native> & *value*")
	f.SetCellValue("Second", "B2", 12)
	if _, err := f.NewSheet("First"); err != nil {
		t.Fatal(err)
	}
	f.SetCellValue("First", "A1", "Other")
	f.SetCellValue("First", "A2", "End")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return injectXLSXImages(t, buf.Bytes())
}

func injectXLSXImages(t *testing.T, data []byte) []byte {
	t.Helper()
	drawing := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<xdr:wsDr xmlns:xdr="http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <xdr:oneCellAnchor><xdr:from><xdr:col>0</xdr:col><xdr:row>0</xdr:row></xdr:from><xdr:pic><xdr:nvPicPr><xdr:cNvPr id="1" name="Image 1"/></xdr:nvPicPr><xdr:blipFill><a:blip r:embed="rId1"/></xdr:blipFill></xdr:pic></xdr:oneCellAnchor>
  <xdr:twoCellAnchor><xdr:pic><xdr:nvPicPr><xdr:cNvPr id="2" name="Image 2"/></xdr:nvPicPr><xdr:blipFill><a:blip r:embed="rId2"/></xdr:blipFill></xdr:pic></xdr:twoCellAnchor>
  <xdr:absoluteAnchor><xdr:pic><xdr:nvPicPr><xdr:cNvPr id="3" name="Image 3"/></xdr:nvPicPr><xdr:blipFill><a:blip r:embed="rId3"/></xdr:blipFill></xdr:pic></xdr:absoluteAnchor>
</xdr:wsDr>`
	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/image1.png"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/image2.png"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/image3.png"/>
</Relationships>`
	sheetRels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rIdDraw" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/drawing" Target="../drawings/drawing1.xml"/>
</Relationships>`
	drawing2 := strings.ReplaceAll(strings.ReplaceAll(drawing, "rId1", "rId9"), "image", "image")
	drawing2 = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<xdr:wsDr xmlns:xdr="http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <xdr:oneCellAnchor><xdr:pic><xdr:nvPicPr><xdr:cNvPr id="4" name="Image 4"/></xdr:nvPicPr><xdr:blipFill><a:blip r:embed="rId1"/></xdr:blipFill></xdr:pic></xdr:oneCellAnchor>
</xdr:wsDr>`
	rels2 := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/image4.png"/>
</Relationships>`
	return rewriteZip(t, data, func(name string, content []byte) []byte {
		if name == "xl/worksheets/sheet1.xml" && !bytes.Contains(content, []byte("<drawing")) {
			content = bytes.Replace(content, []byte("</worksheet>"), []byte(`<drawing r:id="rIdDraw" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"/></worksheet>`), 1)
		}
		if name == "xl/worksheets/sheet2.xml" && !bytes.Contains(content, []byte("<drawing")) {
			content = bytes.Replace(content, []byte("</worksheet>"), []byte(`<drawing r:id="rIdDraw" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"/></worksheet>`), 1)
		}
		return content
	}, zipExtra{name: "xl/drawings/drawing1.xml", body: []byte(drawing)},
		zipExtra{name: "xl/drawings/_rels/drawing1.xml.rels", body: []byte(rels)},
		zipExtra{name: "xl/worksheets/_rels/sheet1.xml.rels", body: []byte(sheetRels)},
		zipExtra{name: "xl/drawings/drawing2.xml", body: []byte(drawing2)},
		zipExtra{name: "xl/drawings/_rels/drawing2.xml.rels", body: []byte(rels2)},
		zipExtra{name: "xl/worksheets/_rels/sheet2.xml.rels", body: bytes.Replace(sheetRelsBytes(sheetRels), []byte("drawing1.xml"), []byte("drawing2.xml"), 1)},
		zipExtra{name: "xl/media/image1.png", body: []byte("red")},
		zipExtra{name: "xl/media/image2.png", body: []byte("blue")},
		zipExtra{name: "xl/media/image3.png", body: []byte("red")},
		zipExtra{name: "xl/media/image4.png", body: []byte("blue")})
}

func sheetRelsBytes(s string) []byte { return []byte(s) }

func rewriteZip(t *testing.T, data []byte, change func(string, []byte) []byte, extra ...zipExtra) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	repl := map[string][]byte{}
	for _, ex := range extra {
		repl[ex.name] = ex.body
	}
	seen := map[string]bool{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if change != nil {
			body = change(f.Name, body)
		}
		if next, ok := repl[f.Name]; ok {
			body = next
			delete(repl, f.Name)
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
		seen[f.Name] = true
	}
	for name, body := range repl {
		if seen[name] {
			continue
		}
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func addZipFile(t *testing.T, data []byte, name string, body []byte) []byte {
	t.Helper()
	return rewriteZip(t, data, nil, zipExtra{name: name, body: body})
}

func docxWithBody(t *testing.T, inner string) []byte {
	t.Helper()
	return docxPackage(t, `<w:p>`+inner+`</w:p>`, nil, "")
}

func docxPackage(t *testing.T, body string, media map[string][]byte, rels string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Default Extension="png" ContentType="image/png"/>
  <Default Extension="gif" ContentType="image/gif"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`,
		"word/_rels/document.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + rels + `</Relationships>`,
		"word/document.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">
  <w:body>` + body + `</w:body></w:document>`,
	}
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range media {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func pptxWithChart(t *testing.T) []byte {
	t.Helper()
	var cats, a, b strings.Builder
	for i := 0; i < 200; i++ {
		cats.WriteString(`<c:pt idx="` + itoa(i) + `"><c:v>C` + itoa(i) + `</c:v></c:pt>`)
		a.WriteString(`<c:pt idx="` + itoa(i) + `"><c:v>` + itoa(i) + `</c:v></c:pt>`)
		b.WriteString(`<c:pt idx="` + itoa(i) + `"><c:v>` + itoa(i*2) + `</c:v></c:pt>`)
	}
	chart := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<c:chartSpace xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
  <c:chart>
    <c:title><c:tx><c:rich><a:p><a:r><a:t>Revenue</a:t></a:r></a:p></c:rich></c:tx></c:title>
    <c:plotArea><c:barChart>
      <c:ser><c:tx><c:v>Series A</c:v></c:tx><c:cat><c:strCache>` + cats.String() + `</c:strCache></c:cat><c:val><c:numCache>` + a.String() + `</c:numCache></c:val></c:ser>
      <c:ser><c:tx><c:v>Series B</c:v></c:tx><c:cat><c:strCache>` + cats.String() + `</c:strCache></c:cat><c:val><c:numCache>` + b.String() + `</c:numCache></c:val></c:ser>
    </c:barChart></c:plotArea>
  </c:chart>
</c:chartSpace>`
	slide := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart">
  <p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/></p:nvGrpSpPr><p:grpSpPr/>
    <p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="2" name="Chart"/><p:nvPr/></p:nvGraphicFramePr>
      <a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/chart"><c:chart r:id="rId2"/></a:graphicData></a:graphic>
    </p:graphicFrame>
  </p:spTree></p:cSld>
</p:sld>`
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml":              `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/></Types>`,
		"_rels/.rels":                      `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/></Relationships>`,
		"ppt/presentation.xml":             `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst><p:sldId id="256" r:id="rId1"/></p:sldIdLst></p:presentation>`,
		"ppt/_rels/presentation.xml.rels":  `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide1.xml"/></Relationships>`,
		"ppt/slides/slide1.xml":            slide,
		"ppt/slides/_rels/slide1.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/chart" Target="../charts/chart1.xml"/></Relationships>`,
		"ppt/charts/chart1.xml":            chart,
	}
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
