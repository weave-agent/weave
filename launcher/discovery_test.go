package launcher

import (
	"os"
	"path/filepath"
	"testing"

	"weave/config"

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

	exts, _, err := Discover(projectDir, []string{"noop"})
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

	exts, _, err := Discover(projectDir, []string{"noop", "logging"})
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

	exts, _, err := Discover(projectDir, []string{"sorted"})
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

	_, _, err := Discover(projectDir, []string{"exists", "missing"})
	require.Error(t, err)
}

func TestDiscoverWithBuiltins_NestedExtension(t *testing.T) {
	moduleRoot := t.TempDir()
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Create a nested extension at moduleRoot/extensions/tools/bash/
	nestedDir := filepath.Join(moduleRoot, "extensions", "tools", "bash")
	createGoFile(t, nestedDir, "bash.go", "package bash")

	exts, _, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"bash"})
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

	exts, _, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"anthropic", "openai"})
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

	exts, _, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"myext"})
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

	exts, _, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"loop", "bash", "anthropic"})
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

	_, _, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"bash"})
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

	exts, _, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"bash"})
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

	exts, _, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"diff-viewer"})
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

	exts, _, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"my-ui-ext"})
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
	_, _, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"nonexistent-ui-ext"})
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

	exts, _, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"bash"})
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

	exts, _, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"mytool"})
	require.NoError(t, err)

	require.Len(t, exts, 1)
	assert.Equal(t, standardDir, exts[0].Dir, "standard nested path should be found before TUI-specific fallback")
}

func TestIsPath(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"./foo", true},
		{"./my-ext", true},
		{"./", true},
		{"../foo", true},
		{"../shared/ext", true},
		{"../", true},
		{"/abs/path", true},
		{"/", true},
		{"~/my-ext", true},
		{"~/", true},
		{"bash", false},
		{"my-extension", false},
		{"loop", false},
		{"", false},
		{"foo/bar", false},
		{"~", false},
		{".", false},
		{"..", false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, isPath(tt.input), "isPath(%q)", tt.input)
	}
}

func TestResolveExtensionPath_Relative(t *testing.T) {
	dir := "/project"

	abs, err := config.ResolveExtPath("./my-ext", dir)
	require.NoError(t, err)
	assert.Equal(t, "/project/my-ext", abs)

	abs, err = config.ResolveExtPath("../shared/ext", dir)
	require.NoError(t, err)
	assert.Equal(t, "/shared/ext", abs)
}

func TestResolveExtensionPath_Absolute(t *testing.T) {
	abs, err := config.ResolveExtPath("/opt/weave/extensions/my-ext", "/irrelevant")
	require.NoError(t, err)
	assert.Equal(t, "/opt/weave/extensions/my-ext", abs)
}

func TestResolveExtensionPath_Tilde(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	abs, err := config.ResolveExtPath(filepath.Join("~", "dev", "my-ext"), "/irrelevant")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "dev", "my-ext"), abs)
}

func TestResolveExtensionPath_BareNameErrors(t *testing.T) {
	// Bare names should not be passed to ResolveExtPath (they use isPath first),
	// but if they are, the result is still a valid path relative to configDir.
	abs, err := config.ResolveExtPath("bash", "/project")
	require.NoError(t, err)
	assert.Equal(t, "/project/bash", abs)
}

func TestDiscoverWithBuiltins_PathEntryRelative(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	// Create extension at ../my-ext relative to projectDir
	parentDir := filepath.Dir(projectDir)
	extDir := filepath.Join(parentDir, "my-ext")
	createGoFile(t, extDir, "ext.go", "package myext")

	exts, _, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"../my-ext"})
	require.NoError(t, err)

	require.Len(t, exts, 1)
	assert.Equal(t, "my-ext", exts[0].Name)
	assert.Equal(t, extDir, exts[0].Dir)
}

func TestDiscoverWithBuiltins_PathEntryAbsolute(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	extDir := filepath.Join(t.TempDir(), "my-ext")
	createGoFile(t, extDir, "ext.go", "package myext")

	exts, _, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{extDir})
	require.NoError(t, err)

	require.Len(t, exts, 1)
	assert.Equal(t, "my-ext", exts[0].Name)
	assert.Equal(t, extDir, exts[0].Dir)
}

func TestDiscoverWithBuiltins_PathEntryTilde(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	// Override HOME so ~ resolves to our temp dir
	t.Setenv("HOME", homeDir)

	// Create extension in fake home
	extDir := filepath.Join(homeDir, ".weave-test-ext-tmp")
	createGoFile(t, extDir, "ext.go", "package ext")

	exts, _, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"~/.weave-test-ext-tmp"})
	require.NoError(t, err)

	require.Len(t, exts, 1)
	assert.Equal(t, ".weave-test-ext-tmp", exts[0].Name)
	assert.Equal(t, extDir, exts[0].Dir)
}

func TestDiscoverWithBuiltins_MixedPathAndBareNames(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	// Bare name "bash" as built-in
	bashDir := filepath.Join(moduleRoot, "extensions", "tools", "bash")
	createGoFile(t, bashDir, "bash.go", "package bash")

	// Path entry relative to projectDir
	extDir := filepath.Join(projectDir, "my-custom")
	createGoFile(t, extDir, "custom.go", "package custom")

	exts, _, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"bash", "./my-custom"})
	require.NoError(t, err)

	require.Len(t, exts, 2)
	assert.Equal(t, "bash", exts[0].Name)
	assert.Equal(t, bashDir, exts[0].Dir)
	assert.Equal(t, "my-custom", exts[1].Name)
	assert.Equal(t, extDir, exts[1].Dir)
}

func TestDiscoverWithBuiltins_PathEntryNotExist(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	_, _, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"./nonexistent"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestDiscoverWithBuiltins_PathEntryNotDir(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	// Create a file (not directory) at the path
	filePath := filepath.Join(projectDir, "notadir.go")
	require.NoError(t, os.WriteFile(filePath, []byte("package main"), 0o600))

	_, _, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"./notadir.go"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

func TestDiscoverWithBuiltins_PathEntryNoGoFiles(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	// Create an empty directory
	emptyDir := filepath.Join(projectDir, "empty")
	require.NoError(t, os.MkdirAll(emptyDir, 0o750))

	_, _, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"./empty"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no .go files")
}

func TestDiscoverWithBuiltins_PathEntryNameIsBaseDir(t *testing.T) {
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	moduleRoot := t.TempDir()

	extDir := filepath.Join(projectDir, "my-special-tool")
	createGoFile(t, extDir, "tool.go", "package tool")

	exts, _, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"./my-special-tool"})
	require.NoError(t, err)

	require.Len(t, exts, 1)
	assert.Equal(t, "my-special-tool", exts[0].Name)
}

func TestCollisionDetection_LocalShadowsBuiltin(t *testing.T) {
	moduleRoot := t.TempDir()
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Create a built-in
	bashDir := filepath.Join(moduleRoot, "extensions", "tools", "bash")
	createGoFile(t, bashDir, "bash.go", "package bash")

	// Create a local override
	localDir := filepath.Join(projectDir, ".weave", "extensions", "bash")
	createGoFile(t, localDir, "bash.go", "package bash")

	exts, warnings, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"bash"})
	require.NoError(t, err)
	require.Len(t, exts, 1)
	assert.Equal(t, localDir, exts[0].Dir)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "bash")
	assert.Contains(t, warnings[0], "built-in")
}

func TestCollisionDetection_GlobalShadowsBuiltin(t *testing.T) {
	moduleRoot := t.TempDir()
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Create a built-in
	bashDir := filepath.Join(moduleRoot, "extensions", "tools", "bash")
	createGoFile(t, bashDir, "bash.go", "package bash")

	// Create a global override
	globalDir := filepath.Join(homeDir, ".weave", "extensions", "bash")
	createGoFile(t, globalDir, "bash.go", "package bash")

	exts, warnings, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"bash"})
	require.NoError(t, err)
	require.Len(t, exts, 1)
	assert.Equal(t, globalDir, exts[0].Dir)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "bash")
	assert.Contains(t, warnings[0], "built-in")
}

func TestCollisionDetection_NoShadow_NoWarning(t *testing.T) {
	moduleRoot := t.TempDir()
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Only a built-in exists, resolved from built-in path
	bashDir := filepath.Join(moduleRoot, "extensions", "tools", "bash")
	createGoFile(t, bashDir, "bash.go", "package bash")

	exts, warnings, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"bash"})
	require.NoError(t, err)
	require.Len(t, exts, 1)
	assert.Empty(t, warnings, "no shadow warning when resolved from built-in")
}

func TestCollisionDetection_PathEntry_NoWarning(t *testing.T) {
	moduleRoot := t.TempDir()
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Built-in exists
	bashDir := filepath.Join(moduleRoot, "extensions", "tools", "bash")
	createGoFile(t, bashDir, "bash.go", "package bash")

	// Path entry — should not generate shadow warning (path entries are explicit)
	extDir := filepath.Join(projectDir, "bash")
	createGoFile(t, extDir, "bash.go", "package bash")

	exts, warnings, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"./bash"})
	require.NoError(t, err)
	require.Len(t, exts, 1)
	assert.Empty(t, warnings, "path entries should not generate shadow warnings")
}

func TestCollisionDetection_MultipleWarnings(t *testing.T) {
	moduleRoot := t.TempDir()
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Create built-ins
	bashBuiltin := filepath.Join(moduleRoot, "extensions", "tools", "bash")
	createGoFile(t, bashBuiltin, "bash.go", "package bash")

	readBuiltin := filepath.Join(moduleRoot, "extensions", "tools", "read")
	createGoFile(t, readBuiltin, "read.go", "package read")

	// Local overrides
	bashLocal := filepath.Join(projectDir, ".weave", "extensions", "bash")
	createGoFile(t, bashLocal, "bash.go", "package bash")

	readLocal := filepath.Join(projectDir, ".weave", "extensions", "read")
	createGoFile(t, readLocal, "read.go", "package read")

	exts, warnings, err := DiscoverCustomHomeWithBuiltins(projectDir, homeDir, moduleRoot, []string{"bash", "read"})
	require.NoError(t, err)
	require.Len(t, exts, 2)
	require.Len(t, warnings, 2)
}

func TestCheckBuiltinShadow(t *testing.T) {
	moduleRoot := t.TempDir()

	// Create a built-in extension
	bashDir := filepath.Join(moduleRoot, "extensions", "tools", "bash")
	createGoFile(t, bashDir, "bash.go", "package bash")

	t.Run("local shadows builtin", func(t *testing.T) {
		w := checkBuiltinShadow(moduleRoot, "bash", "/some/local/path")
		assert.NotEmpty(t, w)
		assert.Contains(t, w, "bash")
	})

	t.Run("resolved from builtin — no shadow", func(t *testing.T) {
		w := checkBuiltinShadow(moduleRoot, "bash", bashDir)
		assert.Empty(t, w)
	})

	t.Run("no builtin exists — no shadow", func(t *testing.T) {
		w := checkBuiltinShadow(moduleRoot, "nonexistent", "/some/local/path")
		assert.Empty(t, w)
	})
}
