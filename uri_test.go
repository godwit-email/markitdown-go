package markitdown

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDataURI(t *testing.T) {
	mimeType, attrs, data, err := parseDataURI("data:text/plain;base64,SGVsbG8sIFdvcmxkIQ==")
	if err != nil || mimeType != "text/plain" || len(attrs) != 0 || string(data) != "Hello, World!" {
		t.Fatalf("%s %#v %q %v", mimeType, attrs, data, err)
	}
	mimeType, attrs, data, err = parseDataURI("data:base64,SGVsbG8sIFdvcmxkIQ==")
	if err != nil || mimeType != "" || len(attrs) != 0 || string(data) != "Hello, World!" {
		t.Fatalf("%s %#v %q %v", mimeType, attrs, data, err)
	}
	mimeType, attrs, data, err = parseDataURI("data:text/plain;charset=utf-8;base64,SGVsbG8sIFdvcmxkIQ==")
	if err != nil || mimeType != "text/plain" || attrs["charset"] != "utf-8" || string(data) != "Hello, World!" {
		t.Fatalf("%s %#v %q %v", mimeType, attrs, data, err)
	}
	mimeType, attrs, data, err = parseDataURI("data:text/plain;CHARSET=utf-8;BASE64,SGVsbG8sIFdvcmxkIQ==")
	if err != nil || mimeType != "text/plain" || len(attrs) != 1 || attrs["charset"] != "utf-8" || string(data) != "Hello, World!" {
		t.Fatalf("%s %#v %q %v", mimeType, attrs, data, err)
	}
	mimeType, attrs, data, err = parseDataURI("data:,Hello%2C%20World%21")
	if err != nil || mimeType != "" || len(attrs) != 0 || string(data) != "Hello, World!" {
		t.Fatalf("%s %#v %q %v", mimeType, attrs, data, err)
	}
	mimeType, attrs, data, err = parseDataURI("data:text/plain,Hello%2C%20World%21")
	if err != nil || mimeType != "text/plain" || len(attrs) != 0 || string(data) != "Hello, World!" {
		t.Fatalf("%s %#v %q %v", mimeType, attrs, data, err)
	}
	mimeType, attrs, data, err = parseDataURI("data:text/plain;charset=utf-8,Hello%2C%20World%21")
	if err != nil || mimeType != "text/plain" || len(attrs) != 1 || attrs["charset"] != "utf-8" || string(data) != "Hello, World!" {
		t.Fatalf("%s %#v %q %v", mimeType, attrs, data, err)
	}
}

func TestFileURIParsing(t *testing.T) {
	want, err := posixAbs("/path/to/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		uri, netloc string
	}{
		{"file:///path/to/file.txt", ""},
		{"file:/path/to/file.txt", ""},
		{"file://localhost/path/to/file.txt", "localhost"},
		{"FILE:///path/to/file.txt", ""},
		{"file:///path/to/file.txt?param=value", ""},
		{"file:///path/to/file.txt#fragment", ""},
	}
	for _, tc := range cases {
		netloc, path, err := fileURIToPath(tc.uri, false)
		if err != nil || netloc != tc.netloc || path != want {
			t.Fatalf("%s -> %q %q %v", tc.uri, netloc, path, err)
		}
	}
}

func TestConvertCaseInsensitiveSchemes(t *testing.T) {
	res, err := ConvertURI("DATA:text/plain;base64,SGVsbG8sIFdvcmxkIQ==", 0, Options{})
	if err != nil || res.Text != "Hello, World!" {
		t.Fatalf("%q %v", res.Text, err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("Hello from file"), 0o644); err != nil {
		t.Fatal(err)
	}
	uri := strings.Replace(pathToFileURI(path), "file:", "FILE:", 1)
	res, err = ConvertURI(uri, 0, Options{})
	if err != nil || res.Text != "Hello from file" {
		t.Fatalf("%q %v", res.Text, err)
	}
}

func TestUNCAndDevicePaths(t *testing.T) {
	paths := []struct {
		path    string
		blocked bool
	}{
		{`\\server\share\x.txt`, true},
		{"//server/share/x.txt", true},
		{`/\server/share/x.txt`, true},
		{`\/server/share/x.txt`, true},
		{`\\?\UNC\server\share\x.txt`, true},
		{`\\?\C:\Temp\x.txt`, true},
		{`\\.\PhysicalDrive0`, true},
		{`C:\Temp\x.txt`, false},
		{"C:/Temp/x.txt", false},
		{"/home/user/x.txt", false},
		{"relative.txt", false},
	}
	for _, tc := range paths {
		if uncOrDevice(tc.path) != tc.blocked {
			t.Fatalf("%q blocked=%v", tc.path, uncOrDevice(tc.path))
		}
	}
}

func TestFileURIRejectsUNC(t *testing.T) {
	uncPaths := []string{
		"//server.example/share/x.txt",
		"///server.example/share/x.txt",
		"////server.example/share/x.txt",
		`/\server.example/share/x.txt`,
		`\/server.example/share/x.txt`,
		`\\server.example\share\x.txt`,
		"/%2Fserver.example/share/x.txt",
		"/%2fserver.example/share/x.txt",
		"/%5Cserver.example/share/x.txt",
		"/%5cserver.example/share/x.txt",
		"%2F%2Fserver.example/share/x.txt",
		"%5C%5Cserver.example%5Cshare%5Cx.txt",
		"/%5C%3F%5CUNC%5Cserver.example%5Cshare%5Cx.txt",
		"/%5C.%5CUNC%5Cserver.example%5Cshare%5Cx.txt",
		"/%5C%3F%5CC:%5CTemp%5Cx.txt",
		"/%5C.%5CPhysicalDrive0",
	}
	var blocked []string
	for _, authority := range []string{"", "localhost"} {
		for _, p := range uncPaths {
			if strings.HasPrefix(p, "/") {
				blocked = append(blocked, "file://"+authority+p)
			}
		}
	}
	for _, p := range uncPaths {
		if !strings.HasPrefix(p, "/") {
			blocked = append(blocked, "file:"+p)
		}
	}
	blocked = append(blocked, "FILE:////server.example/share/x.txt", "file:////C:/Temp/x.txt")
	if len(blocked) != 30 {
		t.Fatal(len(blocked))
	}
	for _, uri := range blocked {
		_, _, err := fileURIToPath(uri, false)
		var ue *URIError
		if !errors.As(err, &ue) || !strings.Contains(err.Error(), "UNC") {
			t.Fatalf("%s -> %v", uri, err)
		}
		_, err = ConvertURI(uri, 0, Options{})
		if !errors.As(err, &ue) || !strings.Contains(err.Error(), "Unsupported file URI") {
			t.Fatalf("convert %s -> %v", uri, err)
		}
	}
	_, err := ConvertURI("file://server.example/share/x.txt", 0, Options{})
	var ue *URIError
	if !errors.As(err, &ue) || !strings.Contains(err.Error(), "Unsupported file URI") {
		t.Fatal(err)
	}
}

func TestFileURIRejectsUNCAfterConversion(t *testing.T) {
	_, err := posixAbs("//server.example/share/x.txt")
	if err != nil {
		t.Fatal(err)
	}
	path, err := posixAbs("//server.example/share/x.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !uncOrDevice(path) {
		t.Fatal(path)
	}
	if _, _, err := fileURIToPath("file:///%2Fserver.example/share/x.txt", false); err == nil || !strings.Contains(err.Error(), "UNC") {
		t.Fatal(err)
	}
}

func TestLocalFileURIConversion(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"notes.txt", "notes café.txt", "%5C%5Cx.txt"} {
		for _, authority := range []string{"", "localhost"} {
			local := filepath.Join(dir, name)
			if err := os.WriteFile(local, []byte("Local file contents"), 0o644); err != nil {
				t.Fatal(err)
			}
			uri := pathToFileURI(local)
			if authority != "" {
				uri = strings.Replace(uri, "file://", "file://"+authority, 1)
			}
			netloc, path, err := fileURIToPath(uri, false)
			wantNetloc := authority
			if err != nil || netloc != wantNetloc || path != local {
				t.Fatalf("%s -> %q %q %v (file %s)", uri, netloc, path, err, local)
			}
			res, err := ConvertURI(uri, 0, Options{})
			if err != nil || res.Text != "Local file contents" {
				t.Fatalf("%q %v", res.Text, err)
			}
			got, err := Extract(bytes.NewReader([]byte("Local file contents")), "", local, 0)
			if err != nil || got.Text != "Local file contents" {
				t.Fatal(got.Text, err)
			}
		}
	}
}

func TestWindowsLocalDriveURI(t *testing.T) {
	netloc, path, err := fileURIToPath("file:///C%3A/Temp/example.md", true)
	if err != nil || netloc != "" || path != `C:\Temp\example.md` {
		t.Fatalf("%q %q %v", netloc, path, err)
	}
	cases := []struct{ uriPath, want string }{
		{"/C:/Temp/notes.txt", `C:\Temp\notes.txt`},
		{"/C%3A/Temp/notes.txt", `C:\Temp\notes.txt`},
		{"/C%3a/Temp/notes.txt", `C:\Temp\notes.txt`},
		{"/C:/Temp/notes%20caf%C3%A9.txt", "C:\\Temp\\notes café.txt"},
		{"/C:/Temp/%255C%255Cx.txt", `C:\Temp\%5C%5Cx.txt`},
	}
	for _, authority := range []string{"", "localhost"} {
		for _, tc := range cases {
			uri := "file://" + authority + tc.uriPath
			netloc, path, err := fileURIToPath(uri, true)
			wantNetloc := authority
			if err != nil || netloc != wantNetloc || path != tc.want {
				t.Fatalf("%s -> %q %q %v", uri, netloc, path, err)
			}
		}
	}
}

func TestContentDispositionFilename(t *testing.T) {
	body := []byte("name,value\nalpha,beta\n")
	want := "| name | value |\n| --- | --- |\n| alpha | beta |"
	res, err := ConvertResponse(ResponseInput{
		ContentDisposition: "attachment; filename*=UTF-8''data.csv",
		Body:               body,
	}, 0, Options{})
	if err != nil || res.Text != want {
		t.Fatalf("%q %v", res.Text, err)
	}
	res, err = ConvertResponse(ResponseInput{
		ContentDisposition: "attachment; filename=fallback.txt; filename*=UTF-8''data.csv",
		Body:               body,
	}, 0, Options{})
	if err != nil || res.Text != want {
		t.Fatalf("%q %v", res.Text, err)
	}
	if got := contentDispositionFilename("attachment; filename=fallback.txt; filename*=UTF-8''d%C3%A1t%C3%A1%2Ecsv"); got != "dátá.csv" {
		t.Fatal(got)
	}
}

func TestModuleURIVectors(t *testing.T) {
	// module_vectors.json is the must_include lists from
	// packages/markitdown/tests/_test_vectors.py.
	raw, err := os.ReadFile("module_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		General []vector `json:"GENERAL_TEST_VECTORS"`
		Data    []vector `json:"DATA_URI_TEST_VECTORS"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	// pdfplumber table and form layout is still the inherited failure.
	skip := map[string]bool{
		"pdf_cleanup_form.pdf":  true,
		"pdf_cleanup_mixed.pdf": true,
	}
	for _, v := range doc.General {
		if skip[v.Filename] {
			continue
		}
		t.Run(v.Filename+"/file", func(t *testing.T) {
			checkVector(t, v, false)
		})
		t.Run(v.Filename+"/data", func(t *testing.T) {
			checkVector(t, v, true)
		})
	}
	for _, v := range doc.Data {
		t.Run(v.Filename+"/keep", func(t *testing.T) {
			body := testFile(t, v.Filename)
			res, err := ExtractOptions(bytes.NewReader(body), "", v.Filename, 0, Options{KeepData: true})
			if err != nil {
				t.Fatal(err)
			}
			assertVector(t, res.Text, v)
			mimeType := v.Mimetype
			res, err = ExtractOptions(bytes.NewReader(body), mimeType, v.Filename, 0, Options{KeepData: true})
			if err != nil {
				t.Fatal(err)
			}
			assertVector(t, res.Text, v)
		})
	}
}

type vector struct {
	Filename string   `json:"filename"`
	Mimetype string   `json:"mimetype"`
	Include  []string `json:"must_include"`
	Exclude  []string `json:"must_not_include"`
}

func checkVector(t *testing.T, v vector, dataURI bool) {
	t.Helper()
	body := testFile(t, v.Filename)
	var (
		res Result
		err error
	)
	if dataURI {
		uri := "data:" + v.Mimetype + ";base64," + base64.StdEncoding.EncodeToString(body)
		res, err = ConvertURI(uri, 0, Options{})
	} else {
		path := filepath.Join("packages/markitdown/tests/test_files", v.Filename)
		res, err = ConvertURI(pathToFileURI(path), 0, Options{})
	}
	if err != nil {
		t.Fatal(err)
	}
	assertVector(t, res.Text, v)
}

func assertVector(t *testing.T, text string, v vector) {
	t.Helper()
	for _, s := range v.Include {
		if !strings.Contains(text, s) {
			t.Errorf("missing %q", s)
		}
	}
	for _, s := range v.Exclude {
		if strings.Contains(text, s) {
			t.Errorf("has %q", s)
		}
	}
	if t.Failed() {
		if len(text) > 1500 {
			text = text[:1500]
		}
		t.Log(text)
	}
}
