package linter

import "time"

// Options configures an audit run.
type Options struct {
	ProfileName   string
	Strict        bool
	EnableVisual  bool
	VisualTimeout time.Duration
	WorkDir       string
	// InputPath is the on-disk .docx used by Visual Gate. If empty and EnableVisual,
	// Audit writes the package to a temporary file.
	InputPath string
	// KeepVisual keeps WorkDir artifacts; when false, temp dirs created for input
	// staging may still retain visual outputs referenced in the report until the
	// process exits (caller may delete WorkDir / PDFPath parent).
	KeepVisual bool
}

// Issue is a single lint finding.
type Issue struct {
	Code         string `json:"code"`
	Severity     string `json:"severity"`
	AnchorID     string `json:"anchor_id"`
	Location     string `json:"location"`
	Message      string `json:"message"`
	SuggestedFix string `json:"suggested_fix"`
}

// VisualResult is the optional LibreOffice visual gate outcome.
type VisualResult struct {
	Status   string   `json:"status"`
	Reason   string   `json:"reason,omitempty"`
	PDFPath  string   `json:"pdf_path"`
	PNGPaths []string `json:"png_paths"`
}

// Report is the machine-readable lint report for LLM/CI.
type Report struct {
	IsValid     bool         `json:"is_valid"`
	ProfileName string       `json:"profile_name"`
	IssueCount  int          `json:"issue_count"`
	Issues      []Issue      `json:"issues"`
	VisualGate  VisualResult `json:"visual_gate"`
}

func (r *Report) add(issue Issue) {
	r.Issues = append(r.Issues, issue)
	r.IssueCount = len(r.Issues)
}

func (r *Report) finalize(strict bool) {
	r.IssueCount = len(r.Issues)
	r.IsValid = true
	for _, iss := range r.Issues {
		switch iss.Severity {
		case "error", "fatal":
			r.IsValid = false
		case "warning":
			if strict {
				r.IsValid = false
			}
		}
	}
}

// ExitCode maps a report to CLI exit codes (0/1/2).
func ExitCode(r *Report, strict bool) int {
	if r == nil {
		return 2
	}
	hasFatal := false
	hasError := false
	hasWarning := false
	for _, iss := range r.Issues {
		switch iss.Severity {
		case "fatal":
			hasFatal = true
		case "error":
			hasError = true
		case "warning":
			hasWarning = true
		}
	}
	if hasFatal {
		return 2
	}
	if hasError || (strict && hasWarning) {
		return 1
	}
	return 0
}
