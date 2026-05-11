package extmanage

import (
	"os"
	"path/filepath"
	"strings"
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
		{"ssh url", "ssh://git@example.com/user/ext.git", "ssh://git@example.com/user/ext.git", "ext"},
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

func TestParseSource_RejectsGit(t *testing.T) {
	_, err := parseSource("git://example.com/ext.git")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insecure URL")
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
	assert.Equal(t, 1, RunInstall(nil))
	assert.Equal(t, 1, RunInstall([]string{}))
}

func TestRunInstall_NameWithoutValue(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "ext")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main\n"), 0o600))

	assert.Equal(t, 1, RunInstall([]string{extDir, "--name"}))
}

func TestRunInstall_UnknownArg(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "ext")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main\n"), 0o600))

	assert.Equal(t, 1, RunInstall([]string{extDir, "--unknown-flag"}))
}

func TestRunInstall_NameWithEqualsForm(t *testing.T) {
	srcDir := t.TempDir()
	extDir := filepath.Join(srcDir, "original-name")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "go.mod"), []byte("module test/ext\n\ngo 1.22\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main\n"), 0o600))

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	code := RunInstall([]string{extDir, "--name=equals-form"})
	assert.Equal(t, 0, code)

	destDir := filepath.Join(homeDir, ".weave", "extensions", "equals-form")
	info, err := os.Stat(destDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestRunInstall_InvalidName(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "ext")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main\n"), 0o600))

	code := RunInstall([]string{dir, "--name", "invalid name!"})
	assert.Equal(t, 1, code)
}

func TestRunInstall_LocalPath(t *testing.T) {
	srcDir := t.TempDir()
	extDir := filepath.Join(srcDir, "my-tool")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "go.mod"), []byte("module test/ext\n\ngo 1.22\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main\n"), 0o600))

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	code := RunInstall([]string{extDir})
	assert.Equal(t, 0, code)

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
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "go.mod"), []byte("module test/ext\n\ngo 1.22\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main\n"), 0o600))

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	code := RunInstall([]string{extDir, "--name", "custom-name"})
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
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "README.md"), []byte("# readme\n"), 0o600))

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	code := RunInstall([]string{extDir})
	assert.Equal(t, 1, code)

	destDir := filepath.Join(homeDir, ".weave", "extensions", "empty-ext")
	_, err := os.Stat(destDir)
	assert.True(t, os.IsNotExist(err), "dest dir should be cleaned up when no .go files")
}

func TestRunInstall_LocalPathNoGoMod(t *testing.T) {
	srcDir := t.TempDir()
	extDir := filepath.Join(srcDir, "no-mod-ext")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main\n"), 0o600))

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	code := RunInstall([]string{extDir})
	assert.Equal(t, 1, code)

	destDir := filepath.Join(homeDir, ".weave", "extensions", "no-mod-ext")
	_, err := os.Stat(destDir)
	assert.True(t, os.IsNotExist(err), "dest dir should be cleaned up when no go.mod")
}

func TestRunInstall_OverwriteExisting(t *testing.T) {
	srcDir := t.TempDir()
	extDir := filepath.Join(srcDir, "my-tool")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "go.mod"), []byte("module test/ext\n\ngo 1.22\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main // v2\n"), 0o600))

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	code := RunInstall([]string{extDir})
	assert.Equal(t, 0, code)

	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main // v3\n"), 0o600))

	code = RunInstall([]string{extDir})
	assert.Equal(t, 0, code)

	data, err := os.ReadFile(filepath.Join(homeDir, ".weave", "extensions", "my-tool", "main.go"))
	require.NoError(t, err)
	assert.Equal(t, "package main // v3\n", string(data))
}

func TestRunInstall_SkipsHiddenDirs(t *testing.T) {
	srcDir := t.TempDir()
	extDir := filepath.Join(srcDir, "my-tool")
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, ".git", "objects"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, ".git", "config"), []byte("[remote]\n  url = secret\n"), 0o600))
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "go.mod"), []byte("module test/ext\n\ngo 1.22\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "main.go"), []byte("package main\n"), 0o600))

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	code := RunInstall([]string{extDir})
	assert.Equal(t, 0, code)

	destDir := filepath.Join(homeDir, ".weave", "extensions", "my-tool")

	_, err := os.Stat(filepath.Join(destDir, ".git"))
	assert.True(t, os.IsNotExist(err), ".git directory should be skipped")

	data, err := os.ReadFile(filepath.Join(destDir, "main.go"))
	require.NoError(t, err)
	assert.Equal(t, "package main\n", string(data))
}

func TestRunInstall_FailedValidationPreservesExisting(t *testing.T) {
	srcDir := t.TempDir()
	goodExt := filepath.Join(srcDir, "my-tool")
	require.NoError(t, os.MkdirAll(goodExt, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(goodExt, "go.mod"), []byte("module test/ext\n\ngo 1.22\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(goodExt, "main.go"), []byte("package main // v1\n"), 0o600))

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	require.Equal(t, 0, RunInstall([]string{goodExt}))

	destDir := filepath.Join(homeDir, ".weave", "extensions", "my-tool")
	require.FileExists(t, filepath.Join(destDir, "main.go"))

	badSrc := t.TempDir()
	badExt := filepath.Join(badSrc, "my-tool")
	require.NoError(t, os.MkdirAll(badExt, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(badExt, "README.md"), []byte("# readme\n"), 0o600))

	require.Equal(t, 1, RunInstall([]string{badExt}))

	data, err := os.ReadFile(filepath.Join(destDir, "main.go"))
	require.NoError(t, err)
	assert.Equal(t, "package main // v1\n", string(data), "existing extension must survive failed install")

	entries, err := os.ReadDir(filepath.Join(homeDir, ".weave", "extensions"))
	require.NoError(t, err)

	for _, e := range entries {
		assert.False(t, strings.HasPrefix(e.Name(), ".staging-"), "staging dir %q should be removed", e.Name())
	}
}

func TestRunInstall_RejectsSelfInstall(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	destDir := filepath.Join(homeDir, ".weave", "extensions", "my-tool")
	require.NoError(t, os.MkdirAll(destDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(destDir, "main.go"), []byte("package main\n"), 0o600))

	code := RunInstall([]string{destDir})
	assert.Equal(t, 1, code)

	parent := filepath.Join(homeDir, ".weave", "extensions")
	code = RunInstall([]string{parent, "--name", "my-tool"})
	assert.Equal(t, 1, code)

	data, err := os.ReadFile(filepath.Join(destDir, "main.go"))
	require.NoError(t, err)
	assert.Equal(t, "package main\n", string(data))
}

func TestRejectSelfInstall(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		dest    string
		wantErr bool
	}{
		{"unrelated", "/tmp/source", "/home/user/.weave/extensions/x", false},
		{"identical", "/home/user/.weave/extensions/x", "/home/user/.weave/extensions/x", true},
		{"src contains dest", "/home/user/.weave/extensions", "/home/user/.weave/extensions/x", true},
		{"sibling", "/home/user/other", "/home/user/.weave/extensions/x", false},
		{"dest contains src", "/home/user/.weave/extensions/x/sub", "/home/user/.weave/extensions/x", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectSelfInstall(tt.src, tt.dest)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestHasGoFiles(t *testing.T) {
	dir := t.TempDir()
	assert.False(t, hasGoFiles(dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o600))
	assert.True(t, hasGoFiles(dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main\n"), 0o600))
	assert.True(t, hasGoFiles(dir))
}

func TestHasGoFiles_OnlyTestFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main\n"), 0o600))
	assert.False(t, hasGoFiles(dir), "test-only files should not count as .go files")
}

func TestCopyExtension_SymlinkCycle(t *testing.T) {
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.go"), []byte("package main\n"), 0o600))

	// Self-referencing symlink: loop -> .
	require.NoError(t, os.Symlink(".", filepath.Join(srcDir, "loop")))

	destDir := filepath.Join(t.TempDir(), "dest")
	err := copyExtension(srcDir, destDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink cycle detected")
}

func TestCopyExtension_SymlinkToAncestor(t *testing.T) {
	srcDir := t.TempDir()
	subDir := filepath.Join(srcDir, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.go"), []byte("package main\n"), 0o600))

	// Symlink pointing to parent: sub/up -> ..
	require.NoError(t, os.Symlink("..", filepath.Join(subDir, "up")))

	destDir := filepath.Join(t.TempDir(), "dest")
	err := copyExtension(srcDir, destDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink cycle detected")
}

func TestCopyExtension_SiblingSymlinksToSameTarget(t *testing.T) {
	srcDir := t.TempDir()

	// Real directory with a .go file
	realDir := filepath.Join(srcDir, "shared")
	require.NoError(t, os.MkdirAll(realDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(realDir, "main.go"), []byte("package main\n"), 0o600))

	// Two sibling symlinks pointing to the same real directory
	require.NoError(t, os.Symlink("shared", filepath.Join(srcDir, "alias-a")))
	require.NoError(t, os.Symlink("shared", filepath.Join(srcDir, "alias-b")))

	destDir := filepath.Join(t.TempDir(), "dest")
	err := copyExtension(srcDir, destDir)
	require.NoError(t, err, "sibling symlinks to same target should not be treated as a cycle")

	// All three copies should exist
	assert.FileExists(t, filepath.Join(destDir, "shared", "main.go"))
	assert.FileExists(t, filepath.Join(destDir, "alias-a", "main.go"))
	assert.FileExists(t, filepath.Join(destDir, "alias-b", "main.go"))
}
