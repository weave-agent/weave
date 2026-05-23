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
		"agent", "guardian", "sandbox", "jsonl", "tui", "tui-guardian",
		"tui-diffview", "tui-subagent",
	}

	for _, name := range expected {
		assert.True(t, slices.Contains(CoreExtensionNames, name),
			"CoreExtensionNames should contain %q", name)
	}

	assert.Len(t, CoreExtensionNames, len(expected),
		"CoreExtensionNames length should match expected")
	assert.False(t, slices.Contains(CoreExtensionNames, "tui-sandbox"),
		"CoreExtensionNames should not contain removed tui-sandbox extension")
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

func TestNeedsBootstrap_DirMissingCoreExtensionsAfterMigration(t *testing.T) {
	homeDir := t.TempDir()
	preCreateAllExtensions(t, homeDir)

	extDir := ExtensionsDir(homeDir)
	require.NoError(t, os.RemoveAll(filepath.Join(extDir, "bash")))

	needs, err := NeedsBootstrap(homeDir)
	require.NoError(t, err)
	assert.False(t, needs, "should not reinstall an intentionally removed core extension")
}

func TestNeedsBootstrap_MissingMigrationExtensionsWithObsoleteCore(t *testing.T) {
	homeDir := t.TempDir()
	preCreateAllExtensions(t, homeDir)

	extDir := ExtensionsDir(homeDir)
	require.NoError(t, os.RemoveAll(filepath.Join(extDir, "guardian")))
	require.NoError(t, os.RemoveAll(filepath.Join(extDir, "tui-guardian")))
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, "tui-sandbox"), 0o750))

	needs, err := NeedsBootstrap(homeDir)
	require.NoError(t, err)
	assert.True(t, needs, "should bootstrap when the guardian/sandbox migration has not completed")
}

func TestNeedsBootstrap_MigrationMarkerRetriesAfterFailure(t *testing.T) {
	homeDir := t.TempDir()
	preCreateAllExtensions(t, homeDir)

	extDir := ExtensionsDir(homeDir)
	require.NoError(t, os.RemoveAll(filepath.Join(extDir, "guardian")))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, guardianSandboxMigrationMarker), []byte("in-progress\n"), 0o600))

	needs, err := NeedsBootstrap(homeDir)
	require.NoError(t, err)
	assert.True(t, needs, "should retry a failed guardian/sandbox migration")
}

func TestNeedsBootstrap_DirHasAllCoreExtensions(t *testing.T) {
	homeDir := t.TempDir()
	preCreateAllExtensions(t, homeDir)

	needs, err := NeedsBootstrap(homeDir)
	require.NoError(t, err)
	assert.False(t, needs, "should not bootstrap when all core extensions exist")
}

func TestNeedsBootstrap_StaleSandbox(t *testing.T) {
	homeDir := t.TempDir()
	preCreateAllExtensions(t, homeDir)
	require.NoError(t, os.WriteFile(
		filepath.Join(ExtensionsDir(homeDir), "sandbox", "main.go"),
		[]byte("package main\n\nvar _ = SandboxMode(\"\")\n"),
		0o600,
	))

	needs, err := NeedsBootstrap(homeDir)
	require.NoError(t, err)
	assert.True(t, needs, "should bootstrap when sandbox uses removed SDK contract")
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

func prependFailingGit(t *testing.T) {
	t.Helper()

	binDir := t.TempDir()
	gitPath := filepath.Join(binDir, "git")
	require.NoError(t, os.WriteFile(gitPath, []byte("#!/bin/sh\nexit 1\n"), 0o700))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
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

func TestBootstrapCoreExtensions_RemovesObsoleteCoreExtensions(t *testing.T) {
	homeDir := t.TempDir()
	preCreateAllExtensions(t, homeDir)
	obsoleteDir := filepath.Join(ExtensionsDir(homeDir), "tui-sandbox")
	require.NoError(t, os.MkdirAll(obsoleteDir, 0o750))

	result, err := BootstrapCoreExtensions(context.Background(), homeDir, nil)
	require.NoError(t, err)
	assert.Empty(t, result.Installed)
	assert.Empty(t, result.Failed)

	_, statErr := os.Stat(obsoleteDir)
	assert.True(t, os.IsNotExist(statErr), "obsolete tui-sandbox should be removed")
}

func TestBootstrapCoreExtensions_ReinstallsStaleSandbox(t *testing.T) {
	homeDir := t.TempDir()
	preCreateAllExtensions(t, homeDir)
	prependFailingGit(t)
	require.NoError(t, os.WriteFile(
		filepath.Join(ExtensionsDir(homeDir), "sandbox", "main.go"),
		[]byte("package main\n\nvar _ = SandboxMode(\"\")\n"),
		0o600,
	))

	result, err := BootstrapCoreExtensions(context.Background(), homeDir, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bootstrap failed for core extensions: sandbox")
	assert.Contains(t, result.Failed, "sandbox")
	assert.NotContains(t, result.Failed, "bash")
}

func TestBootstrapCoreExtensions_DoesNotReinstallRemovedCoreAfterMigration(t *testing.T) {
	homeDir := t.TempDir()
	preCreateAllExtensions(t, homeDir)
	prependFailingGit(t)

	extDir := ExtensionsDir(homeDir)
	require.NoError(t, os.RemoveAll(filepath.Join(extDir, "bash")))

	result, err := BootstrapCoreExtensions(context.Background(), homeDir, nil)
	require.NoError(t, err)
	assert.NotContains(t, result.Installed, "bash")
	assert.NotContains(t, result.Failed, "bash")
}

func TestBootstrapCoreExtensions_InstallsMissingMigrationExtensions(t *testing.T) {
	homeDir := t.TempDir()
	preCreateAllExtensions(t, homeDir)
	prependFailingGit(t)

	extDir := ExtensionsDir(homeDir)
	require.NoError(t, os.RemoveAll(filepath.Join(extDir, "guardian")))
	require.NoError(t, os.RemoveAll(filepath.Join(extDir, "tui-guardian")))
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, "tui-sandbox"), 0o750))

	result, err := BootstrapCoreExtensions(context.Background(), homeDir, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bootstrap failed for core extensions: guardian, tui-guardian")
	assert.ElementsMatch(t, []string{"guardian", "tui-guardian"}, result.Failed)

	_, statErr := os.Stat(filepath.Join(extDir, guardianSandboxMigrationMarker))
	require.NoError(t, statErr, "failed migration should leave marker for retry")
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
	prependFailingGit(t)

	extDir := ExtensionsDir(homeDir)
	require.NoError(t, os.MkdirAll(extDir, 0o750))

	// Pre-create all but one migration extension.
	for _, name := range CoreExtensionNames {
		if name == "guardian" {
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

	require.NoError(t, os.MkdirAll(filepath.Join(extDir, "tui-sandbox"), 0o750))

	result, err := BootstrapCoreExtensions(context.Background(), homeDir, func(msg string) {
		messages = append(messages, msg)
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bootstrap failed for core extensions: guardian")

	// Only guardian was missing in a migration and the fake git makes it fail deterministically.
	assert.True(t, slices.Contains(result.Installed, "guardian") || slices.Contains(result.Failed, "guardian"),
		"guardian should be in installed or failed")

	// No other extension should appear in either list.
	for _, name := range result.Installed {
		assert.Equal(t, "guardian", name, "only guardian should be in installed")
	}

	for _, name := range result.Failed {
		assert.Equal(t, "guardian", name, "only guardian should be in failed")
	}

	// Verify progress messages include the header.
	assert.NotEmpty(t, messages)
	assert.Contains(t, messages[0], "installing core extensions")

	// Should have a "-> guardian" progress line.
	hasGuardianMsg := false

	for _, msg := range messages {
		if strings.Contains(msg, "->") && strings.Contains(msg, "guardian") {
			hasGuardianMsg = true

			break
		}
	}

	assert.True(t, hasGuardianMsg, "should have progress message for guardian")
}
