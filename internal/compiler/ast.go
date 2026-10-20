package compiler

import (
	"encoding/json"
	"fmt"
)

// Document is the top-level declarative AST.
type Document struct {
	Title    string            `json:"title"`
	Metadata map[string]string `json:"metadata"`
	Blocks   []Block           `json:"blocks"`
}

// Block is a typed content node. Type discriminates the payload fields used.
type Block struct {
	Type string `json:"type"`

	// heading
	Level    int    `json:"level,omitempty"`
	Text     string `json:"text,omitempty"`
	AnchorID string `json:"anchor_id,omitempty"`

	// paragraph
	Runs      []Run  `json:"runs,omitempty"`
	Alignment string `json:"alignment,omitempty"`

	// table
	Headers      []string   `json:"headers,omitempty"`
	Rows         [][]string `json:"rows,omitempty"`
	ColWidthsCm  []float64  `json:"col_widths_cm,omitempty"`

	// callout
	Title   string `json:"title,omitempty"`
	Variant string `json:"variant,omitempty"`

	// list
	ListType string   `json:"list_type,omitempty"`
	Items    []string `json:"items,omitempty"`
}

// Run is an inline styled text fragment.
type Run struct {
	Text   string `json:"text"`
	Bold   bool   `json:"bold,omitempty"`
	Italic bool   `json:"italic,omitempty"`
	Color  string `json:"color,omitempty"`
}

// ParseAST unmarshals and validates a JSON AST document.
func ParseAST(data []byte) (*Document, error) {
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("compiler ast: %w", err)
	}
	if err := validateDocument(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func validateDocument(doc *Document) error {
	if doc == nil {
		return fmt.Errorf("compiler ast: nil document")
	}
	for i, b := range doc.Blocks {
		switch b.Type {
		case "heading":
			if b.Level < 1 || b.Level > 3 {
				return fmt.Errorf("compiler ast: block %d heading level must be 1–3", i)
			}
			if b.Text == "" {
				return fmt.Errorf("compiler ast: block %d heading text required", i)
			}
		case "paragraph":
			if b.Text == "" && len(b.Runs) == 0 {
				return fmt.Errorf("compiler ast: block %d paragraph needs text or runs", i)
			}
		case "table":
			if len(b.Headers) == 0 && len(b.Rows) == 0 {
				return fmt.Errorf("compiler ast: block %d table is empty", i)
			}
		case "callout":
			if b.Text == "" && b.Title == "" {
				return fmt.Errorf("compiler ast: block %d callout needs title or text", i)
			}
		case "list":
			if len(b.Items) == 0 {
				return fmt.Errorf("compiler ast: block %d list has no items", i)
			}
		case "page_break":
			// ok
		case "":
			return fmt.Errorf("compiler ast: block %d missing type", i)
		default:
			return fmt.Errorf("compiler ast: block %d unknown type %q", i, b.Type)
		}
	}
	return nil
}
