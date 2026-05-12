package wire

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"weave/settings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunFlagParsing(t *testing.T) {
	dir := t.TempDir()

	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))

	defer func() { _ = os.Chdir(origWd) }()

	tests := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{"invalid flag", []string{"-xyz"}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantCode, run(context.Background(), tt.args...))
		})
	}
}

func TestRunMissingConfig(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))

	defer func() { _ = os.Chdir(origWd) }()

	assert.Equal(t, 1, run(context.Background()))
}

func TestRunCoreDefaultsUsed(t *testing.T) {
	dir := t.TempDir()

	cfgFile := dir + "/.weave/settings.json"
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgFile), 0o750))
	require.NoError(t, os.WriteFile(cfgFile, []byte(`{"ui_extension":"tui","agent_loop":"loop"}`), 0o600))

	_, cf, _, err := settings.LoadFromDir(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, "loop", cf.AgentLoop, "default agent_loop should be 'loop'")
	assert.Equal(t, "tui", cf.UIExtension, "default ui should be 'tui'")
}

func TestValidateCoreConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *settings.Settings
		wantErr error
	}{
		{
			"valid defaults",
			&settings.Settings{AgentLoop: "loop", UIExtension: "tui"},
			nil,
		},
		{
			"empty agent_loop",
			&settings.Settings{AgentLoop: "", UIExtension: "tui"},
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := settings.Validate(tt.config)
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr.Error())
			}
		})
	}
}

func TestRun_InstallSubcommand(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "test-ext")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "go.mod"), []byte("module test/ext\n\ngo 1.22\n"), 0o600))

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	code := Run(context.Background(), []string{"install", extDir})
	assert.Equal(t, 0, code)
}

func TestRun_DefaultRoute(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))

	defer func() { _ = os.Chdir(origWd) }()

	code := Run(context.Background(), []string{"-xyz"})
	assert.Equal(t, 1, code)
}

func TestFindModuleRoot(t *testing.T) {
	_, err := findModuleRoot()
	require.NoError(t, err, "should find module root from the weave project directory")
}

func TestFindModuleRootFrom_Invalid(t *testing.T) {
	_, err := findModuleRootFrom(func() (string, error) {
		return "/nonexistent/path/that/does/not/exist", nil
	})
	require.Error(t, err)
}

func TestIsWeaveModule(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module weave\n\ngo 1.24\n"), 0o600))
	assert.True(t, isWeaveModule(dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module other\n\ngo 1.24\n"), 0o600))
	assert.False(t, isWeaveModule(dir))
}

func TestWritePromptFile(t *testing.T) {
	path, cleanup, ok := writePromptFile("hello world")
	require.True(t, ok)
	require.NotEmpty(t, path)

	defer cleanup()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(data))
}

func TestResolveProjectDir(t *testing.T) {
	assert.Equal(t, "/project", settings.ProjectDirFromConfig("/project/.weave/settings.json"))
	assert.Equal(t, "/project", settings.ProjectDirFromConfig("/project/settings.json"))
	assert.Equal(t, "/", settings.ProjectDirFromConfig("/.weave/settings.json"))
}

func TestRun_SubagentFlagsParsed(t *testing.T) {
	dir := t.TempDir()
	cfgFile := dir + "/.weave/settings.json"
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgFile), 0o750))
	require.NoError(t, os.WriteFile(cfgFile, []byte(`{"ui_extension":"none","agent_loop":"loop"}`), 0o600))

	_, cf, rest, err := settings.LoadFromDir(dir, []string{
		"-p", "test",
		"--output", "json",
		"--tools", "read,grep",
		"--subagent-id", "abc123",
		"--sandbox", "readonly",
		"--model", "claude-haiku-4-5",
	})
	require.NoError(t, err)

	assert.Equal(t, "json", cf.Output)
	assert.Equal(t, "read,grep", cf.ToolsFlag)
	assert.True(t, cf.ToolsSet)
	assert.Equal(t, "abc123", cf.SubagentID)
	assert.Equal(t, "readonly", cf.SandboxMode)
	assert.Equal(t, "claude-haiku-4-5", cf.ModelFlag)
	assert.Empty(t, rest, "all flags should be consumed by loader")
}

func TestRun_EmptyToolsFlagForwarded(t *testing.T) {
	dir := t.TempDir()
	cfgFile := dir + "/.weave/settings.json"
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgFile), 0o750))
	require.NoError(t, os.WriteFile(cfgFile, []byte(`{"ui_extension":"none","agent_loop":"loop"}`), 0o600))

	_, cf, rest, err := settings.LoadFromDir(dir, []string{
		"-p", "test",
		"--tools=",
	})
	require.NoError(t, err)

	assert.Empty(t, cf.ToolsFlag)
	assert.True(t, cf.ToolsSet, "explicit --tools= should set ToolsSet")
	assert.Empty(t, rest, "all flags should be consumed by loader")
}

func TestRun_ProjectDirFromConfig(t *testing.T) {
	// Create a project structure: /tmp/project/.weave/settings.json and /tmp/project/subdir/
	projectDir := t.TempDir()
	subDir := filepath.Join(projectDir, "subdir")
	require.NoError(t, os.MkdirAll(subDir, 0o750))

	cfgFile := filepath.Join(projectDir, ".weave", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgFile), 0o750))
	require.NoError(t, os.WriteFile(cfgFile, []byte(`{"ui_extension":"none","agent_loop":"loop"}`), 0o600))

	// Loading from subdir should find the config at project root.
	_, cf, _, err := settings.LoadFromDir(subDir, []string{"-p", "hello"})
	require.NoError(t, err)
	assert.Equal(t, projectDir, settings.ProjectDirFromConfig(cfgFile))

	// Verify the config was found and parsed.
	assert.Equal(t, "none", cf.UIExtension)
}

func TestRun_ProjectDirNotUsedForGlobalConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// Create a global config but no project settings.
	globalDir := filepath.Join(homeDir, ".weave")
	require.NoError(t, os.MkdirAll(globalDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "settings.json"), []byte("{}\n"), 0o600))

	globalConfigPath := filepath.Join(globalDir, "settings.json")

	// Verify the global config path is detected as being inside the global dir.
	globalDirResult, _ := settings.GlobalConfigDir()
	require.NotEmpty(t, globalDirResult)
	assert.True(t, strings.HasPrefix(globalConfigPath, globalDirResult+string(os.PathSeparator)),
		"global config path should be inside the global config directory")

	// When a project-local config exists, it should resolve to the project dir.
	projectDir := t.TempDir()
	localConfigPath := filepath.Join(projectDir, ".weave", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(localConfigPath), 0o750))
	require.NoError(t, os.WriteFile(localConfigPath, []byte("{}\n"), 0o600))

	// Project-local config should NOT be inside the global dir.
	assert.False(t, strings.HasPrefix(localConfigPath, globalDirResult+string(os.PathSeparator)),
		"project-local config should not be classified as global")

	// Verify ProjectDirFromConfig gives the right directory for each.
	assert.Equal(t, projectDir, settings.ProjectDirFromConfig(localConfigPath))
}
