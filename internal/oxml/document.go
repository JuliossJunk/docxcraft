package oxml

import (
	"bytes"
	"fmt"

	"github.com/beevik/etree"
)

// ApplyWriteSettings configures etree serialization for Word-compatible compact XML.
func ApplyWriteSettings(doc *etree.Document) {
	doc.WriteSettings = etree.WriteSettings{
		CanonicalEndTags: false,
		CanonicalText:    true,
		CanonicalAttrVal: true,
	}
}

// NewDocument creates an empty etree document with Word-safe write settings.
func NewDocument() *etree.Document {
	doc := etree.NewDocument()
	ApplyWriteSettings(doc)
	return doc
}

// ParseXML parses XML bytes into an etree document with Word-safe write settings.
// Never call Indent() on WordprocessingML parts.
func ParseXML(data []byte) (*etree.Document, error) {
	doc := NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return nil, fmt.Errorf("oxml parse: %w", err)
	}
	return doc, nil
}

// SerializeXML writes the document without indentation.
func SerializeXML(doc *etree.Document) ([]byte, error) {
	ApplyWriteSettings(doc)
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("oxml serialize: %w", err)
	}
	return buf.Bytes(), nil
}
