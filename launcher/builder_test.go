package launcher

import (
	"fmt"
	"os"
	"path/filepath"
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

	h1, err := ComputeHash(exts, "", false)
	require.NoError(t, err)

	h2, err := ComputeHash(exts, "", false)
	require.NoError(t, err)

	assert.Equal(t, h1, h2)
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

	h1, err := ComputeHash(exts1, "", false)
	require.NoError(t, err)

	h2, err := ComputeHash(exts2, "", false)
	require.NoError(t, err)

	assert.Equal(t, h1, h2)
}

func TestComputeHash_DifferentContent(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.go")
	f2 := filepath.Join(dir, "b.go")

	require.NoError(t, os.WriteFile(f1, []byte("package a"), 0o600))
	require.NoError(t, os.WriteFile(f2, []byte("package different"), 0o600))

	h1, err := ComputeHash([]ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{f1}}}, "", false)
	require.NoError(t, err)

	h2, err := ComputeHash([]ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{f2}}}, "", false)
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2)
}

func TestComputeHash_DifferentNames(t *testing.T) {
	dir := t.TempDir()

	f := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(f, []byte("package ext"), 0o600))

	h1, err := ComputeHash([]ExtensionInfo{{Name: "alpha", Dir: dir, GoFiles: []string{f}}}, "", false)
	require.NoError(t, err)

	h2, err := ComputeHash([]ExtensionInfo{{Name: "beta", Dir: dir, GoFiles: []string{f}}}, "", false)
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2)
}

func TestComputeHash_ReadError(t *testing.T) {
	exts := []ExtensionInfo{
		{Name: "x", Dir: "/nonexistent", GoFiles: []string{"/nonexistent/missing.go"}},
	}

	_, err := ComputeHash(exts, "", false)
	require.Error(t, err)
}

func TestComputeHash_GoModChangesHash(t *testing.T) {
	dir := t.TempDir()

	f := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(f, []byte("package ext"), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{f}}}

	h1, err := ComputeHash(exts, "", false)
	require.NoError(t, err)

	goMod := filepath.Join(dir, "go.mod")
	require.NoError(t, os.WriteFile(goMod, []byte("module weave/ext/x\ngo 1.22\n"), 0o600))

	h2, err := ComputeHash(exts, "", false)
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2)
}

func TestComputeHash_GoSumChangesHash(t *testing.T) {
	dir := t.TempDir()

	f := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(f, []byte("package ext"), 0o600))

	goMod := filepath.Join(dir, "go.mod")
	require.NoError(t, os.WriteFile(goMod, []byte("module weave/ext/x\ngo 1.22\n"), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{f}}}

	h1, err := ComputeHash(exts, "", false)
	require.NoError(t, err)

	goSum := filepath.Join(dir, "go.sum")
	require.NoError(t, os.WriteFile(goSum, []byte("example.com/pkg v1.0.0 h1:abc123\n"), 0o600))

	h2, err := ComputeHash(exts, "", false)
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, "go.sum change should produce different hash")
}

func TestComputeHash_GoSumIgnoredForShim(t *testing.T) {
	dir := t.TempDir()

	f := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(f, []byte("package ext"), 0o600))

	goMod := filepath.Join(dir, "go.mod")
	require.NoError(t, os.WriteFile(goMod, []byte(shimSentinel+"module weave/ext/x\ngo 1.22\n"), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{f}}}

	h1, err := ComputeHash(exts, "", false)
	require.NoError(t, err)

	goSum := filepath.Join(dir, "go.sum")
	require.NoError(t, os.WriteFile(goSum, []byte("example.com/pkg v1.0.0 h1:abc123\n"), 0o600))

	h2, err := ComputeHash(exts, "", false)
	require.NoError(t, err)

	assert.Equal(t, h1, h2, "go.sum should not affect hash for shim go.mod extensions")
}

func TestComputeHash_HeadlessDiffers(t *testing.T) {
	dir := t.TempDir()

	f := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(f, []byte("package ext"), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{f}}}

	h1, err := ComputeHash(exts, "", false)
	require.NoError(t, err)

	h2, err := ComputeHash(exts, "", true)
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, "headless flag must affect hash")
}

func TestComputeHash_MdFilesChangeHash(t *testing.T) {
	dir := t.TempDir()

	f := filepath.Join(dir, "ext.go")
	require.NoError(t, os.WriteFile(f, []byte("package ext"), 0o600))

	exts := []ExtensionInfo{{Name: "x", Dir: dir, GoFiles: []string{f}}}

	h1, err := ComputeHash(exts, "", false)
	require.NoError(t, err)

	mdFile := filepath.Join(dir, "agents", "test.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(mdFile), 0o755))
	require.NoError(t, os.WriteFile(mdFile, []byte("# Test Agent\n"), 0o600))

	h2, err := ComputeHash(exts, "", false)
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, ".md file change should produce different hash")
}

func TestGenerateMainGo_UIExtFilteredByBuild(t *testing.T) {
	// Build() filters UI extensions before calling GenerateMainGo,
	// so GenerateMainGo only receives non-UI extensions.
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "bash", Dir: "/tmp/exts/bash", ModulePath: "weave/ext/bash"},
	}

	require.NoError(t, GenerateMainGo(dir, exts, "loop"))

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, `_ "weave/ext/bash"`)
	assert.NotContains(t, s, `_ "weave/ext/diff-viewer"`)
}

func TestGenerateMainGo_UIExtIncludedInInteractive(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "bash", Dir: "/tmp/exts/bash", ModulePath: "weave/ext/bash"},
		{Name: "diff-viewer", Dir: "/tmp/exts/diff-viewer", ModulePath: "weave/ext/diff-viewer", IsUIExt: true},
	}

	require.NoError(t, GenerateMainGo(dir, exts, "loop"))

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, `_ "weave/ext/bash"`)
	assert.Contains(t, s, `_ "weave/ext/diff-viewer"`)
}

func TestGenerateGoMod_Content(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "noop", Dir: "/tmp/exts/noop", ModulePath: "weave/ext/noop"},
	}

	require.NoError(t, GenerateGoMod(dir, "/tmp/weave", exts))

	content, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "module weave-built")
	assert.Contains(t, s, goVersion())
	assert.Contains(t, s, "weave v0.0.0")
	assert.Contains(t, s, "weave/ext/noop v0.0.0")
	assert.Contains(t, s, "replace weave => /tmp/weave")
	assert.Contains(t, s, "replace weave/ext/noop => /tmp/exts/noop")
}

func TestGenerateGoMod_NestedModulePath(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "bash", Dir: "/tmp/exts/tools/bash", ModulePath: "weave/ext/tools/bash"},
	}

	require.NoError(t, GenerateGoMod(dir, "/tmp/weave", exts))

	content, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "weave/ext/tools/bash v0.0.0")
	assert.Contains(t, s, "replace weave/ext/tools/bash => /tmp/exts/tools/bash")
}

func TestGenerateMainGo_Content(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "noop", Dir: "/tmp/exts/noop", ModulePath: "weave/ext/noop"},
		{Name: "log", Dir: "/tmp/exts/log", ModulePath: "weave/ext/log"},
	}

	require.NoError(t, GenerateMainGo(dir, exts, "loop"))

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, "package main")
	assert.Contains(t, s, `"weave/sdk"`)
	assert.Contains(t, s, `"weave/sdk/model"`)
	assert.Contains(t, s, `"weave/sdk/wire"`)
	assert.Contains(t, s, `"weave/bus"`)
	assert.Contains(t, s, `"strings"`)
	assert.Contains(t, s, `_ "weave/ext/noop"`)
	assert.Contains(t, s, `_ "weave/ext/log"`)
	assert.Contains(t, s, "bus.New()")
	assert.Contains(t, s, "wire.WireWithCore")
	assert.Contains(t, s, `AgentLoop: "loop"`)
	assert.Contains(t, s, "strings.CutPrefix")
	assert.Contains(t, s, "config.LoadFullConfig")
	assert.Contains(t, s, "config.EnsureLocalSettingsExcluded")
	assert.Contains(t, s, "config.ProjectDirFromConfig")
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
}

func TestGenerateMainGo_CustomSubagentNotTreatedAsBuiltin(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "subagent", Dir: "/tmp/exts/custom/subagent", ModulePath: "weave/ext/custom/subagent"},
	}

	require.NoError(t, GenerateMainGo(dir, exts, "loop"))

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)

	s := string(content)
	// Custom subagent should be blank-imported, not aliased.
	assert.Contains(t, s, `_ "weave/ext/custom/subagent"`)
	assert.NotContains(t, s, `subagentext`)
	assert.NotContains(t, s, `SetStdoutWriter`)
}

func TestGenerateMainGo_WithSubagent(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "subagent", Dir: "/tmp/exts/tools/subagent", ModulePath: "weave/ext/tools/subagent"},
	}

	require.NoError(t, GenerateMainGo(dir, exts, "loop"))

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)

	s := string(content)
	assert.Contains(t, s, `subagentext "weave/ext/tools/subagent"`)
	assert.Contains(t, s, `subagentext.SetStdoutWriter(jsonOut)`)
	assert.Contains(t, s, `jsonOut := &syncWriter{w: os.Stdout}`)
}

func TestGenerateMainGo_CustomAgentLoopExcludesDefaultLoop(t *testing.T) {
	dir := t.TempDir()
	exts := []ExtensionInfo{
		{Name: "loop", Dir: "/tmp/exts/loop", ModulePath: "weave/ext/loop"},
		{Name: "my-loop", Dir: "/tmp/exts/my-loop", ModulePath: "weave/ext/my-loop"},
		{Name: "bash", Dir: "/tmp/exts/bash", ModulePath: "weave/ext/bash"},
	}

	require.NoError(t, GenerateMainGo(dir, exts, "my-loop"))

	content, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)

	s := string(content)
	// optExts should only contain "bash" — both "loop" and "my-loop" are excluded.
	assert.Contains(t, s, `optExts = []string{"bash"}`, "optExts should only have bash")
	assert.NotContains(t, s, `optExts = []string{"loop"`, "loop should not be in optExts")
	// Blank imports still include all extensions for registration.
	assert.Contains(t, s, `_ "weave/ext/loop"`)
	assert.Contains(t, s, `_ "weave/ext/my-loop"`)
	assert.Contains(t, s, `_ "weave/ext/bash"`)
}

func TestBuild_WithTrivialExtension(t *testing.T) {
	moduleRoot, err := findModuleRoot()
	if err != nil {
		t.Skipf("cannot locate module root: %v", err)
	}

	buildDir := t.TempDir()
	extDir := t.TempDir()

	extCode := `package noop

import "weave/sdk"

func init() {
	sdk.RegisterExtension("noop", func(cfg sdk.Config) (sdk.Extension, error) {
		return sdk.NewExtensionFunc("noop", func(b sdk.Bus) error { return nil }), nil
	})
}
`
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "noop.go"), []byte(extCode), 0o600))

	exts := []ExtensionInfo{
		{Name: "noop", Dir: extDir, GoFiles: []string{filepath.Join(extDir, "noop.go")}},
	}

	binaryPath, err := Build(buildDir, moduleRoot, "noop", false, exts)
	require.NoError(t, err, "Build failed")

	_, err = os.Stat(binaryPath)
	assert.NoError(t, err, "binary not found at %s", binaryPath)
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

// Suppress unused import warning.
var _ = strings.TrimSpace
