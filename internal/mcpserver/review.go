package mcpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JuliossJunk/docxcraft/internal/archive"
	"github.com/JuliossJunk/docxcraft/internal/patcher"
	"github.com/JuliossJunk/docxcraft/internal/security"
	"github.com/mark3labs/mcp-go/mcp"
)

type inspectReviewArgs struct {
	FilePath string `json:"file_path" jsonschema:"Source .docx path"`
	Query    string `json:"query,omitempty" jsonschema:"Literal case-sensitive text filter"`
	Offset   int    `json:"offset,omitempty" jsonschema:"Pagination cursor from next_offset"`
}

type applyReviewArgs struct {
	FilePath  string               `json:"file_path" jsonschema:"Source .docx path"`
	SourceSHA string               `json:"source_sha256" jsonschema:"Source SHA-256 returned by inspect_review"`
	Edits     []patcher.ReviewEdit `json:"edits" jsonschema:"One batch against the original text. quote selects literal text. text is the replacement, or the insertion when position is before or after. Empty text deletes on replace. all applies explicitly to every match."`
	Output    string               `json:"output_path,omitempty" jsonschema:"Optional new .docx path. Defaults to an unused -reviewed copy alongside the source."`
	Author    string               `json:"author,omitempty" jsonschema:"Word revision author. Defaults to DocxCraft."`
}

func openReview(cfg *security.Config, file string) (*archive.Package, string, string, error) {
	if !strings.EqualFold(filepath.Ext(file), ".docx") {
		return nil, "", "", fmt.Errorf("review: expected a .docx path")
	}
	path, err := cfg.ResolvePath(file)
	if err != nil {
		return nil, "", "", err
	}
	if err := cfg.AssertFileSize(path); err != nil {
		return nil, "", "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "", "", fmt.Errorf("review: open source: %w", err)
	}
	defer func() { _ = f.Close() }()
	limit := cfg.MaxSize
	if limit <= 0 {
		limit = security.DefaultMaxSize
	}
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, "", "", fmt.Errorf("review: read source: %w", err)
	}
	if int64(len(raw)) > limit {
		return nil, "", "", fmt.Errorf("review: source exceeds max-size")
	}
	pkg, err := archive.OpenWithOptions(bytes.NewReader(raw), int64(len(raw)), archive.OpenOptions{MaxUncompressedSize: limit})
	return pkg, path, fmt.Sprintf("%x", sha256.Sum256(raw)), err
}

func handleInspectReview(_ context.Context, cfg *security.Config, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args inspectReviewArgs
	if err := req.BindArguments(&args); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	pkg, path, sha, err := openReview(cfg, args.FilePath)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	paragraphs, err := patcher.NewSession(pkg).ReviewParagraphs()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if args.Offset < 0 || args.Offset > len(paragraphs) {
		return mcp.NewToolResultError("review: invalid offset"), nil
	}
	const pageBytes = 24000
	page := make([]patcher.ReviewParagraph, 0)
	i, length := args.Offset, 0
	for ; i < len(paragraphs); i++ {
		p := paragraphs[i]
		match := strings.Index(p.Text, args.Query)
		if match < 0 {
			continue
		}
		if len(p.Text) > pageBytes {
			start := max(0, match-pageBytes/4)
			end := min(len(p.Text), start+pageBytes)
			for start < end && !utf8.RuneStart(p.Text[start]) {
				start++
			}
			for end < len(p.Text) && !utf8.RuneStart(p.Text[end]) {
				end++
			}
			p.Text = p.Text[start:end]
			p.Truncated = true
		}
		if len(page) >= 50 || (len(page) > 0 && length+len(p.Text) > pageBytes) {
			break
		}
		length += len(p.Text)
		page = append(page, p)
	}
	result := map[string]any{"ok": true, "file_path": path, "source_sha256": sha, "paragraphs": page}
	if i < len(paragraphs) {
		result["next_offset"] = i
	}
	return jsonResult(result)
}

func handleApplyReview(ctx context.Context, cfg *security.Config, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args applyReviewArgs
	if err := req.BindArguments(&args); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	path, result, err := applyReview(ctx, cfg, args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return jsonResult(map[string]any{"ok": true, "output_path": path, "changes": result.Changes})
}

func applyReview(ctx context.Context, cfg *security.Config, args applyReviewArgs) (string, patcher.ReviewResult, error) {
	var empty patcher.ReviewResult
	if err := cfg.AssertWritable(); err != nil {
		return "", empty, err
	}
	pkg, source, sha, err := openReview(cfg, args.FilePath)
	if err != nil {
		return "", empty, err
	}
	if args.SourceSHA != sha {
		return "", empty, fmt.Errorf("review: source_sha256 does not match; inspect the source again")
	}
	dir, stem := filepath.Dir(source), strings.TrimSuffix(source, filepath.Ext(source))
	out := ""
	if args.Output != "" {
		if !strings.EqualFold(filepath.Ext(args.Output), ".docx") {
			return "", empty, fmt.Errorf("review: output must have a .docx extension")
		}
		out, err = cfg.ResolvePath(args.Output)
		if err != nil {
			return "", empty, err
		}
		if _, err := os.Lstat(out); err == nil {
			return "", empty, fmt.Errorf("review: output already exists: %s", out)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", empty, fmt.Errorf("review: check output: %w", err)
		}
		dir = filepath.Dir(out)
	}
	result, err := patcher.NewSession(pkg).ApplyReview(args.Edits, args.Author, time.Time{})
	if err != nil {
		return "", empty, err
	}
	temp, err := os.CreateTemp(dir, ".docxcraft-review-*")
	if err != nil {
		return "", empty, fmt.Errorf("review: stage output: %w", err)
	}
	defer func() { _ = os.Remove(temp.Name()) }()
	saveErr := pkg.Save(temp)
	st, statErr := temp.Stat()
	limit := cfg.MaxSize
	if limit <= 0 {
		limit = security.DefaultMaxSize
	}
	var verifyErr error
	if saveErr == nil && statErr == nil {
		var reopened *archive.Package
		reopened, verifyErr = archive.OpenWithOptions(temp, st.Size(), archive.OpenOptions{MaxUncompressedSize: limit})
		if verifyErr == nil {
			verifyErr = pkg.VerifyPreservedParts(reopened)
		}
		if verifyErr == nil {
			verifyErr = result.Verify(reopened)
		}
	}
	if err := errors.Join(saveErr, statErr, verifyErr, temp.Close()); err != nil {
		return "", empty, fmt.Errorf("review: verify staged output: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", empty, err
	}
	// A hard link publishes the already verified bytes atomically and fails if
	// another request claimed that name. The private staging name is removed.
	if out != "" {
		if err := os.Link(temp.Name(), out); err != nil {
			return "", empty, fmt.Errorf("review: publish %s: %w", out, err)
		}
		return out, result, nil
	}
	for n := 1; ; n++ {
		out = stem + "-reviewed.docx"
		if n > 1 {
			out = fmt.Sprintf("%s-reviewed-%d.docx", stem, n)
		}
		if err := os.Link(temp.Name(), out); err == nil {
			return out, result, nil
		} else if !errors.Is(err, fs.ErrExist) {
			return "", empty, fmt.Errorf("review: publish %s: %w", out, err)
		}
	}
}
