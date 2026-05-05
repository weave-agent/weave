package wire

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestRunExtensionOverride(t *testing.T) {
	dir := t.TempDir()

	cfgFile := dir + "/.weave.yaml"
	require.NoError(t, os.WriteFile(cfgFile, []byte("extensions: [noop]\n"), 0o600))

	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))

	defer func() { _ = os.Chdir(origWd) }()

	assert.Equal(t, 1, run(context.Background(), "-e", "ext1,ext2"))
}

func TestRunCoreDefaultsUsed(t *testing.T) {
	dir := t.TempDir()

	cfgFile := dir + "/.weave.yaml"
	require.NoError(t, os.WriteFile(cfgFile, []byte("{}\n"), 0o600))

	require.NoError(t, os.WriteFile(dir+"/go.mod", []byte("module weave\n\ngo 1.24\n"), 0o600))

	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))

	defer func() { _ = os.Chdir(origWd) }()

	old := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stderr = w

	defer func() { os.Stderr = old }()

	exitCode := run(context.Background())

	_ = w.Close()

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	_ = r.Close()

	stderr := string(buf[:n])

	assert.Equal(t, 1, exitCode)
	assert.True(t, strings.Contains(stderr, "loop") || strings.Contains(stderr, "anthropic"),
		"stderr should mention 'loop' or 'anthropic' (core defaults), got: %q", stderr)
}

func TestValidateCoreConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *config.File
		wantErr error
	}{
		{
			"valid defaults",
			&config.File{Core: config.CoreConfig{AgentLoop: "loop", Providers: []string{"anthropic"}}, UI: "tui"},
			nil,
		},
		{
			"empty agent_loop",
			&config.File{Core: config.CoreConfig{AgentLoop: "", Providers: []string{"anthropic"}}, UI: "tui"},
			errors.New("agent_loop"),
		},
		{
			"no providers",
			&config.File{Core: config.CoreConfig{AgentLoop: "loop", Providers: nil}, UI: "tui"},
			errors.New("at least one provider"),
		},
		{
			"empty providers",
			&config.File{Core: config.CoreConfig{AgentLoop: "loop", Providers: []string{}}, UI: "tui"},
			errors.New("at least one provider"),
		},
		{
			"duplicate providers",
			&config.File{Core: config.CoreConfig{AgentLoop: "loop", Providers: []string{"anthropic", "anthropic"}}, UI: "tui"},
			errors.New("duplicate provider"),
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

func TestTUIExtensionAddedWhenNoPrompt(t *testing.T) {
	cf := &config.File{
		Core:       config.CoreConfig{AgentLoop: "loop", Providers: []string{"anthropic"}},
		Extensions: []string{"bash"},
		UI:         "tui",
	}

	allExts := cf.AllExtensions()

	if cf.Prompt == "" && cf.UI != "" && cf.UI != "none" {
		allExts = ensurePresent(allExts, cf.UI)
	}

	assert.Contains(t, allExts, "tui", "tui should be in extension list when no prompt and ui=tui")
	assert.Contains(t, allExts, "bash")
	assert.Contains(t, allExts, "loop")
	assert.Contains(t, allExts, "anthropic")
}

func TestTUIExtensionExcludedWhenPromptSet(t *testing.T) {
	cf := &config.File{
		Core:       config.CoreConfig{AgentLoop: "loop", Providers: []string{"anthropic"}},
		Extensions: []string{"bash"},
		Prompt:     "hello",
		UI:         "tui",
	}

	allExts := cf.AllExtensions()

	if cf.Prompt == "" && cf.UI != "" && cf.UI != "none" {
		allExts = ensurePresent(allExts, cf.UI)
	}

	assert.NotContains(t, allExts, "tui", "tui should NOT be in extension list when prompt is set")
}

func TestTUIExtensionExcludedWhenUINone(t *testing.T) {
	cf := &config.File{
		Core:       config.CoreConfig{AgentLoop: "loop", Providers: []string{"anthropic"}},
		Extensions: []string{"bash"},
		UI:         "none",
	}

	allExts := cf.AllExtensions()

	if cf.Prompt == "" && cf.UI != "" && cf.UI != "none" {
		allExts = ensurePresent(allExts, cf.UI)
	}

	assert.NotContains(t, allExts, "tui", "tui should not be included when ui is 'none'")
}

func TestSkillsAlwaysIncluded(t *testing.T) {
	cf := &config.File{
		Core:       config.CoreConfig{AgentLoop: "loop", Providers: []string{"anthropic"}},
		Extensions: []string{"bash"},
		UI:         "tui",
	}

	allExts := cf.AllExtensions()
	allExts = ensurePresent(allExts, "skills")

	assert.Contains(t, allExts, "skills", "skills should always be in extension list")
}

func TestSkillsIncludedInHeadlessMode(t *testing.T) {
	cf := &config.File{
		Core:       config.CoreConfig{AgentLoop: "loop", Providers: []string{"anthropic"}},
		Extensions: []string{"bash"},
		Prompt:     "do something",
		UI:         "none",
	}

	allExts := cf.AllExtensions()
	allExts = ensurePresent(allExts, "skills")

	assert.Contains(t, allExts, "skills", "skills should be included even in headless mode")
	assert.NotContains(t, allExts, "tui", "tui should not be included in headless mode")
}

func TestUIExtensionsIncludedInTUIMode(t *testing.T) {
	cf := &config.File{
		Core:         config.CoreConfig{AgentLoop: "loop", Providers: []string{"anthropic"}},
		Extensions:   []string{"bash"},
		UIExtensions: []string{"diff-viewer"},
		UI:           "tui",
	}

	allExts := cf.AllExtensions()

	assert.Contains(t, allExts, "diff-viewer", "UI extensions should be included when ui is 'tui'")
	assert.Contains(t, allExts, "bash")
	assert.Contains(t, allExts, "loop")
	assert.Contains(t, allExts, "anthropic")
}

func TestUIExtensionsExcludedInHeadlessMode(t *testing.T) {
	cf := &config.File{
		Core:         config.CoreConfig{AgentLoop: "loop", Providers: []string{"anthropic"}},
		Extensions:   []string{"bash"},
		UIExtensions: []string{"diff-viewer"},
		UI:           "none",
	}

	allExts := cf.AllExtensions()

	assert.NotContains(t, allExts, "diff-viewer", "UI extensions should be excluded when ui is 'none'")
	assert.Contains(t, allExts, "bash")
	assert.Contains(t, allExts, "loop")
	assert.Contains(t, allExts, "anthropic")
}

func TestUIExtensionsNotDuplicated(t *testing.T) {
	cf := &config.File{
		Core:         config.CoreConfig{AgentLoop: "loop", Providers: []string{"anthropic"}},
		Extensions:   []string{"bash", "diff-viewer"},
		UIExtensions: []string{"diff-viewer"},
		UI:           "tui",
	}

	allExts := cf.AllExtensions()

	count := 0

	for _, ext := range allExts {
		if ext == "diff-viewer" {
			count++
		}
	}

	assert.Equal(t, 1, count, "UI extension should appear exactly once even if also in extensions list")
}

func TestSkillsNotDuplicated(t *testing.T) {
	cf := &config.File{
		Core:       config.CoreConfig{AgentLoop: "loop", Providers: []string{"anthropic"}},
		Extensions: []string{"skills", "bash"},
		UI:         "tui",
	}

	allExts := cf.AllExtensions()
	allExts = ensurePresent(allExts, "skills")

	skillsCount := 0

	for _, ext := range allExts {
		if ext == "skills" {
			skillsCount++
		}
	}

	assert.Equal(t, 1, skillsCount, "skills should appear exactly once even if already in extensions list")
}

func TestResolveExtensions_AutoProviderIgnoredOnReload(t *testing.T) {
	t.Setenv("WEAVE_PROVIDER", "anthropic")
	t.Setenv("WEAVE_PROVIDER_AUTO", "1")

	cf := &config.File{
		Core: config.CoreConfig{AgentLoop: "loop", Providers: []string{"openai"}},
		UI:   "tui",
	}

	_, providers, _, ok := resolveExtensionsAndMode(cf, nil)
	require.True(t, ok)

	assert.Equal(t, []string{"openai"}, providers, "synthesized WEAVE_PROVIDER must not be added when AUTO=1")
	assert.Equal(t, "openai", os.Getenv("WEAVE_PROVIDER"), "WEAVE_PROVIDER should be re-synthesized from new config")
	assert.Equal(t, "1", os.Getenv("WEAVE_PROVIDER_AUTO"), "AUTO marker should be set after re-synthesis")
}

func TestResolveExtensions_UserProviderRespected(t *testing.T) {
	t.Setenv("WEAVE_PROVIDER", "openai")
	t.Setenv("WEAVE_PROVIDER_AUTO", "")

	cf := &config.File{
		Core: config.CoreConfig{AgentLoop: "loop", Providers: []string{"anthropic"}},
		UI:   "tui",
	}

	_, providers, _, ok := resolveExtensionsAndMode(cf, nil)
	require.True(t, ok)

	assert.Contains(t, providers, "openai", "user-supplied WEAVE_PROVIDER should be added to providers")
	assert.Contains(t, providers, "anthropic", "config providers should be preserved")
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
