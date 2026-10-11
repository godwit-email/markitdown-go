package xls

import (
	"bytes"
	"strings"

	"github.com/extrame/xls"
	"github.com/godwit-email/markitdown-go"
	"github.com/godwit-email/markitdown-go/internal/run"
	"github.com/godwit-email/markitdown-go/internal/table"
)

const maxCells = 50_000

func convertXLS(data []byte) (markitdown.Result, error) {
	wb, err := xls.OpenReader(bytes.NewReader(data), "utf-8")
	if err != nil {
		return markitdown.Result{}, run.Wrap(err)
	}
	var b strings.Builder
	cells := 0
	partial := false
	for i := 0; i < wb.NumSheets(); i++ {
		sheet := wb.GetSheet(i)
		if sheet == nil {
			continue
		}
		var rows [][]string
		any := false
		stop := false
		for r := 0; r <= int(sheet.MaxRow); r++ {
			cols, ok := xlsRow(sheet, r)
			if !ok {
				continue
			}
			out := make([]string, len(cols))
			for c, cell := range cols {
				if cells >= maxCells {
					partial = true
					stop = true
					break
				}
				cells++
				out[c] = cell
				if strings.TrimSpace(cell) != "" {
					any = true
				}
			}
			if len(out) > 0 {
				rows = append(rows, out)
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
		b.WriteString(sheet.Name)
		b.WriteByte('\n')
		b.WriteString(table.Markdown(trimEmptyCols(rows)))
		if stop {
			break
		}
	}
	return markitdown.Result{Text: strings.TrimSpace(b.String()), Partial: partial}, nil
}

func trimEmptyCols(rows [][]string) [][]string {
	width := 0
	for _, row := range rows {
		for i := len(row) - 1; i >= 0; i-- {
			if strings.TrimSpace(row[i]) != "" {
				if i+1 > width {
					width = i + 1
				}
				break
			}
		}
	}
	for i, row := range rows {
		if len(row) > width {
			rows[i] = row[:width]
		}
	}
	return rows
}

func xlsRow(sheet *xls.WorkSheet, i int) (cols []string, ok bool) {
	defer func() { _ = recover() }()
	row := sheet.Row(i)
	if row == nil {
		return nil, false
	}
	n := row.LastCol()
	if n < 0 {
		return nil, false
	}
	cols = make([]string, n+1)
	for c := 0; c <= n; c++ {
		cols[c] = row.Col(c)
	}
	return cols, true
}
