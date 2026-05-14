package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindConfigPath_WeaveYaml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui"}`)

	got, err := FindConfigPath(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ".weave", "settings.json"), got)
}

func TestFindConfigPath_ConfigDir(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".weave")
	mkdir(t, configDir)
	writeFile(t, configDir, "settings.json", `{"ui_extension":"tui"}`)

	got, err := FindConfigPath(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(configDir, "settings.json"), got)
}

func TestFindConfigPath_WalkUp(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "a", "b", "c")
	mkdir(t, child)
	writeFile(t, root, ".weave/settings.json", `{"ui_extension":"tui"}`)

	got, err := FindConfigPath(child)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, ".weave", "settings.json"), got)
}

func TestFindConfigPath_NotFound(t *testing.T) {
	// Only valid when no global config exists
	globalDir, _ := GlobalConfigDir()
	if _, err := os.Stat(filepath.Join(globalDir, "settings.json")); err == nil {
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
	writeFile(t, configDir, "settings.json", `{"ui_extension":"tui"}`)

	got, err := FindConfigPath(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(configDir, "settings.json"), got)
}

func TestLoad_CoreDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui"}`)

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, "agent", cf.AgentLoop)
}

func TestLoad_CoreOverride(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"agent_loop":"custom-loop"}`)

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, "custom-loop", cf.AgentLoop)
}

func TestLoad_ExcludeExtensions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"exclude_extensions":["bash","grep"]}`)

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	require.Len(t, cf.ExcludeExtensions, 2)
	assert.Equal(t, "bash", cf.ExcludeExtensions[0])
	assert.Equal(t, "grep", cf.ExcludeExtensions[1])
}

func TestLoad_MissingFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()

	path, cf, _, err := LoadFromDir(projectDir, nil)
	require.NoError(t, err)

	// Should generate a global config in ~/.weave/settings.json
	assert.NotEmpty(t, path, "should have generated a global config")
	assert.Equal(t, "tui", cf.UIExtension)
	assert.Equal(t, "agent", cf.AgentLoop)

	_, statErr := os.Stat(path)
	require.NoError(t, statErr)
}

func TestLoad_UIDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui"}`)

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, "tui", cf.UIExtension, "default ui should be 'tui'")
}

func TestLoad_UIOverride(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"none"}`)

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, "none", cf.UIExtension)
}

func TestLoad_UIFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui"}`)

	_, cf, _, err := LoadFromDir(dir, []string{"--ui", "none"})
	require.NoError(t, err)

	assert.Equal(t, "none", cf.UIExtension)
}

func TestLoad_OutputFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui"}`)

	_, cf, _, err := LoadFromDir(dir, []string{"--output", "json"})
	require.NoError(t, err)

	assert.Equal(t, "json", cf.Output)
}

func TestLoad_ToolsFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui"}`)

	_, cf, _, err := LoadFromDir(dir, []string{"--tools", "read,grep,find"})
	require.NoError(t, err)

	assert.Equal(t, "read,grep,find", cf.ToolsFlag)
	assert.True(t, cf.ToolsSet, "ToolsSet should be true when --tools is passed")
}

func TestLoad_ToolsFlagEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui"}`)

	_, cf, _, err := LoadFromDir(dir, []string{"--tools="})
	require.NoError(t, err)

	assert.Empty(t, cf.ToolsFlag)
	assert.True(t, cf.ToolsSet, "ToolsSet should be true for explicit --tools=")
}

func TestLoad_ToolsFlagNotSet(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui"}`)

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	assert.Empty(t, cf.ToolsFlag)
	assert.False(t, cf.ToolsSet, "ToolsSet should be false when --tools is omitted")
}

func TestLoad_SubagentIDFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui"}`)

	_, cf, _, err := LoadFromDir(dir, []string{"--subagent-id", "subagent_explore_abc123"})
	require.NoError(t, err)

	assert.Equal(t, "subagent_explore_abc123", cf.SubagentID)
}

func TestLoad_SandboxModeFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui"}`)

	_, cf, _, err := LoadFromDir(dir, []string{"--sandbox", "readonly"})
	require.NoError(t, err)

	assert.Equal(t, "readonly", cf.SandboxMode)
}

func TestLoad_ModelFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui"}`)

	_, cf, _, err := LoadFromDir(dir, []string{"--model", "claude-haiku-4-5"})
	require.NoError(t, err)

	assert.Equal(t, "claude-haiku-4-5", cf.ModelFlag)
}

func TestLoad_PromptFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui"}`)

	_, cf, rest, err := LoadFromDir(dir, []string{"-p", "hello world"})
	require.NoError(t, err)

	assert.Equal(t, "hello world", cf.Prompt)
	assert.Empty(t, rest, "rest should not contain consumed flag args")
}

func TestLoad_PromptFlagWithRemainingArgs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui"}`)

	_, cf, rest, err := LoadFromDir(dir, []string{"-p", "hello", "extra", "args"})
	require.NoError(t, err)

	assert.Equal(t, "hello", cf.Prompt)
	assert.Equal(t, []string{"extra", "args"}, rest)
}

func TestLoad_HelpFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui"}`)

	// --help is no longer intercepted by LoadFromDir; it passes through to the generated binary.
	_, _, rest, err := LoadFromDir(dir, []string{"--help"})
	require.NoError(t, err)
	assert.Equal(t, []string{"--help"}, rest)
}

func TestLoad_HelpShortFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui"}`)

	// -h is no longer intercepted by LoadFromDir; it passes through to the generated binary.
	_, _, rest, err := LoadFromDir(dir, []string{"-h"})
	require.NoError(t, err)
	assert.Equal(t, []string{"-h"}, rest)
}

func TestLoad_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui","provider":"anthropic"}`)
	t.Setenv("WEAVE_PROVIDER", "openai")

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, "openai", cf.Provider, "env var should override file setting")
}

func TestEnsureGlobalConfig_GeneratesFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()

	path, err := EnsureGlobalConfig(projectDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".weave", "settings.json"), path)

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Contains(t, string(data), `"agent_loop"`)
	assert.Contains(t, string(data), `"agent"`)
}

func TestEnsureGlobalConfig_SkipsIfProjectConfigExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()
	writeFile(t, projectDir, ".weave/settings.json", `{"ui_extension":"none"}`)

	path, err := EnsureGlobalConfig(projectDir)
	require.NoError(t, err)
	assert.Empty(t, path, "should skip when project config exists")
}

func TestEnsureGlobalConfig_SkipsIfGlobalConfigExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	mkdir(t, globalDir)
	writeFile(t, globalDir, "settings.json", `{"ui_extension":"none"}`)
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
	assert.Contains(t, j, `"agent_loop"`)
	assert.Contains(t, j, `"agent"`)
	assert.Contains(t, j, `"ui_extension"`)
}

func TestDefaultSettings(t *testing.T) {
	s := DefaultSettings()
	assert.Equal(t, "tui", s.UIExtension)
	assert.Equal(t, "agent", s.AgentLoop)
	assert.Empty(t, s.ExcludeExtensions)
	assert.Nil(t, s.Providers)
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func TestPreferences_LoadsMergedSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	writeJSON(t, filepath.Join(globalDir, "settings.json"), &Settings{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-6",
	})
	writeJSON(t, filepath.Join(projectWeave, "settings.local.json"), &Settings{
		Model:         "gpt-5.5",
		ThinkingLevel: "high",
	})

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "settings.json"),
		settings: DefaultSettings(),
	}

	var prefs struct {
		Provider      string `json:"provider"`
		Model         string `json:"model"`
		ThinkingLevel string `json:"thinking_level"`
	}
	require.NoError(t, cfg.Preferences(&prefs))
	assert.Equal(t, "anthropic", prefs.Provider, "global provider should be preserved")
	assert.Equal(t, "gpt-5.5", prefs.Model, "local model should override global")
	assert.Equal(t, "high", prefs.ThinkingLevel)
}

func TestPreferences_NoSettingsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "settings.json"),
		settings: DefaultSettings(),
	}

	var prefs struct {
		Model string `json:"model"`
	}
	require.NoError(t, cfg.Preferences(&prefs))
	assert.Empty(t, prefs.Model)
}

func TestSavePreferences_MergesIntoGlobal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	// Pre-existing global settings with a model already set.
	writeJSON(t, filepath.Join(globalDir, "settings.json"), &Settings{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-6",
	})

	projectDir := t.TempDir()

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "settings.json"),
		settings: DefaultSettings(),
	}

	prefs := struct {
		Model string `json:"model"`
	}{
		Model: "gpt-5.5",
	}
	require.NoError(t, cfg.SavePreferences(&prefs))

	// Verify global settings now has the new model but kept the provider.
	loaded, err := LoadSettings()
	require.NoError(t, err)
	assert.Equal(t, "anthropic", loaded.Provider, "existing provider should be preserved")
	assert.Equal(t, "gpt-5.5", loaded.Model, "model should be updated")
}

func TestSavePreferences_CreatesFileIfMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "settings.json"),
		settings: DefaultSettings(),
	}

	prefs := struct {
		Model string `json:"model"`
	}{
		Model: "opus",
	}
	require.NoError(t, cfg.SavePreferences(&prefs))

	loaded, err := LoadSettings()
	require.NoError(t, err)
	assert.Equal(t, "opus", loaded.Model)
}

func TestSavePreferences_PreservesUIFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	// Pre-existing global settings with nested UI and tools sections
	writeJSON(t, filepath.Join(globalDir, "settings.json"), &Settings{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-6",
		UI: map[string]any{
			"theme":            "dark",
			"editor_max_lines": 30,
		},
		Tools: map[string]any{
			"bash": map[string]any{
				"timeout": 120,
			},
		},
	})

	projectDir := t.TempDir()

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "settings.json"),
		settings: DefaultSettings(),
	}

	// Save only model change — UI and tools should be preserved
	prefs := struct {
		Model string `json:"model"`
	}{
		Model: "gpt-5.5",
	}
	require.NoError(t, cfg.SavePreferences(&prefs))

	loaded, err := LoadSettings()
	require.NoError(t, err)
	assert.Equal(t, "anthropic", loaded.Provider, "existing provider should be preserved")
	assert.Equal(t, "gpt-5.5", loaded.Model, "model should be updated")
	require.NotNil(t, loaded.UI)
	assert.Equal(t, "dark", loaded.UI["theme"], "ui.theme should be preserved")
	assert.InDelta(t, float64(30), loaded.UI["editor_max_lines"], 0, "ui.editor_max_lines should be preserved")
	require.NotNil(t, loaded.Tools)
	bashConfig, ok := loaded.Tools["bash"].(map[string]any)
	require.True(t, ok, "tools.bash should be preserved")
	assert.InDelta(t, float64(120), bashConfig["timeout"], 0, "tools.bash.timeout should be preserved")
}

func TestSavePreferences_DeepMergesNestedFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	writeJSON(t, filepath.Join(globalDir, "settings.json"), &Settings{
		UI: map[string]any{
			"theme":            "dark",
			"editor_max_lines": 30,
		},
		Tools: map[string]any{
			"bash": map[string]any{
				"timeout": 120,
				"shell":   "fish",
			},
		},
	})

	projectDir := t.TempDir()
	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "settings.json"),
		settings: DefaultSettings(),
	}

	// Save a partial UI update — only theme changes, editor_max_lines must survive
	prefs := struct {
		UI struct {
			Theme string `json:"theme"`
		} `json:"ui"`
		Tools struct {
			Bash struct {
				Timeout int `json:"timeout"`
			} `json:"bash"`
		} `json:"tools"`
	}{
		UI: struct {
			Theme string `json:"theme"`
		}{Theme: "light"},
		Tools: struct {
			Bash struct {
				Timeout int `json:"timeout"`
			} `json:"bash"`
		}{
			Bash: struct {
				Timeout int `json:"timeout"`
			}{Timeout: 60},
		},
	}
	require.NoError(t, cfg.SavePreferences(&prefs))

	loaded, err := LoadSettings()
	require.NoError(t, err)
	require.NotNil(t, loaded.UI)
	assert.Equal(t, "light", loaded.UI["theme"], "ui.theme should be updated")
	assert.InDelta(t, float64(30), loaded.UI["editor_max_lines"], 0, "ui.editor_max_lines should be preserved")
	require.NotNil(t, loaded.Tools)
	bashConfig, ok := loaded.Tools["bash"].(map[string]any)
	require.True(t, ok, "tools.bash should be preserved")
	assert.InDelta(t, float64(60), bashConfig["timeout"], 0, "tools.bash.timeout should be updated")
	assert.Equal(t, "fish", bashConfig["shell"], "tools.bash.shell should be preserved")
}

func TestRespectGitignore_DefaultTrue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "settings.json"),
		settings: DefaultSettings(),
	}

	assert.True(t, cfg.RespectGitignore(), "default should be true when no settings file")
}

func TestRespectGitignore_ExplicitTrue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	v := true
	writeJSON(t, filepath.Join(projectWeave, "settings.local.json"), &Settings{
		RespectGitignore: &v,
	})

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "settings.json"),
		settings: DefaultSettings(),
	}

	assert.True(t, cfg.RespectGitignore())
}

func TestRespectGitignore_ExplicitFalse(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	v := false
	writeJSON(t, filepath.Join(projectWeave, "settings.local.json"), &Settings{
		RespectGitignore: &v,
	})

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "settings.json"),
		settings: DefaultSettings(),
	}

	assert.False(t, cfg.RespectGitignore())
}

func TestRespectGitignore_LocalOverridesGlobal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	globalVal := true
	writeJSON(t, filepath.Join(globalDir, "settings.json"), &Settings{
		RespectGitignore: &globalVal,
	})

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	localVal := false
	writeJSON(t, filepath.Join(projectWeave, "settings.local.json"), &Settings{
		RespectGitignore: &localVal,
	})

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "settings.json"),
		settings: DefaultSettings(),
	}

	assert.False(t, cfg.RespectGitignore(), "local layer should override global")
}

func TestPreferences_IncludesProjectConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	writeJSON(t, filepath.Join(globalDir, "settings.json"), &Settings{
		Provider: "anthropic",
	})
	writeJSON(t, filepath.Join(projectWeave, "settings.json"), &Settings{
		Model: "project-model",
	})

	cfg := &FullConfig{
		filePath: filepath.Join(projectWeave, "settings.json"),
		settings: mustLoadSettings(t, filepath.Join(projectWeave, "settings.json")),
	}

	var prefs struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	require.NoError(t, cfg.Preferences(&prefs))
	assert.Equal(t, "anthropic", prefs.Provider, "global provider should be preserved")
	assert.Equal(t, "project-model", prefs.Model, "project model should be included")
}

func TestPreferences_LocalOverridesProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	writeJSON(t, filepath.Join(projectWeave, "settings.json"), &Settings{
		Provider: "openai",
		Model:    "project-model",
	})
	writeJSON(t, filepath.Join(projectWeave, "settings.local.json"), &Settings{
		Model: "local-model",
	})

	cfg := &FullConfig{
		filePath: filepath.Join(projectWeave, "settings.json"),
		settings: mustLoadSettings(t, filepath.Join(projectWeave, "settings.json")),
	}

	var prefs struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	require.NoError(t, cfg.Preferences(&prefs))
	assert.Equal(t, "openai", prefs.Provider, "project provider should be preserved")
	assert.Equal(t, "local-model", prefs.Model, "local model should override project")
}

func TestExtensionConfig_ProvidersUsesLayeredSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	writeJSON(t, filepath.Join(globalDir, "settings.json"), &Settings{
		Providers: map[string]any{
			"openai": map[string]any{"base_url": "https://global.example.com"},
		},
	})
	writeJSON(t, filepath.Join(projectWeave, "settings.json"), &Settings{
		Providers: map[string]any{
			"openai": map[string]any{"model": "gpt-project"},
		},
	})
	writeJSON(t, filepath.Join(projectWeave, "settings.local.json"), &Settings{
		Providers: map[string]any{
			"openai": map[string]any{"base_url": "https://local.example.com"},
		},
	})

	cfg := &FullConfig{
		filePath: filepath.Join(projectWeave, "settings.json"),
		settings: mustLoadSettings(t, filepath.Join(projectWeave, "settings.json")),
	}

	var pc struct {
		Model   string `json:"model"`
		BaseURL string `json:"base_url"`
	}
	require.NoError(t, cfg.ExtensionConfig("providers", "openai", &pc))
	assert.Equal(t, "gpt-project", pc.Model, "project model should be preserved")
	assert.Equal(t, "https://local.example.com", pc.BaseURL, "local base_url should override global and project")
}

func TestExtensionConfig_ProvidersFallbackToProjectWhenNoLayered(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	writeJSON(t, filepath.Join(projectWeave, "settings.json"), &Settings{
		Providers: map[string]any{
			"anthropic": map[string]any{"model": "claude-project"},
		},
	})

	cfg := &FullConfig{
		filePath: filepath.Join(projectWeave, "settings.json"),
		settings: mustLoadSettings(t, filepath.Join(projectWeave, "settings.json")),
	}

	var pc struct {
		Model string `json:"model"`
	}
	require.NoError(t, cfg.ExtensionConfig("providers", "anthropic", &pc))
	assert.Equal(t, "claude-project", pc.Model)
}

func mustLoadSettings(t *testing.T, path string) *Settings {
	t.Helper()

	s, err := LoadFromFile(path)
	require.NoError(t, err)

	return s
}

func TestFilterExtensionArgs_Basic(t *testing.T) {
	args := []string{"--bash-timeout", "60", "--model", "claude"}
	got := filterExtensionArgs(args, "bash")
	assert.Equal(t, []string{"--timeout", "60"}, got)
}

func TestFilterExtensionArgs_EqualsForm(t *testing.T) {
	args := []string{"--bash-timeout=120", "--model", "claude"}
	got := filterExtensionArgs(args, "bash")
	assert.Equal(t, []string{"--timeout=120"}, got)
}

func TestFilterExtensionArgs_MultipleArgs(t *testing.T) {
	args := []string{"--bash-timeout", "60", "--bash-shell", "fish", "--model", "claude"}
	got := filterExtensionArgs(args, "bash")
	assert.Equal(t, []string{"--timeout", "60", "--shell", "fish"}, got)
}

func TestFilterExtensionArgs_NonMatchingPrefix(t *testing.T) {
	args := []string{"--grep-pattern", "foo", "--model", "claude"}
	got := filterExtensionArgs(args, "bash")
	assert.Nil(t, got)
}

func TestFilterExtensionArgs_EmptyArgs(t *testing.T) {
	got := filterExtensionArgs(nil, "bash")
	assert.Nil(t, got)

	got = filterExtensionArgs([]string{}, "bash")
	assert.Nil(t, got)
}

func TestFilterExtensionArgs_MixedWithOtherFlags(t *testing.T) {
	args := []string{"--model", "claude", "--bash-timeout", "90", "--output", "json"}
	got := filterExtensionArgs(args, "bash")
	assert.Equal(t, []string{"--timeout", "90"}, got)
}

func TestToMapAny_Nil(t *testing.T) {
	got, err := toMapAny(nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestToMapAny_AlreadyMap(t *testing.T) {
	input := map[string]any{"key": "value"}
	got, err := toMapAny(input)
	require.NoError(t, err)
	assert.Equal(t, input, got)
}

func TestToMapAny_Struct(t *testing.T) {
	type testStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	input := testStruct{Name: "test", Value: 42}
	got, err := toMapAny(input)
	require.NoError(t, err)
	assert.Equal(t, "test", got["name"])
	assert.Equal(t, json.Number("42"), got["value"])
}

func TestToMapAny_ErrorOnNonSerializable(t *testing.T) {
	// Struct with a function field cannot be JSON serialized.
	type badStruct struct {
		Name string `json:"name"`
		Fn   func() `json:"fn"`
	}

	input := badStruct{Name: "test", Fn: func() {}}

	_, err := toMapAny(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal")
}
