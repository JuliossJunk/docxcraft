package archive

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"

	"docxcraft/internal/anchor"
	"docxcraft/internal/oxml"

	"github.com/beevik/etree"
)

const (
	// MaxUncompressedSize is the Zip Bomb guard (default 100 MiB).
	MaxUncompressedSize int64 = 100 << 20

	documentPart = "word/document.xml"
)

// Package is an in-memory .docx (OPC ZIP) with lazy XML DOM parsing.
type Package struct {
	mu       sync.RWMutex
	rawFiles map[string][]byte
	xmlDocs  map[string]*etree.Document
	dirty    map[string]bool
	order    []string
	index    *anchor.Index
	maxSize  int64
}

// OpenOptions configures package opening.
type OpenOptions struct {
	MaxUncompressedSize int64
	// SkipAnchors skips EnsureAnchors on word/document.xml (tests / raw round-trip).
	SkipAnchors bool
}

// Open reads a .docx from r with the default size limit and builds the anchor index.
func Open(r io.ReaderAt, size int64) (*Package, error) {
	return OpenWithOptions(r, size, OpenOptions{})
}

// OpenFile opens a .docx from a filesystem path.
func OpenFile(path string) (*Package, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("archive open file: %w", err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("archive stat: %w", err)
	}
	return Open(f, st.Size())
}

// OpenWithOptions reads a .docx with custom options.
func OpenWithOptions(r io.ReaderAt, size int64, opts OpenOptions) (*Package, error) {
	maxSize := opts.MaxUncompressedSize
	if maxSize <= 0 {
		maxSize = MaxUncompressedSize
	}

	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("archive zip: %w", err)
	}

	pkg := &Package{
		rawFiles: make(map[string][]byte),
		xmlDocs:  make(map[string]*etree.Document),
		dirty:    make(map[string]bool),
		order:    make([]string, 0, len(zr.File)),
		index:    anchor.NewIndex(),
		maxSize:  maxSize,
	}

	var total int64
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("archive open part %q: %w", f.Name, err)
		}
		data, err := io.ReadAll(io.LimitReader(rc, maxSize+1))
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("archive read part %q: %w", f.Name, err)
		}
		total += int64(len(data))
		if total > maxSize || int64(len(data)) > maxSize {
			return nil, fmt.Errorf("archive: uncompressed size exceeds limit %d bytes", maxSize)
		}
		pkg.rawFiles[f.Name] = data
		pkg.order = append(pkg.order, f.Name)
	}

	if _, ok := pkg.rawFiles[documentPart]; !ok {
		return nil, fmt.Errorf("archive: missing %s", documentPart)
	}

	if !opts.SkipAnchors {
		if err := pkg.initDocumentAnchors(); err != nil {
			return nil, err
		}
	}

	return pkg, nil
}

func (p *Package) initDocumentAnchors() error {
	doc, err := p.parsePart(documentPart)
	if err != nil {
		return err
	}
	p.xmlDocs[documentPart] = doc
	mutated, err := anchor.EnsureAnchors(doc, documentPart, p.index)
	if err != nil {
		return fmt.Errorf("archive anchors: %w", err)
	}
	if mutated {
		p.dirty[documentPart] = true
	}
	return nil
}

func (p *Package) parsePart(partName string) (*etree.Document, error) {
	data, ok := p.rawFiles[partName]
	if !ok {
		return nil, fmt.Errorf("archive: part %q not found", partName)
	}
	return oxml.ParseXML(data)
}

// Index returns the package anchor index.
func (p *Package) Index() *anchor.Index {
	return p.index
}

// PartNames returns ZIP part names in original archive order.
func (p *Package) PartNames() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, len(p.order))
	copy(out, p.order)
	return out
}

// GetXML returns a lazily parsed XML DOM for partName.
// First read does not mark the part dirty.
func (p *Package) GetXML(partName string) (*etree.Document, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if doc, ok := p.xmlDocs[partName]; ok {
		return doc, nil
	}
	doc, err := p.parsePart(partName)
	if err != nil {
		return nil, err
	}
	p.xmlDocs[partName] = doc
	return doc, nil
}

// MarkDirty marks an XML part for re-serialization on Save.
func (p *Package) MarkDirty(partName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dirty[partName] = true
}

// Document returns word/document.xml DOM (parsed if needed).
func (p *Package) Document() (*etree.Document, error) {
	return p.GetXML(documentPart)
}

// Save writes the package to w. Dirty parts are normalized then serialized;
// untouched parts are written byte-for-byte from the original archive.
func (p *Package) Save(w io.Writer) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	zw := zip.NewWriter(w)
	for _, name := range p.order {
		hdr := &zip.FileHeader{
			Name:   name,
			Method: zip.Deflate,
		}
		fw, err := zw.CreateHeader(hdr)
		if err != nil {
			_ = zw.Close()
			return fmt.Errorf("archive create %q: %w", name, err)
		}

		if p.dirty[name] {
			doc, ok := p.xmlDocs[name]
			if !ok {
				_ = zw.Close()
				return fmt.Errorf("archive: dirty part %q has no DOM", name)
			}
			oxml.NormalizeElementOrder(doc)
			data, err := oxml.SerializeXML(doc)
			if err != nil {
				_ = zw.Close()
				return fmt.Errorf("archive serialize %q: %w", name, err)
			}
			if _, err := fw.Write(data); err != nil {
				_ = zw.Close()
				return fmt.Errorf("archive write %q: %w", name, err)
			}
			continue
		}

		raw, ok := p.rawFiles[name]
		if !ok {
			_ = zw.Close()
			return fmt.Errorf("archive: missing raw part %q", name)
		}
		if _, err := fw.Write(raw); err != nil {
			_ = zw.Close()
			return fmt.Errorf("archive write %q: %w", name, err)
		}
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("archive close zip: %w", err)
	}
	return nil
}

// SaveToBytes is a convenience wrapper around Save.
func (p *Package) SaveToBytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := p.Save(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
