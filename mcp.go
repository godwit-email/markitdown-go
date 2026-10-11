package markitdown

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
)

const unsupportedMsg = "Could not convert stream to Markdown. No converter attempted a conversion, suggesting that the filetype is simply not supported."

// ToolError is the client-visible tool failure. Unexpected errors keep their
// cause but the message does not include it.
type ToolError struct {
	Msg        string
	Err        error
	Unexpected bool
}

func (e *ToolError) Error() string { return e.Msg }
func (e *ToolError) Unwrap() error { return e.Err }

type unsupportedErr struct{ msg string }

func (e unsupportedErr) Error() string { return e.msg }
func (e unsupportedErr) Unwrap() error { return ErrUnsupported }

// Converter is one URI converter tried by an MCP server before the built-in
// path. A failure is aggregated into FileConversionError.
type Converter struct {
	Name    string
	Convert func(uri string) (string, error)
}

// MCP is an in-process MarkItDown tool server. It does not listen.
type MCP struct {
	Convert    func(uri string) (string, error)
	Converters []Converter

	mu       sync.Mutex
	sessions map[string]chan []byte
}

// Handle answers one JSON-RPC message. Notifications produce no response.
func (m *MCP) Handle(line []byte) (resp []byte, notification bool) {
	var req struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(line, &req); err != nil {
		return rpcError(json.RawMessage("null"), -32700, "Parse error"), false
	}
	if len(req.ID) == 0 || string(req.ID) == "null" {
		return nil, true
	}
	switch req.Method {
	case "initialize":
		version := "2025-06-18"
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &params)
		if params.ProtocolVersion != "" {
			version = params.ProtocolVersion
		}
		return rpcResult(req.ID, map[string]any{
			"protocolVersion": version,
			"serverInfo":      map[string]any{"name": "markitdown", "version": "0.0.0"},
			"capabilities":    map[string]any{},
		}), false
	case "server/discover":
		return rpcResult(req.ID, map[string]any{
			"supportedVersions": []string{"2025-06-18", "2026-07-28"},
		}), false
	case "tools/call":
		return rpcResult(req.ID, m.toolResult(req.Params)), false
	default:
		return rpcError(req.ID, -32601, "Method not found"), false
	}
}

func (m *MCP) toolResult(params json.RawMessage) map[string]any {
	var call struct {
		Name      string `json:"name"`
		Arguments struct {
			URI string `json:"uri"`
		} `json:"arguments"`
	}
	_ = json.Unmarshal(params, &call)
	text, err := m.call(call.Arguments.URI)
	if err != nil {
		msg := "An unexpected error occurred."
		var te *ToolError
		if errors.As(err, &te) {
			msg = te.Msg
		}
		return map[string]any{
			"content": []any{map[string]any{"type": "text", "text": msg}},
			"isError": true,
		}
	}
	return map[string]any{
		"content": []any{map[string]any{"type": "text", "text": text}},
		"isError": false,
	}
}

func (m *MCP) call(uri string) (string, error) {
	text, err := m.convert(uri)
	if err != nil {
		return "", sanitizeTool(err)
	}
	return text, nil
}

func (m *MCP) convert(uri string) (string, error) {
	if len(m.Converters) > 0 {
		var attempts []ConversionAttempt
		for _, c := range m.Converters {
			text, err := c.Convert(uri)
			if err == nil {
				return text, nil
			}
			pcs := make([]uintptr, 16)
			n := runtime.Callers(2, pcs)
			attempts = append(attempts, ConversionAttempt{Converter: c.Name, Err: err, Stack: pcs[:n]})
		}
		return "", &FileConversionError{Attempts: attempts}
	}
	if m.Convert != nil {
		return m.Convert(uri)
	}
	res, err := ConvertURI(uri, 0, Options{})
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

func sanitizeTool(err error) error {
	var te *ToolError
	if errors.As(err, &te) {
		return te
	}
	var httpErr *HTTPStatusError
	if errors.As(err, &httpErr) {
		status := "unknown"
		if httpErr.Status != 0 {
			status = fmt.Sprintf("%d", httpErr.Status)
		}
		return &ToolError{Msg: "Fetching the resource failed with HTTP status " + status + ".", Err: httpErr}
	}
	var fetch *FetchError
	if errors.As(err, &fetch) {
		return &ToolError{Msg: "Could not fetch the resource.", Err: fetch}
	}
	var conv *FileConversionError
	if errors.As(err, &conv) {
		return &ToolError{Msg: "File conversion failed.", Err: conv}
	}
	var pe *os.PathError
	if errors.As(err, &pe) || errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
		return &ToolError{Msg: "Could not read the resource: " + osStrerror(err) + ".", Err: err}
	}
	var uriErr *URIError
	if errors.As(err, &uriErr) {
		return &ToolError{Msg: uriErr.Error(), Err: uriErr}
	}
	if errors.Is(err, ErrUnsupported) {
		return &ToolError{Msg: unsupportedMsg, Err: unsupportedErr{unsupportedMsg}}
	}
	return &ToolError{Unexpected: true, Msg: "An unexpected error occurred.", Err: err}
}

func osStrerror(err error) string {
	if errors.Is(err, os.ErrNotExist) {
		return "No such file or directory"
	}
	if errors.Is(err, os.ErrPermission) {
		return "Permission denied"
	}
	var pe *os.PathError
	if errors.As(err, &pe) {
		if errno, ok := pe.Err.(syscall.Errno); ok {
			switch errno {
			case syscall.ENOENT:
				return "No such file or directory"
			case syscall.EACCES, syscall.EPERM:
				return "Permission denied"
			default:
				s := errno.Error()
				if s == "" {
					break
				}
				return strings.ToUpper(s[:1]) + s[1:]
			}
		}
	}
	return "the resource could not be read"
}

func rpcResult(id json.RawMessage, result any) []byte {
	body, _ := json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  any             `json:"result"`
	}{"2.0", id, result})
	return body
}

func rpcError(id json.RawMessage, code int, message string) []byte {
	body, _ := json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   map[string]any  `json:"error"`
	}{"2.0", id, map[string]any{"code": code, "message": message}})
	return body
}

// ServeHTTP is the streamable HTTP and SSE app. It does not open a socket.
func (m *MCP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/mcp":
		m.streamable(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/sse":
		m.sse(w, r)
	case r.Method == http.MethodPost && (r.URL.Path == "/messages/" || r.URL.Path == "/messages"):
		m.ssePost(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (m *MCP) streamable(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	resp, notification := m.Handle(body)
	if notification {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp)
}

func (m *MCP) sse(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	id := newSessionID()
	ch := make(chan []byte, 8)
	m.mu.Lock()
	if m.sessions == nil {
		m.sessions = map[string]chan []byte{}
	}
	m.sessions[id] = ch
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.sessions, id)
		m.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "event: endpoint\ndata: /messages/?session_id=%s\n\n", id)
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

func (m *MCP) ssePost(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("session_id")
	m.mu.Lock()
	ch := m.sessions[id]
	m.mu.Unlock()
	if ch == nil {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	resp, notification := m.Handle(body)
	w.WriteHeader(http.StatusAccepted)
	if notification {
		return
	}
	select {
	case ch <- resp:
	case <-r.Context().Done():
	}
}

func newSessionID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
