package wire

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"weave/config"

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

	cfgFile := dir + "/.weave.yaml"
	require.NoError(t, os.WriteFile(cfgFile, []byte("{}\n"), 0o600))

	_, cf, _, err := config.LoadFromDir(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, "loop", cf.Core.AgentLoop, "default agent_loop should be 'loop'")
	assert.Equal(t, "tui", cf.UI, "default ui should be 'tui'")
}

func TestValidateCoreConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *config.File
		wantErr error
	}{
		{
			"valid defaults",
			&config.File{Core: config.CoreConfig{AgentLoop: "loop"}, UI: "tui"},
			nil,
		},
		{
			"empty agent_loop",
			&config.File{Core: config.CoreConfig{AgentLoop: ""}, UI: "tui"},
			errors.New("agent_loop"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.Validate(tt.config)
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr.Error())
			}
		})
	}
}

func TestResolveExtensionsAndMode_Headless(t *testing.T) {
	cf := &config.File{
		Core:   config.CoreConfig{AgentLoop: "loop"},
		UI:     "tui",
		Prompt: "hello",
	}

	providers, rest, ok := resolveExtensionsAndMode(cf, nil)
	require.True(t, ok)
	assert.Empty(t, providers)
	assert.Contains(t, rest, "--weave-headless=true")
}

func TestResolveExtensionsAndMode_Interactive(t *testing.T) {
	cf := &config.File{
		Core: config.CoreConfig{AgentLoop: "loop"},
		UI:   "tui",
	}

	providers, rest, ok := resolveExtensionsAndMode(cf, nil)
	require.True(t, ok)
	assert.Empty(t, providers)
	assert.Contains(t, rest, "--weave-headless=false")
}

func TestResolveExtensionsAndMode_NoInput(t *testing.T) {
	cf := &config.File{
		Core: config.CoreConfig{AgentLoop: "loop"},
		UI:   "none",
	}

	_, _, ok := resolveExtensionsAndMode(cf, nil)
	assert.False(t, ok)
}

func TestResolveExtensionsAndMode_EnvProvider(t *testing.T) {
	t.Setenv("WEAVE_PROVIDER", "openai")

	cf := &config.File{
		Core: config.CoreConfig{AgentLoop: "loop"},
		UI:   "tui",
	}

	providers, _, ok := resolveExtensionsAndMode(cf, nil)
	require.True(t, ok)
	assert.Equal(t, []string{"openai"}, providers)
}

func TestRun_InstallSubcommand(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "test-ext")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main\n"), 0o600))

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	code := Run(context.Background(), []string{"install", extDir})
	assert.Equal(t, 0, code)
}

func TestRun_DefaultRoute(t *testing.T) {
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

func TestEnsurePresent(t *testing.T) {
	exts := []string{"a", "b"}
	result := ensurePresent(exts, "a")
	assert.Equal(t, []string{"a", "b"}, result)

	result = ensurePresent(exts, "c")
	assert.Equal(t, []string{"a", "b", "c"}, result)
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
	assert.Equal(t, "/project", resolveProjectDir("/project/.weave/config.yaml"))
	assert.Equal(t, "/project", resolveProjectDir("/project/config.yaml"))
	assert.Equal(t, "/", resolveProjectDir("/.weave/config.yaml"))
}
