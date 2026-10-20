package anchor

import (
	"crypto/rand"
	"fmt"
	"strings"

	"docxcraft/internal/oxml"

	"github.com/beevik/etree"
)

const tblPrefix = "tbl_"

// Target is a resolved anchor pointing at a DOM element.
type Target struct {
	ID       string
	Kind     string // "p", "tbl", or "tr"
	Element  *etree.Element
	Parent   *etree.Element
	Location string
}

// Index maps anchor IDs to DOM targets for O(1) lookup.
type Index struct {
	byID map[string]*Target
}

// NewIndex creates an empty anchor index.
func NewIndex() *Index {
	return &Index{byID: make(map[string]*Target)}
}

// Get returns the target for id, if present.
func (idx *Index) Get(id string) (*Target, bool) {
	if idx == nil {
		return nil, false
	}
	t, ok := idx.byID[id]
	return t, ok
}

// Len returns the number of indexed anchors.
func (idx *Index) Len() int {
	if idx == nil {
		return 0
	}
	return len(idx.byID)
}

// EnsureAnchors walks all w:p and w:tbl nodes, assigns valid w14:paraId values,
// and registers them in idx. Returns true if the DOM was mutated.
func EnsureAnchors(doc *etree.Document, location string, idx *Index) (mutated bool, err error) {
	if doc == nil || doc.Root() == nil {
		return false, fmt.Errorf("anchor: empty document")
	}
	if idx == nil {
		return false, fmt.Errorf("anchor: nil index")
	}
	if idx.byID == nil {
		idx.byID = make(map[string]*Target)
	}

	root := doc.Root()
	hadW14 := hasW14Namespace(root)
	oxml.EnsureW14Namespace(root)
	if !hadW14 {
		mutated = true
	}

	existing := make(map[string]struct{})
	for id := range idx.byID {
		existing[id] = struct{}{}
		if strings.HasPrefix(id, tblPrefix) {
			existing[strings.TrimPrefix(id, tblPrefix)] = struct{}{}
		}
	}

	var walk func(parent, el *etree.Element)
	walk = func(parent, el *etree.Element) {
		switch el.Tag {
		case "p":
			if changed := ensureParagraphAnchor(el, parent, location, idx, existing); changed {
				mutated = true
			}
		case "tbl":
			if changed := ensureTableAnchor(el, parent, location, idx, existing); changed {
				mutated = true
			}
		}
		for _, child := range el.ChildElements() {
			walk(el, child)
		}
	}
	walk(nil, root)
	return mutated, nil
}

func hasW14Namespace(root *etree.Element) bool {
	for _, attr := range root.Attr {
		if (attr.Space == "xmlns" && attr.Key == "w14") ||
			attr.Key == "xmlns:w14" ||
			(attr.Space == "" && attr.Key == "xmlns:w14") {
			return true
		}
	}
	return false
}

func ensureParagraphAnchor(el, parent *etree.Element, location string, idx *Index, existing map[string]struct{}) bool {
	mutated := false
	id := getW14Attr(el, "paraId")
	if !isValidParaID(id) || idTaken(idx, id) {
		id = generateValidParaID(existing)
		setW14Attr(el, "paraId", id)
		setW14Attr(el, "textId", id)
		mutated = true
	}
	existing[id] = struct{}{}
	idx.byID[id] = &Target{
		ID:       id,
		Kind:     "p",
		Element:  el,
		Parent:   parent,
		Location: location,
	}
	return mutated
}

func ensureTableAnchor(el, parent *etree.Element, location string, idx *Index, existing map[string]struct{}) bool {
	mutated := false
	firstTR := firstChildByTag(el, "tr")
	if firstTR == nil {
		return false
	}
	rowID := getW14Attr(firstTR, "paraId")
	if !isValidParaID(rowID) || idTaken(idx, rowID) || idTaken(idx, tblPrefix+rowID) {
		rowID = generateValidParaID(existing)
		setW14Attr(firstTR, "paraId", rowID)
		mutated = true
	}
	existing[rowID] = struct{}{}
	tblID := tblPrefix + rowID
	existing[tblID] = struct{}{}
	idx.byID[tblID] = &Target{
		ID:       tblID,
		Kind:     "tbl",
		Element:  el,
		Parent:   parent,
		Location: location,
	}
	idx.byID[rowID] = &Target{
		ID:       rowID,
		Kind:     "tr",
		Element:  firstTR,
		Parent:   el,
		Location: location,
	}
	return mutated
}

func idTaken(idx *Index, id string) bool {
	_, ok := idx.byID[id]
	return ok
}

func firstChildByTag(parent *etree.Element, tag string) *etree.Element {
	for _, c := range parent.ChildElements() {
		if c.Tag == tag {
			return c
		}
	}
	return nil
}

func getW14Attr(el *etree.Element, key string) string {
	for _, attr := range el.Attr {
		if attr.Key == key && (attr.Space == "w14" || attr.Space == "") {
			if attr.Space == "w14" || strings.HasPrefix(attr.Key, "w14:") {
				return strings.ToUpper(attr.Value)
			}
		}
		if attr.Key == "w14:"+key {
			return strings.ToUpper(attr.Value)
		}
		if attr.Space == "w14" && attr.Key == key {
			return strings.ToUpper(attr.Value)
		}
	}
	// etree may store as Space=w14 Key=paraId
	if v := el.SelectAttrValue("w14:"+key, ""); v != "" {
		return strings.ToUpper(v)
	}
	return ""
}

func setW14Attr(el *etree.Element, key, value string) {
	full := "w14:" + key
	for i := range el.Attr {
		if el.Attr[i].Space == "w14" && el.Attr[i].Key == key {
			el.Attr[i].Value = value
			return
		}
		if el.Attr[i].Key == full {
			el.Attr[i].Value = value
			return
		}
	}
	el.CreateAttr(full, value)
}

func isValidParaID(id string) bool {
	if len(id) != 8 {
		return false
	}
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	if id == "00000000" {
		return false
	}
	var n uint32
	_, err := fmt.Sscanf(id, "%X", &n)
	if err != nil {
		return false
	}
	return n < 0x80000000
}

func generateValidParaID(existing map[string]struct{}) string {
	var b [4]byte
	for {
		_, _ = rand.Read(b[:])
		b[0] &= 0x7F
		id := fmt.Sprintf("%02X%02X%02X%02X", b[0], b[1], b[2], b[3])
		if id == "00000000" {
			continue
		}
		if _, exists := existing[id]; exists {
			continue
		}
		if _, exists := existing[tblPrefix+id]; exists {
			continue
		}
		return id
	}
}
