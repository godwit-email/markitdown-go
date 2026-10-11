package markitdown

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
)

// shortcut: no LLM client, so an image has no caption. Metadata is read from
// the file itself; exiftool is not invoked.
// shortcut: no speech recognizer and no ffmpeg, so audio has no transcript.

var imageMetaFields = []string{
	"ImageSize", "Title", "Caption", "Description", "Keywords", "Artist",
	"Author", "DateTimeOriginal", "CreateDate", "GPSPosition",
}

var audioMetaFields = []string{
	"Title", "Artist", "Author", "Band", "Album", "Genre", "Track",
	"DateTimeOriginal", "CreateDate", "NumChannels", "SampleRate",
	"AvgBytesPerSec", "BitsPerSample",
}

func convertImage(data []byte) (Result, error) {
	return Result{Text: formatMeta(jpegMeta(data), imageMetaFields)}, nil
}

func convertAudio(data []byte, _, _ string) (Result, error) {
	meta := map[string]string{}
	if id3, ok := readID3(data); ok {
		for k, v := range id3 {
			meta[k] = v
		}
		if rate := mpegSampleRate(data[len(id3Header(data)):]); rate != "" {
			meta["SampleRate"] = rate
		}
	}
	return Result{Text: strings.TrimSpace(formatMeta(meta, audioMetaFields))}, nil
}

func formatMeta(meta map[string]string, fields []string) string {
	var b strings.Builder
	for _, field := range fields {
		if v, ok := meta[field]; ok && v != "" {
			b.WriteString(field)
			b.WriteString(": ")
			b.WriteString(v)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func jpegMeta(data []byte) map[string]string {
	meta := map[string]string{}
	if w, h, ok := jpegSize(data); ok {
		meta["ImageSize"] = fmt.Sprintf("%dx%d", w, h)
	}
	if date := exifDateTimeOriginal(data); date != "" {
		meta["DateTimeOriginal"] = date
	}
	if xmp := xmpPacket(data); xmp != "" {
		if title := xmpLeaf(xmp, "dc:title"); title != "" {
			meta["Title"] = title
		}
		if desc := xmpLeaf(xmp, "dc:description"); desc != "" {
			meta["Description"] = desc
		}
		if author := xmlSimple(xmp, "pdf:Author"); author != "" {
			meta["Author"] = author
		}
	}
	return meta
}

func jpegSize(data []byte) (int, int, bool) {
	for i := 0; i+9 < len(data); {
		if data[i] != 0xFF {
			i++
			continue
		}
		marker := data[i+1]
		if marker == 0xD8 || marker == 0xD9 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			i += 2
			continue
		}
		if i+4 > len(data) {
			break
		}
		seglen := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if marker >= 0xC0 && marker <= 0xC2 && i+9 <= len(data) {
			h := int(binary.BigEndian.Uint16(data[i+5 : i+7]))
			w := int(binary.BigEndian.Uint16(data[i+7 : i+9]))
			if w > 0 && h > 0 {
				return w, h, true
			}
		}
		if seglen < 2 {
			break
		}
		i += 2 + seglen
	}
	return 0, 0, false
}

func xmpPacket(data []byte) string {
	const sig = "http://ns.adobe.com/xap/1.0/\x00"
	i := strings.Index(string(data), sig)
	if i < 0 {
		return ""
	}
	rest := data[i+len(sig):]
	end := strings.Index(string(rest), "</x:xmpmeta>")
	if end < 0 {
		return string(rest)
	}
	return string(rest[:end])
}

func xmpLeaf(xmp, tag string) string {
	open := "<" + tag
	i := strings.Index(xmp, open)
	if i < 0 {
		return ""
	}
	chunk := xmp[i:]
	if j := strings.Index(chunk, "</"+tag+">"); j >= 0 {
		chunk = chunk[:j]
	}
	li := strings.Index(chunk, "<rdf:li")
	if li < 0 {
		return ""
	}
	gt := strings.Index(chunk[li:], ">")
	if gt < 0 {
		return ""
	}
	start := li + gt + 1
	end := strings.Index(chunk[start:], "<")
	if end < 0 {
		return strings.TrimSpace(chunk[start:])
	}
	return strings.TrimSpace(chunk[start : start+end])
}

func xmlSimple(xmp, tag string) string {
	open := "<" + tag + ">"
	i := strings.Index(xmp, open)
	if i < 0 {
		return ""
	}
	rest := xmp[i+len(open):]
	j := strings.Index(rest, "<")
	if j < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:j])
}

func exifDateTimeOriginal(data []byte) string {
	i := strings.Index(string(data), "Exif\x00\x00")
	if i < 0 || i+8 > len(data) {
		return ""
	}
	tiff := data[i+6:]
	if len(tiff) < 8 {
		return ""
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return ""
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return ""
	}
	ifd := int(order.Uint32(tiff[4:8]))
	exif := ifdPointer(tiff, order, ifd, 0x8769)
	if exif < 0 {
		return ""
	}
	return ifdASCII(tiff, order, exif, 0x9003)
}

func ifdPointer(tiff []byte, order binary.ByteOrder, ifd int, want uint16) int {
	if ifd < 0 || ifd+2 > len(tiff) {
		return -1
	}
	n := int(order.Uint16(tiff[ifd : ifd+2]))
	for i := 0; i < n; i++ {
		e := ifd + 2 + i*12
		if e+12 > len(tiff) {
			return -1
		}
		tag := order.Uint16(tiff[e : e+2])
		if tag != want {
			continue
		}
		return int(order.Uint32(tiff[e+8 : e+12]))
	}
	return -1
}

func ifdASCII(tiff []byte, order binary.ByteOrder, ifd int, want uint16) string {
	if ifd < 0 || ifd+2 > len(tiff) {
		return ""
	}
	n := int(order.Uint16(tiff[ifd : ifd+2]))
	for i := 0; i < n; i++ {
		e := ifd + 2 + i*12
		if e+12 > len(tiff) {
			return ""
		}
		tag := order.Uint16(tiff[e : e+2])
		typ := order.Uint16(tiff[e+2 : e+4])
		cnt := int(order.Uint32(tiff[e+4 : e+8]))
		if tag != want || typ != 2 || cnt <= 0 {
			continue
		}
		var raw []byte
		if cnt <= 4 {
			raw = tiff[e+8 : e+8+cnt]
		} else {
			off := int(order.Uint32(tiff[e+8 : e+12]))
			if off < 0 || off+cnt > len(tiff) {
				return ""
			}
			raw = tiff[off : off+cnt]
		}
		return strings.TrimRight(string(raw), "\x00")
	}
	return ""
}

func readID3(data []byte) (map[string]string, bool) {
	if len(data) < 10 || string(data[:3]) != "ID3" {
		return nil, false
	}
	ver := data[3]
	size := synchsafe(data[6:10])
	if ver == 3 {
		size = int(binary.BigEndian.Uint32(data[6:10]))
	}
	if size < 0 || 10+size > len(data) {
		return nil, false
	}
	meta := map[string]string{}
	i := 10
	end := 10 + size
	for i+10 <= end {
		id := string(data[i : i+4])
		if id == "\x00\x00\x00\x00" || data[i] == 0 {
			break
		}
		fsz := synchsafe(data[i+4 : i+8])
		if ver == 3 {
			fsz = int(binary.BigEndian.Uint32(data[i+4 : i+8]))
		}
		if fsz < 0 || i+10+fsz > end {
			break
		}
		body := data[i+10 : i+10+fsz]
		if name, ok := id3Name(id); ok && len(body) > 0 {
			if text := id3Text(body); text != "" {
				meta[name] = text
			}
		}
		i += 10 + fsz
	}
	if len(meta) == 0 {
		return nil, false
	}
	return meta, true
}

func id3Header(data []byte) []byte {
	if len(data) < 10 || string(data[:3]) != "ID3" {
		return nil
	}
	size := synchsafe(data[6:10])
	if data[3] == 3 {
		size = int(binary.BigEndian.Uint32(data[6:10]))
	}
	if 10+size > len(data) {
		return data[:10]
	}
	return data[:10+size]
}

func id3Name(id string) (string, bool) {
	switch id {
	case "TIT2":
		return "Title", true
	case "TPE1":
		return "Artist", true
	case "TPE2":
		return "Band", true
	case "TALB":
		return "Album", true
	case "TCON":
		return "Genre", true
	case "TRCK":
		return "Track", true
	default:
		return "", false
	}
}

func id3Text(body []byte) string {
	enc := body[0]
	raw := body[1:]
	switch enc {
	case 0:
		return strings.TrimRight(string(raw), "\x00")
	case 1:
		if len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xFE {
			return utf16LE(raw[2:])
		}
		if len(raw) >= 2 && raw[0] == 0xFE && raw[1] == 0xFF {
			return utf16BE(raw[2:])
		}
		return utf16LE(raw)
	case 2:
		return utf16BE(raw)
	default:
		return strings.TrimRight(string(raw), "\x00")
	}
}

func utf16LE(b []byte) string {
	if len(b)%2 == 1 {
		b = b[:len(b)-1]
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[i*2:])
	}
	return strings.TrimRight(string(utf16.Decode(u)), "\x00")
}

func utf16BE(b []byte) string {
	if len(b)%2 == 1 {
		b = b[:len(b)-1]
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.BigEndian.Uint16(b[i*2:])
	}
	return strings.TrimRight(string(utf16.Decode(u)), "\x00")
}

func synchsafe(b []byte) int {
	return int(b[0]&0x7F)<<21 | int(b[1]&0x7F)<<14 | int(b[2]&0x7F)<<7 | int(b[3]&0x7F)
}

func mpegSampleRate(data []byte) string {
	for i := 0; i+4 <= len(data) && i < 4096; i++ {
		if data[i] != 0xFF || data[i+1]&0xE0 != 0xE0 {
			continue
		}
		h := binary.BigEndian.Uint32(data[i : i+4])
		ver := (h >> 19) & 3
		layer := (h >> 17) & 3
		idx := (h >> 10) & 3
		if ver == 1 || layer == 0 || idx == 3 {
			continue
		}
		table := []int{44100, 48000, 32000}
		switch ver {
		case 2:
			table = []int{22050, 24000, 16000}
		case 0:
			table = []int{11025, 12000, 8000}
		}
		return strconv.Itoa(table[idx])
	}
	return ""
}
