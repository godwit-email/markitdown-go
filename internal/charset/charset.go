// Package charset decodes text using a MIME charset. An unknown charset is UTF-8.
package charset

import (
	"bytes"
	"io"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/ianaindex"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

// Decode decodes data. Empty or unknown charset, and UTF-8, replace invalid
// bytes with U+FFFD and strip a leading BOM. It does not return an error.
func Decode(data []byte, charsetName string) string {
	charsetName = strings.TrimSpace(charsetName)
	if charsetName == "" || isUTF8(charsetName) {
		data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
		return string(bytes.ToValidUTF8(data, []byte("\uFFFD")))
	}
	enc := find(charsetName)
	if enc == nil {
		data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
		return string(bytes.ToValidUTF8(data, []byte("\uFFFD")))
	}
	out, err := io.ReadAll(enc.NewDecoder().Reader(bytes.NewReader(data)))
	if err != nil {
		return string(bytes.ToValidUTF8(data, []byte("\uFFFD")))
	}
	return strings.TrimPrefix(string(out), "\uFEFF")
}

func isUTF8(charsetName string) bool {
	switch strings.ToLower(charsetName) {
	case "utf-8", "utf8", "utf-8-sig", "ascii", "us-ascii", "ansi_x3.4-1968":
		return true
	default:
		return false
	}
}

func find(charsetName string) encoding.Encoding {
	c := strings.ToLower(strings.TrimSpace(charsetName))
	c = strings.ReplaceAll(c, "_", "-")
	switch c {
	case "cp1252", "windows-1252", "ibm1252":
		return charmap.Windows1252
	case "cp932", "shift-jis", "sjis", "windows-31j", "ms932", "csshiftjis":
		return japanese.ShiftJIS
	case "gbk", "cp936":
		return simplifiedchinese.GBK
	case "gb18030", "gb2312":
		return simplifiedchinese.GB18030
	case "big5", "cp950":
		return traditionalchinese.Big5
	case "iso-8859-1", "latin1", "latin-1":
		return charmap.ISO8859_1
	}
	if enc, err := ianaindex.IANA.Encoding(c); err == nil && enc != nil {
		return enc
	}
	if enc, err := ianaindex.MIME.Encoding(charsetName); err == nil && enc != nil {
		return enc
	}
	return nil
}
