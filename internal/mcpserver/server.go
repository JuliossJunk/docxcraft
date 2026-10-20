package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"docxcraft/internal/anchor"
	"docxcraft/internal/archive"
	"docxcraft/internal/compiler"
	"docxcraft/internal/linter"
	"docxcraft/internal/patcher"
	"docxcraft/internal/security"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RunStdio starts the MCP server on stdin/stdout.
func RunStdio(cfg *security.Config) error {
	if cfg == nil {
		cfg = security.NewConfig()
	}
	s := server.NewMCPServer("docxcraft", "0.1.0")
	registerTools(s, cfg)
	return server.ServeStdio(s)
}

func registerTools(s *server.MCPServer, cfg *security.Config) {
	s.AddTool(mcp.NewTool("inspect_skeleton",
		mcp.WithDescription("Return compact document skeleton with w14:paraId anchors"),
		mcp.WithString("file_path", mcp.Required(), mcp.Description("Path to .docx inside sandbox")),
	), wrap(cfg, "inspect", handleInspect))

	s.AddTool(mcp.NewTool("compile_ast",
		mcp.WithDescription("Compile JSON AST into a .docx using a style profile"),
		mcp.WithString("profile_name", mcp.Required(), mcp.Description("Style profile name")),
		mcp.WithString("ast_json", mcp.Required(), mcp.Description("JSON AST document string")),
		mcp.WithString("output_path", mcp.Required(), mcp.Description("Output .docx path")),
		mcp.WithBoolean("run_linter", mcp.Description("Run linter after build (default true)")),
	), wrap(cfg, "build", handleCompile))

	s.AddTool(mcp.NewTool("patch_document",
		mcp.WithDescription("Patch an existing .docx by stable anchor"),
		mcp.WithString("file_path", mcp.Required(), mcp.Description("Source .docx")),
		mcp.WithString("anchor_id", mcp.Required(), mcp.Description("w14:paraId or tbl_…")),
		mcp.WithString("action", mcp.Required(), mcp.Description("replace_text|insert_after|delete|apply_tracked_change")),
		mcp.WithString("content", mcp.Description("Text for replace/insert")),
		mcp.WithString("output_path", mcp.Required(), mcp.Description("Output .docx path")),
		mcp.WithString("copy_format_from", mcp.Description("Optional style donor anchor")),
		mcp.WithString("author", mcp.Description("Author for tracked changes")),
	), wrap(cfg, "patch", handlePatch))

	s.AddTool(mcp.NewTool("audit_document",
		mcp.WithDescription("Lint a .docx against a style profile"),
		mcp.WithString("file_path", mcp.Required(), mcp.Description("Path to .docx")),
		mcp.WithString("profile_name", mcp.Description("Style profile name")),
		mcp.WithBoolean("enable_visual", mcp.Description("Enable LibreOffice visual gate")),
		mcp.WithBoolean("strict", mcp.Description("Treat warnings as failures")),
	), wrap(cfg, "lint", handleAudit))
}

type toolHandler func(ctx context.Context, cfg *security.Config, req mcp.CallToolRequest) (*mcp.CallToolResult, error)

func wrap(cfg *security.Config, toolName string, h toolHandler) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := cfg.RequireTool(toolName); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return h(ctx, cfg, req)
	}
}

func handleInspect(ctx context.Context, cfg *security.Config, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	path, err := cfg.ResolvePath(req.GetString("file_path", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := cfg.AssertFileSize(path); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	pkg, err := archive.OpenFile(path)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	doc, err := pkg.Document()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	blocks := anchor.InspectSkeleton(doc, pkg.Index())
	payload := map[string]any{"ok": true, "file_path": path, "blocks": blocks}
	return jsonResult(payload)
}

func handleCompile(ctx context.Context, cfg *security.Config, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	if err := cfg.AssertWritable(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	out, err := cfg.ResolvePath(req.GetString("output_path", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	profileName := req.GetString("profile_name", "")
	astJSON := req.GetString("ast_json", "")
	runLint := req.GetBool("run_linter", true)

	res, err := compiler.Compile([]byte(astJSON), profileName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := res.Package.SaveToBytes()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	payload := map[string]any{"ok": true, "output_path": out}
	if runLint {
		rep, err := linter.Audit(res.Package, linter.Options{ProfileName: profileName})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		payload["lint_report"] = rep
	}
	return jsonResult(payload)
}

func handlePatch(ctx context.Context, cfg *security.Config, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	if err := cfg.AssertWritable(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	inPath, err := cfg.ResolvePath(req.GetString("file_path", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	outPath, err := cfg.ResolvePath(req.GetString("output_path", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := cfg.AssertFileSize(inPath); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	pkg, err := archive.OpenFile(inPath)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	sess := patcher.NewSession(pkg)
	anchorID := req.GetString("anchor_id", "")
	action := req.GetString("action", "")
	content := req.GetString("content", "")
	var newAnchors []string

	switch action {
	case "replace_text":
		if err := sess.ReplaceParagraphText(anchorID, content, true); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	case "insert_after":
		newAnchors, err = sess.InsertAfterAnchor(anchorID, content)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	case "delete":
		if err := sess.DeleteByAnchor(anchorID); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	case "apply_tracked_change":
		if err := sess.ApplyTrackedEdit(anchorID, content, req.GetString("author", "DocxCraft"), time.Time{}); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	default:
		return mcp.NewToolResultError(fmt.Sprintf("unknown action %q", action)), nil
	}

	if donor := req.GetString("copy_format_from", ""); donor != "" && action == "insert_after" && len(newAnchors) > 0 {
		_ = sess.CloneStyleFrom(donor, newAnchors[0])
	}

	data, err := pkg.SaveToBytes()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return jsonResult(map[string]any{
		"ok":              true,
		"output_path":     outPath,
		"new_anchors":     newAnchors,
		"affected_anchor": anchorID,
	})
}

func handleAudit(ctx context.Context, cfg *security.Config, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_ = ctx
	path, err := cfg.ResolvePath(req.GetString("file_path", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := cfg.AssertFileSize(path); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	pkg, err := archive.OpenFile(path)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	rep, err := linter.Audit(pkg, linter.Options{
		ProfileName:  req.GetString("profile_name", ""),
		EnableVisual: req.GetBool("enable_visual", false),
		Strict:       req.GetBool("strict", false),
		InputPath:    path,
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return jsonResult(map[string]any{"ok": true, "report": rep})
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}
