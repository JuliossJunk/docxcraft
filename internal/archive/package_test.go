package archive

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoundTripLosslessParts(t *testing.T) {
	raw := mustMinimalDocx()
	pkg, err := OpenWithOptions(bytes.NewReader(raw), int64(len(raw)), OpenOptions{SkipAnchors: true})
	require.NoError(t, err)

	originalNames := pkg.PartNames()
	out, err := pkg.SaveToBytes()
	require.NoError(t, err)

	pkg2, err := OpenWithOptions(bytes.NewReader(out), int64(len(out)), OpenOptions{SkipAnchors: true})
	require.NoError(t, err)
	require.Equal(t, originalNames, pkg2.PartNames())

	// Untouched parts must be byte-identical.
	zrOrig, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	zrOut, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	require.NoError(t, err)
	require.Equal(t, len(zrOrig.File), len(zrOut.File))

	origMap := map[string][]byte{}
	for _, f := range zrOrig.File {
		rc, err := f.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		require.NoError(t, err)
		origMap[f.Name] = data
	}
	for _, f := range zrOut.File {
		rc, err := f.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		require.NoError(t, err)
		require.Equal(t, origMap[f.Name], data, "part %s", f.Name)
	}
}

func TestOpenBuildsAnchors(t *testing.T) {
	raw := mustMinimalDocx()
	pkg, err := Open(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	require.NotNil(t, pkg.Index())
	require.Greater(t, pkg.Index().Len(), 0)

	_, ok := pkg.Index().Get("1A4F09C1")
	require.True(t, ok)
	_, ok = pkg.Index().Get("tbl_7C01D2E9")
	require.True(t, ok)

	doc, err := pkg.Document()
	require.NoError(t, err)
	require.NotNil(t, doc.Root())
}

func TestZipBombRejected(t *testing.T) {
	raw := mustMinimalDocx()
	_, err := OpenWithOptions(bytes.NewReader(raw), int64(len(raw)), OpenOptions{
		MaxUncompressedSize: 10,
		SkipAnchors:         true,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds limit")
}

func TestGetXMLDoesNotDirty(t *testing.T) {
	raw := mustMinimalDocx()
	pkg, err := OpenWithOptions(bytes.NewReader(raw), int64(len(raw)), OpenOptions{SkipAnchors: true})
	require.NoError(t, err)

	_, err = pkg.GetXML("word/document.xml")
	require.NoError(t, err)

	out, err := pkg.SaveToBytes()
	require.NoError(t, err)

	zrOrig, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	zrOut, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	require.NoError(t, err)

	origDoc := readZipPart(t, zrOrig, "word/document.xml")
	outDoc := readZipPart(t, zrOut, "word/document.xml")
	require.Equal(t, origDoc, outDoc)
}

func readZipPart(t *testing.T, zr *zip.Reader, name string) []byte {
	t.Helper()
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		require.NoError(t, err)
		return data
	}
	t.Fatalf("part %s not found", name)
	return nil
}
