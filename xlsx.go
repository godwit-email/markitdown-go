package markitdown

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/xuri/excelize/v2"
)

func convertXLSX(data []byte, lim budget) (Result, error) {
	repaired, err := repairShowZeroes(data)
	if err != nil {
		return Result{}, err
	}
	f, err := excelize.OpenReader(bytes.NewReader(repaired))
	if err != nil {
		return Result{}, wrapCorrupt(err)
	}
	defer f.Close()

	var images map[string][]EmbeddedImage
	if lim.imageHook != nil {
		images, err = xlsxSheetImages(repaired)
		if err != nil {
			return Result{}, err
		}
	}

	var b strings.Builder
	cells := 0
	partial := false
	for _, name := range f.GetSheetList() {
		rows, err := f.GetRows(name)
		if err != nil {
			return Result{}, wrapCorrupt(err)
		}
		table, used, stop := sheetTable(rows, &cells, &partial)
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
		if len(imgs) > 0 {
			block, err := sheetImagesMarkdown(imgs, lim.imageHook)
			if err != nil {
				return Result{}, err
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
	return Result{Text: strings.TrimSpace(b.String()), Partial: partial}, nil
}

func sheetTable(rows [][]string, cells *int, partial *bool) (string, bool, bool) {
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
	return markdownTable(kept), true, stop
}

func escapeXLSXCell(s string) string {
	s = escapeCell(s)
	var b strings.Builder
	for _, r := range s {
		if r == '*' || r == '_' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func sheetImagesMarkdown(imgs []EmbeddedImage, hook ImageHook) (string, error) {
	var divs []string
	for _, img := range imgs {
		fragment, err := hook(img)
		if err != nil {
			return "", err
		}
		inner, err := imageFragment(fragment)
		if err != nil {
			return "", err
		}
		if inner == "" {
			continue
		}
		divs = append(divs, "<div>"+inner+"</div>")
	}
	if len(divs) == 0 {
		return "", nil
	}
	html := "<h3>Images in this sheet:</h3>" + strings.Join(divs, "")
	res, err := convertHTML([]byte(html), htmlOpt{})
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

func imageFragment(fragment string) (string, error) {
	if strings.TrimSpace(fragment) == "" {
		return "", nil
	}
	low := strings.ToLower(fragment)
	if strings.Contains(low, "<!doctype") || strings.Contains(low, "<html") || strings.Contains(low, "<head") || strings.Contains(low, "<body") {
		return "", fmt.Errorf("_image_to_html must return a fragment, not a document")
	}
	return fragment, nil
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
		return nil, wrapCorrupt(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	changed := false
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			zw.Close()
			return nil, wrapCorrupt(err)
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
			return nil, wrapCorrupt(err)
		}
		if _, err := w.Write(body); err != nil {
			zw.Close()
			return nil, wrapCorrupt(err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, wrapCorrupt(err)
	}
	if !changed {
		return data, nil
	}
	return buf.Bytes(), nil
}
