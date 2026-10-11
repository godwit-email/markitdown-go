package markitdown

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
)

type xnode struct {
	local  string
	space  string
	attr   map[string]string
	text   string
	kids   []*xnode
	parent *xnode
}

func openZip(data []byte) (*zip.Reader, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, ErrCorrupt
	}
	return zr, nil
}

func readZip(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, ErrCorrupt
			}
			b, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, ErrCorrupt
			}
			return b, nil
		}
	}
	return nil, ErrCorrupt
}

func parseXML(data []byte) (*xnode, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = func(charset string, _ io.Reader) (io.Reader, error) {
		return nil, wrapCorrupt(charsetError("xml charset " + charset))
	}
	dec.Strict = true
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, wrapCorrupt(err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			return parseElem(dec, se)
		}
	}
}

func parseElem(dec *xml.Decoder, start xml.StartElement) (*xnode, error) {
	n := &xnode{local: start.Name.Local, space: start.Name.Space}
	if len(start.Attr) > 0 {
		n.attr = make(map[string]string, len(start.Attr))
		for _, a := range start.Attr {
			key := a.Name.Local
			if a.Name.Space == "http://www.w3.org/XML/1998/namespace" {
				key = "xml:" + a.Name.Local
			}
			n.attr[key] = a.Value
		}
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, wrapCorrupt(err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			c, err := parseElem(dec, t)
			if err != nil {
				return nil, err
			}
			c.parent = n
			n.kids = append(n.kids, c)
		case xml.EndElement:
			return n, nil
		case xml.CharData:
			s := string(append([]byte(nil), t...))
			n.text += s
			child := &xnode{local: "#text", text: s, parent: n}
			n.kids = append(n.kids, child)
		}
	}
}

func direct(n *xnode, local string) []*xnode {
	var out []*xnode
	for _, c := range n.kids {
		if c.local == local {
			out = append(out, c)
		}
	}
	return out
}

func findFirst(n *xnode, local string) *xnode {
	if n == nil {
		return nil
	}
	if n.local == local {
		return n
	}
	for _, c := range n.kids {
		if f := findFirst(c, local); f != nil {
			return f
		}
	}
	return nil
}

func localText(n *xnode, tag string) string {
	var b strings.Builder
	var walk func(*xnode)
	walk = func(n *xnode) {
		if n.local == tag {
			b.WriteString(n.text)
			for _, c := range n.kids {
				walk(c)
			}
			return
		}
		for _, c := range n.kids {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}
