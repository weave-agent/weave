package agent

import (
	"strings"
	"testing"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptBuilder_Creation(t *testing.T) {
	pb := newPromptBuilder(sdk.FilePathConfig("/project/.weave/settings.json"))
	require.NotNil(t, pb)
	assert.NotNil(t, pb.cfg)
}

// --- buildContextSection tests ---

func TestBuildContextSection_Empty(t *testing.T) {
	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.buildContextSection(nil)
	assert.Empty(t, result)
}

func TestBuildContextSection_SingleFile(t *testing.T) {
	pb := newPromptBuilder(sdk.FilePathConfig(""))
	files := []contextFile{
		{Path: "/project/CLAUDE.md", Content: "Do stuff"},
	}
	result := pb.buildContextSection(files)
	assert.Contains(t, result, "# Project Context")
	assert.Contains(t, result, "## /project/CLAUDE.md")
	assert.Contains(t, result, "Do stuff")
}

func TestBuildContextSection_MultipleFiles(t *testing.T) {
	pb := newPromptBuilder(sdk.FilePathConfig(""))
	files := []contextFile{
		{Path: "/a.md", Content: "AAA"},
		{Path: "/b.md", Content: "BBB"},
	}
	result := pb.buildContextSection(files)
	assert.Contains(t, result, "# Project Context")
	assert.Contains(t, result, "## /a.md")
	assert.Contains(t, result, "AAA")
	assert.Contains(t, result, "## /b.md")
	assert.Contains(t, result, "BBB")
}

// --- formatInstructionsPrompt tests ---

func TestFormatInstructionsPrompt_EmptyInput(t *testing.T) {
	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.formatInstructionsPrompt(nil, "", "")
	assert.Empty(t, result)
}

func TestFormatInstructionsPrompt_ContextFilesOnly(t *testing.T) {
	pb := newPromptBuilder(sdk.FilePathConfig(""))
	files := []contextFile{
		{Path: "/project/CLAUDE.md", Content: "Do stuff"},
	}
	result := pb.formatInstructionsPrompt(files, "", "")
	assert.Contains(t, result, "# Project Context")
	assert.Contains(t, result, "## /project/CLAUDE.md")
	assert.Contains(t, result, "Do stuff")
}

func TestFormatInstructionsPrompt_SystemBaseOnly(t *testing.T) {
	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.formatInstructionsPrompt(nil, "You are a helpful assistant.", "")
	assert.Equal(t, "You are a helpful assistant.", result)
}

func TestFormatInstructionsPrompt_SystemAppendOnly(t *testing.T) {
	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.formatInstructionsPrompt(nil, "", "Always be concise.")
	assert.Equal(t, "Always be concise.", result)
}

func TestFormatInstructionsPrompt_AllCombined(t *testing.T) {
	pb := newPromptBuilder(sdk.FilePathConfig(""))
	files := []contextFile{
		{Path: "/project/CLAUDE.md", Content: "Project rules"},
	}
	result := pb.formatInstructionsPrompt(files, "Base prompt.", "Append this.")
	assert.Contains(t, result, "Base prompt.")
	assert.Contains(t, result, "# Project Context")
	assert.Contains(t, result, "## /project/CLAUDE.md")
	assert.Contains(t, result, "Project rules")
	assert.Contains(t, result, "Append this.")
}

func TestFormatInstructionsPrompt_Ordering(t *testing.T) {
	pb := newPromptBuilder(sdk.FilePathConfig(""))
	files := []contextFile{
		{Path: "/a.md", Content: "AAA"},
	}
	result := pb.formatInstructionsPrompt(files, "BASE", "APPEND")

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
