package markitdown

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const expectedMarkdown = "# Hello\n\nA **markitdown** fixture."

func TestLegacyInitializeHandshake(t *testing.T) {
	uri := writeFixtureURI(t)
	responses := exchange(t, &MCP{}, []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"convert_to_markdown","arguments":{"uri":"` + uri + `"}}}`,
	})
	info := responses[0]["result"].(map[string]any)["serverInfo"].(map[string]any)
	if info["name"] != "markitdown" {
		t.Fatal(info)
	}
	text, isErr := toolText(t, responses[1])
	if isErr || !strings.Contains(text, expectedMarkdown) {
		t.Fatalf("%v %q", isErr, text)
	}
}

func TestModernDiscoverHandshake(t *testing.T) {
	uri := writeFixtureURI(t)
	responses := exchange(t, &MCP{}, []string{
		`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"convert_to_markdown","arguments":{"uri":"` + uri + `"},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`,
	})
	versions := responses[0]["result"].(map[string]any)["supportedVersions"].([]any)
	found := false
	for _, v := range versions {
		if v == "2026-07-28" {
			found = true
		}
	}
	if !found {
		t.Fatal(versions)
	}
	text, isErr := toolText(t, responses[1])
	if isErr || !strings.Contains(text, expectedMarkdown) {
		t.Fatalf("%v %q", isErr, text)
	}
}

func TestUnknownMethodDoesNotKillTheServer(t *testing.T) {
	responses := exchange(t, &MCP{}, []string{
		`{"jsonrpc":"2.0","id":1,"method":"totally/bogus","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`,
	})
	if responses[0]["error"] == nil {
		t.Fatal(responses[0])
	}
	info := responses[1]["result"].(map[string]any)["serverInfo"].(map[string]any)
	if info["name"] != "markitdown" {
		t.Fatal(info)
	}
}

func TestConversionErrorsAreReported(t *testing.T) {
	uri := writeFixtureURI(t)
	missing := filepath.Join(t.TempDir(), "missing.md")
	failures := []struct{ uri, diagnosis string }{
		{pathToFileURI(missing), "No such file or directory"},
		{"ftp://example.com/sample.md", "Unsupported URI scheme: ftp"},
		{"data:text/plain", "Malformed data URI, missing ',' separator"},
	}
	for _, modern := range []bool{false, true} {
		var reqs []string
		if modern {
			reqs = append(reqs, `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{}}`)
		} else {
			reqs = append(reqs,
				`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`,
				`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
			)
		}
		id := 2
		for _, f := range failures {
			reqs = append(reqs, toolCall(id, f.uri))
			id++
		}
		reqs = append(reqs, toolCall(id, uri))
		responses := exchange(t, &MCP{}, reqs)
		if len(responses) != len(failures)+2 {
			t.Fatal(len(responses))
		}
		for i, f := range failures {
			text, isErr := toolText(t, responses[i+1])
			if !isErr || !strings.Contains(text, f.diagnosis) {
				t.Fatalf("modern %v %q -> %v %q", modern, f.uri, isErr, text)
			}
			if strings.Contains(text, missing) {
				t.Fatalf("path leaked: %s", text)
			}
		}
		text, isErr := toolText(t, responses[len(responses)-1])
		if isErr || !strings.Contains(text, expectedMarkdown) {
			t.Fatalf("%v %q", isErr, text)
		}
	}
}

func TestClientInputFailuresReachTheSDK(t *testing.T) {
	bin := pathToFileURI("packages/markitdown/tests/test_files/random.bin")
	cases := []struct{ uri, message string }{
		{bin, "No converter attempted a conversion"},
		{"gopher://example.com/sample.pdf", "Unsupported URI scheme: gopher. Supported schemes are: file:, data:, http:, https:"},
	}
	for _, tc := range cases {
		_, err := (&MCP{}).call(tc.uri)
		var te *ToolError
		if !errors.As(err, &te) || te.Unexpected {
			t.Fatal(tc.uri, err)
		}
		if !strings.Contains(te.Error(), tc.message) {
			t.Fatal(te)
		}
		cause := rootCause(err)
		if !strings.Contains(te.Error(), cause.Error()) {
			t.Fatalf("cause %q not in %q", cause, te)
		}
	}
}

func TestHTTPStatusIsReturnedWithoutTransportDetails(t *testing.T) {
	raw := &HTTPStatusError{Status: 404, Detail: "404 Client Error: Not Found for url: https://internal.corp/sample.pdf"}
	m := &MCP{Convert: func(string) (string, error) { return "", raw }}
	_, err := m.call("https://example.com/sample.pdf")
	var te *ToolError
	if !errors.As(err, &te) || te.Unexpected {
		t.Fatal(err)
	}
	if !strings.Contains(te.Error(), "HTTP status 404") || strings.Contains(te.Error(), "internal.corp") {
		t.Fatal(te)
	}
	if rootCause(err) != raw {
		t.Fatal(rootCause(err))
	}
}

func TestNetworkFailureDetailsRemainPrivate(t *testing.T) {
	for _, detail := range []string{
		"HTTPSConnectionPool(host='10.0.0.5', port=443): Max retries exceeded (Caused by ProxyError('Unable to connect to proxy', NameResolutionError(\"Failed to resolve 'proxy.internal.corp'\")))",
		"HTTPSConnectionPool(host='10.0.0.5', port=443): Read timed out. (proxy.internal.corp)",
	} {
		raw := &FetchError{Detail: detail}
		m := &MCP{Convert: func(string) (string, error) { return "", raw }}
		_, err := m.call("https://example.com/sample.pdf")
		var te *ToolError
		if !errors.As(err, &te) || te.Unexpected {
			t.Fatal(err)
		}
		if !strings.Contains(te.Error(), "Could not fetch the resource") || strings.Contains(te.Error(), "10.0.0.5") || strings.Contains(te.Error(), "proxy.internal.corp") {
			t.Fatal(te)
		}
		if !strings.Contains(rootCause(err).Error(), "proxy.internal.corp") {
			t.Fatal(rootCause(err))
		}
	}
}

func TestLocalReadFailureReportsErrnoWithoutThePath(t *testing.T) {
	raw := &os.PathError{Op: "open", Path: "/srv/private/sample.pdf", Err: os.ErrPermission}
	m := &MCP{Convert: func(string) (string, error) { return "", raw }}
	_, err := m.call("file:///srv/private/sample.pdf")
	var te *ToolError
	if !errors.As(err, &te) || te.Unexpected {
		t.Fatal(err)
	}
	if !strings.Contains(te.Error(), "Permission denied") || strings.Contains(te.Error(), "/srv/private/sample.pdf") {
		t.Fatal(te)
	}
	if !strings.Contains(rootCause(err).Error(), "/srv/private/sample.pdf") {
		t.Fatal(rootCause(err))
	}
}

func TestUnexpectedFailureDetailsRemainPrivate(t *testing.T) {
	m := &MCP{Convert: func(string) (string, error) {
		return "", errors.New("internal implementation detail")
	}}
	_, err := m.call("file:///sample.pdf")
	var te *ToolError
	if !errors.As(err, &te) || !te.Unexpected {
		t.Fatal(err)
	}
	if strings.Contains(te.Error(), "internal implementation detail") {
		t.Fatal(te)
	}
}

func TestWrappedConverterCrashRetainsTheCause(t *testing.T) {
	private := "private converter implementation detail"
	for _, err := range []error{
		errors.New(private),
		&URIError{Msg: private},
		&os.PathError{Op: "read", Path: "x", Err: errors.New(private)},
	} {
		raised := err
		m := &MCP{Converters: []Converter{{
			Name:    "CrashingConverter",
			Convert: func(string) (string, error) { return "", raised },
		}}}
		_, got := m.call("data:text/plain,example")
		var te *ToolError
		if !errors.As(got, &te) || te.Unexpected || !strings.Contains(te.Error(), "File conversion failed") || strings.Contains(te.Error(), private) {
			t.Fatal(got)
		}
		cause, ok := rootCause(got).(*FileConversionError)
		if !ok || len(cause.Attempts) != 1 {
			t.Fatal(rootCause(got))
		}
		if cause.Attempts[0].Err.Error() != raised.Error() && !errors.Is(cause.Attempts[0].Err, raised) {
			// PathError.Error includes the op and path plus the inner message.
			if !strings.Contains(cause.Attempts[0].Err.Error(), private) {
				t.Fatal(cause.Attempts[0].Err)
			}
		}
		if len(cause.Attempts[0].Stack) == 0 {
			t.Fatal("missing stack")
		}
		if cause.Attempts[0].Converter != "CrashingConverter" {
			t.Fatal(cause.Attempts[0].Converter)
		}
	}
}

func TestWrappedConverterCrashRemainsPrivateOverHTTP(t *testing.T) {
	private := "private converter implementation detail"
	for _, name := range []string{"RuntimeError", "ValueError", "OSError"} {
		m := &MCP{Converters: []Converter{{
			Name: "CrashingConverter",
			Convert: func(string) (string, error) {
				return "", errors.New(private)
			},
		}}}
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"convert_to_markdown","arguments":{"uri":"data:text/plain,example"}}}`))
		rec := httptest.NewRecorder()
		m.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatal(rec.Code, rec.Body)
		}
		var msg map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &msg); err != nil {
			t.Fatal(err)
		}
		text, isErr := toolText(t, msg)
		body := rec.Body.String()
		if !isErr || !strings.Contains(text, "File conversion failed") || strings.Contains(body, private) || strings.Contains(body, name) || strings.Contains(body, "CrashingConverter") {
			t.Fatalf("%s %s", name, body)
		}
	}
}

func TestStreamableHTTPInitializes(t *testing.T) {
	m := &MCP{}
	init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`
	rec := postMCP(t, m, "/mcp", init)
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatal(rec.Code, rec.Header())
	}
	if rec.Header().Get("mcp-session-id") != "" {
		t.Fatal("session id set")
	}
	var msg map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &msg); err != nil {
		t.Fatal(err)
	}
	if msg["id"].(float64) != 1 {
		t.Fatal(msg["id"])
	}
	info := msg["result"].(map[string]any)["serverInfo"].(map[string]any)
	if info["name"] != "markitdown" {
		t.Fatal(info)
	}
	note := postMCP(t, m, "/mcp", `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if note.Code != 202 {
		t.Fatal(note.Code)
	}
	conv := postMCP(t, m, "/mcp", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"convert_to_markdown","arguments":{"uri":"data:text/plain,Hello%20from%20HTTP"}}}`)
	if conv.Code != 200 {
		t.Fatal(conv.Code, conv.Body)
	}
	if err := json.Unmarshal(conv.Body.Bytes(), &msg); err != nil {
		t.Fatal(err)
	}
	text, isErr := toolText(t, msg)
	if isErr || text != "Hello from HTTP" || msg["id"].(float64) != 2 {
		t.Fatalf("%v %q %v", isErr, text, msg["id"])
	}
}

func TestSSESessionUsesPreservedMessageRoute(t *testing.T) {
	m := &MCP{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := &flushBody{ch: make(chan struct{}, 8)}
	req := httptest.NewRequest(http.MethodGet, "/sse", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		m.ServeHTTP(body, req)
		close(done)
	}()
	text := body.wait(t, "event: endpoint")
	if body.code != 200 || !strings.HasPrefix(body.header.Get("Content-Type"), "text/event-stream") {
		t.Fatal(body.code, body.header)
	}
	ev := parseSSE(text)
	if ev[0].event != "endpoint" {
		t.Fatal(ev)
	}
	u, err := url.Parse(ev[0].data)
	if err != nil || u.Path != "/messages/" || u.Query().Get("session_id") == "" {
		t.Fatal(ev[0].data)
	}
	initRec := postMCP(t, m, ev[0].data, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`)
	if initRec.Code != 202 {
		t.Fatal(initRec.Code)
	}
	more := body.wait(t, "event: message")
	ev = parseSSE(more)
	var init map[string]any
	if err := json.Unmarshal([]byte(ev[len(ev)-1].data), &init); err != nil {
		t.Fatal(err, more)
	}
	if init["id"].(float64) != 1 || init["result"].(map[string]any)["serverInfo"].(map[string]any)["name"] != "markitdown" {
		t.Fatal(init)
	}
	note := postMCP(t, m, evEndpoint(more), `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if note.Code != 202 {
		t.Fatal(note.Code)
	}
	before := body.snapshot()
	conv := postMCP(t, m, evEndpoint(more), `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"convert_to_markdown","arguments":{"uri":"data:text/plain,Hello%20from%20HTTP"}}}`)
	if conv.Code != 202 {
		t.Fatal(conv.Code)
	}
	after := body.waitAfter(t, before, "event: message")
	ev = parseSSE(after[len(before):])
	var got map[string]any
	if err := json.Unmarshal([]byte(ev[len(ev)-1].data), &got); err != nil {
		t.Fatal(err, after)
	}
	gotText, isErr := toolText(t, got)
	if isErr || gotText != "Hello from HTTP" {
		t.Fatalf("%v %q", isErr, gotText)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sse did not return")
	}
}

type sseEvent struct{ event, data string }

func parseSSE(s string) []sseEvent {
	var out []sseEvent
	var cur sseEvent
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if cur.event != "" || cur.data != "" {
				out = append(out, cur)
				cur = sseEvent{}
			}
			continue
		}
		if name, val, ok := strings.Cut(line, ":"); ok {
			val = strings.TrimPrefix(val, " ")
			switch name {
			case "event":
				cur.event = val
			case "data":
				if cur.data != "" {
					cur.data += "\n"
				}
				cur.data += val
			}
		}
	}
	if cur.event != "" || cur.data != "" {
		out = append(out, cur)
	}
	return out
}

func evEndpoint(s string) string {
	for _, ev := range parseSSE(s) {
		if ev.event == "endpoint" {
			return ev.data
		}
	}
	return ""
}

type flushBody struct {
	header http.Header
	code   int
	buf    bytes.Buffer
	mu     sync.Mutex
	ch     chan struct{}
}

func (f *flushBody) Header() http.Header {
	if f.header == nil {
		f.header = http.Header{}
	}
	return f.header
}
func (f *flushBody) WriteHeader(code int) { f.code = code }
func (f *flushBody) Write(p []byte) (int, error) {
	f.mu.Lock()
	n, err := f.buf.Write(p)
	f.mu.Unlock()
	select {
	case f.ch <- struct{}{}:
	default:
	}
	return n, err
}
func (f *flushBody) Flush() {}
func (f *flushBody) snapshot() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.buf.String()
}
func (f *flushBody) wait(t *testing.T, sub string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s := f.snapshot(); strings.Contains(s, sub) {
			return s
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("missing %q in %q", sub, f.snapshot())
	return ""
}
func (f *flushBody) waitAfter(t *testing.T, before, sub string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s := f.snapshot()
		if len(s) > len(before) && strings.Contains(s[len(before):], sub) {
			return s
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("missing %q after %d bytes in %q", sub, len(before), f.snapshot())
	return ""
}

func postMCP(t *testing.T, m *MCP, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)
	return rec
}

func exchange(t *testing.T, m *MCP, reqs []string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, req := range reqs {
		resp, notification := m.Handle([]byte(req))
		if notification {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal(resp, &msg); err != nil {
			t.Fatal(err, string(resp))
		}
		out = append(out, msg)
	}
	sort.Slice(out, func(i, j int) bool { return idNum(out[i]["id"]) < idNum(out[j]["id"]) })
	return out
}

func idNum(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	default:
		return 0
	}
}

func toolText(t *testing.T, msg map[string]any) (string, bool) {
	t.Helper()
	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %#v", msg)
	}
	content := result["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	return text, result["isError"].(bool)
}

func toolCall(id int, uri string) string {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "convert_to_markdown",
			"arguments": map[string]any{"uri": uri},
		},
	})
	return string(body)
}

func writeFixtureURI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.md")
	if err := os.WriteFile(path, []byte(expectedMarkdown+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return pathToFileURI(path)
}

func pathToFileURI(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	var b strings.Builder
	b.WriteString("file://")
	for _, part := range strings.Split(filepath.ToSlash(abs), "/") {
		if part == "" {
			continue
		}
		b.WriteByte('/')
		b.WriteString(url.PathEscape(part))
	}
	return b.String()
}

func rootCause(err error) error {
	for {
		var te *ToolError
		if !errors.As(err, &te) || te.Err == nil {
			return err
		}
		err = te.Err
	}
}
