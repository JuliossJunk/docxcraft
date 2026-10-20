package oxml

import (
	"strings"

	"github.com/beevik/etree"
)

// SetRunText sets <w:t> text and manages xml:space="preserve" for leading/trailing spaces.
func SetRunText(tElem *etree.Element, text string) {
	if tElem == nil {
		return
	}
	tElem.SetText(text)
	if strings.HasPrefix(text, " ") || strings.HasSuffix(text, " ") {
		setXMLSpacePreserve(tElem)
		return
	}
	removeXMLSpace(tElem)
}

func setXMLSpacePreserve(el *etree.Element) {
	for i := range el.Attr {
		if (el.Attr[i].Space == "xml" && el.Attr[i].Key == "space") ||
			el.Attr[i].Key == "xml:space" {
			el.Attr[i].Value = "preserve"
			return
		}
	}
	el.CreateAttr("xml:space", "preserve")
}

func removeXMLSpace(el *etree.Element) {
	el.RemoveAttr("xml:space")
	el.RemoveAttr("space")
}
