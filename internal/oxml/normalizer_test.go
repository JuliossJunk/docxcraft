package oxml

import (
	"testing"

	"github.com/beevik/etree"
	"github.com/stretchr/testify/require"
)

func TestSchemaNormalization(t *testing.T) {
	tcPr := etree.NewElement("tcPr")
	tcPr.Space = "w"

	shd := etree.NewElement("shd")
	shd.Space = "w"
	tcW := etree.NewElement("tcW")
	tcW.Space = "w"
	vAlign := etree.NewElement("vAlign")
	vAlign.Space = "w"
	tcBorders := etree.NewElement("tcBorders")
	tcBorders.Space = "w"

	// Intentionally wrong order: shd before tcW, vAlign before tcBorders.
	tcPr.AddChild(shd)
	tcPr.AddChild(vAlign)
	tcPr.AddChild(tcW)
	tcPr.AddChild(tcBorders)

	NormalizeContainer(tcPr, tcPrOrder)

	children := tcPr.ChildElements()
	require.Len(t, children, 4)
	require.Equal(t, "tcW", children[0].Tag)
	require.Equal(t, "tcBorders", children[1].Tag)
	require.Equal(t, "shd", children[2].Tag)
	require.Equal(t, "vAlign", children[3].Tag)
}

func TestNormalizeElementOrderDocument(t *testing.T) {
	xml := `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:tcPr><w:shd w:val="clear"/><w:tcW w:w="100" w:type="dxa"/></w:tcPr>` +
		`</w:document>`
	doc, err := ParseXML([]byte(xml))
	require.NoError(t, err)

	NormalizeElementOrder(doc)

	tcPr := doc.FindElement("//tcPr")
	require.NotNil(t, tcPr)
	children := tcPr.ChildElements()
	require.GreaterOrEqual(t, len(children), 2)
	require.Equal(t, "tcW", children[0].Tag)
	require.Equal(t, "shd", children[1].Tag)
}

func TestSetRunTextPreserve(t *testing.T) {
	tElem := etree.NewElement("t")
	tElem.Space = "w"
	SetRunText(tElem, " hello ")
	require.Equal(t, " hello ", tElem.Text())
	found := false
	for _, a := range tElem.Attr {
		if a.Key == "space" || a.Key == "xml:space" || (a.Space == "xml" && a.Key == "space") {
			require.Equal(t, "preserve", a.Value)
			found = true
		}
	}
	require.True(t, found)

	SetRunText(tElem, "hello")
	for _, a := range tElem.Attr {
		require.NotEqual(t, "preserve", a.Value)
	}
}
