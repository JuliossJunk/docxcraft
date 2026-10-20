// Command docxcraft is the CLI and MCP entrypoint for DocxCraft.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"docxcraft/internal/anchor"
	"docxcraft/internal/archive"
	"docxcraft/internal/compiler"
	"docxcraft/internal/linter"
	"docxcraft/internal/mcpserver"
	"docxcraft/internal/patcher"
	"docxcraft/internal/security"

	"github.com/spf13/cobra"
)

func main() {
	cfg := security.NewConfig()
	var (
		sandboxDir string
		maxSize    int64
		readonly   bool
		whitelist  string
	)

	root := &cobra.Command{
		Use:           "docxcraft",
		Short:         "DocxCraft — deterministic .docx generation, patching, and linting",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg.SandboxDir = sandboxDir
			if cfg.SandboxDir == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				cfg.SandboxDir = cwd
			}
			cfg.MaxSize = maxSize
			cfg.ReadOnly = readonly
			cfg.Whitelist = security.ParseWhitelist(whitelist)
			return nil
		},
	}
	root.PersistentFlags().StringVar(&sandboxDir, "sandbox-dir", "", "filesystem sandbox root (default: cwd)")
	root.PersistentFlags().Int64Var(&maxSize, "max-size", security.DefaultMaxSize, "max input .docx size in bytes")
	root.PersistentFlags().BoolVar(&readonly, "readonly", false, "forbid write operations")
	root.PersistentFlags().StringVar(&whitelist, "whitelist", "", "CSV of allowed commands (empty = all)")

	root.AddCommand(cmdMCP(cfg))
	root.AddCommand(cmdBuild(cfg))
	root.AddCommand(cmdInspect(cfg))
	root.AddCommand(cmdLint(cfg))
	root.AddCommand(cmdPatch(cfg))

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func requireTool(cfg *security.Config, name string) error {
	return cfg.RequireTool(name)
}

func cmdMCP(cfg *security.Config) *cobra.Command {
	var transport string
	c := &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP server (stdio)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireTool(cfg, "mcp"); err != nil {
				return err
			}
			if transport != "" && transport != "stdio" {
				return fmt.Errorf("only stdio transport is supported")
			}
			return mcpserver.RunStdio(cfg)
		},
	}
	c.Flags().StringVar(&transport, "transport", "stdio", "MCP transport")
	return c
}

func cmdBuild(cfg *security.Config) *cobra.Command {
	var profile, output string
	var runLint bool
	c := &cobra.Command{
		Use:   "build [ast.json]",
		Short: "Compile JSON AST to .docx",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireTool(cfg, "build"); err != nil {
				return err
			}
			if err := cfg.AssertWritable(); err != nil {
				return err
			}
			astPath, err := cfg.ResolvePath(args[0])
			if err != nil {
				return err
			}
			outPath, err := cfg.ResolvePath(output)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(astPath)
			if err != nil {
				return err
			}
			res, err := compiler.Compile(data, profile)
			if err != nil {
				return err
			}
			bytes, err := res.Package.SaveToBytes()
			if err != nil {
				return err
			}
			if err := os.WriteFile(outPath, bytes, 0o644); err != nil {
				return err
			}
			fmt.Println(outPath)
			if runLint {
				rep, err := linter.Audit(res.Package, linter.Options{ProfileName: profile})
				if err != nil {
					return err
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(rep)
				os.Exit(linter.ExitCode(rep, false))
			}
			return nil
		},
	}
	c.Flags().StringVarP(&profile, "profile", "p", "corporate_executive", "style profile")
	c.Flags().StringVarP(&output, "output", "o", "out.docx", "output .docx path")
	c.Flags().BoolVar(&runLint, "lint", false, "run linter after build")
	return c
}

func cmdInspect(cfg *security.Config) *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "inspect [file.docx]",
		Short: "Print document skeleton with anchors",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireTool(cfg, "inspect"); err != nil {
				return err
			}
			path, err := cfg.ResolvePath(args[0])
			if err != nil {
				return err
			}
			if err := cfg.AssertFileSize(path); err != nil {
				return err
			}
			pkg, err := archive.OpenFile(path)
			if err != nil {
				return err
			}
			doc, err := pkg.Document()
			if err != nil {
				return err
			}
			blocks := anchor.InspectSkeleton(doc, pkg.Index())
			if asJSON || true {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(blocks)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", true, "emit JSON")
	return c
}

func cmdLint(cfg *security.Config) *cobra.Command {
	var profile string
	var strict, visual bool
	c := &cobra.Command{
		Use:   "lint [file.docx]",
		Short: "Audit .docx against a style profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireTool(cfg, "lint"); err != nil {
				return err
			}
			path, err := cfg.ResolvePath(args[0])
			if err != nil {
				return err
			}
			if err := cfg.AssertFileSize(path); err != nil {
				return err
			}
			pkg, err := archive.OpenFile(path)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
			rep, err := linter.Audit(pkg, linter.Options{
				ProfileName:  profile,
				Strict:       strict,
				EnableVisual: visual,
			})
			if err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(rep)
			os.Exit(linter.ExitCode(rep, strict))
			return nil
		},
	}
	c.Flags().StringVarP(&profile, "profile", "p", "corporate_executive", "style profile")
	c.Flags().BoolVar(&strict, "strict", false, "treat warnings as errors")
	c.Flags().BoolVar(&visual, "visual", false, "enable LibreOffice visual gate")
	return c
}

func cmdPatch(cfg *security.Config) *cobra.Command {
	var anchorID, action, content, output, copyFrom string
	c := &cobra.Command{
		Use:   "patch [file.docx]",
		Short: "Patch document by w14:paraId anchor",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireTool(cfg, "patch"); err != nil {
				return err
			}
			if err := cfg.AssertWritable(); err != nil {
				return err
			}
			inPath, err := cfg.ResolvePath(args[0])
			if err != nil {
				return err
			}
			outPath, err := cfg.ResolvePath(output)
			if err != nil {
				return err
			}
			if err := cfg.AssertFileSize(inPath); err != nil {
				return err
			}
			pkg, err := archive.OpenFile(inPath)
			if err != nil {
				return err
			}
			sess := patcher.NewSession(pkg)
			switch action {
			case "replace_text":
				if err := sess.ReplaceParagraphText(anchorID, content, true); err != nil {
					return err
				}
			case "insert_after":
				ids, err := sess.InsertAfterAnchor(anchorID, content)
				if err != nil {
					return err
				}
				if copyFrom != "" && len(ids) > 0 {
					_ = sess.CloneStyleFrom(copyFrom, ids[0])
				}
				fmt.Println(ids)
			default:
				return fmt.Errorf("unsupported action %q", action)
			}
			data, err := pkg.SaveToBytes()
			if err != nil {
				return err
			}
			return os.WriteFile(outPath, data, 0o644)
		},
	}
	c.Flags().StringVarP(&anchorID, "anchor", "a", "", "anchor id")
	c.Flags().StringVar(&action, "action", "replace_text", "replace_text|insert_after")
	c.Flags().StringVar(&content, "content", "", "text content")
	c.Flags().StringVarP(&output, "output", "o", "patched.docx", "output path")
	c.Flags().StringVar(&copyFrom, "copy-format-from", "", "donor anchor for style clone")
	_ = c.MarkFlagRequired("anchor")
	return c
}
