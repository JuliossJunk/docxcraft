package oxml_test

import (
	"strings"
	"testing"

	"github.com/JuliossJunk/docxcraft/internal/anchor"
	"github.com/JuliossJunk/docxcraft/internal/oxml"

	"github.com/beevik/etree"
	"github.com/stretchr/testify/require"
)

func TestParaID_readsPrefixedAttributeForms(t *testing.T) {
	cases := []struct {
		name string
		set  func(*etree.Element)
	}{
		{
			name: "w14 space and local key",
			set: func(p *etree.Element) {
				p.CreateAttr("w14:paraId", "AABBCCDD")
			},
		},
		{
			name: "bare paraId key",
			set: func(p *etree.Element) {
				p.CreateAttr("paraId", "aabbccdd")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := etree.NewElement("w:p")
			tc.set(p)
			require.Equal(t, "AABBCCDD", oxml.ParaID(p))
		})
	}
}

func TestParaID_emptyWhenMissing(t *testing.T) {
	p := etree.NewElement("w:p")
	require.Empty(t, oxml.ParaID(p))
}

func TestSetParaID_roundTrip(t *testing.T) {
	p := etree.NewElement("w:p")
	oxml.SetParaID(p, "11223344")
	require.Equal(t, "11223344", oxml.ParaID(p))

	oxml.SetParaID(p, "55667788")
	require.Equal(t, "55667788", oxml.ParaID(p))
	// still a single paraId attribute after overwrite
	count := 0
	for _, a := range p.Attr {
		if a.Key == "paraId" || a.Key == "w14:paraId" || (a.Space == "w14" && a.Key == "paraId") {
			count++
		}
	}
	require.Equal(t, 1, count)
}

func TestParagraphPlainText_includesTAndDelText(t *testing.T) {
	p := etree.NewElement("w:p")
	r := etree.NewElement("w:r")
	tt := etree.NewElement("w:t")
	tt.SetText("Hello ")
	r.AddChild(tt)
	p.AddChild(r)

	del := etree.NewElement("w:del")
	delR := etree.NewElement("w:r")
	delT := etree.NewElement("w:delText")
	delT.SetText("gone")
	delR.AddChild(delT)
	del.AddChild(delR)
	p.AddChild(del)

	require.Equal(t, "Hello gone", oxml.ParagraphPlainText(p))
}

func TestParagraphStyle_readsPStyleVal(t *testing.T) {
	p := etree.NewElement("w:p")
	pPr := etree.NewElement("w:pPr")
	pStyle := etree.NewElement("w:pStyle")
	pStyle.CreateAttr("w:val", "Heading1")
	pPr.AddChild(pStyle)
	p.AddChild(pPr)

	require.Equal(t, "Heading1", oxml.ParagraphStyle(p))
	require.Empty(t, oxml.ParagraphStyle(etree.NewElement("w:p")))
}

func TestBody_findsWordBody(t *testing.T) {
	doc := etree.NewDocument()
	root := etree.NewElement("w:document")
	body := etree.NewElement("w:body")
	root.AddChild(body)
	doc.SetRoot(root)

	require.Equal(t, body, oxml.Body(doc))
	require.Nil(t, oxml.Body(nil))
	require.Nil(t, oxml.Body(etree.NewDocument()))
}

func TestParaID_agreesWithEnsureAnchorsAcrossAttributeForms(t *testing.T) {
	// Document with three paragraphs using different paraId attribute shapes.
	// After EnsureAnchors, oxml.ParaID must report the same IDs the index holds.
	xml := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
 xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml"
 xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"
 mc:Ignorable="w14">
  <w:body>
    <w:p w14:paraId="1AAA1111"><w:r><w:t>one</w:t></w:r></w:p>
    <w:p paraId="2BBB2222"><w:r><w:t>two</w:t></w:r></w:p>
    <w:p><w:r><w:t>three needs id</w:t></w:r></w:p>
  </w:body>
</w:document>`

	doc, err := oxml.ParseXML([]byte(xml))
	require.NoError(t, err)

	idx := anchor.NewIndex()
	_, err = anchor.EnsureAnchors(doc, "word/document.xml", idx)
	require.NoError(t, err)

	body := oxml.Body(doc)
	require.NotNil(t, body)

	var paras []*etree.Element
	for _, c := range body.ChildElements() {
		if c.Tag == "p" {
			paras = append(paras, c)
		}
	}
	require.Len(t, paras, 3)

	for _, p := range paras {
		id := oxml.ParaID(p)
		require.NotEmpty(t, id, "every paragraph must have a paraId after EnsureAnchors")
		require.Equal(t, strings.ToUpper(id), id)

		target, ok := idx.Get(id)
		require.True(t, ok, "index must contain paraId %s", id)
		require.Same(t, p, target.Element)
	}

	// Prefixed and bare forms must survive indexing without reassignment when already valid
	// (ids must be < 0x80000000 per Word w14:paraId rules).
	require.Equal(t, "1AAA1111", oxml.ParaID(paras[0]))
	require.Equal(t, "2BBB2222", oxml.ParaID(paras[1]))
}
