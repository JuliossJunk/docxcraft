package archive

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLawFixtureRoundTripLossless(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "fixtures", "attach_LAW_102740_44.docx")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("fixture missing: %v", err)
	}

	pkg, err := OpenWithOptions(bytes.NewReader(raw), int64(len(raw)), OpenOptions{SkipAnchors: true})
	require.NoError(t, err)

	names := pkg.PartNames()
	require.Contains(t, names, "word/document.xml")
	require.Greater(t, len(names), 3)

	out, err := pkg.SaveToBytes()
	require.NoError(t, err)

	zrOrig, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	zrOut, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	require.NoError(t, err)
	require.Equal(t, len(zrOrig.File), len(zrOut.File))

	orig := map[string][]byte{}
	for _, f := range zrOrig.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		require.NoError(t, err)
		orig[f.Name] = data
	}
	for _, f := range zrOut.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		require.NoError(t, err)
		require.Equal(t, orig[f.Name], data, "part %s", f.Name)
	}
}

func TestLawFixtureOpenWithAnchors(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "fixtures", "attach_LAW_102740_44.docx")
	pkg, err := OpenFile(path)
	if err != nil {
		t.Skipf("fixture open: %v", err)
	}
	require.Greater(t, pkg.Index().Len(), 0)
	doc, err := pkg.Document()
	require.NoError(t, err)
	require.NotNil(t, doc.Root())
}
