package compiler

import (
	"fmt"
	"strconv"

	"docxcraft/internal/oxml"
	"docxcraft/internal/profile"

	"github.com/beevik/etree"
)

func wEl(tag string) *etree.Element {
	el := etree.NewElement("w:" + tag)
	return el
}

func setWVal(el *etree.Element, val string) {
	el.CreateAttr("w:val", val)
}

func appendRun(p *etree.Element, text string, font string, sizeHalf int, bold, italic bool, color string) {
	r := wEl("r")
	rPr := wEl("rPr")
	if font != "" {
		rf := wEl("rFonts")
		rf.CreateAttr("w:ascii", font)
		rf.CreateAttr("w:hAnsi", font)
		rf.CreateAttr("w:cs", font)
		rPr.AddChild(rf)
	}
	if bold {
		rPr.AddChild(wEl("b"))
	}
	if italic {
		rPr.AddChild(wEl("i"))
	}
	if color != "" {
		c := wEl("color")
		setWVal(c, profile.HexColor(color))
		rPr.AddChild(c)
	}
	if sizeHalf > 0 {
		sz := wEl("sz")
		setWVal(sz, strconv.Itoa(sizeHalf))
		rPr.AddChild(sz)
		szCs := wEl("szCs")
		setWVal(szCs, strconv.Itoa(sizeHalf))
		rPr.AddChild(szCs)
	}
	if len(rPr.Child) > 0 {
		r.AddChild(rPr)
	}
	t := wEl("t")
	oxml.SetRunText(t, text)
	r.AddChild(t)
	p.AddChild(r)
}

func applySectPr(body *etree.Element, res *profile.Resolved) {
	var sect *etree.Element
	for _, c := range body.ChildElements() {
		if c.Tag == "sectPr" {
			sect = c
			break
		}
	}
	if sect == nil {
		sect = wEl("sectPr")
		body.AddChild(sect)
	}
	// rebuild page size / margins
	for i := len(sect.Child) - 1; i >= 0; i-- {
		if el, ok := sect.Child[i].(*etree.Element); ok {
			if el.Tag == "pgSz" || el.Tag == "pgMar" {
				sect.RemoveChildAt(i)
			}
		}
	}
	pgSz := wEl("pgSz")
	pgSz.CreateAttr("w:w", strconv.Itoa(res.PageW))
	pgSz.CreateAttr("w:h", strconv.Itoa(res.PageH))
	sect.InsertChildAt(0, pgSz)

	pgMar := wEl("pgMar")
	pgMar.CreateAttr("w:top", strconv.Itoa(res.MarginTop))
	pgMar.CreateAttr("w:right", strconv.Itoa(res.MarginRight))
	pgMar.CreateAttr("w:bottom", strconv.Itoa(res.MarginBottom))
	pgMar.CreateAttr("w:left", strconv.Itoa(res.MarginLeft))
	pgMar.CreateAttr("w:header", "708")
	pgMar.CreateAttr("w:footer", "708")
	pgMar.CreateAttr("w:gutter", "0")
	sect.InsertChildAt(1, pgMar)
}

func clearBodyKeepSect(body *etree.Element) *etree.Element {
	var sect *etree.Element
	for _, c := range body.ChildElements() {
		if c.Tag == "sectPr" {
			sect = c.Copy()
			break
		}
	}
	for len(body.Child) > 0 {
		body.RemoveChildAt(0)
	}
	if sect == nil {
		sect = wEl("sectPr")
	}
	body.AddChild(sect)
	return sect
}

func insertBeforeSect(body, el *etree.Element) {
	children := body.ChildElements()
	for i, c := range children {
		if c.Tag == "sectPr" {
			// find token index of sectPr
			for ti, tok := range body.Child {
				if tok == c {
					body.InsertChildAt(ti, el)
					_ = i
					return
				}
			}
		}
	}
	body.AddChild(el)
}

func borderEl(tag, val, sz, space, color string) *etree.Element {
	el := wEl(tag)
	el.CreateAttr("w:val", val)
	el.CreateAttr("w:sz", sz)
	el.CreateAttr("w:space", space)
	el.CreateAttr("w:color", color)
	return el
}

func shadeEl(fill string) *etree.Element {
	shd := wEl("shd")
	shd.CreateAttr("w:val", "clear")
	shd.CreateAttr("w:color", "auto")
	shd.CreateAttr("w:fill", profile.HexColor(fill))
	return shd
}

func tcBordersThin(color string) *etree.Element {
	b := wEl("tcBorders")
	c := profile.HexColor(color)
	if c == "" {
		c = "CCCCCC"
	}
	b.AddChild(borderEl("top", "single", "4", "0", c))
	b.AddChild(borderEl("left", "single", "4", "0", c))
	b.AddChild(borderEl("bottom", "single", "4", "0", c))
	b.AddChild(borderEl("right", "single", "4", "0", c))
	return b
}

func styleIDForHeading(level int) string {
	return fmt.Sprintf("Heading%d", level)
}
