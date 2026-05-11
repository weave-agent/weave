package instructions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"weave/sdk"
)

// --- discoverContextFiles tests ---

func TestDiscoverContextFiles_NoFilesFound(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()

	files := discoverContextFiles(projectDir, globalDir)
	assert.Empty(t, files)
}

func TestDiscoverContextFiles_SingleFileInProjectDir(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "CLAUDE.md"), []byte("project instructions"), 0o644))

	files := discoverContextFiles(projectDir, globalDir)
	require.Len(t, files, 1)
	assert.Contains(t, files[0].Path, "CLAUDE.md")
	assert.Equal(t, "project instructions", files[0].Content)
}

func TestDiscoverContextFiles_WalkUpPrecedence(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	require.NoError(t, os.MkdirAll(child, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("root instructions"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(child, "CLAUDE.md"), []byte("child instructions"), 0o644))

	files := discoverContextFiles(child, t.TempDir())
	require.Len(t, files, 2)

	// Walk starts at child (closest), prepends each found file.
	// After child: files=[child]. After root: files=[root, child].
	// Root (farthest) ends up first due to prepend semantics.
	assert.Equal(t, filepath.Join(root, "CLAUDE.md"), files[0].Path)
	assert.Equal(t, "root instructions", files[0].Content)
	assert.Equal(t, filepath.Join(child, "CLAUDE.md"), files[1].Path)
	assert.Equal(t, "child instructions", files[1].Content)
}

func TestDiscoverContextFiles_Deduplication(t *testing.T) {
	// globalDir is a parent of projectDir so the walk-up reaches the same file.
	globalDir := t.TempDir()
	projectDir := filepath.Join(globalDir, "sub")

	require.NoError(t, os.MkdirAll(projectDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "CLAUDE.md"), []byte("shared"), 0o644))

	files := discoverContextFiles(projectDir, globalDir)
	require.Len(t, files, 1, "same file found via walk-up and global should be deduplicated")
	assert.Equal(t, "shared", files[0].Content)
}

func TestDiscoverContextFiles_AGENTSMD(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "AGENTS.md"), []byte("agents instructions"), 0o644))

	files := discoverContextFiles(projectDir, globalDir)
	require.Len(t, files, 1)
	assert.Equal(t, "agents instructions", files[0].Content)
}

func TestDiscoverContextFiles_CLAUDEmdPreferredOverAGENTSMD(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "CLAUDE.md"), []byte("claude"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "AGENTS.md"), []byte("agents"), 0o644))

	// CLAUDE.md is checked first, break after first match per directory
	files := discoverContextFiles(projectDir, globalDir)
	require.Len(t, files, 1)
	assert.Equal(t, "claude", files[0].Content)
}

func TestDiscoverContextFiles_AGENTSmdWhenNoCLAUDEmd(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "AGENTS.md"), []byte("agents only"), 0o644))

	files := discoverContextFiles(projectDir, globalDir)
	require.Len(t, files, 1)
	assert.Equal(t, "agents only", files[0].Content)
}

func TestDiscoverContextFiles_GlobalFallback(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "CLAUDE.md"), []byte("global context"), 0o644))

	files := discoverContextFiles(projectDir, globalDir)
	require.Len(t, files, 1)
	assert.Contains(t, files[0].Path, globalDir)
	assert.Equal(t, "global context", files[0].Content)
}

func TestDiscoverContextFiles_EmptyGlobalDir(t *testing.T) {
	projectDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "CLAUDE.md"), []byte("project only"), 0o644))

	files := discoverContextFiles(projectDir, "")
	require.Len(t, files, 1)
	assert.Equal(t, "project only", files[0].Content)
}

// --- loadSystemPrompt tests ---

func TestLoadSystemPrompt_NoFiles(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()

	base, append_ := loadSystemPrompt(projectDir, globalDir)
	assert.Empty(t, base)
	assert.Empty(t, append_)
}

func TestLoadSystemPrompt_ProjectOverride(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".weave"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, ".weave", "SYSTEM.md"), []byte("project system"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "SYSTEM.md"), []byte("global system"), 0o644))

	base, append_ := loadSystemPrompt(projectDir, globalDir)
	assert.Equal(t, "project system", base)
	assert.Empty(t, append_)
}

func TestLoadSystemPrompt_GlobalFallback(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "SYSTEM.md"), []byte("global system"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "APPEND_SYSTEM.md"), []byte("global append"), 0o644))

	base, append_ := loadSystemPrompt(projectDir, globalDir)
	assert.Equal(t, "global system", base)
	assert.Equal(t, "global append", append_)
}

func TestLoadSystemPrompt_BothProjectAndGlobal(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".weave"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, ".weave", "SYSTEM.md"), []byte("project base"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "APPEND_SYSTEM.md"), []byte("global append"), 0o644))

	base, append_ := loadSystemPrompt(projectDir, globalDir)
	assert.Equal(t, "project base", base)
	assert.Equal(t, "global append", append_)
}

func TestLoadSystemPrompt_AppendSystemProjectOverridesGlobal(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".weave"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, ".weave", "APPEND_SYSTEM.md"), []byte("project append"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "APPEND_SYSTEM.md"), []byte("global append"), 0o644))

	base, append_ := loadSystemPrompt(projectDir, globalDir)
	assert.Empty(t, base)
	assert.Equal(t, "project append", append_)
}

// --- formatInstructionsPrompt tests ---

func TestFormatInstructionsPrompt_EmptyInput(t *testing.T) {
	result := formatInstructionsPrompt(nil, "", "")
	assert.Empty(t, result)
}

func TestFormatInstructionsPrompt_ContextFilesOnly(t *testing.T) {
	files := []ContextFile{
		{Path: "/project/CLAUDE.md", Content: "Do stuff"},
	}
	result := formatInstructionsPrompt(files, "", "")
	assert.Contains(t, result, "# Project Context")
	assert.Contains(t, result, "## /project/CLAUDE.md")
	assert.Contains(t, result, "Do stuff")
}

func TestFormatInstructionsPrompt_SystemBaseOnly(t *testing.T) {
	result := formatInstructionsPrompt(nil, "You are a helpful assistant.", "")
	assert.Equal(t, "You are a helpful assistant.", result)
}

func TestFormatInstructionsPrompt_SystemAppendOnly(t *testing.T) {
	result := formatInstructionsPrompt(nil, "", "Always be concise.")
	assert.Equal(t, "Always be concise.", result)
}

func TestFormatInstructionsPrompt_AllCombined(t *testing.T) {
	files := []ContextFile{
		{Path: "/project/CLAUDE.md", Content: "Project rules"},
	}
	result := formatInstructionsPrompt(files, "Base prompt.", "Append this.")
	assert.Contains(t, result, "Base prompt.")
	assert.Contains(t, result, "# Project Context")
	assert.Contains(t, result, "## /project/CLAUDE.md")
	assert.Contains(t, result, "Project rules")
	assert.Contains(t, result, "Append this.")
}

func TestFormatInstructionsPrompt_Ordering(t *testing.T) {
	files := []ContextFile{
		{Path: "/a.md", Content: "AAA"},
	}
	result := formatInstructionsPrompt(files, "BASE", "APPEND")

	baseIdx := assertHasSubstring(t, result, "BASE")
	ctxIdx := assertHasSubstring(t, result, "# Project Context")
	appendIdx := assertHasSubstring(t, result, "APPEND")

	assert.Less(t, baseIdx, ctxIdx, "system base should come before context")
	assert.Less(t, ctxIdx, appendIdx, "context should come before append")
}

func assertHasSubstring(t *testing.T, s, substr string) int {
	t.Helper()

	idx := strings.Index(s, substr)
	require.NotEqual(t, -1, idx, "expected to find %q in result", substr)

	return idx
}

// --- Subscribe integration test ---

func TestSubscribe_PublishesInstructionsLoaded(t *testing.T) {
	projectDir := t.TempDir()
	fakeHome := t.TempDir()
	fakeGlobalDir := filepath.Join(fakeHome, ".weave")
	require.NoError(t, os.MkdirAll(fakeGlobalDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "CLAUDE.md"), []byte("project context"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(fakeGlobalDir, "APPEND_SYSTEM.md"), []byte("append text"), 0o644))

	configPath := filepath.Join(projectDir, ".weave", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	require.NoError(t, os.WriteFile(configPath, []byte(`{"agent_loop":"loop"}`), 0o644))

	ext, err := NewInstructionsExtension(sdk.FilePathConfig(configPath))
	require.NoError(t, err)

	bus := &BusMock{
		PublishFunc: func(event sdk.Event) {},
	}

	// globalConfigDir() reads HOME and appends ".weave"
	t.Setenv("HOME", fakeHome)

	require.NoError(t, ext.Subscribe(bus))

	require.Len(t, bus.PublishCalls(), 1)
	call := bus.PublishCalls()[0]
	assert.Equal(t, TopicInstructionsLoaded, call.Event.Topic)

	payload, ok := call.Event.Payload.(string)
	require.True(t, ok, "payload should be a string")
	assert.Contains(t, payload, "project context")
	assert.Contains(t, payload, "append text")
}

func TestSubscribe_NoFilesPublishesEmpty(t *testing.T) {
	projectDir := t.TempDir()
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	configPath := filepath.Join(projectDir, ".weave", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	require.NoError(t, os.WriteFile(configPath, []byte(`{"agent_loop":"loop"}`), 0o644))

	ext, err := NewInstructionsExtension(sdk.FilePathConfig(configPath))
	require.NoError(t, err)

	bus := &BusMock{
		PublishFunc: func(event sdk.Event) {},
	}

	require.NoError(t, ext.Subscribe(bus))

	require.Len(t, bus.PublishCalls(), 1)
	call := bus.PublishCalls()[0]
	assert.Equal(t, TopicInstructionsLoaded, call.Event.Topic)
	assert.Empty(t, call.Event.Payload)
}
