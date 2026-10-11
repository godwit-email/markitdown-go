package xlsx

import (
	"fmt"

	"github.com/godwit-email/markitdown-go/internal/ooxml"
	"github.com/godwit-email/markitdown-go/internal/run"
	"github.com/godwit-email/markitdown-go/internal/xmlsafe"
)

func corruptf(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{run.ErrCorrupt}, args...)...)
}

type zipRel struct {
	id     string
	target string
}

func xlsxSheetImages(data []byte) (map[string][]ooxml.Image, error) {
	zr, err := ooxml.Open(data)
	if err != nil {
		return nil, err
	}
	wbRels := ooxml.Rels(zr, "_rels/.rels", "officeDocument")
	if len(wbRels) == 0 {
		return map[string][]ooxml.Image{}, nil
	}
	wbPart := wbRels[0].Target
	wb, err := zr.Read(wbPart)
	if err != nil {
		return nil, err
	}
	wbXML, err := xmlsafe.Parse(wb)
	if err != nil {
		return nil, err
	}
	byID := map[string]string{}
	for _, rel := range ooxml.Rels(zr, ooxml.RelsPath(wbPart), "") {
		byID[rel.ID] = rel.Target
	}
	defaults, overrides := ooxml.ContentTypes(zr)
	out := map[string][]ooxml.Image{}
	for _, sheet := range xmlsafe.FindAll(wbXML, "sheet") {
		part := byID[sheet.Attr["id"]]
		if part == "" {
			continue
		}
		body, err := zr.Read(part)
		if err != nil {
			return nil, err
		}
		sheetXML, err := xmlsafe.Parse(body)
		if err != nil {
			return nil, err
		}
		drawings := xmlsafe.FindAll(sheetXML, "drawing")
		if len(drawings) == 0 {
			continue
		}
		drawByID := map[string]string{}
		for _, rel := range ooxml.Rels(zr, ooxml.RelsPath(part), "drawing") {
			drawByID[rel.ID] = rel.Target
		}
		var images []ooxml.Image
		for _, drawing := range drawings {
			drawPart := drawByID[drawing.Attr["id"]]
			if drawPart == "" {
				return nil, corruptf("missing drawing relationship")
			}
			drawBody, err := zr.Read(drawPart)
			if err != nil {
				return nil, err
			}
			drawXML, err := xmlsafe.Parse(drawBody)
			if err != nil {
				return nil, err
			}
			imageByID := map[string]string{}
			for _, rel := range ooxml.Rels(zr, ooxml.RelsPath(drawPart), "image") {
				imageByID[rel.ID] = rel.Target
			}
			for _, kind := range []string{"absoluteAnchor", "oneCellAnchor", "twoCellAnchor"} {
				for _, anchor := range xmlsafe.Direct(drawXML, kind) {
					for _, blip := range xmlsafe.FindAll(anchor, "blip") {
						rid := blip.Attr["embed"]
						if rid == "" {
							continue
						}
						imagePart, ok := imageByID[rid]
						if !ok {
							return nil, corruptf("missing image relationship %s", rid)
						}
						blob, err := zr.Read(imagePart)
						if err != nil {
							return nil, err
						}
						images = append(images, ooxml.ImageInfo(imagePart, blob, defaults, overrides))
					}
				}
			}
		}
		if len(images) > 0 {
			out[sheet.Attr["name"]] = images
		}
	}
	return out, nil
}
