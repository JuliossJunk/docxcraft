package security

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultMaxSize int64 = 100 << 20 // 100 MiB

// Config holds enterprise sandbox settings.
type Config struct {
	SandboxDir string
	MaxSize    int64
	ReadOnly   bool
	Whitelist  map[string]struct{} // empty / nil = allow all
}

// NewConfig returns defaults (cwd sandbox, 100MiB, writable, all tools).
func NewConfig() *Config {
	cwd, _ := os.Getwd()
	return &Config{
		SandboxDir: cwd,
		MaxSize:    DefaultMaxSize,
		Whitelist:  nil,
	}
}

// ParseWhitelist builds a whitelist map from a CSV string.
func ParseWhitelist(csv string) map[string]struct{} {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return nil
	}
	m := make(map[string]struct{})
	for _, p := range strings.Split(csv, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			m[p] = struct{}{}
		}
	}
	return m
}

// ResolvePath resolves userPath inside the sandbox.
func (c *Config) ResolvePath(userPath string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("security: nil config")
	}
	sandbox := c.SandboxDir
	if sandbox == "" {
		var err error
		sandbox, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("security: cwd: %w", err)
		}
	}
	sandboxAbs, err := filepath.Abs(sandbox)
	if err != nil {
		return "", fmt.Errorf("security: sandbox abs: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(sandboxAbs); err == nil {
		sandboxAbs = resolved
	}

	candidate := userPath
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(sandboxAbs, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("security: path abs: %w", err)
	}
	// EvalSymlinks on existing paths; for new output files, eval parent.
	if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
		candidate = resolved
	} else {
		parent := filepath.Dir(candidate)
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			candidate = filepath.Join(resolved, filepath.Base(candidate))
		}
	}

	rel, err := filepath.Rel(sandboxAbs, candidate)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("security: path escapes sandbox-dir: %s", userPath)
	}
	return candidate, nil
}

// AllowTool reports whether a CLI/MCP tool name is permitted.
func (c *Config) AllowTool(name string) bool {
	if c == nil || len(c.Whitelist) == 0 {
		return true
	}
	_, ok := c.Whitelist[name]
	return ok
}

// AssertWritable fails when readonly mode is on.
func (c *Config) AssertWritable() error {
	if c != nil && c.ReadOnly {
		return fmt.Errorf("security: readonly mode forbids write operations")
	}
	return nil
}

// AssertFileSize checks on-disk size against MaxSize.
func (c *Config) AssertFileSize(path string) error {
	max := DefaultMaxSize
	if c != nil && c.MaxSize > 0 {
		max = c.MaxSize
	}
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("security: stat: %w", err)
	}
	if st.Size() > max {
		return fmt.Errorf("security: file exceeds max-size (%d > %d bytes)", st.Size(), max)
	}
	return nil
}

// RequireTool returns an error if the tool is not whitelisted.
func (c *Config) RequireTool(name string) error {
	if !c.AllowTool(name) {
		return fmt.Errorf("security: tool %q not in whitelist", name)
	}
	return nil
}
