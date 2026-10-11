package csv

import (
	"encoding/csv"
	"io"
	"strings"

	"github.com/godwit-email/markitdown-go"
	"github.com/godwit-email/markitdown-go/internal/charset"
	"github.com/godwit-email/markitdown-go/internal/run"
	"github.com/godwit-email/markitdown-go/internal/table"
)

func convertCSV(in markitdown.Input) (markitdown.Result, error) {
	text := charset.Decode(in.Data, in.Params["charset"])
	rows, err := parseCSV(text)
	if err != nil {
		return markitdown.Result{}, run.Wrap(err)
	}
	rows = trimBlankRows(rows)
	return markitdown.Result{Text: table.Markdown(rows)}, nil
}

// blankSentinel is a one-field row standing in for a blank CSV line.
// encoding/csv drops blank lines; Python's csv reader keeps them.
const blankSentinel = "\x00blank\x00"

func parseCSV(text string) ([][]string, error) {
	r := csv.NewReader(strings.NewReader(markBlankLines(text)))
	r.FieldsPerRecord = -1
	r.ReuseRecord = false
	var rows [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			return rows, nil
		}
		if err != nil {
			return nil, err
		}
		if len(rec) == 1 && rec[0] == blankSentinel {
			rows = append(rows, []string{})
			continue
		}
		rows = append(rows, rec)
	}
}

// markBlankLines turns blank records into a sentinel field and normalizes
// CR / CRLF record separators. Newlines inside quotes are left alone.
func markBlankLines(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	inQuote := false
	empty := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote {
			b.WriteByte(c)
			if c == '"' {
				if i+1 < len(s) && s[i+1] == '"' {
					b.WriteByte('"')
					i++
					continue
				}
				inQuote = false
			}
			empty = false
			continue
		}
		if c == '"' {
			inQuote = true
			empty = false
			b.WriteByte(c)
			continue
		}
		if c == '\n' || c == '\r' {
			if c == '\r' && i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
			if empty {
				b.WriteString(`"` + blankSentinel + `"`)
			}
			b.WriteByte('\n')
			empty = true
			continue
		}
		empty = false
		b.WriteByte(c)
	}
	return b.String()
}

// trimBlankRows drops empty rows at the start, immediately after the header,
// and at the end. Empty rows between data rows stay.
func trimBlankRows(rows [][]string) [][]string {
	start := 0
	for start < len(rows) && len(rows[start]) == 0 {
		start++
	}
	if start == len(rows) {
		return nil
	}
	header := start
	start++
	for start < len(rows) && len(rows[start]) == 0 {
		start++
	}
	end := len(rows)
	for end > start && len(rows[end-1]) == 0 {
		end--
	}
	out := make([][]string, 0, 1+end-start)
	out = append(out, rows[header])
	out = append(out, rows[start:end]...)
	return out
}
