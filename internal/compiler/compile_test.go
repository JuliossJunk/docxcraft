package compiler

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompileSimpleAST(t *testing.T) {
	ast := []byte(`{
		"title": "Test",
		"blocks": [
			{"type": "heading", "level": 1, "text": "1. Overview"},
			{"type": "paragraph", "text": "Hello DocxCraft."}
		]
	}`)
	res, err := Compile(ast, "corporate_executive")
	require.NoError(t, err)
	require.NotNil(t, res.Package)

	doc, err := res.Package.Document()
	require.NoError(t, err)
	require.NotNil(t, doc.Root())

	paras := doc.FindElements("//p")
	require.GreaterOrEqual(t, len(paras), 2)

	foundHeading := false
	for _, p := range paras {
		for _, c := range p.ChildElements() {
			if c.Tag != "pPr" {
				continue
			}
			for _, pp := range c.ChildElements() {
				if pp.Tag != "pStyle" {
					continue
				}
				for _, a := range pp.Attr {
					if a.Key == "val" && a.Value == "Heading1" {
						foundHeading = true
					}
				}
			}
		}
	}
	require.True(t, foundHeading)
	require.Greater(t, res.Package.Index().Len(), 0)

	out, err := res.Package.SaveToBytes()
	require.NoError(t, err)
	require.Greater(t, len(out), 100)
}

func TestTableFormattingAndSplitting(t *testing.T) {
	rows := make([][]string, 20)
	for i := range rows {
		rows[i] = []string{"a", "b", "c"}
	}
	doc := &Document{
		Blocks: []Block{{
			Type:        "table",
			ColWidthsCm: []float64{3.0, 9.0, 4.5},
			Headers:     []string{"A", "B", "C"},
			Rows:        rows,
		}},
	}
	raw, err := json.Marshal(doc)
	require.NoError(t, err)

	res, err := Compile(raw, "corporate_executive")
	require.NoError(t, err)

	xmlDoc, err := res.Package.Document()
	require.NoError(t, err)

	cant := xmlDoc.FindElements("//cantSplit")
	require.GreaterOrEqual(t, len(cant), 21)

	headers := xmlDoc.FindElements("//tblHeader")
	require.GreaterOrEqual(t, len(headers), 1)

	gridCols := xmlDoc.FindElements("//gridCol")
	require.Len(t, gridCols, 3)
}

func TestCompileUnknownProfile(t *testing.T) {
	_, err := Compile([]byte(`{"blocks":[{"type":"paragraph","text":"x"}]}`), "nope")
	require.Error(t, err)
}

func TestParseAST_stripsUTF8BOM(t *testing.T) {
	payload := []byte(`{"blocks":[{"type":"paragraph","text":"ok"}]}`)
	withBOM := append([]byte{0xEF, 0xBB, 0xBF}, payload...)
	doc, err := ParseAST(withBOM)
	require.NoError(t, err)
	require.Len(t, doc.Blocks, 1)
	require.Equal(t, "ok", doc.Blocks[0].Text)
}
