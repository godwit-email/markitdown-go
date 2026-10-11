package markitdown

import "unicode/utf8"

// rtfSkip are destination groups whose text is not document content.
var rtfSkip = map[string]bool{
	"fonttbl": true, "colortbl": true, "stylesheet": true, "info": true,
	"pict": true, "object": true, "themedata": true, "datastore": true,
	"xmlnstbl": true, "rsidtbl": true, "latentstyles": true, "listtable": true,
	"listoverridetable": true, "revtbl": true, "pnseclvl": true, "wgrffmtfilter": true,
	"filetbl": true, "colorschememapping": true, "generator": true, "header": true,
	"footer": true, "footnote": true, "annotation": true, "fldinst": true,
	"field": true, "mmathpr": true, "defchp": true, "defpap": true, "panose": true,
	"pgptbl": true, "rsids": true, "userprops": true, "template": true,
	"docvar": true, "listtext": true, "nonshppict": true, "shppict": true,
	"background": true, "fontemb": true, "objdata": true, "result": true,
	"shpinst": true, "blipuid": true, "formfield": true, "datafield": true,
	"atnid": true, "atnauthor": true, "bkmkstart": true, "bkmkend": true,
}

func convertRTF(data []byte) (Result, error) {
	return Result{Text: rtfToText(data)}, nil
}

func rtfToText(data []byte) string {
	var b []byte
	i := 0
	depth := 0
	skipUntil := 0
	ucSkip := 1
	for i < len(data) {
		if skipUntil > 0 {
			c := data[i]
			switch c {
			case '{':
				depth++
				i++
			case '}':
				depth--
				i++
				if depth < skipUntil {
					skipUntil = 0
				}
			case '\\':
				i++
				if i < len(data) && data[i] == '\'' && i+2 < len(data) {
					i += 3
					continue
				}
				word, num, next, hasNum := readRTFControl(data, i)
				if word == "bin" && hasNum && num > 0 {
					next += num
					if next > len(data) {
						next = len(data)
					}
				}
				i = next
			default:
				i++
			}
			continue
		}
		c := data[i]
		switch c {
		case '{':
			if rtfDestSkip(data, i+1) {
				depth++
				skipUntil = depth
				i++
				continue
			}
			depth++
			i++
		case '}':
			if depth > 0 {
				depth--
			}
			i++
		case '\\':
			i++
			if i >= len(data) {
				break
			}
			if data[i] == '\'' && i+2 < len(data) {
				b = utf8.AppendRune(b, rtfByte(hexByte(data[i+1], data[i+2])))
				i += 3
				continue
			}
			word, num, next, hasNum := readRTFControl(data, i)
			i = next
			switch word {
			case "par", "line":
				b = append(b, '\n')
			case "tab":
				b = append(b, '\t')
			case "uc":
				if hasNum && num >= 0 {
					ucSkip = num
				}
			case "u":
				r := rune(num)
				if r < 0 {
					r += 65536
				}
				b = utf8.AppendRune(b, r)
				i = skipRTFFallback(data, i, ucSkip)
			case "bin":
				if hasNum && num > 0 {
					if i+num > len(data) {
						i = len(data)
					} else {
						i += num
					}
				}
			case "{", "}", "\\":
				b = append(b, word[0])
			}
		case '\r', '\n':
			i++
		default:
			b = append(b, c)
			i++
		}
	}
	return string(b)
}

func rtfDestSkip(data []byte, i int) bool {
	for i < len(data) && (data[i] == ' ' || data[i] == '\r' || data[i] == '\n') {
		i++
	}
	if i >= len(data) || data[i] != '\\' {
		return false
	}
	i++
	if i < len(data) && data[i] == '*' {
		return true
	}
	word, _, _, _ := readRTFControl(data, i)
	return rtfSkip[word]
}

func readRTFControl(data []byte, i int) (word string, num int, next int, hasNum bool) {
	if i >= len(data) {
		return "", 0, i, false
	}
	if data[i] < 'A' || (data[i] > 'Z' && data[i] < 'a') || data[i] > 'z' {
		return string(data[i : i+1]), 0, i + 1, false
	}
	start := i
	for i < len(data) {
		c := data[i]
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
			break
		}
		i++
	}
	word = asciiLower(data[start:i])
	if i < len(data) && (data[i] == '-' || (data[i] >= '0' && data[i] <= '9')) {
		hasNum = true
		sign := 1
		if data[i] == '-' {
			sign = -1
			i++
		}
		n := 0
		for i < len(data) && data[i] >= '0' && data[i] <= '9' {
			n = n*10 + int(data[i]-'0')
			i++
		}
		num = sign * n
	}
	if i < len(data) && data[i] == ' ' {
		i++
	}
	return word, num, i, hasNum
}

func skipRTFFallback(data []byte, i, n int) int {
	for skipped := 0; skipped < n && i < len(data); {
		if data[i] == '\\' && i+1 < len(data) && data[i+1] == '\'' && i+3 < len(data) {
			i += 4
			skipped++
			continue
		}
		if data[i] == '{' || data[i] == '}' || data[i] == '\\' {
			break
		}
		if data[i] == '\n' || data[i] == '\r' {
			i++
			continue
		}
		i++
		skipped++
	}
	return i
}

func hexByte(a, b byte) byte {
	return hexVal(a)<<4 | hexVal(b)
}

func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	default:
		return 0
	}
}

// rtfByte maps an ansicpg1252 byte. The sample plugin fixture is ASCII.
func rtfByte(b byte) rune {
	if b < 0x80 {
		return rune(b)
	}
	// windows-1252 bytes that are not latin-1.
	switch b {
	case 0x80:
		return 0x20AC
	case 0x82:
		return 0x201A
	case 0x83:
		return 0x0192
	case 0x84:
		return 0x201E
	case 0x85:
		return 0x2026
	case 0x86:
		return 0x2020
	case 0x87:
		return 0x2021
	case 0x88:
		return 0x02C6
	case 0x89:
		return 0x2030
	case 0x8A:
		return 0x0160
	case 0x8B:
		return 0x2039
	case 0x8C:
		return 0x0152
	case 0x8E:
		return 0x017D
	case 0x91:
		return 0x2018
	case 0x92:
		return 0x2019
	case 0x93:
		return 0x201C
	case 0x94:
		return 0x201D
	case 0x95:
		return 0x2022
	case 0x96:
		return 0x2013
	case 0x97:
		return 0x2014
	case 0x98:
		return 0x02DC
	case 0x99:
		return 0x2122
	case 0x9A:
		return 0x0161
	case 0x9B:
		return 0x203A
	case 0x9C:
		return 0x0153
	case 0x9E:
		return 0x017E
	case 0x9F:
		return 0x0178
	default:
		return rune(b)
	}
}

func asciiLower(b []byte) string {
	out := make([]byte, len(b))
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}
