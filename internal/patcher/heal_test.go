package patcher

import (
	"testing"

	"docxcraft/internal/oxml"

	"github.com/stretchr/testify/require"
)

func TestHealSplitTemplateRuns(t *testing.T) {
	xml := `<?xml version="1.0"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body><w:p>` +
		`<w:r><w:t>{</w:t></w:r>` +
		`<w:proofErr w:type="spellStart"/>` +
		`<w:r><w:t>{ user_name }</w:t></w:r>` +
		`<w:r><w:t>}</w:t></w:r>` +
		`</w:p></w:body></w:document>`
	doc, err := oxml.ParseXML([]byte(xml))
	require.NoError(t, err)

	p := doc.FindElement("//p")
	require.NotNil(t, p)
	HealRuns(p)

	var texts []string
	for _, c := range p.ChildElements() {
		require.NotEqual(t, "proofErr", c.Tag)
		if c.Tag != "r" {
			continue
		}
		for _, rc := range c.ChildElements() {
			if rc.Tag == "t" {
				texts = append(texts, rc.Text())
			}
		}
	}
	require.Len(t, texts, 1)
	require.Equal(t, "{{ user_name }}", texts[0])
}
