package linter

import (
	"os/exec"
	"time"
)

// runVisualGate attempts to locate LibreOffice; v1 only reports skip/ok discovery.
func runVisualGate(workDir string, timeout time.Duration) VisualResult {
	_ = workDir
	_ = timeout
	if _, err := exec.LookPath("soffice"); err != nil {
		if _, err2 := exec.LookPath("libreoffice"); err2 != nil {
			return VisualResult{
				Status:   "skipped",
				Reason:   "libreoffice_not_found",
				PNGPaths: []string{},
			}
		}
	}
	// Full conversion deferred; presence of LO is enough to mark skipped with reason pending.
	return VisualResult{
		Status:   "skipped",
		Reason:   "conversion_not_implemented",
		PNGPaths: []string{},
	}
}
