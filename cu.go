package markitdown

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// CUClient is the local stand-in for ContentUnderstandingClient.
type CUClient interface {
	BeginAnalyzeBinary(analyzerID string, input []byte, contentType string) (CUPoller, error)
	GetAnalyzer(id string) (baseAnalyzerID string, err error)
}

// CUPoller is the local stand-in for the analyze poller. Result is called once.
type CUPoller interface {
	Result() (CUResult, error)
}

// CUResult is the analysis payload to_llm_input formats.
type CUResult struct {
	Contents []CUContent
}

// CUContent is one document or audiovisual segment.
type CUContent struct {
	MIME        string
	Markdown    string
	AudioVisual bool
	StartPage   int
	EndPage     int
	HasPages    bool
	Fields      map[string]string
	Category    string
}

// ContentUnderstanding routes files to a caller-supplied Content Understanding client.
type ContentUnderstanding struct {
	Endpoint   string
	Credential string
	AnalyzerID string
	Modality   string
	FileTypes  []cuType
	Client     CUClient
}

type cuType string

const (
	cuPDF  cuType = "pdf"
	cuDOCX cuType = "docx"
	cuPPTX cuType = "pptx"
	cuXLSX cuType = "xlsx"
	cuHTML cuType = "html"
	cuTXT  cuType = "txt"
	cuMD   cuType = "md"
	cuRTF  cuType = "rtf"
	cuXML  cuType = "xml"
	cuEML  cuType = "eml"
	cuMSG  cuType = "msg"
	cuJPEG cuType = "jpeg"
	cuPNG  cuType = "png"
	cuBMP  cuType = "bmp"
	cuTIFF cuType = "tiff"
	cuHEIF cuType = "heif"
	cuMP4  cuType = "mp4"
	cuM4V  cuType = "m4v"
	cuMOV  cuType = "mov"
	cuAVI  cuType = "avi"
	cuMKV  cuType = "mkv"
	cuWEBM cuType = "webm"
	cuFLV  cuType = "flv"
	cuWMV  cuType = "wmv"
	cuWAV  cuType = "wav"
	cuMP3  cuType = "mp3"
	cuM4A  cuType = "m4a"
	cuFLAC cuType = "flac"
	cuOGG  cuType = "ogg"
	cuAAC  cuType = "aac"
	cuWMA  cuType = "wma"
)

// ParseCUFileType matches ContentUnderstandingFileType(value).
func ParseCUFileType(s string) (cuType, bool) {
	switch cuType(s) {
	case cuPDF, cuDOCX, cuPPTX, cuXLSX, cuHTML, cuTXT, cuMD, cuRTF, cuXML,
		cuEML, cuMSG, cuJPEG, cuPNG, cuBMP, cuTIFF, cuHEIF, cuMP4, cuM4V,
		cuMOV, cuAVI, cuMKV, cuWEBM, cuFLV, cuWMV, cuWAV, cuMP3, cuM4A,
		cuFLAC, cuOGG, cuAAC, cuWMA:
		return cuType(s), true
	default:
		return "", false
	}
}

var allCUTypes = []cuType{
	cuPDF, cuDOCX, cuPPTX, cuXLSX, cuHTML, cuTXT, cuMD, cuRTF, cuXML,
	cuEML, cuMSG, cuJPEG, cuPNG, cuBMP, cuTIFF, cuHEIF, cuMP4, cuM4V,
	cuMOV, cuAVI, cuMKV, cuWEBM, cuFLV, cuWMV, cuWAV, cuMP3, cuM4A,
	cuFLAC, cuOGG, cuAAC, cuWMA,
}

var cuExt = map[string]cuType{
	".pdf": cuPDF, ".docx": cuDOCX, ".pptx": cuPPTX, ".xlsx": cuXLSX,
	".html": cuHTML, ".txt": cuTXT, ".md": cuMD, ".rtf": cuRTF, ".xml": cuXML,
	".eml": cuEML, ".msg": cuMSG,
	".jpg": cuJPEG, ".jpeg": cuJPEG, ".jpe": cuJPEG, ".png": cuPNG,
	".bmp": cuBMP, ".tiff": cuTIFF, ".heif": cuHEIF, ".heic": cuHEIF,
	".mp4": cuMP4, ".m4v": cuM4V, ".mov": cuMOV, ".avi": cuAVI,
	".mkv": cuMKV, ".webm": cuWEBM, ".flv": cuFLV, ".wmv": cuWMV,
	".wav": cuWAV, ".mp3": cuMP3, ".m4a": cuM4A, ".flac": cuFLAC,
	".ogg": cuOGG, ".aac": cuAAC, ".wma": cuWMA,
}

var cuMIME = []struct {
	kind     cuType
	prefixes []string
}{
	{cuPDF, []string{"application/pdf", "application/x-pdf"}},
	{cuDOCX, []string{"application/vnd.openxmlformats-officedocument.wordprocessingml.document"}},
	{cuPPTX, []string{"application/vnd.openxmlformats-officedocument.presentationml"}},
	{cuXLSX, []string{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"}},
	{cuHTML, []string{"text/html", "application/xhtml+xml"}},
	{cuTXT, []string{"text/plain"}},
	{cuMD, []string{"text/markdown"}},
	{cuRTF, []string{"text/rtf", "application/rtf"}},
	{cuXML, []string{"text/xml", "application/xml"}},
	{cuEML, []string{"message/rfc822"}},
	{cuMSG, []string{"application/vnd.ms-outlook"}},
	{cuJPEG, []string{"image/jpeg"}},
	{cuPNG, []string{"image/png"}},
	{cuBMP, []string{"image/bmp"}},
	{cuTIFF, []string{"image/tiff"}},
	{cuHEIF, []string{"image/heif", "image/heic"}},
	{cuMP4, []string{"video/mp4"}},
	{cuM4V, []string{"video/x-m4v"}},
	{cuMOV, []string{"video/quicktime"}},
	{cuAVI, []string{"video/x-msvideo"}},
	{cuMKV, []string{"video/x-matroska"}},
	{cuWEBM, []string{"video/webm"}},
	{cuFLV, []string{"video/x-flv"}},
	{cuWMV, []string{"video/x-ms-wmv"}},
	{cuWAV, []string{"audio/wav", "audio/x-wav"}},
	{cuMP3, []string{"audio/mpeg", "audio/mp3"}},
	{cuM4A, []string{"audio/mp4", "audio/m4a", "audio/x-m4a"}},
	{cuFLAC, []string{"audio/flac", "audio/x-flac"}},
	{cuOGG, []string{"audio/ogg"}},
	{cuAAC, []string{"audio/aac"}},
	{cuWMA, []string{"audio/x-ms-wma"}},
}

var cuMIMEAlias = map[string]string{
	"audio/x-wav":  "audio/wav",
	"audio/x-flac": "audio/flac",
	"audio/x-m4a":  "audio/mp4",
	"video/x-m4v":  "video/mp4",
}

var cuPrebuilt = map[string]string{
	"document": "prebuilt-documentSearch",
	"image":    "prebuilt-documentSearch",
	"video":    "prebuilt-videoSearch",
	"audio":    "prebuilt-audioSearch",
}

var cuBaseModality = map[string]string{
	"prebuilt-document": "document",
	"prebuilt-image":    "image",
	"prebuilt-audio":    "audio",
	"prebuilt-video":    "video",
}

var cuKnownModality = map[string]string{
	"prebuilt-documentSearch": "document", "prebuilt-layout": "document",
	"prebuilt-read": "document", "prebuilt-document": "document",
	"prebuilt-invoice": "document", "prebuilt-receipt": "document",
	"prebuilt-receipt.generic": "document", "prebuilt-receipt.hotel": "document",
	"prebuilt-idDocument": "document", "prebuilt-idDocument.generic": "document",
	"prebuilt-idDocument.passport": "document", "prebuilt-healthInsuranceCard.us": "document",
	"prebuilt-contract": "document", "prebuilt-creditCard": "document",
	"prebuilt-creditMemo": "document", "prebuilt-bankStatement.us": "document",
	"prebuilt-check.us": "document", "prebuilt-purchaseOrder": "document",
	"prebuilt-procurement": "document", "prebuilt-payStub.us": "document",
	"prebuilt-utilityBill": "document", "prebuilt-marriageCertificate.us": "document",
	"prebuilt-documentFieldSchema": "document", "prebuilt-documentFields": "document",
	"prebuilt-tax.us": "document", "prebuilt-tax.us.w2": "document",
	"prebuilt-tax.us.w4": "document", "prebuilt-tax.us.1040": "document",
	"prebuilt-mortgage.us": "document", "prebuilt-mortgage.us.1003": "document",
	"prebuilt-mortgage.us.closingDisclosure": "document",
	"prebuilt-image":                         "image", "prebuilt-imageSearch": "image",
	"prebuilt-audio": "audio", "prebuilt-audioSearch": "audio", "prebuilt-callCenter": "audio",
	"prebuilt-video": "video", "prebuilt-videoSearch": "video", "prebuilt-videoSynopsis": "video",
}

// NewContentUnderstanding records the endpoint and, when analyzerID is set,
// resolves its modality through the client. It does not call Azure on its own.
func NewContentUnderstanding(endpoint, analyzerID string, client CUClient) (*ContentUnderstanding, error) {
	cu := &ContentUnderstanding{
		Endpoint:   endpoint,
		Credential: "default",
		AnalyzerID: analyzerID,
		FileTypes:  append([]cuType(nil), allCUTypes...),
		Client:     client,
	}
	if os.Getenv("AZURE_API_KEY") != "" {
		cu.Credential = "api-key"
	}
	if analyzerID != "" {
		mod, err := resolveAnalyzerModality(client, analyzerID)
		if err != nil {
			return nil, err
		}
		cu.Modality = mod
	}
	return cu, nil
}

func (c *ContentUnderstanding) Accepts(mimeType, filename string) bool {
	if c == nil {
		return false
	}
	return detectCUType(mimeType, filename, c.FileTypes) != ""
}

func (c *ContentUnderstanding) Convert(data []byte, mimeType, filename string) (Result, error) {
	if c == nil || c.Client == nil {
		return Result{}, ErrCloudClient
	}
	kind := detectCUType(mimeType, filename, c.FileTypes)
	if kind == "" {
		return Result{}, fmt.Errorf("unsupported file type for Content Understanding conversion")
	}
	modality := cuModality(kind)
	analyzer := cuPrebuilt[modality]
	if analyzer == "" {
		analyzer = "prebuilt-documentSearch"
	}
	if c.AnalyzerID != "" && c.Modality != "" && analyzerCompatible(modality, c.Modality) {
		analyzer = c.AnalyzerID
	}
	contentType := contentTypeFor(kind, mimeType)
	poller, err := c.Client.BeginAnalyzeBinary(analyzer, data, contentType)
	if err != nil {
		return Result{}, err
	}
	result, err := poller.Result()
	if err != nil {
		return Result{}, err
	}
	return Result{Text: toLLMInput(result)}, nil
}

func detectCUType(mimeType, filename string, allowed []cuType) cuType {
	allow := map[cuType]bool{}
	restricted := allowed != nil
	for _, t := range allowed {
		allow[t] = true
	}
	if extn := ext(filename); extn != "." {
		if kind, ok := cuExt[extn]; ok && (!restricted || allow[kind]) {
			return kind
		}
	}
	mt := cleanMIME(mimeType)
	if mt == "" {
		return ""
	}
	for _, item := range cuMIME {
		if restricted && !allow[item.kind] {
			continue
		}
		for _, prefix := range item.prefixes {
			if strings.HasPrefix(mt, prefix) {
				return item.kind
			}
		}
	}
	return ""
}

func cuModality(kind cuType) string {
	switch kind {
	case cuPDF, cuDOCX, cuPPTX, cuXLSX, cuHTML, cuTXT, cuMD, cuRTF, cuXML, cuEML, cuMSG:
		return "document"
	case cuJPEG, cuPNG, cuBMP, cuTIFF, cuHEIF:
		return "image"
	case cuMP4, cuM4V, cuMOV, cuAVI, cuMKV, cuWEBM, cuFLV, cuWMV:
		return "video"
	case cuWAV, cuMP3, cuM4A, cuFLAC, cuOGG, cuAAC, cuWMA:
		return "audio"
	default:
		return ""
	}
}

func analyzerCompatible(fileModality, analyzerModality string) bool {
	if analyzerModality == "document" {
		return fileModality == "document" || fileModality == "image"
	}
	return fileModality == analyzerModality
}

func canonicalMIME(mimeType string) string {
	cleaned := cleanMIME(mimeType)
	if cleaned == "" {
		return "application/octet-stream"
	}
	if alias, ok := cuMIMEAlias[cleaned]; ok {
		return alias
	}
	return cleaned
}

func contentTypeFor(kind cuType, mimeType string) string {
	var prefixes []string
	for _, item := range cuMIME {
		if item.kind == kind {
			prefixes = item.prefixes
			break
		}
	}
	canonical := canonicalMIME(mimeType)
	if len(prefixes) > 0 && canonical != "application/octet-stream" {
		for _, prefix := range prefixes {
			if strings.HasPrefix(canonical, prefix) {
				return canonical
			}
		}
	}
	if len(prefixes) > 0 {
		return canonicalMIME(prefixes[0])
	}
	return canonical
}

func resolveAnalyzerModality(client CUClient, analyzerID string) (string, error) {
	if mod, ok := cuKnownModality[analyzerID]; ok {
		return mod, nil
	}
	if client == nil {
		return "", fmt.Errorf("Failed to resolve analyzer '%s': %w", analyzerID, ErrCloudClient)
	}
	base, err := client.GetAnalyzer(analyzerID)
	if err != nil {
		return "", fmt.Errorf("Failed to resolve analyzer '%s': %v", analyzerID, err)
	}
	if base != "" {
		if mod, ok := cuBaseModality[base]; ok {
			return mod, nil
		}
		return "document", nil
	}
	return "document", nil
}

func toLLMInput(result CUResult) string {
	if len(result.Contents) == 0 {
		return ""
	}
	av := 0
	for _, c := range result.Contents {
		if c.AudioVisual {
			av++
		}
	}
	var blocks []string
	for _, c := range result.Contents {
		if block := renderCUBlock(c, av > 1); block != "" {
			blocks = append(blocks, block)
		}
	}
	return strings.Join(blocks, "\n\n*****\n\n")
}

func renderCUBlock(c CUContent, multi bool) string {
	var lines []string
	lines = append(lines, "---")
	mime := c.MIME
	if mime == "" {
		mime = "unknown"
	}
	lines = append(lines, "mimeType: "+mime)
	if c.Category != "" {
		lines = append(lines, "category: "+c.Category)
	}
	if !c.AudioVisual && c.HasPages && c.StartPage > 0 && c.EndPage > 0 {
		if c.StartPage == c.EndPage {
			lines = append(lines, fmt.Sprintf("pages: %d", c.StartPage))
		} else {
			lines = append(lines, fmt.Sprintf("pages: %d-%d", c.StartPage, c.EndPage))
		}
	}
	if len(c.Fields) > 0 {
		lines = append(lines, "fields:")
		keys := make([]string, 0, len(c.Fields))
		for k := range c.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			lines = append(lines, "  "+k+": "+c.Fields[k])
		}
	}
	lines = append(lines, "---")
	front := strings.Join(lines, "\n")
	if c.Markdown == "" {
		return front
	}
	md := c.Markdown
	if !c.AudioVisual {
		md = addCUPageMarkers(md, c.StartPage)
	}
	return front + "\n" + md
}

func addCUPageMarkers(md string, start int) string {
	if strings.Contains(md, "<!-- InputPageNumber:") {
		return md
	}
	if start <= 0 {
		start = 1
	}
	if !strings.Contains(md, "<!-- PageBreak -->") {
		text := strings.TrimSpace(md)
		if text == "" {
			return md
		}
		return fmt.Sprintf("<!-- InputPageNumber: %d -->\n\n%s", start, text)
	}
	parts := strings.Split(md, "<!-- PageBreak -->")
	var out []string
	for i, chunk := range parts {
		text := strings.TrimSpace(chunk)
		if text == "" {
			continue
		}
		out = append(out, fmt.Sprintf("<!-- InputPageNumber: %d -->\n\n%s", start+i, text))
	}
	return strings.Join(out, "\n\n")
}
