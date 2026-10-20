package oxml

import (
	"strings"

	"github.com/beevik/etree"
)

// WordprocessingML / Markup Compatibility namespace URIs.
const (
	NSW   = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	NSW14 = "http://schemas.microsoft.com/office/word/2010/wordml"
	NSR   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	NSWP  = "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
	NSMC  = "http://schemas.openxmlformats.org/markup-compatibility/2006"
)

// EnsureW14Namespace adds xmlns:w14 and ensures w14 is listed in mc:Ignorable on root.
func EnsureW14Namespace(root *etree.Element) {
	if root == nil {
		return
	}

	if root.SelectAttr("xmlns:w14") == nil && root.SelectAttrValue("xmlns:w14", "") == "" {
		// etree stores xmlns as regular attributes with the xmlns prefix in the name.
		hasW14 := false
		for _, attr := range root.Attr {
			if (attr.Space == "xmlns" && attr.Key == "w14") ||
				(attr.Key == "xmlns:w14") ||
				(attr.Space == "" && attr.Key == "xmlns:w14") {
				hasW14 = true
				break
			}
		}
		if !hasW14 {
			root.CreateAttr("xmlns:w14", NSW14)
		}
	}

	ignorable := ""
	for _, attr := range root.Attr {
		if attr.Space == "mc" && attr.Key == "Ignorable" {
			ignorable = attr.Value
			break
		}
		if attr.Key == "mc:Ignorable" {
			ignorable = attr.Value
			break
		}
	}

	parts := strings.Fields(ignorable)
	hasW14Ignorable := false
	for _, p := range parts {
		if p == "w14" {
			hasW14Ignorable = true
			break
		}
	}
	if !hasW14Ignorable {
		if ignorable == "" {
			ignorable = "w14"
		} else {
			ignorable = ignorable + " w14"
		}
		setMCIgnorable(root, ignorable)
	}
}

func setMCIgnorable(root *etree.Element, value string) {
	for i := range root.Attr {
		if (root.Attr[i].Space == "mc" && root.Attr[i].Key == "Ignorable") ||
			root.Attr[i].Key == "mc:Ignorable" {
			root.Attr[i].Value = value
			return
		}
	}
	root.CreateAttr("mc:Ignorable", value)
}

// LocalName returns the local tag name (without namespace prefix).
func LocalName(el *etree.Element) string {
	if el == nil {
		return ""
	}
	return el.Tag
}

// IsW returns true if the element is in the WordprocessingML main namespace
// or uses the conventional "w" prefix / unprefixed local tags used in docx.
func IsW(el *etree.Element) bool {
	if el == nil {
		return false
	}
	if el.Space == "w" || el.Space == "" {
		return true
	}
	return false
}
