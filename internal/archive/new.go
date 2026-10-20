package archive

import (
	"archive/zip"
	"bytes"
	"fmt"

	"docxcraft/internal/anchor"

	"github.com/beevik/etree"
)

const emptyDocumentXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"` +
	` xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"` +
	` xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"` +
	` xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"` +
	` xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml"` +
	` mc:Ignorable="w14">` +
	`<w:body>` +
	`<w:sectPr>` +
	`<w:pgSz w:w="11906" w:h="16838"/>` +
	`<w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440" w:header="708" w:footer="708" w:gutter="0"/>` +
	`</w:sectPr>` +
	`</w:body></w:document>`

const newContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>` +
	`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
	`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
	`</Types>`

const newRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
	`</Relationships>`

const newDocRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
	`</Relationships>`

const newStylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
	`<w:style w:type="paragraph" w:default="1" w:styleId="Normal">` +
	`<w:name w:val="Normal"/><w:qFormat/>` +
	`</w:style>` +
	`<w:style w:type="paragraph" w:styleId="Heading1">` +
	`<w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:uiPriority w:val="9"/><w:qFormat/>` +
	`</w:style>` +
	`<w:style w:type="paragraph" w:styleId="Heading2">` +
	`<w:name w:val="heading 2"/><w:basedOn w:val="Normal"/><w:uiPriority w:val="9"/><w:qFormat/>` +
	`</w:style>` +
	`<w:style w:type="paragraph" w:styleId="Heading3">` +
	`<w:name w:val="heading 3"/><w:basedOn w:val="Normal"/><w:uiPriority w:val="9"/><w:qFormat/>` +
	`</w:style>` +
	`</w:styles>`

// New creates an empty in-memory .docx package ready for compilation.
func New() (*Package, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	parts := []struct{ name, data string }{
		{"[Content_Types].xml", newContentTypes},
		{"_rels/.rels", newRels},
		{"word/document.xml", emptyDocumentXML},
		{"word/_rels/document.xml.rels", newDocRels},
		{"word/styles.xml", newStylesXML},
	}
	for _, p := range parts {
		w, err := zw.Create(p.name)
		if err != nil {
			return nil, fmt.Errorf("archive new: %w", err)
		}
		if _, err := w.Write([]byte(p.data)); err != nil {
			return nil, fmt.Errorf("archive new write: %w", err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("archive new close: %w", err)
	}
	raw := buf.Bytes()
	return OpenWithOptions(bytes.NewReader(raw), int64(len(raw)), OpenOptions{SkipAnchors: true})
}

// SetDocument replaces word/document.xml DOM and marks it dirty.
func (p *Package) SetDocument(doc *etree.Document) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.xmlDocs[documentPart] = doc
	p.dirty[documentPart] = true
}

// RebuildIndex clears and rebuilds the anchor index for word/document.xml.
func (p *Package) RebuildIndex() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	doc, ok := p.xmlDocs[documentPart]
	if !ok {
		parsed, err := p.parsePart(documentPart)
		if err != nil {
			return err
		}
		doc = parsed
		p.xmlDocs[documentPart] = doc
	}
	p.index = anchor.NewIndex()
	mutated, err := anchor.EnsureAnchors(doc, documentPart, p.index)
	if err != nil {
		return err
	}
	if mutated {
		p.dirty[documentPart] = true
	}
	return nil
}
