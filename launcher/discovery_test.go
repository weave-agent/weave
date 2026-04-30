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

func TestDiscoverWithBuiltins_NestedExtension(t *testing.T) {
	moduleRoot := t.TempDir()
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Create a nested extension at moduleRoot/extensions/tools/bash/
	nestedDir := filepath.Join(moduleRoot, "extensions", "tools", "bash")
	createGoFile(t, nestedDir, "bash.go", "package bash")

	exts, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"bash"})
	require.NoError(t, err, "DiscoverCustomHomeWithBuiltins")

	require.Len(t, exts, 1)
	assert.Equal(t, "bash", exts[0].Name)
	assert.Equal(t, nestedDir, exts[0].Dir)
	assert.Len(t, exts[0].GoFiles, 1)
}

func TestDiscoverWithBuiltins_NestedProviders(t *testing.T) {
	moduleRoot := t.TempDir()
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Create nested provider extensions
	anthropicDir := filepath.Join(moduleRoot, "extensions", "providers", "anthropic")
	createGoFile(t, anthropicDir, "anthropic.go", "package anthropic")

	openaiDir := filepath.Join(moduleRoot, "extensions", "providers", "openai")
	createGoFile(t, openaiDir, "openai.go", "package openai")

	exts, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"anthropic", "openai"})
	require.NoError(t, err, "DiscoverCustomHomeWithBuiltins")

	require.Len(t, exts, 2)
	assert.Equal(t, "anthropic", exts[0].Name)
	assert.Equal(t, "openai", exts[1].Name)
}

func TestDiscoverWithBuiltins_DirectPreferredOverNested(t *testing.T) {
	moduleRoot := t.TempDir()
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Create both direct and nested paths for the same name
	directDir := filepath.Join(moduleRoot, "extensions", "myext")
	createGoFile(t, directDir, "myext.go", "package myext")

	nestedDir := filepath.Join(moduleRoot, "extensions", "tools", "myext")
	createGoFile(t, nestedDir, "myext.go", "package myext")

	exts, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"myext"})
	require.NoError(t, err)

	require.Len(t, exts, 1)
	assert.Equal(t, directDir, exts[0].Dir, "direct path should take priority over nested")
}

func TestDiscoverWithBuiltins_MixedNestedAndDirect(t *testing.T) {
	moduleRoot := t.TempDir()
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Direct: extensions/loop/
	loopDir := filepath.Join(moduleRoot, "extensions", "loop")
	createGoFile(t, loopDir, "loop.go", "package loop")

	// Nested: extensions/tools/bash/
	bashDir := filepath.Join(moduleRoot, "extensions", "tools", "bash")
	createGoFile(t, bashDir, "bash.go", "package bash")

	// Nested: extensions/providers/anthropic/
	anthropicDir := filepath.Join(moduleRoot, "extensions", "providers", "anthropic")
	createGoFile(t, anthropicDir, "anthropic.go", "package anthropic")

	exts, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"loop", "bash", "anthropic"})
	require.NoError(t, err)

	require.Len(t, exts, 3)

	extMap := make(map[string]ExtensionInfo, len(exts))
	for _, e := range exts {
		extMap[e.Name] = e
	}

	assert.Equal(t, loopDir, extMap["loop"].Dir)
	assert.Equal(t, bashDir, extMap["bash"].Dir)
	assert.Equal(t, anthropicDir, extMap["anthropic"].Dir)
}

func TestDiscoverWithBuiltins_NestedNotFound(t *testing.T) {
	moduleRoot := t.TempDir()
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Create a tools directory but no "bash" inside it
	require.NoError(t, os.MkdirAll(filepath.Join(moduleRoot, "extensions", "tools"), 0o750))

	_, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"bash"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bash")
}

func TestDiscoverWithBuiltins_LocalOverridesNestedBuiltin(t *testing.T) {
	moduleRoot := t.TempDir()
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Nested builtin
	bashDir := filepath.Join(moduleRoot, "extensions", "tools", "bash")
	createGoFile(t, bashDir, "bash.go", "package bash")

	// Local override
	localDir := filepath.Join(projectDir, ".weave", "extensions", "bash")
	createGoFile(t, localDir, "bash.go", "package bash")

	exts, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"bash"})
	require.NoError(t, err)

	require.Len(t, exts, 1)
	assert.Equal(t, localDir, exts[0].Dir, "local should override nested builtin")
}

func TestDiscoverWithBuiltins_TUIExtension(t *testing.T) {
	moduleRoot := t.TempDir()
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// TUI extension at extensions/ui/tui/extensions/diff-viewer/
	tuiExtDir := filepath.Join(moduleRoot, "extensions", "ui", "tui", "extensions", "diff-viewer")
	createGoFile(t, tuiExtDir, "diff.go", "package diffviewer")

	exts, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"diff-viewer"})
	require.NoError(t, err, "DiscoverCustomHomeWithBuiltins")

	require.Len(t, exts, 1)
	assert.Equal(t, "diff-viewer", exts[0].Name)
	assert.Equal(t, tuiExtDir, exts[0].Dir)
	assert.Len(t, exts[0].GoFiles, 1)
}

func TestDiscoverWithBuiltins_TUIExtensionWithModulePath(t *testing.T) {
	moduleRoot := t.TempDir()
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// TUI extension with its own go.mod
	tuiExtDir := filepath.Join(moduleRoot, "extensions", "ui", "tui", "extensions", "my-ui-ext")
	createGoFile(t, tuiExtDir, "ext.go", "package myuiext")

	goModContent := "module weave/ext/my-ui-ext\n\ngo 1.22\n"
	require.NoError(t, os.WriteFile(filepath.Join(tuiExtDir, "go.mod"), []byte(goModContent), 0o600))

	exts, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"my-ui-ext"})
	require.NoError(t, err, "DiscoverCustomHomeWithBuiltins")

	require.Len(t, exts, 1)
	assert.Equal(t, "my-ui-ext", exts[0].Name)
	assert.Equal(t, tuiExtDir, exts[0].Dir)

	// Verify the module path is read from the extension's go.mod by the builder
	modPath := readModulePath(exts[0].Dir)
	assert.Equal(t, "weave/ext/my-ui-ext", modPath)
}

func TestDiscoverWithBuiltins_TUIExtensionNotFound(t *testing.T) {
	moduleRoot := t.TempDir()
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// No extensions created
	_, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"nonexistent-ui-ext"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent-ui-ext")
}

func TestDiscoverWithBuiltins_StandardExtensionFound(t *testing.T) {
	moduleRoot := t.TempDir()
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Standard nested extension at extensions/tools/bash/
	bashDir := filepath.Join(moduleRoot, "extensions", "tools", "bash")
	createGoFile(t, bashDir, "bash.go", "package bash")

	exts, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"bash"})
	require.NoError(t, err, "DiscoverCustomHomeWithBuiltins")

	require.Len(t, exts, 1)
	assert.Equal(t, "bash", exts[0].Name)
	assert.Equal(t, bashDir, exts[0].Dir)
}

func TestDiscoverWithBuiltins_TUIExtensionFallbackAfterStandard(t *testing.T) {
	moduleRoot := t.TempDir()
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Create both a standard nested extension and a TUI extension with the same name
	// The standard one should be found first (one level deep vs TUI-specific three levels)
	standardDir := filepath.Join(moduleRoot, "extensions", "tools", "mytool")
	createGoFile(t, standardDir, "mytool.go", "package mytool")

	tuiExtDir := filepath.Join(moduleRoot, "extensions", "ui", "tui", "extensions", "mytool")
	createGoFile(t, tuiExtDir, "mytool.go", "package mytool")

	exts, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"mytool"})
	require.NoError(t, err)

	require.Len(t, exts, 1)
	assert.Equal(t, standardDir, exts[0].Dir, "standard nested path should be found before TUI-specific fallback")
}
