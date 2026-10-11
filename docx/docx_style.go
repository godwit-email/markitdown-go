package docx

import (
	"bytes"
	"regexp"
)

const wordNS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

// repairStyles fills a missing w:type, drops styles with no styleId, and
// renames word-namespace dstrike to strike. Unchanged input is returned as-is.
func repairStyles(content []byte) []byte {
	prefixes := wordPrefixes(content)
	if len(prefixes) == 0 {
		return content
	}
	out := content
	changed := false
	for _, prefix := range prefixes {
		next, ok := repairPrefix(out, prefix)
		if ok {
			changed = true
			out = next
		}
	}
	if !changed {
		return content
	}
	return out
}

func wordPrefixes(content []byte) []string {
	re := regexp.MustCompile(`xmlns:([A-Za-z0-9]+)="` + regexp.QuoteMeta(wordNS) + `"`)
	var out []string
	seen := map[string]bool{}
	for _, m := range re.FindAllSubmatch(content, -1) {
		p := string(m[1])
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	if bytes.Contains(content, []byte(`xmlns="`+wordNS+`"`)) {
		out = append(out, "")
	}
	return out
}

func repairPrefix(content []byte, prefix string) ([]byte, bool) {
	name := "style"
	if prefix != "" {
		name = prefix + ":style"
	}
	re := regexp.MustCompile(`(?s)<` + regexp.QuoteMeta(name) + `\b([^>]*)>(.*?)</` + regexp.QuoteMeta(name) + `>`)
	changed := false
	out := re.ReplaceAllFunc(content, func(el []byte) []byte {
		m := re.FindSubmatch(el)
		if m == nil {
			return el
		}
		attrs := string(m[1])
		idAttr := "styleId="
		typeAttr := "type="
		if prefix != "" {
			idAttr = prefix + ":styleId="
			typeAttr = prefix + ":type="
		}
		if !stringsContains(attrs, idAttr) {
			changed = true
			return nil
		}
		body := m[2]
		if b, ok := renameDStrike(body, prefix); ok {
			body = b
			changed = true
		}
		if !stringsContains(attrs, typeAttr) {
			changed = true
			insert := " "
			if prefix != "" {
				insert += prefix + ":"
			}
			insert += `type="paragraph"`
			return []byte("<" + name + insert + attrs + ">" + string(body) + "</" + name + ">")
		}
		if bytes.Equal(body, m[2]) {
			return el
		}
		return []byte("<" + name + attrs + ">" + string(body) + "</" + name + ">")
	})
	return out, changed
}

func renameDStrike(body []byte, prefix string) ([]byte, bool) {
	open := "dstrike"
	if prefix != "" {
		open = prefix + ":dstrike"
	}
	if !bytes.Contains(body, []byte(open)) {
		return body, false
	}
	repl := "strike"
	if prefix != "" {
		repl = prefix + ":strike"
	}
	return bytes.ReplaceAll(body, []byte(open), []byte(repl)), true
}

func stringsContains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && bytes.Contains([]byte(s), []byte(sub)))
}
