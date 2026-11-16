package patcher_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/JuliossJunk/docxcraft/internal/archive"
	"github.com/JuliossJunk/docxcraft/internal/oxml"
	"github.com/JuliossJunk/docxcraft/internal/patcher"
	"github.com/stretchr/testify/require"
)

func TestReviewSpansKeepStyleAndProtectedContent(t *testing.T) {
	pkg, _ := openFixture(t,
		`<w:p w14:paraId="11111111"><w:r><w:rPr><w:b/></w:rPr><w:t>Срок 3</w:t></w:r>`+
			`<w:r><w:rPr><w:i/></w:rPr><w:t>0 дней</w:t></w:r>`+
			`<w:hyperlink r:id="rId1"><w:r><w:t>ссылка</w:t></w:r></w:hyperlink></w:p>`+
			`<w:p w14:paraId="22222222"><w:pPr><w:jc w:val="center"/><w:pStyle w:val="Normal"/></w:pPr><w:r><w:t>Keep</w:t></w:r></w:p>`+
			`<w:tbl><w:tr w14:paraId="33333333"><w:tc><w:p><w:r><w:t>100 ₽</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`)
	s := patcher.NewSession(pkg)
	paragraphs, err := s.ReviewParagraphs()
	require.NoError(t, err)
	require.Equal(t, "tbl_33333333", paragraphs[2].Table)
	require.Equal(t, 1, paragraphs[2].Row)
	require.Equal(t, 1, paragraphs[2].Cell)
	stamp := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	result, err := s.ApplyReview([]patcher.ReviewEdit{
		{AnchorID: "11111111", Quote: "30", Text: "15"},
		{AnchorID: paragraphs[2].AnchorID, Quote: "100", Text: "200"},
	}, "Reviewer", stamp)
	require.NoError(t, err)
	raw, err := pkg.SaveToBytes()
	require.NoError(t, err)
	reopened, err := archive.Open(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	require.NoError(t, pkg.VerifyPreservedParts(reopened))
	require.NoError(t, result.Verify(reopened))
	doc, err := reopened.Document()
	require.NoError(t, err)
	p := doc.FindElement("//w:p")
	assertGoldenParagraph(t, "review_inline.xml", p)
	require.Equal(t, "jc", doc.FindElements("//w:p")[1].FindElement("w:pPr").ChildElements()[0].Tag)
	visible, err := patcher.NewSession(reopened).ReviewParagraphs()
	require.NoError(t, err)
	require.Equal(t, "Срок 15 днейссылка", visible[0].Text)
	require.Equal(t, "200 ₽", visible[2].Text)
}

func TestReviewOriginalRangesAndBatchFailure(t *testing.T) {
	pkg, _ := openFixture(t, `<w:p w14:paraId="11111111"><w:r><w:t>A 30 B 30</w:t></w:r>`+
		`<w:r><w:fldChar w:fldCharType="begin"/><w:instrText>LINK</w:instrText></w:r>`+
		`<w:r><w:fldChar w:fldCharType="separate"/><w:t>Field</w:t><w:fldChar w:fldCharType="end"/><w:t>Tail</w:t></w:r>`+
		`<w:ins w:id="004"><w:r><w:t>Prior</w:t></w:r></w:ins>`+
		`<w:sdt><w:sdtContent><w:r><w:t>Control</w:t></w:r></w:sdtContent></w:sdt></w:p>`)
	s := patcher.NewSession(pkg)
	doc, err := pkg.Document()
	require.NoError(t, err)
	original, err := oxml.SerializeXML(doc)
	require.NoError(t, err)
	valid := patcher.ReviewEdit{Quote: "30", Prefix: "B ", Text: "15"}
	for _, tc := range []struct {
		edit patcher.ReviewEdit
		err  string
	}{
		{patcher.ReviewEdit{Quote: "30", Text: "60"}, "ambiguous"},
		{patcher.ReviewEdit{Quote: "B 3", Text: "X"}, "overlap"},
		{patcher.ReviewEdit{Quote: "Field", Text: "X"}, "complex field"},
		{patcher.ReviewEdit{Quote: "Prior", Text: "X"}, "w:ins"},
		{patcher.ReviewEdit{Quote: "Control", Text: "X"}, "w:sdt"},
	} {
		_, err := s.ApplyReview([]patcher.ReviewEdit{valid, tc.edit}, "", time.Time{})
		require.ErrorContains(t, err, tc.err)
		unchanged, err := oxml.SerializeXML(doc)
		require.NoError(t, err)
		require.Equal(t, original, unchanged)
	}
	_, err = s.ApplyReview([]patcher.ReviewEdit{
		valid,
		{Quote: "Tail", Text: "New ", Position: "before"},
		{Quote: "A ", Text: ""},
	}, "", time.Time{})
	require.NoError(t, err)
	visible, err := s.ReviewParagraphs()
	require.NoError(t, err)
	require.Equal(t, "30 B 15FieldNew TailPriorControl", visible[0].Text)
}
