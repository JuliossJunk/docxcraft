package patcher_test

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JuliossJunk/docxcraft/internal/archive"
	"github.com/JuliossJunk/docxcraft/internal/oxml"
	"github.com/JuliossJunk/docxcraft/internal/patcher"
	"github.com/beevik/etree"
	"github.com/stretchr/testify/require"
)

// openFixture exercises the same ZIP → package → patch → ZIP path as callers.
func openFixture(t *testing.T, paragraphs string) (*archive.Package, map[string][]byte) {
	t.Helper()
	doc := `<w:document xmlns:w="` + oxml.NSW + `" xmlns:w14="` + oxml.NSW14 +
		`" xmlns:mc="` + oxml.NSMC + `" xmlns:r="` + oxml.NSR + `" mc:Ignorable="w14"><w:body>` +
		paragraphs + `<w:sectPr/></w:body></w:document>`
	parts := map[string][]byte{
		"word/document.xml":            []byte(doc),
		"word/comments.xml":            []byte(`<w:comments xmlns:w="` + oxml.NSW + `"><w:comment w:id="4" w:author="Reviewer"><w:p><w:r><w:t>Keep comment</w:t></w:r></w:p></w:comment></w:comments>`),
		"word/_rels/document.xml.rels": []byte(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://example.com" TargetMode="External"/></Relationships>`),
		"word/media/opaque.bin":        {0, 255, 1, 42},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range parts {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	pkg, err := archive.Open(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	return pkg, parts
}

func TestTrackedEditsAllocateIDsAcrossSessionsAndExistingRevisions(t *testing.T) {
	pkg, _ := openFixture(t, `<w:p w14:paraId="11111111"><w:ins w:id="0"><w:r><w:t>Existing</w:t></w:r></w:ins><w:del w:id="0001"><w:r><w:delText>Old</w:delText></w:r></w:del></w:p>`+
		`<w:p w14:paraId="22222222"><w:r><w:t>Second</w:t></w:r></w:p>`+
		`<w:p w14:paraId="33333333"><w:r><w:t>Third</w:t></w:r></w:p>`+
		`<w:p w14:paraId="44444444"><w:r><w:t>Fourth</w:t></w:r></w:p>`)
	s := patcher.NewSession(pkg)
	ts := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	require.NoError(t, s.ApplyTrackedEdit("22222222", "New second", "Reviewer", ts))
	require.NoError(t, s.ApplyTrackedEdit("33333333", "New third", "Reviewer", ts))
	raw, err := pkg.SaveToBytes()
	require.NoError(t, err)
	reopened, err := archive.Open(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	require.NoError(t, patcher.NewSession(reopened).ApplyTrackedEdit("44444444", "New fourth", "Reviewer", ts))
	_, doc := saveParts(t, reopened)
	seen := map[string]bool{"0": true, "0001": true}
	var newIDs []string
	for _, p := range doc.FindElements("//p")[1:] {
		for _, child := range p.ChildElements() {
			if child.Tag != "ins" && child.Tag != "del" {
				continue
			}
			id := child.SelectAttrValue("w:id", "")
			require.NotEmpty(t, id)
			require.NotEqual(t, "1", id, "must not collide numerically with existing 0001")
			require.False(t, seen[id], "duplicate revision ID %s", id)
			seen[id] = true
			newIDs = append(newIDs, id)
		}
	}
	require.Len(t, newIDs, 6)
}

func saveParts(t *testing.T, pkg *archive.Package) (map[string][]byte, *etree.Document) {
	t.Helper()
	raw, err := pkg.SaveToBytes()
	require.NoError(t, err)
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	parts := make(map[string][]byte)
	for _, f := range zr.File {
		r, err := f.Open()
		require.NoError(t, err)
		parts[f.Name], err = io.ReadAll(r)
		require.NoError(t, err)
		require.NoError(t, r.Close())
	}
	doc, err := oxml.ParseXML(parts["word/document.xml"])
	require.NoError(t, err)
	return parts, doc
}

func TestReplacePreservesRangesAndUntouchedParts(t *testing.T) {
	pkg, original := openFixture(t, `<w:p w14:paraId="11111111"><w:pPr><w:jc w:val="center"/></w:pPr>`+
		`<w:bookmarkStart w:id="9" w:name="keep"/><w:commentRangeStart w:id="4"/>`+
		`<w:r><w:rPr><w:b/></w:rPr><w:t>Old</w:t></w:r>`+
		`<w:bookmarkEnd w:id="9"/><w:commentRangeEnd w:id="4"/>`+
		`<w:r><w:commentReference w:id="4"/></w:r></w:p>`+
		`<w:p w14:paraId="22222222"><w:r><w:t>Untouched</w:t></w:r></w:p>`)
	require.NoError(t, patcher.NewSession(pkg).ReplaceParagraphText("11111111", " New text ", true))
	parts, doc := saveParts(t, pkg)
	p := doc.FindElement("//p")
	assertGoldenParagraph(t, "patch_replace_ranges.xml", p)
	require.Equal(t, " New text ", oxml.ParagraphPlainText(p))
	require.Equal(t, "11111111", oxml.ParaID(p))
	for _, tag := range []string{"pPr", "bookmarkStart", "bookmarkEnd", "commentRangeStart", "commentRangeEnd", "commentReference", "b"} {
		require.NotNil(t, p.FindElement(".//"+tag), "must preserve %s", tag)
	}
	require.Equal(t, "preserve", p.FindElement(".//t").SelectAttrValue("xml:space", ""))
	children := p.ChildElements()
	require.Equal(t, "bookmarkStart", children[1].Tag)
	require.Equal(t, "commentRangeStart", children[2].Tag)
	require.Equal(t, "r", children[3].Tag)
	require.Equal(t, "bookmarkEnd", children[4].Tag)
	require.Equal(t, "commentRangeEnd", children[5].Tag)
	for name, data := range original {
		if name != "word/document.xml" {
			require.Equal(t, data, parts[name], "untouched ZIP part %s", name)
		}
	}
	before, err := oxml.ParseXML(original["word/document.xml"])
	require.NoError(t, err)
	beforePart := etree.NewDocument()
	beforePart.SetRoot(before.FindElements("//p")[1].Copy())
	afterPart := etree.NewDocument()
	afterPart.SetRoot(doc.FindElements("//p")[1].Copy())
	beforeXML, err := oxml.SerializeXML(beforePart)
	require.NoError(t, err)
	afterXML, err := oxml.SerializeXML(afterPart)
	require.NoError(t, err)
	require.Equal(t, beforeXML, afterXML)
}

func TestReplaceTextContentWithoutRemovingDrawingOrReferences(t *testing.T) {
	pkg, _ := openFixture(t, `<w:p w14:paraId="11111111"><w:r><w:rPr><w:b/></w:rPr><w:t>First</w:t><w:drawing><image/></w:drawing><w:tab/><w:t>more</w:t></w:r><w:r><w:t>Second</w:t><w:br/><w:commentReference w:id="4"/></w:r></w:p>`)
	require.NoError(t, patcher.NewSession(pkg).ReplaceParagraphText("11111111", "Replacement", false))
	_, doc := saveParts(t, pkg)
	p := doc.FindElement("//p")
	require.Equal(t, "Replacement", oxml.ParagraphPlainText(p))
	require.Nil(t, p.FindElement(".//b"))
	require.Nil(t, p.FindElement(".//tab"))
	require.Nil(t, p.FindElement(".//br"))
	require.NotNil(t, p.FindElement(".//drawing"))
	require.NotNil(t, p.FindElement(".//commentReference"))
}

func TestReplaceEmptyParagraphKeepsRanges(t *testing.T) {
	pkg, _ := openFixture(t, `<w:p w14:paraId="11111111"><w:bookmarkStart w:id="9" w:name="empty"/><w:bookmarkEnd w:id="9"/></w:p>`)
	require.NoError(t, patcher.NewSession(pkg).ReplaceParagraphText("11111111", "Added", true))
	_, doc := saveParts(t, pkg)
	p := doc.FindElement("//p")
	require.Equal(t, "Added", oxml.ParagraphPlainText(p))
	require.Equal(t, "bookmarkStart", p.ChildElements()[0].Tag)
	require.Equal(t, "r", p.ChildElements()[1].Tag)
	require.Equal(t, "bookmarkEnd", p.ChildElements()[2].Tag)
}

func TestTrackedEditOfMultipleRunsAndEmptyParagraph(t *testing.T) {
	pkg, _ := openFixture(t, `<w:p w14:paraId="11111111"><w:r><w:t>First</w:t></w:r><w:bookmarkStart w:id="9" w:name="second"/><w:r><w:rPr><w:i/></w:rPr><w:t>Second</w:t></w:r><w:bookmarkEnd w:id="9"/></w:p>`+
		`<w:p w14:paraId="22222222"><w:bookmarkStart w:id="10" w:name="empty"/><w:bookmarkEnd w:id="10"/></w:p>`)
	s := patcher.NewSession(pkg)
	require.NoError(t, s.ApplyTrackedEdit("11111111", "Replacement", "Reviewer", time.Time{}))
	require.NoError(t, s.ApplyTrackedEdit("22222222", "Added", "Reviewer", time.Time{}))
	_, doc := saveParts(t, pkg)
	ps := doc.FindElements("//p")
	require.Equal(t, "Replacement", ps[0].FindElement("ins/r/t").Text())
	dels := ps[0].FindElements("del")
	require.Len(t, dels, 2)
	require.Equal(t, "First", dels[0].FindElement("r/delText").Text())
	require.Equal(t, "Second", dels[1].FindElement("r/delText").Text())
	require.NotNil(t, dels[1].FindElement("r/rPr/i"))
	require.Len(t, ps[1].FindElements("del"), 0)
	require.Equal(t, "Added", ps[1].FindElement("ins/r/t").Text())
	require.Equal(t, "bookmarkStart", ps[1].ChildElements()[0].Tag)
	require.Equal(t, "ins", ps[1].ChildElements()[1].Tag)
	require.Equal(t, "bookmarkEnd", ps[1].ChildElements()[2].Tag)
}

func TestReplaceRejectsAmbiguousContentWithoutChangingPackage(t *testing.T) {
	for name, content := range map[string]string{
		"hyperlink":         `<w:hyperlink r:id="rId1"><w:r><w:t>Link</w:t></w:r></w:hyperlink>`,
		"simple field":      `<w:fldSimple w:instr="DATE"><w:r><w:t>Date</w:t></w:r></w:fldSimple>`,
		"complex field":     `<w:r><w:fldChar w:fldCharType="begin"/><w:instrText>DATE</w:instrText></w:r>`,
		"tracked insertion": `<w:ins w:id="1"><w:r><w:t>Inserted</w:t></w:r></w:ins>`,
		"content control":   `<w:sdt><w:sdtContent><w:r><w:t>Protected</w:t></w:r></w:sdtContent></w:sdt>`,
		"unknown container": `<extension xmlns="urn:custom"><w:r><w:t>Protected</w:t></w:r></extension>`,
	} {
		t.Run(name, func(t *testing.T) {
			pkg, original := openFixture(t, `<w:p w14:paraId="11111111"><w:r><w:t>Before</w:t></w:r>`+content+`</w:p>`)
			err := patcher.NewSession(pkg).ReplaceParagraphText("11111111", "Replacement", true)
			require.ErrorContains(t, err, "11111111")
			require.ErrorContains(t, err, "replace_text")
			parts, _ := saveParts(t, pkg)
			require.Equal(t, original, parts, "a rejected edit must not modify the package")
		})
	}
}

func TestTrackedEditPreservesRangesFormattingAndNonTextContent(t *testing.T) {
	pkg, original := openFixture(t, `<w:p w14:paraId="11111111"><w:bookmarkStart w:id="9" w:name="keep"/><w:commentRangeStart w:id="4"/>`+
		`<w:r><w:rPr><w:b/></w:rPr><w:t>Old</w:t><w:drawing><image/></w:drawing></w:r>`+
		`<w:bookmarkEnd w:id="9"/><w:commentRangeEnd w:id="4"/><w:r><w:commentReference w:id="4"/></w:r></w:p>`)
	s := patcher.NewSession(pkg)
	require.NoError(t, s.ApplyTrackedEdit("11111111", " New ", "Reviewer", time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)))
	parts, doc := saveParts(t, pkg)
	p := doc.FindElement("//p")
	assertGoldenParagraph(t, "patch_tracked_ranges.xml", p)
	for _, tag := range []string{"bookmarkStart", "bookmarkEnd", "commentRangeStart", "commentRangeEnd", "commentReference", "drawing"} {
		require.NotNil(t, p.FindElement(".//"+tag), "must preserve %s", tag)
	}
	require.Equal(t, "Old", p.FindElement(".//delText").Text())
	require.Equal(t, " New ", p.FindElement("ins/r/t").Text())
	require.NotNil(t, p.FindElement("del/r/rPr/b"))
	require.NotNil(t, p.FindElement("ins/r/rPr/b"))
	require.Nil(t, p.FindElement("del/r/drawing"), "drawing must not become a deletion")
	for name, data := range original {
		if name != "word/document.xml" {
			require.Equal(t, data, parts[name])
		}
	}
	// A second whole-paragraph edit must not silently erase the first revision.
	err := s.ApplyTrackedEdit("11111111", "Another", "Reviewer", time.Time{})
	require.ErrorContains(t, err, "11111111")
	after, _ := saveParts(t, pkg)
	require.Equal(t, parts, after)
}

func assertGoldenParagraph(t *testing.T, name string, p *etree.Element) {
	t.Helper()
	want, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", name))
	require.NoError(t, err)
	doc := oxml.NewDocument()
	doc.SetRoot(p.Copy())
	got, err := oxml.SerializeXML(doc)
	require.NoError(t, err)
	require.Equal(t, string(bytes.TrimSpace(want)), string(got))
}

func TestLawFixtureTargetedEditPreservesOtherParts(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "attach_LAW_102740_44.docx"))
	require.NoError(t, err)
	pkg, err := archive.Open(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	doc, err := pkg.Document()
	require.NoError(t, err)
	var id string
	for _, p := range doc.FindElements("//p") {
		if oxml.ParagraphPlainText(p) == "" {
			continue
		}
		if err := patcher.NewSession(pkg).ReplaceParagraphText(oxml.ParaID(p), "Targeted replacement", true); err == nil {
			id = oxml.ParaID(p)
			break
		}
	}
	require.NotEmpty(t, id, "LAW fixture needs at least one editable paragraph")
	parts, _ := saveParts(t, pkg)
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || f.Name == "word/document.xml" {
			continue
		}
		r, err := f.Open()
		require.NoError(t, err)
		original, err := io.ReadAll(r)
		require.NoError(t, err)
		require.NoError(t, r.Close())
		require.Equal(t, original, parts[f.Name], "untouched LAW part %s", f.Name)
	}
	saved, err := pkg.SaveToBytes()
	require.NoError(t, err)
	reopened, err := archive.Open(bytes.NewReader(saved), int64(len(saved)))
	require.NoError(t, err)
	target, ok := reopened.Index().Get(id)
	require.True(t, ok)
	require.Equal(t, "Targeted replacement", oxml.ParagraphPlainText(target.Element))
}
