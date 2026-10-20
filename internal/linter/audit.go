package linter

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"docxcraft/internal/archive"
	"docxcraft/internal/profile"

	"github.com/beevik/etree"
)

const (
	locationDoc = "word/document.xml"
	dxaTol      = 20
	szTol       = 1 // half-points
)

// Audit runs deterministic DF rules (and optional visual gate) against an open package.
func Audit(pkg *archive.Package, opts Options) (*Report, error) {
	if pkg == nil {
		return fatalReport(opts.ProfileName, "DF-020", "nil package"), nil
	}
	rep := &Report{
		ProfileName: opts.ProfileName,
		Issues:      nil,
		VisualGate: VisualResult{
			Status:   "skipped",
			Reason:   "disabled",
			PNGPaths: []string{},
		},
	}

	var res *profile.Resolved
	if opts.ProfileName != "" {
		loader, err := profile.Default()
		if err != nil {
			return nil, err
		}
		p, err := loader.Get(opts.ProfileName)
		if err != nil {
			return nil, err
		}
		res, err = profile.Resolve(p)
		if err != nil {
			return nil, err
		}
	}

	doc, err := pkg.Document()
	if err != nil {
		rep.add(Issue{
			Code: "DF-020", Severity: "fatal", Location: locationDoc,
			Message: err.Error(), SuggestedFix: "Pass a valid .docx; check --max-size",
		})
		rep.finalize(opts.Strict)
		return rep, nil
	}

	body := findBody(doc)
	if body == nil {
		rep.add(Issue{
			Code: "DF-020", Severity: "fatal", Location: locationDoc,
			Message: "missing w:body", SuggestedFix: "Repair or recreate the document",
		})
		rep.finalize(opts.Strict)
		return rep, nil
	}

	runStructural(body, rep)
	if res != nil {
		runProfileRules(body, res, rep)
	}

	if opts.EnableVisual {
		timeout := opts.VisualTimeout
		if timeout <= 0 {
			timeout = 60 * time.Second
		}
		docxPath, cleanup, err := resolveVisualInput(pkg, opts)
		if err != nil {
			rep.VisualGate = VisualResult{Status: "error", Reason: err.Error(), PNGPaths: []string{}}
		} else {
			rep.VisualGate = runVisualGate(docxPath, opts.WorkDir, timeout, opts.KeepVisual)
			if cleanup != "" && !opts.KeepVisual && rep.VisualGate.Status != "ok" {
				_ = os.Remove(cleanup)
			}
		}
	}

	rep.finalize(opts.Strict)
	return rep, nil
}

func resolveVisualInput(pkg *archive.Package, opts Options) (path string, tempFile string, err error) {
	if opts.InputPath != "" {
		return opts.InputPath, "", nil
	}
	dir := opts.WorkDir
	if dir == "" {
		dir, err = os.MkdirTemp("", "docxcraft-visual-in-*")
		if err != nil {
			return "", "", fmt.Errorf("visual input tmpdir: %w", err)
		}
	}
	path = filepath.Join(dir, "document.docx")
	data, err := pkg.SaveToBytes()
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", "", err
	}
	return path, path, nil
}

func fatalReport(profileName, code, msg string) *Report {
	r := &Report{
		ProfileName: profileName,
		Issues: []Issue{{
			Code: code, Severity: "fatal", Location: locationDoc,
			Message: msg, SuggestedFix: "Pass a valid .docx",
		}},
		VisualGate: VisualResult{Status: "skipped", Reason: "disabled", PNGPaths: []string{}},
	}
	r.finalize(false)
	return r
}

func findBody(doc *etree.Document) *etree.Element {
	if doc == nil || doc.Root() == nil {
		return nil
	}
	for _, c := range doc.Root().ChildElements() {
		if c.Tag == "body" {
			return c
		}
	}
	return nil
}

func runStructural(body *etree.Element, rep *Report) {
	sect := findSectPr(body)
	if sect == nil {
		rep.add(Issue{
			Code: "DF-003", Severity: "warning", Location: locationDoc,
			Message: "Missing w:sectPr in w:body", SuggestedFix: "Add section properties with page geometry",
		})
	}

	seenIDs := map[string]int{}
	var paras []*etree.Element
	collectParasAndTables(body, &paras, nil)

	for _, p := range paras {
		id := paraID(p)
		if id == "" {
			rep.add(Issue{
				Code: "DF-015", Severity: "warning", Location: locationDoc,
				Message: "Paragraph without w14:paraId", SuggestedFix: "Call EnsureAnchors and save",
			})
		} else {
			seenIDs[id]++
			if seenIDs[id] == 2 {
				rep.add(Issue{
					Code: "DF-016", Severity: "error", AnchorID: id, Location: locationDoc,
					Message: fmt.Sprintf("Duplicate w14:paraId %s", id),
					SuggestedFix: "Regenerate conflicting paraId values",
				})
			}
		}
		if hasBrokenTemplate(p) {
			rep.add(Issue{
				Code: "DF-017", Severity: "warning", AnchorID: id, Location: locationDoc,
				Message: "Split template placeholder across runs",
				SuggestedFix: "Run HealRuns to coalesce {{ placeholders }}",
			})
		}
	}

	checkHeadingHierarchy(paras, rep)
	checkEmptyParagraphGaps(body, rep)
	checkTables(body, nil, rep)
	checkTrackedChanges(body, rep)
}

func runProfileRules(body *etree.Element, res *profile.Resolved, rep *Report) {
	sect := findSectPr(body)
	if sect != nil {
		checkPageGeometry(sect, res, rep)
	}
	checkTableContentWidth(body, res, rep)
	checkTypography(body, res, rep)
}

func findSectPr(body *etree.Element) *etree.Element {
	for _, c := range body.ChildElements() {
		if c.Tag == "sectPr" {
			return c
		}
	}
	return nil
}

func checkPageGeometry(sect *etree.Element, res *profile.Resolved, rep *Report) {
	var pgSz, pgMar *etree.Element
	for _, c := range sect.ChildElements() {
		switch c.Tag {
		case "pgSz":
			pgSz = c
		case "pgMar":
			pgMar = c
		}
	}
	if pgSz != nil {
		w := attrInt(pgSz, "w")
		h := attrInt(pgSz, "h")
		if !near(w, res.PageW, dxaTol) || !near(h, res.PageH, dxaTol) {
			rep.add(Issue{
				Code: "DF-001", Severity: "error", Location: locationDoc,
				Message: fmt.Sprintf("Page size %dx%d dxa does not match profile %dx%d", w, h, res.PageW, res.PageH),
				SuggestedFix: "Set sectPr/pgSz from the style profile",
			})
		}
	}
	if pgMar != nil {
		checks := []struct{ key string; want int }{
			{"top", res.MarginTop}, {"bottom", res.MarginBottom},
			{"left", res.MarginLeft}, {"right", res.MarginRight},
		}
		for _, c := range checks {
			got := attrInt(pgMar, c.key)
			if !near(got, c.want, dxaTol) {
				rep.add(Issue{
					Code: "DF-002", Severity: "error", Location: locationDoc,
					Message: fmt.Sprintf("Margin %s=%d dxa, profile expects %d", c.key, got, c.want),
					SuggestedFix: "Sync pgMar with YAML page.margins",
				})
				break
			}
		}
	}
}

func checkHeadingHierarchy(paras []*etree.Element, rep *Report) {
	lastLevel := 0
	for i, p := range paras {
		level := headingLevel(p)
		if level == 0 {
			continue
		}
		if lastLevel > 0 && level > lastLevel+1 {
			rep.add(Issue{
				Code: "DF-005", Severity: "error", AnchorID: paraID(p), Location: locationDoc,
				Message: fmt.Sprintf("Heading level jumps from H%d to H%d", lastLevel, level),
				SuggestedFix: "Insert intermediate heading level or lower this heading",
			})
		}
		// DF-006: heading followed by another heading of same/higher or end
		nextLevel := 0
		for j := i + 1; j < len(paras); j++ {
			if isEmptyParagraph(paras[j]) {
				continue
			}
			nextLevel = headingLevel(paras[j])
			if nextLevel == 0 {
				nextLevel = -1 // content
			}
			break
		}
		if nextLevel == 0 || (nextLevel > 0 && nextLevel <= level) {
			rep.add(Issue{
				Code: "DF-006", Severity: "warning", AnchorID: paraID(p), Location: locationDoc,
				Message: "Heading without following body content",
				SuggestedFix: "Add a paragraph after the heading or merge headings",
			})
		}
		lastLevel = level
	}
}

// checkEmptyParagraphGaps flags DF-004 only for empty body-level paragraphs
// sandwiched between non-empty blocks (not leading/trailing spacers alone).
func checkEmptyParagraphGaps(body *etree.Element, rep *Report) {
	type block struct {
		el    *etree.Element
		empty bool
		kind  string // p or tbl
	}
	var blocks []block
	for _, c := range body.ChildElements() {
		switch c.Tag {
		case "p":
			blocks = append(blocks, block{el: c, empty: isEmptyParagraph(c), kind: "p"})
		case "tbl":
			blocks = append(blocks, block{el: c, empty: false, kind: "tbl"})
		}
	}
	for i, b := range blocks {
		if b.kind != "p" || !b.empty {
			continue
		}
		hasBefore, hasAfter := false, false
		for j := i - 1; j >= 0; j-- {
			if !blocks[j].empty {
				hasBefore = true
				break
			}
		}
		for j := i + 1; j < len(blocks); j++ {
			if !blocks[j].empty {
				hasAfter = true
				break
			}
		}
		if hasBefore && hasAfter {
			rep.add(Issue{
				Code: "DF-004", Severity: "error", AnchorID: paraID(b.el), Location: locationDoc,
				Message: "Empty paragraph used for vertical spacing between content",
				SuggestedFix: "Remove empty paragraph; use w:spacing instead",
			})
		}
	}
}

func checkTables(body *etree.Element, res *profile.Resolved, rep *Report) {
	_ = res
	var walk func(*etree.Element)
	walk = func(el *etree.Element) {
		if el.Tag == "tbl" {
			auditTable(el, nil, rep)
			return
		}
		for _, c := range el.ChildElements() {
			walk(c)
		}
	}
	walk(body)
}

func checkTableContentWidth(body *etree.Element, res *profile.Resolved, rep *Report) {
	if res == nil {
		return
	}
	var walk func(*etree.Element)
	walk = func(el *etree.Element) {
		if el.Tag == "tbl" {
			anchor := tableAnchor(el)
			var gridCols []int
			for _, c := range el.ChildElements() {
				if c.Tag != "tblGrid" {
					continue
				}
				for _, g := range c.ChildElements() {
					if g.Tag == "gridCol" {
						gridCols = append(gridCols, attrInt(g, "w"))
					}
				}
			}
			if len(gridCols) == 0 {
				return
			}
			sum := 0
			for _, w := range gridCols {
				sum += w
			}
			if sum > res.ContentWidthDxa+dxaTol {
				rep.add(Issue{
					Code: "DF-007", Severity: "error", AnchorID: anchor, Location: locationDoc,
					Message: fmt.Sprintf("Table column widths sum %d > content width %d", sum, res.ContentWidthDxa),
					SuggestedFix: "Recalculate col_widths / tcW in dxa",
				})
			}
			return
		}
		for _, c := range el.ChildElements() {
			walk(c)
		}
	}
	walk(body)
}

func auditTable(tbl *etree.Element, res *profile.Resolved, rep *Report) {
	_ = res
	anchor := tableAnchor(tbl)
	var gridCols []int
	var trs []*etree.Element
	fixed := false
	for _, c := range tbl.ChildElements() {
		switch c.Tag {
		case "tblGrid":
			for _, g := range c.ChildElements() {
				if g.Tag == "gridCol" {
					gridCols = append(gridCols, attrInt(g, "w"))
				}
			}
		case "tr":
			trs = append(trs, c)
		case "tblPr":
			for _, p := range c.ChildElements() {
				if p.Tag == "tblLayout" && strings.EqualFold(attrVal(p, "type"), "fixed") {
					fixed = true
				}
			}
		}
	}
	if len(gridCols) == 0 {
		rep.add(Issue{
			Code: "DF-008", Severity: "error", AnchorID: anchor, Location: locationDoc,
			Message: "Table missing explicit tblGrid/gridCol widths",
			SuggestedFix: "Set gridCol and tcW in dxa",
		})
	}

	for i, tr := range trs {
		hasHeader, hasCant := false, false
		for _, c := range tr.ChildElements() {
			if c.Tag != "trPr" {
				continue
			}
			for _, p := range c.ChildElements() {
				switch p.Tag {
				case "tblHeader":
					hasHeader = true
				case "cantSplit":
					hasCant = true
				}
			}
		}
		if i == 0 && !hasHeader {
			rep.add(Issue{
				Code: "DF-009", Severity: "error", AnchorID: anchor, Location: locationDoc,
				Message: "Table header row missing w:tblHeader",
				SuggestedFix: "Add <w:tblHeader/> to the first row",
			})
		}
		if !hasCant {
			rep.add(Issue{
				Code: "DF-010", Severity: "warning", AnchorID: anchor, Location: locationDoc,
				Message: "Table row missing w:cantSplit",
				SuggestedFix: "Add <w:cantSplit/> on table rows",
			})
		}
		if fixed {
			for _, tc := range tr.ChildElements() {
				if tc.Tag != "tc" {
					continue
				}
				if !tcHasWidth(tc) {
					rep.add(Issue{
						Code: "DF-018", Severity: "error", AnchorID: anchor, Location: locationDoc,
						Message: "Fixed-layout table cell missing tcW",
						SuggestedFix: "Set cell width tcW in dxa",
					})
					break
				}
			}
		}
	}
}

func checkTypography(body *etree.Element, res *profile.Resolved, rep *Report) {
	wantFont := res.Profile.Typography.BodyFont
	wantSz := res.BodySizeHalfPts
	wantLine := res.LineSpacingTwips

	var paras []*etree.Element
	collectParasAndTables(body, &paras, nil)
	for _, p := range paras {
		level := headingLevel(p)
		id := paraID(p)
		if level > 0 {
			h, ok := res.Profile.Heading(level)
			if !ok {
				continue
			}
			wantHSz := profile.PtToHalfPoints(h.SizePt)
			sz, bold := firstRunMetrics(p)
			if sz > 0 && !near(sz, wantHSz, szTol) {
				rep.add(Issue{
					Code: "DF-014", Severity: "error", AnchorID: id, Location: locationDoc,
					Message: fmt.Sprintf("Heading H%d size %d half-pts, profile %d", level, sz, wantHSz),
					SuggestedFix: "Apply heading size from the style profile",
				})
			}
			if h.Bold && !bold {
				rep.add(Issue{
					Code: "DF-014", Severity: "error", AnchorID: id, Location: locationDoc,
					Message: fmt.Sprintf("Heading H%d should be bold per profile", level),
					SuggestedFix: "Set bold on heading runs",
				})
			}
			continue
		}
		font, sz := firstRunFontSize(p)
		if font != "" && !strings.EqualFold(font, wantFont) {
			rep.add(Issue{
				Code: "DF-011", Severity: "error", AnchorID: id, Location: locationDoc,
				Message: fmt.Sprintf("Body font %q does not match profile %q", font, wantFont),
				SuggestedFix: "Set rFonts to the profile body_font",
			})
		}
		if sz > 0 && !near(sz, wantSz, szTol) {
			rep.add(Issue{
				Code: "DF-012", Severity: "error", AnchorID: id, Location: locationDoc,
				Message: fmt.Sprintf("Body size %d half-pts, profile %d", sz, wantSz),
				SuggestedFix: "Fix w:sz / w:szCs to match body_size_pt",
			})
		}
		if line := paragraphLine(p); line > 0 && !near(line, wantLine, 5) {
			rep.add(Issue{
				Code: "DF-013", Severity: "warning", AnchorID: id, Location: locationDoc,
				Message: fmt.Sprintf("Line spacing %d, profile %d", line, wantLine),
				SuggestedFix: "Set spacing/@w:line from profile line_spacing",
			})
		}
	}
}

func checkTrackedChanges(body *etree.Element, rep *Report) {
	found := false
	var walk func(*etree.Element)
	walk = func(el *etree.Element) {
		if el.Tag == "ins" || el.Tag == "del" {
			found = true
			return
		}
		for _, c := range el.ChildElements() {
			walk(c)
		}
	}
	walk(body)
	if found {
		rep.add(Issue{
			Code: "DF-019", Severity: "info", Location: locationDoc,
			Message: "Document contains tracked changes (w:ins/w:del)",
			SuggestedFix: "Accept or reject revisions if not intentional",
		})
	}
}

func collectParasAndTables(el *etree.Element, paras *[]*etree.Element, tables *[]*etree.Element) {
	for _, c := range el.ChildElements() {
		switch c.Tag {
		case "p":
			*paras = append(*paras, c)
		case "tbl":
			if tables != nil {
				*tables = append(*tables, c)
			}
			// still collect nested paras for structural ID checks
			collectParasAndTables(c, paras, tables)
		case "sectPr":
			continue
		default:
			collectParasAndTables(c, paras, tables)
		}
	}
}

func paraID(p *etree.Element) string {
	for _, a := range p.Attr {
		if (a.Space == "w14" && a.Key == "paraId") || a.Key == "w14:paraId" || a.Key == "paraId" {
			return strings.ToUpper(a.Value)
		}
	}
	return ""
}

func tableAnchor(tbl *etree.Element) string {
	for _, c := range tbl.ChildElements() {
		if c.Tag == "tr" {
			id := paraID(c)
			if id != "" {
				return "tbl_" + id
			}
			break
		}
	}
	return ""
}

func headingLevel(p *etree.Element) int {
	style := paragraphStyle(p)
	switch style {
	case "Heading1", "heading 1":
		return 1
	case "Heading2", "heading 2":
		return 2
	case "Heading3", "heading 3":
		return 3
	}
	return 0
}

func paragraphStyle(p *etree.Element) string {
	for _, c := range p.ChildElements() {
		if c.Tag != "pPr" {
			continue
		}
		for _, pp := range c.ChildElements() {
			if pp.Tag == "pStyle" {
				return attrVal(pp, "val")
			}
		}
	}
	return ""
}

func isEmptyParagraph(p *etree.Element) bool {
	hasDrawing := false
	text := ""
	var walk func(*etree.Element)
	walk = func(el *etree.Element) {
		switch el.Tag {
		case "t", "delText":
			text += el.Text()
		case "drawing", "pict":
			hasDrawing = true
		}
		for _, c := range el.ChildElements() {
			walk(c)
		}
	}
	walk(p)
	return !hasDrawing && strings.TrimSpace(text) == ""
}

func hasBrokenTemplate(p *etree.Element) bool {
	full := ""
	wholeInOne := false
	var walk func(*etree.Element)
	walk = func(el *etree.Element) {
		if el.Tag == "t" {
			t := el.Text()
			full += t
			if strings.Contains(t, "{{") && strings.Contains(t, "}}") {
				wholeInOne = true
			}
			if strings.Contains(t, "{%") && strings.Contains(t, "%}") {
				wholeInOne = true
			}
		}
		for _, c := range el.ChildElements() {
			walk(c)
		}
	}
	walk(p)
	if wholeInOne {
		return false
	}
	hasOpen := strings.Contains(full, "{{") || strings.Contains(full, "{%")
	hasClose := strings.Contains(full, "}}") || strings.Contains(full, "%}")
	return hasOpen && hasClose
}

func tcHasWidth(tc *etree.Element) bool {
	for _, c := range tc.ChildElements() {
		if c.Tag != "tcPr" {
			continue
		}
		for _, p := range c.ChildElements() {
			if p.Tag == "tcW" {
				return attrInt(p, "w") > 0 || attrVal(p, "type") != "auto"
			}
		}
	}
	return false
}

func firstRunFontSize(p *etree.Element) (font string, sz int) {
	for _, c := range p.ChildElements() {
		if c.Tag != "r" {
			continue
		}
		for _, rc := range c.ChildElements() {
			if rc.Tag != "rPr" {
				continue
			}
			for _, x := range rc.ChildElements() {
				switch x.Tag {
				case "rFonts":
					font = attrVal(x, "ascii")
					if font == "" {
						font = attrVal(x, "hAnsi")
					}
				case "sz":
					sz = attrInt(x, "val")
				}
			}
		}
		return font, sz
	}
	return "", 0
}

func firstRunMetrics(p *etree.Element) (sz int, bold bool) {
	for _, c := range p.ChildElements() {
		if c.Tag != "r" {
			continue
		}
		for _, rc := range c.ChildElements() {
			if rc.Tag != "rPr" {
				continue
			}
			for _, x := range rc.ChildElements() {
				switch x.Tag {
				case "sz":
					sz = attrInt(x, "val")
				case "b":
					bold = true
				}
			}
		}
		return sz, bold
	}
	return 0, false
}

func paragraphLine(p *etree.Element) int {
	for _, c := range p.ChildElements() {
		if c.Tag != "pPr" {
			continue
		}
		for _, pp := range c.ChildElements() {
			if pp.Tag == "spacing" {
				return attrInt(pp, "line")
			}
		}
	}
	return 0
}

func attrVal(el *etree.Element, key string) string {
	for _, a := range el.Attr {
		if a.Key == key || a.Key == "w:"+key || (a.Space == "w" && a.Key == key) {
			return a.Value
		}
	}
	return el.SelectAttrValue("w:"+key, el.SelectAttrValue(key, ""))
}

func attrInt(el *etree.Element, key string) int {
	v := attrVal(el, key)
	n, _ := strconv.Atoi(v)
	return n
}

func near(got, want, tol int) bool {
	d := got - want
	if d < 0 {
		d = -d
	}
	return d <= tol
}
