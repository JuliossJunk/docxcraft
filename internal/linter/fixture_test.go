package linter

import (
	"path/filepath"
	"testing"

	"github.com/JuliossJunk/docxcraft/internal/archive"

	"github.com/stretchr/testify/require"
)

func TestLawFixtureAuditRuns(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "fixtures", "attach_LAW_102740_44.docx")
	pkg, err := archive.OpenFile(path)
	if err != nil {
		t.Skipf("fixture: %v", err)
	}
	rep, err := Audit(pkg, Options{
		ProfileName:  "legal_contract",
		EnableVisual: false,
		InputPath:    path,
	})
	require.NoError(t, err)
	require.NotNil(t, rep)
	require.Equal(t, "legal_contract", rep.ProfileName)
	// Structural audit must complete; real contracts often have DF findings.
	require.Equal(t, len(rep.Issues), rep.IssueCount)
	require.Equal(t, "skipped", rep.VisualGate.Status)
}

func TestVisualGateFullWhenLibreOfficePresent(t *testing.T) {
	if !LibreOfficeAvailable() {
		t.Skip("LibreOffice not installed")
	}
	path := filepath.Join("..", "..", "testdata", "fixtures", "attach_LAW_102740_44.docx")
	pkg, err := archive.OpenFile(path)
	require.NoError(t, err)

	work := t.TempDir()
	rep, err := Audit(pkg, Options{
		EnableVisual:  true,
		InputPath:     path,
		WorkDir:       work,
		KeepVisual:    true,
		VisualTimeout: 0, // default 60s
	})
	require.NoError(t, err)
	require.Equal(t, "ok", rep.VisualGate.Status)
	require.NotEmpty(t, rep.VisualGate.PDFPath)
	require.FileExists(t, rep.VisualGate.PDFPath)
}

func TestFindLibreOfficePaths(t *testing.T) {
	// Smoke: function does not panic; result depends on environment.
	_ = LibreOfficeAvailable()
}
