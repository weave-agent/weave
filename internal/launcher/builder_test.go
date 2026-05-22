package launcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeHash_Deterministic(t *testing.T) {
	dir := t.TempDir()

	f1 := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(f1, []byte("package noop"), 0o600))

	exts := []ExtensionInfo{
		{Name: "alpha", Dir: dir, GoFiles: []string{f1}},
	}

	h1, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	h2, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	assert.Equal(t, h1, h2)
}

func TestComputeHash_IncludesRuntimePlatform(t *testing.T) {
	got, err := ComputeHash(nil, "", "", false, "")
	require.NoError(t, err)

	payload := "go" + runtime.Version() + "\n" +
		"os:" + runtime.GOOS + "\n" +
		"arch:" + runtime.GOARCH + "\n" +
		"cgo:" + strconv.FormatBool(launcherBuildContext().CgoEnabled) + "\n" +
		"headless:false\n" +
		"agentLoop:\n" +
		"version:\n"
	sum := sha256.Sum256([]byte(payload))

	assert.Equal(t, hex.EncodeToString(sum[:]), got)
}

func TestGoCommandEnv_ForcesHostBuildTarget(t *testing.T) {
	got := envMap(goCommandEnv([]string{
		"GOOS=plan9",
		"GOARCH=386",
		"GOFLAGS=-tags=custom",
		"CGO_ENABLED=0",
		"GOPRIVATE=example.com/private",
		"PATH=/bin",
	}))

	assert.Equal(t, runtime.GOOS, got["GOOS"])
	assert.Equal(t, runtime.GOARCH, got["GOARCH"])
	assert.Empty(t, got["GOFLAGS"])
	assert.Equal(t, launcherCGOEnabledEnv(), got["CGO_ENABLED"])
	assert.Equal(t, "example.com/private", got["GOPRIVATE"])
	assert.Equal(t, "/bin", got["PATH"])
}

func TestComputeHash_SortedByName(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.go")
	f2 := filepath.Join(dir, "b.go")

	require.NoError(t, os.WriteFile(f1, []byte("package a"), 0o600))
	require.NoError(t, os.WriteFile(f2, []byte("package b"), 0o600))

	exts1 := []ExtensionInfo{
		{Name: "alpha", Dir: dir, GoFiles: []string{f1}},
		{Name: "beta", Dir: dir, GoFiles: []string{f2}},
	}
	exts2 := []ExtensionInfo{
		{Name: "beta", Dir: dir, GoFiles: []string{f2}},
		{Name: "alpha", Dir: dir, GoFiles: []string{f1}},
	}

	h1, err := ComputeHash(exts1, "", "", false, "")
	require.NoError(t, err)

	h2, err := ComputeHash(exts2, "", "", false, "")
	require.NoError(t, err)

	assert.Equal(t, h1, h2)
}

func TestComputeHash_DifferentContent(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.go")
	f2 := filepath.Join(dir, "b.go")

	require.NoError(t, os.WriteFile(f1, []byte("package a"), 0o600))
	require.NoError(t, os.WriteFile(f2, []byte("package different"), 0o600))

	h1, err := ComputeHash([]ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{f1}}}, "", "", false, "")
	require.NoError(t, err)

	h2, err := ComputeHash([]ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{f2}}}, "", "", false, "")
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2)
}

func TestComputeHash_DifferentNames(t *testing.T) {
	dir := t.TempDir()

	f := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(f, []byte("package ext"), 0o600))

	h1, err := ComputeHash([]ExtensionInfo{{Name: "alpha", Dir: dir, GoFiles: []string{f}}}, "", "", false, "")
	require.NoError(t, err)

	h2, err := ComputeHash([]ExtensionInfo{{Name: "beta", Dir: dir, GoFiles: []string{f}}}, "", "", false, "")
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2)
}

func TestComputeHash_ReadError(t *testing.T) {
	exts := []ExtensionInfo{
		{Name: "x", Dir: "/nonexistent", GoFiles: []string{"/nonexistent/missing.go"}},
	}

	_, err := ComputeHash(exts, "", "", false, "")
	require.Error(t, err)
}

func TestComputeHash_GoModChangesHash(t *testing.T) {
	dir := t.TempDir()

	f := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(f, []byte("package ext"), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{f}}}

	h1, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	goMod := filepath.Join(dir, "go.mod")
	require.NoError(t, os.WriteFile(goMod, []byte("module github.com/weave-agent/weave-ext-test\ngo 1.22\n"), 0o600))

	h2, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2)
}

func TestComputeHash_GoSumChangesHash(t *testing.T) {
	dir := t.TempDir()

	f := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(f, []byte("package ext"), 0o600))

	goMod := filepath.Join(dir, "go.mod")
	require.NoError(t, os.WriteFile(goMod, []byte("module github.com/weave-agent/weave-ext-test\ngo 1.22\n"), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{f}}}

	h1, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	goSum := filepath.Join(dir, "go.sum")
	require.NoError(t, os.WriteFile(goSum, []byte("example.com/pkg v1.0.0 h1:abc123\n"), 0o600))

	h2, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, "go.sum change should produce different hash")
}

func TestComputeHash_GoSumIgnoredForShim(t *testing.T) {
	dir := t.TempDir()

	f := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(f, []byte("package ext"), 0o600))

	goMod := filepath.Join(dir, "go.mod")
	require.NoError(t, os.WriteFile(goMod, []byte(shimSentinel+"module github.com/weave-agent/weave/ext/x\ngo 1.22\n"), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{f}}}

	h1, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	goSum := filepath.Join(dir, "go.sum")
	require.NoError(t, os.WriteFile(goSum, []byte("example.com/pkg v1.0.0 h1:abc123\n"), 0o600))

	h2, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	assert.Equal(t, h1, h2, "go.sum should not affect hash for shim go.mod extensions")
}

func TestComputeHash_HeadlessDiffers(t *testing.T) {
	dir := t.TempDir()

	f := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(f, []byte("package ext"), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{f}}}

	h1, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	h2, err := ComputeHash(exts, "", "", true, "")
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, "headless flag must affect hash")
}

func TestComputeHash_AgentLoopDiffers(t *testing.T) {
	dir := t.TempDir()

	f := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(f, []byte("package ext"), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{f}}}

	h1, err := ComputeHash(exts, "", "", false, "loop")
	require.NoError(t, err)

	h2, err := ComputeHash(exts, "", "", false, "custom-loop")
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, "agent loop must affect hash")
}

func TestComputeHash_UnreferencedMdFilesIgnored(t *testing.T) {
	dir := t.TempDir()

	f := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(f, []byte("package ext"), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{f}}}

	h1, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	mdFile := filepath.Join(dir, "agents", "test.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(mdFile), 0o755))
	require.NoError(t, os.WriteFile(mdFile, []byte("# Test Agent\n"), 0o600))

	h2, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	assert.Equal(t, h1, h2, "unreferenced .md files should not affect the launcher hash")
}

func TestComputeHash_EmbeddedSbplFileChangesHash(t *testing.T) {
	dir := t.TempDir()

	goFile := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(goFile, []byte(`package ext

import _ "embed"

//go:embed policies/default.sbpl
var policy string
`), 0o600))

	policyFile := filepath.Join(dir, "policies", "default.sbpl")
	require.NoError(t, os.MkdirAll(filepath.Dir(policyFile), 0o750))
	require.NoError(t, os.WriteFile(policyFile, []byte("(version 1)\n"), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{goFile}}}

	h1, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(policyFile, []byte("(version 2)\n"), 0o600))

	h2, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, "embedded .sbpl file changes should produce different hash")
}

func TestComputeHash_CoreDirEmbeddedResourceChangesHash(t *testing.T) {
	dir := t.TempDir()

	goFile := filepath.Join(dir, "policy.go")
	require.NoError(t, os.WriteFile(goFile, []byte(`package core

import _ "embed"

//go:embed policies/default.sbpl
var policy string
`), 0o600))

	policyFile := filepath.Join(dir, "policies", "default.sbpl")
	require.NoError(t, os.MkdirAll(filepath.Dir(policyFile), 0o750))
	require.NoError(t, os.WriteFile(policyFile, []byte("(version 1)\n"), 0o600))

	h1, err := ComputeHash(nil, "", "", false, "", dir)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(policyFile, []byte("(version 2)\n"), 0o600))

	h2, err := ComputeHash(nil, "", "", false, "", dir)
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, "embedded files under core dirs should affect the launcher hash")
}

func TestComputeHash_LocalReplaceEmbeddedResourceChangesHash(t *testing.T) {
	depDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(depDir, "go.mod"), []byte("module example.com/shared\n\ngo 1.22\n"), 0o600))

	depGoFile := filepath.Join(depDir, "shared.go")
	require.NoError(t, os.WriteFile(depGoFile, []byte(`package shared

import _ "embed"

//go:embed templates/prompt.txt
var prompt string
`), 0o600))

	templateFile := filepath.Join(depDir, "templates", "prompt.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(templateFile), 0o750))
	require.NoError(t, os.WriteFile(templateFile, []byte("prompt v1\n"), 0o600))

	extDir := t.TempDir()
	extFile := filepath.Join(extDir, "ext.go")
	require.NoError(t, os.WriteFile(extFile, []byte("package ext\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "go.mod"), fmt.Appendf(nil, `module example.com/ext

go 1.22

require example.com/shared v0.0.0

replace example.com/shared => %s
`, depDir), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: extDir, GoFiles: []string{extFile}}}

	h1, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(templateFile, []byte("prompt v2\n"), 0o600))

	h2, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, "embedded files in local replace modules should affect the launcher hash")
}

func TestComputeHash_RecursiveLocalReplaceChangesHash(t *testing.T) {
	nestedDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(nestedDir, "go.mod"), []byte("module example.com/nested\n\ngo 1.22\n"), 0o600))

	nestedFile := filepath.Join(nestedDir, "nested.go")
	require.NoError(t, os.WriteFile(nestedFile, []byte("package nested\nconst Version = 1\n"), 0o600))

	sharedDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sharedDir, "go.mod"), fmt.Appendf(nil, `module example.com/shared

go 1.22

require example.com/nested v0.0.0

replace example.com/nested => %s
`, nestedDir), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(sharedDir, "shared.go"), []byte("package shared\n"), 0o600))

	extDir := t.TempDir()
	extFile := filepath.Join(extDir, "ext.go")
	require.NoError(t, os.WriteFile(extFile, []byte("package ext\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "go.mod"), fmt.Appendf(nil, `module example.com/ext

go 1.22

require example.com/shared v0.0.0

replace example.com/shared => %s
`, sharedDir), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: extDir, GoFiles: []string{extFile}}}

	h1, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(nestedFile, []byte("package nested\nconst Version = 2\n"), 0o600))

	h2, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, "recursive local replace modules should affect the launcher hash")
}

func TestComputeHash_RootLocalReplaceChangesHash(t *testing.T) {
	sharedDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sharedDir, "go.mod"), []byte("module example.com/shared\n\ngo 1.22\n"), 0o600))
	sharedFile := filepath.Join(sharedDir, "shared.go")
	require.NoError(t, os.WriteFile(sharedFile, []byte("package shared\n\nconst Value = 1\n"), 0o600))

	moduleRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(moduleRoot, "go.mod"), fmt.Appendf(nil, `module github.com/weave-agent/weave

go 1.22

require example.com/shared v0.0.0

replace example.com/shared => %s
`, sharedDir), 0o600))

	h1, err := ComputeHash(nil, moduleRoot, "", false, "")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(sharedFile, []byte("package shared\n\nconst Value = 2\n"), 0o600))

	h2, err := ComputeHash(nil, moduleRoot, "", false, "")
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, "root module local replace contents should affect the launcher hash")
}

func TestComputeHash_EmbeddedGlobPatternChangesHash(t *testing.T) {
	dir := t.TempDir()

	goFile := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(goFile, []byte(`package ext

import "embed"

//go:embed prompts/*.txt
var prompts embed.FS
`), 0o600))

	promptsDir := filepath.Join(dir, "prompts")
	require.NoError(t, os.MkdirAll(promptsDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "a.txt"), []byte("alpha\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "b.txt"), []byte("beta\n"), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{goFile}}}

	h1, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "b.txt"), []byte("beta updated\n"), 0o600))

	h2, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, "embedded glob matches should affect the launcher hash")
}

func TestComputeHash_EmbeddedMultiplePatternsChangeHash(t *testing.T) {
	dir := t.TempDir()

	goFile := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(goFile, []byte(`package ext

import "embed"

//go:embed one.txt two.txt
var files embed.FS
`), 0o600))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "one.txt"), []byte("one\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "two.txt"), []byte("two\n"), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{goFile}}}

	h1, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "two.txt"), []byte("two updated\n"), 0o600))

	h2, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, "all embedded patterns on a directive should affect the launcher hash")
}

func TestComputeHash_EmbeddedDirectoryChangesHashAndRespectsModuleBoundary(t *testing.T) {
	dir := t.TempDir()

	goFile := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(goFile, []byte(`package ext

import "embed"

//go:embed assets
var assets embed.FS
`), 0o600))

	nestedFile := filepath.Join(dir, "assets", "nested", "value.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(nestedFile), 0o750))
	require.NoError(t, os.WriteFile(nestedFile, []byte("nested v1\n"), 0o600))

	submoduleDir := filepath.Join(dir, "assets", "submodule")
	require.NoError(t, os.MkdirAll(submoduleDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(submoduleDir, "go.mod"), []byte("module example.com/submodule\n\ngo 1.22\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(submoduleDir, "ignored.txt"), []byte("ignored v1\n"), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{goFile}}}

	h1, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(submoduleDir, "ignored.txt"), []byte("ignored v2\n"), 0o600))

	h2, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)
	assert.Equal(t, h1, h2, "embedded directory hashing should not cross nested module boundaries")

	require.NoError(t, os.WriteFile(nestedFile, []byte("nested v2\n"), 0o600))

	h3, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)
	assert.NotEqual(t, h2, h3, "embedded directory file changes should affect the launcher hash")
}

func TestComputeHash_EmbedPatternMustMatchFiles(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
	}{
		{name: "missing file", pattern: "missing.txt"},
		{name: "unmatched glob", pattern: "assets/*.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			goFile := filepath.Join(dir, "ext.go")
			require.NoError(t, os.WriteFile(goFile, fmt.Appendf(nil, `package ext

import "embed"

//go:embed %s
var files embed.FS
`, tt.pattern), 0o600))

			if strings.HasPrefix(tt.pattern, "assets/") {
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o750))
			}

			_, err := ComputeHash([]ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{goFile}}}, "", "", false, "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "matched no files")
		})
	}
}

func TestComputeHash_BuildTaggedEmbedFileIgnored(t *testing.T) {
	dir := t.TempDir()

	goFile := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(goFile, []byte("package ext\n"), 0o600))

	taggedFile := filepath.Join(dir, "tagged.go")
	require.NoError(t, os.WriteFile(taggedFile, []byte(`//go:build inactive_launcher_hash_test

package ext

import "embed"

//go:embed missing.txt
var files embed.FS
`), 0o600))

	_, err := ComputeHash([]ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{goFile, taggedFile}}}, "", "", false, "")
	require.NoError(t, err)
}

func TestComputeHash_EmbeddedQuotedAndRawPatternsChangeHash(t *testing.T) {
	dir := t.TempDir()

	goFile := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(goFile, []byte("package ext\n\nimport \"embed\"\n\n//go:embed \"space file.txt\" `raw.txt`\nvar files embed.FS\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "space file.txt"), []byte("space v1\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "raw.txt"), []byte("raw v1\n"), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{goFile}}}

	h1, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "space file.txt"), []byte("space v2\n"), 0o600))

	h2, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)
	assert.NotEqual(t, h1, h2, "quoted go:embed pattern matches should affect the launcher hash")
}

func TestComputeHash_EmbeddedDirectoryHiddenFilesRequireAllPrefix(t *testing.T) {
	dir := t.TempDir()

	goFile := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(goFile, []byte(`package ext

import "embed"

//go:embed assets
var assets embed.FS
`), 0o600))

	visibleFile := filepath.Join(dir, "assets", "visible.txt")
	hiddenFile := filepath.Join(dir, "assets", ".secret")

	require.NoError(t, os.MkdirAll(filepath.Dir(visibleFile), 0o750))
	require.NoError(t, os.WriteFile(visibleFile, []byte("visible v1\n"), 0o600))
	require.NoError(t, os.WriteFile(hiddenFile, []byte("hidden v1\n"), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{goFile}}}

	h1, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(hiddenFile, []byte("hidden v2\n"), 0o600))

	h2, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)
	assert.Equal(t, h1, h2, "directory embeds without all: should ignore hidden files")

	require.NoError(t, os.WriteFile(goFile, []byte(`package ext

import "embed"

//go:embed all:assets
var assets embed.FS
`), 0o600))

	h3, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(hiddenFile, []byte("hidden v3\n"), 0o600))

	h4, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)
	assert.NotEqual(t, h3, h4, "directory embeds with all: should include hidden files")
}

func TestComputeHash_EmbeddedGlobAndDirectoryAddRemoveFiles(t *testing.T) {
	tests := []struct {
		name      string
		directive string
		addPath   string
		removeDir bool
	}{
		{name: "glob", directive: "assets/*.txt", addPath: filepath.Join("assets", "b.txt")},
		{name: "directory", directive: "assets", addPath: filepath.Join("assets", "nested", "b.txt"), removeDir: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			goFile := filepath.Join(dir, "ext.go")
			require.NoError(t, os.WriteFile(goFile, fmt.Appendf(nil, `package ext

import "embed"

//go:embed %s
var assets embed.FS
`, tt.directive), 0o600))

			initialFile := filepath.Join(dir, "assets", "a.txt")
			require.NoError(t, os.MkdirAll(filepath.Dir(initialFile), 0o750))
			require.NoError(t, os.WriteFile(initialFile, []byte("a\n"), 0o600))

			exts := []ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{goFile}}}

			h1, err := ComputeHash(exts, "", "", false, "")
			require.NoError(t, err)

			addedFile := filepath.Join(dir, tt.addPath)
			require.NoError(t, os.MkdirAll(filepath.Dir(addedFile), 0o750))
			require.NoError(t, os.WriteFile(addedFile, []byte("b\n"), 0o600))

			h2, err := ComputeHash(exts, "", "", false, "")
			require.NoError(t, err)
			assert.NotEqual(t, h1, h2, "adding embedded files should affect the launcher hash")

			if tt.removeDir {
				require.NoError(t, os.RemoveAll(filepath.Dir(addedFile)))
			} else {
				require.NoError(t, os.Remove(addedFile))
			}

			h3, err := ComputeHash(exts, "", "", false, "")
			require.NoError(t, err)
			assert.Equal(t, h1, h3, "removing the added embedded file should restore the original hash")
		})
	}
}

func TestComputeHash_EmbeddedPatternRelativeToPackageDirectory(t *testing.T) {
	dir := t.TempDir()

	pkgDir := filepath.Join(dir, "pkg")
	goFile := filepath.Join(pkgDir, "ext.go")
	require.NoError(t, os.MkdirAll(filepath.Join(pkgDir, "templates"), 0o750))
	require.NoError(t, os.WriteFile(goFile, []byte(`package pkg

import "embed"

//go:embed templates/*.txt
var templates embed.FS
`), 0o600))

	templateFile := filepath.Join(pkgDir, "templates", "prompt.txt")
	require.NoError(t, os.WriteFile(templateFile, []byte("prompt v1\n"), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{goFile}}}

	h1, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(templateFile, []byte("prompt v2\n"), 0o600))

	h2, err := ComputeHash(exts, "", "", false, "")
	require.NoError(t, err)
	assert.NotEqual(t, h1, h2, "go:embed patterns should be resolved relative to their package directory")
}

func TestComputeHash_EmbeddedInvalidMatchesReturnError(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     string
	}{
		{name: "vendor", filePath: filepath.Join("assets", "vendor", "value.txt"), want: "vendor directory"},
		{name: "invalid name", filePath: filepath.Join("assets", "bad'name.txt"), want: "invalid name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			goFile := filepath.Join(dir, "ext.go")
			require.NoError(t, os.WriteFile(goFile, []byte(`package ext

import "embed"

//go:embed assets
var assets embed.FS
`), 0o600))

			embeddedFile := filepath.Join(dir, tt.filePath)
			require.NoError(t, os.MkdirAll(filepath.Dir(embeddedFile), 0o750))
			require.NoError(t, os.WriteFile(embeddedFile, []byte("value\n"), 0o600))

			_, err := ComputeHash([]ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{goFile}}}, "", "", false, "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestGenerateMainGo_UIExtFilteredByBuild(t *testing.T) {
	// Build() filters UI extensions before calling GenerateMainGo,
	// so GenerateMainGo only receives non-UI extensions.
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "bash", Dir: "/tmp/exts/bash", ModulePath: "github.com/weave-agent/weave/ext/bash"},
	}

	require.NoError(t, GenerateMainGo(dir, exts, "loop"))

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, `_ "github.com/weave-agent/weave/ext/bash"`)
	assert.NotContains(t, s, `_ "github.com/weave-agent/weave/ext/tui-diffview"`)
}

func TestGenerateMainGo_UIExtIncludedInInteractive(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "bash", Dir: "/tmp/exts/bash", ModulePath: "github.com/weave-agent/weave/ext/bash"},
		{Name: "tui-diffview", Dir: "/tmp/exts/tui-diffview", ModulePath: "github.com/weave-agent/weave/ext/tui-diffview", IsUIExt: true},
	}

	require.NoError(t, GenerateMainGo(dir, exts, "loop"))

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, `_ "github.com/weave-agent/weave/ext/bash"`)
	assert.Contains(t, s, `_ "github.com/weave-agent/weave/ext/tui-diffview"`)
}

func TestGenerateGoMod_Content(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "noop", Dir: "/tmp/exts/noop", ModulePath: "github.com/weave-agent/weave/ext/noop"},
	}

	require.NoError(t, GenerateGoMod(dir, "/tmp/weave", "", exts))

	content, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "module github.com/weave-agent/weave/built")
	assert.Contains(t, s, goVersion())
	assert.Contains(t, s, "github.com/weave-agent/weave v0.0.0")
	assert.Contains(t, s, "github.com/weave-agent/weave/ext/noop v0.0.0")
	assert.Contains(t, s, "replace github.com/weave-agent/weave => /tmp/weave")
	assert.Contains(t, s, "replace github.com/weave-agent/weave/ext/noop => /tmp/exts/noop")
}

func TestGenerateGoMod_NestedModulePath(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "bash", Dir: "/tmp/exts/tools/bash", ModulePath: "github.com/weave-agent/weave/ext/tools/bash"},
	}

	require.NoError(t, GenerateGoMod(dir, "/tmp/weave", "", exts))

	content, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "github.com/weave-agent/weave/ext/tools/bash v0.0.0")
	assert.Contains(t, s, "replace github.com/weave-agent/weave/ext/tools/bash => /tmp/exts/tools/bash")
}

func TestGenerateGoMod_IncludesRecursiveLocalReplaces(t *testing.T) {
	nestedDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(nestedDir, "go.mod"), []byte("module example.com/nested\n\ngo 1.22\n"), 0o600))

	sharedDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sharedDir, "go.mod"), fmt.Appendf(nil, `module example.com/shared

go 1.22

require example.com/nested v0.0.0

replace example.com/nested => %s
`, nestedDir), 0o600))

	extDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "go.mod"), fmt.Appendf(nil, `module example.com/ext

go 1.22

require example.com/shared v0.0.0

replace example.com/shared => %s
`, sharedDir), 0o600))

	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "ext", Dir: extDir, ModulePath: "example.com/ext"},
	}

	require.NoError(t, GenerateGoMod(dir, "/tmp/weave", "", exts))

	content, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "example.com/shared v0.0.0")
	assert.Contains(t, s, "example.com/nested v0.0.0")
	assert.Contains(t, s, "replace example.com/shared => "+sharedDir)
	assert.Contains(t, s, "replace example.com/nested => "+nestedDir)
}

func TestGenerateGoMod_IncludesRootLocalReplaces(t *testing.T) {
	sharedDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sharedDir, "go.mod"), []byte("module example.com/shared\n\ngo 1.22\n"), 0o600))

	moduleRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(moduleRoot, "go.mod"), fmt.Appendf(nil, `module github.com/weave-agent/weave

go 1.22

require example.com/shared v0.0.0

replace example.com/shared => %s
`, sharedDir), 0o600))

	dir := t.TempDir()
	require.NoError(t, GenerateGoMod(dir, moduleRoot, "", nil))

	content, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "example.com/shared v0.0.0")
	assert.Contains(t, s, "replace example.com/shared => "+sharedDir)
}

func TestGenerateMainGo_Content(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "noop", Dir: "/tmp/exts/noop", ModulePath: "github.com/weave-agent/weave/ext/noop"},
		{Name: "log", Dir: "/tmp/exts/log", ModulePath: "github.com/weave-agent/weave/ext/log"},
	}

	require.NoError(t, GenerateMainGo(dir, exts, "loop"))

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "package main")
	assert.Contains(t, s, `"github.com/weave-agent/weave/sdk"`)
	assert.Contains(t, s, `"github.com/weave-agent/weave/sdk/model"`)
	assert.Contains(t, s, `"github.com/weave-agent/weave/internal/wire"`)
	assert.Contains(t, s, `"github.com/weave-agent/weave/bus"`)
	assert.Contains(t, s, `"strings"`)
	assert.Contains(t, s, `_ "github.com/weave-agent/weave/ext/noop"`)
	assert.Contains(t, s, `_ "github.com/weave-agent/weave/ext/log"`)
	assert.Contains(t, s, "bus.New()")
	assert.Contains(t, s, "wire.WireWithCore")
	assert.Contains(t, s, `AgentLoop: "loop"`)
	assert.Contains(t, s, "strings.CutPrefix")
	assert.Contains(t, s, `if a == "--"`)
	assert.Contains(t, s, "settings.LoadFullConfig")
	assert.Contains(t, s, "fullCfg.SetArgs(filtered)")
	assert.Contains(t, s, "settings.EnsureLocalSettingsExcluded")
	assert.Contains(t, s, "settings.ProjectDirFromConfig")
	assert.Contains(t, s, "signal.Notify")
	assert.Contains(t, s, `b.On("agent.end"`)
	assert.Contains(t, s, "wired.Close()")
	assert.Contains(t, s, "shutdown error")
	assert.Contains(t, s, "os.Args = append([]string{os.Args[0]}, filtered...)")
	assert.Contains(t, s, "--weave-headless=")
	assert.Contains(t, s, "--weave-project-dir=")
	assert.Contains(t, s, "--weave-output=")
	assert.Contains(t, s, "--weave-tools=")
	assert.Contains(t, s, "--weave-subagent-id=")
	assert.Contains(t, s, "--weave-sandbox-mode=")
	assert.Contains(t, s, "--weave-model=")
	assert.Contains(t, s, "--weave-messaging=")
	assert.Contains(t, s, "WEAVE_MESSAGING")
	assert.Contains(t, s, "os.Unsetenv(\"WEAVE_SUBAGENT_ID\")")
	assert.Contains(t, s, "os.Unsetenv(\"WEAVE_MESSAGING\")")
	assert.Contains(t, s, "fullCfg.SetProjectDir(projectDir)")
	assert.Contains(t, s, "sdk.HeadlessConfig")
	assert.Contains(t, s, "sdk.SetToolFilter")
	assert.Contains(t, s, "toolNames = []string{}", "empty --weave-tools= should pass empty slice, not nil")
	assert.Contains(t, s, "model.GetModel")
	assert.Contains(t, s, `"encoding/json"`)
	assert.Contains(t, s, `"sync"`)
	assert.Contains(t, s, `"io"`)
	assert.Contains(t, s, `type syncWriter struct`)
	assert.Contains(t, s, `fmt.Fprintln(jsonOut, string(data))`)
	assert.Contains(t, s, `outputMode == "json"`)
	assert.Contains(t, s, `b.OnAll(`)
	assert.Contains(t, s, `"agent.message_start"`)
	assert.Contains(t, s, `"agent.message_end"`)
	assert.Contains(t, s, `"agent.tool_call"`)
	assert.Contains(t, s, `"agent.tool_result"`)
	assert.Contains(t, s, `"model.change"`)
	assert.Contains(t, s, `jsonQueue = make(chan map[string]any, 10000)`)
	assert.Contains(t, s, `jsonWg.Wait()`)
	assert.Contains(t, s, `close(jsonQueue)`)
	assert.Contains(t, s, `"path/filepath"`)
	assert.Contains(t, s, `"github.com/weave-agent/weave/internal/log"`)
	assert.Contains(t, s, "--weave-debug=")
	assert.Contains(t, s, "log.Setup(")
	assert.Contains(t, s, "logDir")
	assert.Contains(t, s, "extraWriters")
	assert.Contains(t, s, "WEAVE_DEBUG")
}

func TestGenerateMainGo_AllExtensionsBlankImported(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "subagent", Dir: "/tmp/exts/custom/subagent", ModulePath: "github.com/weave-agent/weave/ext/custom/subagent"},
	}

	require.NoError(t, GenerateMainGo(dir, exts, "loop"))

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)

	s := string(content)
	// All extensions are blank-imported; no special-casing for subagent.
	assert.Contains(t, s, `_ "github.com/weave-agent/weave/ext/custom/subagent"`)
	assert.NotContains(t, s, `subagentext`)
	assert.Contains(t, s, `sdk.OutputRedirectPayload{Writer: jsonOut}`)
}

func TestGenerateMainGo_OutputWriterSetterCalled(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "subagent", Dir: "/tmp/exts/tools/subagent", ModulePath: "github.com/weave-agent/weave/ext/tools/subagent"},
	}

	require.NoError(t, GenerateMainGo(dir, exts, "loop"))

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)

	s := string(content)
	// Generic hook is used instead of named import.
	assert.Contains(t, s, `sdk.OutputRedirectPayload{Writer: jsonOut}`)
	assert.Contains(t, s, `jsonOut := &syncWriter{w: os.Stdout}`)
	assert.NotContains(t, s, `subagentext`)
	assert.NotContains(t, s, `SetStdoutWriter`)
}

func TestGenerateMainGo_CustomAgentLoopIncludesAllExtensions(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "loop", Dir: "/tmp/exts/loop", ModulePath: "github.com/weave-agent/weave/ext/loop"},
		{Name: "my-loop", Dir: "/tmp/exts/my-loop", ModulePath: "github.com/weave-agent/weave/ext/my-loop"},
		{Name: "bash", Dir: "/tmp/exts/bash", ModulePath: "github.com/weave-agent/weave/ext/bash"},
	}

	require.NoError(t, GenerateMainGo(dir, exts, "my-loop"))

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)

	s := string(content)
	// optExts includes all extensions; filtering happens at runtime in WireWithCore.
	assert.Contains(t, s, `optExts = []string{"loop", "my-loop", "bash"}`)
	// Blank imports still include all extensions for registration.
	assert.Contains(t, s, `_ "github.com/weave-agent/weave/ext/loop"`)
	assert.Contains(t, s, `_ "github.com/weave-agent/weave/ext/my-loop"`)
	assert.Contains(t, s, `_ "github.com/weave-agent/weave/ext/bash"`)
}

func TestGenerateMainGo_LogSetupBeforeWire(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "noop", Dir: "/tmp/exts/noop", ModulePath: "github.com/weave-agent/weave/ext/noop"},
	}

	require.NoError(t, GenerateMainGo(dir, exts, "loop"))

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)

	s := string(content)
	// log.Setup must appear before wire.WireWithCore so extension wiring logs go to file.
	logSetupIdx := strings.Index(s, "log.Setup(")
	wireIdx := strings.Index(s, "wire.WireWithCore")

	require.NotEqual(t, -1, logSetupIdx, "generated main.go should contain log.Setup")
	require.NotEqual(t, -1, wireIdx, "generated main.go should contain wire.WireWithCore")
	assert.Less(t, logSetupIdx, wireIdx, "log.Setup must be called before wire.WireWithCore")
}

func TestGenerateMainGo_LogSetupUsesHomeDir(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "noop", Dir: "/tmp/exts/noop", ModulePath: "github.com/weave-agent/weave/ext/noop"},
	}

	require.NoError(t, GenerateMainGo(dir, exts, "loop"))

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)

	s := string(content)
	// Log directory should always be ~/.weave/logs.
	assert.Contains(t, s, "filepath.Join(homeDir, \".weave\", \"logs\")", "should use home dir for log path")
	assert.Contains(t, s, "os.UserHomeDir()", "should get home dir for log path")
}

func TestGenerateMainGo_LogSetupIncludesStderrInHeadless(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "noop", Dir: "/tmp/exts/noop", ModulePath: "github.com/weave-agent/weave/ext/noop"},
	}

	require.NoError(t, GenerateMainGo(dir, exts, "loop"))

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)

	s := string(content)
	// In headless mode, os.Stderr should be added as an extra writer.
	assert.Contains(t, s, "extraWriters = append(extraWriters, os.Stderr)")
	assert.Contains(t, s, "if headless {")
}

func TestBuild_WithTrivialExtension(t *testing.T) {
	moduleRoot, err := findModuleRoot()
	if err != nil {
		t.Skipf("cannot locate module root: %v", err)
	}

	buildDir := t.TempDir()
	extDir := t.TempDir()

	extCode := `package noop

import "github.com/weave-agent/weave/sdk"

func init() {
	sdk.RegisterExtension[struct{}]("noop", func(cfg sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("noop", func(b sdk.Bus) error { return nil }), nil
	})
}
`
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "noop.go"), []byte(extCode), 0o600))

	exts := []ExtensionInfo{
		{Name: "noop", Dir: extDir, GoFiles: []string{filepath.Join(extDir, "noop.go")}},
	}

	binaryPath, err := Build(context.Background(), buildDir, moduleRoot, "", "noop", false, exts)
	require.NoError(t, err, "Build failed")

	_, err = os.Stat(binaryPath)
	assert.NoError(t, err, "binary not found at %s", binaryPath)
}

func TestBuild_UsesContextForGoCommands(t *testing.T) {
	moduleRoot, err := findModuleRoot()
	if err != nil {
		t.Skipf("cannot locate module root: %v", err)
	}

	buildDir := t.TempDir()
	extDir := t.TempDir()
	extFile := filepath.Join(extDir, "noop.go")
	require.NoError(t, os.WriteFile(extFile, []byte("package noop\n"), 0o600))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = Build(ctx, buildDir, moduleRoot, "", "noop", false, []ExtensionInfo{
		{Name: "noop", Dir: extDir, GoFiles: []string{extFile}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "build: go mod tidy")
	assert.ErrorIs(t, err, context.Canceled, "expected canceled context error, got %v", err)
}

func findModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("find module root: %w", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	return "", os.ErrNotExist
}

func TestExtModulePath_FallbackUsesNewModulePath(t *testing.T) {
	ext := ExtensionInfo{Name: "bash", Dir: "/tmp/exts/bash"}
	assert.Equal(t, "github.com/weave-agent/weave/ext/bash", extModulePath(ext))
}

func TestExtModulePath_ModulePathFromGoMod(t *testing.T) {
	ext := ExtensionInfo{Name: "bash", Dir: "/tmp/exts/bash", ModulePath: "github.com/weave-agent/weave-bash"}
	assert.Equal(t, "github.com/weave-agent/weave-bash", extModulePath(ext))
}

func TestEnsureExtGoMod_UsesNewModulePath(t *testing.T) {
	dir := t.TempDir()
	ext := ExtensionInfo{Name: "myext", Dir: dir}
	moduleRoot := "/tmp/weave-root"

	require.NoError(t, ensureExtGoMod(ext, moduleRoot, ""))

	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	require.NoError(t, err)

	s := string(data)
	assert.Contains(t, s, "module github.com/weave-agent/weave/ext/myext")
	assert.Contains(t, s, "require github.com/weave-agent/weave v0.0.0")
	assert.Contains(t, s, "replace github.com/weave-agent/weave => "+moduleRoot)
}

func TestGenerateGoMod_UsesNewModulePathForRoot(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "noop", Dir: "/tmp/exts/noop", ModulePath: "github.com/weave-agent/weave-ext-noop"},
	}

	require.NoError(t, GenerateGoMod(dir, "/tmp/weave-root", "", exts))

	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	require.NoError(t, err)

	s := string(data)
	assert.Contains(t, s, "module github.com/weave-agent/weave/built")
	assert.Contains(t, s, "github.com/weave-agent/weave v0.0.0")
	assert.Contains(t, s, "replace github.com/weave-agent/weave => /tmp/weave-root")
}

func envMap(env []string) map[string]string {
	result := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}

		result[key] = value
	}

	return result
}

// Suppress unused import warning.
var _ = strings.TrimSpace
