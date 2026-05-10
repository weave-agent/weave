package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"weave/sdk/model"
)

func TestFindConfigPath_WeaveYaml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "ui: tui\n")

	got, err := FindConfigPath(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ".weave.yaml"), got)
}

func TestFindConfigPath_ConfigDir(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".weave")
	mkdir(t, configDir)
	writeFile(t, configDir, "config.yaml", "ui: tui\n")

	got, err := FindConfigPath(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(configDir, "config.yaml"), got)
}

func TestFindConfigPath_WalkUp(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "a", "b", "c")
	mkdir(t, child)
	writeFile(t, root, ".weave.yaml", "ui: tui\n")

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
	writeFile(t, configDir, "config.json", `{"ui":"tui"}`)

	got, err := FindConfigPath(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(configDir, "config.json"), got)
}

func TestFindConfigPath_PrefersWeaveYaml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "ui: tui\n")
	configDir := filepath.Join(dir, ".weave")
	mkdir(t, configDir)
	writeFile(t, configDir, "config.yaml", "ui: none\n")

	got, err := FindConfigPath(dir)
	require.NoError(t, err)
	assert.Equal(t, ".weave.yaml", filepath.Base(got))
}

func TestLoad_CoreDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "ui: tui\n")

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, "loop", cf.Core.AgentLoop)
}

func TestLoad_CoreOverride(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "core:\n  agent_loop: custom-loop\n")

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, "custom-loop", cf.Core.AgentLoop)
}

func TestLoad_ExcludeExtensions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "exclude_extensions:\n  - bash\n  - grep\n")

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

	// Should generate a global config in ~/.weave/config.json
	assert.NotEmpty(t, path, "should have generated a global config")
	assert.Equal(t, "tui", cf.UI)
	assert.Equal(t, "loop", cf.Core.AgentLoop)

	_, statErr := os.Stat(path)
	require.NoError(t, statErr)
}

func TestLoad_UIDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "ui: tui\n")

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, "tui", cf.UI, "default ui should be 'tui'")
}

func TestLoad_UIOverride(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "ui: none\n")

	_, cf, _, err := LoadFromDir(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, "none", cf.UI)
}

func TestLoad_UIFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "ui: tui\n")

	_, cf, _, err := LoadFromDir(dir, []string{"--ui", "none"})
	require.NoError(t, err)

	assert.Equal(t, "none", cf.UI)
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
	assert.Contains(t, string(data), `"agent_loop"`)
	assert.Contains(t, string(data), `"loop"`)
}

func TestEnsureGlobalConfig_SkipsIfProjectConfigExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()
	writeFile(t, projectDir, ".weave.yaml", "ui: none\n")

	path, err := EnsureGlobalConfig(projectDir)
	require.NoError(t, err)
	assert.Empty(t, path, "should skip when project config exists")
}

func TestEnsureGlobalConfig_SkipsIfGlobalConfigExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	mkdir(t, globalDir)
	writeFile(t, globalDir, "config.json", `{"ui":"none"}`)
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
	assert.Contains(t, j, `"loop"`)
	assert.Contains(t, j, `"ui"`)
}

func TestDefaultFile(t *testing.T) {
	f := DefaultFile()
	assert.Equal(t, "tui", f.UI)
	assert.Equal(t, "loop", f.Core.AgentLoop)
	assert.Empty(t, f.ExcludeExtensions)
	assert.Nil(t, f.Providers)
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
	writeJSON(t, filepath.Join(projectWeave, "settings.json"), &Settings{
		Model:         "gpt-5.5",
		ThinkingLevel: "high",
	})

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "config.yaml"),
		file:     DefaultFile(),
		auth:     &AuthFile{},
	}

	var prefs struct {
		Provider      string `json:"provider"`
		Model         string `json:"model"`
		ThinkingLevel string `json:"thinking_level"`
	}
	require.NoError(t, cfg.Preferences(&prefs))
	assert.Equal(t, "anthropic", prefs.Provider, "global provider should be preserved")
	assert.Equal(t, "gpt-5.5", prefs.Model, "project model should override global")
	assert.Equal(t, "high", prefs.ThinkingLevel)
}

func TestPreferences_NoSettingsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "config.yaml"),
		file:     DefaultFile(),
		auth:     &AuthFile{},
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
		filePath: filepath.Join(projectDir, ".weave", "config.yaml"),
		file:     DefaultFile(),
		auth:     &AuthFile{},
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
		filePath: filepath.Join(projectDir, ".weave", "config.yaml"),
		file:     DefaultFile(),
		auth:     &AuthFile{},
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

func TestProviderHasKey_EnvVar(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Setenv("ANTHROPIC_API_KEY", "sk-test-key")

	// Register the env var mapping.
	model.RegisterProviderEnvVar("anthropic", "ANTHROPIC_API_KEY")
	t.Cleanup(func() { model.ResetProviderEnvVarRegistry() })

	cfg := &FullConfig{
		file: DefaultFile(),
		auth: &AuthFile{},
	}

	assert.True(t, cfg.ProviderHasKey("anthropic"))
}

func TestProviderHasKey_AuthFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Register but don't set env var.
	model.RegisterProviderEnvVar("anthropic", "ANTHROPIC_API_KEY")
	t.Cleanup(func() { model.ResetProviderEnvVarRegistry() })

	require.NoError(t, os.MkdirAll(filepath.Join(home, ".weave"), 0o750))
	require.NoError(t, SetProviderKey("anthropic", "sk-from-auth"))

	cfg := &FullConfig{
		file: DefaultFile(),
		auth: &AuthFile{},
	}

	assert.True(t, cfg.ProviderHasKey("anthropic"))
}

func TestProviderHasKey_NotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &FullConfig{
		file: DefaultFile(),
		auth: &AuthFile{},
	}

	assert.False(t, cfg.ProviderHasKey("anthropic"))
}

func TestSetProviderKey_Delegates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &FullConfig{
		file: DefaultFile(),
		auth: &AuthFile{},
	}

	require.NoError(t, cfg.SetProviderKey("openai", "sk-openai-key"))

	auth, err := LoadAuth()
	require.NoError(t, err)
	assert.Equal(t, "sk-openai-key", auth.GetProviderKey("openai"))
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
		UI: &UISettings{
			Theme:          "dark",
			EditorMaxLines: 30,
		},
		Tools: map[string]any{
			"bash": map[string]any{
				"timeout": 120,
			},
		},
	})

	projectDir := t.TempDir()

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "config.yaml"),
		file:     DefaultFile(),
		auth:     &AuthFile{},
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
	assert.Equal(t, "dark", loaded.UI.Theme, "ui.theme should be preserved")
	assert.Equal(t, 30, loaded.UI.EditorMaxLines, "ui.editor_max_lines should be preserved")
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
		UI: &UISettings{
			Theme:          "dark",
			EditorMaxLines: 30,
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
		filePath: filepath.Join(projectDir, ".weave", "config.yaml"),
		file:     DefaultFile(),
		auth:     &AuthFile{},
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
	assert.Equal(t, "light", loaded.UI.Theme, "ui.theme should be updated")
	assert.Equal(t, 30, loaded.UI.EditorMaxLines, "ui.editor_max_lines should be preserved")
	require.NotNil(t, loaded.Tools)
	bashConfig, ok := loaded.Tools["bash"].(map[string]any)
	require.True(t, ok, "tools.bash should be preserved")
	assert.InDelta(t, float64(60), bashConfig["timeout"], 0, "tools.bash.timeout should be updated")
	assert.Equal(t, "fish", bashConfig["shell"], "tools.bash.shell should be preserved")
}

func TestProviderHasKey_ConfigFileAPIKey(t *testing.T) {
	cfg := &FullConfig{
		file: &File{
			Providers: map[string]any{
				"anthropic": map[string]any{"api_key": "sk-config-key"},
			},
		},
		auth: &AuthFile{},
	}

	assert.True(t, cfg.ProviderHasKey("anthropic"), "should return true when API key is in config file")
}

func TestProviderHasKey_LoadAuthFails_FallsBackToCachedAuth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Register env var but don't set it
	model.RegisterProviderEnvVar("anthropic", "ANTHROPIC_API_KEY")
	t.Cleanup(func() { model.ResetProviderEnvVarRegistry() })

	// Create a corrupted auth file so LoadAuth fails
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".weave"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".weave", "auth.json"), []byte("not-json"), 0o600))

	cfg := &FullConfig{
		file: DefaultFile(),
		auth: &AuthFile{
			//nolint:gosec // test credential, not a real secret
			Providers: map[string]ProviderAuth{
				"anthropic": {APIKey: "cached-auth-key"},
			},
		},
	}

	assert.True(t, cfg.ProviderHasKey("anthropic"), "should fall back to cached auth when LoadAuth fails")
}

func TestProviderHasKey_LoadAuthFails_NoCachedAuth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Register env var but don't set it
	model.RegisterProviderEnvVar("anthropic", "ANTHROPIC_API_KEY")
	t.Cleanup(func() { model.ResetProviderEnvVarRegistry() })

	// Create a corrupted auth file so LoadAuth fails
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".weave"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".weave", "auth.json"), []byte("not-json"), 0o600))

	cfg := &FullConfig{
		file: DefaultFile(),
		auth: &AuthFile{},
	}

	assert.False(t, cfg.ProviderHasKey("anthropic"), "should return false when auth load fails and no cached auth")
}

func TestRespectGitignore_DefaultTrue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "config.yaml"),
		file:     DefaultFile(),
		auth:     &AuthFile{},
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
	writeJSON(t, filepath.Join(projectWeave, "settings.json"), &Settings{
		RespectGitignore: &v,
	})

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "config.yaml"),
		file:     DefaultFile(),
		auth:     &AuthFile{},
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
	writeJSON(t, filepath.Join(projectWeave, "settings.json"), &Settings{
		RespectGitignore: &v,
	})

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "config.yaml"),
		file:     DefaultFile(),
		auth:     &AuthFile{},
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
	writeJSON(t, filepath.Join(projectWeave, "settings.json"), &Settings{
		RespectGitignore: &localVal,
	})

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "config.yaml"),
		file:     DefaultFile(),
		auth:     &AuthFile{},
	}

	assert.False(t, cfg.RespectGitignore(), "project layer should override global")
}
