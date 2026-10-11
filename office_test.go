package markitdown

import (
	"archive/zip"
	"bytes"
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
	res, err := Extract(bytes.NewReader(buf.Bytes()), "", "a.xlsx", 0)
	if err != nil {
		t.Fatal(err)
	}
	want := "## Second\n| Header\\_one | Unnamed: 1 |\n| --- | --- |\n| <native> & \\*value\\* | 12 |\n\n## First\n| Other |\n| --- |\n| End |"
	if res.Text != want {
		t.Fatalf("got:\n%s", res.Text)
	}
}

func TestShowZeroesRenameIsScoped(t *testing.T) {
	in := []byte("<worksheet><sheetViews>" +
		`<sheetView showZeroes="0" workbookViewId="0"/>` +
		"<sheetView\n\tshowZeroes=\"1\"\ttabSelected=\"1\"/>" +
		"</sheetViews>" +
		`<customSheetViews><customSheetView showZeroes="0"/></customSheetViews>` +
		`<sheetData><row r="1">` +
		`<c r="A1" t="inlineStr"><is><t xml:space="preserve"> showZeroes="0" </t></is></c>` +
		`<c r="B1"><f>IF(A1=" showZeroes=","x","y")</f><v>y</v></c>` +
		`</row></sheetData></worksheet>`)
	got := renameShowZeroes(in)
	if bytes.Count(got, []byte("showZeros=")) != 2 {
		t.Fatalf("count %d\n%s", bytes.Count(got, []byte("showZeros=")), got)
	}
	for _, s := range []string{
		`<sheetView showZeros="0" workbookViewId="0"/>`,
		"<sheetView\n\tshowZeros=\"1\"\ttabSelected=\"1\"/>",
		`<t xml:space="preserve"> showZeroes="0" </t>`,
		`<f>IF(A1=" showZeroes=","x","y")</f>`,
		`<customSheetView showZeroes="0"/>`,
	} {
		if !bytes.Contains(got, []byte(s)) {
			t.Fatalf("missing %s\n%s", s, got)
		}
	}
}

func TestXLSXLegacyShowZeroesCellSurvives(t *testing.T) {
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "Data")
	f.SetCellValue("Data", "A1", "hello")
	f.SetCellValue("Data", "B1", "world")
	f.SetCellValue("Data", "A2", ` showZeroes="0" `)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	raw := rewriteZip(t, buf.Bytes(), func(name string, content []byte) []byte {
		if name == "xl/worksheets/sheet1.xml" {
			return bytes.Replace(content, []byte("<sheetView "), []byte(`<sheetView showZeroes="0" `), 1)
		}
		return content
	})
	res, err := Extract(bytes.NewReader(raw), "", "legacy.xlsx", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"## Data", "hello", "world", `showZeroes="0"`} {
		if !strings.Contains(res.Text, s) {
			t.Fatalf("missing %s\n%s", s, res.Text)
		}
	}
	if strings.Contains(res.Text, "showZeros") {
		t.Fatalf("renamed cell text:\n%s", res.Text)
	}
}

func TestXLSXImageHookOrderAndNative(t *testing.T) {
	book := xlsxWithImages(t)
	native, err := Extract(bytes.NewReader(book), "", "a.xlsx", 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(native.Text, "### Images") || strings.Contains(native.Text, "image_") {
		t.Fatalf("native parsed images:\n%s", native.Text)
	}
	var seen []string
	hook := func(img EmbeddedImage) (string, error) {
		seen = append(seen, img.Name+":"+string(img.Data))
		return "<p><strong>image_" + itoa(len(seen)) + "</strong></p>", nil
	}
	res, err := ExtractOptions(bytes.NewReader(book), "", "a.xlsx", 0, Options{ImageHook: hook})
	if err != nil {
		t.Fatal(err)
	}
	wantSeen := []string{"image3.png:red", "image1.png:red", "image2.png:blue", "image4.png:blue"}
	if strings.Join(seen, ",") != strings.Join(wantSeen, ",") {
		t.Fatalf("order %v", seen)
	}
	order := []string{"## Second", "| <native> & \\*value\\* | 12 |", "### Images in this sheet:", "**image_1**", "**image_2**", "**image_3**", "## First", "| End |", "**image_4**"}
	pos := -1
	for _, s := range order {
		i := strings.Index(res.Text, s)
		if i < 0 || i < pos {
			t.Fatalf("missing or out of order %s\n%s", s, res.Text)
		}
		pos = i
	}
	blank, err := ExtractOptions(bytes.NewReader(book), "", "a.xlsx", 0, Options{ImageHook: func(EmbeddedImage) (string, error) {
		return "  \n", nil
	}})
	if err != nil || blank.Text != native.Text {
		t.Fatalf("declining hook\n%q\n%q", blank.Text, native.Text)
	}
}

func TestXLSXImageHookRejectsDocumentAndMissing(t *testing.T) {
	book := xlsxWithImages(t)
	_, err := ExtractOptions(bytes.NewReader(book), "", "a.xlsx", 0, Options{ImageHook: func(EmbeddedImage) (string, error) {
		return "<html><body>text</body></html>", nil
	}})
	if err == nil || !strings.Contains(err.Error(), "_image_to_html must return") {
		t.Fatal(err)
	}
	broken := rewriteZip(t, book, func(name string, content []byte) []byte {
		if name == "xl/drawings/drawing1.xml" {
			return bytes.Replace(content, []byte(`embed="rId1"`), []byte(`embed="missing"`), 1)
		}
		return content
	})
	_, err = ExtractOptions(bytes.NewReader(broken), "", "a.xlsx", 0, Options{ImageHook: func(EmbeddedImage) (string, error) {
		return "<p>recognized</p>", nil
	}})
	if err == nil {
		t.Fatal("missing relationship was ignored")
	}
	entity := rewriteZip(t, book, func(name string, content []byte) []byte {
		if name != "xl/drawings/drawing1.xml" {
			return content
		}
		return append([]byte(`<!DOCTYPE wsDr [<!ENTITY label "not expanded">]>`), bytes.Replace(content, []byte(`name="Image 1"`), []byte(`name="&label;"`), 1)...)
	})
	called := false
	_, err = ExtractOptions(bytes.NewReader(entity), "", "a.xlsx", 0, Options{ImageHook: func(EmbeddedImage) (string, error) {
		called = true
		return "<p>recognized</p>", nil
	}})
	if err == nil || called {
		t.Fatalf("entity err=%v called=%v", err, called)
	}
}

func TestDOCXUnderlineAndStyleMap(t *testing.T) {
	plain := docxWithBody(t, `<w:r><w:t xml:space="preserve">plain </w:t></w:r><w:r><w:rPr><w:u w:val="single"/></w:rPr><w:t>underlined</w:t></w:r>`)
	res, err := Extract(bytes.NewReader(plain), "", "a.docx", 0)
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
		res, err = Extract(bytes.NewReader(docxWithBody(t, body)), "", "a.docx", 0)
		if err != nil || res.Text != tc.want {
			t.Fatalf("%s got %q err %v", tc.run, res.Text, err)
		}
	}
	embedded := docxWithBody(t, `<w:r><w:t xml:space="preserve">plain </w:t></w:r><w:r><w:rPr><w:u w:val="single"/></w:rPr><w:t>underlined</w:t></w:r>`)
	embedded = addZipFile(t, embedded, "mammoth/style-map", []byte("u => em"))
	res, err = Extract(bytes.NewReader(embedded), "", "a.docx", 0)
	if err != nil || !strings.Contains(res.Text, "plain *underlined*") || strings.Contains(res.Text, "<u>") {
		t.Fatalf("%s %v", res.Text, err)
	}
	res, err = ExtractOptions(bytes.NewReader(embedded), "", "a.docx", 0, Options{StyleMap: "u => strong"})
	if err != nil || !strings.Contains(res.Text, "plain **underlined**") {
		t.Fatalf("%s %v", res.Text, err)
	}
}

func TestDOCXStylesRepair(t *testing.T) {
	valid := []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:style w:type="paragraph" w:styleId="Normal"><w:name w:val="Normal"/></w:style></w:styles>`)
	if !bytes.Equal(repairStyles(valid), valid) {
		t.Fatal("valid styles changed")
	}
	missing := bytes.Replace(valid, []byte(` w:type="paragraph"`), nil, 1)
	got := repairStyles(missing)
	root, err := parseXML(got)
	if err != nil {
		t.Fatal(err)
	}
	style := findFirst(root, "style")
	if style == nil || style.attr["type"] != "paragraph" || style.attr["styleId"] != "Normal" {
		t.Fatalf("%v", style)
	}
	noID := bytes.Replace(valid, []byte(` w:styleId="Normal"`), nil, 1)
	root, err = parseXML(repairStyles(noID))
	if err != nil || findFirst(root, "style") != nil {
		t.Fatal("style without id kept", err)
	}
	strike := []byte(`<?xml version="1.0"?><w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:style w:styleId="Normal"><w:rPr><w:dstrike w:val="1"/><w:b/></w:rPr></w:style></w:styles>`)
	text := string(repairStyles(strike))
	if !strings.Contains(text, `w:type="paragraph"`) || !strings.Contains(text, "<w:strike") || strings.Contains(text, "dstrike") {
		t.Fatalf("%s", text)
	}
	other := []byte(`<?xml version="1.0"?><w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:e="urn:extension"><w:style w:type="paragraph" w:styleId="Normal"/><e:dstrike/></w:styles>`)
	if !bytes.Equal(repairStyles(other), other) {
		t.Fatalf("other namespace changed: %s", repairStyles(other))
	}
	orig, err := Extract(bytes.NewReader(testFile(t, "test.docx")), "", "test.docx", 0)
	if err != nil {
		t.Fatal(err)
	}
	mod := rewriteZip(t, testFile(t, "test.docx"), func(name string, content []byte) []byte {
		if name != "word/styles.xml" {
			return content
		}
		return bytes.Replace(content, []byte(`<w:style w:type="paragraph" w:styleId="1"`), []byte(`<w:style w:styleId="1"`), 1)
	})
	alt, err := Extract(bytes.NewReader(mod), "", "test.docx", 0)
	if err != nil {
		t.Fatal(err)
	}
	if orig.Text != alt.Text || !strings.Contains(alt.Text, "# Abstract") {
		t.Fatal("style repair changed the document")
	}
}

func TestDOCXImageHook(t *testing.T) {
	png := []byte("png-bytes")
	gif := []byte("gif-bytes")
	body := `<w:r><w:drawing><wp:inline><wp:docPr id="1" name="Image 1" descr="Image 1"/>` +
		`<a:graphic><a:graphicData><pic:pic><pic:nvPicPr><pic:cNvPr id="1" name="Image 1"/></pic:nvPicPr>` +
		`<pic:blipFill><a:blip r:embed="rIdPng"/></pic:blipFill></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`
	doc := docxPackage(t, `<w:p>`+body+`</w:p>`, map[string][]byte{
		"word/media/image.png": png,
	}, `  <Relationship Id="rIdPng" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image.png"/>`)
	native, err := Extract(bytes.NewReader(doc), "", "a.docx", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(native.Text, "![Image 1](data:image/png;base64...)") {
		t.Fatalf("%s", native.Text)
	}
	var seen []string
	hook := func(img EmbeddedImage) (string, error) {
		seen = append(seen, img.Ext+":"+img.MIME+":"+string(img.Data))
		return "<strong>image" + itoa(len(seen)) + "</strong>", nil
	}
	two := docxPackage(t, `<w:p>`+
		strings.Replace(body, `rIdPng`, `rIdGif`, 1)+
		`<w:r><w:t xml:space="preserve"> </w:t></w:r>`+
		body+
		`<w:r><w:t xml:space="preserve"> </w:t></w:r>`+
		strings.Replace(body, `rIdPng`, `rIdGif`, 1)+
		`</w:p>`, map[string][]byte{"word/media/image.png": png, "word/media/image.gif": gif},
		`  <Relationship Id="rIdPng" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image.png"/>`+
			`  <Relationship Id="rIdGif" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image.gif"/>`)
	// The first and third drawings still say rIdPng in the template before replace of only the first occurrence of the whole body.
	res, err := ExtractOptions(bytes.NewReader(two), "", "a.docx", 0, Options{ImageHook: hook})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "**image1** **image2** **image3**" {
		t.Fatalf("%q seen %v", res.Text, seen)
	}
	decl, err := ExtractOptions(bytes.NewReader(doc), "", "a.docx", 0, Options{ImageHook: func(EmbeddedImage) (string, error) { return "", nil }})
	if err != nil || decl.Text != native.Text {
		t.Fatalf("decline %q native %q %v", decl.Text, native.Text, err)
	}
	boom := errImage("image failure")
	_, err = ExtractOptions(bytes.NewReader(doc), "", "a.docx", 0, Options{ImageHook: func(EmbeddedImage) (string, error) { return "", boom }})
	if err != boom {
		t.Fatal(err)
	}
}

type errImage string

func (e errImage) Error() string { return string(e) }

func TestPPTXSVGAndChart(t *testing.T) {
	res, err := Extract(bytes.NewReader(testFile(t, "test_svg_no_fallback.pptx")), "", "test_svg_no_fallback.pptx", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "Red square SVG") {
		t.Fatalf("%s", res.Text)
	}
	kept, err := ExtractOptions(bytes.NewReader(testFile(t, "test_svg_no_fallback.pptx")), "", "a.pptx", 0, Options{KeepData: true})
	if err != nil || !strings.Contains(kept.Text, "data:image/svg+xml;base64,") {
		t.Fatalf("%s %v", kept.Text, err)
	}
	chart := pptxWithChart(t)
	res, err = Extract(bytes.NewReader(chart), "", "chart.pptx", 0)
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
	res, err = Extract(bytes.NewReader(plain), "", "chart.pptx", 0)
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

func TestDOCXUnreferencedMediaSkipsHook(t *testing.T) {
	doc := docxPackage(t, `<w:p><w:r><w:t>Hello</w:t></w:r></w:p>`, map[string][]byte{"word/media/orphan.png": []byte("png")}, "")
	called := false
	res, err := ExtractOptions(bytes.NewReader(doc), "", "a.docx", 0, Options{ImageHook: func(EmbeddedImage) (string, error) {
		called = true
		return "<p>nope</p>", nil
	}})
	if err != nil || called || !strings.Contains(res.Text, "Hello") {
		t.Fatalf("%v %v %s", err, called, res.Text)
	}
}

type zipExtra struct {
	name string
	body []byte
}

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
