package compiler

import (
	"fmt"

	"docxcraft/internal/archive"
	"docxcraft/internal/oxml"
	"docxcraft/internal/profile"

	"github.com/beevik/etree"
)

// Result is the output of a successful compilation.
type Result struct {
	Package *archive.Package
}

// Compile builds a new .docx from AST and a resolved style profile name via Default loader.
func Compile(astJSON []byte, profileName string) (*Result, error) {
	loader, err := profile.Default()
	if err != nil {
		return nil, err
	}
	p, err := loader.Get(profileName)
	if err != nil {
		return nil, err
	}
	return CompileWithProfile(astJSON, p)
}

// CompileWithProfile builds a .docx from AST and an already-loaded profile.
func CompileWithProfile(astJSON []byte, style *profile.StyleProfile) (*Result, error) {
	docAST, err := ParseAST(astJSON)
	if err != nil {
		return nil, err
	}
	res, err := profile.Resolve(style)
	if err != nil {
		return nil, err
	}

	pkg, err := archive.New()
	if err != nil {
		return nil, fmt.Errorf("compiler: %w", err)
	}
	xmlDoc, err := pkg.Document()
	if err != nil {
		return nil, fmt.Errorf("compiler document: %w", err)
	}

	body := oxml.Body(xmlDoc)
	if body == nil {
		return nil, fmt.Errorf("compiler: missing w:body")
	}
	clearBodyKeepSect(body)
	applySectPr(body, res)

	for i, b := range docAST.Blocks {
		if err := renderBlock(body, b, res); err != nil {
			return nil, fmt.Errorf("compiler block %d: %w", i, err)
		}
	}

	pkg.MarkDirty("word/document.xml")
	if err := pkg.RebuildIndex(); err != nil {
		return nil, fmt.Errorf("compiler anchors: %w", err)
	}
	return &Result{Package: pkg}, nil
}

func renderBlock(body *etree.Element, b Block, res *profile.Resolved) error {
	switch b.Type {
	case "heading":
		renderHeading(body, b, res)
	case "paragraph":
		renderParagraph(body, b, res)
	case "table":
		renderTable(body, b, res)
	case "callout":
		renderCallout(body, b, res)
	case "list":
		renderList(body, b, res)
	case "page_break":
		renderPageBreak(body)
	default:
		return fmt.Errorf("unknown type %q", b.Type)
	}
	return nil
}

