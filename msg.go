package markitdown

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/richardlehane/mscfb"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	xunicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

const (
	msgPropStream = "__properties_version1.0"
	msgHeaderSize = 32
	msgEntrySize  = 16
	ptLong        = 0x0003
	prCodepage    = 0x3FFD
	prInternetCP  = 0x3FDE
)

func convertMSG(data []byte) (Result, error) {
	streams, err := msgStreams(data)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: renderMSG(streams)}, nil
}

func msgStreams(data []byte) (map[string][]byte, error) {
	r, err := mscfb.New(bytes.NewReader(data))
	if err != nil {
		return nil, wrapCorrupt(err)
	}
	out := map[string][]byte{}
	for {
		f, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, wrapCorrupt(err)
		}
		if f.FileInfo().IsDir() || f.Size == 0 {
			continue
		}
		buf := make([]byte, f.Size)
		n, err := f.Read(buf)
		if err != nil && !errors.Is(err, io.EOF) {
			continue
		}
		out[f.Name] = buf[:n]
	}
	if len(out) == 0 {
		return nil, ErrCorrupt
	}
	return out, nil
}

func renderMSG(streams map[string][]byte) string {
	longs := msgLongProps(streams[msgPropStream])
	headerEnc := codepageEncoding(longs[prCodepage])
	bodyEnc := codepageEncoding(longs[prInternetCP])
	if headerEnc == nil {
		headerEnc = bodyEnc
	}
	if bodyEnc == nil {
		bodyEnc = headerEnc
	}
	headers := []struct{ key, tag string }{
		{"From", "0C1F"},
		{"To", "0E04"},
		{"Subject", "0037"},
	}
	var b strings.Builder
	b.WriteString("# Email Message\n\n")
	for _, h := range headers {
		if v := msgString(streams, h.tag, headerEnc); v != "" {
			b.WriteString("**")
			b.WriteString(h.key)
			b.WriteString(":** ")
			b.WriteString(v)
			b.WriteByte('\n')
		}
	}
	b.WriteString("\n## Content\n\n")
	if body := msgString(streams, "1000", bodyEnc); body != "" {
		b.WriteString(body)
	}
	return strings.TrimSpace(b.String())
}

func msgLongProps(data []byte) map[int]int {
	out := map[int]int{}
	for off := msgHeaderSize; off+msgEntrySize <= len(data); off += msgEntrySize {
		tag := binary.LittleEndian.Uint32(data[off : off+4])
		value := binary.LittleEndian.Uint32(data[off+8 : off+12])
		if tag&0xFFFF == ptLong {
			out[int(tag>>16)] = int(value)
		}
	}
	return out
}

func msgString(streams map[string][]byte, tag string, enc encoding.Encoding) string {
	if v := decodeUTF16(streams["__substg1.0_"+tag+"001F"]); v != "" {
		return v
	}
	return decodeANSI(streams["__substg1.0_"+tag+"001E"], enc)
}

func decodeUTF16(data []byte) string {
	if len(data) < 2 {
		return ""
	}
	if len(data)%2 == 1 {
		data = data[:len(data)-1]
	}
	u := make([]uint16, len(data)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(data[i*2:])
	}
	return strings.TrimSpace(strings.ReplaceAll(string(utf16.Decode(u)), "\x00", ""))
}

func decodeANSI(data []byte, enc encoding.Encoding) string {
	data = bytes.TrimRight(data, "\x00")
	if len(data) == 0 {
		return ""
	}
	if enc != nil {
		if s, err := decodeStrict(enc, data); err == nil {
			return strings.TrimSpace(s)
		}
	}
	return strings.TrimSpace(detect8bit(data))
}

func decodeStrict(enc encoding.Encoding, data []byte) (string, error) {
	s, _, err := transform.Bytes(enc.NewDecoder(), data)
	if err != nil {
		return "", err
	}
	if bytes.Contains(s, []byte{0xEF, 0xBF, 0xBD}) && !bytes.Contains(data, []byte{0xEF, 0xBF, 0xBD}) {
		// x/text replaces invalid bytes; a declared page that does not fit must fall back.
		if !utf8.Valid(data) {
			return "", errors.New("undecodable")
		}
	}
	return string(s), nil
}

func detect8bit(data []byte) string {
	if utf8.Valid(data) && hasHigh(data) {
		return string(data)
	}
	s, _, err := transform.Bytes(charmap.Windows1252.NewDecoder(), data)
	if err != nil {
		return string(bytes.ToValidUTF8(data, nil))
	}
	return string(s)
}

func hasHigh(data []byte) bool {
	for _, b := range data {
		if b >= 0x80 {
			return true
		}
	}
	return false
}

func codepageEncoding(cp int) encoding.Encoding {
	switch cp {
	case 0:
		return nil
	case 1250:
		return charmap.Windows1250
	case 1251:
		return charmap.Windows1251
	case 1252:
		return charmap.Windows1252
	case 1253:
		return charmap.Windows1253
	case 1254:
		return charmap.Windows1254
	case 1255:
		return charmap.Windows1255
	case 1256:
		return charmap.Windows1256
	case 1257:
		return charmap.Windows1257
	case 1258:
		return charmap.Windows1258
	case 932:
		return japanese.ShiftJIS
	case 936:
		return simplifiedchinese.GBK
	case 949:
		return korean.EUCKR
	case 950:
		return traditionalchinese.Big5
	case 20127:
		return asciiEncoding{}
	case 28591:
		return charmap.ISO8859_1
	case 28595:
		return charmap.ISO8859_5
	case 28597:
		return charmap.ISO8859_7
	case 65001:
		return xunicode.UTF8
	case 50220, 50221, 50222:
		return iso2022Encoding{cp: cp}
	default:
		return nil
	}
}

// asciiEncoding rejects any byte above 7-bit ASCII (Windows code page 20127).
type asciiEncoding struct{}

func (asciiEncoding) NewDecoder() *encoding.Decoder {
	return &encoding.Decoder{Transformer: asciiTransform{}}
}
func (asciiEncoding) NewEncoder() *encoding.Encoder {
	return &encoding.Encoder{Transformer: asciiTransform{}}
}

type asciiTransform struct{}

func (asciiTransform) Reset() {}
func (asciiTransform) Transform(dst, src []byte, atEOF bool) (int, int, error) {
	n := len(src)
	if n > len(dst) {
		n = len(dst)
	}
	for i := 0; i < n; i++ {
		if src[i] >= 0x80 {
			return i, i, errors.New("ascii")
		}
		dst[i] = src[i]
	}
	if n < len(src) {
		return n, n, transform.ErrShortDst
	}
	return n, n, nil
}

type iso2022Encoding struct{ cp int }

func (e iso2022Encoding) NewDecoder() *encoding.Decoder {
	return &encoding.Decoder{Transformer: iso2022Transform{}}
}
func (e iso2022Encoding) NewEncoder() *encoding.Encoder {
	return &encoding.Encoder{Transformer: iso2022Transform{}}
}

type iso2022Transform struct{}

func (iso2022Transform) Reset() {}
func (iso2022Transform) Transform(dst, src []byte, atEOF bool) (int, int, error) {
	s, err := decodeISO2022JP(src)
	if err != nil {
		return 0, 0, err
	}
	if len(s) > len(dst) {
		return 0, 0, transform.ErrShortDst
	}
	copy(dst, s)
	return len(s), len(src), nil
}

func decodeISO2022JP(data []byte) (string, error) {
	const (
		ascii = iota
		roman
		kana
		dbcs
	)
	mode := ascii
	saved := ascii
	var b strings.Builder
	var pair []byte
	euc := japanese.EUCJP.NewDecoder()
	for i := 0; i < len(data); i++ {
		c := data[i]
		if c == 0x1b {
			if i+2 < len(data) && data[i+1] == '(' {
				switch data[i+2] {
				case 'B':
					mode = ascii
					i += 2
					continue
				case 'J':
					mode = roman
					i += 2
					continue
				case 'I':
					mode = kana
					i += 2
					continue
				}
			}
			if i+2 < len(data) && data[i+1] == '$' && (data[i+2] == 'B' || data[i+2] == '@') {
				mode = dbcs
				i += 2
				continue
			}
			if i+3 < len(data) && data[i+1] == '$' && data[i+2] == '(' && data[i+3] == 'D' {
				mode = dbcs
				i += 3
				continue
			}
		}
		if c == 0x0e {
			saved = mode
			mode = kana
			continue
		}
		if c == 0x0f {
			mode = saved
			continue
		}
		switch mode {
		case ascii:
			b.WriteByte(c)
		case roman:
			switch c {
			case '\\':
				b.WriteRune('¥')
			case '~':
				b.WriteRune('‾')
			default:
				b.WriteByte(c)
			}
		case kana:
			if c < 0x21 || c > 0x5F {
				return "", errors.New("kana")
			}
			b.WriteRune(rune(0xFF61 + int(c-0x21)))
		case dbcs:
			pair = append(pair, c)
			if len(pair) == 2 {
				out, err := euc.Bytes([]byte{pair[0] + 0x80, pair[1] + 0x80})
				if err != nil || bytes.Contains(out, []byte("\uFFFD")) {
					return "", errors.New("dbcs")
				}
				b.Write(out)
				pair = pair[:0]
			}
		}
	}
	if len(pair) != 0 {
		return "", errors.New("dbcs")
	}
	return b.String(), nil
}
