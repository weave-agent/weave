package launcher

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsUIExtension_DetectsRegisterUIExtension(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package x\n\nfunc init() { RegisterUIExtension(\"x\", nil) }\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.go"), []byte("package x\n"), 0o600))

	assert.True(t, isUIExtension(dir))
}

func TestIsUIExtension_DetectsRegisterUI(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "ui.go"), []byte("package x\n\nfunc init() { RegisterUI(\"tui\", nil) }\n"), 0o600))

	assert.True(t, isUIExtension(dir))
}

func TestIsUIExtension_NoUIRegistration(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package x\n\nfunc init() { RegisterExtension(\"x\", nil) }\n"), 0o600))

	assert.False(t, isUIExtension(dir))
}

func TestIsUIExtension_SkipsTestFiles(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a_test.go"), []byte("package x\n\nfunc init() { RegisterUIExtension(\"x\", nil) }\n"), 0o600))

	assert.False(t, isUIExtension(dir))
}

func TestIsUIExtension_RespectsModuleBoundaries(t *testing.T) {
	dir := t.TempDir()

	// Parent module
	require.NoError(t, os.WriteFile(filepath.Join(dir, "parent.go"), []byte("package parent\n"), 0o600))

	// Child module with UI registration — should be skipped
	childDir := filepath.Join(dir, "child")
	require.NoError(t, os.MkdirAll(childDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(childDir, "go.mod"), []byte("module child\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(childDir, "child.go"), []byte("package child\n\nfunc init() { RegisterUIExtension(\"x\", nil) }\n"), 0o600))

	assert.False(t, isUIExtension(dir))
}

func TestIsUIExtension_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	assert.False(t, isUIExtension(dir))
}

func TestIsUIExtension_SkipsOversizedFiles(t *testing.T) {
	dir := t.TempDir()

	// Create a file larger than maxUIExtScanSize with UI registration.
	largeContent := make([]byte, maxUIExtScanSize+1)
	copy(largeContent, "package x\n\nfunc init() { RegisterUIExtension(\"x\", nil) }\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), largeContent, 0o600))

	assert.False(t, isUIExtension(dir))
}
