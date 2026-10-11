package markitdown

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"regexp"
	"strings"
	"testing"
	"unicode/utf16"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
)

func TestXLSVector(t *testing.T) {
	res, err := Extract(bytes.NewReader(testFile(t, "test.xls")), "application/vnd.ms-excel", "test.xls", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindXLS {
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
	res, err := Extract(bytes.NewReader(testFile(t, "test_with_comment.docx")), "", "test_with_comment.docx", 0)
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
		"This is a test comment. 12df-321a",
		"Yet another comment in the doc. 55yiyi-asd09",
	} {
		if !strings.Contains(strings.ReplaceAll(res.Text, `\`, ""), s) {
			t.Fatalf("missing %s", s)
		}
	}
}

func TestDOCXEquations(t *testing.T) {
	res, err := Extract(bytes.NewReader(testFile(t, "equations.docx")), "", "equations.docx", 0)
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

func TestOMMLFunctions(t *testing.T) {
	log := mathFrom(`<m:oMath xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math"><m:func><m:fName><m:r><m:t>log</m:t></m:r></m:fName><m:e><m:r><m:t>x</m:t></m:r></m:e></m:func></m:oMath>`)
	if !strings.Contains(log, `\log`) {
		t.Fatal(log)
	}
	custom := mathFrom(`<m:oMath xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math"><m:func><m:fName><m:r><m:t>custom</m:t></m:r><m:r><m:t>Func</m:t></m:r></m:fName><m:e><m:r><m:t>x</m:t></m:r></m:e></m:func></m:oMath>`)
	if custom != `\operatorname{customFunc}(x)` {
		t.Fatal(custom)
	}
	sin := mathFrom(`<m:oMath xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math"><m:func><m:fName><m:r><m:t>si</m:t></m:r><m:r><m:t>n</m:t></m:r></m:fName><m:e><m:r><m:t>x</m:t></m:r></m:e></m:func></m:oMath>`)
	if sin != `\sin(x)` {
		t.Fatal(sin)
	}
}

func TestOMMLAccentsAndItalic(t *testing.T) {
	doc := func(inner string) string {
		return mathFrom(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math">` + inner + `</w:document>`)
	}
	acc := func(chr string) string {
		return doc(`<m:oMath><m:acc><m:accPr><m:chr m:val="` + chr + `"/></m:accPr><m:e><m:r><m:t>x</m:t></m:r></m:e></m:acc></m:oMath>`)
	}
	if acc("\u030c") != `\check{x}` || acc("\u030a") != `\ocirc{x}` || acc("\u0302") != `\hat{x}` {
		t.Fatalf("caron %q ring %q hat %q", acc("\u030c"), acc("\u030a"), acc("\u0302"))
	}
	if acc("\u20d6") != `\overleftarrow{x}` || acc("\u20ef") != `\underrightarrow{x}` || acc("\u20ee") != `\underleftarrow{x}` {
		t.Fatalf("arrows %q %q %q", acc("\u20d6"), acc("\u20ef"), acc("\u20ee"))
	}
	run := func(text string) string {
		return doc(`<m:oMath><m:r><m:t>` + text + `</m:t></m:r></m:oMath>`)
	}
	for i, c := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		got := run(string(rune(0x1D434 + i)))
		if got != string(c) {
			t.Fatalf("upper %c got %q", c, got)
		}
	}
	for i, c := range "abcdefghijklmnopqrstuvwxyz" {
		cp := rune(0x1D44E + i)
		if cp == 0x1D455 {
			continue
		}
		if run(string(cp)) != string(c) {
			t.Fatalf("lower %c got %q", c, run(string(cp)))
		}
	}
	if run("ℎ") != "h" {
		t.Fatal(run("ℎ"))
	}
	expr := "ℎ(" + string(rune(0x1D465)) + ")=" + string(rune(0x1D454)) + "(" + string(rune(0x1D465)) + ")"
	if run(expr) != "h(x)=g(x)" {
		t.Fatal(run(expr))
	}
}

func TestOMMLEmptyRunAndSubscript(t *testing.T) {
	empty := mathFrom(`<m:oMath xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math"><m:r><m:rPr/></m:r></m:oMath>`)
	if empty != "" {
		t.Fatal(empty)
	}
	if mathFrom(`<m:oMath xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math"><m:r><m:t>x</m:t></m:r></m:oMath>`) != "x" {
		t.Fatal("x")
	}
	sub := mathFrom(`<m:oMath xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math"><m:sSub><m:e><m:r><m:t>l</m:t></m:r></m:e><m:sub><m:r><m:rPr/></m:r><m:r><m:t>1</m:t></m:r></m:sub></m:sSub></m:oMath>`)
	if !strings.Contains(sub, "l") || !strings.Contains(sub, "1") {
		t.Fatal(sub)
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
	res, err := Extract(bytes.NewReader(zipDoc(t, "word/document.xml", xml)), "", "a.docx", 0)
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
	res, err := Extract(bytes.NewReader(zipDoc(t, "word/document.xml", xml)), "", "a.docx", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"Before", "Inside text box 9f3c", "Inserted revision", "Deleted revision"} {
		if !strings.Contains(res.Text, s) {
			t.Fatalf("missing %s\n%s", s, res.Text)
		}
	}
}

func TestMSGFixture(t *testing.T) {
	res, err := Extract(bytes.NewReader(testFile(t, "test_outlook_msg.msg")), "", "test_outlook_msg.msg", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"# Email Message",
		"**From:** test.sender@example.com",
		"**To:** test.recipient@example.com",
		"**Subject:** Test Email Message",
		"## Content",
		"This is the body of the test email message",
	} {
		if !strings.Contains(res.Text, s) {
			t.Fatalf("missing %s\n%s", s, res.Text)
		}
	}
}

func TestMSGCodepages(t *testing.T) {
	const (
		sender    = "ana.lopez@example.com"
		recipient = "carlos.ruiz@example.com"
		subject   = "Confirmación de la reunión del martes"
		body      = "Hola Carlos,\r\n\r\nTe confirmo la reunión del martes a las diez en la oficina de Bilbao. He adjuntado el informe de facturación del último trimestre para que puedas revisarlo antes, junto con la propuesta de calendario que comentamos por teléfono la semana pasada.\r\n\r\nUn saludo,\r\nAna"
	)
	ansi := func(enc string, cp int) map[string][]byte {
		e := charmap.Windows1252.NewEncoder()
		encBytes := func(s string) []byte {
			b, err := e.Bytes([]byte(s))
			if err != nil {
				t.Fatal(err)
			}
			return b
		}
		if enc != "cp1252" {
			t.Fatalf("unsupported helper enc %s", enc)
		}
		streams := map[string][]byte{
			"__substg1.0_0C1F001E": encBytes(sender),
			"__substg1.0_0E04001E": encBytes(recipient),
			"__substg1.0_0037001E": encBytes(subject),
			"__substg1.0_1000001E": encBytes(body),
		}
		if cp != 0 {
			streams["__properties_version1.0"] = propStream(map[int]int{0x3FFD: cp})
		}
		return streams
	}
	want := "# Email Message\n\n**From:** " + sender + "\n**To:** " + recipient + "\n**Subject:** " + subject + "\n\n## Content\n\n" + body
	for _, cp := range []int{1252, 0, 99999, 20127} {
		streams := ansi("cp1252", cp)
		if cp == 20127 {
			streams["__properties_version1.0"] = propStream(map[int]int{0x3FFD: 20127})
		}
		for path, b := range streams {
			if strings.HasSuffix(path, "001E") {
				streams[path] = append(append([]byte(" \t"), b...), []byte("\r\n \x00")...)
			}
		}
		if got := renderMSG(streams); got != want {
			t.Fatalf("cp %d\n%s", cp, got)
		}
	}

	empty := ansi("cp1252", 1252)
	for path := range empty {
		if strings.HasSuffix(path, "001E") {
			empty[path] = []byte{0}
		}
	}
	if got := renderMSG(empty); got != "# Email Message\n\n\n## Content" {
		t.Fatalf("empty %q", got)
	}

	keep := ansi("cp1252", 1252)
	if !strings.Contains(renderMSG(keep), "Te confirmo la reunión del martes") || !strings.Contains(renderMSG(keep), "**Subject:**") {
		t.Fatal(renderMSG(keep))
	}

	cyr := map[string][]byte{"__substg1.0_0037001E": mustEncode(t, charmap.Windows1251, "Привет мир"), "__properties_version1.0": propStream(map[int]int{0x3FFD: 1251})}
	if !strings.Contains(renderMSG(cyr), "**Subject:** Привет мир") {
		t.Fatal(renderMSG(cyr))
	}
	lat := map[string][]byte{"__substg1.0_0037001E": mustEncode(t, charmap.Windows1252, "Résumé"), "__properties_version1.0": propStream(map[int]int{0x3FFD: 1252})}
	if !strings.Contains(renderMSG(lat), "**Subject:** Résumé") {
		t.Fatal(renderMSG(lat))
	}
	greek := map[string][]byte{"__substg1.0_0037001E": mustEncode(t, charmap.Windows1253, "Ελληνικά"), "__properties_version1.0": propStream(map[int]int{0x3FFD: 1253})}
	if !strings.Contains(renderMSG(greek), "**Subject:** Ελληνικά") {
		t.Fatal(renderMSG(greek))
	}
	iso := map[string][]byte{"__substg1.0_0037001E": mustEncode(t, charmap.ISO8859_5, "Привет мир"), "__properties_version1.0": propStream(map[int]int{0x3FFD: 28595})}
	if !strings.Contains(renderMSG(iso), "**Subject:** Привет мир") {
		t.Fatal(renderMSG(iso))
	}
	u8 := map[string][]byte{"__substg1.0_0037001E": []byte(subject), "__properties_version1.0": propStream(map[int]int{0x3FFD: 65001})}
	if !strings.Contains(renderMSG(u8), "**Subject:** "+subject) {
		t.Fatal(renderMSG(u8))
	}
	both := map[string][]byte{
		"__substg1.0_0037001E":    mustEncode(t, charmap.Windows1252, "Résumé"),
		"__substg1.0_1000001E":    mustEncode(t, charmap.Windows1251, "Привет мир"),
		"__properties_version1.0": propStream(map[int]int{0x3FFD: 1252, 0x3FDE: 1251}),
	}
	got := renderMSG(both)
	if !strings.Contains(got, "**Subject:** Résumé") || !strings.Contains(got, "Привет мир") {
		t.Fatal(got)
	}
	uni := map[string][]byte{
		"__substg1.0_0C1F001F": utf16le(sender),
		"__substg1.0_0E04001F": utf16le(recipient),
		"__substg1.0_0037001F": utf16le(subject),
		"__substg1.0_1000001F": utf16le(body),
	}
	if renderMSG(uni) != want {
		t.Fatalf("unicode\n%s", renderMSG(uni))
	}
}

func TestMSGJapaneseCodepages(t *testing.T) {
	cases := []struct {
		cp   int
		in   []byte
		want string
	}{
		{50220, []byte("\x1b$BF|K\\8l\x1b(B"), "日本語"},
		{50221, []byte("\x1b(I6@6E\x1b(B"), "ｶﾀｶﾅ"},
		{50221, []byte("\x1b$BF|K\\\x1b(I6@6E\x1b(B ABC"), "日本ｶﾀｶﾅ ABC"},
		{50222, []byte("Plain ASCII"), "Plain ASCII"},
		{50222, []byte("\x0e6@6E\x0f"), "ｶﾀｶﾅ"},
		{50222, []byte("ABC \x0e6@6E\x0f XYZ"), "ABC ｶﾀｶﾅ XYZ"},
		{50222, []byte("\x1b$BF|\x0e6@6E\x0fK\\\x1b(B"), "日ｶﾀｶﾅ本"},
		{50222, []byte("\x1b(J\\\x0e6@6E\x0f~\x1b(B"), "¥ｶﾀｶﾅ‾"},
		{50222, []byte("\x0e6@\x0f/\x0e6E\x0f"), "ｶﾀ/ｶﾅ"},
		{50222, []byte("\x0e6@6E"), "ｶﾀｶﾅ"},
	}
	for _, tc := range cases {
		streams := map[string][]byte{"__properties_version1.0": propStream(map[int]int{0x3FFD: tc.cp})}
		for _, tag := range []string{"0C1F", "0E04", "0037", "1000"} {
			streams["__substg1.0_"+tag+"001E"] = append(append([]byte{}, tc.in...), 0)
		}
		got := renderMSG(streams)
		want := "# Email Message\n\n**From:** " + tc.want + "\n**To:** " + tc.want + "\n**Subject:** " + tc.want + "\n\n## Content\n\n" + tc.want
		if got != want {
			t.Fatalf("cp %d\n got %q\nwant %q", tc.cp, got, want)
		}
	}
	resume := mustEncode(t, charmap.Windows1252, "Résumé")
	for _, tc := range []struct {
		cp   int
		body []byte
	}{{50221, []byte("\x1b(I6@6E\x1b(B")}, {50222, []byte("\x0e6@6E\x0f")}} {
		streams := map[string][]byte{
			"__substg1.0_0037001E":    append(append([]byte{}, resume...), 0),
			"__substg1.0_1000001E":    append(append([]byte{}, tc.body...), 0),
			"__properties_version1.0": propStream(map[int]int{0x3FFD: 1252, 0x3FDE: tc.cp}),
		}
		want := "# Email Message\n\n**Subject:** Résumé\n\n## Content\n\nｶﾀｶﾅ"
		if renderMSG(streams) != want {
			t.Fatalf("%d %q", tc.cp, renderMSG(streams))
		}
	}
}

func TestMSGInvalidJapaneseDetectionStub(t *testing.T) {
	// Original assertion: a stubbed charset detector returns "Recovered subject"
	// and is called with the raw bytes. Pure Go has no detector hook, so this
	// stays failed instead of being rewritten.
	value := []byte{0x1b, '(', 'I', '~', 0x1b, '(', 'B'}
	streams := map[string][]byte{
		"__substg1.0_0037001E":    append(append([]byte{}, value...), 0),
		"__properties_version1.0": propStream(map[int]int{0x3FFD: 50221}),
	}
	got := renderMSG(streams)
	if !strings.Contains(got, "**Subject:** Recovered subject") {
		t.Errorf("original assertion title == %q; got %q", "Recovered subject", got)
	}
}

func TestPDFVectorsAndMasterformat(t *testing.T) {
	check := func(name string, include, exclude []string) {
		t.Helper()
		res, err := Extract(bytes.NewReader(testFile(t, name)), "application/pdf", name, 0)
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

	res, err := Extract(bytes.NewReader(testFile(t, "masterformat_partial_numbering.pdf")), "", "masterformat_partial_numbering.pdf", 0)
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

	scan, err := Extract(bytes.NewReader(testFile(t, "MEDRPT-2024-PAT-3847_medical_report_scan.pdf")), "", "scan.pdf", 0)
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
		res, err := Extract(bytes.NewReader(testFile(t, name+".pdf")), "", name+".pdf", 0)
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile("packages/markitdown/tests/test_files/expected_outputs/" + name + ".md")
		if err != nil {
			t.Fatal(err)
		}
		if res.Text != string(want) {
			t.Errorf("%s output != expected markdown (%d vs %d bytes)", name, len(res.Text), len(want))
		}
	}
}

func mathFrom(xml string) string {
	root, err := parseXML([]byte(xml))
	if err != nil {
		return "ERR " + err.Error()
	}
	return mathLatex(root)
}

func propStream(codepages map[int]int) []byte {
	data := make([]byte, 32)
	for id, cp := range codepages {
		tag := uint32(id<<16) | 0x0003
		entry := make([]byte, 16)
		binary.LittleEndian.PutUint32(entry[0:4], tag)
		binary.LittleEndian.PutUint32(entry[4:8], 0x00000006)
		binary.LittleEndian.PutUint32(entry[8:12], uint32(cp))
		data = append(data, entry...)
	}
	return data
}

func mustEncode(t *testing.T, enc encoding.Encoding, s string) []byte {
	t.Helper()
	b, err := enc.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return b
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

func utf16le(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, len(u)*2)
	for i, v := range u {
		binary.LittleEndian.PutUint16(b[i*2:], v)
	}
	return b
}
