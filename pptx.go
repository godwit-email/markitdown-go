package markitdown

import "strings"

func convertPPTX(data []byte) (Result, error) {
	zr, err := openZip(data)
	if err != nil {
		return Result{}, err
	}
	names := slideNames(zr)
	if len(names) == 0 {
		if _, err := readZip(zr, "ppt/presentation.xml"); err != nil {
			return Result{}, ErrCorrupt
		}
		return Result{}, nil
	}
	var b strings.Builder
	for _, name := range names {
		body, err := readZip(zr, name)
		if err != nil {
			return Result{}, ErrCorrupt
		}
		root, err := parseXML(body)
		if err != nil {
			return Result{}, err
		}
		slide := strings.TrimSpace(renderSlide(root))
		notes := notesFor(zr, name)
		if slide == "" && notes == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(slide)
		if notes != "" {
			if slide != "" {
				b.WriteString("\n\n")
			}
			b.WriteString("### Notes:\n")
			b.WriteString(notes)
		}
	}
	return Result{Text: strings.TrimSpace(b.String())}, nil
}
