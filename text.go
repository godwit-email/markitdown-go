package markitdown

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

func convertText(data []byte, charset string) (Result, error) {
	text, err := decodeText(data, charset)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text}, nil
}

func decodeText(data []byte, charset string) (string, error) {
	charset = strings.TrimSpace(charset)
	if charset == "" || isUTF8Charset(charset) {
		data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
		return string(bytes.ToValidUTF8(data, []byte("\uFFFD"))), nil
	}
	enc := findEncoding(charset)
	if enc == nil {
		return "", wrapCorrupt(errUnknownCharset(charset))
	}
	out, err := io.ReadAll(enc.NewDecoder().Reader(bytes.NewReader(data)))
	if err != nil {
		return "", wrapCorrupt(err)
	}
	s := string(out)
	s = strings.TrimPrefix(s, "\uFEFF")
	return s, nil
}

func findEncoding(charset string) encoding.Encoding {
	c := strings.ToLower(strings.TrimSpace(charset))
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
	if enc, err := ianaindex.MIME.Encoding(charset); err == nil && enc != nil {
		return enc
	}
	return nil
}

func isUTF8Charset(charset string) bool {
	switch strings.ToLower(charset) {
	case "utf-8", "utf8", "ascii", "us-ascii", "ansi_x3.4-1968":
		return true
	default:
		return false
	}
}

type charsetError string

func (e charsetError) Error() string { return string(e) }

func errUnknownCharset(charset string) error {
	return charsetError("unknown charset " + charset)
}
