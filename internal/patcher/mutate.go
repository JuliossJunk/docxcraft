package patcher

import (
	"fmt"
	"time"

	"docxcraft/internal/archive"
	"docxcraft/internal/oxml"

	"github.com/beevik/etree"
)

// Session binds patch operations to an open archive package.
type Session struct {
	Pkg *archive.Package
}

// NewSession creates a patcher session for pkg.
func NewSession(pkg *archive.Package) *Session {
	return &Session{Pkg: pkg}
}

// ReplaceParagraphText replaces visible text of a paragraph anchor.
func (s *Session) ReplaceParagraphText(anchorID, newText string, keepFormat bool) error {
	t, err := s.lookupP(anchorID)
	if err != nil {
		return err
	}
	p := t.Element

	var keptRPr *etree.Element
	if keepFormat {
		for _, c := range p.ChildElements() {
			if c.Tag != "r" {
				continue
			}
			if kids := c.ChildElements(); len(kids) > 0 && kids[0].Tag == "rPr" {
				keptRPr = kids[0].Copy()
			}
			break
		}
	}

	// Remove existing runs (keep pPr).
	for i := len(p.Child) - 1; i >= 0; i-- {
		el, ok := p.Child[i].(*etree.Element)
		if !ok {
			continue
		}
		if el.Tag == "pPr" {
			continue
		}
		p.RemoveChildAt(i)
	}

	r := etree.NewElement("w:r")
	if keptRPr != nil {
		r.AddChild(keptRPr)
	}
	tt := etree.NewElement("w:t")
	oxml.SetRunText(tt, newText)
	r.AddChild(tt)
	p.AddChild(r)

	s.Pkg.MarkDirty("word/document.xml")
	return nil
}

// InsertAfterAnchor inserts new paragraph elements after the given anchor.
// Each inserted node receives a fresh w14:paraId. Returns new anchor IDs.
func (s *Session) InsertAfterAnchor(anchorID string, texts ...string) ([]string, error) {
	t, err := s.lookup(anchorID)
	if err != nil {
		return nil, err
	}
	parent := t.Parent
	if parent == nil {
		return nil, fmt.Errorf("patcher: anchor %q has no parent", anchorID)
	}

	// Find insert index after target in parent.Child
	insertAt := -1
	for i, tok := range parent.Child {
		if tok == t.Element {
			insertAt = i + 1
			break
		}
	}
	if insertAt < 0 {
		return nil, fmt.Errorf("patcher: anchor element %q not found in parent", anchorID)
	}

	inserted := make([]*etree.Element, 0, len(texts))
	for _, text := range texts {
		p := newParagraph(text)
		parent.InsertChildAt(insertAt, p)
		insertAt++
		inserted = append(inserted, p)
	}

	s.Pkg.MarkDirty("word/document.xml")
	if err := s.Pkg.RebuildIndex(); err != nil {
		return nil, err
	}

	out := make([]string, 0, len(inserted))
	for _, p := range inserted {
		id := oxml.ParaID(p)
		if id == "" {
			return nil, fmt.Errorf("patcher: inserted paragraph missing paraId after reindex")
		}
		out = append(out, id)
	}
	return out, nil
}

// DeleteByAnchor removes a paragraph or table addressed by anchor_id from its parent.
func (s *Session) DeleteByAnchor(anchorID string) error {
	t, err := s.lookup(anchorID)
	if err != nil {
		return err
	}
	if t.Kind != "p" && t.Kind != "tbl" {
		return fmt.Errorf("patcher: delete supports p/tbl anchors, got %s", t.Kind)
	}
	parent := t.Parent
	if parent == nil {
		return fmt.Errorf("patcher: anchor %q has no parent", anchorID)
	}
	removed := false
	for i, tok := range parent.Child {
		if tok == t.Element {
			parent.RemoveChildAt(i)
			removed = true
			break
		}
	}
	if !removed {
		return fmt.Errorf("patcher: could not remove anchor %q from parent", anchorID)
	}
	s.Pkg.MarkDirty("word/document.xml")
	return s.Pkg.RebuildIndex()
}

// CloneStyleFrom copies pPr and first-run rPr from src to dst, preserving dst paraId.
func (s *Session) CloneStyleFrom(srcAnchor, dstAnchor string) error {
	src, err := s.lookupP(srcAnchor)
	if err != nil {
		return err
	}
	dst, err := s.lookupP(dstAnchor)
	if err != nil {
		return err
	}

	paraID := oxml.ParaID(dst.Element)
	textID := oxml.GetW14Attr(dst.Element, "textId")

	// remove dst pPr
	for i := len(dst.Element.Child) - 1; i >= 0; i-- {
		el, ok := dst.Element.Child[i].(*etree.Element)
		if ok && el.Tag == "pPr" {
			dst.Element.RemoveChildAt(i)
		}
	}
	for _, c := range src.Element.ChildElements() {
		if c.Tag == "pPr" {
			dst.Element.InsertChildAt(0, c.Copy())
			break
		}
	}

	var srcRPr *etree.Element
	for _, c := range src.Element.ChildElements() {
		if c.Tag != "r" {
			continue
		}
		if kids := c.ChildElements(); len(kids) > 0 && kids[0].Tag == "rPr" {
			srcRPr = kids[0].Copy()
		}
		break
	}
	if srcRPr != nil {
		for _, c := range dst.Element.ChildElements() {
			if c.Tag != "r" {
				continue
			}
			for i := len(c.Child) - 1; i >= 0; i-- {
				el, ok := c.Child[i].(*etree.Element)
				if ok && el.Tag == "rPr" {
					c.RemoveChildAt(i)
				}
			}
			c.InsertChildAt(0, srcRPr)
			break
		}
	}

	if paraID != "" {
		oxml.SetParaID(dst.Element, paraID)
	}
	if textID != "" {
		oxml.SetW14Attr(dst.Element, "textId", textID)
	}

	s.Pkg.MarkDirty("word/document.xml")
	return nil
}

// ApplyTrackedEdit wraps old text in w:del and inserts w:ins with newText.
func (s *Session) ApplyTrackedEdit(anchorID, newText, author string, ts time.Time) error {
	t, err := s.lookupP(anchorID)
	if err != nil {
		return err
	}
	if author == "" {
		author = "DocxCraft"
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	date := ts.Format("2006-01-02T15:04:05Z")
	p := t.Element

	oldText := oxml.ParagraphPlainText(p)
	// clear runs
	for i := len(p.Child) - 1; i >= 0; i-- {
		el, ok := p.Child[i].(*etree.Element)
		if !ok || el.Tag == "pPr" {
			continue
		}
		p.RemoveChildAt(i)
	}

	del := etree.NewElement("w:del")
	del.CreateAttr("w:id", "1")
	del.CreateAttr("w:author", author)
	del.CreateAttr("w:date", date)
	delR := etree.NewElement("w:r")
	delT := etree.NewElement("w:delText")
	oxml.SetRunText(delT, oldText)
	delR.AddChild(delT)
	del.AddChild(delR)
	p.AddChild(del)

	ins := etree.NewElement("w:ins")
	ins.CreateAttr("w:id", "2")
	ins.CreateAttr("w:author", author)
	ins.CreateAttr("w:date", date)
	insR := etree.NewElement("w:r")
	insT := etree.NewElement("w:t")
	oxml.SetRunText(insT, newText)
	insR.AddChild(insT)
	ins.AddChild(insR)
	p.AddChild(ins)

	s.Pkg.MarkDirty("word/document.xml")
	return nil
}

func (s *Session) lookup(id string) (*etreeTarget, error) {
	t, ok := s.Pkg.Index().Get(id)
	if !ok {
		return nil, fmt.Errorf("patcher: anchor %q not found", id)
	}
	return &etreeTarget{Kind: t.Kind, Element: t.Element, Parent: t.Parent}, nil
}

type etreeTarget struct {
	Kind    string
	Element *etree.Element
	Parent  *etree.Element
}

func (s *Session) lookupP(id string) (*etreeTarget, error) {
	t, err := s.lookup(id)
	if err != nil {
		return nil, err
	}
	if t.Kind != "p" {
		return nil, fmt.Errorf("patcher: anchor %q is %s, want p", id, t.Kind)
	}
	return t, nil
}

func newParagraph(text string) *etree.Element {
	p := etree.NewElement("w:p")
	r := etree.NewElement("w:r")
	tt := etree.NewElement("w:t")
	oxml.SetRunText(tt, text)
	r.AddChild(tt)
	p.AddChild(r)
	return p
}

