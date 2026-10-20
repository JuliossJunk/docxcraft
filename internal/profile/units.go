package profile

import "fmt"

// Twips (dxa): 1 inch = 1440 dxa, 1 pt = 20 dxa, 1 cm ≈ 567 dxa.
const (
	DxaPerInch = 1440
	DxaPerPt   = 20
	// DxaPerCm is twips per centimeter (1440 / 2.54).
	DxaPerCm = 567
	// DxaPerMm is twips per millimeter.
	DxaPerMm = 56.7
)

// CmToDxa converts centimeters to twips (dxa), rounded to nearest int.
func CmToDxa(cm float64) int {
	return int(cm*DxaPerCm + 0.5)
}

// MmToDxa converts millimeters to twips.
func MmToDxa(mm float64) int {
	return int(mm*DxaPerMm + 0.5)
}

// InToDxa converts inches to twips.
func InToDxa(inches float64) int {
	return int(inches*DxaPerInch + 0.5)
}

// PtToDxa converts points to twips.
func PtToDxa(pt float64) int {
	return int(pt*DxaPerPt + 0.5)
}

// PtToHalfPoints converts font size in points to OpenXML half-points (sz).
func PtToHalfPoints(pt float64) int {
	return int(pt*2 + 0.5)
}

// LineSpacingToTwips converts a multiplier (e.g. 1.15) to w:spacing/@w:line
// with w:lineRule="auto" (240 = single spacing).
func LineSpacingToTwips(multiplier float64) int {
	return int(multiplier*240 + 0.5)
}

// PageSizeDxa returns width and height in dxa for named page sizes.
func PageSizeDxa(size string) (w, h int, err error) {
	switch size {
	case "A4", "a4", "":
		return 11906, 16838, nil
	case "Letter", "letter":
		return 12240, 15840, nil
	default:
		return 0, 0, fmt.Errorf("profile: unknown page size %q", size)
	}
}

// HexColor strips a leading '#' from a CSS-like hex color for w:color/@w:val.
func HexColor(s string) string {
	if len(s) > 0 && s[0] == '#' {
		return s[1:]
	}
	return s
}
