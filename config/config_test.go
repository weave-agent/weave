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
	// Only valid when no global config exists
	globalDir, _ := GlobalConfigDir()
	if _, err := os.Stat(filepath.Join(globalDir, "config.json")); err == nil {
		t.Skip("skipping: global config exists, so FindConfigPath always succeeds")
	}

	dir := t.TempDir()

	_, err := FindConfigPath(dir)
	require.Error(t, err)
}

func TestFindConfigPath_ConfigJSON(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".weave")
	mkdir(t, configDir)
	writeFile(t, configDir, "config.json", `{"extensions":["noop"]}`)

	got, err := FindConfigPath(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(configDir, "config.json"), got)
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
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()

	path, cf, _, err := LoadFromDir(projectDir, nil)
	require.NoError(t, err)

	// Should generate a global config in ~/.weave/config.json
	assert.NotEmpty(t, path, "should have generated a global config")
	assert.Equal(t, "tui", cf.UI)
	assert.Equal(t, "loop", cf.Core.AgentLoop)
	require.Len(t, cf.Extensions, 8)

	_, statErr := os.Stat(path)
	require.NoError(t, statErr)
}

func TestLoad_UIDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: []\n")

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, "tui", cf.UI, "default ui should be 'tui'")
}

func TestLoad_UIOverride(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "ui: none\nextensions: []\n")

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, "none", cf.UI)
}

func TestLoad_UIFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: []\n")

	_, cf, _, err := LoadFromDir(dir, []string{"--ui", "custom"})
	require.NoError(t, err)

	assert.Equal(t, "custom", cf.UI)
}

func TestEnsureGlobalConfig_GeneratesFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()

	path, err := EnsureGlobalConfig(projectDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".weave", "config.json"), path)

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Contains(t, string(data), `"anthropic"`)
	assert.Contains(t, string(data), `"openai"`)
	assert.Contains(t, string(data), `"zai"`)
	assert.Contains(t, string(data), `"bash"`)
	assert.Contains(t, string(data), `"jsonl"`)
}

func TestEnsureGlobalConfig_SkipsIfProjectConfigExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()
	writeFile(t, projectDir, ".weave.yaml", "extensions: [custom]\n")

	path, err := EnsureGlobalConfig(projectDir)
	require.NoError(t, err)
	assert.Empty(t, path, "should skip when project config exists")
}

func TestEnsureGlobalConfig_SkipsIfGlobalConfigExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	mkdir(t, globalDir)
	writeFile(t, globalDir, "config.json", `{"core":{"providers":["openai"]}}`)
	projectDir := t.TempDir()

	path, err := EnsureGlobalConfig(projectDir)
	require.NoError(t, err)
	assert.Empty(t, path, "should skip when global config already exists")
}

func TestEnsureGlobalConfig_Idempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()

	path1, err := EnsureGlobalConfig(projectDir)
	require.NoError(t, err)
	assert.NotEmpty(t, path1)

	path2, err := EnsureGlobalConfig(projectDir)
	require.NoError(t, err)
	assert.Empty(t, path2, "should skip on second call")
}

func TestDefaultConfigJSON(t *testing.T) {
	j := DefaultConfigJSON()
	assert.Contains(t, j, `"anthropic"`)
	assert.Contains(t, j, `"openai"`)
	assert.Contains(t, j, `"zai"`)
	assert.Contains(t, j, `"jsonl"`)
	assert.Contains(t, j, `"bash"`)
	assert.Contains(t, j, `"edit"`)
	assert.Contains(t, j, `"find"`)
	assert.Contains(t, j, `"grep"`)
	assert.Contains(t, j, `"ls"`)
	assert.Contains(t, j, `"read"`)
	assert.Contains(t, j, `"write"`)
}

func TestLoad_SkillsField(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: []\nskills:\n  my-skill:\n    enabled: true\n    timeout: 30\n")

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	require.NotNil(t, cf.Skills)
	require.Contains(t, cf.Skills, "my-skill")

	skillConfig, ok := cf.Skills["my-skill"].(map[string]any)
	require.True(t, ok, "skill config should be a map")
	assert.Equal(t, true, skillConfig["enabled"])
	assert.Equal(t, 30, skillConfig["timeout"])
}

func TestLoad_SkillsEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: []\n")

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	assert.Nil(t, cf.Skills, "skills should be nil when not specified in config")
}

func TestLoad_SkillsMultiple(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "extensions: []\nskills:\n  skill-a:\n    opt: val\n  skill-b:\n    flag: true\n")

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	require.NotNil(t, cf.Skills)
	assert.Len(t, cf.Skills, 2)
	assert.Contains(t, cf.Skills, "skill-a")
	assert.Contains(t, cf.Skills, "skill-b")
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
