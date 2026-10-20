package oxml

import (
	"sort"

	"github.com/beevik/etree"
)

var pPrOrder = map[string]int{
	"pStyle":          1,
	"keepNext":        2,
	"keepLines":       3,
	"pageBreakBefore": 4,
	"widowControl":    5,
	"numPr":           6,
	"pBdr":            7,
	"shd":             8,
	"spacing":         9,
	"ind":             10,
	"jc":              11,
	"rPr":             12,
	"sectPr":          13,
}

var rPrOrder = map[string]int{
	"rStyle":  1,
	"rFonts":  2,
	"b":       3,
	"bCs":     3,
	"i":       4,
	"iCs":     4,
	"strike":  5,
	"dstrike": 5,
	"u":       6,
	"shd":     7,
	"color":   8,
	"sz":      9,
	"szCs":    9,
	"spacing": 10,
	"lang":    11,
}

var tcPrOrder = map[string]int{
	"cnfStyle":  1,
	"tcW":       2,
	"gridSpan":  3,
	"vMerge":    4,
	"tcBorders": 5,
	"shd":       6,
	"tcMar":     7,
	"vAlign":    8,
}

var tblPrOrder = map[string]int{
	"tblStyle":       1,
	"tblpPr":         2,
	"tblOverlap":     3,
	"bidiVisual":     4,
	"tblStyleRowBandSize": 5,
	"tblStyleColBandSize": 6,
	"tblW":           7,
	"jc":             8,
	"tblCellSpacing": 9,
	"tblInd":         10,
	"tblBorders":     11,
	"shd":            12,
	"tblLayout":      13,
	"tblCellMar":     14,
	"tblLook":        15,
}

var trPrOrder = map[string]int{
	"cnfStyle":   1,
	"divId":      2,
	"gridBefore": 3,
	"gridAfter":  4,
	"wBefore":    5,
	"wAfter":     6,
	"cantSplit":  7,
	"trHeight":   8,
	"tblHeader":  9,
	"tblCellSpacing": 10,
	"jc":         11,
	"hidden":     12,
}

// NormalizeContainer sorts children of a property container per ECMA-376 rank map.
// Unknown tags keep relative order and are placed after known tags.
func NormalizeContainer(parent *etree.Element, orderMap map[string]int) {
	if parent == nil {
		return
	}
	children := parent.ChildElements()
	if len(children) <= 1 {
		return
	}

	sorted := make([]*etree.Element, len(children))
	copy(sorted, children)
	sort.SliceStable(sorted, func(i, j int) bool {
		tagI := sorted[i].Tag
		tagJ := sorted[j].Tag
		rankI, existsI := orderMap[tagI]
		rankJ, existsJ := orderMap[tagJ]
		if existsI && existsJ {
			return rankI < rankJ
		}
		if existsI && !existsJ {
			return true
		}
		if !existsI && existsJ {
			return false
		}
		return false
	})

	var nonElements []etree.Token
	for _, tok := range parent.Child {
		if _, ok := tok.(*etree.Element); !ok {
			nonElements = append(nonElements, tok)
		}
	}
	for len(parent.Child) > 0 {
		parent.RemoveChildAt(0)
	}
	for _, child := range sorted {
		parent.AddChild(child)
	}
	for _, tok := range nonElements {
		parent.AddChild(tok)
	}
}

// NormalizeElementOrder walks the document and normalizes w:pPr, w:rPr, w:tcPr, w:tblPr, w:trPr.
func NormalizeElementOrder(doc *etree.Document) {
	if doc == nil || doc.Root() == nil {
		return
	}
	normalizeMatching(doc.Root(), "pPr", pPrOrder)
	normalizeMatching(doc.Root(), "rPr", rPrOrder)
	normalizeMatching(doc.Root(), "tcPr", tcPrOrder)
	normalizeMatching(doc.Root(), "tblPr", tblPrOrder)
	normalizeMatching(doc.Root(), "trPr", trPrOrder)
}

func normalizeMatching(root *etree.Element, tag string, orderMap map[string]int) {
	var walk func(*etree.Element)
	walk = func(el *etree.Element) {
		if el.Tag == tag {
			NormalizeContainer(el, orderMap)
		}
		for _, child := range el.ChildElements() {
			walk(child)
		}
	}
	walk(root)
}
