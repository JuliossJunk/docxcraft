package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/JuliossJunk/docxcraft/internal/patcher"
	"github.com/JuliossJunk/docxcraft/internal/security"
	"github.com/stretchr/testify/require"
)

func TestReviewPublishesVerifiedCopies(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	source := filepath.Join(dir, "law.docx")
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "attach_LAW_102740_44.docx"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(source, raw, 0600))
	cfg := &security.Config{SandboxDir: dir}
	pkg, _, hash, err := openReview(cfg, source)
	require.NoError(t, err)
	paragraphs, err := patcher.NewSession(pkg).ReviewParagraphs()
	require.NoError(t, err)
	args := applyReviewArgs{FilePath: source, SourceSHA: hash}
	for _, p := range paragraphs {
		if p.Table != "" && len(p.Text) > 2 && !strings.ContainsAny(p.Text, "\r\n\t") {
			args.Edits = []patcher.ReviewEdit{{AnchorID: p.AnchorID, Quote: p.Text, Text: "Updated table cell"}}
			break
		}
	}
	require.Len(t, args.Edits, 1)

	failed := args
	failed.SourceSHA = "stale"
	_, _, err = applyReview(ctx, cfg, failed)
	require.ErrorContains(t, err, "does not match")
	failed = args
	failed.Edits = append(append([]patcher.ReviewEdit(nil), args.Edits...), patcher.ReviewEdit{
		AnchorID: "FFFFFFFF", Quote: "Missing", Text: "Replacement",
	})
	_, _, err = applyReview(ctx, cfg, failed)
	require.ErrorContains(t, err, "edit 2")
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 1)

	type output struct {
		path string
		err  error
	}
	results := make(chan output, 2)
	var wg sync.WaitGroup
	for range cap(results) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			path, _, err := applyReview(ctx, cfg, args)
			results <- output{path, err}
		}()
	}
	wg.Wait()
	close(results)
	var paths []string
	for result := range results {
		require.NoError(t, result.err)
		paths = append(paths, filepath.Base(result.path))
	}
	require.ElementsMatch(t, []string{"law-reviewed.docx", "law-reviewed-2.docx"}, paths)

	args.Output = source
	_, _, err = applyReview(ctx, cfg, args)
	require.ErrorContains(t, err, "output already exists")
	after, err := os.ReadFile(source)
	require.NoError(t, err)
	require.Equal(t, raw, after)
	files, err = os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 3)
}
