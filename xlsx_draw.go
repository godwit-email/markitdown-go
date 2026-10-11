package markitdown

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/url"
	"path"
	"strings"
)

func parseOfficeXML(data []byte) (*xnode, error) {
	low := bytes.ToLower(data)
	if bytes.Contains(low, []byte("<!doctype")) || bytes.Contains(low, []byte("<!entity")) {
		return nil, fmt.Errorf("%w: xml entities are forbidden", ErrCorrupt)
	}
	return parseXML(data)
}

func findAll(n *xnode, local string) []*xnode {
	if n == nil {
		return nil
	}
	var out []*xnode
	if n.local == local {
		out = append(out, n)
	}
	for _, c := range n.kids {
		out = append(out, findAll(c, local)...)
	}
	return out
}

func corruptf(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrCorrupt}, args...)...)
}

type zipRel struct {
	id     string
	target string
}

func xlsxSheetImages(data []byte) (map[string][]EmbeddedImage, error) {
	zr, err := openZip(data)
	if err != nil {
		return nil, err
	}
	wbRels := zipRels(zr, "_rels/.rels", "officeDocument")
	if len(wbRels) == 0 {
		return map[string][]EmbeddedImage{}, nil
	}
	wbPart := wbRels[0].target
	wb, err := readZip(zr, wbPart)
	if err != nil {
		return nil, err
	}
	wbXML, err := parseOfficeXML(wb)
	if err != nil {
		return nil, err
	}
	byID := map[string]string{}
	for _, rel := range zipRels(zr, relsPath(wbPart), "") {
		byID[rel.id] = rel.target
	}
	defaults, overrides := contentTypes(zr)
	out := map[string][]EmbeddedImage{}
	for _, sheet := range findAll(wbXML, "sheet") {
		part := byID[sheet.attr["id"]]
		if part == "" {
			continue
		}
		body, err := readZip(zr, part)
		if err != nil {
			return nil, err
		}
		sheetXML, err := parseOfficeXML(body)
		if err != nil {
			return nil, err
		}
		drawings := findAll(sheetXML, "drawing")
		if len(drawings) == 0 {
			continue
		}
		drawByID := map[string]string{}
		for _, rel := range zipRels(zr, relsPath(part), "drawing") {
			drawByID[rel.id] = rel.target
		}
		var images []EmbeddedImage
		for _, drawing := range drawings {
			drawPart := drawByID[drawing.attr["id"]]
			if drawPart == "" {
				return nil, corruptf("missing drawing relationship")
			}
			drawBody, err := readZip(zr, drawPart)
			if err != nil {
				return nil, err
			}
			drawXML, err := parseOfficeXML(drawBody)
			if err != nil {
				return nil, err
			}
			imageByID := map[string]string{}
			for _, rel := range zipRels(zr, relsPath(drawPart), "image") {
				imageByID[rel.id] = rel.target
			}
			for _, kind := range []string{"absoluteAnchor", "oneCellAnchor", "twoCellAnchor"} {
				for _, anchor := range direct(drawXML, kind) {
					for _, blip := range findAll(anchor, "blip") {
						rid := blip.attr["embed"]
						if rid == "" {
							continue
						}
						imagePart, ok := imageByID[rid]
						if !ok {
							return nil, corruptf("missing image relationship %s", rid)
						}
						blob, err := readZip(zr, imagePart)
						if err != nil {
							return nil, err
						}
						images = append(images, imageInfo(imagePart, blob, defaults, overrides))
					}
				}
			}
		}
		if len(images) > 0 {
			out[sheet.attr["name"]] = images
		}
	}
	return out, nil
}

func relsPath(part string) string {
	if part == "" {
		return "_rels/.rels"
	}
	return path.Join(path.Dir(part), "_rels", path.Base(part)+".rels")
}

func resolveRel(part, target string) string {
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

func zipRels(zr *zip.Reader, part, kind string) []zipRel {
	body, err := readZip(zr, part)
	if err != nil {
		return nil
	}
	root, err := parseOfficeXML(body)
	if err != nil {
		return nil
	}
	dirPart := strings.TrimSuffix(part, "/_rels/"+path.Base(part))
	if part == "_rels/.rels" {
		dirPart = ""
	}
	var out []zipRel
	for _, rel := range findAll(root, "Relationship") {
		if strings.EqualFold(rel.attr["TargetMode"], "External") {
			continue
		}
		typ := rel.attr["Type"]
		if kind != "" && !strings.HasSuffix(typ, "/"+kind) {
			continue
		}
		out = append(out, zipRel{
			id:     rel.attr["Id"],
			target: resolveRel(dirPart, rel.attr["Target"]),
		})
	}
	return out
}

func contentTypes(zr *zip.Reader) (defaults, overrides map[string]string) {
	defaults = map[string]string{}
	overrides = map[string]string{}
	body, err := readZip(zr, "[Content_Types].xml")
	if err != nil {
		return defaults, overrides
	}
	root, err := parseOfficeXML(body)
	if err != nil {
		return defaults, overrides
	}
	for _, n := range root.kids {
		switch n.local {
		case "Default":
			defaults[strings.ToLower(n.attr["Extension"])] = n.attr["ContentType"]
		case "Override":
			part := n.attr["PartName"]
			if un, err := url.PathUnescape(part); err == nil {
				part = un
			}
			overrides[strings.TrimPrefix(part, "/")] = n.attr["ContentType"]
		}
	}
	return defaults, overrides
}

func imageInfo(part string, blob []byte, defaults, overrides map[string]string) EmbeddedImage {
	ext := strings.ToLower(path.Ext(part))
	mime := overrides[part]
	if mime == "" {
		mime = defaults[strings.TrimPrefix(ext, ".")]
	}
	return EmbeddedImage{
		Data: blob,
		Name: path.Base(part),
		MIME: mime,
		Ext:  ext,
	}
}
