package xlsx

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/godwit-email/markitdown-go"
	"github.com/godwit-email/markitdown-go/internal/ooxml"
	"github.com/godwit-email/markitdown-go/internal/run"
	"github.com/godwit-email/markitdown-go/internal/table"
	"github.com/xuri/excelize/v2"
)

const maxCells = 50_000

func convertXLSX(ctx context.Context, in markitdown.Input) (markitdown.Result, error) {
	data := in.Data
	repaired, err := repairShowZeroes(data)
	if err != nil {
		return markitdown.Result{}, err
	}
	f, err := excelize.OpenReader(bytes.NewReader(repaired))
	if err != nil {
		return markitdown.Result{}, run.Wrap(err)
	}
	defer f.Close()

	var images map[string][]ooxml.Image
	if in.OCR != nil {
		images, err = xlsxSheetImages(repaired)
		if err != nil {
			return markitdown.Result{}, err
		}
	}

	var b strings.Builder
	cells := 0
	partial := false
	session := newOCRSession(ctx, in.OCR)
	for _, name := range f.GetSheetList() {
		rows, err := f.GetRows(name)
		if err != nil {
			return markitdown.Result{}, run.Wrap(err)
		}
		table, used, stop := sheetTable(rows, &cells, &partial, in.OCR != nil)
		imgs := images[name]
		if !used && len(imgs) == 0 {
			if stop {
				break
			}
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		if used {
			b.WriteString("## ")
			b.WriteString(name)
			b.WriteByte('\n')
			b.WriteString(table)
		}
		if len(imgs) > 0 && session != nil {
			block, err := ocrSheetImages(imgs, session)
			if err != nil {
				return markitdown.Result{}, err
			}
			if block != "" {
				if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n\n") {
					b.WriteString("\n\n")
				}
				b.WriteString(block)
			}
		}
		if stop {
			break
		}
	}
	if session != nil && session.partial {
		partial = true
	}
	return markitdown.Result{Text: strings.TrimSpace(b.String()), Partial: partial}, nil
}

func ocrSheetImages(imgs []ooxml.Image, session *ocrSession) (string, error) {
	var blocks []string
	for _, img := range imgs {
		md, ok := session.markdown(OCRImage{Data: img.Data, Name: img.Name, MIME: img.MIME})
		if session.err != nil {
			return "", session.err
		}
		if ok {
			blocks = append(blocks, md)
		}
	}
	if len(blocks) == 0 {
		return "", nil
	}
	return "### Images in this sheet:\n\n" + strings.Join(blocks, "\n\n"), nil
}

func sheetTable(rows [][]string, cells *int, partial *bool, emptyAsNA bool) (string, bool, bool) {
	width := 0
	for _, row := range rows {
		if len(row) > width {
			width = len(row)
		}
	}
	if width == 0 {
		return "", false, false
	}
	kept := make([][]string, 0, len(rows))
	any := false
	stop := false
	for i, row := range rows {
		out := make([]string, width)
		for col := 0; col < width; col++ {
			if *cells >= maxCells {
				*partial = true
				stop = true
				break
			}
			*cells++
			cell := ""
			if col < len(row) {
				cell = row[col]
			}
			if strings.TrimSpace(cell) != "" {
				any = true
			}
			if i == 0 && strings.TrimSpace(cell) == "" {
				out[col] = fmt.Sprintf("Unnamed: %d", col)
				continue
			}
			if emptyAsNA && strings.TrimSpace(cell) == "" {
				out[col] = "NaN"
				continue
			}
			out[col] = escapeXLSXCell(cell)
		}
		kept = append(kept, out)
		if stop {
			break
		}
	}
	if !any {
		return "", false, stop
	}
	return table.Markdown(kept), true, stop
}

func escapeXLSXCell(s string) string {
	s = table.EscapeCell(s)
	var b strings.Builder
	for _, r := range s {
		if r == '*' || r == '_' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

var (
	sheetViewTag   = regexp.MustCompile(`<sheetView(?:[\t\n\f\r ]|/)[^>]*>`)
	showZeroesAttr = regexp.MustCompile(`(^|[\t\n\f\r ])showZeroes(\s*=)`)
)

func renameShowZeroes(data []byte) []byte {
	return sheetViewTag.ReplaceAllFunc(data, func(tag []byte) []byte {
		return showZeroesAttr.ReplaceAll(tag, []byte("${1}showZeros${2}"))
	})
}

func repairShowZeroes(data []byte) ([]byte, error) {
	if !bytes.Contains(data, []byte("showZeroes")) {
		return data, nil
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, run.Wrap(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	changed := false
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			zw.Close()
			return nil, run.Wrap(err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			zw.Close()
			return nil, err
		}
		name := f.Name
		if strings.HasPrefix(name, "xl/worksheets/") && strings.HasSuffix(name, ".xml") && bytes.Contains(body, []byte("showZeroes")) {
			next := renameShowZeroes(body)
			if !bytes.Equal(next, body) {
				changed = true
				body = next
			}
		}
		w, err := zw.Create(name)
		if err != nil {
			zw.Close()
			return nil, run.Wrap(err)
		}
		if _, err := w.Write(body); err != nil {
			zw.Close()
			return nil, run.Wrap(err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, run.Wrap(err)
	}
	if !changed {
		return data, nil
	}
	return buf.Bytes(), nil
}
