package markitdown_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/godwit-email/markitdown-go"
	"github.com/godwit-email/markitdown-go/html"
)

func htmlOf(t *testing.T, s string) string {
	t.Helper()
	res, err := markitdown.Extract(context.Background(), markitdown.Request{R: strings.NewReader(s), MIME: "text/html", Filename: "a.html", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	return res.Text
}

func htmlKeep(t *testing.T, s string, keep bool) markitdown.Result {
	t.Helper()
	res, err := html.ExtractOptions(context.Background(), markitdown.Request{R: strings.NewReader(s), MIME: "text/html", Filename: "a.html"}, html.Options{KeepDataURIs: keep})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestUnderlineWhitespace(t *testing.T) {
	cases := []struct{ content, want string }{
		{"", "FirstLast"},
		{" ", "First Last"},
		{"\t", "First Last"},
		{"&#160;", "First\u00a0Last"},
		{"<br>", "First\nLast"},
		{"word", "First<u>word</u>Last"},
		{" word ", "First <u>word</u> Last"},
	}
	for _, tc := range cases {
		got := htmlOf(t, "<p>First<u>"+tc.content+"</u>Last</p>")
		if got != tc.want {
			t.Fatalf("content %q\n got %q\nwant %q", tc.content, got, tc.want)
		}
	}
}

func TestHTMLHrefs(t *testing.T) {
	href := "https://abc.com/hist/" + "%a5%c8%a5%c3%a5%d7%a5%da%a1%bc%a5%b8"
	got := htmlOf(t, `<a href="`+href+`">example</a>`)
	if !strings.Contains(got, "[example]("+href+")") || strings.Contains(got, "%EF%BF%BD") {
		t.Fatal(got)
	}
	got = htmlOf(t, `<a href="https://example.com/a path/日本語">example</a>`)
	if !strings.Contains(got, "[example](https://example.com/a%20path/%E6%97%A5%E6%9C%AC%E8%AA%9E)") {
		t.Fatal(got)
	}
	got = htmlOf(t, `<a href="https://example.com/100% complete">example</a>`)
	if !strings.Contains(got, "[example](https://example.com/100%25%20complete)") {
		t.Fatal(got)
	}
	got = htmlOf(t, `<a href="https://example.com/items/%ZZ/%2F">example</a>`)
	if !strings.Contains(got, "[example](https://example.com/items/%25ZZ/%2F)") {
		t.Fatal(got)
	}
	got = htmlOf(t, `<a href="https://example.com/items/a%2Fb">example</a>`)
	if !strings.Contains(got, "[example](https://example.com/items/a%2Fb)") {
		t.Fatal(got)
	}
	got = htmlOf(t, `<a href="https://example.com/a path?query=a b%20c#fragment with spaces">example</a>`)
	if !strings.Contains(got, "[example](https://example.com/a%20path?query=a b%20c#fragment with spaces)") {
		t.Fatal(got)
	}
}

func TestHTMLImages(t *testing.T) {
	placeholder := "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBTAA7"
	real := "https://example.com/photo.jpg"
	other := "https://example.com/photo-alt.jpg"
	got := htmlOf(t, `<img src="`+placeholder+`" data-src="`+real+`" alt="A photo">`)
	if !strings.Contains(got, "![A photo]("+real+")") || strings.Contains(got, placeholder) {
		t.Fatal(got)
	}
	got = htmlOf(t, `<img src="`+real+`" data-src="`+other+`" alt="A photo">`)
	if !strings.Contains(got, "![A photo]("+real+")") {
		t.Fatal(got)
	}
	got = htmlOf(t, `<img data-src="`+real+`" alt="A photo">`)
	if !strings.Contains(got, "![A photo]("+real+")") {
		t.Fatal(got)
	}
	got = htmlOf(t, `<img src="`+placeholder+`" alt="A photo">`)
	if !strings.Contains(got, "![A photo](data:image/gif;base64...)") {
		t.Fatal(got)
	}
	res := htmlKeep(t, `<html><body><img src="`+placeholder+`" data-src="`+other+`" alt="A photo"></body></html>`, true)
	if !strings.Contains(res.Text, "![A photo]("+placeholder+")") || strings.Contains(res.Text, other) {
		t.Fatal(res.Text)
	}
	got = htmlOf(t, `<html><body><img alt="dot" src="DATA:image/png;base64,AAAA"></body></html>`)
	if got != "![dot](DATA:image/png;base64...)" || strings.Contains(got, "AAAA") {
		t.Fatal(got)
	}
	res = htmlKeep(t, `<html><body><img alt="dot" src="DATA:image/png;base64,AAAA"></body></html>`, true)
	if res.Text != "![dot](DATA:image/png;base64,AAAA)" {
		t.Fatalf("%q", res.Text)
	}
}

func TestWikipediaTitles(t *testing.T) {
	page := func(heading, documentTitle string) string {
		return `<html><head><title>` + documentTitle + ` - Wikipedia</title></head><body>
<h1 id="firstHeading" class="firstHeading mw-first-heading">` + heading + `</h1>
<div id="mw-content-text"><p>Body text.</p></div>
</body></html>`
	}
	res := htmlKeep(t, page(`<span lang="en" dir="ltr"><span class="mw-page-title-main">Paris</span></span>`, "Paris"), false)
	if !strings.HasPrefix(strings.TrimLeft(res.Text, "\n"), "# Paris") {
		t.Fatalf("%q", res.Text)
	}
	res = htmlKeep(t, page("<i>Escherichia coli</i>", "Escherichia coli"), false)
	if !strings.HasPrefix(strings.TrimLeft(res.Text, "\n"), "# Escherichia coli") {
		t.Fatalf("%q", res.Text)
	}
	res = htmlKeep(t, page("<i>Titanic</i> (1997 film)", "Titanic (1997 film)"), false)
	if !strings.HasPrefix(strings.TrimLeft(res.Text, "\n"), "# Titanic (1997 film)") {
		t.Fatalf("%q", res.Text)
	}
	res = htmlKeep(t, `<html><body><div id='mw-content-text'><p>Hello</p></div></body></html>`, false)
	if strings.Contains(res.Text, "# None") || strings.TrimSpace(res.Text) != "Hello" {
		t.Fatalf("%q", res.Text)
	}
	res = htmlKeep(t, `<html><head><title>   </title></head><body><div id='mw-content-text'><p>Hello</p></div></body></html>`, false)
	if strings.HasPrefix(strings.TrimLeft(res.Text, "\n"), "#") || strings.TrimSpace(res.Text) != "Hello" {
		t.Fatalf("%q", res.Text)
	}
}

func TestStrikethroughAndDeepHTML(t *testing.T) {
	html := `<!doctype html><html><body>
<p>Plain <s>s element</s> after.</p>
<p>Plain <del>del element</del> after.</p>
<p>Plain <strike>strike element</strike> after.</p>
<p>Spaces A<strike> B </strike>C.</p>
<p>Runs D<strike>  E  </strike>F.</p>
<p>Empty G<strike></strike>H.</p>
<p>Newline I<strike>J
K</strike>L.</p>
<p>Break M<strike>N<br>O</strike>P.</p>
</body></html>`
	want := strings.Join([]string{
		"Plain ~~s element~~ after.",
		"Plain ~~del element~~ after.",
		"Plain ~~strike element~~ after.",
		"Spaces A ~~B~~ C.",
		"Runs D ~~E~~ F.",
		"Empty GH.",
		"Newline I~~J\nK~~L.",
		"Break M~~N\nO~~P.",
	}, "\n\n")
	if got := htmlOf(t, html); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}

	var b strings.Builder
	b.WriteString("<html><body>")
	for i := 0; i < 500; i++ {
		b.WriteString(`<div style="margin-left:10px">`)
	}
	b.WriteString("<p>Deep content with <b>bold text</b></p>")
	for i := 0; i < 500; i++ {
		b.WriteString("</div>")
	}
	b.WriteString("</body></html>")
	got := htmlOf(t, b.String())
	if !strings.Contains(got, "Deep content") || !strings.Contains(got, "bold text") || strings.Contains(got, "<div") || strings.Contains(got, "<p>") {
		t.Fatalf("%s", got)
	}
}

func TestHTMLVectors(t *testing.T) {
	blog, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, "test_blog.html")), MIME: "text/html; charset=utf-8", Filename: "test_blog.html", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"Large language models (LLMs) are powerful tools that can generate natural language texts for various applications, such as chatbots, summarization, translation, and more. GPT-4 is currently the state of the art LLM in the world. Is model selection irrelevant? What about inference parameters?",
		"an example where high cost can easily prevent a generic complex",
	} {
		if !strings.Contains(blog.Text, s) {
			t.Fatalf("blog missing %s", s[:40])
		}
	}
	wiki, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, "test_wikipedia.html")), MIME: "text/html; charset=utf-8", Filename: "test_wikipedia.html", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"Microsoft entered the operating system (OS) business in 1980 with its own version of [Unix]",
		`[Bill Gates](/wiki/Bill_Gates "Bill Gates")`,
	} {
		if !strings.Contains(wiki.Text, s) {
			t.Fatalf("wiki missing %s", s)
		}
	}
	for _, s := range []string{
		"You are encouraged to create an account and log in",
		"154 languages",
		"move to sidebar",
	} {
		if strings.Contains(wiki.Text, s) {
			t.Fatalf("wiki has %s", s)
		}
	}
	serp, err := markitdown.Extract(context.Background(), markitdown.Request{R: bytes.NewReader(testFile(t, "test_serp.html")), MIME: "text/html; charset=utf-8", Filename: "test_serp.html", MaxOut: 0})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"](https://en.wikipedia.org/wiki/Microsoft",
		"Microsoft Corporation is **an American multinational corporation and technology company headquartered** in Redmond",
		"1995–2007: Foray into the Web, Windows 95, Windows XP, and Xbox",
	} {
		if !strings.Contains(serp.Text, s) {
			t.Fatalf("serp missing %s", s)
		}
	}
	for _, s := range []string{
		"https://www.bing.com/ck/a?!&&p=",
		"data:image/svg+xml,%3Csvg%20width%3D",
	} {
		if strings.Contains(serp.Text, s) {
			t.Fatalf("serp has %s", s)
		}
	}
}
