package markitdown_test

import (
	zipfile "archive/zip"
	"bytes"
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/godwit-email/markitdown-go"
	"github.com/godwit-email/markitdown-go/csv"
	"github.com/godwit-email/markitdown-go/docx"
	"github.com/godwit-email/markitdown-go/html"
	"github.com/godwit-email/markitdown-go/pptx"
	"github.com/godwit-email/markitdown-go/text"
	"github.com/godwit-email/markitdown-go/xls"
	"github.com/godwit-email/markitdown-go/xlsx"
	"github.com/godwit-email/markitdown-go/zip"
)

type parityCase struct {
	name      string
	mediaType string
	filename  string
	charset   string
	body      []byte
	file      string
	direct    func(context.Context, markitdown.Request) (markitdown.Result, error)
}

func TestPythonParity(t *testing.T) {
	bin := os.Getenv("MARKITDOWN_ORACLE")
	if bin == "" {
		t.Fatal("MARKITDOWN_ORACLE is not set")
	}
	rawZip := zipMembers(t, []zipPart{
		{"../x.txt", []byte("hello\n")},
		{`dir\notes.txt`, []byte("world\n")},
		{"inner.zip", zipMembers(t, []zipPart{{"b.txt", []byte("inside\n")}})},
	})
	cases := []parityCase{
		{
			name: "text/trailing-space", mediaType: "text/plain", filename: "a.txt",
			body: []byte("a \r\nb  \r\n\r\n\r\nc\n\n\n\nd"), direct: text.Extract,
		},
		{
			name: "text/json", mediaType: "application/json", filename: "test.json",
			file: "test.json", direct: text.Extract,
		},
		{
			name: "text/json-late", mediaType: "application/json", filename: "json_late_non_ascii.json",
			file: "json_late_non_ascii.json", direct: text.Extract,
		},
		{
			name: "text/markdown", mediaType: "text/markdown", filename: "note.md",
			body: []byte("# hi \n"), direct: text.Extract,
		},
		{
			name: "csv/simple", mediaType: "text/csv", filename: "a.csv",
			body: []byte("a,b\n1,2\n"), direct: csv.Extract,
		},
		{
			name: "csv/empty", mediaType: "text/csv", filename: "empty.csv",
			body: []byte(""), direct: csv.Extract,
		},
		{
			name: "csv/pipe", mediaType: "text/csv", filename: "pipe.csv",
			body: []byte("a|b,c\n"), direct: csv.Extract,
		},
		{
			name: "csv/mskanji", mediaType: "text/csv", filename: "test_mskanji.csv",
			charset: "cp932", file: "test_mskanji.csv", direct: csv.Extract,
		},
		{
			name: "csv/octet", mediaType: "application/octet-stream", filename: "a.csv",
			body: []byte("a,b\n1,2\n"), direct: csv.Extract,
		},
		{
			name: "html/blog", mediaType: "text/html", filename: "test_blog.html",
			charset: "utf-8", file: "test_blog.html", direct: html.Extract,
		},
		{
			name: "html/wikipedia", mediaType: "text/html", filename: "test_wikipedia.html",
			charset: "utf-8", file: "test_wikipedia.html", direct: html.Extract,
		},
		{
			name: "html/serp", mediaType: "text/html", filename: "test_serp.html",
			charset: "utf-8", file: "test_serp.html", direct: html.Extract,
		},
		{
			name: "xlsx/fixture", mediaType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			filename: "test.xlsx", file: "test.xlsx", direct: xlsx.Extract,
		},
		{
			name: "xls/fixture", mediaType: "application/vnd.ms-excel",
			filename: "test.xls", file: "test.xls", direct: xls.Extract,
		},
		{
			name: "xls/excel-mime", mediaType: "application/excel",
			filename: "test.xls", file: "test.xls", direct: xls.Extract,
		},
		{
			name: "docx/fixture", mediaType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			filename: "test.docx", file: "test.docx", direct: docx.Extract,
		},
		{
			name: "docx/comment", mediaType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			filename: "test_with_comment.docx", file: "test_with_comment.docx", direct: docx.Extract,
		},
		{
			name: "docx/equations", mediaType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			filename: "equations.docx", file: "equations.docx", direct: docx.Extract,
		},
		{
			name: "docx/rlink", mediaType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			filename: "rlink.docx", file: "rlink.docx", direct: docx.Extract,
		},
		{
			name: "pptx/fixture", mediaType: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
			filename: "test.pptx", file: "test.pptx", direct: pptx.Extract,
		},
		{
			name: "pptx/svg", mediaType: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
			filename: "test_svg_no_fallback.pptx", file: "test_svg_no_fallback.pptx", direct: pptx.Extract,
		},
		{
			name: "pptx/empty-body", mediaType: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
			filename: "pptx_empty_body_run.pptx", file: "pptx_empty_body_run.pptx", direct: pptx.Extract,
		},
		{
			name: "pptx/notes-empty", mediaType: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
			filename: "pptx_notes_empty_run.pptx", file: "pptx_notes_empty_run.pptx", direct: pptx.Extract,
		},
		{
			name: "zip/raw-names", mediaType: "application/zip", filename: "bundle.zip",
			body: rawZip, direct: zip.Extract,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.body
			if tc.file != "" {
				body = testFile(t, tc.file)
			}
			mime := tc.mediaType
			if tc.charset != "" {
				mime += "; charset=" + tc.charset
			}
			req := markitdown.Request{
				R: bytes.NewReader(body), MIME: mime, Filename: tc.filename, MaxOut: 0,
			}
			want := oracleMarkdown(t, bin, body, tc.mediaType, tc.filename, tc.charset)
			got, err := markitdown.Extract(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			sameText(t, "registry", got.Text, want)
			req.R = bytes.NewReader(body)
			direct, err := tc.direct(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			sameText(t, "direct", direct.Text, want)
		})
	}
}

type zipPart struct {
	name string
	body []byte
}

func zipMembers(t *testing.T, parts []zipPart) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zipfile.NewWriter(&buf)
	for _, p := range parts {
		w, err := zw.Create(p.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(p.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func oracleMarkdown(t *testing.T, bin string, body []byte, mediaType, filename, charset string) string {
	t.Helper()
	cmd := exec.Command(bin, mediaType, filename, charset)
	cmd.Stdin = bytes.NewReader(body)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("oracle: %v", err)
	}
	return string(out)
}

func sameText(t *testing.T, which, got, want string) {
	t.Helper()
	if got == want {
		return
	}
	n := 0
	for n < len(got) && n < len(want) && got[n] == want[n] {
		n++
	}
	t.Fatalf("%s mismatch at byte %d\n got %q\nwant %q", which, n, snippet(got, n), snippet(want, n))
}

func snippet(s string, at int) string {
	lo := at - 40
	if lo < 0 {
		lo = 0
	}
	hi := at + 40
	if hi > len(s) {
		hi = len(s)
	}
	return s[lo:hi]
}
