package extmanage

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoreExtensionRepo(t *testing.T) {
	assert.Equal(t, "github.com/weave-agent/weave-bash", coreExtensionRepo("bash"))
	assert.Equal(t, "github.com/weave-agent/weave-tui-diffview", coreExtensionRepo("tui-diffview"))
	assert.Equal(t, "github.com/weave-agent/weave-tui-subagent", coreExtensionRepo("tui-subagent"))
}

func TestCoreExtensionNames_ContainsExpected(t *testing.T) {
	expected := []string{
		"bash", "read", "edit", "write", "grep", "find", "ls",
		"search", "webfetch", "subagent", "anthropic", "openai", "zai", "kimi", "codex",
		"agent", "sandbox", "tui-sandbox", "jsonl", "tui",
		"tui-diffview", "tui-subagent",
	}

	for _, name := range expected {
		assert.True(t, slices.Contains(CoreExtensionNames, name),
			"CoreExtensionNames should contain %q", name)
	}

	assert.Len(t, CoreExtensionNames, len(expected),
		"CoreExtensionNames length should match expected")
}

func TestExtensionsDir(t *testing.T) {
	homeDir := "/home/user"
	expected := homeDir + "/.weave/extensions"

	assert.Equal(t, expected, ExtensionsDir(homeDir))
}

func TestNeedsBootstrap_DirNotExist(t *testing.T) {
	homeDir := t.TempDir()

	// Extensions dir does not exist.
	needs, err := NeedsBootstrap(homeDir)
	require.NoError(t, err)
	assert.True(t, needs, "should bootstrap when dir does not exist")
}

func TestNeedsBootstrap_DirEmpty(t *testing.T) {
	homeDir := t.TempDir()

	extDir := ExtensionsDir(homeDir)
	require.NoError(t, os.MkdirAll(extDir, 0o750))

	needs, err := NeedsBootstrap(homeDir)
	require.NoError(t, err)
	assert.True(t, needs, "should bootstrap when extensions dir is empty")
}

func TestNeedsBootstrap_DirHasOnlyHidden(t *testing.T) {
	homeDir := t.TempDir()

	extDir := ExtensionsDir(homeDir)
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, ".staging-foo"), 0o750))

	needs, err := NeedsBootstrap(homeDir)
	require.NoError(t, err)
	assert.True(t, needs, "should bootstrap when extensions dir has only hidden entries")
}

func TestNeedsBootstrap_DirHasExtensions(t *testing.T) {
	homeDir := t.TempDir()

	extDir := ExtensionsDir(homeDir)
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, "bash"), 0o750))

	needs, err := NeedsBootstrap(homeDir)
	require.NoError(t, err)
	assert.False(t, needs, "should not bootstrap when extensions dir has entries")
}

// preCreateAllExtensions creates a minimal extension directory for each core
// extension name in the given homeDir. This avoids network calls in tests.
func preCreateAllExtensions(t *testing.T, homeDir string) {
	t.Helper()

	extDir := ExtensionsDir(homeDir)
	require.NoError(t, os.MkdirAll(extDir, 0o750))

	for _, name := range CoreExtensionNames {
		dir := filepath.Join(extDir, name)
		require.NoError(t, os.MkdirAll(dir, 0o750))

		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
			[]byte("module github.com/weave-agent/weave-"+name+"\n\ngo 1.22\n"), 0o600))

		require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"),
			[]byte("package main\n"), 0o600))
	}
}

func TestBootstrapCoreExtensions_SkipsExisting(t *testing.T) {
	homeDir := t.TempDir()
	preCreateAllExtensions(t, homeDir)

	var messages []string

	result, err := BootstrapCoreExtensions(context.Background(), homeDir, func(msg string) {
		messages = append(messages, msg)
	})
	require.NoError(t, err)

	// All extensions already exist, so nothing should be installed or failed.
	assert.Empty(t, result.Installed, "no extensions should be installed when all exist")
	assert.Empty(t, result.Failed, "no extensions should fail when all exist")

	// Bootstrap emits the header even when everything is already installed,
	// but no per-extension progress messages.
	assert.Len(t, messages, 1, "only the header message when everything is already installed")
	assert.Contains(t, messages[0], "installing core extensions")
}

func TestBootstrapCoreExtensions_SkipsSingleExisting(t *testing.T) {
	homeDir := t.TempDir()
	preCreateAllExtensions(t, homeDir)

	// Remove one extension to test that only that one would be attempted.
	extDir := ExtensionsDir(homeDir)
	require.NoError(t, os.RemoveAll(filepath.Join(extDir, "bash")))

	// Now bootstrap will attempt to install bash, which will try git clone.
	// We can't predict success/failure (depends on network), but we can verify
	// bash is not skipped.
	result, err := BootstrapCoreExtensions(context.Background(), homeDir, nil)
	require.NoError(t, err)

	// bash should appear in either Installed or Failed, not both.
	inInstalled := slices.Contains(result.Installed, "bash")
	inFailed := slices.Contains(result.Failed, "bash")
	assert.True(t, inInstalled || inFailed, "bash should be attempted since it was removed")
	assert.False(t, inInstalled && inFailed, "bash should not be in both installed and failed")
}

func TestBootstrapCoreExtensions_Canceled(t *testing.T) {
	homeDir := t.TempDir()

	extDir := ExtensionsDir(homeDir)
	require.NoError(t, os.MkdirAll(extDir, 0o750))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	result, err := BootstrapCoreExtensions(ctx, homeDir, func(msg string) {})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, result.Installed)
}

func TestBootstrapCoreExtensions_NilOutput(t *testing.T) {
	homeDir := t.TempDir()
	preCreateAllExtensions(t, homeDir)

	// Should not panic with nil output.
	result, err := BootstrapCoreExtensions(context.Background(), homeDir, nil)
	require.NoError(t, err)
	assert.Empty(t, result.Installed, "all pre-created, nothing to install")
	assert.Empty(t, result.Failed)
}

func TestBootstrapCoreExtensions_CreatesExtDir(t *testing.T) {
	homeDir := t.TempDir()

	// Do NOT create the extensions dir — bootstrap should create it.
	extDir := ExtensionsDir(homeDir)

	// Run with canceled context so nothing actually installs.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := BootstrapCoreExtensions(ctx, homeDir, nil)
	require.Error(t, err) // canceled

	// The extensions dir should have been created before the cancellation took effect.
	info, statErr := os.Stat(extDir)
	require.NoError(t, statErr, "extensions dir should be created even before clone attempts")
	assert.True(t, info.IsDir())
}

func TestBootstrapResult_Tracking(t *testing.T) {
	homeDir := t.TempDir()

	extDir := ExtensionsDir(homeDir)
	require.NoError(t, os.MkdirAll(extDir, 0o750))

	// Pre-create all but one extension.
	for _, name := range CoreExtensionNames {
		if name == "bash" {
			continue
		}

		dir := filepath.Join(extDir, name)
		require.NoError(t, os.MkdirAll(dir, 0o750))

		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
			[]byte("module x\n\ngo 1.22\n"), 0o600))

		require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"),
			[]byte("package main\n"), 0o600))
	}

	var messages []string

	result, err := BootstrapCoreExtensions(context.Background(), homeDir, func(msg string) {
		messages = append(messages, msg)
	})
	require.NoError(t, err)

	// Only bash was missing. It may succeed or fail depending on network.
	assert.True(t, slices.Contains(result.Installed, "bash") || slices.Contains(result.Failed, "bash"),
		"bash should be in installed or failed")

	// No other extension should appear in either list.
	for _, name := range result.Installed {
		assert.Equal(t, "bash", name, "only bash should be in installed")
	}

	for _, name := range result.Failed {
		assert.Equal(t, "bash", name, "only bash should be in failed")
	}

	// Verify progress messages include the header.
	assert.NotEmpty(t, messages)
	assert.Contains(t, messages[0], "installing core extensions")

	// Should have a "-> bash" progress line.
	hasBashMsg := false

	for _, msg := range messages {
		if strings.Contains(msg, "->") && strings.Contains(msg, "bash") {
			hasBashMsg = true

			break
		}
	}

	assert.True(t, hasBashMsg, "should have progress message for bash")
}
