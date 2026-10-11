// Package xmlsafe decodes XML and rejects documents that use a doctype.
// It does not fetch external entities.
package xmlsafe

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"

	"github.com/godwit-email/markitdown-go/internal/run"
)

// Node is one element or a #text child.
type Node struct {
	Local  string
	Space  string
	Attr   map[string]string
	Text   string
	Kids   []*Node
	Parent *Node
}

// Parse decodes the first element. A doctype or a broken document is ErrCorrupt.
func Parse(data []byte) (*Node, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = func(charset string, _ io.Reader) (io.Reader, error) {
		return nil, run.Wrap(errCharset(charset))
	}
	dec.Strict = true
	dec.Entity = map[string]string{}
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, run.Wrap(err)
		}
		switch t := tok.(type) {
		case xml.Directive:
			return nil, run.ErrCorrupt
		case xml.StartElement:
			return parseElem(dec, t)
		}
	}
}

func parseElem(dec *xml.Decoder, start xml.StartElement) (*Node, error) {
	n := &Node{Local: start.Name.Local, Space: start.Name.Space}
	if len(start.Attr) > 0 {
		n.Attr = make(map[string]string, len(start.Attr))
		for _, a := range start.Attr {
			key := a.Name.Local
			if a.Name.Space == "http://www.w3.org/XML/1998/namespace" {
				key = "xml:" + a.Name.Local
			}
			n.Attr[key] = a.Value
		}
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, run.Wrap(err)
		}
		switch t := tok.(type) {
		case xml.Directive:
			return nil, run.ErrCorrupt
		case xml.StartElement:
			c, err := parseElem(dec, t)
			if err != nil {
				return nil, err
			}
			c.Parent = n
			n.Kids = append(n.Kids, c)
		case xml.EndElement:
			return n, nil
		case xml.CharData:
			s := string(append([]byte(nil), t...))
			n.Text += s
			n.Kids = append(n.Kids, &Node{Local: "#text", Text: s, Parent: n})
		}
	}
}

type errCharset string

func (e errCharset) Error() string { return string(e) }

// Direct returns the immediate children with that local name.
func Direct(n *Node, local string) []*Node {
	if n == nil {
		return nil
	}
	var out []*Node
	for _, c := range n.Kids {
		if c.Local == local {
			out = append(out, c)
		}
	}
	return out
}

// FindAll returns every descendant with that local name, not including n.
func FindAll(n *Node, local string) []*Node {
	if n == nil {
		return nil
	}
	var out []*Node
	var walk func(*Node)
	walk = func(n *Node) {
		for _, c := range n.Kids {
			if c.Local == local {
				out = append(out, c)
			}
			walk(c)
		}
	}
	walk(n)
	return out
}

// FindFirst returns n or the first descendant with that local name.
func FindFirst(n *Node, local string) *Node {
	if n == nil {
		return nil
	}
	if n.Local == local {
		return n
	}
	for _, c := range n.Kids {
		if f := FindFirst(c, local); f != nil {
			return f
		}
	}
	return nil
}

// LocalText concatenates the text of every element with that local name.
func LocalText(n *Node, tag string) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var walk func(*Node)
	walk = func(n *Node) {
		if n.Local == tag {
			b.WriteString(n.Text)
			for _, c := range n.Kids {
				walk(c)
			}
			return
		}
		for _, c := range n.Kids {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}
