package table

import "strings"

// Markdown renders rows as a Markdown table. The first row is the header.
func Markdown(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	width := 0
	for _, row := range rows {
		if len(row) > width {
			width = len(row)
		}
	}
	if width == 0 {
		return ""
	}
	var b strings.Builder
	writeRow := func(row []string) {
		b.WriteByte('|')
		for i := 0; i < width; i++ {
			cell := ""
			if i < len(row) {
				cell = EscapeCell(row[i])
			}
			b.WriteByte(' ')
			b.WriteString(cell)
			b.WriteString(" |")
		}
		b.WriteByte('\n')
	}
	writeRow(rows[0])
	b.WriteByte('|')
	for i := 0; i < width; i++ {
		b.WriteString(" --- |")
	}
	b.WriteByte('\n')
	for _, row := range rows[1:] {
		writeRow(row)
	}
	return strings.TrimRight(b.String(), "\n")
}

// EscapeCell doubles a run of backslashes in front of a pipe, then escapes
// the pipe. Newlines inside a cell become a single space.
func EscapeCell(s string) string {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	var b strings.Builder
	bs := 0
	flush := func(beforePipe bool) {
		n := bs
		if beforePipe {
			n *= 2
		}
		for i := 0; i < n; i++ {
			b.WriteByte('\\')
		}
		if beforePipe {
			b.WriteString(`\|`)
		}
		bs = 0
	}
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			bs++
		case '|':
			flush(true)
		default:
			if bs > 0 {
				flush(false)
			}
			b.WriteByte(s[i])
		}
	}
	if bs > 0 {
		flush(false)
	}
	return b.String()
}
