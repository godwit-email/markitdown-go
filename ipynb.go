package markitdown

import (
	"encoding/json"
	"mime"
	"strings"
	"unicode/utf8"
)

func notebookInput(data []byte, charset, mimeType, filename string) bool {
	if ext(filename) == ".ipynb" {
		return true
	}
	mt, _, err := mime.ParseMediaType(strings.TrimSpace(mimeType))
	if err != nil || !strings.HasPrefix(mt, "application/json") {
		return false
	}
	return looksLikeNotebook(data, charset)
}

func looksLikeNotebook(data []byte, charset string) bool {
	text, ok := strictText(data, charset)
	if !ok {
		return false
	}
	text = strings.TrimLeft(text, "\uFEFF")
	return strings.Contains(text, "nbformat") && strings.Contains(text, "nbformat_minor")
}

func strictText(data []byte, charset string) (string, bool) {
	charset = strings.TrimSpace(charset)
	if charset == "" || isUTF8Charset(charset) {
		rest := data
		if bytesHasBOM(rest) {
			rest = rest[3:]
		}
		if !utf8.Valid(rest) {
			return "", false
		}
		return string(rest), true
	}
	text, err := decodeText(data, charset)
	if err != nil {
		return "", false
	}
	return text, true
}

func bytesHasBOM(data []byte) bool {
	return len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF
}

func convertIPYNB(data []byte, charset string) (Result, error) {
	text, err := decodeText(data, charset)
	if err != nil {
		return Result{}, err
	}
	text = strings.TrimLeft(text, "\uFEFF")
	var nb nbFile
	if err := json.Unmarshal([]byte(text), &nb); err != nil {
		return Result{}, wrapCorrupt(err)
	}
	return nb.convert(), nil
}

type nbFile struct {
	Metadata map[string]any `json:"metadata"`
	Cells    []nbCell       `json:"cells"`
}

type nbCell struct {
	CellType string   `json:"cell_type"`
	Source   nbSource `json:"source"`
}

type nbSource []string

func (s *nbSource) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var one string
		if err := json.Unmarshal(b, &one); err != nil {
			return err
		}
		*s = []string{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*s = many
	return nil
}

func (nb nbFile) convert() Result {
	var parts []string
	var title string
	foundTitle := false
	for _, cell := range nb.Cells {
		source := strings.Join(cell.Source, "")
		switch cell.CellType {
		case "markdown":
			parts = append(parts, source)
			if !foundTitle {
				for _, line := range cell.Source {
					if strings.HasPrefix(line, "# ") {
						title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
						foundTitle = true
						break
					}
				}
			}
		case "code":
			parts = append(parts, "```python\n"+source+"\n```")
		case "raw":
			parts = append(parts, "```\n"+source+"\n```")
		}
	}
	if nb.Metadata != nil {
		if t, ok := nb.Metadata["title"]; ok {
			if s, ok := t.(string); ok {
				title = s
			}
		}
	}
	return Result{Text: strings.Join(parts, "\n\n"), Title: title}
}
