package compiler

import (
	"strconv"

	"github.com/JuliossJunk/docxcraft/internal/profile"

	"github.com/beevik/etree"
)

func renderHeading(body *etree.Element, b Block, res *profile.Resolved) {
	p := wEl("p")
	pPr := wEl("pPr")
	style := wEl("pStyle")
	setWVal(style, styleIDForHeading(b.Level))
	pPr.AddChild(style)

	h, ok := res.Profile.Heading(b.Level)
	if ok {
		if h.KeepWithNext {
			pPr.AddChild(wEl("keepNext"))
		}
		sp := wEl("spacing")
		sp.CreateAttr("w:before", strconv.Itoa(profile.PtToDxa(h.SpaceBeforePt)))
		sp.CreateAttr("w:after", strconv.Itoa(profile.PtToDxa(h.SpaceAfterPt)))
		sp.CreateAttr("w:line", strconv.Itoa(res.LineSpacingTwips))
		sp.CreateAttr("w:lineRule", "auto")
		pPr.AddChild(sp)
	}
	p.AddChild(pPr)

	font := res.Profile.Typography.HeadingFont
	size := res.BodySizeHalfPts
	bold := true
	if ok {
		size = profile.PtToHalfPoints(h.SizePt)
		bold = h.Bold
	}
	appendRun(p, b.Text, font, size, bold, false, res.Profile.Palette.Primary)
	insertBeforeSect(body, p)
}

func renderParagraph(body *etree.Element, b Block, res *profile.Resolved) {
	p := wEl("p")
	pPr := wEl("pPr")
	sp := wEl("spacing")
	sp.CreateAttr("w:after", strconv.Itoa(res.ParagraphSpaceAfterDxa))
	sp.CreateAttr("w:line", strconv.Itoa(res.LineSpacingTwips))
	sp.CreateAttr("w:lineRule", "auto")
	pPr.AddChild(sp)
	if b.Alignment != "" {
		jc := wEl("jc")
		setWVal(jc, b.Alignment)
		pPr.AddChild(jc)
	}
	p.AddChild(pPr)

	font := res.Profile.Typography.BodyFont
	size := res.BodySizeHalfPts
	color := res.Profile.Palette.BodyText

	if len(b.Runs) > 0 {
		for _, r := range b.Runs {
			c := color
			if r.Color != "" {
				c = r.Color
			}
			appendRun(p, r.Text, font, size, r.Bold, r.Italic, c)
		}
	} else {
		appendRun(p, b.Text, font, size, false, false, color)
	}
	insertBeforeSect(body, p)
}

func renderTable(body *etree.Element, b Block, res *profile.Resolved) {
	cols := len(b.Headers)
	if cols == 0 && len(b.Rows) > 0 {
		cols = len(b.Rows[0])
	}
	if cols == 0 {
		return
	}

	widths := make([]int, cols)
	if len(b.ColWidthsCm) == cols {
		sum := 0
		for i, cm := range b.ColWidthsCm {
			widths[i] = profile.CmToDxa(cm)
			sum += widths[i]
		}
		// scale to content width if needed
		if sum > res.ContentWidthDxa && sum > 0 {
			for i := range widths {
				widths[i] = widths[i] * res.ContentWidthDxa / sum
			}
		}
	} else {
		each := res.ContentWidthDxa / cols
		for i := range widths {
			widths[i] = each
		}
	}

	tbl := wEl("tbl")
	tblPr := wEl("tblPr")
	tblW := wEl("tblW")
	tblW.CreateAttr("w:w", strconv.Itoa(res.ContentWidthDxa))
	tblW.CreateAttr("w:type", "dxa")
	tblPr.AddChild(tblW)
	layout := wEl("tblLayout")
	layout.CreateAttr("w:type", "fixed")
	tblPr.AddChild(layout)
	tbl.AddChild(tblPr)

	grid := wEl("tblGrid")
	for _, w := range widths {
		gc := wEl("gridCol")
		gc.CreateAttr("w:w", strconv.Itoa(w))
		grid.AddChild(gc)
	}
	tbl.AddChild(grid)

	if len(b.Headers) > 0 {
		tbl.AddChild(tableRow(b.Headers, widths, res, true))
	}
	for _, row := range b.Rows {
		cells := make([]string, cols)
		copy(cells, row)
		tbl.AddChild(tableRow(cells, widths, res, false))
	}
	insertBeforeSect(body, tbl)
}

func tableRow(cells []string, widths []int, res *profile.Resolved, header bool) *etree.Element {
	tr := wEl("tr")
	trPr := wEl("trPr")
	trPr.AddChild(wEl("cantSplit"))
	if header {
		trPr.AddChild(wEl("tblHeader"))
	}
	tr.AddChild(trPr)

	for i, text := range cells {
		w := widths[0]
		if i < len(widths) {
			w = widths[i]
		}
		tc := wEl("tc")
		tcPr := wEl("tcPr")
		tcW := wEl("tcW")
		tcW.CreateAttr("w:w", strconv.Itoa(w))
		tcW.CreateAttr("w:type", "dxa")
		tcPr.AddChild(tcW)
		tcPr.AddChild(tcBordersThin(res.Profile.Palette.Secondary))
		if header {
			tcPr.AddChild(shadeEl(res.Profile.Palette.MutedBg))
		}
		tcMar := wEl("tcMar")
		for _, side := range []string{"top", "left", "bottom", "right"} {
			m := wEl(side)
			m.CreateAttr("w:w", "60")
			m.CreateAttr("w:type", "dxa")
			tcMar.AddChild(m)
		}
		tcPr.AddChild(tcMar)
		tc.AddChild(tcPr)

		p := wEl("p")
		bold := header
		appendRun(p, text, res.Profile.Typography.BodyFont, res.BodySizeHalfPts, bold, false, res.Profile.Palette.BodyText)
		tc.AddChild(p)
		tr.AddChild(tc)
	}
	return tr
}

func renderCallout(body *etree.Element, b Block, res *profile.Resolved) {
	width := res.ContentWidthDxa
	tbl := wEl("tbl")
	tblPr := wEl("tblPr")
	tblW := wEl("tblW")
	tblW.CreateAttr("w:w", strconv.Itoa(width))
	tblW.CreateAttr("w:type", "dxa")
	tblPr.AddChild(tblW)
	tbl.AddChild(tblPr)

	grid := wEl("tblGrid")
	gc := wEl("gridCol")
	gc.CreateAttr("w:w", strconv.Itoa(width))
	grid.AddChild(gc)
	tbl.AddChild(grid)

	tr := wEl("tr")
	trPr := wEl("trPr")
	trPr.AddChild(wEl("cantSplit"))
	tr.AddChild(trPr)

	tc := wEl("tc")
	tcPr := wEl("tcPr")
	tcW := wEl("tcW")
	tcW.CreateAttr("w:w", strconv.Itoa(width))
	tcW.CreateAttr("w:type", "dxa")
	tcPr.AddChild(tcW)

	borders := wEl("tcBorders")
	primary := profile.HexColor(res.Profile.Palette.Primary)
	borders.AddChild(borderEl("left", "single", "24", "0", primary)) // ~3pt
	borders.AddChild(borderEl("top", "nil", "0", "0", "auto"))
	borders.AddChild(borderEl("bottom", "nil", "0", "0", "auto"))
	borders.AddChild(borderEl("right", "nil", "0", "0", "auto"))
	tcPr.AddChild(borders)
	tcPr.AddChild(shadeEl(res.Profile.Palette.MutedBg))
	tc.AddChild(tcPr)

	if b.Title != "" {
		pTitle := wEl("p")
		appendRun(pTitle, b.Title, res.Profile.Typography.BodyFont, res.BodySizeHalfPts, true, false, res.Profile.Palette.Primary)
		tc.AddChild(pTitle)
	}
	p := wEl("p")
	appendRun(p, b.Text, res.Profile.Typography.BodyFont, res.BodySizeHalfPts, false, true, res.Profile.Palette.BodyText)
	tc.AddChild(p)

	tr.AddChild(tc)
	tbl.AddChild(tr)
	insertBeforeSect(body, tbl)
}

func renderList(body *etree.Element, b Block, res *profile.Resolved) {
	for i, item := range b.Items {
		p := wEl("p")
		pPr := wEl("pPr")
		ind := wEl("ind")
		ind.CreateAttr("w:left", "720")
		ind.CreateAttr("w:hanging", "360")
		pPr.AddChild(ind)
		p.AddChild(pPr)

		prefix := "• "
		if b.ListType == "numbered" {
			prefix = strconv.Itoa(i+1) + ". "
		}
		appendRun(p, prefix+item, res.Profile.Typography.BodyFont, res.BodySizeHalfPts, false, false, res.Profile.Palette.BodyText)
		insertBeforeSect(body, p)
	}
}

func renderPageBreak(body *etree.Element) {
	p := wEl("p")
	r := wEl("r")
	br := wEl("br")
	br.CreateAttr("w:type", "page")
	r.AddChild(br)
	p.AddChild(r)
	insertBeforeSect(body, p)
}
