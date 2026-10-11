// Package ooxml opens an Office package and enforces the 25 MB part budget.
package ooxml

import (
	"archive/zip"
	"bytes"
	"io"
	"net/url"
	"path"
	"strings"

	"github.com/godwit-email/markitdown-go/internal/run"
	"github.com/godwit-email/markitdown-go/internal/xmlsafe"
)

const maxPart = 25 << 20

// Package is an OOXML zip with a running uncompressed total.
type Package struct {
	zr   *zip.Reader
	used int64
}

// Open reads a zip. A broken container is ErrCorrupt.
func Open(data []byte) (*Package, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, run.ErrCorrupt
	}
	return &Package{zr: zr}, nil
}

// Zip returns the underlying reader.
func (p *Package) Zip() *zip.Reader { return p.zr }

// Read returns one part. A missing part, a part over 25 MB, or a package
// whose parts sum past 25 MB is ErrCorrupt.
func (p *Package) Read(name string) ([]byte, error) {
	if p == nil || p.zr == nil {
		return nil, run.ErrCorrupt
	}
	for _, f := range p.zr.File {
		if f.Name != name {
			continue
		}
		if f.UncompressedSize64 > maxPart || p.used+int64(f.UncompressedSize64) > maxPart {
			return nil, run.ErrCorrupt
		}
		rc, err := f.Open()
		if err != nil {
			return nil, run.ErrCorrupt
		}
		b, err := io.ReadAll(io.LimitReader(rc, maxPart-p.used+1))
		rc.Close()
		if err != nil {
			return nil, run.ErrCorrupt
		}
		if int64(len(b)) > maxPart-p.used {
			return nil, run.ErrCorrupt
		}
		p.used += int64(len(b))
		return b, nil
	}
	return nil, run.ErrCorrupt
}

// Rel is one relationship whose target has been resolved to a part name.
type Rel struct {
	ID     string
	Target string
}

// Rels reads relationships of kind ("image", "chart", ...). An empty kind
// returns every internal relationship. A missing rels part is an empty list.
func Rels(p *Package, part, kind string) []Rel {
	body, err := p.Read(part)
	if err != nil {
		return nil
	}
	root, err := xmlsafe.Parse(body)
	if err != nil {
		return nil
	}
	dirPart := strings.TrimSuffix(part, "/_rels/"+path.Base(part))
	if part == "_rels/.rels" {
		dirPart = ""
	}
	var out []Rel
	for _, rel := range xmlsafe.FindAll(root, "Relationship") {
		if strings.EqualFold(rel.Attr["TargetMode"], "External") {
			continue
		}
		typ := rel.Attr["Type"]
		if kind != "" && !strings.HasSuffix(typ, "/"+kind) {
			continue
		}
		out = append(out, Rel{ID: rel.Attr["Id"], Target: resolve(dirPart, rel.Attr["Target"])})
	}
	return out
}

// ContentTypes returns Default and Override maps from [Content_Types].xml.
func ContentTypes(p *Package) (defaults, overrides map[string]string) {
	defaults = map[string]string{}
	overrides = map[string]string{}
	body, err := p.Read("[Content_Types].xml")
	if err != nil {
		return defaults, overrides
	}
	root, err := xmlsafe.Parse(body)
	if err != nil {
		return defaults, overrides
	}
	for _, n := range root.Kids {
		switch n.Local {
		case "Default":
			defaults[strings.ToLower(n.Attr["Extension"])] = n.Attr["ContentType"]
		case "Override":
			part := n.Attr["PartName"]
			if un, err := url.PathUnescape(part); err == nil {
				part = un
			}
			overrides[strings.TrimPrefix(part, "/")] = n.Attr["ContentType"]
		}
	}
	return defaults, overrides
}

// Image is an embedded image part.
type Image struct {
	Data []byte
	Name string
	MIME string
	Ext  string
}

// ImageInfo fills MIME from content types and the part extension.
func ImageInfo(part string, blob []byte, defaults, overrides map[string]string) Image {
	ext := strings.ToLower(path.Ext(part))
	mimeType := overrides[part]
	if mimeType == "" {
		mimeType = defaults[strings.TrimPrefix(ext, ".")]
	}
	return Image{Data: blob, Name: path.Base(part), MIME: mimeType, Ext: ext}
}

// RelsPath is the relationships part for an OOXML part.
func RelsPath(part string) string {
	if part == "" {
		return "_rels/.rels"
	}
	return path.Join(path.Dir(part), "_rels", path.Base(part)+".rels")
}

func resolve(part, target string) string {
	if un, err := url.PathUnescape(target); err == nil {
		target = un
	}
	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(path.Clean(target), "/")
	}
	if part == "" || part == "." {
		return strings.TrimPrefix(path.Clean(target), "/")
	}
	return strings.TrimPrefix(path.Clean(path.Join(part, target)), "/")
}
