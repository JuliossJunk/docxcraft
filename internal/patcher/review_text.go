package patcher

import (
	"fmt"
	"strings"

	"github.com/JuliossJunk/docxcraft/internal/oxml"
	"github.com/beevik/etree"
)

// ReviewParagraph describes a paragraph in the main document story.
type ReviewParagraph struct {
	AnchorID  string `json:"anchor_id"`
	Text      string `json:"text"`
	Heading   string `json:"heading,omitempty"`
	Style     string `json:"style,omitempty"`
	Table     string `json:"table_anchor,omitempty"`
	Row       int    `json:"row,omitempty"`
	Cell      int    `json:"cell,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

type reviewParagraph struct {
	ReviewParagraph
	element *etree.Element
	spans   []reviewSpan
	blocked string
}

type reviewSpan struct {
	start, end int
	run, node  *etree.Element
	blocked    string
}

// ReviewParagraphs lists main-story paragraphs, including table cells.
func (s *Session) ReviewParagraphs() ([]ReviewParagraph, error) {
	paragraphs, err := s.reviewParagraphs()
	if err != nil {
		return nil, err
	}
	result := make([]ReviewParagraph, 0, len(paragraphs))
	for _, p := range paragraphs {
		result = append(result, p.ReviewParagraph)
	}
	return result, nil
}

func (s *Session) reviewParagraphs() ([]reviewParagraph, error) {
	doc, err := s.Pkg.Document()
	if err != nil {
		return nil, err
	}
	body := doc.Root().SelectElement("w:body")
	if body == nil {
		return nil, fmt.Errorf("review: main document has no w:body")
	}
	var result []reviewParagraph
	var heading string
	fieldDepth := 0
	var walk func(*etree.Element, ReviewParagraph, string)
	walk = func(container *etree.Element, location ReviewParagraph, blocked string) {
		for _, child := range container.ChildElements() {
			if !oxml.IsW(child) {
				continue
			}
			switch child.Tag {
			case "p":
				p := reviewParagraph{ReviewParagraph: location, element: child, blocked: blocked}
				p.AnchorID = oxml.ParaID(child)
				if style := child.FindElement("w:pPr/w:pStyle"); style != nil {
					p.Style = style.SelectAttrValue("w:val", "")
				}
				p.project(&fieldDepth)
				if location.Table == "" && (strings.HasPrefix(p.Style, "Heading") ||
					child.FindElement("w:pPr/w:outlineLvl") != nil) {
					heading = p.Text
				}
				p.Heading = heading
				result = append(result, p)
			case "tbl":
				table := ReviewParagraph{}
				if row := child.SelectElement("w:tr"); row != nil {
					table.Table = "tbl_" + oxml.ParaID(row)
				}
				walk(child, table, blocked)
			case "tr":
				location.Row++
				walk(child, location, blocked)
			case "tc":
				location.Cell++
				walk(child, location, blocked)
			case "sdt", "customXml", "ins", "moveTo":
				walk(child, location, child.FullTag())
			case "sdtContent":
				walk(child, location, blocked)
			}
		}
	}
	walk(body, ReviewParagraph{}, "")
	return result, nil
}

func (p *reviewParagraph) project(fieldDepth *int) {
	var text strings.Builder
	add := func(value string, run, node *etree.Element, blocked string) {
		start := text.Len()
		text.WriteString(value)
		p.spans = append(p.spans, reviewSpan{start, text.Len(), run, node, blocked})
	}
	for _, child := range p.element.ChildElements() {
		if oxml.IsW(child) && child.Tag == "r" {
			runReason := p.blocked
			if child.FindElement("w:rPr/w:rPrChange") != nil {
				runReason = "w:rPrChange"
			}
			for _, part := range child.ChildElements() {
				if oxml.IsW(part) && part.Tag == "rPr" {
					continue
				}
				reason := runReason
				switch {
				case oxml.IsW(part) && part.Tag == "fldChar":
					reason = "complex field"
					switch part.SelectAttrValue("w:fldCharType", "") {
					case "begin":
						(*fieldDepth)++
					case "end":
						if *fieldDepth > 0 {
							(*fieldDepth)--
						}
					}
				case *fieldDepth > 0:
					reason = "complex field"
				case !oxml.IsW(part) || part.Tag != "t":
					reason = part.FullTag()
				}
				add(reviewText(part), child, part, reason)
			}
			continue
		}
		if oxml.IsW(child) {
			switch child.Tag {
			case "pPr", "bookmarkStart", "bookmarkEnd", "commentRangeStart", "commentRangeEnd", "proofErr", "permStart", "permEnd":
				continue
			}
		}
		add(reviewText(child), nil, child, child.FullTag())
	}
	p.Text = text.String()
}

func reviewText(el *etree.Element) string {
	if oxml.IsW(el) {
		switch el.Tag {
		case "t":
			return el.Text()
		case "tab":
			return "\t"
		case "br", "cr":
			return "\n"
		case "noBreakHyphen":
			return "\u2011"
		case "softHyphen":
			return "\u00ad"
		case "del", "moveFrom", "drawing", "pict", "object", "rPr", "pPr":
			return ""
		}
	}
	var text strings.Builder
	for _, child := range el.ChildElements() {
		text.WriteString(reviewText(child))
	}
	return text.String()
}
