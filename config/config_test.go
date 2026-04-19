package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindConfigPath_WeaveYaml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: [noop]\n")

	got, err := FindConfigPath(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ".weave.yaml"), got)
}

func TestFindConfigPath_ConfigDir(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".weave")
	mkdir(t, configDir)
	writeFile(t, configDir, "config.yaml", "extensions: []\n")

	got, err := FindConfigPath(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(configDir, "config.yaml"), got)
}

func TestFindConfigPath_WalkUp(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "a", "b", "c")
	mkdir(t, child)
	writeFile(t, root, ".weave.yaml", "extensions: []\n")

	got, err := FindConfigPath(child)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, ".weave.yaml"), got)
}

func TestFindConfigPath_NotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := FindConfigPath(dir)
	require.Error(t, err)
}

func TestFindConfigPath_PrefersWeaveYaml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: [first]\n")
	configDir := filepath.Join(dir, ".weave")
	mkdir(t, configDir)
	writeFile(t, configDir, "config.yaml", "extensions: [second]\n")

	got, err := FindConfigPath(dir)
	require.NoError(t, err)
	assert.Equal(t, ".weave.yaml", filepath.Base(got))
}

func TestLoad_Extensions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: [noop, logging]\n")

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	require.Len(t, cf.Extensions, 2)
	assert.Equal(t, "noop", cf.Extensions[0])
	assert.Equal(t, "logging", cf.Extensions[1])
}

func TestLoad_CoreDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: []\n")

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, "loop", cf.Core.AgentLoop)
	assert.Equal(t, []string{"anthropic"}, cf.Core.Providers)
}

func TestLoad_CoreOverride(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "core:\n  agent_loop: custom-loop\n  providers:\n    - openai\n    - google\nextensions:\n  - bash\n")

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, "custom-loop", cf.Core.AgentLoop)
	require.Len(t, cf.Core.Providers, 2)
	assert.Equal(t, "openai", cf.Core.Providers[0])
	assert.Equal(t, "google", cf.Core.Providers[1])
	require.Len(t, cf.Extensions, 1)
	assert.Equal(t, "bash", cf.Extensions[0])
}

func TestLoad_CoreExts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "core:\n  providers:\n    - anthropic\n    - openai\nextensions:\n  - bash\n  - file\n")

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	coreExts, optExts := cf.CoreExts()

	expectedCore := []string{"loop", "anthropic", "openai"}
	require.Len(t, coreExts, len(expectedCore))

	for i, name := range expectedCore {
		assert.Equal(t, name, coreExts[i])
	}

	expectedOpt := []string{"bash", "file"}
	require.Len(t, optExts, len(expectedOpt))

	for i, name := range expectedOpt {
		assert.Equal(t, name, optExts[i])
	}
}

func TestLoad_CoreExtsDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: []\n")

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	coreExts, optExts := cf.CoreExts()

	require.Len(t, coreExts, 2)
	assert.Equal(t, "loop", coreExts[0])
	assert.Equal(t, "anthropic", coreExts[1])
	assert.Empty(t, optExts)
}

func TestLoad_MissingFile(t *testing.T) {
	_, _, _, err := LoadFromDir("/nonexistent", nil)
	require.Error(t, err)
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
