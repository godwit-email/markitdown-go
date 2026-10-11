// Package markitdown extracts text from already-downloaded attachments.
// It does not fetch URLs and it does not take an LLM client.
package markitdown

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"path"
	"strings"
	"unicode/utf8"
)

const (
	maxInput           = 25 << 20
	defaultMaxOut      = 2 << 20
	maxCells           = 50_000
	maxZipDepth        = 3
	maxZipFiles        = 100
	maxZipUncompressed = 25 << 20

	KindText     = "text"
	KindCSV      = "csv"
	KindXLSX     = "xlsx"
	KindDOCX     = "docx"
	KindPPTX     = "pptx"
	KindPDF      = "pdf"
	KindZIP      = "zip"
	KindXLS      = "xls"
	KindMSG      = "msg"
	KindHTML     = "html"
	KindRSS      = "rss"
	KindEPUB     = "epub"
	KindIPYNB    = "ipynb"
	KindImage    = "image"
	KindAudio    = "audio"
	KindYouTube  = "youtube"
	KindDocIntel = "document-intelligence"
	KindCU       = "content-understanding"
)

var (
	ErrCorrupt     = errors.New("markitdown: corrupt input")
	ErrTooLarge    = errors.New("markitdown: input too large")
	ErrUnsupported = errors.New("markitdown: unsupported format")
)

// Result is the text extracted from one attachment.
type Result struct {
	Text    string
	Kind    string
	Partial bool
	Title   string
}

type budget struct {
	depth        int
	files        *int
	uncompressed *int64
	keepData     bool
	baseURL      string
	pageURL      string
	imageHook    ImageHook
	styleMap     string
	docIntel     *DocIntel
	contentCU    *ContentUnderstanding
}

// EmbeddedImage is one image stored inside an Office package.
type EmbeddedImage struct {
	Data []byte
	Name string
	MIME string
	Ext  string
}

// ImageHook renders an embedded image as an HTML fragment. A blank fragment
// keeps the native representation. The package does not take an LLM client.
type ImageHook func(img EmbeddedImage) (string, error)

// Options are the caller inputs Extract does not carry: a page URL, an image
// hook, a DOCX style map, and whether image data URIs are kept.
type Options struct {
	PageURL   string
	ImageHook ImageHook
	StyleMap  string
	KeepData  bool
	// DocIntel and ContentUnderstanding are used only when set. They do not
	// call Azure by themselves; the caller supplies the client.
	DocIntel             *DocIntel
	ContentUnderstanding *ContentUnderstanding
}

// Extract reads r up to 25 MB and dispatches on MIME, then on the file name
// when MIME is empty or application/octet-stream. maxOut is the output byte
// cap; zero means 2 MB. Output past the cap is truncated and Partial is set.
// ErrUnsupported is not a failure of the file contents.
func Extract(r io.Reader, mimeType, filename string, maxOut int) (Result, error) {
	return extract(r, mimeType, filename, Options{}, maxOut)
}

// ExtractPage is Extract for a document whose page URL is known. A YouTube
// watch, shorts, embed, or youtu.be URL selects that converter. The package
// still does not fetch the URL.
func ExtractPage(r io.Reader, mimeType, filename, pageURL string, maxOut int) (Result, error) {
	return extract(r, mimeType, filename, Options{PageURL: pageURL}, maxOut)
}

// ExtractOptions is Extract with a page URL, image hook, DOCX style map, or
// kept image data URIs.
func ExtractOptions(r io.Reader, mimeType, filename string, maxOut int, opt Options) (Result, error) {
	return extract(r, mimeType, filename, opt, maxOut)
}

func extract(r io.Reader, mimeType, filename string, opt Options, maxOut int) (Result, error) {
	data, err := readLimited(r)
	if err != nil {
		return Result{}, err
	}
	files := maxZipFiles
	left := int64(maxZipUncompressed)
	res, err := convertBytes(data, mimeType, filename, budget{
		depth:        maxZipDepth,
		files:        &files,
		uncompressed: &left,
		pageURL:      opt.PageURL,
		imageHook:    opt.ImageHook,
		styleMap:     opt.StyleMap,
		keepData:     opt.KeepData,
		docIntel:     opt.DocIntel,
		contentCU:    opt.ContentUnderstanding,
	})
	if err != nil {
		return Result{}, err
	}
	if maxOut <= 0 {
		maxOut = defaultMaxOut
	}
	if len(res.Text) > maxOut {
		res.Text = cutUTF8(res.Text, maxOut)
		res.Partial = true
	}
	return res, nil
}

func readLimited(r io.Reader) ([]byte, error) {
	var buf bytes.Buffer
	tmp := make([]byte, 32<<10)
	for {
		room := maxInput + 1 - buf.Len()
		if room <= 0 {
			return nil, ErrTooLarge
		}
		want := tmp
		if len(want) > room {
			want = want[:room]
		}
		n, err := r.Read(want)
		if n > 0 {
			buf.Write(want[:n])
			if buf.Len() > maxInput {
				return nil, ErrTooLarge
			}
		}
		if err == io.EOF {
			return buf.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func cutUTF8(s string, maxOut int) string {
	if maxOut >= len(s) {
		return s
	}
	cut := maxOut
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

func convertBytes(data []byte, mimeType, filename string, lim budget) (Result, error) {
	if res, handled, err := convertCloud(data, mimeType, filename, lim); handled || err != nil {
		return res, err
	}
	kind, charset, ok := detect(mimeType, filename)
	if !ok && strings.TrimSpace(mimeType) == "" && strings.TrimSpace(filename) == "" && len(data) > 0 && utf8.Valid(data) {
		// No MIME and no name: keep valid UTF-8, including a character that
		// straddles the 64 KiB charset sample. Binary stays unsupported.
		kind, ok = KindText, true
	}
	if !ok {
		return Result{}, ErrUnsupported
	}
	if kind == KindHTML && htmlLike(mimeType, filename) && youtubeVideoID(lim.pageURL) != "" {
		kind = KindYouTube
	}
	var (
		res Result
		err error
	)
	switch kind {
	case KindText:
		if notebookInput(data, charset, mimeType, filename) {
			res, err = convertIPYNB(data, charset)
			kind = KindIPYNB
		} else {
			res, err = convertText(data, charset)
		}
	case KindCSV:
		res, err = convertCSV(data, charset)
	case KindXLSX:
		res, err = convertXLSX(data, lim)
	case KindDOCX:
		res, err = convertDOCX(data, lim)
	case KindPPTX:
		res, err = convertPPTX(data, lim)
	case KindPDF:
		res, err = convertPDF(data)
	case KindXLS:
		res, err = convertXLS(data)
	case KindMSG:
		res, err = convertMSG(data)
	case KindHTML:
		res, err = convertHTML(data, htmlOpt{keepData: lim.keepData})
	case KindRSS:
		res, err = convertRSS(data, lim.baseURL)
	case KindEPUB:
		res, err = convertEPUB(data, lim.keepData)
	case KindZIP:
		res, err = convertZIP(data, filename, lim)
	case KindIPYNB:
		res, err = convertIPYNB(data, charset)
	case KindYouTube:
		res, err = convertYouTube(data, charset)
	case KindImage:
		res, err = convertImage(data)
	case KindAudio:
		res, err = convertAudio(data, filename, mimeType)
	default:
		return Result{}, ErrUnsupported
	}
	if err != nil {
		return Result{}, err
	}
	res.Kind = kind
	return res, nil
}

func detect(mimeType, filename string) (kind, charset string, ok bool) {
	mt, params, err := mime.ParseMediaType(strings.TrimSpace(mimeType))
	if err == nil {
		charset = params["charset"]
	}
	if err != nil || mt == "" || mt == "application/octet-stream" {
		kind, ok = kindFromExt(filename)
		return kind, charset, ok
	}
	if k, known := kindFromMIME(mt); known {
		return k, charset, k != ""
	}
	return "", charset, false
}

func kindFromMIME(mt string) (string, bool) {
	switch mt {
	case "text/plain", "text/markdown", "text/x-markdown", "application/markdown", "application/json":
		return KindText, true
	case "text/csv", "application/csv":
		return KindCSV, true
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return KindXLSX, true
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return KindDOCX, true
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return KindPPTX, true
	case "application/pdf":
		return KindPDF, true
	case "application/vnd.ms-excel", "application/excel":
		return KindXLS, true
	case "application/vnd.ms-outlook":
		return KindMSG, true
	case "application/zip", "application/x-zip-compressed":
		return KindZIP, true
	case "text/html", "application/xhtml+xml":
		return KindHTML, true
	case "application/rss+xml", "application/rss", "application/atom+xml", "application/atom", "text/xml", "application/xml":
		return KindRSS, true
	case "application/epub+zip", "application/epub", "application/x-epub+zip":
		return KindEPUB, true
	case "image/jpeg", "image/png":
		return KindImage, true
	case "audio/x-wav", "audio/wav", "audio/mpeg", "audio/mp4", "video/mp4":
		return KindAudio, true
	default:
		return "", false
	}
}

func kindFromExt(filename string) (string, bool) {
	switch ext(filename) {
	case ".txt", ".text", ".md", ".markdown", ".json", ".jsonl":
		return KindText, true
	case ".csv":
		return KindCSV, true
	case ".xlsx":
		return KindXLSX, true
	case ".docx":
		return KindDOCX, true
	case ".pptx":
		return KindPPTX, true
	case ".pdf":
		return KindPDF, true
	case ".xls":
		return KindXLS, true
	case ".msg":
		return KindMSG, true
	case ".html", ".htm":
		return KindHTML, true
	case ".xml", ".rss", ".atom":
		return KindRSS, true
	case ".epub":
		return KindEPUB, true
	case ".zip":
		return KindZIP, true
	case ".ipynb":
		return KindIPYNB, true
	case ".jpg", ".jpeg", ".png":
		return KindImage, true
	case ".wav", ".mp3", ".m4a", ".mp4":
		return KindAudio, true
	default:
		return "", false
	}
}

func ext(filename string) string {
	filename = strings.ReplaceAll(filename, "\\", "/")
	return strings.ToLower(path.Ext(path.Base(filename)))
}

func wrapCorrupt(err error) error {
	if err == nil {
		return ErrCorrupt
	}
	if errors.Is(err, ErrCorrupt) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrCorrupt, err)
}
