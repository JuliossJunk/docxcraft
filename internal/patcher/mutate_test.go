package patcher

import (
	"testing"

	"github.com/JuliossJunk/docxcraft/internal/compiler"
	"github.com/JuliossJunk/docxcraft/internal/oxml"

	"github.com/stretchr/testify/require"
)

func TestReplaceAndInsertStableAnchors(t *testing.T) {
	ast := []byte(`{
		"blocks": [
			{"type": "paragraph", "text": "First"},
			{"type": "paragraph", "text": "Second"},
			{"type": "paragraph", "text": "Third"}
		]
	}`)
	res, err := compiler.Compile(ast, "corporate_executive")
	require.NoError(t, err)

	idx := res.Package.Index()
	require.GreaterOrEqual(t, idx.Len(), 3)

	// collect paragraph anchors in order
	doc, err := res.Package.Document()
	require.NoError(t, err)
	var anchors []string
	for _, p := range doc.FindElements("//p") {
		id := oxml.ParaID(p)
		if id != "" {
			anchors = append(anchors, id)
		}
	}
	require.GreaterOrEqual(t, len(anchors), 3)

	first, third := anchors[0], anchors[2]
	sess := NewSession(res.Package)

	require.NoError(t, sess.ReplaceParagraphText(first, "Updated first", true))

	newIDs, err := sess.InsertAfterAnchor(first, "Inserted A", "Inserted B")
	require.NoError(t, err)
	require.Len(t, newIDs, 2)

	// old third anchor still resolvable
	_, ok := res.Package.Index().Get(third)
	require.True(t, ok, "third anchor must remain stable")

	got, ok := res.Package.Index().Get(first)
	require.True(t, ok)
	require.Contains(t, oxml.ParagraphPlainText(got.Element), "Updated first")
}

func TestDeleteByAnchor(t *testing.T) {
	ast := []byte(`{
		"blocks": [
			{"type": "paragraph", "text": "Keep"},
			{"type": "paragraph", "text": "RemoveMe"},
			{"type": "paragraph", "text": "AlsoKeep"}
		]
	}`)
	res, err := compiler.Compile(ast, "corporate_executive")
	require.NoError(t, err)

	doc, err := res.Package.Document()
	require.NoError(t, err)
	var anchors []string
	for _, p := range doc.FindElements("//p") {
		if id := oxml.ParaID(p); id != "" {
			anchors = append(anchors, id)
		}
	}
	require.GreaterOrEqual(t, len(anchors), 3)
	removeID, keepID := anchors[1], anchors[2]

	sess := NewSession(res.Package)
	require.NoError(t, sess.DeleteByAnchor(removeID))

	_, ok := res.Package.Index().Get(removeID)
	require.False(t, ok)
	_, ok = res.Package.Index().Get(keepID)
	require.True(t, ok)

	doc2, err := res.Package.Document()
	require.NoError(t, err)
	text := ""
	for _, tEl := range doc2.FindElements("//t") {
		text += tEl.Text()
	}
	require.NotContains(t, text, "RemoveMe")
	require.Contains(t, text, "Keep")
	require.Contains(t, text, "AlsoKeep")
}
