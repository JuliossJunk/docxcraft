package oxml

import (
	"strings"

	"github.com/beevik/etree"
)

// ParaID returns the w14:paraId of el in uppercase, accepting every common
// attribute encoding etree produces when parsing WordprocessingML.
// Returns "" when the attribute is absent.
func ParaID(el *etree.Element) string {
	return GetW14Attr(el, "paraId")
}

// SetParaID writes w14:paraId on el, replacing any existing encoding of the attribute.
func SetParaID(el *etree.Element, id string) {
	SetW14Attr(el, "paraId", id)
}

// GetW14Attr reads a w14 attribute by local key (e.g. "paraId", "textId").
// Values are returned uppercased. Empty string means missing.
func GetW14Attr(el *etree.Element, key string) string {
	if el == nil || key == "" {
		return ""
	}
	full := "w14:" + key
	for _, attr := range el.Attr {
		switch {
		case attr.Space == "w14" && attr.Key == key:
			return strings.ToUpper(attr.Value)
		case attr.Key == full:
			return strings.ToUpper(attr.Value)
		case attr.Key == key && (attr.Space == "" || attr.Space == "w14"):
			// Bare local key (as emitted by some Word / etree round-trips).
			return strings.ToUpper(attr.Value)
		}
	}
	if v := el.SelectAttrValue(full, ""); v != "" {
		return strings.ToUpper(v)
	}
	if v := el.SelectAttrValue(key, ""); v != "" {
		return strings.ToUpper(v)
	}
	return ""
}

// SetW14Attr writes a w14 attribute as "w14:<key>", replacing any existing form.
func SetW14Attr(el *etree.Element, key, value string) {
	if el == nil || key == "" {
		return
	}
	full := "w14:" + key
	for i := range el.Attr {
		if (el.Attr[i].Space == "w14" && el.Attr[i].Key == key) || el.Attr[i].Key == full || el.Attr[i].Key == key {
			el.Attr[i].Key = full
			el.Attr[i].Space = ""
			el.Attr[i].Value = value
			return
		}
	}
	el.CreateAttr(full, value)
}

// ParagraphPlainText concatenates all w:t and w:delText descendants of p.
func ParagraphPlainText(p *etree.Element) string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	var walk func(*etree.Element)
	walk = func(el *etree.Element) {
		if el.Tag == "t" || el.Tag == "delText" {
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

// ParagraphStyle returns the w:pStyle/@w:val of p, or "" when absent.
func ParagraphStyle(p *etree.Element) string {
	if p == nil {
		return ""
	}
	for _, child := range p.ChildElements() {
		if child.Tag != "pPr" {
			continue
		}
		for _, pp := range child.ChildElements() {
			if pp.Tag != "pStyle" {
				continue
			}
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
	return ""
}

// Body returns the w:body element under the document root, or nil.
func Body(doc *etree.Document) *etree.Element {
	if doc == nil || doc.Root() == nil {
		return nil
	}
	for _, c := range doc.Root().ChildElements() {
		if c.Tag == "body" {
			return c
		}
	}
	return nil
}
