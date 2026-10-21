// Package docxcraft is the public Go SDK for embedding DocxCraft in other projects.
package docxcraft

import (
	"github.com/JuliossJunk/docxcraft/internal/anchor"
	"github.com/JuliossJunk/docxcraft/internal/archive"
	"github.com/JuliossJunk/docxcraft/internal/compiler"
	"github.com/JuliossJunk/docxcraft/internal/linter"
	"github.com/JuliossJunk/docxcraft/internal/patcher"
)

// Open opens a .docx package from path.
func Open(path string) (*archive.Package, error) {
	return archive.OpenFile(path)
}

// Build compiles AST JSON with the named style profile into a new package.
func Build(astJSON []byte, profileName string) (*compiler.Result, error) {
	return compiler.Compile(astJSON, profileName)
}

// Lint audits an open package.
func Lint(pkg *archive.Package, opts linter.Options) (*linter.Report, error) {
	return linter.Audit(pkg, opts)
}

// Inspect returns the compact skeleton for a package.
func Inspect(pkg *archive.Package) ([]anchor.Block, error) {
	doc, err := pkg.Document()
	if err != nil {
		return nil, err
	}
	return anchor.InspectSkeleton(doc, pkg.Index()), nil
}

// PatchSession returns a patcher session for the package.
func PatchSession(pkg *archive.Package) *patcher.Session {
	return patcher.NewSession(pkg)
}
