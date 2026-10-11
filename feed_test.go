package markitdown

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestAtomNamespacePrefixes(t *testing.T) {
	feed := `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Example feed</title>
  <subtitle>Feed subtitle</subtitle>
  <id>urn:example:feed</id>
  <updated>2026-01-01T00:00:00Z</updated>
  <author><name>Example author</name></author>
  <entry>
    <title>Example entry</title>
    <id>urn:example:entry</id>
    <updated>2026-01-02T00:00:00Z</updated>
    <summary type="html">&lt;p&gt;An &lt;em&gt;entry summary&lt;/em&gt;.&lt;/p&gt;</summary>
    <content type="xhtml">
      <div xmlns="http://www.w3.org/1999/xhtml">
        <p>Read the <strong>important details</strong>.</p>
      </div>
    </content>
  </entry>
</feed>`
	res, err := Extract(strings.NewReader(feed), "application/xml", "feed.xml", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "Example feed" {
		t.Fatal(res.Title)
	}
	if !strings.HasPrefix(res.Text, "# Example feed\nFeed subtitle\n") {
		t.Fatalf("%q", res.Text)
	}
	for _, s := range []string{
		"## Example entry\nUpdated on: 2026-01-02T00:00:00Z",
		"An *entry summary*.",
		"Read the **important details**.",
	} {
		if !strings.Contains(res.Text, s) {
			t.Fatalf("missing %q\n%s", s, res.Text)
		}
	}
}

func TestAtomWithoutNamespace(t *testing.T) {
	feed := `<feed><title>Example feed</title><entry><title>Example entry</title><content>Entry content.</content></entry></feed>`
	res, err := Extract(strings.NewReader(feed), "", "a.xml", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "Example feed" || !strings.Contains(res.Text, "## Example entry") || !strings.Contains(res.Text, "Entry content.") {
		t.Fatalf("%q", res.Text)
	}
}

func TestRSSVector(t *testing.T) {
	res, err := Extract(bytes.NewReader(testFile(t, "test_rss.xml")), "text/xml", "test_rss.xml", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"# The Official Microsoft Blog",
		"## Ignite 2024: Why nearly 70% of the Fortune 500 now use Microsoft 365 Copilot",
		"In the case of AI, it is absolutely true that the industry is moving incredibly fast",
	} {
		if !strings.Contains(res.Text, s) {
			t.Fatalf("missing %s\n%s", s, res.Text[:min(500, len(res.Text))])
		}
	}
	if strings.Contains(res.Text, "<rss") || strings.Contains(res.Text, "<feed") {
		t.Fatal("raw xml leaked")
	}
}

func TestRSSTitlesLinksAndEncoded(t *testing.T) {
	pretty := `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0"><channel>
  <title>
    Example feed
  </title>
  <description>
    Example feed description
  </description>
  <item>
    <title>
      A story about things
    </title>
    <pubDate>
      Mon, 01 Jan 2024 00:00:00 GMT
    </pubDate>
    <description>Body text.</description>
  </item>
</channel></rss>`
	res, err := convertRSS([]byte(pretty), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "Example feed" {
		t.Fatal(res.Title)
	}
	wantLines := []string{
		"# Example feed",
		"",
		"    Example feed description",
		"  ",
		"",
		"## A story about things",
		"Published on: Mon, 01 Jan 2024 00:00:00 GMT",
		"Body text.",
	}
	if strings.Join(strings.Split(res.Text, "\n"), "\n") != strings.Join(wantLines, "\n") && res.Text != strings.Join(wantLines, "\n") {
		t.Fatalf("%q", res.Text)
	}
	encoded := `<rss xmlns:c="http://purl.org/rss/1.0/modules/content/"><channel><title>Feed</title><item><title>Entry</title><extension><description>Wrong body.</description></extension><c:encoded>Before <b>nested</b> <![CDATA[after.]]></c:encoded></item></channel></rss>`
	res, err = convertRSS([]byte(encoded), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "# Feed\n\n## Entry\nBefore **nested** after." {
		t.Fatalf("%q", res.Text)
	}
	link := `<feed xmlns="http://www.w3.org/2005/Atom"><title>Feed</title><entry><title>Entry</title><content type="html">&lt;a href="../stories/café%20notes?x=1&amp;y=2#part"&gt;Read&lt;/a&gt;</content></entry></feed>`
	res, err = convertRSS([]byte(link), "https://example.com/news/feed.xml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(res.Text, "[Read](https://example.com/stories/caf%C3%A9%20notes?x=1&y=2#part)") {
		t.Fatalf("%q", res.Text)
	}
}

func TestAtomXMLBase(t *testing.T) {
	feed := `<a:feed xmlns:a="http://www.w3.org/2005/Atom" xmlns:x="http://www.w3.org/1999/xhtml" xml:base="https://example.com/root/">
      <a:title>Feed</a:title>
      <a:entry xml:base="entries/">
        <a:title>Entry</a:title>
        <a:summary type="html" xml:base="../summaries/"><![CDATA[<a href="summary">Summary</a>]]></a:summary>
        <a:content type="xhtml" xml:base="../articles/">
          <x:div xml:base="chapter/">
            <x:p xml:base="../appendix/">
              <x:a xml:base="../../assets/" href="story">Story</x:a>
              <x:a href="sibling">Sibling</x:a>
            </x:p>
            <x:a href="after">After</x:a>
            <x:img src="diagram.png" alt="Diagram"/>
            <x:img src="" data-src="../lazy.png" alt="Lazy"/>
          </x:div>
        </a:content>
      </a:entry>
      <a:entry><a:title>Next</a:title>
        <a:content type="html">&lt;a href="next"&gt;Next link&lt;/a&gt;</a:content>
      </a:entry>
    </a:feed>`
	res, err := convertRSS([]byte(feed), "https://other.example/feed.xml")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"[Summary](https://example.com/root/summaries/summary)",
		"[Story](https://example.com/root/assets/story)",
		"[Sibling](https://example.com/root/articles/appendix/sibling)",
		"[After](https://example.com/root/articles/chapter/after)",
		"![Diagram](https://example.com/root/articles/chapter/diagram.png)",
		"![Lazy](https://example.com/root/articles/lazy.png)",
		"[Next link](https://example.com/root/next)",
	} {
		if !strings.Contains(res.Text, s) {
			t.Errorf("missing %s\n%s", s, res.Text)
		}
	}
}

func TestFeedCDATAPreAndZipDataURI(t *testing.T) {
	feed := `<feed xmlns="http://www.w3.org/2005/Atom"><title>Feed</title><entry><title>Entry</title><content type="html"><![CDATA[<pre>line 1
  line 2</pre>]]></content></entry></feed>`
	res, err := convertRSS([]byte(feed), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "```\nline 1\n  line 2\n```") {
		t.Fatalf("%q", res.Text)
	}
	page := `<html><body><img alt="diagram" src="data:image/png;base64,AAAA"></body></html>`
	buf := zipEntries(t, []zipEntry{{"page.html", page}})
	full, err := convertZIP(buf, "a.zip", budget{depth: 3, files: intPtr(10), uncompressed: intPtr64(1 << 20), keepData: true})
	if err != nil || !strings.Contains(full.Text, "data:image/png;base64,AAAA") {
		t.Fatalf("%q %v", full.Text, err)
	}
	cut, err := convertZIP(buf, "a.zip", budget{depth: 3, files: intPtr(10), uncompressed: intPtr64(1 << 20)})
	if err != nil || strings.Contains(cut.Text, "AAAA") || !strings.Contains(cut.Text, "data:image/png;base64...") {
		t.Fatalf("%q %v", cut.Text, err)
	}
}

func intPtr(n int) *int       { return &n }
func intPtr64(n int64) *int64 { return &n }

func TestEPUBFixture(t *testing.T) {
	res, err := Extract(bytes.NewReader(testFile(t, "test.epub")), "application/epub+zip", "test.epub", 0)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(res.Text, `\`, "")
	for _, s := range []string{
		"**Authors:** Test Author",
		"A test EPUB document for MarkItDown testing",
		"# Chapter 1: Test Content",
		"This is a **test** paragraph with some formatting",
		"* A bullet point",
		"* Another point",
		"# Chapter 2: More Content",
		"*different* style",
		"> This is a blockquote for testing",
	} {
		if !strings.Contains(text, s) {
			t.Fatalf("missing %s\n%s", s, text)
		}
	}
}

func TestEPUBHrefAndDataURI(t *testing.T) {
	spaced := buildEPUB(t, [][2]string{{"c1", "chapter%201.xhtml"}, {"c2", "plain.xhtml"}}, []string{"c1", "c2"}, map[string][2]string{
		"OEBPS/chapter 1.xhtml": {"First", "SPACED_BODY"},
		"OEBPS/plain.xhtml":     {"Second", "PLAIN_BODY"},
	})
	res, err := convertEPUB(spaced, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "SPACED_BODY") || !strings.Contains(res.Text, "PLAIN_BODY") || strings.Index(res.Text, "SPACED_BODY") > strings.Index(res.Text, "PLAIN_BODY") {
		t.Fatal(res.Text)
	}
	accent := buildEPUB(t, [][2]string{{"c1", "cap%C3%ADtulo.xhtml"}}, []string{"c1"}, map[string][2]string{
		"OEBPS/capítulo.xhtml": {"Capítulo", "ACCENTED_BODY"},
	})
	res, err = convertEPUB(accent, false)
	if err != nil || !strings.Contains(res.Text, "ACCENTED_BODY") {
		t.Fatal(res.Text, err)
	}
	lit := buildEPUB(t, [][2]string{{"c1", "chapter%201.xhtml"}}, []string{"c1"}, map[string][2]string{
		"OEBPS/chapter%201.xhtml": {"Literal", "LITERAL_BODY"},
	})
	res, err = convertEPUB(lit, false)
	if err != nil || !strings.Contains(res.Text, "LITERAL_BODY") {
		t.Fatal(res.Text, err)
	}
	shared := buildEPUB(t, [][2]string{{"c1", "../shared/chapter.xhtml"}}, []string{"c1"}, map[string][2]string{
		"shared/chapter.xhtml": {"Shared", "SHARED_BODY"},
	})
	res, err = convertEPUB(shared, false)
	if err != nil || !strings.Contains(res.Text, "SHARED_BODY") {
		t.Fatal(res.Text, err)
	}

	chapter := `<html xmlns="http://www.w3.org/1999/xhtml"><body><p>Chapter text.</p><img alt="diagram" src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUg=="/></body></html>`
	book := buildRawEPUB(t, "Example book", "ch1.xhtml", chapter)
	res, err = convertEPUB(book, true)
	if err != nil || !strings.Contains(res.Text, "![diagram](data:image/png;base64,iVBORw0KGgoAAAANSUhEUg==)") {
		t.Fatal(res.Text, err)
	}
	res, err = convertEPUB(book, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "![diagram](data:image/png;base64...)") || strings.Contains(res.Text, "iVBORw0KGgo") {
		t.Fatal(res.Text)
	}
	if res.Title != "Example book" || !strings.Contains(res.Text, "**Title:** Example book") || !strings.Contains(res.Text, "Chapter text.") {
		t.Fatal(res.Text)
	}
}

func TestZIPRemaining(t *testing.T) {
	buf := zipEntries(t, []zipEntry{{"notes.txt", "First archived entry."}, {"notes.txt", "Second archived entry."}})
	res, err := Extract(bytes.NewReader(buf), "application/zip", "duplicate.zip", 0)
	if err != nil {
		t.Fatal(err)
	}
	want := "Content from the zip file `duplicate.zip`:\n\n## File: notes.txt\n\nFirst archived entry.\n\n## File: notes.txt\n\nSecond archived entry."
	if res.Text != want {
		t.Fatalf("%q", res.Text)
	}
	hello := zipOf(t, map[string]string{"hello.txt": "Hello world"})
	res, err = Extract(bytes.NewReader(hello), "application/zip", "", 0)
	if err != nil || !strings.HasPrefix(res.Text, "Content from the zip file `(unknown)`:\n\n") || !strings.Contains(res.Text, "Hello world") {
		t.Fatalf("%q %v", res.Text, err)
	}
	docx := testFile(t, "test.docx")
	alone, err := Extract(bytes.NewReader(docx), "", "test.docx", 0)
	if err != nil {
		t.Fatal(err)
	}
	wrapped := zipEntries(t, []zipEntry{{"test.docx", string(docx)}})
	res, err = Extract(bytes.NewReader(wrapped), "", "archive.zip", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "## File: test.docx") || !strings.Contains(res.Text, strings.TrimSpace(alone.Text)) {
		t.Fatal("docx member mismatch")
	}
	if strings.Contains(res.Text, "word/document.xml") || strings.Contains(res.Text, "[Content_Types].xml") {
		t.Fatal("unpacked docx")
	}
	bundle, err := Extract(bytes.NewReader(testFile(t, "test_files.zip")), "application/zip", "test_files.zip", 0)
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

func TestMoreFeedBodies(t *testing.T) {
	cases := []string{
		`<feed xmlns="http://www.w3.org/2005/Atom"><title>Example feed</title><entry><title>Example entry</title><content type="text">Run &lt;job_id&gt; with &amp;lt;literal&amp;gt;.</content></entry></feed>`,
		`<feed xmlns="http://www.w3.org/2005/Atom"><title>Example feed</title><entry><title>Example entry</title><summary>A &lt;placeholder&gt; summary.</summary></entry></feed>`,
		`<feed xmlns="http://www.w3.org/2005/Atom"><title>Example feed</title><entry><title>Example entry</title><content type="text/plain">Run &lt;job_id&gt; to start.</content></entry></feed>`,
		`<feed xmlns="http://www.w3.org/2005/Atom"><title>Example feed</title><entry><title>Example entry</title><summary type="text/plain">A &lt;placeholder&gt; summary.</summary></entry></feed>`,
		`<feed xmlns="http://www.w3.org/2005/Atom"><title>Example feed</title><entry><title>Example entry</title><content type="application/octet-stream">iVBORw0KGgoAAAANSUhEUg==</content></entry></feed>`,
	}
	wants := []string{
		"Run <job_id> with &lt;literal&gt;.",
		"A <placeholder> summary.",
		"Run <job_id> to start.",
		"A <placeholder> summary.",
		"",
	}
	for i, feed := range cases {
		res, err := Extract(strings.NewReader(feed), "application/atom+xml", "a.atom", 0)
		if err != nil {
			t.Fatal(err)
		}
		if wants[i] == "" {
			if strings.Contains(res.Text, "iVBORw0KGgo") {
				t.Fatal(res.Text)
			}
			continue
		}
		if !strings.Contains(res.Text, wants[i]) {
			t.Fatalf("case %d\n%s", i, res.Text)
		}
	}
	other := `<a:feed xmlns:a="http://www.w3.org/2005/Atom" xmlns="urn:example:other"><title>Unrelated title</title><a:title>Example feed</a:title><entry><title>Unrelated entry</title><content>Unrelated content.</content></entry><a:entry><a:title>Example entry</a:title><content>Unrelated content.</content><a:content>Entry content.</a:content></a:entry></a:feed>`
	res, err := Extract(strings.NewReader(other), "", "a.atom", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "Example feed" || strings.Contains(res.Text, "Unrelated") || !strings.Contains(res.Text, "Entry content.") {
		t.Fatalf("%q %q", res.Title, res.Text)
	}
	body := `<feed xmlns="http://www.w3.org/2005/Atom"><title>Feed</title><entry><title>Entry</title><content type="html">First&lt;br/&gt;Second</content></entry></feed>`
	res, err = Extract(strings.NewReader(body), "", "a.atom", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "First  \nSecond") {
		t.Fatalf("%q", res.Text)
	}
}

func buildEPUB(t *testing.T, items [][2]string, spine []string, docs map[string][2]string) []byte {
	t.Helper()
	var manifest, spineXML strings.Builder
	for _, item := range items {
		manifest.WriteString(`<item id="` + item[0] + `" href="` + item[1] + `" media-type="application/xhtml+xml"/>`)
	}
	for _, id := range spine {
		spineXML.WriteString(`<itemref idref="` + id + `"/>`)
	}
	opf := `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Encoded Hrefs</dc:title></metadata><manifest>` + manifest.String() + `</manifest><spine>` + spineXML.String() + `</spine></package>`
	container := `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZip(t, zw, "mimetype", "application/epub+zip")
	writeZip(t, zw, "META-INF/container.xml", container)
	writeZip(t, zw, "OEBPS/content.opf", opf)
	for name, doc := range docs {
		writeZip(t, zw, name, `<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>`+doc[0]+`</h1><p>`+doc[1]+`</p></body></html>`)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func buildRawEPUB(t *testing.T, title, chapterName, chapter string) []byte {
	t.Helper()
	opf := `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="2.0"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>` + title + `</dc:title></metadata><manifest><item id="c1" href="` + chapterName + `" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="c1"/></spine></package>`
	container := `<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf"/></rootfiles></container>`
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZip(t, zw, "mimetype", "application/epub+zip")
	writeZip(t, zw, "META-INF/container.xml", container)
	writeZip(t, zw, "OEBPS/content.opf", opf)
	writeZip(t, zw, "OEBPS/"+chapterName, chapter)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
