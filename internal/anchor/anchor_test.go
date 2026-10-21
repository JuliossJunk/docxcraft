package anchor

import (
	"testing"

	"github.com/JuliossJunk/docxcraft/internal/oxml"

	"github.com/stretchr/testify/require"
)

func TestEnsureAnchorsAssignsIDs(t *testing.T) {
	xml := `<?xml version="1.0"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body>` +
		`<w:p><w:r><w:t>No id</w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Heading</w:t></w:r></w:p>` +
		`</w:body></w:document>`
	doc, err := oxml.ParseXML([]byte(xml))
	require.NoError(t, err)

	idx := NewIndex()
	mutated, err := EnsureAnchors(doc, "word/document.xml", idx)
	require.NoError(t, err)
	require.True(t, mutated)
	require.GreaterOrEqual(t, idx.Len(), 2)

	blocks := InspectSkeleton(doc, idx)
	require.Len(t, blocks, 2)
	require.Equal(t, "p", blocks[0].Type)
	require.Equal(t, "No id", blocks[0].Text)
	require.Equal(t, "Heading1", blocks[1].Style)
	require.True(t, isValidParaID(blocks[0].Anchor))
	require.True(t, isValidParaID(blocks[1].Anchor))

	got, ok := idx.Get(blocks[0].Anchor)
	require.True(t, ok)
	require.Equal(t, "p", got.Kind)
}

func TestEnsureAnchorsDeduplicates(t *testing.T) {
	xml := `<?xml version="1.0"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"` +
		` xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml" mc:Ignorable="w14"` +
		` xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006">` +
		`<w:body>` +
		`<w:p w14:paraId="AAAAAAAA" w14:textId="AAAAAAAA"><w:r><w:t>One</w:t></w:r></w:p>` +
		`<w:p w14:paraId="AAAAAAAA" w14:textId="AAAAAAAA"><w:r><w:t>Two</w:t></w:r></w:p>` +
		`</w:body></w:document>`
	doc, err := oxml.ParseXML([]byte(xml))
	require.NoError(t, err)

	idx := NewIndex()
	mutated, err := EnsureAnchors(doc, "word/document.xml", idx)
	require.NoError(t, err)
	require.True(t, mutated)

	blocks := InspectSkeleton(doc, idx)
	require.Len(t, blocks, 2)
	require.NotEqual(t, blocks[0].Anchor, blocks[1].Anchor)
	require.True(t, isValidParaID(blocks[0].Anchor))
	require.True(t, isValidParaID(blocks[1].Anchor))
}

func TestTableAnchorPrefix(t *testing.T) {
	xml := `<?xml version="1.0"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"` +
		` xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml"` +
		` xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006" mc:Ignorable="w14">` +
		`<w:body><w:tbl>` +
		`<w:tr w14:paraId="7C01D2E9"><w:tc><w:p w14:paraId="11111111"><w:r><w:t>H1</w:t></w:r></w:p></w:tc></w:tr>` +
		`</w:tbl></w:body></w:document>`
	doc, err := oxml.ParseXML([]byte(xml))
	require.NoError(t, err)

	idx := NewIndex()
	_, err = EnsureAnchors(doc, "word/document.xml", idx)
	require.NoError(t, err)

	tbl, ok := idx.Get("tbl_7C01D2E9")
	require.True(t, ok)
	require.Equal(t, "tbl", tbl.Kind)

	blocks := InspectSkeleton(doc, idx)
	require.Len(t, blocks, 1)
	require.Equal(t, "tbl", blocks[0].Type)
	require.Equal(t, "tbl_7C01D2E9", blocks[0].Anchor)
	require.Equal(t, []string{"H1"}, blocks[0].HeaderPreview)
}

func TestIsValidParaID(t *testing.T) {
	require.True(t, isValidParaID("1A4F09C1"))
	require.False(t, isValidParaID("00000000"))
	require.False(t, isValidParaID("80000000"))
	require.False(t, isValidParaID("abc"))
	require.False(t, isValidParaID("1a4f09c1")) // must be uppercase after normalize; raw lowercase fails len/hex check of A-F only — lowercase letters fail
}
