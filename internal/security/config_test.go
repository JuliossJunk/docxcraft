package security

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecuritySandboxEscape(t *testing.T) {
	dir := t.TempDir()
	c := &Config{SandboxDir: dir, MaxSize: DefaultMaxSize}
	_, err := c.ResolvePath("../outside.docx")
	require.Error(t, err)
	require.Contains(t, err.Error(), "escapes sandbox")

	inside := filepath.Join(dir, "ok.docx")
	abs, err := c.ResolvePath("ok.docx")
	require.NoError(t, err)
	require.Equal(t, inside, abs)
}

func TestSecurityReadonlyBlocksWrite(t *testing.T) {
	c := &Config{ReadOnly: true}
	require.Error(t, c.AssertWritable())
	c.ReadOnly = false
	require.NoError(t, c.AssertWritable())
}

func TestWhitelistFiltersTools(t *testing.T) {
	c := &Config{Whitelist: ParseWhitelist("inspect,lint")}
	require.True(t, c.AllowTool("inspect"))
	require.True(t, c.AllowTool("lint"))
	require.False(t, c.AllowTool("patch"))
	require.Error(t, c.RequireTool("patch"))
}

func TestAssertFileSize(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.docx")
	require.NoError(t, os.WriteFile(p, []byte("hello"), 0o644))
	c := &Config{MaxSize: 2}
	require.Error(t, c.AssertFileSize(p))
	c.MaxSize = 100
	require.NoError(t, c.AssertFileSize(p))
}
