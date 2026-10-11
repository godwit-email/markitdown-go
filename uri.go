package markitdown

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

// URIError is a file or data URI the client supplied, restated without a
// resolved path beyond what the URI itself contains.
type URIError struct{ Msg string }

func (e *URIError) Error() string { return e.Msg }

// HTTPStatusError is a failed HTTP fetch. Detail may name the host; the MCP
// tool result must not.
type HTTPStatusError struct {
	Status int
	Detail string
}

func (e *HTTPStatusError) Error() string { return e.Detail }

// FetchError is a transport failure. Detail may name addresses; the MCP tool
// result must not.
type FetchError struct{ Detail string }

func (e *FetchError) Error() string { return e.Detail }

// ConversionAttempt is one converter that accepted the input and then failed.
type ConversionAttempt struct {
	Converter string
	Err       error
	Stack     []uintptr
}

// FileConversionError aggregates converter failures. The client-facing MCP
// message is the fixed sentence; attempts stay on the error.
type FileConversionError struct {
	Attempts []ConversionAttempt
}

func (e *FileConversionError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "File conversion failed after %d attempts:\n", len(e.Attempts))
	for _, a := range e.Attempts {
		fmt.Fprintf(&b, " - %s threw %T with message: %v\n", a.Converter, a.Err, a.Err)
	}
	return b.String()
}

// ConvertURI reads a file: or data: URI and extracts it. http and https are
// not fetched.
func ConvertURI(uri string, maxOut int, opt Options) (Result, error) {
	uri = strings.TrimSpace(uri)
	scheme, _, _, ok := splitURL(uri)
	if !ok || (scheme != "file" && scheme != "data" && scheme != "http" && scheme != "https") {
		return Result{}, &URIError{Msg: fmt.Sprintf(
			"Unsupported URI scheme: %s. Supported schemes are: file:, data:, http:, https:",
			schemeToken(uri),
		)}
	}
	switch scheme {
	case "file":
		netloc, path, err := fileURIToPath(uri, runtime.GOOS == "windows")
		if err != nil {
			return Result{}, err
		}
		if netloc != "" && !strings.EqualFold(netloc, "localhost") {
			return Result{}, &URIError{Msg: fmt.Sprintf(
				"Unsupported file URI: %s. Netloc must be empty or localhost.", uri,
			)}
		}
		f, err := os.Open(path)
		if err != nil {
			return Result{}, err
		}
		defer f.Close()
		data, err := readLimited(f)
		if err != nil {
			return Result{}, err
		}
		return extract(bytes.NewReader(data), charsetMIME(data, "", path), path, opt, maxOut)
	case "data":
		mt, attrs, data, err := parseDataURI(uri)
		if err != nil {
			return Result{}, err
		}
		mimeType := mt
		if cs := attrs["charset"]; cs != "" {
			if mimeType == "" {
				mimeType = "application/octet-stream; charset=" + cs
			} else {
				mimeType += "; charset=" + cs
			}
		}
		return extract(bytes.NewReader(data), charsetMIME(data, mimeType, ""), "", opt, maxOut)
	default:
		return Result{}, &FetchError{Detail: "network fetch is not available"}
	}
}

// ResponseInput is the part of an HTTP response ConvertResponse reads.
type ResponseInput struct {
	ContentType        string
	ContentDisposition string
	URL                string
	Body               []byte
}

// ConvertResponse extracts a response body, preferring an RFC 5987 filename.
func ConvertResponse(in ResponseInput, maxOut int, opt Options) (Result, error) {
	mt, params, err := mime.ParseMediaType(strings.TrimSpace(in.ContentType))
	mimeType := ""
	if err == nil && mt != "" {
		mimeType = mt
		if cs := params["charset"]; cs != "" {
			mimeType += "; charset=" + cs
		}
	}
	filename := contentDispositionFilename(in.ContentDisposition)
	if filename == "" && in.URL != "" {
		if u, err := url.Parse(in.URL); err == nil {
			base := path.Base(u.Path)
			if base != "." && base != "/" && path.Ext(base) != "" {
				filename = base
			}
		}
	}
	return extract(bytes.NewReader(in.Body), mimeType, filename, opt, maxOut)
}

func charsetMIME(data []byte, mimeType, filename string) string {
	if _, params, err := mime.ParseMediaType(strings.TrimSpace(mimeType)); err == nil && params["charset"] != "" {
		return mimeType
	}
	kind, _, ok := detect(mimeType, filename)
	if !ok || (kind != KindText && kind != KindCSV && kind != KindHTML && kind != KindRSS && kind != KindIPYNB) {
		return mimeType
	}
	if len(data) == 0 || utf8.Valid(data) || !looksLikeCP932(data) {
		return mimeType
	}
	if strings.TrimSpace(mimeType) == "" || strings.EqualFold(strings.TrimSpace(mimeType), "application/octet-stream") {
		return "application/octet-stream; charset=cp932"
	}
	return mimeType + "; charset=cp932"
}

func looksLikeCP932(data []byte) bool {
	out, _, err := transform.Bytes(japanese.ShiftJIS.NewDecoder(), data)
	if err != nil || bytes.ContainsRune(out, '\uFFFD') {
		return false
	}
	for _, r := range string(out) {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) {
			return true
		}
	}
	return false
}

func schemeToken(uri string) string {
	if i := strings.IndexByte(uri, ':'); i >= 0 {
		return uri[:i]
	}
	return uri
}

func splitURL(uri string) (scheme, netloc, path string, ok bool) {
	i := strings.IndexByte(uri, ':')
	if i <= 0 {
		return "", "", uri, false
	}
	for _, c := range uri[:i] {
		if !isSchemeChar(c) {
			return "", "", uri, false
		}
	}
	scheme = strings.ToLower(uri[:i])
	rest := uri[i+1:]
	if strings.HasPrefix(rest, "//") {
		rest = rest[2:]
		j := strings.IndexAny(rest, "/?#")
		if j < 0 {
			netloc = rest
			rest = ""
		} else {
			netloc = rest[:j]
			rest = rest[j:]
		}
	}
	if j := strings.IndexAny(rest, "?#"); j >= 0 {
		rest = rest[:j]
	}
	return scheme, netloc, rest, true
}

func isSchemeChar(c rune) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.'
}

func fileURIToPath(uri string, windows bool) (string, string, error) {
	scheme, netloc, p, ok := splitURL(uri)
	if !ok || scheme != "file" {
		return "", "", &URIError{Msg: "Not a file URL: " + uri}
	}
	decoded := unquoteToBytes(p)
	for i, c := range decoded {
		if c == '\\' {
			decoded[i] = '/'
		}
	}
	if bytes.HasPrefix(decoded, []byte("//")) {
		return "", "", uncURI(uri)
	}
	var path string
	if windows {
		var err error
		path, err = windowsURL2Path(p)
		if err != nil {
			return "", "", err
		}
		if len(path) >= 3 && (path[0] == '/' || path[0] == '\\') && path[2] == ':' {
			path = path[1:]
		}
	} else {
		var err error
		path, err = posixAbs(pathUnquote(p))
		if err != nil {
			return "", "", err
		}
	}
	if uncOrDevice(path) {
		return "", "", uncURI(uri)
	}
	if netloc == "" {
		return "", path, nil
	}
	return netloc, path, nil
}

func uncURI(uri string) error {
	return &URIError{Msg: "Unsupported file URI: " + uri + ". UNC and Windows device paths are not supported."}
}

func uncOrDevice(p string) bool {
	drive, _ := splitDrive(p)
	return strings.HasPrefix(strings.ReplaceAll(drive, `\`, "/"), "//")
}

func splitDrive(p string) (string, string) {
	norm := strings.ReplaceAll(p, "/", `\`)
	if strings.HasPrefix(norm, `\`) {
		if len(norm) > 1 && norm[1] == '\\' {
			start := 2
			if len(norm) >= 8 && strings.EqualFold(norm[:8], `\\?\UNC\`) {
				start = 8
			}
			index := strings.Index(norm[start:], `\`)
			if index < 0 {
				return p, ""
			}
			index += start
			index2 := strings.Index(norm[index+1:], `\`)
			if index2 < 0 {
				return p, ""
			}
			index2 += index + 1
			return p[:index2], p[index2:]
		}
		return "", p
	}
	if len(norm) >= 2 && norm[1] == ':' {
		return p[:2], p[2:]
	}
	return "", p
}

func posixAbs(p string) (string, error) {
	if !strings.HasPrefix(p, "/") {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		p = cwd + "/" + p
	}
	two := strings.HasPrefix(p, "//") && !strings.HasPrefix(p, "///")
	cleaned := path.Clean(p)
	if two && strings.HasPrefix(cleaned, "/") && !strings.HasPrefix(cleaned, "//") {
		cleaned = "/" + cleaned
	}
	return cleaned, nil
}

func windowsURL2Path(urlPath string) (string, error) {
	u := strings.ReplaceAll(urlPath, ":", "|")
	if !strings.Contains(u, "|") {
		if strings.HasPrefix(u, "////") {
			u = u[2:]
		}
		parts := strings.Split(u, "/")
		for i, part := range parts {
			parts[i] = pathUnquote(part)
		}
		return strings.Join(parts, `\`), nil
	}
	comp := strings.SplitN(u, "|", 2)
	if len(comp) != 2 || comp[0] == "" || !isASCIILetter(comp[0][len(comp[0])-1]) {
		return "", &URIError{Msg: "Bad URL: " + u}
	}
	drive := strings.ToUpper(comp[0][len(comp[0])-1:])
	out := drive + ":"
	for _, part := range strings.Split(comp[1], "/") {
		if part != "" {
			out += `\` + pathUnquote(part)
		}
	}
	if strings.HasSuffix(out, ":") && strings.HasSuffix(urlPath, "/") {
		out += `\`
	}
	return out, nil
}

func isASCIILetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func parseDataURI(uri string) (string, map[string]string, []byte, error) {
	if len(uri) < 5 || !strings.EqualFold(uri[:5], "data:") {
		return "", nil, nil, &URIError{Msg: "Not a data URI"}
	}
	header, payload, ok := strings.Cut(uri, ",")
	if !ok {
		return "", nil, nil, &URIError{Msg: "Malformed data URI, missing ',' separator"}
	}
	parts := strings.Split(header[5:], ";")
	b64 := false
	if len(parts) > 0 && strings.EqualFold(parts[len(parts)-1], "base64") {
		parts = parts[:len(parts)-1]
		b64 = true
	}
	mimeType := ""
	if len(parts) > 0 && parts[0] != "" {
		mimeType = parts[0]
		parts = parts[1:]
	}
	attrs := map[string]string{}
	for _, part := range parts {
		if part == "" {
			continue
		}
		if k, v, ok := strings.Cut(part, "="); ok {
			attrs[strings.ToLower(k)] = v
		} else {
			attrs[strings.ToLower(part)] = ""
		}
	}
	if b64 {
		data, err := decodeB64(payload)
		if err != nil {
			return "", nil, nil, err
		}
		return mimeType, attrs, data, nil
	}
	return mimeType, attrs, unquoteToBytes(payload), nil
}

func decodeB64(s string) ([]byte, error) {
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\n', '\r', '\t':
			return -1
		default:
			return r
		}
	}, s)
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	return base64.StdEncoding.DecodeString(s)
}

func unquoteToBytes(s string) []byte {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if b, err := hex.DecodeString(s[i+1 : i+3]); err == nil {
				out = append(out, b...)
				i += 2
				continue
			}
		}
		out = append(out, s[i])
	}
	return out
}

func pathUnquote(s string) string { return string(unquoteToBytes(s)) }

func contentDispositionFilename(header string) string {
	fallback, extended := "", ""
	for _, part := range splitParams(header) {
		name, val, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		switch name {
		case "filename*":
			if extended == "" {
				extended = decodeRFC5987(val)
			}
		case "filename":
			if fallback == "" {
				fallback = trimQuotes(strings.TrimSpace(val))
			}
		}
	}
	if extended != "" {
		return extended
	}
	return fallback
}

func splitParams(header string) []string {
	var parts []string
	var b strings.Builder
	quote := false
	for i := 0; i < len(header); i++ {
		c := header[i]
		if c == '"' {
			quote = !quote
			b.WriteByte(c)
			continue
		}
		if c == ';' && !quote {
			parts = append(parts, strings.TrimSpace(b.String()))
			b.Reset()
			continue
		}
		b.WriteByte(c)
	}
	if strings.TrimSpace(b.String()) != "" {
		parts = append(parts, strings.TrimSpace(b.String()))
	}
	return parts
}

func decodeRFC5987(val string) string {
	val = trimQuotes(strings.TrimSpace(val))
	charset, rest, ok := strings.Cut(val, "'")
	if !ok {
		return pathUnquote(val)
	}
	_, encoded, ok := strings.Cut(rest, "'")
	if !ok {
		return pathUnquote(val)
	}
	raw := unquoteToBytes(encoded)
	if enc := findEncoding(charset); enc != nil && !isUTF8Charset(charset) && !strings.EqualFold(charset, "utf-8") {
		if out, err := io.ReadAll(enc.NewDecoder().Reader(bytes.NewReader(raw))); err == nil {
			return string(out)
		}
	}
	return string(raw)
}

func trimQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
