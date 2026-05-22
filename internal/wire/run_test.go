package wire

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weave-agent/weave/settings"

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
			assert.Equal(t, tt.wantCode, run(context.Background(), tt.args, "unknown"))
		})
	}
}

func TestRunMissingConfig(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))

	defer func() { _ = os.Chdir(origWd) }()

	assert.Equal(t, 1, run(context.Background(), nil, "unknown"))
}

func TestRunCoreDefaultsUsed(t *testing.T) {
	dir := t.TempDir()

	cfgFile := dir + "/.weave/settings.json"
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgFile), 0o750))
	require.NoError(t, os.WriteFile(cfgFile, []byte(`{"ui_extension":"tui","agent_loop":"agent"}`), 0o600))

	_, cf, _, err := settings.LoadFromDir(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, "agent", cf.AgentLoop, "default agent_loop should be 'agent'")
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
			&settings.Settings{AgentLoop: "agent", UIExtension: "tui"},
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

	code := Run(context.Background(), []string{"install", extDir}, "unknown")
	assert.Equal(t, 0, code)
}

func TestRun_DefaultRoute(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))

	defer func() { _ = os.Chdir(origWd) }()

	code := Run(context.Background(), []string{"-xyz"}, "unknown")
	assert.Equal(t, 1, code)
}

func TestFindModuleRoot(t *testing.T) {
	_, err := findModuleRoot("unknown")
	require.NoError(t, err, "should find module root from the weave project directory")
}

func TestFindModuleRoot_ReleaseMode(t *testing.T) {
	// When running from a temp dir (no source tree) with a versioned revision,
	// findModuleRoot should return "" instead of erroring.
	dir := t.TempDir()

	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))

	defer func() { _ = os.Chdir(origWd) }()

	root, err := findModuleRoot("v0.0.1-abc0123-2026-05-17")
	require.NoError(t, err)
	assert.Empty(t, root, "release mode should return empty module root")
}

func TestFindModuleRoot_UnknownRevisionFails(t *testing.T) {
	dir := t.TempDir()

	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))

	defer func() { _ = os.Chdir(origWd) }()

	_, err := findModuleRoot("unknown")
	require.Error(t, err, "dev build without source tree should error")
}

func TestTagFromRevision(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"v0.0.1-abc0123-2026-05-17", "v0.0.1"},
		{"v1.2.3-def4567-2026-01-01", "v1.2.3"},
		{"v0.1.0-beta-abc0123-2026-05-17", "v0.1.0-beta"},
		{"v2.0.0-rc1-a1b2c3d4-2025-12-31", "v2.0.0-rc1"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, tagFromRevision(tt.input))
		})
	}
}

func TestFindModuleRootFrom_Invalid(t *testing.T) {
	_, err := findModuleRootFrom(func() (string, error) {
		return "/nonexistent/path/that/does/not/exist", nil
	})
	require.Error(t, err)
}

func TestIsWeaveModule(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/weave-agent/weave\n\ngo 1.24\n"), 0o600))
	assert.True(t, isWeaveModule(dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module other\n\ngo 1.24\n"), 0o600))
	assert.False(t, isWeaveModule(dir))
}

func TestRun_HelpFlagPrintsGlobalHelpWithoutBootstrapOrLauncher(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"long", []string{helpFlagLong}},
		{"short", []string{helpFlagShort}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgFile := filepath.Join(dir, ".weave", "settings.json")
			require.NoError(t, os.MkdirAll(filepath.Dir(cfgFile), 0o750))
			require.NoError(t, os.WriteFile(cfgFile, []byte(`{"ui_extension":"none","agent_loop":"agent"}`), 0o600))

			origWd, _ := os.Getwd()

			require.NoError(t, os.Chdir(dir))
			defer func() { _ = os.Chdir(origWd) }()

			var bootstrapCalls, launcherCalls int

			deps := runDeps{
				runBootstrap: func(context.Context, *settings.Settings) {
					bootstrapCalls++
				},
				runLauncher: func(context.Context, string, string, string, string, []string, string, string, bool, []string) error {
					launcherCalls++

					return nil
				},
			}

			var code int

			stderr := captureStderr(t, func() {
				code = runWithDeps(context.Background(), tt.args, "unknown", deps)
			})

			assert.Equal(t, 0, code)
			assert.Contains(t, stderr, "Usage: weave [options] [command]")
			assert.Equal(t, 0, bootstrapCalls)
			assert.Equal(t, 0, launcherCalls)
		})
	}
}

func TestRun_HelpFlagBypassesConfigLoading(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, ".weave", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgFile), 0o750))
	require.NoError(t, os.WriteFile(cfgFile, []byte(`{"ui_extension":`), 0o600))

	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origWd) }()

	var bootstrapCalls, launcherCalls int

	deps := runDeps{
		runBootstrap: func(context.Context, *settings.Settings) {
			bootstrapCalls++
		},
		runLauncher: func(context.Context, string, string, string, string, []string, string, string, bool, []string) error {
			launcherCalls++

			return nil
		},
	}

	var code int

	stderr := captureStderr(t, func() {
		code = runWithDeps(context.Background(), []string{helpFlagLong}, "unknown", deps)
	})

	assert.Equal(t, 0, code)
	assert.Contains(t, stderr, "Usage: weave [options] [command]")
	assert.Equal(t, 0, bootstrapCalls)
	assert.Equal(t, 0, launcherCalls)
}

func TestRun_HelpFlagDoesNotGenerateDefaultConfig(t *testing.T) {
	dir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origWd) }()

	var code int

	_ = captureStderr(t, func() {
		code = runWithDeps(context.Background(), []string{helpFlagLong}, "unknown", runDeps{})
	})

	assert.Equal(t, 0, code)
	assert.NoFileExists(t, filepath.Join(homeDir, ".weave", "settings.json"))
}

func TestHasHelpFlagRespectsFlagValuesAndDoubleDash(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "long help", args: []string{helpFlagLong}, want: true},
		{name: "short help", args: []string{helpFlagShort}, want: true},
		{name: "help after boolean flag", args: []string{"--debug", helpFlagLong}, want: true},
		{name: "long prompt value", args: []string{"--prompt", helpFlagLong}, want: false},
		{name: "short prompt value", args: []string{promptFlagShort, helpFlagShort}, want: false},
		{name: "equals prompt value", args: []string{"--prompt=" + helpFlagLong}, want: false},
		{name: "double dash", args: []string{"--", helpFlagLong}, want: false},
		{name: "help after prompt value", args: []string{promptFlagShort, "hello", helpFlagLong}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasHelpFlag(tt.args))
		})
	}
}

func TestRun_HelpFlagAsPromptValueLaunches(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, ".weave", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgFile), 0o750))
	require.NoError(t, os.WriteFile(cfgFile, []byte(`{"ui_extension":"none","agent_loop":"agent"}`), 0o600))

	t.Setenv("HOME", t.TempDir())

	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origWd) }()

	var bootstrapCalls, launcherCalls int

	deps := runDeps{
		runBootstrap: func(context.Context, *settings.Settings) {
			bootstrapCalls++
		},
		runLauncher: func(_ context.Context, _, _, _, _ string, args []string, _, _ string, headless bool, _ []string) error {
			launcherCalls++

			assert.True(t, headless)

			var prompt string

			for _, arg := range args {
				if promptFile, ok := strings.CutPrefix(arg, "--weave-prompt-file="); ok {
					data, err := os.ReadFile(promptFile)
					require.NoError(t, err)

					prompt = string(data)
				}
			}

			assert.Equal(t, helpFlagLong, prompt)

			return nil
		},
	}

	var code int

	stderr := captureStderr(t, func() {
		code = runWithDeps(context.Background(), []string{promptFlagShort, helpFlagLong}, "v0.0.1-abc0123-2026-05-17", deps)
	})

	assert.Equal(t, 0, code)
	assert.NotContains(t, stderr, "Usage: weave [options] [command]")
	assert.Equal(t, 1, bootstrapCalls)
	assert.Equal(t, 1, launcherCalls)
}

func TestRun_DoubleDashStopsHelpFastPath(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, ".weave", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgFile), 0o750))
	require.NoError(t, os.WriteFile(cfgFile, []byte(`{"ui_extension":"none","agent_loop":"agent"}`), 0o600))

	t.Setenv("HOME", t.TempDir())

	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origWd) }()

	var launcherCalls int

	deps := runDeps{
		runBootstrap: func(context.Context, *settings.Settings) {},
		runLauncher: func(_ context.Context, _, _, _, _ string, args []string, _, _ string, headless bool, _ []string) error {
			launcherCalls++

			assert.True(t, headless)
			assert.Contains(t, args, "--")
			assert.Contains(t, args, helpFlagLong)

			return nil
		},
	}

	var code int

	stderr := captureStderr(t, func() {
		code = runWithDeps(context.Background(), []string{promptFlagShort, "hello", "--", helpFlagLong}, "v0.0.1-abc0123-2026-05-17", deps)
	})

	assert.Equal(t, 0, code)
	assert.NotContains(t, stderr, "Usage: weave [options] [command]")
	assert.Equal(t, 1, launcherCalls)
}

func TestRun_NoInputSkipsBootstrapAndLauncher(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, ".weave", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgFile), 0o750))
	require.NoError(t, os.WriteFile(cfgFile, []byte(`{"ui_extension":"none","agent_loop":"agent"}`), 0o600))

	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origWd) }()

	var bootstrapCalls, launcherCalls int

	deps := runDeps{
		runBootstrap: func(context.Context, *settings.Settings) {
			bootstrapCalls++
		},
		runLauncher: func(context.Context, string, string, string, string, []string, string, string, bool, []string) error {
			launcherCalls++

			return nil
		},
	}

	var code int

	stderr := captureStderr(t, func() {
		code = runWithDeps(context.Background(), nil, "unknown", deps)
	})

	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, errNoInput.Error())
	assert.Equal(t, 0, bootstrapCalls)
	assert.Equal(t, 0, launcherCalls)
}

func TestRun_NormalLaunchRunsBootstrapBeforeLauncher(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, ".weave", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgFile), 0o750))
	require.NoError(t, os.WriteFile(cfgFile, []byte(`{"ui_extension":"tui","agent_loop":"agent"}`), 0o600))

	realDir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	realCfgFile := filepath.Join(realDir, ".weave", "settings.json")

	t.Setenv("HOME", t.TempDir())

	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origWd) }()

	var events []string

	deps := runDeps{
		runBootstrap: func(context.Context, *settings.Settings) {
			events = append(events, "bootstrap")
		},
		runLauncher: func(_ context.Context, _, _, _, projectDir string, _ []string, gotConfigFile, agentLoop string, headless bool, _ []string) error {
			events = append(events, "launcher")

			assert.Equal(t, realDir, projectDir)
			assert.Equal(t, realCfgFile, gotConfigFile)
			assert.Equal(t, "agent", agentLoop)
			assert.False(t, headless)

			return nil
		},
	}

	code := runWithDeps(context.Background(), nil, "v0.0.1-abc0123-2026-05-17", deps)

	assert.Equal(t, 0, code)
	assert.Equal(t, []string{"bootstrap", "launcher"}, events)
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

func TestLoadConfig_WeavePromptFile(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, ".weave", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgFile), 0o750))
	require.NoError(t, os.WriteFile(cfgFile, []byte(`{"ui_extension":"none","agent_loop":"agent"}`), 0o600))

	promptFile := filepath.Join(dir, "prompt.txt")
	require.NoError(t, os.WriteFile(promptFile, []byte("hidden prompt"), 0o600))

	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origWd) }()

	_, cf, rest, err := loadConfig([]string{"--weave-prompt-file=" + promptFile, "--output=json"})
	require.NoError(t, err)

	assert.Equal(t, "hidden prompt", cf.Prompt)
	assert.Equal(t, "json", cf.Output)
	assert.Contains(t, rest, "--weave-prompt-file="+promptFile)
}

func TestLoadConfig_WeaveProjectDir(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, ".weave", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgFile), 0o750))
	require.NoError(t, os.WriteFile(cfgFile, []byte(`{"ui_extension":"none","agent_loop":"agent"}`), 0o600))

	projectDir := filepath.Join(dir, "project")
	require.NoError(t, os.MkdirAll(projectDir, 0o750))

	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origWd) }()

	_, _, rest, err := loadConfig([]string{"--weave-project-dir=" + projectDir})
	require.NoError(t, err)

	assert.Contains(t, rest, "--weave-project-dir="+projectDir)
	assert.Equal(t, projectDir, weaveProjectDirFromRest(rest))
}

func TestHandleSubcommand_Install(t *testing.T) {
	// install subcommand needs a valid extension dir, so we create one.
	dir := t.TempDir()
	extDir := filepath.Join(dir, "test-ext")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "go.mod"), []byte("module test/ext\n\ngo 1.22\n"), 0o600))

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	code, ok := handleSubcommand([]string{"install", extDir})
	assert.True(t, ok)
	assert.Equal(t, 0, code)
}

func TestHandleSubcommand_List(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	code, ok := handleSubcommand([]string{"list"})
	assert.True(t, ok)
	assert.Equal(t, 0, code)
}

func TestHandleSubcommand_CacheClean(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	cacheDir := filepath.Join(homeDir, ".weave", "bin")
	hash := strings.Repeat("a", 64)
	require.NoError(t, os.MkdirAll(filepath.Join(cacheDir, hash), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, hash, "weave"), []byte("binary"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "notes.txt"), []byte("keep"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(cacheDir, "not-cache"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "not-cache", "weave"), []byte("keep"), 0o600))

	extensionFile := filepath.Join(homeDir, ".weave", "extensions", "keep", "main.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(extensionFile), 0o750))
	require.NoError(t, os.WriteFile(extensionFile, []byte("package main\n"), 0o600))

	var (
		code int
		ok   bool
	)

	stdout := captureStdout(t, func() {
		code, ok = handleSubcommand([]string{cacheCommand, "clean"})
	})

	assert.True(t, ok)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "removed 1 launcher cache entries")
	assert.NoFileExists(t, filepath.Join(cacheDir, hash, "weave"))
	assert.FileExists(t, filepath.Join(cacheDir, "notes.txt"))
	assert.FileExists(t, filepath.Join(cacheDir, "not-cache", "weave"))
	assert.FileExists(t, extensionFile)
}

func TestHandleSubcommand_CacheMissingCommand(t *testing.T) {
	var (
		code int
		ok   bool
	)

	stderr := captureStderr(t, func() {
		code, ok = handleSubcommand([]string{cacheCommand})
	})

	assert.True(t, ok)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "weave cache: missing command")
	assert.Contains(t, stderr, "usage: weave cache clean")
}

func TestHandleSubcommand_CacheUnknownCommand(t *testing.T) {
	var (
		code int
		ok   bool
	)

	stderr := captureStderr(t, func() {
		code, ok = handleSubcommand([]string{cacheCommand, "prune"})
	})

	assert.True(t, ok)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, `weave cache: unknown command "prune"`)
	assert.Contains(t, stderr, "usage: weave cache clean")
}

func TestHandleSubcommand_CacheCleanRejectsExtraArgs(t *testing.T) {
	var (
		code int
		ok   bool
	)

	stderr := captureStderr(t, func() {
		code, ok = handleSubcommand([]string{cacheCommand, "clean", "extra"})
	})

	assert.True(t, ok)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "usage: weave cache clean")
}

func TestHandleSubcommand_CacheCleanReportsHomeDirError(t *testing.T) {
	t.Setenv("HOME", "")

	var (
		code int
		ok   bool
	)

	stderr := captureStderr(t, func() {
		code, ok = handleSubcommand([]string{cacheCommand, "clean"})
	})

	assert.True(t, ok)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "weave cache clean: cache: get home dir")
}

func TestHandleSubcommand_CacheCleanReportsCleanError(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".weave"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".weave", "bin"), []byte("not a directory"), 0o600))

	var (
		code int
		ok   bool
	)

	stderr := captureStderr(t, func() {
		code, ok = handleSubcommand([]string{cacheCommand, "clean"})
	})

	assert.True(t, ok)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "weave cache clean: cache: read root")
}

func TestHandleSubcommand_Unknown(t *testing.T) {
	code, ok := handleSubcommand([]string{"unknown"})
	assert.False(t, ok)
	assert.Equal(t, 0, code)
}

func TestHandleSubcommand_Empty(t *testing.T) {
	code, ok := handleSubcommand([]string{})
	assert.False(t, ok)
	assert.Equal(t, 0, code)
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
	require.NoError(t, os.WriteFile(cfgFile, []byte(`{"ui_extension":"none","agent_loop":"agent"}`), 0o600))

	_, cf, rest, err := settings.LoadFromDir(dir, []string{
		promptFlagShort, "test",
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

func TestRun_DebugFlagParsed(t *testing.T) {
	dir := t.TempDir()
	cfgFile := dir + "/.weave/settings.json"
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgFile), 0o750))
	require.NoError(t, os.WriteFile(cfgFile, []byte(`{"ui_extension":"none","agent_loop":"agent"}`), 0o600))

	_, cf, rest, err := settings.LoadFromDir(dir, []string{
		promptFlagShort, "test",
		"--debug",
	})
	require.NoError(t, err)

	assert.True(t, cf.Debug, "--debug should set Debug to true")
	assert.Empty(t, rest, "all flags should be consumed by loader")
}

func TestRun_EmptyToolsFlagForwarded(t *testing.T) {
	dir := t.TempDir()
	cfgFile := dir + "/.weave/settings.json"
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgFile), 0o750))
	require.NoError(t, os.WriteFile(cfgFile, []byte(`{"ui_extension":"none","agent_loop":"agent"}`), 0o600))

	_, cf, rest, err := settings.LoadFromDir(dir, []string{
		promptFlagShort, "test",
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
	require.NoError(t, os.WriteFile(cfgFile, []byte(`{"ui_extension":"none","agent_loop":"agent"}`), 0o600))

	// Loading from subdir should find the config at project root.
	_, cf, _, err := settings.LoadFromDir(subDir, []string{promptFlagShort, "hello"})
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

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stderr = w
	defer func() {
		os.Stderr = orig
	}()

	fn()

	require.NoError(t, w.Close())

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	return string(out)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w
	defer func() {
		os.Stdout = orig
	}()

	fn()

	require.NoError(t, w.Close())

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	return string(out)
}
