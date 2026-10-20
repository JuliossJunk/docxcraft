package archive

import (
	"archive/zip"
	"bytes"
	"fmt"
)

const minimalDocumentXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"` +
	` xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml"` +
	` xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"` +
	` mc:Ignorable="w14">` +
	`<w:body>` +
	`<w:p w14:paraId="1A4F09C1" w14:textId="1A4F09C1">` +
	`<w:pPr><w:pStyle w:val="Heading1"/></w:pPr>` +
	`<w:r><w:t>Title</w:t></w:r>` +
	`</w:p>` +
	`<w:p w14:paraId="3B8E12A4" w14:textId="3B8E12A4">` +
	`<w:r><w:t>Hello world</w:t></w:r>` +
	`</w:p>` +
	`<w:tbl>` +
	`<w:tblGrid><w:gridCol w:w="2000"/><w:gridCol w:w="2000"/></w:tblGrid>` +
	`<w:tr w14:paraId="7C01D2E9">` +
	`<w:tc><w:p w14:paraId="11111111" w14:textId="11111111"><w:r><w:t>A</w:t></w:r></w:p></w:tc>` +
	`<w:tc><w:p w14:paraId="22222222" w14:textId="22222222"><w:r><w:t>B</w:t></w:r></w:p></w:tc>` +
	`</w:tr>` +
	`</w:tbl>` +
	`<w:sectPr><w:pgSz w:w="11906" w:h="16838"/></w:sectPr>` +
	`</w:body></w:document>`

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>` +
	`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
	`</Types>`

const relsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
	`</Relationships>`

const docRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`</Relationships>`

// buildMinimalDocx creates a synthetic valid .docx ZIP in memory.
func buildMinimalDocx() ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	parts := []struct {
		name string
		data string
	}{
		{"[Content_Types].xml", contentTypesXML},
		{"_rels/.rels", relsXML},
		{"word/document.xml", minimalDocumentXML},
		{"word/_rels/document.xml.rels", docRelsXML},
	}
	for _, p := range parts {
		w, err := zw.Create(p.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(p.data)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildDocxWithDocument(documentXML string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	parts := map[string]string{
		"[Content_Types].xml":          contentTypesXML,
		"_rels/.rels":                  relsXML,
		"word/document.xml":            documentXML,
		"word/_rels/document.xml.rels": docRelsXML,
	}
	order := []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml", "word/_rels/document.xml.rels"}
	for _, name := range order {
		w, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(parts[name])); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func mustMinimalDocx() []byte {
	b, err := buildMinimalDocx()
	if err != nil {
		panic(fmt.Sprintf("build minimal docx: %v", err))
	}
	return b
}
