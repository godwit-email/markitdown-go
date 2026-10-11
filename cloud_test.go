package markitdown

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestDocIntelAcceptsHTML(t *testing.T) {
	d := NewDocIntel("https://example.cognitiveservices.azure.com/", nil, nil)
	d.FileTypes = []string{"html"}
	if !d.Accepts("", "a.html") {
		t.Fatal("html extension")
	}
	if !d.Accepts("text/html", "") || !d.Accepts("application/xhtml+xml", "") {
		t.Fatal("html mime")
	}
}

func TestDocIntelAPIVersion(t *testing.T) {
	d := NewDocIntel("https://example.cognitiveservices.azure.com/", nil, nil)
	if d.Config.HasAPIVersion || d.Config.APIVersion != "" {
		t.Fatalf("%+v", d.Config)
	}
	ver := "2024-07-31-preview"
	d = NewDocIntel("https://example.cognitiveservices.azure.com/", &ver, nil)
	if !d.Config.HasAPIVersion || d.Config.APIVersion != ver {
		t.Fatalf("%+v", d.Config)
	}
}

func TestDocIntelStripsCommentsAndSkipsOCRForHTML(t *testing.T) {
	var gotFeatures []string
	var gotBody []byte
	client := stubDocIntel{analyze: func(model string, body []byte, features []string, format string) (string, error) {
		if model != "prebuilt-layout" || format != "markdown" {
			t.Fatalf("%s %s", model, format)
		}
		gotFeatures = features
		gotBody = append([]byte(nil), body...)
		return "Hello <!-- secret --> world", nil
	}}
	d := NewDocIntel("https://example.cognitiveservices.azure.com/", nil, client)
	res, err := d.Convert([]byte("pdf-bytes"), "application/pdf", "a.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "Hello  world" || string(gotBody) != "pdf-bytes" {
		t.Fatalf("%q %q", res.Text, gotBody)
	}
	if len(gotFeatures) != 3 {
		t.Fatalf("pdf features %v", gotFeatures)
	}
	if features := docIntelFeatures("text/html", "a.html"); features != nil {
		t.Fatalf("html features %v", features)
	}
	if _, err := d.Convert([]byte("x"), "application/pdf", "a.pdf"); err != nil {
		// client is set; this call is the second analyze and should succeed
	}
	bare := NewDocIntel("https://example.cognitiveservices.azure.com/", nil, nil)
	if _, err := bare.Convert([]byte("x"), "application/pdf", "a.pdf"); !errors.Is(err, ErrCloudClient) {
		t.Fatal(err)
	}
}

type stubDocIntel struct {
	analyze func(model string, body []byte, features []string, format string) (string, error)
}

func (s stubDocIntel) Analyze(model string, body []byte, features []string, format string) (string, error) {
	return s.analyze(model, body, features, format)
}

func TestCUAcceptsExtensionsAndMIME(t *testing.T) {
	cu, err := NewContentUnderstanding("https://fake", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, extn := range []string{
		".pdf", ".docx", ".pptx", ".xlsx", ".html", ".txt", ".md", ".rtf", ".xml",
		".eml", ".msg", ".jpg", ".jpeg", ".jpe", ".png", ".bmp", ".tiff", ".heif", ".heic",
		".mp4", ".m4v", ".mov", ".avi", ".mkv", ".webm", ".flv", ".wmv",
		".wav", ".mp3", ".m4a", ".flac", ".ogg", ".aac", ".wma",
	} {
		if !cu.Accepts("", "a"+extn) {
			t.Fatal("accept", extn)
		}
	}
	for _, extn := range []string{".csv", ".json", ".zip", ".epub", ".py", ".rs"} {
		if cu.Accepts("", "a"+extn) {
			t.Fatal("reject", extn)
		}
	}
	for _, mime := range []string{
		"application/pdf", "image/jpeg", "video/mp4", "audio/wav", "audio/x-wav",
		"text/html", "audio/mpeg", "audio/x-m4a", "audio/x-flac", "video/quicktime",
		"video/webm", "video/x-m4v", "video/x-flv", "video/x-ms-wmv", "audio/aac", "audio/x-ms-wma",
	} {
		if !cu.Accepts(mime, "") {
			t.Fatal("mime", mime)
		}
	}
	for _, mime := range []string{"text/csv", "application/json", "application/zip"} {
		if cu.Accepts(mime, "") {
			t.Fatal("reject mime", mime)
		}
	}
	cu.FileTypes = []cuType{cuPDF}
	if !cu.Accepts("", "a.pdf") || cu.Accepts("", "a.mp4") || cu.Accepts("", "a.wav") || cu.Accepts("", "a.jpg") {
		t.Fatal("pdf restriction")
	}
	cu.FileTypes = []cuType{cuWAV, cuMP3}
	if !cu.Accepts("", "a.wav") || !cu.Accepts("", "a.mp3") || cu.Accepts("", "a.pdf") {
		t.Fatal("audio restriction")
	}
	if got, ok := ParseCUFileType("webm"); !ok || got != cuWEBM {
		t.Fatal(got, ok)
	}
	if got, ok := ParseCUFileType("m4v"); !ok || got != cuM4V {
		t.Fatal(got, ok)
	}
	if detectCUType("video/mp4", "", []cuType{cuPDF}) != "" {
		t.Fatal("mime restriction")
	}
}

func TestCUDetectAndContentType(t *testing.T) {
	if detectCUType("video/mp4", "", nil) != cuMP4 {
		t.Fatal("mp4")
	}
	if detectCUType("audio/mpeg", "", nil) != cuMP3 {
		t.Fatal("mp3")
	}
	if detectCUType("audio/x-wav", "", nil) != cuWAV {
		t.Fatal("wav")
	}
	if detectCUType("video/x-m4v", "", nil) != cuM4V {
		t.Fatal("m4v")
	}
	cases := []struct{ in, want string }{
		{"audio/x-wav", "audio/wav"},
		{"audio/x-flac", "audio/flac"},
		{"audio/x-m4a", "audio/mp4"},
		{"video/x-m4v", "video/mp4"},
		{"video/mp4", "video/mp4"},
		{"", "application/octet-stream"},
	}
	for _, tc := range cases {
		if canonicalMIME(tc.in) != tc.want {
			t.Fatalf("%s -> %s", tc.in, canonicalMIME(tc.in))
		}
	}
	if contentTypeFor(cuPDF, "") != "application/pdf" || contentTypeFor(cuM4V, "") != "video/mp4" || contentTypeFor(cuFLAC, "audio/x-flac") != "audio/flac" {
		t.Fatal("content type")
	}
	if contentTypeFor(cuPDF, "audio/mpeg") != "application/pdf" || contentTypeFor(cuMP3, "application/pdf") != "audio/mpeg" {
		t.Fatal("conflict")
	}
	if contentTypeFor(cuMP4, "image/jpeg") != "video/mp4" || contentTypeFor(cuJPEG, "video/mp4") != "image/jpeg" {
		t.Fatal("conflict media")
	}
	if contentTypeFor(cuHEIF, "image/heic") != "image/heic" || contentTypeFor(cuHEIF, "image/heif") != "image/heif" {
		t.Fatal("heif")
	}
}

type cuCall struct {
	analyzer, contentType string
	body                  []byte
	results               int
}

func TestCURouting(t *testing.T) {
	var got cuCall
	client := &recordCU{result: CUResult{}, on: func(c cuCall) { got = c }}
	run := func(cu *ContentUnderstanding, mime, name string, body []byte) {
		t.Helper()
		got = cuCall{}
		if _, err := cu.Convert(body, mime, name); err != nil {
			t.Fatal(err)
		}
	}
	cu, err := NewContentUnderstanding("https://fake", "", client)
	if err != nil {
		t.Fatal(err)
	}
	cu.AnalyzerID = "my-doc-analyzer"
	cu.Modality = "document"
	run(cu, "application/pdf", "a.pdf", []byte("fake pdf"))
	if got.analyzer != "my-doc-analyzer" {
		t.Fatal(got)
	}
	run(cu, "audio/mpeg", "a.mp3", []byte("fake audio"))
	if got.analyzer != "prebuilt-audioSearch" {
		t.Fatal(got)
	}
	run(cu, "video/mp4", "a.mp4", []byte("fake video"))
	if got.analyzer != "prebuilt-videoSearch" {
		t.Fatal(got)
	}
	cu.AnalyzerID, cu.Modality = "", ""
	run(cu, "application/pdf", "a.pdf", []byte("fake pdf"))
	if got.analyzer != "prebuilt-documentSearch" {
		t.Fatal(got)
	}
	run(cu, "image/jpeg", "a.jpg", []byte("fake image"))
	if got.analyzer != "prebuilt-documentSearch" {
		t.Fatal(got)
	}
	cu.AnalyzerID, cu.Modality = "my-doc-analyzer", "document"
	run(cu, "image/jpeg", "a.jpg", []byte("fake image"))
	if got.analyzer != "my-doc-analyzer" {
		t.Fatal(got)
	}
	cu.AnalyzerID, cu.Modality = "my-image-analyzer", "image"
	run(cu, "image/jpeg", "a.jpg", []byte("fake image"))
	if got.analyzer != "my-image-analyzer" {
		t.Fatal(got)
	}
	run(cu, "application/pdf", "a.pdf", []byte("fake pdf"))
	if got.analyzer != "prebuilt-documentSearch" {
		t.Fatal(got)
	}
	cu.AnalyzerID, cu.Modality = "", ""
	for _, tc := range []struct{ mime, analyzer string }{
		{"video/mp4", "prebuilt-videoSearch"},
		{"video/x-m4v", "prebuilt-videoSearch"},
		{"audio/mpeg", "prebuilt-audioSearch"},
		{"audio/x-wav", "prebuilt-audioSearch"},
	} {
		run(cu, tc.mime, "", []byte("fake content"))
		if got.analyzer != tc.analyzer {
			t.Fatalf("%s %s", tc.mime, got.analyzer)
		}
	}
	run(cu, "video/x-m4v", "", []byte("fake video"))
	if got.analyzer != "prebuilt-videoSearch" || got.contentType != "video/mp4" {
		t.Fatal(got)
	}
	run(cu, "", "a.pdf", []byte("fake pdf"))
	if got.analyzer != "prebuilt-documentSearch" || got.contentType != "application/pdf" {
		t.Fatal(got)
	}
	run(cu, "audio/mpeg", "a.pdf", []byte("fake"))
	if got.analyzer != "prebuilt-documentSearch" || got.contentType != "application/pdf" || string(got.body) != "fake" || got.results != 1 {
		t.Fatal(got)
	}
}

type recordCU struct {
	result CUResult
	on     func(cuCall)
	gets   int
}

func (r *recordCU) BeginAnalyzeBinary(analyzerID string, input []byte, contentType string) (CUPoller, error) {
	p := &recordPoller{result: r.result}
	if r.on != nil {
		r.on(cuCall{analyzerID, contentType, append([]byte(nil), input...), 0})
	}
	p.after = func() {
		if r.on != nil {
			r.on(cuCall{analyzerID, contentType, append([]byte(nil), input...), p.n})
		}
	}
	return p, nil
}

func (r *recordCU) GetAnalyzer(id string) (string, error) {
	r.gets++
	return "", nil
}

type recordPoller struct {
	result CUResult
	n      int
	after  func()
}

func (p *recordPoller) Result() (CUResult, error) {
	p.n++
	if p.after != nil {
		p.after()
	}
	return p.result, nil
}

func TestCUAnalyzerModality(t *testing.T) {
	client := &recordCU{}
	for _, id := range []string{"prebuilt-documentSearch", "prebuilt-invoice", "prebuilt-layout", "prebuilt-receipt", "prebuilt-tax.us.w2"} {
		mod, err := resolveAnalyzerModality(client, id)
		if err != nil || mod != "document" {
			t.Fatal(id, mod, err)
		}
	}
	for _, id := range []string{"prebuilt-audioSearch", "prebuilt-callCenter"} {
		mod, err := resolveAnalyzerModality(client, id)
		if err != nil || mod != "audio" {
			t.Fatal(id, mod, err)
		}
	}
	for _, id := range []string{"prebuilt-videoSearch", "prebuilt-videoSynopsis"} {
		mod, err := resolveAnalyzerModality(client, id)
		if err != nil || mod != "video" {
			t.Fatal(id, mod, err)
		}
	}
	for _, id := range []string{"prebuilt-imageSearch", "prebuilt-image"} {
		mod, err := resolveAnalyzerModality(client, id)
		if err != nil || mod != "image" {
			t.Fatal(id, mod, err)
		}
	}
	if client.gets != 0 {
		t.Fatal("known prebuilt called get_analyzer")
	}
	lookup := &lookupCU{base: "prebuilt-audio"}
	mod, err := resolveAnalyzerModality(lookup, "prebuilt-newAnalyzer")
	if err != nil || mod != "audio" || lookup.id != "prebuilt-newAnalyzer" {
		t.Fatal(mod, err, lookup.id)
	}
	lookup = &lookupCU{base: "prebuilt-document"}
	mod, err = resolveAnalyzerModality(lookup, "my-custom-doc-analyzer")
	if err != nil || mod != "document" || lookup.id != "my-custom-doc-analyzer" {
		t.Fatal(mod, err)
	}
	lookup = &lookupCU{base: ""}
	mod, err = resolveAnalyzerModality(lookup, "my-custom-analyzer")
	if err != nil || mod != "document" {
		t.Fatal(mod, err)
	}
	lookup = &lookupCU{err: errors.New("not found")}
	_, err = resolveAnalyzerModality(lookup, "bad-id")
	if err == nil || !strings.Contains(err.Error(), "Failed to resolve analyzer 'bad-id'") {
		t.Fatal(err)
	}
	_, err = NewContentUnderstanding("https://fake", "bad-id", lookup)
	if err == nil || !strings.Contains(err.Error(), "Failed to resolve analyzer 'bad-id'") {
		t.Fatal(err)
	}
}

type lookupCU struct {
	base string
	id   string
	err  error
}

func (l *lookupCU) BeginAnalyzeBinary(string, []byte, string) (CUPoller, error) {
	return &recordPoller{}, nil
}
func (l *lookupCU) GetAnalyzer(id string) (string, error) {
	l.id = id
	return l.base, l.err
}

func TestCUConvertFormatsStubResult(t *testing.T) {
	body := "# Test\n\n| Item | Qty |\n| --- | --- |\n| Pen | 2 |"
	client := &recordCU{result: CUResult{Contents: []CUContent{{
		MIME: "application/pdf", Markdown: body, HasPages: true, StartPage: 1, EndPage: 1,
		Fields: map[string]string{"Customer": "Ada"},
	}}}}
	cu, err := NewContentUnderstanding("https://fake", "", client)
	if err != nil {
		t.Fatal(err)
	}
	res, err := cu.Convert([]byte("service input"), "application/pdf", "a.pdf")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"mimeType: application/pdf", "Customer: Ada", body} {
		if !strings.Contains(res.Text, s) {
			t.Fatalf("missing %s\n%s", s, res.Text)
		}
	}
	client.result = CUResult{Contents: []CUContent{{MIME: "video/mp4", Markdown: "Speaker 1: Hello", AudioVisual: true}}}
	res, err = cu.Convert([]byte("service input"), "video/mp4", "a.mp4")
	if err != nil || !strings.Contains(res.Text, "mimeType: video/mp4") || !strings.Contains(res.Text, "Speaker 1: Hello") {
		t.Fatal(res.Text, err)
	}
	client.result = CUResult{Contents: []CUContent{{MIME: "audio/wav", Markdown: "Speaker 1: Hi", AudioVisual: true}}}
	res, err = cu.Convert([]byte("service input"), "audio/wav", "a.wav")
	if err != nil || !strings.Contains(res.Text, "mimeType: audio/wav") || !strings.Contains(res.Text, "Speaker 1: Hi") {
		t.Fatal(res.Text, err)
	}
	client.result = CUResult{}
	res, err = cu.Convert([]byte("service input"), "application/pdf", "a.pdf")
	if err != nil || res.Text != "" {
		t.Fatal(res.Text, err)
	}
	client.result = CUResult{Contents: []CUContent{{MIME: "image/jpeg", Markdown: "# Photo"}}}
	res, err = cu.Convert([]byte("service input"), "image/jpeg", "a.jpg")
	if err != nil || !strings.Contains(res.Text, "mimeType: image/jpeg") || !strings.Contains(res.Text, "# Photo") {
		t.Fatal(res.Text, err)
	}
}

func TestCURegisteredBeforeDocIntel(t *testing.T) {
	cu, err := NewContentUnderstanding("https://fake-cu", "", &recordCU{result: CUResult{Contents: []CUContent{{
		MIME: "application/pdf", Markdown: "from-cu",
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	di := NewDocIntel("https://fake-di", nil, stubDocIntel{analyze: func(string, []byte, []string, string) (string, error) {
		return "from-di", nil
	}})
	order := CloudConverterOrder(Options{ContentUnderstanding: cu, DocIntel: di})
	if len(order) != 2 || order[0] != KindCU || order[1] != KindDocIntel {
		t.Fatal(order)
	}
	res, err := ExtractOptions(bytes.NewReader([]byte("%PDF")), "application/pdf", "a.pdf", 0, Options{
		ContentUnderstanding: cu, DocIntel: di,
	})
	if err != nil || res.Kind != KindCU || !strings.Contains(res.Text, "from-cu") || strings.Contains(res.Text, "from-di") {
		t.Fatalf("%#v %v", res, err)
	}
}

func TestCloudWithoutClientDoesNotInventText(t *testing.T) {
	cu, err := NewContentUnderstanding("https://fake-cu", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ExtractOptions(bytes.NewReader([]byte("%PDF")), "application/pdf", "a.pdf", 0, Options{ContentUnderstanding: cu})
	if !errors.Is(err, ErrCloudClient) {
		t.Fatal(err)
	}
	di := NewDocIntel("https://fake-di", nil, nil)
	_, err = ExtractOptions(bytes.NewReader([]byte("%PDF")), "application/pdf", "a.pdf", 0, Options{DocIntel: di})
	if !errors.Is(err, ErrCloudClient) {
		t.Fatal(err)
	}
}

func TestCUModalityMapping(t *testing.T) {
	if cuModality(cuPDF) != "document" || cuModality(cuDOCX) != "document" {
		t.Fatal("document")
	}
	if cuModality(cuJPEG) != "image" || cuModality(cuPNG) != "image" {
		t.Fatal("image")
	}
	if cuModality(cuMP4) != "video" || cuModality(cuMOV) != "video" {
		t.Fatal("video")
	}
	if cuModality(cuWAV) != "audio" || cuModality(cuMP3) != "audio" {
		t.Fatal("audio")
	}
}

func TestDocIntelDoesNotReplaceLocalPDF(t *testing.T) {
	res, err := Extract(bytes.NewReader([]byte("%PDF-1.4\nnot really\n%%EOF\n")), "", "a.pdf", 0)
	if err == nil && res.Kind == KindCU {
		t.Fatal("cloud ran without a client")
	}
}
