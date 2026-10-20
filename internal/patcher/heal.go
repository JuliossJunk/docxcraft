package patcher

import (
	"bytes"
	"regexp"
	"strings"

	"docxcraft/internal/oxml"

	"github.com/beevik/etree"
)

var templateRe = regexp.MustCompile(`(\{\{[\s\S]*?\}\}|\{%[\s\S]*?%\})`)

// HealRuns coalesces split runs inside a paragraph so template placeholders are intact.
func HealRuns(p *etree.Element) {
	if p == nil || p.Tag != "p" {
		return
	}
	phase1CleanAndMerge(p)
	phase2MergeTemplates(p)
}

// HealDocumentRuns runs HealRuns on every paragraph in the document.
func HealDocumentRuns(doc *etree.Document) {
	if doc == nil || doc.Root() == nil {
		return
	}
	var walk func(*etree.Element)
	walk = func(el *etree.Element) {
		if el.Tag == "p" {
			HealRuns(el)
			return
		}
		for _, c := range el.ChildElements() {
			walk(c)
		}
	}
	walk(doc.Root())
}

func phase1CleanAndMerge(p *etree.Element) {
	// Remove proofErr and _GoBack bookmarks between runs.
	for i := len(p.Child) - 1; i >= 0; i-- {
		el, ok := p.Child[i].(*etree.Element)
		if !ok {
			continue
		}
		switch el.Tag {
		case "proofErr":
			p.RemoveChildAt(i)
		case "bookmarkStart", "bookmarkEnd":
			name := attrVal(el, "name")
			if name == "_GoBack" {
				p.RemoveChildAt(i)
			}
		}
	}

	changed := true
	for changed {
		changed = false
		children := p.ChildElements()
		for i := 0; i < len(children)-1; i++ {
			a, b := children[i], children[i+1]
			if a.Tag != "r" || b.Tag != "r" {
				continue
			}
			if !runsMergeable(a, b) {
				continue
			}
			ta := firstT(a)
			tb := firstT(b)
			if ta == nil || tb == nil {
				continue
			}
			oxml.SetRunText(ta, ta.Text()+tb.Text())
			// remove b from parent
			for ti, tok := range p.Child {
				if tok == b {
					p.RemoveChildAt(ti)
					break
				}
			}
			changed = true
			break
		}
	}
}

func runsMergeable(a, b *etree.Element) bool {
	if !runIsPlainText(a) || !runIsPlainText(b) {
		return false
	}
	return rPrCanonical(a) == rPrCanonical(b)
}

func runIsPlainText(r *etree.Element) bool {
	hasT := false
	for _, c := range r.ChildElements() {
		switch c.Tag {
		case "rPr":
			continue
		case "t":
			hasT = true
		case "br", "tab", "drawing", "pict", "object":
			return false
		default:
			return false
		}
	}
	return hasT
}

func firstT(r *etree.Element) *etree.Element {
	for _, c := range r.ChildElements() {
		if c.Tag == "t" {
			return c
		}
	}
	return nil
}

func rPrCanonical(r *etree.Element) string {
	for _, c := range r.ChildElements() {
		if c.Tag != "rPr" {
			continue
		}
		cp := c.Copy()
		stripRsidAttrs(cp)
		doc := oxml.NewDocument()
		doc.SetRoot(cp)
		var buf bytes.Buffer
		_, _ = doc.WriteTo(&buf)
		return buf.String()
	}
	return ""
}

func stripRsidAttrs(el *etree.Element) {
	filtered := el.Attr[:0]
	for _, a := range el.Attr {
		if strings.HasPrefix(a.Key, "rsid") || a.Key == "rsidR" || a.Key == "rsidRPr" || a.Key == "rsidRDefault" {
			continue
		}
		filtered = append(filtered, a)
	}
	el.Attr = filtered
	for _, c := range el.ChildElements() {
		stripRsidAttrs(c)
	}
}

func phase2MergeTemplates(p *etree.Element) {
	type span struct {
		run *etree.Element
		t   *etree.Element
		start, end int // offsets in fullText belonging to this t
	}
	var spans []span
	var full strings.Builder
	for _, c := range p.ChildElements() {
		if c.Tag != "r" {
			continue
		}
		t := firstT(c)
		if t == nil {
			continue
		}
		text := t.Text()
		start := full.Len()
		full.WriteString(text)
		spans = append(spans, span{run: c, t: t, start: start, end: full.Len()})
	}
	fullText := full.String()
	matches := templateRe.FindAllStringIndex(fullText, -1)
	if len(matches) == 0 {
		return
	}

	// Process from end to start so offsets stay valid.
	for mi := len(matches) - 1; mi >= 0; mi-- {
		m := matches[mi]
		start, end := m[0], m[1]
		rStart, rEnd := -1, -1
		for i, s := range spans {
			if s.start <= start && start < s.end {
				rStart = i
			}
			if s.start < end && end <= s.end {
				rEnd = i
			}
		}
		if rStart < 0 || rEnd < 0 || rStart == rEnd {
			continue
		}
		placeholder := fullText[start:end]
		// prefix in first run
		s0 := spans[rStart]
		prefix := fullText[s0.start:start]
		sN := spans[rEnd]
		suffix := fullText[end:sN.end]
		oxml.SetRunText(s0.t, prefix+placeholder+suffix)

		// clear intermediate and last run texts (remove empty runs)
		for i := rStart + 1; i <= rEnd; i++ {
			run := spans[i].run
			for ti, tok := range p.Child {
				if tok == run {
					p.RemoveChildAt(ti)
					break
				}
			}
		}
		// rebuild spans for remaining matches — restart phase2 once after edits
		phase2MergeTemplates(p)
		return
	}
}

func attrVal(el *etree.Element, key string) string {
	for _, a := range el.Attr {
		if a.Key == key || (a.Space == "w" && a.Key == key) {
			return a.Value
		}
	}
	return el.SelectAttrValue("w:"+key, el.SelectAttrValue(key, ""))
}
