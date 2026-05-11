package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolConfig_PopulatedStruct(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	settings := Settings{
		Tools: map[string]any{
			"bash": map[string]any{"timeout": 60},
		},
	}
	writeJSON(t, filepath.Join(projectWeave, "settings.json"), &settings)

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "config.json"),
		file:     DefaultFile(),
		auth:     &AuthFile{},
	}

	var target struct {
		Timeout int `json:"timeout"`
	}
	require.NoError(t, cfg.ToolConfig("bash", &target))
	assert.Equal(t, 60, target.Timeout)
}

func TestToolConfig_DefaultsApplied(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	settings := Settings{
		Tools: map[string]any{
			"bash": map[string]any{},
		},
	}
	writeJSON(t, filepath.Join(projectWeave, "settings.json"), &settings)

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "config.json"),
		file:     DefaultFile(),
		auth:     &AuthFile{},
	}

	var target struct {
		Timeout int `json:"timeout" default:"120"`
	}
	require.NoError(t, cfg.ToolConfig("bash", &target))
	assert.Equal(t, 120, target.Timeout)
}

func TestToolConfig_MissingSection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "config.json"),
		file:     DefaultFile(),
		auth:     &AuthFile{},
	}

	var target struct {
		Timeout int `json:"timeout" default:"42"`
	}
	require.NoError(t, cfg.ToolConfig("bash", &target))
	assert.Equal(t, 42, target.Timeout, "default should be applied when no settings file exists")
}

func TestToolConfig_MissingToolName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	settings := Settings{
		Tools: map[string]any{
			"grep": map[string]any{"context": 3},
		},
	}
	writeJSON(t, filepath.Join(projectWeave, "settings.json"), &settings)

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "config.json"),
		file:     DefaultFile(),
		auth:     &AuthFile{},
	}

	var target struct {
		Timeout int `json:"timeout" default:"99"`
	}
	require.NoError(t, cfg.ToolConfig("bash", &target))
	assert.Equal(t, 99, target.Timeout, "default should be applied when tool not in settings")
}

func TestUIConfig_PopulatedStruct(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	settings := Settings{
		UI: &UISettings{
			Theme:          "dark",
			EditorMaxLines: 40,
		},
	}
	writeJSON(t, filepath.Join(projectWeave, "settings.json"), &settings)

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "config.json"),
		file:     DefaultFile(),
		auth:     &AuthFile{},
	}

	var target UISettings
	require.NoError(t, cfg.UIConfig(&target))
	assert.Equal(t, "dark", target.Theme)
	assert.Equal(t, 40, target.EditorMaxLines)
}

func TestUIConfig_DefaultsApplied(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	settings := Settings{
		UI: &UISettings{},
	}
	writeJSON(t, filepath.Join(projectWeave, "settings.json"), &settings)

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "config.json"),
		file:     DefaultFile(),
		auth:     &AuthFile{},
	}

	var target struct {
		Theme          string `json:"theme" default:"dark"`
		EditorMaxLines int    `json:"editor_max_lines" default:"15"`
	}
	require.NoError(t, cfg.UIConfig(&target))
	assert.Equal(t, "dark", target.Theme)
	assert.Equal(t, 15, target.EditorMaxLines)
}

func TestUIConfig_MissingSection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "config.json"),
		file:     DefaultFile(),
		auth:     &AuthFile{},
	}

	var target struct {
		Theme string `json:"theme" default:"dark"`
	}
	require.NoError(t, cfg.UIConfig(&target))
	assert.Equal(t, "dark", target.Theme, "default should be applied when no UI settings")
}

func TestApplyDefaults_StringField(t *testing.T) {
	var s struct {
		Name string `default:"unnamed"`
	}
	applyDefaults(&s)
	assert.Equal(t, "unnamed", s.Name)
}

func TestApplyDefaults_IntField(t *testing.T) {
	var s struct {
		Port int `default:"8080"`
	}
	applyDefaults(&s)
	assert.Equal(t, 8080, s.Port)
}

func TestApplyDefaults_BoolField(t *testing.T) {
	var s struct {
		Verbose bool `default:"true"`
	}
	applyDefaults(&s)
	assert.True(t, s.Verbose)
}

func TestApplyDefaults_SkipsNonZero(t *testing.T) {
	s := struct {
		Name string `default:"unnamed"`
	}{Name: "custom"}
	applyDefaults(&s)
	assert.Equal(t, "custom", s.Name)
}

func TestApplyDefaults_SkipsNoTag(t *testing.T) {
	var s struct {
		Name string
	}
	applyDefaults(&s)
	assert.Empty(t, s.Name)
}

func TestApplyDefaults_NilPointer(t *testing.T) {
	// Should not panic on nil pointer.
	applyDefaults(nil)

	var p *struct {
		Name string `default:"unnamed"`
	}
	applyDefaults(p)
}

func TestPopulateConfig_RoundTrip(t *testing.T) {
	raw := map[string]any{
		"timeout": float64(30),
		"name":    "test",
	}

	var target struct {
		Timeout int    `json:"timeout"`
		Name    string `json:"name"`
	}
	require.NoError(t, populateConfig(raw, &target))
	assert.Equal(t, 30, target.Timeout)
	assert.Equal(t, "test", target.Name)
}

func TestLayeredSettings_IntegrationWithToolConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	// Global sets bash timeout to 120
	writeJSON(t, filepath.Join(globalDir, "settings.json"), &Settings{
		Tools: map[string]any{
			"bash": map[string]any{"timeout": 120},
		},
	})

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	// Project overrides bash timeout to 60
	writeJSON(t, filepath.Join(projectWeave, "settings.json"), &Settings{
		Tools: map[string]any{
			"bash": map[string]any{"timeout": 60},
		},
	})

	// Local overrides bash timeout to 30
	writeJSON(t, filepath.Join(projectWeave, "settings.local.json"), &Settings{
		Tools: map[string]any{
			"bash": map[string]any{"timeout": 30},
		},
	})

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "config.json"),
		file:     DefaultFile(),
		auth:     &AuthFile{},
	}

	var target struct {
		Timeout int `json:"timeout"`
	}
	require.NoError(t, cfg.ToolConfig("bash", &target))
	assert.Equal(t, 30, target.Timeout, "local layer should override global and project")
}

func TestToolConfig_LocalOnlyFromWeaveDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	// Only local settings, no project settings — config file is inside .weave/
	writeJSON(t, filepath.Join(projectWeave, "settings.local.json"), &Settings{
		Tools: map[string]any{
			"bash": map[string]any{"timeout": 45},
		},
	})

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "config.json"),
		file:     DefaultFile(),
		auth:     &AuthFile{},
	}

	var target struct {
		Timeout int `json:"timeout" default:"120"`
	}
	require.NoError(t, cfg.ToolConfig("bash", &target))
	assert.Equal(t, 45, target.Timeout, "local settings should be found when config is inside .weave/")
}

func TestProjectDirFromConfig(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"config.json at root", "/project/.weave/config.json", "/project"},
		{"config.json inside weave", "/project/.weave/config.json", "/project"},
		{"json inside weave", "/project/.weave/config.json", "/project"},
		{"nested project", "/a/b/c/.weave/config.json", "/a/b/c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ProjectDirFromConfig(tt.path))
		})
	}
}

// Verify the JSON key in Settings matches what FullConfig reads.
func TestSettingsJSONRoundTrip(t *testing.T) {
	s := &Settings{
		Provider:      "anthropic",
		Model:         "opus",
		ThinkingLevel: "high",
		UI:            &UISettings{Theme: "dark", EditorMaxLines: 40},
		Tools:         map[string]any{"bash": map[string]any{"timeout": 60}},
	}

	data, err := json.MarshalIndent(s, "", "  ")
	require.NoError(t, err)

	var back Settings
	require.NoError(t, json.Unmarshal(data, &back))

	assert.Equal(t, s.Provider, back.Provider)
	assert.Equal(t, s.Model, back.Model)
	assert.Equal(t, s.ThinkingLevel, back.ThinkingLevel)
	require.NotNil(t, back.UI)
	assert.Equal(t, "dark", back.UI.Theme)
	assert.Equal(t, 40, back.UI.EditorMaxLines)
	assert.Equal(t, map[string]any{"timeout": float64(60)}, back.Tools["bash"])
}
