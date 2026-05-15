package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// --- discoverCompactPrompt tests ---

func TestDiscoverCompactPrompt_NoFiles(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()

	result := discoverCompactPrompt(projectDir, globalDir)
	assert.Empty(t, result)
}

func TestDiscoverCompactPrompt_ProjectOverridesGlobal(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".weave"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, ".weave", "COMPACT.md"), []byte("project compact"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "COMPACT.md"), []byte("global compact"), 0o644))

	result := discoverCompactPrompt(projectDir, globalDir)
	assert.Equal(t, "project compact", result)
}

func TestDiscoverCompactPrompt_GlobalFallback(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(globalDir, "COMPACT.md"), []byte("global compact"), 0o644))

	result := discoverCompactPrompt(projectDir, globalDir)
	assert.Equal(t, "global compact", result)
}

func TestDiscoverCompactPrompt_EmptyGlobalDir(t *testing.T) {
	projectDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".weave"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, ".weave", "COMPACT.md"), []byte("project only"), 0o644))

	result := discoverCompactPrompt(projectDir, "")
	assert.Equal(t, "project only", result)
}

// --- resolveCompactPrompt tests ---

func TestResolveCompactPrompt_CustomInstructionsWin(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".weave"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, ".weave", "COMPACT.md"), []byte("from file"), 0o644))

	result := resolveCompactPrompt("custom instructions", projectDir, globalDir)
	assert.Equal(t, "custom instructions", result)
}

func TestResolveCompactPrompt_CompactMDWhenNoCustom(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".weave"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, ".weave", "COMPACT.md"), []byte("from file"), 0o644))

	result := resolveCompactPrompt("", projectDir, globalDir)
	assert.Equal(t, "from file", result)
}

func TestResolveCompactPrompt_DefaultWhenNothingFound(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()

	result := resolveCompactPrompt("", projectDir, globalDir)
	assert.Contains(t, result, "Summarize the following conversation")
	assert.Contains(t, result, "## Goal")
}
