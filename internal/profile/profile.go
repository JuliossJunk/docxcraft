package profile

// StyleProfile is the parsed YAML style profile (user units on the boundary).
type StyleProfile struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description" json:"description"`
	Page        PageSpec          `yaml:"page" json:"page"`
	Palette     PaletteSpec       `yaml:"palette" json:"palette"`
	Typography  TypographySpec    `yaml:"typography" json:"typography"`
	Headings    map[string]HeadingSpec `yaml:"headings" json:"headings"`
}

// PageSpec describes page geometry in centimeters at the YAML boundary.
type PageSpec struct {
	Size    string      `yaml:"size" json:"size"`
	Margins MarginSpec  `yaml:"margins" json:"margins"`
}

// MarginSpec holds margins in cm.
type MarginSpec struct {
	TopCm    float64 `yaml:"top_cm" json:"top_cm"`
	BottomCm float64 `yaml:"bottom_cm" json:"bottom_cm"`
	LeftCm   float64 `yaml:"left_cm" json:"left_cm"`
	RightCm  float64 `yaml:"right_cm" json:"right_cm"`
}

// PaletteSpec holds hex colors.
type PaletteSpec struct {
	Primary   string `yaml:"primary" json:"primary"`
	Secondary string `yaml:"secondary" json:"secondary"`
	BodyText  string `yaml:"body_text" json:"body_text"`
	MutedBg   string `yaml:"muted_bg" json:"muted_bg"`
}

// TypographySpec holds body typography in pt / unitless spacing.
type TypographySpec struct {
	BodyFont              string  `yaml:"body_font" json:"body_font"`
	HeadingFont           string  `yaml:"heading_font" json:"heading_font"`
	BodySizePt            float64 `yaml:"body_size_pt" json:"body_size_pt"`
	LineSpacing           float64 `yaml:"line_spacing" json:"line_spacing"`
	ParagraphSpaceAfterPt float64 `yaml:"paragraph_space_after_pt" json:"paragraph_space_after_pt"`
}

// HeadingSpec describes a single heading level.
type HeadingSpec struct {
	SizePt       float64 `yaml:"size_pt" json:"size_pt"`
	Bold         bool    `yaml:"bold" json:"bold"`
	SpaceBeforePt float64 `yaml:"space_before_pt" json:"space_before_pt"`
	SpaceAfterPt  float64 `yaml:"space_after_pt" json:"space_after_pt"`
	KeepWithNext bool    `yaml:"keep_with_next" json:"keep_with_next"`
}

// Resolved holds geometry already converted to dxa / half-points for oxml use.
type Resolved struct {
	Profile *StyleProfile

	PageW, PageH                   int
	MarginTop, MarginBottom        int
	MarginLeft, MarginRight        int
	BodySizeHalfPts                int
	LineSpacingTwips               int
	ParagraphSpaceAfterDxa         int
	ContentWidthDxa                int
}

// Resolve converts user units in p to internal dxa / half-points.
func Resolve(p *StyleProfile) (*Resolved, error) {
	if p == nil {
		return nil, ErrNilProfile
	}
	w, h, err := PageSizeDxa(p.Page.Size)
	if err != nil {
		return nil, err
	}
	left := CmToDxa(p.Page.Margins.LeftCm)
	right := CmToDxa(p.Page.Margins.RightCm)
	r := &Resolved{
		Profile:                p,
		PageW:                  w,
		PageH:                  h,
		MarginTop:              CmToDxa(p.Page.Margins.TopCm),
		MarginBottom:           CmToDxa(p.Page.Margins.BottomCm),
		MarginLeft:             left,
		MarginRight:            right,
		BodySizeHalfPts:        PtToHalfPoints(p.Typography.BodySizePt),
		LineSpacingTwips:       LineSpacingToTwips(p.Typography.LineSpacing),
		ParagraphSpaceAfterDxa: PtToDxa(p.Typography.ParagraphSpaceAfterPt),
		ContentWidthDxa:        w - left - right,
	}
	return r, nil
}

// Heading returns the heading spec for level 1–3 (keys h1/h2/h3).
func (p *StyleProfile) Heading(level int) (HeadingSpec, bool) {
	if p == nil || p.Headings == nil {
		return HeadingSpec{}, false
	}
	key := ""
	switch level {
	case 1:
		key = "h1"
	case 2:
		key = "h2"
	case 3:
		key = "h3"
	default:
		return HeadingSpec{}, false
	}
	h, ok := p.Headings[key]
	return h, ok
}
