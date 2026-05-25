package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weave-agent/weave/sdk"
)

func TestMergeSettings_EmptyLayers(t *testing.T) {
	result := MergeSettings()
	assert.Equal(t, &Settings{}, result)
}

func TestMergeSettings_NilLayers(t *testing.T) {
	result := MergeSettings(nil, nil, nil)
	assert.Equal(t, &Settings{}, result)
}

func TestMergeSettings_SingleLayer(t *testing.T) {
	s := &Settings{
		Provider:      "anthropic",
		Model:         "claude-opus-4-7",
		ThinkingLevel: "high",
		UI: map[string]any{
			"theme":            "dark",
			"editor_max_lines": 30,
		},
		Tools: map[string]any{
			"bash": map[string]any{"timeout": 60},
		},
	}

	result := MergeSettings(s)
	assert.Equal(t, s, result)
}

func TestMergeSettings_PrimitiveOverride(t *testing.T) {
	layer1 := &Settings{Provider: "anthropic", Model: "model-a"}
	layer2 := &Settings{Provider: "openai", ThinkingLevel: "high"}

	result := MergeSettings(layer1, layer2)

	assert.Equal(t, "openai", result.Provider)
	assert.Equal(t, "model-a", result.Model)
	assert.Equal(t, "high", result.ThinkingLevel)
}

func TestMergeSettings_UIDeepMerge(t *testing.T) {
	layer1 := &Settings{
		UI: map[string]any{
			"theme":            "dark",
			"editor_max_lines": 20,
		},
	}
	layer2 := &Settings{
		UI: map[string]any{
			"editor_max_lines": 40,
		},
	}

	result := MergeSettings(layer1, layer2)

	require.NotNil(t, result.UI)
	assert.Equal(t, "dark", result.UI["theme"])
	assert.Equal(t, 40, result.UI["editor_max_lines"])
}

func TestMergeSettings_UINilToNonNil(t *testing.T) {
	layer1 := &Settings{Provider: "anthropic"}
	layer2 := &Settings{
		UI: map[string]any{"theme": "light"},
	}

	result := MergeSettings(layer1, layer2)

	require.NotNil(t, result.UI)
	assert.Equal(t, "light", result.UI["theme"])
}

func TestMergeSettings_ToolsMergeByKey(t *testing.T) {
	layer1 := &Settings{
		Tools: map[string]any{
			"bash": map[string]any{"timeout": 120, "shell": "/bin/bash"},
			"grep": map[string]any{"context": 3},
		},
	}
	layer2 := &Settings{
		Tools: map[string]any{
			"bash": map[string]any{"timeout": 60},
			"read": map[string]any{"max_lines": 500},
		},
	}

	result := MergeSettings(layer1, layer2)

	require.Len(t, result.Tools, 3)
	assert.Equal(t, map[string]any{"timeout": 60, "shell": "/bin/bash"}, result.Tools["bash"])
	assert.Equal(t, map[string]any{"context": 3}, result.Tools["grep"])
	assert.Equal(t, map[string]any{"max_lines": 500}, result.Tools["read"])
}

func TestMergeSettings_NilInMiddle(t *testing.T) {
	layer1 := &Settings{Provider: "anthropic"}
	layer2 := (*Settings)(nil)
	layer3 := &Settings{Model: "opus"}

	result := MergeSettings(layer1, layer2, layer3)

	assert.Equal(t, "anthropic", result.Provider)
	assert.Equal(t, "opus", result.Model)
}

func TestMergeSettings_ThreeLayers(t *testing.T) {
	global := &Settings{
		ThinkingLevel: "medium",
		UI:            map[string]any{"theme": "dark"},
	}
	project := &Settings{
		Model: "claude-opus-4-7",
	}
	local := &Settings{
		UI: map[string]any{"editor_max_lines": 20},
	}

	result := MergeSettings(global, project, local)

	assert.Equal(t, "medium", result.ThinkingLevel)
	assert.Equal(t, "claude-opus-4-7", result.Model)
	require.NotNil(t, result.UI)
	assert.Equal(t, "dark", result.UI["theme"])
	assert.Equal(t, 20, result.UI["editor_max_lines"])
}

func TestMergeSettings_RespectGitignore(t *testing.T) {
	t.Run("nil preserved when no layer sets it", func(t *testing.T) {
		result := MergeSettings(&Settings{Provider: "anthropic"})
		assert.Nil(t, result.RespectGitignore)
	})

	t.Run("explicit value wins", func(t *testing.T) {
		v := false
		result := MergeSettings(&Settings{RespectGitignore: &v})
		require.NotNil(t, result.RespectGitignore)
		assert.False(t, *result.RespectGitignore)
	})

	t.Run("later layer overrides", func(t *testing.T) {
		t1 := true
		f1 := false
		result := MergeSettings(&Settings{RespectGitignore: &t1}, &Settings{RespectGitignore: &f1})
		require.NotNil(t, result.RespectGitignore)
		assert.False(t, *result.RespectGitignore)
	})

	t.Run("nil layer does not clear value", func(t *testing.T) {
		v := true
		result := MergeSettings(&Settings{RespectGitignore: &v}, &Settings{Provider: "openai"})
		require.NotNil(t, result.RespectGitignore)
		assert.True(t, *result.RespectGitignore)
	})
}

func TestMergeSettings_GuardianProfilesMergeByName(t *testing.T) {
	askFallback := true
	global := &Settings{
		Guardian: GuardianFileConfig{
			Profile:     "ask",
			AskFallback: &askFallback,
			Profiles: map[string]sdk.GuardianProfile{
				"team": {
					Name:        "team",
					Description: "global team profile",
					Rules: []sdk.GuardianProfileRule{
						{Actions: []sdk.GuardianAction{sdk.GuardianActionRead}, Decision: sdk.GuardianDecisionAllow},
					},
				},
			},
		},
	}
	local := &Settings{
		Guardian: GuardianFileConfig{
			Profile: "team",
			Profiles: map[string]sdk.GuardianProfile{
				"team": {
					Name:        "team",
					Description: "local team profile",
					Rules: []sdk.GuardianProfileRule{
						{Actions: []sdk.GuardianAction{sdk.GuardianActionWrite}, Decision: sdk.GuardianDecisionAsk},
					},
				},
			},
		},
	}

	result := MergeSettings(global, local)

	assert.Equal(t, "team", result.Guardian.Profile)
	require.NotNil(t, result.Guardian.AskFallback)
	assert.True(t, *result.Guardian.AskFallback)
	require.Contains(t, result.Guardian.Profiles, "team")
	assert.Equal(t, "local team profile", result.Guardian.Profiles["team"].Description)
	assert.Equal(t, sdk.GuardianDecisionAsk, result.Guardian.Profiles["team"].Rules[0].Decision)
}

func TestMergeSettings_SandboxContainmentSettings(t *testing.T) {
	enabled := true
	failUnavailable := true
	allowFallback := true
	networkEnabled := true
	allowListen := true
	global := &Settings{
		Sandbox: SandboxFileConfig{
			Enabled:                  &enabled,
			FailIfUnavailable:        &failUnavailable,
			AllowUnsandboxedFallback: &allowFallback,
			Filesystem: SandboxFilesystemConfig{
				ReadOnly:  []string{"/"},
				ReadWrite: []string{"/var/tmp"},
				Blocked:   []string{"/private"},
			},
			Network: SandboxNetworkConfig{
				Enabled:     &networkEnabled,
				AllowHosts:  []string{"example.com"},
				AllowPorts:  []string{"443"},
				BlockHosts:  []string{"global.blocked"},
				AllowListen: &allowListen,
			},
		},
	}

	disabled := false
	noListen := false
	local := &Settings{
		Sandbox: SandboxFileConfig{
			Enabled: &disabled,
			Filesystem: SandboxFilesystemConfig{
				ReadWrite: []string{"/tmp"},
				Blocked:   []string{"/secret"},
			},
			Network: SandboxNetworkConfig{
				Enabled:     &disabled,
				AllowPorts:  []string{"8443"},
				BlockHosts:  []string{"metadata.google.internal"},
				AllowListen: &noListen,
			},
		},
	}

	result := MergeSettings(global, local)

	require.NotNil(t, result.Sandbox.Enabled)
	assert.False(t, *result.Sandbox.Enabled)
	require.NotNil(t, result.Sandbox.FailIfUnavailable)
	assert.True(t, *result.Sandbox.FailIfUnavailable)
	require.NotNil(t, result.Sandbox.AllowUnsandboxedFallback)
	assert.True(t, *result.Sandbox.AllowUnsandboxedFallback)
	assert.Equal(t, []string{"/"}, result.Sandbox.Filesystem.ReadOnly)
	assert.Equal(t, []string{"/tmp"}, result.Sandbox.Filesystem.ReadWrite)
	assert.Equal(t, []string{"/secret"}, result.Sandbox.Filesystem.Blocked)
	require.NotNil(t, result.Sandbox.Network.Enabled)
	assert.False(t, *result.Sandbox.Network.Enabled)
	assert.Equal(t, []string{"example.com"}, result.Sandbox.Network.AllowHosts)
	assert.Equal(t, []string{"8443"}, result.Sandbox.Network.AllowPorts)
	assert.Equal(t, []string{"metadata.google.internal"}, result.Sandbox.Network.BlockHosts)
	require.NotNil(t, result.Sandbox.Network.AllowListen)
	assert.False(t, *result.Sandbox.Network.AllowListen)
}

func TestLoadLayeredSettings_NoFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()

	result, err := LoadLayeredSettings(projectDir)
	require.NoError(t, err)
	assert.Equal(t, &Settings{}, result)
}

func TestLoadLayeredSettings_GlobalOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	globalSettings := Settings{
		Provider:      "anthropic",
		ThinkingLevel: "medium",
	}
	writeJSON(t, filepath.Join(globalDir, "settings.json"), &globalSettings)

	projectDir := t.TempDir()

	result, err := LoadLayeredSettings(projectDir)
	require.NoError(t, err)
	assert.Equal(t, "anthropic", result.Provider)
	assert.Equal(t, "medium", result.ThinkingLevel)
}

func TestLoadLayeredSettings_AllLayers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	globalSettings := Settings{
		ThinkingLevel: "medium",
		UI:            map[string]any{"theme": "dark"},
	}
	writeJSON(t, filepath.Join(globalDir, "settings.json"), &globalSettings)

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	localSettings := Settings{
		Model: "claude-opus-4-7",
		UI:    map[string]any{"editor_max_lines": 20},
	}
	writeJSON(t, filepath.Join(projectWeave, "settings.local.json"), &localSettings)

	result, err := LoadLayeredSettings(projectDir)
	require.NoError(t, err)

	assert.Equal(t, "medium", result.ThinkingLevel)
	assert.Equal(t, "claude-opus-4-7", result.Model)
	require.NotNil(t, result.UI)
	assert.Equal(t, "dark", result.UI["theme"])
	assert.InDelta(t, float64(20), result.UI["editor_max_lines"], 0)
}

func TestLoadLayeredSettings_GuardianAndSandbox(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	enabled := true
	writeJSON(t, filepath.Join(globalDir, "settings.json"), &Settings{
		Guardian: GuardianFileConfig{
			Profile: "ask",
			Profiles: map[string]sdk.GuardianProfile{
				"team": {Name: "team", Description: "global team profile"},
			},
		},
		Sandbox: SandboxFileConfig{
			Enabled: &enabled,
			Filesystem: SandboxFilesystemConfig{
				ReadOnly: []string{"/"},
			},
		},
	})

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	fail := true
	writeJSON(t, filepath.Join(projectWeave, "settings.local.json"), &Settings{
		Guardian: GuardianFileConfig{Profile: "team"},
		Sandbox: SandboxFileConfig{
			FailIfUnavailable: &fail,
			Filesystem: SandboxFilesystemConfig{
				ReadWrite: []string{"/tmp"},
			},
		},
	})

	result, err := LoadLayeredSettings(projectDir)
	require.NoError(t, err)

	assert.Equal(t, "team", result.Guardian.Profile)
	require.Contains(t, result.Guardian.Profiles, "team")
	require.NotNil(t, result.Sandbox.Enabled)
	assert.True(t, *result.Sandbox.Enabled)
	require.NotNil(t, result.Sandbox.FailIfUnavailable)
	assert.True(t, *result.Sandbox.FailIfUnavailable)
	assert.Equal(t, []string{"/"}, result.Sandbox.Filesystem.ReadOnly)
	assert.Equal(t, []string{"/tmp"}, result.Sandbox.Filesystem.ReadWrite)
}

func TestLoadLayeredSettings_LocalOverridesGlobal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	writeJSON(t, filepath.Join(globalDir, "settings.json"), &Settings{
		Provider:      "anthropic",
		ThinkingLevel: "medium",
	})

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	writeJSON(t, filepath.Join(projectWeave, "settings.local.json"), &Settings{
		Provider: "openai",
	})

	result, err := LoadLayeredSettings(projectDir)
	require.NoError(t, err)

	assert.Equal(t, "openai", result.Provider)
	assert.Equal(t, "medium", result.ThinkingLevel)
}

func TestLoadLayeredSettings_LocalOverridesGlobalValues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	writeJSON(t, filepath.Join(globalDir, "settings.json"), &Settings{
		ThinkingLevel: "medium",
		UI:            map[string]any{"editor_max_lines": 30},
	})

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	writeJSON(t, filepath.Join(projectWeave, "settings.local.json"), &Settings{
		ThinkingLevel: "high",
	})

	result, err := LoadLayeredSettings(projectDir)
	require.NoError(t, err)

	assert.Equal(t, "high", result.ThinkingLevel)
	require.NotNil(t, result.UI)
	assert.InDelta(t, float64(30), result.UI["editor_max_lines"], 0)
}

func TestLoadLayeredSettings_OnlyLocal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	// Only local settings, no project settings.json
	writeJSON(t, filepath.Join(projectWeave, "settings.local.json"), &Settings{
		ThinkingLevel: "high",
	})

	result, err := LoadLayeredSettings(projectDir)
	require.NoError(t, err)

	assert.Equal(t, "high", result.ThinkingLevel)
}

func TestLoadLayeredSettings_DoesNotReuseGlobalAsProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	writeJSON(t, filepath.Join(globalDir, "settings.json"), &Settings{
		Model: "global-model",
	})

	// Global-local settings that should NOT be treated as project-local
	writeJSON(t, filepath.Join(globalDir, "settings.local.json"), &Settings{
		Model: "global-local-model",
	})

	// Project under HOME with no project settings
	projectDir := filepath.Join(home, "myproject")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))

	result, err := LoadLayeredSettings(projectDir)
	require.NoError(t, err)

	// Global-local should not override via the project layer
	assert.Equal(t, "global-model", result.Model)
}

func TestMergeSettings_ProvidersDeepMerge(t *testing.T) {
	layer1 := &Settings{
		Providers: map[string]any{
			"anthropic": map[string]any{"model": "claude-opus-4-7"},
			"openai":    map[string]any{"base_url": "https://api.openai.com/v1"},
		},
	}
	layer2 := &Settings{
		Providers: map[string]any{
			"anthropic": map[string]any{"api_key": "sk-test"},
			"zai":       map[string]any{"model": "glm-5.1"},
		},
	}

	result := MergeSettings(layer1, layer2)

	require.Len(t, result.Providers, 3)
	anthropic := result.Providers["anthropic"].(map[string]any)
	assert.Equal(t, "claude-opus-4-7", anthropic["model"])
	assert.Equal(t, "sk-test", anthropic["api_key"])
	assert.Equal(t, "https://api.openai.com/v1", result.Providers["openai"].(map[string]any)["base_url"])
	assert.Equal(t, "glm-5.1", result.Providers["zai"].(map[string]any)["model"])
}

func TestSaveSettings_GlobalLayer(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	s := &Settings{Provider: "anthropic", Model: "opus"}
	err := SaveSettings(s, SettingsGlobal, "")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(home, ".weave", "settings.json"))
	require.NoError(t, err)

	var loaded Settings
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.Equal(t, "anthropic", loaded.Provider)
	assert.Equal(t, "opus", loaded.Model)
}

func TestSaveSettings_ProjectLayer(t *testing.T) {
	projectDir := t.TempDir()

	s := &Settings{Model: "gpt-5.5"}
	err := SaveSettings(s, SettingsProject, projectDir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(projectDir, ".weave", "settings.json"))
	require.NoError(t, err)

	var loaded Settings
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.Equal(t, "gpt-5.5", loaded.Model)
}

func TestSaveSettings_LocalLayer(t *testing.T) {
	projectDir := t.TempDir()

	s := &Settings{ThinkingLevel: "high"}
	err := SaveSettings(s, SettingsLocal, projectDir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(projectDir, ".weave", "settings.local.json"))
	require.NoError(t, err)

	var loaded Settings
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.Equal(t, "high", loaded.ThinkingLevel)
}

func TestSaveSettings_ConcurrentCallsDoNotCorruptFile(t *testing.T) {
	projectDir := t.TempDir()
	expected := []*Settings{
		{
			Provider:      "anthropic",
			Model:         "claude-opus-4-7",
			AgentLoop:     "agent-a",
			UIExtension:   "tui-a",
			ThinkingLevel: "medium",
			Tools: map[string]any{
				"bash": map[string]any{"timeout": 30},
				"read": map[string]any{"limit": 500},
			},
		},
		{
			Provider:      "openai",
			Model:         "gpt-5.5",
			AgentLoop:     "agent-b",
			UIExtension:   "tui-b",
			ThinkingLevel: "high",
			Extensions: map[string]any{
				"agent": map[string]any{"max_steps": 50},
				"jsonl": map[string]any{"enabled": true},
			},
		},
	}

	var wg sync.WaitGroup

	const writes = 50

	errs := make(chan error, writes)

	for i := range writes {
		s := expected[i%len(expected)]

		wg.Add(1)

		go func(s *Settings) {
			defer wg.Done()

			errs <- SaveSettings(s, SettingsProject, projectDir)
		}(s)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	data, err := os.ReadFile(filepath.Join(projectDir, ".weave", "settings.json"))
	require.NoError(t, err)

	var loaded Settings
	require.NoError(t, json.Unmarshal(data, &loaded))

	switch loaded.Provider {
	case "anthropic":
		assert.Equal(t, "claude-opus-4-7", loaded.Model)
		assert.Equal(t, "agent-a", loaded.AgentLoop)
		assert.Equal(t, "tui-a", loaded.UIExtension)
		assert.Equal(t, "medium", loaded.ThinkingLevel)
		assert.Equal(t, map[string]any{
			"bash": map[string]any{"timeout": float64(30)},
			"read": map[string]any{"limit": float64(500)},
		}, loaded.Tools)
		assert.Nil(t, loaded.Extensions)
	case "openai":
		assert.Equal(t, "gpt-5.5", loaded.Model)
		assert.Equal(t, "agent-b", loaded.AgentLoop)
		assert.Equal(t, "tui-b", loaded.UIExtension)
		assert.Equal(t, "high", loaded.ThinkingLevel)
		assert.Equal(t, map[string]any{
			"agent": map[string]any{"max_steps": float64(50)},
			"jsonl": map[string]any{"enabled": true},
		}, loaded.Extensions)
		assert.Nil(t, loaded.Tools)
	default:
		t.Fatalf("provider %q was not one complete expected write", loaded.Provider)
	}
}

func TestSaveSettings_ProjectRequiresDir(t *testing.T) {
	err := SaveSettings(&Settings{}, SettingsProject, "")
	assert.Error(t, err)
}

func TestSaveSettings_LocalRequiresDir(t *testing.T) {
	err := SaveSettings(&Settings{}, SettingsLocal, "")
	assert.Error(t, err)
}

func TestSaveSettingsGlobal_Convenience(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	s := &Settings{Provider: "zai"}
	err := SaveSettingsGlobal(s)
	require.NoError(t, err)

	loaded, err := LoadSettings()
	require.NoError(t, err)
	assert.Equal(t, "zai", loaded.Provider)
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()

	data, err := json.MarshalIndent(v, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func TestEnsureLocalSettingsExcluded_AddsEntry(t *testing.T) {
	projectDir := t.TempDir()
	gitDir := filepath.Join(projectDir, ".git")
	require.NoError(t, os.MkdirAll(filepath.Join(gitDir, "info"), 0o755))

	EnsureLocalSettingsExcluded(projectDir)

	data, err := os.ReadFile(filepath.Join(gitDir, "info", "exclude"))
	require.NoError(t, err)
	assert.Contains(t, string(data), ".weave/settings.local.json")
}

func TestEnsureLocalSettingsExcluded_SkipsExisting(t *testing.T) {
	projectDir := t.TempDir()
	gitDir := filepath.Join(projectDir, ".git")
	require.NoError(t, os.MkdirAll(filepath.Join(gitDir, "info"), 0o755))

	excludePath := filepath.Join(gitDir, "info", "exclude")
	require.NoError(t, os.WriteFile(excludePath, []byte(".weave/settings.local.json\n"), 0o644))

	EnsureLocalSettingsExcluded(projectDir)

	data, err := os.ReadFile(excludePath)
	require.NoError(t, err)
	assert.Equal(t, ".weave/settings.local.json\n", string(data))
}

func TestEnsureLocalSettingsExcluded_NoGitRepo(t *testing.T) {
	projectDir := t.TempDir()
	// Should not panic or create anything
	EnsureLocalSettingsExcluded(projectDir)

	_, err := os.Stat(filepath.Join(projectDir, ".git"))
	assert.True(t, os.IsNotExist(err))
}

func TestEnsureLocalSettingsExcluded_WalksUpToGitRoot(t *testing.T) {
	projectDir := t.TempDir()
	gitDir := filepath.Join(projectDir, ".git")
	require.NoError(t, os.MkdirAll(filepath.Join(gitDir, "info"), 0o755))

	subDir := filepath.Join(projectDir, "src", "pkg")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	EnsureLocalSettingsExcluded(subDir)

	data, err := os.ReadFile(filepath.Join(gitDir, "info", "exclude"))
	require.NoError(t, err)
	assert.Contains(t, string(data), ".weave/settings.local.json")
}
