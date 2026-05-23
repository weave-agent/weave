package launcher

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createExtension(t *testing.T, dir, name, content string) {
	t.Helper()

	extDir := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, name+".go"), []byte(content), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "go.mod"), []byte("module test/ext/"+name+"\n\ngo 1.22\n"), 0o600))
}

func TestAutoDiscover_LocalExtension(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	createExtension(t, filepath.Join(projectDir, ".weave", "extensions"), "noop", "package noop")

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err, "AutoDiscover")

	require.Len(t, exts, 1)
	assert.Equal(t, "noop", exts[0].Name)
	assert.False(t, exts[0].IsUIExt)
}

func TestAutoDiscover_GlobalExtension(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	createExtension(t, filepath.Join(homeDir, ".weave", "extensions"), "logging", "package logging")

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err, "AutoDiscover")

	require.Len(t, exts, 1)
	assert.Equal(t, "logging", exts[0].Name)
}

func TestAutoDiscover_BuiltinExtension(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	createExtension(t, filepath.Join(moduleRoot, "extensions"), "builtin", "package builtin")

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err, "AutoDiscover")

	require.Len(t, exts, 1)
	assert.Equal(t, "builtin", exts[0].Name)
}

func TestAutoDiscover_LocalPreferredOverGlobal(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	createExtension(t, filepath.Join(projectDir, ".weave", "extensions"), "noop", "package noop")
	createExtension(t, filepath.Join(homeDir, ".weave", "extensions"), "noop", "package noop")

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err)

	require.Len(t, exts, 1)
	assert.Contains(t, exts[0].Dir, filepath.Join(projectDir, ".weave", "extensions"))
}

func TestAutoDiscover_GlobalPreferredOverBuiltin(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	createExtension(t, filepath.Join(homeDir, ".weave", "extensions"), "noop", "package noop")
	createExtension(t, filepath.Join(moduleRoot, "extensions"), "noop", "package noop")

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err)

	require.Len(t, exts, 1)
	assert.Contains(t, exts[0].Dir, filepath.Join(homeDir, ".weave", "extensions"))
}

func TestAutoDiscover_MultipleExtensions(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	createExtension(t, filepath.Join(projectDir, ".weave", "extensions"), "alpha", "package alpha")
	createExtension(t, filepath.Join(projectDir, ".weave", "extensions"), "beta", "package beta")

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err, "AutoDiscover")

	require.Len(t, exts, 2)
	names := []string{exts[0].Name, exts[1].Name}
	assert.Contains(t, names, "alpha")
	assert.Contains(t, names, "beta")
}

func TestAutoDiscover_EmptyExtensionDir(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	// Create an empty extensions dir (no go.mod, no .go files)
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".weave", "extensions", "empty"), 0o750))

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err)
	assert.Empty(t, exts)
}

func TestAutoDiscover_GoFilesSorted(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	extDir := filepath.Join(projectDir, ".weave", "extensions", "sorted")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "go.mod"), []byte("module test/ext/sorted\n\ngo 1.22\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "z.go"), []byte("package sorted"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "a.go"), []byte("package sorted"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "m.go"), []byte("package sorted"), 0o600))

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err, "AutoDiscover")

	require.Len(t, exts, 1)

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

func TestAutoDiscover_NestedExtension(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	// Create a nested extension at moduleRoot/extensions/tools/bash/
	createExtension(t, filepath.Join(moduleRoot, "extensions", "tools"), "bash", "package bash")

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err, "AutoDiscover")

	require.Len(t, exts, 1)
	assert.Equal(t, "bash", exts[0].Name)
	assert.Contains(t, exts[0].Dir, filepath.Join("extensions", "tools", "bash"))
}

func TestAutoDiscover_TUIExtension(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	// TUI extension at extensions/ui/tui/extensions/tui-diffview/
	extDir := filepath.Join(moduleRoot, "extensions", "ui", "tui", "extensions", "tui-diffview")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "go.mod"), []byte("module test/ext/tui-diffview\n\ngo 1.22\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "diff.go"), []byte("package diffviewer\n\nimport \"github.com/weave-agent/weave/sdk\"\n\nfunc init() { sdk.RegisterUIExtension(\"diff\", nil) }\n"), 0o600))

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err, "AutoDiscover")

	var diffExt *ExtensionInfo

	for i := range exts {
		if exts[i].Name == "tui-diffview" {
			diffExt = &exts[i]
			break
		}
	}

	require.NotNil(t, diffExt, "tui-diffview should be discovered")
	assert.True(t, diffExt.IsUIExt, "tui-diffview should be detected as UI extension")
}

func TestAutoDiscover_ExcludeList(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	createExtension(t, filepath.Join(projectDir, ".weave", "extensions"), "keep", "package keep")
	createExtension(t, filepath.Join(projectDir, ".weave", "extensions"), "exclude", "package exclude")

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, []string{"exclude"})
	require.NoError(t, err)

	require.Len(t, exts, 1)
	assert.Equal(t, "keep", exts[0].Name)
}

func TestAutoDiscover_SkipsObsoleteCoreExtension(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	createExtension(t, filepath.Join(homeDir, ".weave", "extensions"), "tui-sandbox", "package tuisandbox")
	createExtension(t, filepath.Join(homeDir, ".weave", "extensions"), "tui-guardian", "package tuiguardian")

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err)

	require.Len(t, exts, 1)
	assert.Equal(t, "tui-guardian", exts[0].Name)
}

func TestAutoDiscover_SkipsInvalidName(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	extBase := filepath.Join(projectDir, ".weave", "extensions")
	createExtension(t, extBase, "bad name", "package bad")

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err)
	assert.Empty(t, exts, "extension with spaces in name should be skipped")
}

func TestAutoDiscover_NoExtensionsFound(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err)
	assert.Empty(t, exts)
}

func TestAutoDiscover_ModuleBoundaryRespected(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	// Parent extension
	parentDir := filepath.Join(moduleRoot, "extensions", "parent")
	require.NoError(t, os.MkdirAll(parentDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(parentDir, "go.mod"), []byte("module test/ext/parent\n\ngo 1.22\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(parentDir, "parent.go"), []byte("package parent"), 0o600))

	// Nested extension with its own go.mod
	childDir := filepath.Join(parentDir, "child")
	require.NoError(t, os.MkdirAll(childDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(childDir, "go.mod"), []byte("module test/ext/child\n\ngo 1.22\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(childDir, "child.go"), []byte("package child"), 0o600))

	exts, err := AutoDiscover(projectDir, homeDir, moduleRoot, nil)
	require.NoError(t, err)

	// Should find both parent and child as separate extensions
	extMap := make(map[string]ExtensionInfo)
	for _, e := range exts {
		extMap[e.Name] = e
	}

	require.Contains(t, extMap, "parent")
	require.Contains(t, extMap, "child")

	// Parent's GoFiles should NOT include child's .go files
	parentGoFiles := extMap["parent"].GoFiles
	for _, f := range parentGoFiles {
		assert.NotContains(t, f, "child", "parent should not include child's Go files")
	}
}

func TestCollectGoFiles_RespectsModuleBoundaries(t *testing.T) {
	dir := t.TempDir()

	// Main module files
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o600))

	// Subdir with its own go.mod (module boundary)
	subDir := filepath.Join(dir, "submod")
	require.NoError(t, os.MkdirAll(subDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "go.mod"), []byte("module submod\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "sub.go"), []byte("package submod"), 0o600))

	files, err := collectGoFiles(dir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Contains(t, files[0], "main.go")
}
