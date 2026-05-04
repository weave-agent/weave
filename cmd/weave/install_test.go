package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSource_GitURL(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		wantURL  string
		wantName string
	}{
		{"https url", "https://github.com/user/weave-ext-mcp", "https://github.com/user/weave-ext-mcp", "weave-ext-mcp"},
		{"https url with .git", "https://github.com/user/repo.git", "https://github.com/user/repo.git", "repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := parseSource(tt.source)
			require.NoError(t, err)
			assert.Equal(t, sourceGitURL, parsed.kind)
			assert.Equal(t, tt.wantURL, parsed.gitURL)
			assert.Equal(t, tt.wantName, parsed.rawName)
		})
	}
}

func TestParseSource_RejectsHTTP(t *testing.T) {
	_, err := parseSource("http://example.com/ext/my-tool")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insecure URL")
}

func TestParseSource_RejectsGitProtocol(t *testing.T) {
	_, err := parseSource("git://example.com/ext.git")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid source")
}

func TestParseSource_GitHubShorthand(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		wantURL  string
		wantName string
	}{
		{"simple", "github.com/user/weave-ext-mcp", "https://github.com/user/weave-ext-mcp", "weave-ext-mcp"},
		{"with .git", "github.com/user/repo.git", "https://github.com/user/repo.git", "repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := parseSource(tt.source)
			require.NoError(t, err)
			assert.Equal(t, sourceGitHub, parsed.kind)
			assert.Equal(t, tt.wantURL, parsed.gitURL)
			assert.Equal(t, tt.wantName, parsed.rawName)
		})
	}
}

func TestParseSource_GitHubShorthandInvalid(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{"only user", "github.com/user"},
		{"too many parts", "github.com/user/repo/extra"},
		{"empty user", "github.com//repo"},
		{"empty repo", "github.com/user/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSource(tt.source)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid GitHub shorthand")
		})
	}
}

func TestParseSource_LocalPath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(dir, 0o750))

	parsed, err := parseSource(dir)
	require.NoError(t, err)
	assert.Equal(t, sourceLocalPath, parsed.kind)
	assert.Equal(t, dir, parsed.localDir)
	assert.Equal(t, filepath.Base(dir), parsed.rawName)
}

func TestParseSource_LocalPathNotExist(t *testing.T) {
	_, err := parseSource("/nonexistent/path/ext")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path")
}

func TestParseSource_LocalPathFile(t *testing.T) {
	f, err := os.CreateTemp("", "weave-test-*.go")
	require.NoError(t, err)

	_ = f.Close()

	t.Cleanup(func() { _ = os.Remove(f.Name()) })

	_, err = parseSource(f.Name())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

func TestParseSource_InvalidSource(t *testing.T) {
	_, err := parseSource("just-a-name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid source")
}

func TestParseSource_RelativePath(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "my-ext")
	require.NoError(t, os.MkdirAll(extDir, 0o750))

	origWd, _ := os.Getwd()

	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	parsed, err := parseSource("./my-ext")
	require.NoError(t, err)
	assert.Equal(t, sourceLocalPath, parsed.kind)
	assert.Equal(t, "my-ext", parsed.rawName)
}

func TestDeriveNameFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://github.com/user/weave-ext-mcp", "weave-ext-mcp"},
		{"https://github.com/user/repo.git", "repo"},
		{"https://example.com/ext.git", "ext"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, tt.want, deriveNameFromURL(tt.url))
		})
	}
}

func TestRunInstall_MissingSource(t *testing.T) {
	assert.Equal(t, 1, runInstall(nil))
	assert.Equal(t, 1, runInstall([]string{}))
}

func TestRunInstall_NameWithoutValue(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "ext")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main\n"), 0o600))

	assert.Equal(t, 1, runInstall([]string{extDir, "--name"}))
}

func TestRunInstall_UnknownArg(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "ext")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main\n"), 0o600))

	assert.Equal(t, 1, runInstall([]string{extDir, "--unknown-flag"}))
}

func TestRunInstall_NameWithEqualsForm(t *testing.T) {
	srcDir := t.TempDir()
	extDir := filepath.Join(srcDir, "original-name")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main\n"), 0o600))

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	code := runInstall([]string{extDir, "--name=equals-form"})
	assert.Equal(t, 0, code)

	destDir := filepath.Join(homeDir, ".weave", "extensions", "equals-form")
	info, err := os.Stat(destDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestRunInstall_InvalidName(t *testing.T) {
	// Provide an explicit --name that's invalid.
	dir := t.TempDir()
	extDir := filepath.Join(dir, "ext")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main\n"), 0o600))

	code := runInstall([]string{dir, "--name", "invalid name!"})
	assert.Equal(t, 1, code)
}

func TestRunInstall_LocalPath(t *testing.T) {
	// Create a source extension with a .go file.
	srcDir := t.TempDir()
	extDir := filepath.Join(srcDir, "my-tool")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main\n"), 0o600))

	// Create a fake home dir.
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	code := runInstall([]string{extDir})
	assert.Equal(t, 0, code)

	// Verify the extension was copied.
	destDir := filepath.Join(homeDir, ".weave", "extensions", "my-tool")
	info, err := os.Stat(destDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	data, err := os.ReadFile(filepath.Join(destDir, "main.go"))
	require.NoError(t, err)
	assert.Equal(t, "package main\n", string(data))
}

func TestRunInstall_LocalPathWithExplicitName(t *testing.T) {
	srcDir := t.TempDir()
	extDir := filepath.Join(srcDir, "original-name")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main\n"), 0o600))

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	code := runInstall([]string{extDir, "--name", "custom-name"})
	assert.Equal(t, 0, code)

	destDir := filepath.Join(homeDir, ".weave", "extensions", "custom-name")
	info, err := os.Stat(destDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestRunInstall_LocalPathNoGoFiles(t *testing.T) {
	srcDir := t.TempDir()
	extDir := filepath.Join(srcDir, "empty-ext")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	// No .go files — just a readme.
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "README.md"), []byte("# readme\n"), 0o600))

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	code := runInstall([]string{extDir})
	assert.Equal(t, 1, code)

	// dest dir should have been cleaned up.
	destDir := filepath.Join(homeDir, ".weave", "extensions", "empty-ext")
	_, err := os.Stat(destDir)
	assert.True(t, os.IsNotExist(err), "dest dir should be cleaned up when no .go files")
}

func TestRunInstall_OverwriteExisting(t *testing.T) {
	srcDir := t.TempDir()
	extDir := filepath.Join(srcDir, "my-tool")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main // v2\n"), 0o600))

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	// First install.
	code := runInstall([]string{extDir})
	assert.Equal(t, 0, code)

	// Install again (overwrite).
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main // v3\n"), 0o600))

	code = runInstall([]string{extDir})
	assert.Equal(t, 0, code)

	data, err := os.ReadFile(filepath.Join(homeDir, ".weave", "extensions", "my-tool", "main.go"))
	require.NoError(t, err)
	assert.Equal(t, "package main // v3\n", string(data))
}

func TestDispatch_InstallSubcommand(t *testing.T) {
	// Verify that "install" is routed to runInstall.
	dir := t.TempDir()
	extDir := filepath.Join(dir, "test-ext")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main\n"), 0o600))

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	code := dispatch([]string{"install", extDir})
	assert.Equal(t, 0, code)
}

func TestDispatch_DefaultRun(t *testing.T) {
	// Non-install args should go to run() which will fail because no config.
	// This just verifies the routing, not run() behavior.
	assert.Equal(t, 1, dispatch([]string{"-xyz"}))
}

func TestHasGoFiles(t *testing.T) {
	dir := t.TempDir()
	assert.False(t, hasGoFiles(dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o600))
	assert.True(t, hasGoFiles(dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main\n"), 0o600))
	// Still true — test files don't count but main.go is still there.
	assert.True(t, hasGoFiles(dir))
}

func TestHasGoFiles_OnlyTestFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main\n"), 0o600))
	assert.False(t, hasGoFiles(dir), "test-only files should not count as .go files")
}

func TestRunInstall_SkipsHiddenDirs(t *testing.T) {
	srcDir := t.TempDir()
	extDir := filepath.Join(srcDir, "my-tool")
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, ".git", "objects"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, ".git", "config"), []byte("[remote]\n  url = secret\n"), 0o600))
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main\n"), 0o600))

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	code := runInstall([]string{extDir})
	assert.Equal(t, 0, code)

	destDir := filepath.Join(homeDir, ".weave", "extensions", "my-tool")

	// .git directory should not be copied.
	_, err := os.Stat(filepath.Join(destDir, ".git"))
	assert.True(t, os.IsNotExist(err), ".git directory should be skipped")

	// But the .go file should be copied.
	data, err := os.ReadFile(filepath.Join(destDir, "main.go"))
	require.NoError(t, err)
	assert.Equal(t, "package main\n", string(data))
}
