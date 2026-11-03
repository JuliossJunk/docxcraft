package patcher

import (
	"fmt"
	"strconv"
	"time"

	"github.com/JuliossJunk/docxcraft/internal/archive"
	"github.com/JuliossJunk/docxcraft/internal/oxml"

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

// ReplaceParagraphText replaces paragraph text while retaining range markers,
// references, drawings, paragraph properties, and the first text run's format.
// Rich text containers with ambiguous replacement semantics are rejected before
// mutation; callers can choose a more specific operation instead of losing data.
func (s *Session) ReplaceParagraphText(anchorID, newText string, keepFormat bool) error {
	t, err := s.lookupP(anchorID)
	if err != nil {
		return err
	}
	p := t.Element
	runs, err := editableRuns(p)
	if err != nil {
		return fmt.Errorf("patcher replace_text anchor %q: %w", anchorID, err)
	}
	var target *etree.Element
	insertAt := 0
	for _, r := range runs {
		for i := len(r.Child) - 1; i >= 0; i-- {
			el, ok := r.Child[i].(*etree.Element)
			if !ok || !isTextContent(el) {
				continue
			}
			if target == nil || target == r {
				target = r
				insertAt = i
			}
			r.RemoveChildAt(i)
		}
	}
	if target == nil {
		target = etree.NewElement("w:r")
		p.InsertChildAt(paragraphInsertionIndex(p), target)
	}
	text := etree.NewElement("w:t")
	oxml.SetRunText(text, newText)
	target.InsertChildAt(insertAt, text)
	if !keepFormat {
		for _, child := range target.ChildElements() {
			if oxml.IsW(child) && child.Tag == "rPr" {
				target.RemoveChild(child)
			}
		}
	}
	s.Pkg.MarkDirty("word/document.xml")
	return nil
}

// editableRuns validates the complete paragraph before a mutation starts.
// Only direct runs are edited: text in a drawing or embedded document is not
// paragraph text. Unknown markup stays in place rather than being flattened.
func editableRuns(p *etree.Element) ([]*etree.Element, error) {
	var runs []*etree.Element
	for _, child := range p.ChildElements() {
		if !oxml.IsW(child) {
			if containsTextContent(child) {
				return nil, fmt.Errorf("unsupported text container %s", child.FullTag())
			}
			continue
		}
		switch child.Tag {
		case "hyperlink", "fldSimple", "sdt", "customXml", "ins", "del", "moveFrom", "moveTo", "subDoc":
			return nil, fmt.Errorf("cannot replace paragraph containing w:%s without discarding its semantics", child.Tag)
		case "r":
			for _, content := range child.ChildElements() {
				if oxml.IsW(content) && (content.Tag == "fldChar" || content.Tag == "instrText" || content.Tag == "delText") {
					return nil, fmt.Errorf("cannot replace paragraph containing w:%s without discarding its semantics", content.Tag)
				}
			}
			runs = append(runs, child)
		default:
			if child.Tag != "pPr" && containsTextContent(child) {
				return nil, fmt.Errorf("unsupported text container %s", child.FullTag())
			}
		}
	}
	return runs, nil
}

func containsTextContent(el *etree.Element) bool {
	if isTextContent(el) || (oxml.IsW(el) && el.Tag == "delText") {
		return true
	}
	for _, child := range el.ChildElements() {
		if containsTextContent(child) {
			return true
		}
	}
	return false
}

func isTextContent(el *etree.Element) bool {
	if !oxml.IsW(el) {
		return false
	}
	switch el.Tag {
	case "t", "tab", "br", "cr", "noBreakHyphen", "softHyphen":
		return true
	}
	return false
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

// ApplyTrackedEdit records deletions at the original text runs and inserts the
// replacement at the first edit location. Markers and non-text content stay in
// place. Existing revisions and ambiguous rich text are rejected before mutation.
func (s *Session) ApplyTrackedEdit(anchorID, newText, author string, ts time.Time) error {
	t, err := s.lookupP(anchorID)
	if err != nil {
		return err
	}
	p := t.Element
	runs, err := editableRuns(p)
	if err != nil {
		return fmt.Errorf("patcher apply_tracked_change anchor %q: %w", anchorID, err)
	}
	var textRuns []*etree.Element
	for _, r := range runs {
		for _, child := range r.ChildElements() {
			if isTextContent(child) {
				textRuns = append(textRuns, r)
				break
			}
		}
	}
	ids, err := s.revisionIDs(len(textRuns) + 1)
	if err != nil {
		return fmt.Errorf("patcher apply_tracked_change anchor %q: %w", anchorID, err)
	}
	if author == "" {
		author = "DocxCraft"
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	date := ts.Format("2006-01-02T15:04:05Z")
	ins := etree.NewElement("w:ins")
	ins.CreateAttr("w:id", ids[len(ids)-1])
	ins.CreateAttr("w:author", author)
	ins.CreateAttr("w:date", date)
	insR := etree.NewElement("w:r")
	if len(textRuns) > 0 {
		for _, child := range textRuns[0].ChildElements() {
			if oxml.IsW(child) && child.Tag == "rPr" {
				insR.AddChild(child.Copy())
			}
		}
	}
	insT := etree.NewElement("w:t")
	oxml.SetRunText(insT, newText)
	insR.AddChild(insT)
	ins.AddChild(insR)

	for i, r := range textRuns {
		del := etree.NewElement("w:del")
		del.CreateAttr("w:id", ids[i])
		del.CreateAttr("w:author", author)
		del.CreateAttr("w:date", date)
		deletedRun := r.Copy()
		for _, child := range deletedRun.ChildElements() {
			if isTextContent(child) {
				if child.Tag == "t" {
					child.Tag = "delText"
				}
			} else if !oxml.IsW(child) || child.Tag != "rPr" {
				deletedRun.RemoveChild(child)
			}
		}
		del.AddChild(deletedRun)
		for _, child := range r.ChildElements() {
			if isTextContent(child) {
				r.RemoveChild(child)
			}
		}
		pos := r.Index()
		p.InsertChildAt(pos, del)
		if i == 0 {
			p.InsertChildAt(pos+1, ins)
		}
	}
	if len(textRuns) == 0 {
		p.InsertChildAt(paragraphInsertionIndex(p), ins)
	}
	s.Pkg.MarkDirty("word/document.xml")
	return nil
}

// paragraphInsertionIndex places text inside existing ranges in empty paragraphs.
func paragraphInsertionIndex(p *etree.Element) int {
	for i, tok := range p.Child {
		if el, ok := tok.(*etree.Element); ok && oxml.IsW(el) &&
			(el.Tag == "bookmarkEnd" || el.Tag == "commentRangeEnd" || el.Tag == "r") {
			return i
		}
	}
	return len(p.Child)
}

// revisionIDs reserves unused nonnegative decimal IDs in the current document.
// Scanning the DOM, rather than a process counter, covers reopened documents and
// existing IDs with leading zeroes. Other w:id ranges are reserved as well.
func (s *Session) revisionIDs(count int) ([]string, error) {
	doc, err := s.Pkg.Document()
	if err != nil {
		return nil, err
	}
	used := make(map[int64]bool)
	var walk func(*etree.Element)
	walk = func(el *etree.Element) {
		for _, attr := range el.Attr {
			if (attr.Space == "w" && attr.Key == "id") || attr.Key == "w:id" {
				if id, err := strconv.ParseInt(attr.Value, 10, 64); err == nil {
					used[id] = true
				}
			}
		}
		for _, child := range el.ChildElements() {
			walk(child)
		}
	}
	walk(doc.Root())
	ids := make([]string, 0, count)
	for candidate := int64(0); len(ids) < count; candidate++ {
		if !used[candidate] {
			ids = append(ids, strconv.FormatInt(candidate, 10))
		}
	}
	return ids, nil
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
