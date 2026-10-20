package linter

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// runVisualGate converts docxPath to PDF (+ PNG) via LibreOffice headless.
func runVisualGate(docxPath, workDir string, timeout time.Duration, keepVisual bool) VisualResult {
	_ = keepVisual // artifacts retained in workDir for caller; cleanup is caller's job
	if docxPath == "" {
		return VisualResult{Status: "error", Reason: "empty_input_path", PNGPaths: []string{}}
	}

	bin, err := findLibreOffice()
	if err != nil {
		return VisualResult{Status: "skipped", Reason: "libreoffice_not_found", PNGPaths: []string{}}
	}

	outDir := workDir
	if outDir == "" {
		outDir, err = os.MkdirTemp("", "docxcraft-visual-*")
		if err != nil {
			return VisualResult{Status: "error", Reason: "tmpdir: " + err.Error(), PNGPaths: []string{}}
		}
	} else if err := os.MkdirAll(outDir, 0o755); err != nil {
		return VisualResult{Status: "error", Reason: "workdir: " + err.Error(), PNGPaths: []string{}}
	}

	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	base := strings.TrimSuffix(filepath.Base(docxPath), filepath.Ext(docxPath))
	pdfPath := filepath.Join(outDir, base+".pdf")

	if err := runLOConvert(bin, timeout, "pdf", outDir, docxPath); err != nil {
		if isTimeout(err) {
			return VisualResult{Status: "timeout", Reason: err.Error(), PNGPaths: []string{}}
		}
		return VisualResult{Status: "error", Reason: truncateReason(err.Error()), PNGPaths: []string{}}
	}
	if st, err := os.Stat(pdfPath); err != nil || st.Size() == 0 {
		if alt := firstFileWithExt(outDir, ".pdf"); alt != "" {
			pdfPath = alt
		} else {
			return VisualResult{Status: "error", Reason: "empty_or_missing_pdf", PNGPaths: []string{}}
		}
	}

	pngPaths := []string{}
	if err := runLOConvert(bin, timeout, "png", outDir, pdfPath); err == nil {
		pngPaths = listFilesWithExt(outDir, ".png")
	}

	return VisualResult{
		Status:   "ok",
		PDFPath:  pdfPath,
		PNGPaths: pngPaths,
	}
}

func runLOConvert(bin string, timeout time.Duration, format, outDir, input string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	profileDir := filepath.Join(outDir, ".lo-profile")
	_ = os.MkdirAll(profileDir, 0o755)

	args := []string{
		"-env:UserInstallation=" + fileURL(profileDir),
		"--headless",
		"--norestore",
		"--nolockcheck",
		"--nodefault",
		"--nofirststartwizard",
		"--convert-to", format,
		"--outdir", outDir,
		input,
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	cmd.Env = append(os.Environ(), "SAL_USE_VCLPLUGIN=svp")

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout after %s: %w", timeout, err)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func findLibreOffice() (string, error) {
	if p := os.Getenv("LIBREOFFICE_PATH"); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
		candidate := filepath.Join(p, "soffice.exe")
		if runtime.GOOS != "windows" {
			candidate = filepath.Join(p, "soffice")
		}
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate, nil
		}
	}
	for _, name := range []string{"soffice", "libreoffice"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	for _, p := range typicalLOPaths() {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("libreoffice not found")
}

func typicalLOPaths() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			`C:\Program Files\LibreOffice\program\soffice.exe`,
			`C:\Program Files (x86)\LibreOffice\program\soffice.exe`,
		}
	case "darwin":
		return []string{"/Applications/LibreOffice.app/Contents/MacOS/soffice"}
	default:
		return []string{"/usr/bin/soffice", "/usr/bin/libreoffice", "/snap/bin/libreoffice"}
	}
}

func fileURL(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.ToSlash(abs)
	if runtime.GOOS == "windows" && len(abs) >= 2 && abs[1] == ':' {
		return "file:///" + abs
	}
	return "file://" + abs
}

func firstFileWithExt(dir, ext string) string {
	for _, p := range listFilesWithExt(dir, ext) {
		return p
	}
	return ""
}

func listFilesWithExt(dir, ext string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	ext = strings.ToLower(ext)
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.ToLower(filepath.Ext(e.Name())) == ext {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}

func isTimeout(err error) bool {
	return err != nil && strings.Contains(err.Error(), "timeout")
}

func truncateReason(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 500 {
		return s[:500] + "…"
	}
	return s
}

// LibreOfficeAvailable reports whether a LO binary can be resolved.
func LibreOfficeAvailable() bool {
	_, err := findLibreOffice()
	return err == nil
}
