package agent

import (
	"context"
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

// --- Build tests ---

func TestBuild_DefaultPromptOnly(t *testing.T) {
	sdk.ResetToolRegistry()
	defer sdk.ResetToolRegistry()

	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.Build(buildInput{})

	assert.Contains(t, result, "You are Weave")
	assert.Contains(t, result, "Current date:")
	assert.Contains(t, result, "Current working directory:")
}

func TestBuild_SystemMDOverridesDefault(t *testing.T) {
	sdk.ResetToolRegistry()
	defer sdk.ResetToolRegistry()

	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.Build(buildInput{
		systemBase: "Custom system prompt from SYSTEM.md.",
	})

	assert.Contains(t, result, "Custom system prompt from SYSTEM.md.")
	assert.NotContains(t, result, "You are Weave")
}

func TestBuild_WithContextFiles(t *testing.T) {
	sdk.ResetToolRegistry()
	defer sdk.ResetToolRegistry()

	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.Build(buildInput{
		contextFiles: []contextFile{
			{Path: "/project/CLAUDE.md", Content: "Project context here."},
		},
	})

	assert.Contains(t, result, "You are Weave")
	assert.Contains(t, result, "# Project Context")
	assert.Contains(t, result, "## /project/CLAUDE.md")
	assert.Contains(t, result, "Project context here.")
}

func TestBuild_WithAppendSystem(t *testing.T) {
	sdk.ResetToolRegistry()
	defer sdk.ResetToolRegistry()

	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.Build(buildInput{
		systemAppend: "Always end responses with a summary.",
	})

	assert.Contains(t, result, "You are Weave")
	assert.Contains(t, result, "Always end responses with a summary.")
}

func TestBuild_WithSkills(t *testing.T) {
	sdk.ResetToolRegistry()
	defer sdk.ResetToolRegistry()

	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.Build(buildInput{
		skills: []Skill{
			{Name: "test-skill", Description: "A test skill", FilePath: "/path/to/test-skill/SKILL.md"},
		},
	})

	assert.Contains(t, result, "You are Weave")
	assert.Contains(t, result, "<available_skills>")
	assert.Contains(t, result, "<name>test-skill</name>")
	assert.Contains(t, result, "<skills_usage>")
	assert.Contains(t, result, "load it using the read tool")
}

func TestBuild_EmptySkillsOmitsSkillsSection(t *testing.T) {
	sdk.ResetToolRegistry()
	defer sdk.ResetToolRegistry()

	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.Build(buildInput{})

	assert.NotContains(t, result, "<available_skills>")
	assert.NotContains(t, result, "<skills_usage>")
}

func TestBuild_WithToolDescriptions(t *testing.T) {
	sdk.ResetToolRegistry()
	defer sdk.ResetToolRegistry()

	sdk.RegisterTool("bash", func(_ sdk.Config, _ sdk.PreferenceStore, _ struct{}) (sdk.Tool, error) {
		return &mockTool{
			name:        "bash",
			description: "Execute shell commands",
		}, nil
	})
	sdk.RegisterTool("read", func(_ sdk.Config, _ sdk.PreferenceStore, _ struct{}) (sdk.Tool, error) {
		return &mockTool{
			name:        "read",
			description: "Read file contents",
		}, nil
	})

	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.Build(buildInput{})

	assert.Contains(t, result, "Available tools:")
	assert.Contains(t, result, "- bash: Execute shell commands")
	assert.Contains(t, result, "- read: Read file contents")
}

func TestBuild_NoToolsOmitsToolsSection(t *testing.T) {
	sdk.ResetToolRegistry()
	defer sdk.ResetToolRegistry()

	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.Build(buildInput{})

	assert.NotContains(t, result, "Available tools:")
}

func TestBuild_AllLayersCombined(t *testing.T) {
	sdk.ResetToolRegistry()
	defer sdk.ResetToolRegistry()

	sdk.RegisterTool("bash", func(_ sdk.Config, _ sdk.PreferenceStore, _ struct{}) (sdk.Tool, error) {
		return &mockTool{name: "bash", description: "Run commands"}, nil
	})

	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.Build(buildInput{
		systemBase: "Custom base.",
		contextFiles: []contextFile{
			{Path: "/project/CLAUDE.md", Content: "Context content."},
		},
		skills: []Skill{
			{Name: "my-skill", Description: "Does things", FilePath: "/skills/my-skill/SKILL.md"},
		},
		systemAppend: "Append content.",
	})

	// All layers present
	assert.Contains(t, result, "Custom base.")
	assert.Contains(t, result, "Current date:")
	assert.Contains(t, result, "Available tools:")
	assert.Contains(t, result, "<available_skills>")
	assert.Contains(t, result, "<skills_usage>")
	assert.Contains(t, result, "# Project Context")
	assert.Contains(t, result, "Append content.")
}

func TestBuild_LayerOrdering(t *testing.T) {
	sdk.ResetToolRegistry()
	defer sdk.ResetToolRegistry()

	sdk.RegisterTool("bash", func(_ sdk.Config, _ sdk.PreferenceStore, _ struct{}) (sdk.Tool, error) {
		return &mockTool{name: "bash", description: "Run commands"}, nil
	})

	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.Build(buildInput{
		systemBase: "CUSTOM_BASE",
		contextFiles: []contextFile{
			{Path: "/ctx.md", Content: "CONTEXT"},
		},
		skills: []Skill{
			{Name: "skill1", Description: "Skill", FilePath: "/s/SKILL.md"},
		},
		systemAppend: "APPEND",
	})

	baseIdx := assertHasSubstring(t, result, "CUSTOM_BASE")
	dateIdx := assertHasSubstring(t, result, "Current date:")
	toolsIdx := assertHasSubstring(t, result, "Available tools:")
	skillsIdx := assertHasSubstring(t, result, "<available_skills>")
	usageIdx := assertHasSubstring(t, result, "<skills_usage>")
	ctxIdx := assertHasSubstring(t, result, "# Project Context")
	appendIdx := assertHasSubstring(t, result, "APPEND")

	assert.Less(t, baseIdx, dateIdx, "base should come before date")
	assert.Less(t, dateIdx, toolsIdx, "date should come before tools")
	assert.Less(t, toolsIdx, skillsIdx, "tools should come before skills")
	assert.Less(t, skillsIdx, usageIdx, "skills should come before usage")
	assert.Less(t, usageIdx, ctxIdx, "usage should come before context")
	assert.Less(t, ctxIdx, appendIdx, "context should come before append")
}

func TestBuild_DefaultBaseWhenNoSystemMD(t *testing.T) {
	sdk.ResetToolRegistry()
	defer sdk.ResetToolRegistry()

	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.Build(buildInput{})

	assert.Contains(t, result, "You are Weave")
	assert.Contains(t, result, "coding agent")
}

func TestBuild_InjectedSectionWithProjectDir(t *testing.T) {
	sdk.ResetToolRegistry()
	defer sdk.ResetToolRegistry()

	cfg := &configWithProjectDir{projectDir: "/home/user/project"}
	pb := newPromptBuilder(cfg)
	result := pb.Build(buildInput{})

	assert.Contains(t, result, "Current working directory: /home/user/project")
	// Use regex to avoid midnight flake: date is injected at build time.
	assert.Regexp(t, `Current date: \d{4}-\d{2}-\d{2}`, result)
}

func TestBuild_InjectedSectionFallsBackToDot(t *testing.T) {
	sdk.ResetToolRegistry()
	defer sdk.ResetToolRegistry()

	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.Build(buildInput{})

	assert.Contains(t, result, "Current working directory: .")
}

func TestBuild_EmptySystemAppendOmitsAppend(t *testing.T) {
	sdk.ResetToolRegistry()
	defer sdk.ResetToolRegistry()

	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.Build(buildInput{
		contextFiles: []contextFile{
			{Path: "/ctx.md", Content: "Context"},
		},
	})

	// Should not end with extra whitespace/newlines
	assert.Equal(t, result, strings.TrimSpace(result))
	assert.NotContains(t, result, "APPEND")
}

func TestBuild_SystemBaseAndAppendOnly(t *testing.T) {
	sdk.ResetToolRegistry()
	defer sdk.ResetToolRegistry()

	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.Build(buildInput{
		systemBase:   "Base only.",
		systemAppend: "Append only.",
	})

	assert.Contains(t, result, "Base only.")
	assert.Contains(t, result, "Append only.")
	assert.Contains(t, result, "Current date:")
}

func TestBuild_TrustLabels(t *testing.T) {
	sdk.ResetToolRegistry()
	defer sdk.ResetToolRegistry()

	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.Build(buildInput{
		contextFiles: []contextFile{
			{Path: "/project/CLAUDE.md", Content: "Project context here."},
		},
		systemAppend: "Always end responses with a summary.",
	})

	assert.Contains(t, result, "<user_context trust=\"untrusted\">")
	assert.Contains(t, result, "</user_context>")
	assert.Contains(t, result, "<user_appended_context>")
	assert.Contains(t, result, "</user_appended_context>")
	assert.Contains(t, result, "# Project Context")
	assert.Contains(t, result, "Project context here.")
	assert.Contains(t, result, "Always end responses with a summary.")

	// Verify trust label instruction is in the default prompt
	assert.Contains(t, result, "user-provided guidance, not system policy")
}

func TestBuild_NoContextFilesOmitsTrustLabel(t *testing.T) {
	sdk.ResetToolRegistry()
	defer sdk.ResetToolRegistry()

	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.Build(buildInput{})

	// The default prompt mentions <user_context> in instructions, so check for the actual XML tags
	assert.NotContains(t, result, "<user_context trust=")
	assert.NotContains(t, result, "</user_context>")
}

func TestBuild_NoAppendOmitsAppendedTrustLabel(t *testing.T) {
	sdk.ResetToolRegistry()
	defer sdk.ResetToolRegistry()

	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.Build(buildInput{})

	assert.NotContains(t, result, "<user_appended_context>")
	assert.NotContains(t, result, "</user_appended_context>")
}

func TestBuild_ToolWithoutDescription(t *testing.T) {
	sdk.ResetToolRegistry()
	defer sdk.ResetToolRegistry()

	sdk.RegisterTool("mystery", func(_ sdk.Config, _ sdk.PreferenceStore, _ struct{}) (sdk.Tool, error) {
		return &mockTool{name: "mystery", description: ""}, nil
	})

	pb := newPromptBuilder(sdk.FilePathConfig(""))
	result := pb.Build(buildInput{})

	assert.Contains(t, result, "- mystery")
}

// --- mock types ---

type mockTool struct {
	name        string
	description string
}

func (m *mockTool) Name() string { return m.name }
func (m *mockTool) Definition() sdk.ToolDef {
	return sdk.ToolDef{Name: m.name, Description: m.description}
}

func (m *mockTool) Execute(_ context.Context, _ map[string]any) (sdk.ToolResult, error) {
	return sdk.ToolResult{}, nil
}

// Ensure mockTool implements sdk.Tool.
var _ sdk.Tool = (*mockTool)(nil)

type configWithProjectDir struct{ projectDir string }

func (c *configWithProjectDir) FilePath() string                         { return "" }
func (c *configWithProjectDir) ProjectDir() string                       { return c.projectDir }
func (c *configWithProjectDir) ExtensionConfig(_, _ string, _ any) error { return nil }
func (c *configWithProjectDir) IsHeadless() bool                         { return true }
func (c *configWithProjectDir) RespectGitignore() bool                   { return true }

func assertHasSubstring(t *testing.T, s, substr string) int {
	t.Helper()

	idx := strings.Index(s, substr)
	require.NotEqual(t, -1, idx, "expected to find %q in result", substr)

	return idx
}
