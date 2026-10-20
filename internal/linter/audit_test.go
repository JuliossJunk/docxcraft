package linter

import (
	"testing"

	"docxcraft/internal/compiler"

	"github.com/beevik/etree"
	"github.com/stretchr/testify/require"
)

func TestDF004EmptyParagraph(t *testing.T) {
	ast := []byte(`{"blocks":[{"type":"paragraph","text":"A"},{"type":"paragraph","text":"MID"},{"type":"paragraph","text":"B"}]}`)
	res, err := compiler.Compile(ast, "corporate_executive")
	require.NoError(t, err)

	doc, err := res.Package.Document()
	require.NoError(t, err)
	body := doc.FindElement("//body")
	require.NotNil(t, body)
	var bodyParas []*etree.Element
	for _, c := range body.ChildElements() {
		if c.Tag == "p" {
			bodyParas = append(bodyParas, c)
		}
	}
	require.GreaterOrEqual(t, len(bodyParas), 3)
	for _, c := range bodyParas[1].ChildElements() {
		if c.Tag != "r" {
			continue
		}
		for _, rc := range c.ChildElements() {
			if rc.Tag == "t" {
				rc.SetText("")
			}
		}
	}
	res.Package.MarkDirty("word/document.xml")

	rep, err := Audit(res.Package, Options{ProfileName: "corporate_executive"})
	require.NoError(t, err)
	require.Contains(t, issueCodes(rep), "DF-004")
	for _, iss := range rep.Issues {
		if iss.Code == "DF-004" {
			require.NotEmpty(t, iss.AnchorID)
		}
	}
}

func TestDF005HeadingSkip(t *testing.T) {
	ast := []byte(`{
		"blocks": [
			{"type": "heading", "level": 1, "text": "H1"},
			{"type": "heading", "level": 3, "text": "H3"},
			{"type": "paragraph", "text": "body"}
		]
	}`)
	res, err := compiler.Compile(ast, "corporate_executive")
	require.NoError(t, err)
	rep, err := Audit(res.Package, Options{ProfileName: "corporate_executive"})
	require.NoError(t, err)
	require.Contains(t, issueCodes(rep), "DF-005")
}

func TestDF009TblHeader(t *testing.T) {
	ast := []byte(`{
		"blocks": [{
			"type": "table",
			"headers": ["A","B"],
			"rows": [["1","2"]],
			"col_widths_cm": [5,5]
		}]
	}`)
	res, err := compiler.Compile(ast, "corporate_executive")
	require.NoError(t, err)
	doc, err := res.Package.Document()
	require.NoError(t, err)
	removeTblHeaders(doc)
	res.Package.MarkDirty("word/document.xml")

	rep, err := Audit(res.Package, Options{ProfileName: "corporate_executive"})
	require.NoError(t, err)
	require.Contains(t, issueCodes(rep), "DF-009")
}

func TestReportIsValidAggregation(t *testing.T) {
	r := &Report{Issues: []Issue{
		{Severity: "warning"},
		{Severity: "error"},
	}}
	r.finalize(false)
	require.False(t, r.IsValid)

	r2 := &Report{Issues: []Issue{{Severity: "warning"}}}
	r2.finalize(false)
	require.True(t, r2.IsValid)
	r2.finalize(true)
	require.False(t, r2.IsValid)
}

func TestVisualGateSkippedWithoutLO(t *testing.T) {
	ast := []byte(`{"blocks":[{"type":"paragraph","text":"x"}]}`)
	res, err := compiler.Compile(ast, "corporate_executive")
	require.NoError(t, err)
	rep, err := Audit(res.Package, Options{EnableVisual: true})
	require.NoError(t, err)
	require.Equal(t, "skipped", rep.VisualGate.Status)
}

func removeTblHeaders(doc *etree.Document) {
	for _, trPr := range doc.FindElements("//trPr") {
		for i := len(trPr.Child) - 1; i >= 0; i-- {
			el, ok := trPr.Child[i].(*etree.Element)
			if ok && el.Tag == "tblHeader" {
				trPr.RemoveChildAt(i)
			}
		}
	}
}

func issueCodes(r *Report) []string {
	out := make([]string, 0, len(r.Issues))
	for _, i := range r.Issues {
		out = append(out, i.Code)
	}
	return out
}
