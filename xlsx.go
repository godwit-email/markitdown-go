package markitdown

import (
	"bytes"
	"strings"

	"github.com/xuri/excelize/v2"
)

func convertXLSX(data []byte) (Result, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return Result{}, wrapCorrupt(err)
	}
	defer f.Close()

	var b strings.Builder
	cells := 0
	partial := false
	for _, name := range f.GetSheetList() {
		rows, err := f.GetRows(name)
		if err != nil {
			return Result{}, wrapCorrupt(err)
		}
		kept := make([][]string, 0, len(rows))
		any := false
		stop := false
		for _, row := range rows {
			out := make([]string, len(row))
			for i, cell := range row {
				if cells >= maxCells {
					partial = true
					stop = true
					break
				}
				cells++
				out[i] = cell
				if strings.TrimSpace(cell) != "" {
					any = true
				}
			}
			if len(out) > 0 {
				kept = append(kept, out)
			}
			if stop {
				break
			}
		}
		if !any {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("## ")
		b.WriteString(name)
		b.WriteString("\n\n")
		b.WriteString(markdownTable(kept))
		if stop {
			break
		}
	}
	return Result{Text: strings.TrimSpace(b.String()), Partial: partial}, nil
}
