package anchor

import (
	"strings"

	"github.com/beevik/etree"
)

// Block is a compact skeleton entry for LLM inspection.
type Block struct {
	Anchor        string   `json:"anchor"`
	Type          string   `json:"type"`
	Style         string   `json:"style,omitempty"`
	Text          string   `json:"text,omitempty"`
	Rows          int      `json:"rows,omitempty"`
	Cols          int      `json:"cols,omitempty"`
	HeaderPreview []string `json:"header_preview,omitempty"`
}

// InspectSkeleton returns a compact ordered list of paragraph and table anchors.
func InspectSkeleton(doc *etree.Document, idx *Index) []Block {
	if doc == nil || doc.Root() == nil {
		return nil
	}
	var blocks []Block
	var walk func(*etree.Element)
	walk = func(el *etree.Element) {
		switch el.Tag {
		case "p":
			id := getW14Attr(el, "paraId")
			if id == "" && idx != nil {
				// fall back to index reverse lookup by element pointer
				id = findIDByElement(idx, el)
			}
			blocks = append(blocks, Block{
				Anchor: id,
				Type:   "p",
				Style:  paragraphStyle(el),
				Text:   paragraphText(el),
			})
			return // do not descend into nested content of p
		case "tbl":
			id := ""
			if idx != nil {
				id = findIDByElement(idx, el)
			}
			if id == "" {
				if tr := firstChildByTag(el, "tr"); tr != nil {
					rowID := getW14Attr(tr, "paraId")
					if rowID != "" {
						id = tblPrefix + rowID
					}
				}
			}
			rows, cols, headers := tableMeta(el)
			blocks = append(blocks, Block{
				Anchor:        id,
				Type:          "tbl",
				Rows:          rows,
				Cols:          cols,
				HeaderPreview: headers,
			})
			return // skip nested paragraphs inside table for top-level skeleton
		}
		for _, child := range el.ChildElements() {
			walk(child)
		}
	}
	walk(doc.Root())
	return blocks
}

func findIDByElement(idx *Index, el *etree.Element) string {
	for id, t := range idx.byID {
		if t.Element == el {
			return id
		}
	}
	return ""
}

func paragraphStyle(p *etree.Element) string {
	for _, child := range p.ChildElements() {
		if child.Tag != "pPr" {
			continue
		}
		for _, pp := range child.ChildElements() {
			if pp.Tag == "pStyle" {
				for _, attr := range pp.Attr {
					if attr.Key == "val" || attr.Key == "w:val" || (attr.Space == "w" && attr.Key == "val") {
						return attr.Value
					}
				}
				if v := pp.SelectAttrValue("w:val", ""); v != "" {
					return v
				}
				if v := pp.SelectAttrValue("val", ""); v != "" {
					return v
				}
			}
		}
	}
	return "Normal"
}

func paragraphText(p *etree.Element) string {
	var b strings.Builder
	var walk func(*etree.Element)
	walk = func(el *etree.Element) {
		if el.Tag == "t" {
			b.WriteString(el.Text())
			return
		}
		for _, c := range el.ChildElements() {
			walk(c)
		}
	}
	walk(p)
	return b.String()
}

func tableMeta(tbl *etree.Element) (rows, cols int, headers []string) {
	var trs []*etree.Element
	for _, c := range tbl.ChildElements() {
		if c.Tag == "tr" {
			trs = append(trs, c)
		}
	}
	rows = len(trs)
	if rows == 0 {
		return 0, 0, nil
	}
	first := trs[0]
	var tcs []*etree.Element
	for _, c := range first.ChildElements() {
		if c.Tag == "tc" {
			tcs = append(tcs, c)
		}
	}
	cols = len(tcs)
	for _, tc := range tcs {
		headers = append(headers, cellText(tc))
	}
	// Prefer tblGrid col count if larger
	for _, c := range tbl.ChildElements() {
		if c.Tag == "tblGrid" {
			n := 0
			for _, g := range c.ChildElements() {
				if g.Tag == "gridCol" {
					n++
				}
			}
			if n > cols {
				cols = n
			}
		}
	}
	return rows, cols, headers
}

func cellText(tc *etree.Element) string {
	var b strings.Builder
	var walk func(*etree.Element)
	walk = func(el *etree.Element) {
		if el.Tag == "t" {
			b.WriteString(el.Text())
			return
		}
		for _, c := range el.ChildElements() {
			walk(c)
		}
	}
	walk(tc)
	return b.String()
}
