package markitdown

import (
	"io"
	"unicode/utf8"
)

const charsetSampleSize = 65536

// readCharsetSample reads 64 KiB and, when that slice ends on a split UTF-8
// character, up to three more bytes so the sample can finish the character.
// Invalid UTF-8 is returned unchanged. Matches _read_charset_sample.
func readCharsetSample(r io.Reader) ([]byte, error) {
	buf := make([]byte, charsetSampleSize)
	n, err := io.ReadFull(r, buf)
	sample := buf[:n]
	if n < charsetSampleSize {
		if err == io.EOF || err == io.ErrUnexpectedEOF || err == nil {
			return sample, nil
		}
		return sample, err
	}
	state, bad := utf8PrefixState(sample)
	if bad || len(state) == 0 {
		return sample, nil
	}
	var suffix []byte
	one := make([]byte, 1)
	for i := 0; i < 3; i++ {
		n, err := r.Read(one)
		if n == 1 {
			suffix = append(suffix, one[0])
			done, derr := feedUTF8(&state, one[0])
			if derr != nil {
				return sample, nil
			}
			if done {
				out := make([]byte, 0, len(sample)+len(suffix))
				out = append(out, sample...)
				out = append(out, suffix...)
				return out, nil
			}
		}
		if n == 0 || err != nil {
			break
		}
	}
	return sample, nil
}

// utf8PrefixState walks sample with the same accept/reject timing as
// CPython's incremental UTF-8 decoder. incomplete is the trailing partial
// sequence; bad means the sample itself is not valid UTF-8.
func utf8PrefixState(sample []byte) (incomplete []byte, bad bool) {
	var state []byte
	for _, b := range sample {
		if _, err := feedUTF8(&state, b); err != nil {
			return nil, true
		}
	}
	return state, false
}

func feedUTF8(state *[]byte, b byte) (done bool, err error) {
	if len(*state) == 0 {
		need := utf8SeqLen(b)
		if need == 0 {
			return false, errBadUTF8
		}
		if need == 1 {
			return true, nil
		}
		*state = append(*state, b)
		return false, nil
	}
	if b&0xC0 != 0x80 || (len(*state) == 1 && !utf8ContOK((*state)[0], b)) {
		return false, errBadUTF8
	}
	*state = append(*state, b)
	if len(*state) < utf8SeqLen((*state)[0]) {
		return false, nil
	}
	if !utf8SeqValid(*state) {
		return false, errBadUTF8
	}
	*state = (*state)[:0]
	return true, nil
}

type utf8Error struct{}

func (utf8Error) Error() string { return "invalid utf-8" }

var errBadUTF8 error = utf8Error{}

func utf8SeqLen(b byte) int {
	switch {
	case b < 0x80:
		return 1
	case b >= 0xC2 && b <= 0xDF:
		return 2
	case b >= 0xE0 && b <= 0xEF:
		return 3
	case b >= 0xF0 && b <= 0xF4:
		return 4
	default:
		return 0
	}
}

func utf8ContOK(lead, b byte) bool {
	switch lead {
	case 0xE0:
		return b >= 0xA0
	case 0xED:
		return true
	case 0xF0:
		return b >= 0x90
	case 0xF4:
		return b <= 0x8F
	default:
		return true
	}
}

func utf8SeqValid(seq []byte) bool {
	return utf8.Valid(seq)
}
