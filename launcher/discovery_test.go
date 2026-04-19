package launcher

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createGoFile(t *testing.T, dir, name, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
}

func TestDiscover_LocalExtension(t *testing.T) {
	projectDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	createGoFile(t, extDir, "noop.go", "package noop")

	exts, err := Discover(projectDir, []string{"noop"})
	require.NoError(t, err, "Discover")

	require.Len(t, exts, 1)
	assert.Equal(t, "noop", exts[0].Name)
	assert.Equal(t, extDir, exts[0].Dir)
	assert.Len(t, exts[0].GoFiles, 1)
}

func TestDiscover_GlobalExtension(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	extDir := filepath.Join(homeDir, ".weave", "extensions", "logging")
	createGoFile(t, extDir, "logging.go", "package logging")

	info, err := findExtension(projectDir, homeDir, "logging")
	require.NoError(t, err, "findExtension")
	assert.Equal(t, "logging", info.Name)
	assert.Equal(t, extDir, info.Dir)
}

func TestDiscover_LocalPreferredOverGlobal(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	localDir := filepath.Join(projectDir, ".weave", "extensions", "noop")
	createGoFile(t, localDir, "noop.go", "package noop")

	globalDir := filepath.Join(homeDir, ".weave", "extensions", "noop")
	createGoFile(t, globalDir, "noop.go", "package noop")

	info, err := findExtension(projectDir, homeDir, "noop")
	require.NoError(t, err)
	assert.Equal(t, localDir, info.Dir)
}

func TestDiscover_MissingExtension(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	_, err := findExtension(projectDir, homeDir, "nonexistent")
	require.Error(t, err)
}

func TestDiscover_MultipleExtensions(t *testing.T) {
	projectDir := t.TempDir()
	for _, name := range []string{"noop", "logging"} {
		extDir := filepath.Join(projectDir, ".weave", "extensions", name)
		createGoFile(t, extDir, name+".go", "package "+name)
	}

	exts, err := Discover(projectDir, []string{"noop", "logging"})
	require.NoError(t, err, "Discover")

	require.Len(t, exts, 2)
	assert.Equal(t, "noop", exts[0].Name)
	assert.Equal(t, "logging", exts[1].Name)
}

func TestDiscover_EmptyExtensionDir(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	extDir := filepath.Join(projectDir, ".weave", "extensions", "empty")
	require.NoError(t, os.MkdirAll(extDir, 0o750))

	_, err := findExtension(projectDir, homeDir, "empty")
	require.Error(t, err)
}

func TestDiscover_GoFilesSorted(t *testing.T) {
	projectDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "sorted")
	createGoFile(t, extDir, "z.go", "package sorted")
	createGoFile(t, extDir, "a.go", "package sorted")
	createGoFile(t, extDir, "m.go", "package sorted")

	exts, err := Discover(projectDir, []string{"sorted"})
	require.NoError(t, err, "Discover")

	expected := []string{
		filepath.Join(extDir, "a.go"),
		filepath.Join(extDir, "m.go"),
		filepath.Join(extDir, "z.go"),
	}
	require.Len(t, exts[0].GoFiles, len(expected))

	for i, f := range exts[0].GoFiles {
		assert.Equal(t, expected[i], f)
	}
}

func TestDiscover_PartialMissing(t *testing.T) {
	projectDir := t.TempDir()
	extDir := filepath.Join(projectDir, ".weave", "extensions", "exists")
	createGoFile(t, extDir, "exists.go", "package exists")

	_, err := Discover(projectDir, []string{"exists", "missing"})
	require.Error(t, err)
}
