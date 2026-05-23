package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weave-agent/weave/sdk"
)

type configTestExtension struct{}

func (configTestExtension) Name() string { return "config-test" }
func (configTestExtension) Subscribe(sdk.Bus) error {
	return nil
}
func (configTestExtension) Close() error { return nil }

func isolateHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func TestSettingsWeaveFlags(t *testing.T) {
	tests := []struct {
		name     string
		settings Settings
		want     []string
	}{
		{
			name:     "no flags",
			settings: Settings{},
			want:     nil,
		},
		{
			name:     "output flag",
			settings: Settings{Output: "json"},
			want:     []string{"--weave-output=json"},
		},
		{
			name:     "tools flag",
			settings: Settings{ToolsFlag: "read,grep"},
			want:     []string{"--weave-tools=read,grep"},
		},
		{
			name:     "empty tools flag with ToolsSet",
			settings: Settings{ToolsFlag: "", ToolsSet: true},
			want:     []string{"--weave-tools="},
		},
		{
			name:     "subagent id flag",
			settings: Settings{SubagentID: "abc123"},
			want:     []string{"--weave-subagent-id=abc123"},
		},
		{
			name:     "guardian profile flag",
			settings: Settings{GuardianProfile: "auto"},
			want:     []string{"--weave-guardian-profile=auto"},
		},
		{
			name:     "model flag",
			settings: Settings{ModelFlag: "claude-sonnet-4-6"},
			want:     []string{"--weave-model=claude-sonnet-4-6"},
		},
		{
			name:     "debug flag",
			settings: Settings{Debug: true},
			want:     []string{"--weave-debug=true"},
		},
		{
			name: "multiple flags",
			settings: Settings{
				Output:          "json",
				ToolsFlag:       "read",
				SubagentID:      "id1",
				GuardianProfile: "yolo",
				ModelFlag:       "gpt-5.5",
				Debug:           true,
			},
			want: []string{
				"--weave-debug=true",
				"--weave-output=json",
				"--weave-tools=read",
				"--weave-subagent-id=id1",
				"--weave-guardian-profile=yolo",
				"--weave-model=gpt-5.5",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.settings.WeaveFlags()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtensionConfig_ToolPopulatedStruct(t *testing.T) {
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
	writeJSON(t, filepath.Join(projectWeave, "settings.local.json"), &settings)

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "settings.json"),
		settings: DefaultSettings(),
	}

	var target struct {
		Timeout int `json:"timeout"`
	}
	require.NoError(t, cfg.ExtensionConfig("tools", "bash", &target))
	assert.Equal(t, 60, target.Timeout)
}

func TestExtensionConfig_ToolDefaultsApplied(t *testing.T) {
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
	writeJSON(t, filepath.Join(projectWeave, "settings.local.json"), &settings)

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "settings.json"),
		settings: DefaultSettings(),
	}

	var target struct {
		Timeout int `json:"timeout" default:"120"`
	}
	require.NoError(t, cfg.ExtensionConfig("tools", "bash", &target))
	assert.Equal(t, 120, target.Timeout)
}

func TestExtensionConfig_ToolMissingSection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "settings.json"),
		settings: DefaultSettings(),
	}

	var target struct {
		Timeout int `json:"timeout" default:"42"`
	}
	require.NoError(t, cfg.ExtensionConfig("tools", "bash", &target))
	assert.Equal(t, 42, target.Timeout, "default should be applied when no settings file exists")
}

func TestExtensionConfig_ToolMissingToolName(t *testing.T) {
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
	writeJSON(t, filepath.Join(projectWeave, "settings.local.json"), &settings)

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "settings.json"),
		settings: DefaultSettings(),
	}

	var target struct {
		Timeout int `json:"timeout" default:"99"`
	}
	require.NoError(t, cfg.ExtensionConfig("tools", "bash", &target))
	assert.Equal(t, 99, target.Timeout, "default should be applied when tool not in settings")
}

func TestExtensionConfig_UIPopulatedStruct(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	settings := Settings{
		UI: map[string]any{
			"theme":            "dark",
			"editor_max_lines": 40,
		},
	}
	writeJSON(t, filepath.Join(projectWeave, "settings.local.json"), &settings)

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "settings.json"),
		settings: DefaultSettings(),
	}

	var target struct {
		Theme          string `json:"theme,omitempty"`
		EditorMaxLines int    `json:"editor_max_lines,omitempty"`
	}
	require.NoError(t, cfg.ExtensionConfig("ui", "", &target))
	assert.Equal(t, "dark", target.Theme)
	assert.Equal(t, 40, target.EditorMaxLines)
}

func TestExtensionConfig_UIDefaultsApplied(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o750))

	settings := Settings{
		UI: map[string]any{},
	}
	writeJSON(t, filepath.Join(projectWeave, "settings.local.json"), &settings)

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "settings.json"),
		settings: DefaultSettings(),
	}

	var target struct {
		Theme          string `json:"theme" default:"dark"`
		EditorMaxLines int    `json:"editor_max_lines" default:"15"`
	}
	require.NoError(t, cfg.ExtensionConfig("ui", "", &target))
	assert.Equal(t, "dark", target.Theme)
	assert.Equal(t, 15, target.EditorMaxLines)
}

func TestExtensionConfig_UIMissingSection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	globalDir := filepath.Join(home, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))

	projectDir := t.TempDir()

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "settings.json"),
		settings: DefaultSettings(),
	}

	var target struct {
		Theme string `json:"theme" default:"dark"`
	}
	require.NoError(t, cfg.ExtensionConfig("ui", "", &target))
	assert.Equal(t, "dark", target.Theme, "default should be applied when no UI settings")
}

func TestApplyDefaults_StringField(t *testing.T) {
	var s struct {
		Name string `default:"unnamed"`
	}
	require.NoError(t, applyDefaults(&s))
	assert.Equal(t, "unnamed", s.Name)
}

func TestApplyDefaults_IntField(t *testing.T) {
	var s struct {
		Port int `default:"8080"`
	}
	require.NoError(t, applyDefaults(&s))
	assert.Equal(t, 8080, s.Port)
}

func TestApplyDefaults_BoolField(t *testing.T) {
	var s struct {
		Verbose bool `default:"true"`
	}
	require.NoError(t, applyDefaults(&s))
	assert.True(t, s.Verbose)
}

func TestApplyDefaults_SkipsNonZero(t *testing.T) {
	s := struct {
		Name string `default:"unnamed"`
	}{Name: "custom"}
	require.NoError(t, applyDefaults(&s))
	assert.Equal(t, "custom", s.Name)
}

func TestApplyDefaults_SkipsNoTag(t *testing.T) {
	var s struct {
		Name string
	}
	require.NoError(t, applyDefaults(&s))
	assert.Empty(t, s.Name)
}

func TestApplyDefaults_NilPointer(t *testing.T) {
	// Should not panic on nil pointer.
	require.NoError(t, applyDefaults(nil))

	var p *struct {
		Name string `default:"unnamed"`
	}
	require.NoError(t, applyDefaults(p))
}

func TestLoader_RoundTrip(t *testing.T) {
	raw := map[string]any{
		"timeout": float64(30),
		"name":    "test",
	}

	var target struct {
		Timeout int    `json:"timeout"`
		Name    string `json:"name"`
	}

	loader := Loader{Data: raw}
	require.NoError(t, loader.Load(&target))
	assert.Equal(t, 30, target.Timeout)
	assert.Equal(t, "test", target.Name)
}

func TestLayeredSettings_IntegrationWithExtensionConfig(t *testing.T) {
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

	// Local overrides bash timeout to 30
	writeJSON(t, filepath.Join(projectWeave, "settings.local.json"), &Settings{
		Tools: map[string]any{
			"bash": map[string]any{"timeout": 30},
		},
	})

	cfg := &FullConfig{
		filePath: filepath.Join(projectDir, ".weave", "settings.json"),
		settings: DefaultSettings(),
	}

	var target struct {
		Timeout int `json:"timeout"`
	}
	require.NoError(t, cfg.ExtensionConfig("tools", "bash", &target))
	assert.Equal(t, 30, target.Timeout, "local layer should override global")
}

func TestExtensionConfig_ToolLocalOnlyFromWeaveDir(t *testing.T) {
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
		filePath: filepath.Join(projectDir, ".weave", "settings.json"),
		settings: DefaultSettings(),
	}

	var target struct {
		Timeout int `json:"timeout" default:"120"`
	}
	require.NoError(t, cfg.ExtensionConfig("tools", "bash", &target))
	assert.Equal(t, 45, target.Timeout, "local settings should be found when config is inside .weave/")
}

func TestExtensionConfig_SandboxScope(t *testing.T) {
	isolateHome(t)

	dir := t.TempDir()
	path := filepath.Join(dir, ".weave", "settings.json")
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui","sandbox":{"enabled":true,"fail_if_unavailable":true,"filesystem":{"read_write":["/tmp"]},"network":{"enabled":false}}}`)

	cfg := &FullConfig{
		filePath: path,
		settings: mustLoadSettings(t, path),
	}

	var target struct {
		Enabled           bool                    `json:"enabled"`
		FailIfUnavailable bool                    `json:"fail_if_unavailable"`
		Filesystem        SandboxFilesystemConfig `json:"filesystem"`
		Network           SandboxNetworkConfig    `json:"network"`
	}
	require.NoError(t, cfg.ExtensionConfig("sandbox", "", &target))
	assert.True(t, target.Enabled)
	assert.True(t, target.FailIfUnavailable)
	assert.Equal(t, []string{"/tmp"}, target.Filesystem.ReadWrite)
	require.NotNil(t, target.Network.Enabled)
	assert.False(t, *target.Network.Enabled)
}

func TestExtensionConfig_GuardianScope(t *testing.T) {
	isolateHome(t)

	dir := t.TempDir()
	path := filepath.Join(dir, ".weave", "settings.json")
	writeFile(t, dir, ".weave/settings.json", `{
		"ui_extension":"tui",
		"guardian":{
			"profile":"team",
			"ask_fallback":true,
			"profiles":{
				"team":{
					"name":"team",
					"description":"team defaults",
					"rules":[{"actions":["write"],"decision":"ask"}]
				}
			}
		}
	}`)

	cfg := &FullConfig{
		filePath: path,
		settings: mustLoadSettings(t, path),
	}

	var target GuardianFileConfig
	require.NoError(t, cfg.ExtensionConfig("guardian", "", &target))
	assert.Equal(t, "team", target.Profile)
	require.NotNil(t, target.AskFallback)
	assert.True(t, *target.AskFallback)
	require.Contains(t, target.Profiles, "team")
	assert.Equal(t, sdk.GuardianDecisionAsk, target.Profiles["team"].Rules[0].Decision)
}

func TestExtensionConfig_GuardianScopeRegisteredExtensionUsesRootEnv(t *testing.T) {
	isolateHome(t)
	sdk.ResetExtensionRegistry()
	t.Cleanup(sdk.ResetExtensionRegistry)
	t.Setenv("WEAVE_GUARDIAN_PROFILE", "env-team")
	t.Setenv("WEAVE_GUARDIAN_ASK_FALLBACK", "true")

	dir := t.TempDir()
	path := filepath.Join(dir, ".weave", "settings.json")
	writeFile(t, dir, ".weave/settings.json", `{
		"guardian":{
			"profile":"file-team",
			"profiles":{
				"file-team":{"name":"file-team","description":"from file"}
			}
		}
	}`)

	cfg := &FullConfig{
		filePath: path,
		settings: mustLoadSettings(t, path),
	}

	var got GuardianFileConfig

	sdk.RegisterExtensionWithScope("guardian", "guardian", func(_ sdk.Config, _ sdk.PreferenceReader, c GuardianFileConfig) (sdk.Extension, error) {
		got = c
		return configTestExtension{}, nil
	})

	_, err := sdk.GetExtension("guardian", cfg)
	require.NoError(t, err)

	assert.Equal(t, "env-team", got.Profile)
	require.NotNil(t, got.AskFallback)
	assert.True(t, *got.AskFallback)
	require.Contains(t, got.Profiles, "file-team")
}

func TestExtensionConfig_GuardianScopeRegisteredExtensionUsesCLI(t *testing.T) {
	isolateHome(t)
	sdk.ResetExtensionRegistry()
	t.Cleanup(sdk.ResetExtensionRegistry)

	dir := t.TempDir()
	path := filepath.Join(dir, ".weave", "settings.json")
	writeFile(t, dir, ".weave/settings.json", `{
		"guardian":{
			"profile":"file-team",
			"ask_fallback":true
		}
	}`)

	cfg := &FullConfig{
		filePath: path,
		settings: mustLoadSettings(t, path),
	}
	cfg.SetArgs([]string{"--guardian-profile", "cli-team", "--guardian-ask_fallback", "false"})

	var got GuardianFileConfig

	sdk.RegisterExtensionWithScope("guardian", "guardian", func(_ sdk.Config, _ sdk.PreferenceReader, c GuardianFileConfig) (sdk.Extension, error) {
		got = c
		return configTestExtension{}, nil
	})

	_, err := sdk.GetExtension("guardian", cfg)
	require.NoError(t, err)

	assert.Equal(t, "cli-team", got.Profile)
	require.NotNil(t, got.AskFallback)
	assert.False(t, *got.AskFallback)
}

func TestExtensionConfig_GuardianScopeRejectsInvalidBoolCLI(t *testing.T) {
	isolateHome(t)

	dir := t.TempDir()
	path := filepath.Join(dir, ".weave", "settings.json")
	writeFile(t, dir, ".weave/settings.json", `{"guardian":{"ask_fallback":true}}`)

	cfg := &FullConfig{
		filePath: path,
		settings: mustLoadSettings(t, path),
	}
	cfg.SetArgs([]string{"--guardian-ask_fallback=not-bool"})

	var target GuardianFileConfig

	err := cfg.ExtensionConfig("guardian", "", &target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid boolean value")
}

func TestExtensionConfig_SandboxScopeRegisteredExtensionUsesRootEnv(t *testing.T) {
	isolateHome(t)
	sdk.ResetExtensionRegistry()
	t.Cleanup(sdk.ResetExtensionRegistry)
	t.Setenv("WEAVE_SANDBOX_ENABLED", "false")
	t.Setenv("WEAVE_SANDBOX_FAIL_IF_UNAVAILABLE", "true")
	t.Setenv("WEAVE_SANDBOX_ALLOW_UNSANDBOXED_FALLBACK", "false")

	dir := t.TempDir()
	path := filepath.Join(dir, ".weave", "settings.json")
	writeFile(t, dir, ".weave/settings.json", `{
		"sandbox":{
			"enabled":true,
			"allow_unsandboxed_fallback":true,
			"filesystem":{"read_only":["/repo"]}
		}
	}`)

	cfg := &FullConfig{
		filePath: path,
		settings: mustLoadSettings(t, path),
	}

	var got SandboxFileConfig

	sdk.RegisterExtensionWithScope("sandbox", "sandbox", func(_ sdk.Config, _ sdk.PreferenceReader, c SandboxFileConfig) (sdk.Extension, error) {
		got = c
		return configTestExtension{}, nil
	})

	_, err := sdk.GetExtension("sandbox", cfg)
	require.NoError(t, err)

	require.NotNil(t, got.Enabled)
	assert.False(t, *got.Enabled)
	require.NotNil(t, got.FailIfUnavailable)
	assert.True(t, *got.FailIfUnavailable)
	require.NotNil(t, got.AllowUnsandboxedFallback)
	assert.False(t, *got.AllowUnsandboxedFallback)
	assert.Equal(t, []string{"/repo"}, got.Filesystem.ReadOnly)
}

func TestExtensionConfig_SandboxScopeRegisteredExtensionUsesCLI(t *testing.T) {
	isolateHome(t)
	sdk.ResetExtensionRegistry()
	t.Cleanup(sdk.ResetExtensionRegistry)

	dir := t.TempDir()
	path := filepath.Join(dir, ".weave", "settings.json")
	writeFile(t, dir, ".weave/settings.json", `{
		"sandbox":{
			"enabled":true,
			"fail_if_unavailable":false,
			"allow_unsandboxed_fallback":true
		}
	}`)

	cfg := &FullConfig{
		filePath: path,
		settings: mustLoadSettings(t, path),
	}
	cfg.SetArgs([]string{
		"--sandbox-enabled", "false",
		"--sandbox-fail_if_unavailable=true",
		"--sandbox-allow_unsandboxed_fallback=false",
	})

	var got SandboxFileConfig

	sdk.RegisterExtensionWithScope("sandbox", "sandbox", func(_ sdk.Config, _ sdk.PreferenceReader, c SandboxFileConfig) (sdk.Extension, error) {
		got = c
		return configTestExtension{}, nil
	})

	_, err := sdk.GetExtension("sandbox", cfg)
	require.NoError(t, err)

	require.NotNil(t, got.Enabled)
	assert.False(t, *got.Enabled)
	require.NotNil(t, got.FailIfUnavailable)
	assert.True(t, *got.FailIfUnavailable)
	require.NotNil(t, got.AllowUnsandboxedFallback)
	assert.False(t, *got.AllowUnsandboxedFallback)
}

func TestExtensionConfig_SandboxScopeRejectsInvalidBoolCLI(t *testing.T) {
	isolateHome(t)

	for _, flag := range []string{
		"--sandbox-enabled=not-bool",
		"--sandbox-fail_if_unavailable=not-bool",
		"--sandbox-allow_unsandboxed_fallback=not-bool",
	} {
		t.Run(flag, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".weave", "settings.json")
			writeFile(t, dir, ".weave/settings.json", `{"sandbox":{"enabled":true}}`)

			cfg := &FullConfig{
				filePath: path,
				settings: mustLoadSettings(t, path),
			}
			cfg.SetArgs([]string{flag})

			var target SandboxFileConfig

			err := cfg.ExtensionConfig("sandbox", "", &target)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid boolean value")
		})
	}
}

func TestExtensionConfig_JSONLScope(t *testing.T) {
	isolateHome(t)

	dir := t.TempDir()
	path := filepath.Join(dir, ".weave", "settings.json")
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui","jsonl":{"dir":"/custom/sessions"}}`)

	cfg := &FullConfig{
		filePath: path,
		settings: mustLoadSettings(t, path),
	}

	var target struct {
		Dir string `json:"dir" default:"~/.weave/sessions"`
	}
	require.NoError(t, cfg.ExtensionConfig("jsonl", "", &target))
	assert.Equal(t, "/custom/sessions", target.Dir)
}

func TestExtensionConfig_DerivesEnvPrefixForTools(t *testing.T) {
	isolateHome(t)
	t.Setenv("WEAVE_BASH_TIMEOUT", "77")

	dir := t.TempDir()
	path := filepath.Join(dir, ".weave", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	writeFile(t, dir, ".weave/settings.json", `{}`)

	cfg := &FullConfig{
		filePath: path,
		settings: mustLoadSettings(t, path),
	}

	var target struct {
		Timeout int `json:"timeout" env:"TIMEOUT"`
	}
	require.NoError(t, cfg.ExtensionConfig("tools", "bash", &target))
	assert.Equal(t, 77, target.Timeout, "tool env prefix should be WEAVE_BASH_")
}

func TestExtensionConfig_DerivesEnvPrefixForProviders(t *testing.T) {
	isolateHome(t)
	t.Setenv("ANTHROPIC_MODEL", "claude-test")

	dir := t.TempDir()
	path := filepath.Join(dir, ".weave", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	writeFile(t, dir, ".weave/settings.json", `{}`)

	cfg := &FullConfig{
		filePath: path,
		settings: mustLoadSettings(t, path),
	}

	var target struct {
		Model string `json:"model" env:"ANTHROPIC_MODEL"`
	}
	require.NoError(t, cfg.ExtensionConfig("providers", "anthropic", &target))
	assert.Equal(t, "claude-test", target.Model, "provider env prefix should be empty")
}

func TestExtensionConfig_DerivesEnvPrefixWithHyphens(t *testing.T) {
	isolateHome(t)
	t.Setenv("WEAVE_MY_TOOL_TIMEOUT", "99")

	dir := t.TempDir()
	path := filepath.Join(dir, ".weave", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	writeFile(t, dir, ".weave/settings.json", `{}`)

	cfg := &FullConfig{
		filePath: path,
		settings: mustLoadSettings(t, path),
	}

	var target struct {
		Timeout int `json:"timeout" env:"TIMEOUT"`
	}
	require.NoError(t, cfg.ExtensionConfig("tools", "my-tool", &target))
	assert.Equal(t, 99, target.Timeout, "hyphens in name should become underscores in prefix")
}

func TestProjectDirFromConfig(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"settings.json at root", "/project/.weave/settings.json", "/project"},
		{"settings.json inside weave", "/project/.weave/settings.json", "/project"},
		{"json inside weave", "/project/.weave/settings.json", "/project"},
		{"nested project", "/a/b/c/.weave/settings.json", "/a/b/c"},
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
		UI:            map[string]any{"theme": "dark", "editor_max_lines": 40},
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
	assert.Equal(t, "dark", back.UI["theme"])
	assert.InDelta(t, float64(40), back.UI["editor_max_lines"], 0)
	assert.Equal(t, map[string]any{"timeout": float64(60)}, back.Tools["bash"])
}
