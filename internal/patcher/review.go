package patcher

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/JuliossJunk/docxcraft/internal/archive"
	"github.com/JuliossJunk/docxcraft/internal/oxml"
	"github.com/beevik/etree"
)

// ReviewEdit selects literal text in the original document. Position defaults
// to replace; before and after insert next to a match. An empty replacement
// deletes the match. All permits multiple matches only when explicitly set.
type ReviewEdit struct {
	AnchorID string `json:"anchor_id,omitempty"`
	Quote    string `json:"quote"`
	Position string `json:"position,omitempty" jsonschema:"enum=replace,enum=before,enum=after"`
	Text     string `json:"text"`
	All      bool   `json:"all,omitempty"`
	Prefix   string `json:"prefix,omitempty" jsonschema:"Exact text immediately before quote, outside the edit"`
	Suffix   string `json:"suffix,omitempty" jsonschema:"Exact text immediately after quote, outside the edit"`
}

type ReviewChange struct {
	Edit     int    `json:"edit"`
	AnchorID string `json:"anchor_id"`
	Before   string `json:"before,omitempty"`
	After    string `json:"after,omitempty"`
}

type ReviewResult struct {
	Changes   []ReviewChange
	revisions []reviewRevision
}

type reviewRevision struct {
	id, kind, anchor, text string
}

type reviewRange struct {
	start, end int
	change     ReviewChange
}

type reviewPiece struct {
	start, end int
	insertion  string
	format     *etree.Element
}

type paragraphReviewPlan struct {
	p      reviewParagraph
	ranges []reviewRange
	pieces map[*etree.Element][]reviewPiece
}

// ApplyReview validates the entire batch against the original DOM before
// writing revisions. Plans and signatures are scoped to a single session.
func (s *Session) ApplyReview(edits []ReviewEdit, author string, date time.Time) (ReviewResult, error) {
	var result ReviewResult
	if len(edits) == 0 {
		return result, fmt.Errorf("review: at least one edit is required")
	}
	if author == "" {
		author = "DocxCraft"
	}
	if !validReviewText(author) {
		return result, fmt.Errorf("review: author contains unsupported characters")
	}
	paragraphs, err := s.reviewParagraphs()
	if err != nil {
		return result, err
	}
	plans := make([]paragraphReviewPlan, len(paragraphs))
	for i, p := range paragraphs {
		plans[i] = paragraphReviewPlan{p: p, pieces: make(map[*etree.Element][]reviewPiece)}
	}
	for i, edit := range edits {
		if err := planReviewEdit(plans, edit, i+1); err != nil {
			return result, fmt.Errorf("review edit %d: %w", i+1, err)
		}
	}
	count := 0
	for i := range plans {
		plan := &plans[i]
		sort.Slice(plan.ranges, func(i, j int) bool {
			a, b := plan.ranges[i], plan.ranges[j]
			return a.start < b.start || (a.start == b.start && a.end < b.end)
		})
		for j, edit := range plan.ranges {
			if j > 0 {
				prev := plan.ranges[j-1]
				if prev.end > edit.start || (prev.start == prev.end && edit.start == edit.end && prev.start == edit.start) {
					return result, fmt.Errorf("review edits %d and %d overlap at anchor %s",
						prev.change.Edit, edit.change.Edit, plan.p.AnchorID)
				}
			}
			n, err := plan.addPieces(edit)
			if err != nil {
				return result, fmt.Errorf("review edit %d at anchor %s: %w", edit.change.Edit, plan.p.AnchorID, err)
			}
			count += n
			result.Changes = append(result.Changes, edit.change)
		}
	}
	ids, err := s.revisionIDs(count)
	if err != nil {
		return ReviewResult{}, err
	}
	if date.IsZero() {
		date = time.Now()
	}
	stamp := date.UTC().Format(time.RFC3339)
	for _, plan := range plans {
		if len(plan.ranges) == 0 {
			continue
		}
		revision := func(kind, text string, run *etree.Element) *etree.Element {
			id := ids[0]
			ids = ids[1:]
			el := etree.NewElement("w:" + kind)
			el.CreateAttr("w:id", id)
			el.CreateAttr("w:author", author)
			el.CreateAttr("w:date", stamp)
			el.AddChild(run)
			result.revisions = append(result.revisions, reviewRevision{id, kind, plan.p.AnchorID, text})
			return el
		}
		plan.apply(revision)
	}
	s.Pkg.MarkNormalizedDocumentDirty()
	return result, nil
}

func validReviewText(text string) bool {
	return !strings.ContainsFunc(text, func(r rune) bool {
		return unicode.IsControl(r) || r == '\uFFFE' || r == '\uFFFF'
	})
}

func planReviewEdit(plans []paragraphReviewPlan, edit ReviewEdit, number int) error {
	position := edit.Position
	if position == "" {
		position = "replace"
	}
	if position != "replace" && position != "before" && position != "after" {
		return fmt.Errorf("unknown position %q", position)
	}
	if !validReviewText(edit.Text) || (position != "replace" && edit.Text == "") ||
		(position == "replace" && edit.Quote == edit.Text) {
		return fmt.Errorf("expected a nontrivial edit using inline text")
	}
	if edit.Quote == "" && (edit.AnchorID == "" || position == "replace" || edit.All || edit.Prefix != "" || edit.Suffix != "") {
		return fmt.Errorf("provide a literal quote, or an anchor and before/after to insert at the paragraph boundary")
	}
	matches := 0
	for i := range plans {
		plan := &plans[i]
		p := plan.p
		if edit.AnchorID != "" && !strings.EqualFold(edit.AnchorID, p.AnchorID) {
			continue
		}
		add := func(start, end int) {
			plan.ranges = append(plan.ranges, reviewRange{start, end, ReviewChange{
				Edit: number, AnchorID: p.AnchorID, Before: p.Text[start:end], After: edit.Text,
			}})
			matches++
		}
		if edit.Quote == "" {
			offset := 0
			if position == "after" {
				offset = len(p.Text)
			}
			add(offset, offset)
			continue
		}
		for from := 0; from < len(p.Text); {
			index := strings.Index(p.Text[from:], edit.Quote)
			if index == -1 {
				break
			}
			start := from + index
			end := start + len(edit.Quote)
			from = start + 1 // Count overlapping matches as ambiguous too.
			if !strings.HasSuffix(p.Text[:start], edit.Prefix) || !strings.HasPrefix(p.Text[end:], edit.Suffix) {
				continue
			}
			switch position {
			case "before":
				end = start
			case "after":
				start = end
			}
			add(start, end)
			if matches > 1 && !edit.All {
				return fmt.Errorf("quote is ambiguous; select one anchor, add prefix or suffix context, or explicitly request all matches")
			}
		}
	}
	if matches == 0 {
		return fmt.Errorf("quote or anchor not found")
	}
	return nil
}

func (plan *paragraphReviewPlan) addPieces(edit reviewRange) (int, error) {
	if plan.p.blocked != "" {
		return 0, fmt.Errorf("paragraph is inside %s", plan.p.blocked)
	}
	var left, right *reviewSpan
	count := 0
	for i := range plan.p.spans {
		span := &plan.p.spans[i]
		if edit.start == edit.end {
			if span.start < edit.start && edit.start <= span.end {
				left = span
			}
			if span.start <= edit.start && edit.start < span.end {
				right = span
			}
		}
		if edit.end <= span.start || span.end <= edit.start {
			continue
		}
		if span.blocked != "" {
			return 0, fmt.Errorf("range crosses protected %s", span.blocked)
		}
		if span.start == span.end {
			continue
		}
		piece := reviewPiece{
			start: max(edit.start, span.start) - span.start,
			end:   min(edit.end, span.end) - span.start,
		}
		if count == 0 && edit.change.After != "" {
			piece.insertion = edit.change.After
			piece.format = span.run
		}
		plan.pieces[span.node] = append(plan.pieces[span.node], piece)
		count++
	}
	if edit.start != edit.end {
		if edit.change.After != "" {
			count++
		}
		return count, nil
	}
	if left != nil && left == right && left.blocked != "" {
		return 0, fmt.Errorf("insertion is inside protected %s", left.blocked)
	}
	target := left
	if target == nil || target.blocked != "" {
		target = right
	}
	if target == nil || target.blocked != "" {
		if plan.p.Text != "" {
			return 0, fmt.Errorf("insertion has no editable text boundary")
		}
		var format *etree.Element
		for _, span := range plan.p.spans {
			if span.blocked != "" {
				return 0, fmt.Errorf("empty text boundary contains protected %s", span.blocked)
			}
			format = span.run
		}
		plan.pieces[nil] = append(plan.pieces[nil], reviewPiece{insertion: edit.change.After, format: format})
		return 1, nil
	}
	format := target.run
	if left != nil && left.run != nil {
		format = left.run
	}
	offset := edit.start - target.start
	plan.pieces[target.node] = append(plan.pieces[target.node], reviewPiece{offset, offset, edit.change.After, format})
	return 1, nil
}

func reviewRun(source *etree.Element, copyAttributes bool) *etree.Element {
	run := etree.NewElement("w:r")
	if source != nil {
		if copyAttributes {
			run.Attr = append(run.Attr, source.Attr...)
		}
		for _, child := range source.ChildElements() {
			if child.Tag == "rPr" && oxml.IsW(child) {
				run.AddChild(child.Copy())
			}
		}
	}
	oxml.NormalizeRunProperties(run)
	return run
}

func (plan paragraphReviewPlan) apply(revision func(string, string, *etree.Element) *etree.Element) {
	paragraph := plan.p.element
	insert := func(piece reviewPiece) *etree.Element {
		run := reviewRun(piece.format, false)
		oxml.SetRunText(run.CreateElement("w:t"), piece.insertion)
		return revision("ins", piece.insertion, run)
	}
	for _, piece := range plan.pieces[nil] {
		paragraph.InsertChildAt(paragraphInsertionIndex(paragraph), insert(piece))
	}
	for _, original := range paragraph.ChildElements() {
		if !oxml.IsW(original) || original.Tag != "r" {
			continue
		}
		touched := false
		for _, child := range original.ChildElements() {
			if len(plan.pieces[child]) != 0 {
				touched = true
				break
			}
		}
		if !touched {
			continue
		}
		at := original.Index()
		paragraph.RemoveChild(original)
		run := reviewRun(original, true)
		put := func(el *etree.Element) {
			paragraph.InsertChildAt(at, el)
			at++
		}
		propertyCount := len(run.Child)
		flush := func() {
			if len(run.Child) > propertyCount {
				put(run)
				run = reviewRun(original, true)
			}
		}
		for _, token := range append([]etree.Token(nil), original.Child...) {
			node, ok := token.(*etree.Element)
			if ok && oxml.IsW(node) && node.Tag == "rPr" {
				continue
			}
			pieces := plan.pieces[node]
			if len(pieces) == 0 {
				run.AddChild(token)
				continue
			}
			text := node.Text()
			cursor := 0
			writeText := func(value string) {
				if value != "" {
					t := node.Copy()
					oxml.SetRunText(t, value)
					run.AddChild(t)
				}
			}
			for _, piece := range pieces {
				writeText(text[cursor:piece.start])
				flush()
				if piece.end > piece.start {
					deleted := text[piece.start:piece.end]
					oldRun := reviewRun(original, true)
					oldText := node.Copy()
					oldText.Tag = "delText"
					oxml.SetRunText(oldText, deleted)
					oldRun.AddChild(oldText)
					put(revision("del", deleted, oldRun))
				}
				if piece.insertion != "" {
					put(insert(piece))
				}
				cursor = piece.end
			}
			writeText(text[cursor:])
		}
		flush()
	}
}

// Verify confirms the expected revisions survived serialization and reopening.
func (r ReviewResult) Verify(reopened *archive.Package) error {
	byAnchor := make(map[string]map[string]*etree.Element)
	for _, expected := range r.revisions {
		revisions, seen := byAnchor[expected.anchor]
		if !seen {
			target, ok := reopened.Index().Get(expected.anchor)
			if !ok {
				return fmt.Errorf("review verification: anchor %s missing", expected.anchor)
			}
			revisions = make(map[string]*etree.Element)
			for _, el := range target.Element.ChildElements() {
				if oxml.IsW(el) && (el.Tag == "ins" || el.Tag == "del") {
					revisions[el.SelectAttrValue("w:id", "")] = el
				}
			}
			byAnchor[expected.anchor] = revisions
		}
		el := revisions[expected.id]
		if el == nil || el.Tag != expected.kind || oxml.ParagraphPlainText(el) != expected.text {
			return fmt.Errorf("review verification: unexpected revision %s at anchor %s", expected.id, expected.anchor)
		}
	}
	return nil
}
