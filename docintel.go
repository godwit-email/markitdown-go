package markitdown

import (
	"os"
	"regexp"
	"strings"
)

// ErrCloudClient means a Document Intelligence or Content Understanding
// converter was asked to run without a caller-supplied client. This package
// does not call Azure itself.
var ErrCloudClient = errorsNew("markitdown: cloud client is not configured")

func errorsNew(s string) error { return cloudError(s) }

type cloudError string

func (e cloudError) Error() string { return string(e) }

// DocIntelClient is the local stand-in for DocumentIntelligenceClient.
// Analyze returns the service markdown. Features is empty for Office and HTML.
type DocIntelClient interface {
	Analyze(modelID string, body []byte, features []string, format string) (string, error)
}

// DocIntelConfig is what the Python client constructor receives.
// HasAPIVersion is false when api_version is omitted.
type DocIntelConfig struct {
	Endpoint      string
	Credential    string
	APIVersion    string
	HasAPIVersion bool
}

// DocIntel extracts text through a caller-supplied Document Intelligence client.
type DocIntel struct {
	Config    DocIntelConfig
	FileTypes []string
	Client    DocIntelClient
}

// NewDocIntel records endpoint and credential choice. It does not contact Azure.
// A nil API version is omitted, matching the default constructor.
func NewDocIntel(endpoint string, apiVersion *string, client DocIntelClient) *DocIntel {
	cfg := DocIntelConfig{Endpoint: endpoint, Credential: "default"}
	if key := os.Getenv("AZURE_API_KEY"); key != "" {
		cfg.Credential = "api-key"
	}
	if apiVersion != nil {
		cfg.HasAPIVersion = true
		cfg.APIVersion = *apiVersion
	}
	return &DocIntel{
		Config:    cfg,
		FileTypes: []string{"docx", "pptx", "xlsx", "pdf", "jpeg", "png", "bmp", "tiff"},
		Client:    client,
	}
}

func (d *DocIntel) Accepts(mimeType, filename string) bool {
	if d == nil {
		return false
	}
	extn := ext(filename)
	if extn != "." && containsStr(docIntelExts(d.FileTypes), extn) {
		return true
	}
	mt := cleanMIME(mimeType)
	for _, prefix := range docIntelMIMEPrefixes(d.FileTypes) {
		if strings.HasPrefix(mt, prefix) {
			return true
		}
	}
	return false
}

func (d *DocIntel) Convert(data []byte, mimeType, filename string) (Result, error) {
	if d == nil || d.Client == nil {
		return Result{}, ErrCloudClient
	}
	content, err := d.Client.Analyze("prebuilt-layout", data, docIntelFeatures(mimeType, filename), "markdown")
	if err != nil {
		return Result{}, err
	}
	return Result{Text: docIntelComment.ReplaceAllString(content, "")}, nil
}

var docIntelComment = regexp.MustCompile(`(?s)<!--.*?-->`)

func docIntelFeatures(mimeType, filename string) []string {
	noOCR := []string{"docx", "pptx", "xlsx", "html"}
	extn := ext(filename)
	if extn != "." && containsStr(docIntelExts(noOCR), extn) {
		return nil
	}
	mt := cleanMIME(mimeType)
	for _, prefix := range docIntelMIMEPrefixes(noOCR) {
		if mt != "" && strings.HasPrefix(mt, prefix) {
			return nil
		}
	}
	return []string{"formulas", "ocrHighResolution", "styleFont"}
}

func docIntelExts(types []string) []string {
	var out []string
	for _, t := range types {
		switch t {
		case "docx":
			out = append(out, ".docx")
		case "pptx":
			out = append(out, ".pptx")
		case "xlsx":
			out = append(out, ".xlsx")
		case "pdf":
			out = append(out, ".pdf")
		case "jpeg":
			out = append(out, ".jpg", ".jpeg")
		case "png":
			out = append(out, ".png")
		case "bmp":
			out = append(out, ".bmp")
		case "tiff":
			out = append(out, ".tiff")
		case "html":
			out = append(out, ".html")
		}
	}
	return out
}

func docIntelMIMEPrefixes(types []string) []string {
	var out []string
	for _, t := range types {
		switch t {
		case "docx":
			out = append(out, "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
		case "pptx":
			out = append(out, "application/vnd.openxmlformats-officedocument.presentationml")
		case "xlsx":
			out = append(out, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		case "html":
			out = append(out, "text/html", "application/xhtml+xml")
		case "pdf":
			out = append(out, "application/pdf", "application/x-pdf")
		case "jpeg":
			out = append(out, "image/jpeg")
		case "png":
			out = append(out, "image/png")
		case "bmp":
			out = append(out, "image/bmp")
		case "tiff":
			out = append(out, "image/tiff")
		}
	}
	return out
}

func containsStr(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func cleanMIME(mimeType string) string {
	mt := strings.TrimSpace(mimeType)
	if i := strings.IndexByte(mt, ';'); i >= 0 {
		mt = mt[:i]
	}
	return strings.ToLower(strings.TrimSpace(mt))
}

func convertCloud(data []byte, mimeType, filename string, lim budget) (Result, bool, error) {
	if lim.contentCU != nil && lim.contentCU.Accepts(mimeType, filename) {
		res, err := lim.contentCU.Convert(data, mimeType, filename)
		if err != nil {
			return Result{}, true, err
		}
		res.Kind = KindCU
		return res, true, nil
	}
	if lim.docIntel != nil && lim.docIntel.Accepts(mimeType, filename) {
		res, err := lim.docIntel.Convert(data, mimeType, filename)
		if err != nil {
			return Result{}, true, err
		}
		res.Kind = KindDocIntel
		return res, true, nil
	}
	return Result{}, false, nil
}

// CloudConverterOrder is the try-order when both cloud converters are configured.
// Content Understanding is before Document Intelligence.
func CloudConverterOrder(opt Options) []string {
	var out []string
	if opt.ContentUnderstanding != nil {
		out = append(out, KindCU)
	}
	if opt.DocIntel != nil {
		out = append(out, KindDocIntel)
	}
	return out
}
