package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSkillMD(t *testing.T, dir, name, desc, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))

	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n" + body
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))
}

func makeValidExtModule(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+filepath.Base(dir)+"\n\ngo 1.22\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644))
}

// --- validateName tests ---

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "my-skill", false},
		{"valid alphanumeric", "skill123", false},
		{"valid multi-hyphen", "my-cool-skill", false},
		{"valid single char", "a", false},
		{"uppercase rejected", "My-Skill", true},
		{"leading hyphen", "-skill", true},
		{"trailing hyphen", "skill-", true},
		{"consecutive hyphens", "my--skill", true},
		{"exact anthropic", "anthropic", true},
		{"exact claude", "claude", true},
		{"contains anthropic but not exact", "anthropic-tool", false},
		{"contains claude but not exact", "claude-helper", false},
		{"empty", "", true},
		{"spaces", "my skill", true},
		{"underscores", "my_skill", true},
		{"special chars", "skill!", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateName(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// --- loadSkillFromDir tests ---

func TestLoadSkillFromDir(t *testing.T) {
	t.Run("valid skill", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "my-skill")
		writeSkillMD(t, dir, "my-skill", "A test skill", "# Instructions\nDo stuff.")

		skill, err := loadSkillFromDir(dir)
		require.NoError(t, err)
		assert.Equal(t, "my-skill", skill.Name)
		assert.Equal(t, "A test skill", skill.Description)
		assert.Equal(t, filepath.Join(dir, "SKILL.md"), skill.FilePath)
		assert.Equal(t, dir, skill.BaseDir)
		assert.Equal(t, "# Instructions\nDo stuff.", skill.Body())
	})

	t.Run("missing SKILL.md", func(t *testing.T) {
		dir := t.TempDir()
		_, err := loadSkillFromDir(dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reading SKILL.md")
	})

	t.Run("invalid frontmatter", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "bad-skill")
		require.NoError(t, os.MkdirAll(dir, 0o755))

		content := "not frontmatter at all"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))

		_, err := loadSkillFromDir(dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "frontmatter")
	})

	t.Run("name mismatch with directory", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "right-name")
		writeSkillMD(t, dir, "wrong-name", "A skill", "body")

		_, err := loadSkillFromDir(dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not match directory")
	})

	t.Run("missing description", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "nodesc")
		require.NoError(t, os.MkdirAll(dir, 0o755))

		content := "---\nname: nodesc\n---\nbody"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))

		_, err := loadSkillFromDir(dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "description is required")
	})

	t.Run("missing name", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "noname")
		require.NoError(t, os.MkdirAll(dir, 0o755))

		content := "---\ndescription: has desc\n---\nbody"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))

		_, err := loadSkillFromDir(dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("invalid name in frontmatter", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "BAD")
		require.NoError(t, os.MkdirAll(dir, 0o755))

		content := "---\nname: BAD\ndescription: desc\n---\nbody"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))

		_, err := loadSkillFromDir(dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid skill name")
	})

	t.Run("full frontmatter fields", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "full-skill")
		require.NoError(t, os.MkdirAll(dir, 0o755))

		content := `---
name: full-skill
description: Full skill test
license: Apache-2.0
compatibility: Requires python3
metadata:
  author: test
  version: "1.0"
allowed-tools: bash read write
---
Full instructions here.`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))

		skill, err := loadSkillFromDir(dir)
		require.NoError(t, err)
		assert.Equal(t, "Apache-2.0", skill.License)
		assert.Equal(t, "Requires python3", skill.Compatibility)
		assert.Equal(t, []string{"bash", "read", "write"}, skill.AllowedTools)
		assert.Equal(t, map[string]any{"author": "test", "version": "1.0"}, skill.Metadata)
		assert.Equal(t, "Full instructions here.", skill.Body())
	})

	t.Run("unclosed frontmatter", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "unclosed")
		require.NoError(t, os.MkdirAll(dir, 0o755))

		content := "---\nname: unclosed\ndescription: test\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))

		_, err := loadSkillFromDir(dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not closed")
	})

	t.Run("windows line endings", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "crlf-skill")
		require.NoError(t, os.MkdirAll(dir, 0o755))

		content := "---\r\nname: crlf-skill\r\ndescription: CRLF test\r\n---\r\n\r\nBody here."
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))

		skill, err := loadSkillFromDir(dir)
		require.NoError(t, err)
		assert.Equal(t, "crlf-skill", skill.Name)
		assert.Equal(t, "CRLF test", skill.Description)
	})
}

// --- discoverSkills tests ---

func TestDiscoverSkills(t *testing.T) {
	t.Run("multiple paths with dedup", func(t *testing.T) {
		globalDir := t.TempDir()
		projectDir := t.TempDir()

		writeSkillMD(t, filepath.Join(globalDir, "alpha"), "alpha", "Global alpha", "instructions")
		writeSkillMD(t, filepath.Join(globalDir, "beta"), "beta", "Global beta", "instructions")
		writeSkillMD(t, filepath.Join(projectDir, "beta"), "beta", "Project beta", "project instructions")
		writeSkillMD(t, filepath.Join(projectDir, "gamma"), "gamma", "Project gamma", "instructions")

		skills := discoverSkills(globalDir, projectDir)
		require.Len(t, skills, 3)

		assert.Equal(t, "alpha", skills[0].Name)
		assert.Equal(t, "beta", skills[1].Name)
		assert.Equal(t, "gamma", skills[2].Name)

		assert.Equal(t, "Global beta", skills[1].Description, "first path should win on dedup")
	})

	t.Run("empty directories", func(t *testing.T) {
		emptyDir := t.TempDir()
		skills := discoverSkills(emptyDir)
		assert.Empty(t, skills)
	})

	t.Run("nonexistent paths are skipped", func(t *testing.T) {
		skills := discoverSkills("/nonexistent/path/skills")
		assert.Empty(t, skills)
	})

	t.Run("invalid skills are skipped", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "bad"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "bad", "SKILL.md"), []byte("no frontmatter"), 0o644))

		skills := discoverSkills(dir)
		assert.Empty(t, skills)
	})

	t.Run("sorted output", func(t *testing.T) {
		dir := t.TempDir()
		writeSkillMD(t, filepath.Join(dir, "zeta"), "zeta", "Z skill", "z")
		writeSkillMD(t, filepath.Join(dir, "alpha"), "alpha", "A skill", "a")
		writeSkillMD(t, filepath.Join(dir, "middle"), "middle", "M skill", "m")

		skills := discoverSkills(dir)
		require.Len(t, skills, 3)
		assert.Equal(t, "alpha", skills[0].Name)
		assert.Equal(t, "middle", skills[1].Name)
		assert.Equal(t, "zeta", skills[2].Name)
	})

	t.Run("symlinks are ignored", func(t *testing.T) {
		dir := t.TempDir()
		targetDir := filepath.Join(t.TempDir(), "real-skill")
		writeSkillMD(t, targetDir, "real-skill", "Real", "body")

		linkPath := filepath.Join(dir, "linked-skill")
		require.NoError(t, os.Symlink(targetDir, linkPath))

		skills := discoverSkills(dir)
		assert.Empty(t, skills)
	})
}

// --- formatSkillsPrompt tests ---

func TestFormatSkillsPrompt(t *testing.T) {
	t.Run("XML structure", func(t *testing.T) {
		skills := []Skill{
			{Name: "my-skill", Description: "Does things", FilePath: "/path/to/my-skill/SKILL.md"},
		}
		result := formatSkillsPrompt(skills)
		assert.Contains(t, result, "<available_skills>")
		assert.Contains(t, result, "</available_skills>")
		assert.Contains(t, result, "<name>my-skill</name>")
		assert.Contains(t, result, "<description>Does things</description>")
		assert.Contains(t, result, "<location>/path/to/my-skill/SKILL.md</location>")
	})

	t.Run("empty skills list", func(t *testing.T) {
		result := formatSkillsPrompt(nil)
		assert.Empty(t, result)
	})

	t.Run("multiple skills", func(t *testing.T) {
		skills := []Skill{
			{Name: "alpha", Description: "First skill", FilePath: "/a"},
			{Name: "beta", Description: "Second skill", FilePath: "/b"},
		}
		result := formatSkillsPrompt(skills)
		assert.Contains(t, result, "<name>alpha</name>")
		assert.Contains(t, result, "<name>beta</name>")
	})

	t.Run("special chars escaped", func(t *testing.T) {
		skills := []Skill{
			{Name: "xml-skill", Description: "Uses <tags> & \"quotes\"", FilePath: "/path"},
		}
		result := formatSkillsPrompt(skills)
		assert.Contains(t, result, "Uses &lt;tags&gt; &amp; &quot;quotes&quot;")
	})

	t.Run("disabled skills are excluded", func(t *testing.T) {
		skills := []Skill{
			{Name: "enabled-skill", Description: "Enabled", FilePath: "/a"},
			{Name: "disabled-skill", Description: "Disabled", FilePath: "/b", DisableModelInvocation: true},
		}
		result := formatSkillsPrompt(skills)
		assert.Contains(t, result, "<name>enabled-skill</name>")
		assert.NotContains(t, result, "<name>disabled-skill</name>")
	})

	t.Run("all disabled returns empty", func(t *testing.T) {
		skills := []Skill{
			{Name: "disabled", Description: "Disabled", FilePath: "/a", DisableModelInvocation: true},
		}
		result := formatSkillsPrompt(skills)
		assert.Empty(t, result)
	})
}

// --- discoverExtensionSkills tests ---

type stubExt struct{}

func (s stubExt) Name() string              { return "stub" }
func (s stubExt) Subscribe(_ sdk.Bus) error { return nil }
func (s stubExt) Close() error              { return nil }

func TestDiscoverExtensionSkills(t *testing.T) {
	t.Run("discovers registered extension skills", func(t *testing.T) {
		resetRegistries()
		defer resetRegistries()

		sdk.RegisterExtension("test-ext", func(cfg sdk.Config, _ sdk.PreferenceStore, _ struct{}) (sdk.Extension, error) {
			return stubExt{}, nil
		})

		projectDir := t.TempDir()
		extDir := filepath.Join(projectDir, ".weave", "extensions", "test-ext")
		makeValidExtModule(t, extDir)
		skillsDir := filepath.Join(extDir, "skills")
		writeSkillMD(t, filepath.Join(skillsDir, "ext-skill"), "ext-skill", "Extension skill", "body")

		paths := discoverExtensionSkills(projectDir, "")
		require.Len(t, paths, 1)
		assert.Equal(t, skillsDir, paths[0])
	})

	t.Run("ignores unregistered extensions", func(t *testing.T) {
		resetRegistries()
		defer resetRegistries()

		projectDir := t.TempDir()
		extDir := filepath.Join(projectDir, ".weave", "extensions", "unknown-ext")
		skillsDir := filepath.Join(extDir, "skills")
		writeSkillMD(t, filepath.Join(skillsDir, "unknown-skill"), "unknown-skill", "Unknown", "body")

		paths := discoverExtensionSkills(projectDir, "")
		assert.Empty(t, paths)
	})

	t.Run("discovers from global dir", func(t *testing.T) {
		resetRegistries()
		defer resetRegistries()

		sdk.RegisterExtension("global-ext", func(cfg sdk.Config, _ sdk.PreferenceStore, _ struct{}) (sdk.Extension, error) {
			return stubExt{}, nil
		})

		globalDir := t.TempDir()
		extDir := filepath.Join(globalDir, "extensions", "global-ext")
		makeValidExtModule(t, extDir)
		skillsDir := filepath.Join(extDir, "skills")
		writeSkillMD(t, filepath.Join(skillsDir, "global-skill"), "global-skill", "Global skill", "body")

		paths := discoverExtensionSkills("", globalDir)
		require.Len(t, paths, 1)
		assert.Equal(t, skillsDir, paths[0])
	})

	t.Run("ignores extensions without skills dir", func(t *testing.T) {
		resetRegistries()
		defer resetRegistries()

		sdk.RegisterExtension("no-skills", func(cfg sdk.Config, _ sdk.PreferenceStore, _ struct{}) (sdk.Extension, error) {
			return stubExt{}, nil
		})

		projectDir := t.TempDir()
		extDir := filepath.Join(projectDir, ".weave", "extensions", "no-skills")
		makeValidExtModule(t, extDir)

		paths := discoverExtensionSkills(projectDir, "")
		assert.Empty(t, paths)
	})

	t.Run("nonexistent dirs return empty", func(t *testing.T) {
		resetRegistries()
		defer resetRegistries()

		paths := discoverExtensionSkills("/nonexistent", "/also-nonexistent")
		assert.Empty(t, paths)
	})

	t.Run("discovers from both project and global", func(t *testing.T) {
		resetRegistries()
		defer resetRegistries()

		sdk.RegisterExtension("proj-ext", func(cfg sdk.Config, _ sdk.PreferenceStore, _ struct{}) (sdk.Extension, error) {
			return stubExt{}, nil
		})
		sdk.RegisterExtension("glob-ext", func(cfg sdk.Config, _ sdk.PreferenceStore, _ struct{}) (sdk.Extension, error) {
			return stubExt{}, nil
		})

		projectDir := t.TempDir()
		globalDir := t.TempDir()

		projExtDir := filepath.Join(projectDir, ".weave", "extensions", "proj-ext")
		makeValidExtModule(t, projExtDir)
		projSkillsDir := filepath.Join(projExtDir, "skills")
		writeSkillMD(t, filepath.Join(projSkillsDir, "proj-skill"), "proj-skill", "Project", "body")

		globExtDir := filepath.Join(globalDir, "extensions", "glob-ext")
		makeValidExtModule(t, globExtDir)
		globSkillsDir := filepath.Join(globExtDir, "skills")
		writeSkillMD(t, filepath.Join(globSkillsDir, "glob-skill"), "glob-skill", "Global", "body")

		paths := discoverExtensionSkills(projectDir, globalDir)
		require.Len(t, paths, 2)
		assert.Contains(t, paths, projSkillsDir)
		assert.Contains(t, paths, globSkillsDir)
	})

	t.Run("discovers nested extensions", func(t *testing.T) {
		resetRegistries()
		defer resetRegistries()

		sdk.RegisterExtension("nested-ext", func(cfg sdk.Config, _ sdk.PreferenceStore, _ struct{}) (sdk.Extension, error) {
			return stubExt{}, nil
		})

		projectDir := t.TempDir()
		nestedExtDir := filepath.Join(projectDir, ".weave", "extensions", "vendor", "nested-ext")
		makeValidExtModule(t, nestedExtDir)
		skillsDir := filepath.Join(nestedExtDir, "skills")
		writeSkillMD(t, filepath.Join(skillsDir, "nested-skill"), "nested-skill", "Nested", "body")

		paths := discoverExtensionSkills(projectDir, "")
		require.Len(t, paths, 1)
		assert.Equal(t, skillsDir, paths[0])
	})

	t.Run("project shadows global extension skills", func(t *testing.T) {
		resetRegistries()
		defer resetRegistries()

		sdk.RegisterExtension("shadow-ext", func(cfg sdk.Config, _ sdk.PreferenceStore, _ struct{}) (sdk.Extension, error) {
			return stubExt{}, nil
		})

		projectDir := t.TempDir()
		globalDir := t.TempDir()

		// Global has skills
		globSkillsDir := filepath.Join(globalDir, "extensions", "shadow-ext", "skills")
		writeSkillMD(t, filepath.Join(globSkillsDir, "global-skill"), "global-skill", "Global", "body")

		// Project has the same extension as a valid module but no skills dir
		projExtDir := filepath.Join(projectDir, ".weave", "extensions", "shadow-ext")
		require.NoError(t, os.MkdirAll(projExtDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(projExtDir, "go.mod"), []byte("module shadow-ext\n\ngo 1.22\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(projExtDir, "main.go"), []byte("package main\n"), 0o644))

		paths := discoverExtensionSkills(projectDir, globalDir)
		// Project shadows global, so global skills should NOT be included
		assert.Empty(t, paths)
	})

	t.Run("non-module dirs do not shadow", func(t *testing.T) {
		resetRegistries()
		defer resetRegistries()

		sdk.RegisterExtension("no-shadow-ext", func(cfg sdk.Config, _ sdk.PreferenceStore, _ struct{}) (sdk.Extension, error) {
			return stubExt{}, nil
		})

		projectDir := t.TempDir()
		globalDir := t.TempDir()

		// Global has skills
		globExtDir := filepath.Join(globalDir, "extensions", "no-shadow-ext")
		makeValidExtModule(t, globExtDir)
		globSkillsDir := filepath.Join(globExtDir, "skills")
		writeSkillMD(t, filepath.Join(globSkillsDir, "global-skill"), "global-skill", "Global", "body")

		// Project has an empty dir (not a valid module) — should NOT shadow global
		require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".weave", "extensions", "no-shadow-ext"), 0o755))

		paths := discoverExtensionSkills(projectDir, globalDir)
		// Empty non-module dir is ignored, so global skills ARE discovered
		require.Len(t, paths, 1)
		assert.Equal(t, globSkillsDir, paths[0])
	})
}

// --- integration: full skill discovery with precedence ---

func TestDiscoverSkills_WithExtensionSkills(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	sdk.RegisterExtension("my-ext", func(cfg sdk.Config, _ sdk.PreferenceStore, _ struct{}) (sdk.Extension, error) {
		return stubExt{}, nil
	})

	projectDir := t.TempDir()
	globalDir := t.TempDir()

	// Project skills
	projSkillsDir := filepath.Join(projectDir, ".weave", "skills")
	writeSkillMD(t, filepath.Join(projSkillsDir, "proj-skill"), "proj-skill", "Project skill", "proj body")
	writeSkillMD(t, filepath.Join(projSkillsDir, "shared-skill"), "shared-skill", "Project shared", "proj shared")

	// Global skills
	globSkillsDir := filepath.Join(globalDir, "skills")
	writeSkillMD(t, filepath.Join(globSkillsDir, "glob-skill"), "glob-skill", "Global skill", "glob body")
	writeSkillMD(t, filepath.Join(globSkillsDir, "shared-skill"), "shared-skill", "Global shared", "glob shared")

	// Extension skills
	extDir := filepath.Join(projectDir, ".weave", "extensions", "my-ext")
	makeValidExtModule(t, extDir)
	extSkillsDir := filepath.Join(extDir, "skills")
	writeSkillMD(t, filepath.Join(extSkillsDir, "ext-skill"), "ext-skill", "Extension skill", "ext body")
	writeSkillMD(t, filepath.Join(extSkillsDir, "shared-skill"), "shared-skill", "Extension shared", "ext shared")

	// Build paths with precedence: project > global > extension
	extPaths := discoverExtensionSkills(projectDir, globalDir)
	paths := make([]string, 0, 2+len(extPaths))
	paths = append(paths, projSkillsDir, globSkillsDir)
	paths = append(paths, extPaths...)

	skills := discoverSkills(paths...)
	require.Len(t, skills, 4)

	// Should be sorted
	assert.Equal(t, "ext-skill", skills[0].Name)
	assert.Equal(t, "glob-skill", skills[1].Name)
	assert.Equal(t, "proj-skill", skills[2].Name)
	assert.Equal(t, "shared-skill", skills[3].Name)

	// Project should win on shared-skill
	assert.Equal(t, "Project shared", skills[3].Description)
}

// --- findModuleRoot tests ---

func TestFindModuleRoot(t *testing.T) {
	t.Run("env var takes precedence", func(t *testing.T) {
		t.Setenv("WEAVE_MODULE_ROOT", "/some/path")
		assert.Equal(t, "/some/path", findModuleRoot())
	})

	t.Run("walks up from cwd", func(t *testing.T) {
		t.Setenv("WEAVE_MODULE_ROOT", "")

		// When running inside the weave repo, we should find it.
		root := findModuleRoot()
		if root != "" {
			info, err := os.Stat(filepath.Join(root, "go.mod"))
			require.NoError(t, err)
			assert.False(t, info.IsDir())
		}
	})
}

// --- isValidExtensionModule tests ---

func TestIsValidExtensionModule(t *testing.T) {
	t.Run("valid module", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644))
		assert.True(t, isValidExtensionModule(dir))
	})

	t.Run("missing go.mod", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644))
		assert.False(t, isValidExtensionModule(dir))
	})

	t.Run("no go files", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644))
		assert.False(t, isValidExtensionModule(dir))
	})

	t.Run("only test files", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main\n"), 0o644))
		assert.False(t, isValidExtensionModule(dir))
	})

	t.Run("nested go files count", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "helper.go"), []byte("package sub\n"), 0o644))
		assert.True(t, isValidExtensionModule(dir))
	})

	t.Run("stops at submodule boundary", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644))

		sub := filepath.Join(dir, "sub")
		require.NoError(t, os.MkdirAll(sub, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(sub, "go.mod"), []byte("module sub\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(sub, "main.go"), []byte("package sub\n"), 0o644))

		// The sub-module's go files should not count toward parent
		assert.False(t, isValidExtensionModule(dir))
		// But the submodule itself is valid
		assert.True(t, isValidExtensionModule(sub))
	})
}

// --- Skill.Body tests ---

func TestSkillBody(t *testing.T) {
	s := Skill{body: "test body"}
	assert.Equal(t, "test body", s.Body())
}

// --- AllowedTools enforcement tests ---

func TestSkill_AllowedToolsEnforced(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	ext, err := NewAgentExtension(sdk.FilePathConfig(""), sdk.NoopPreferenceStore{}, CompactionConfig{})
	require.NoError(t, err)

	skill := Skill{
		Name:         "restricted-skill",
		FilePath:     "/path/to/restricted-skill/SKILL.md",
		BaseDir:      "/path/to/restricted-skill",
		AllowedTools: []string{"bash", "read"},
	}
	skill.body = "# Instructions"

	b := &BusMock{}

	handler := ext.makeSkillHandler(skill, b)

	// No filter initially
	assert.Nil(t, sdk.GetToolFilter())

	require.NoError(t, handler(""))

	// Filter should be set to skill's allowed tools
	filter := sdk.GetToolFilter()
	assert.Equal(t, []string{"bash", "read"}, filter)

	// Simulate turn end: restore the filter
	ext.mu.Lock()
	sdk.SetToolFilter(ext.savedToolFilter)
	ext.skillFilterActive = false
	ext.savedToolFilter = nil
	ext.mu.Unlock()

	// Filter should be restored (nil)
	assert.Nil(t, sdk.GetToolFilter())
}

func TestSkill_AllowedToolsEmpty_NoFilter(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	// Set an existing filter
	sdk.SetToolFilter([]string{"bash"})
	defer sdk.SetToolFilter(nil)

	ext, err := NewAgentExtension(sdk.FilePathConfig(""), sdk.NoopPreferenceStore{}, CompactionConfig{})
	require.NoError(t, err)

	skill := Skill{
		Name:     "open-skill",
		FilePath: "/path/to/open-skill/SKILL.md",
		BaseDir:  "/path/to/open-skill",
	}
	skill.body = "# Instructions"

	b := &BusMock{}

	handler := ext.makeSkillHandler(skill, b)

	require.NoError(t, handler(""))

	// Existing filter should not be changed
	filter := sdk.GetToolFilter()
	assert.Equal(t, []string{"bash"}, filter)
}

func TestSkill_AllowedToolsRestoresPreviousFilter(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	// Set an existing filter
	sdk.SetToolFilter([]string{"bash", "write"})
	defer sdk.SetToolFilter(nil)

	ext, err := NewAgentExtension(sdk.FilePathConfig(""), sdk.NoopPreferenceStore{}, CompactionConfig{})
	require.NoError(t, err)

	skill := Skill{
		Name:         "restricted-skill",
		FilePath:     "/path/to/restricted-skill/SKILL.md",
		BaseDir:      "/path/to/restricted-skill",
		AllowedTools: []string{"read"},
	}
	skill.body = "# Instructions"

	b := &BusMock{}

	handler := ext.makeSkillHandler(skill, b)

	require.NoError(t, handler(""))

	// Filter should be the skill's filter
	assert.Equal(t, []string{"read"}, sdk.GetToolFilter())

	// Simulate turn end: restore
	ext.mu.Lock()
	sdk.SetToolFilter(ext.savedToolFilter)
	ext.skillFilterActive = false
	ext.savedToolFilter = nil
	ext.mu.Unlock()

	// Filter should be the original filter
	assert.Equal(t, []string{"bash", "write"}, sdk.GetToolFilter())
}

func TestSkill_AllowedToolsFilterClearedWhenNoPreviousFilter(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	ext, err := NewAgentExtension(sdk.FilePathConfig(""), sdk.NoopPreferenceStore{}, CompactionConfig{})
	require.NoError(t, err)

	skill := Skill{
		Name:         "restricted-skill",
		FilePath:     "/path/to/restricted-skill/SKILL.md",
		BaseDir:      "/path/to/restricted-skill",
		AllowedTools: []string{"read", "write"},
	}
	skill.body = "# Instructions"

	b := &BusMock{}

	handler := ext.makeSkillHandler(skill, b)

	require.NoError(t, handler(""))

	// Filter should be the skill's filter
	assert.Equal(t, []string{"read", "write"}, sdk.GetToolFilter())

	// Simulate turn end: restore (savedToolFilter is nil, so filter is cleared)
	ext.mu.Lock()
	sdk.SetToolFilter(ext.savedToolFilter)
	ext.skillFilterActive = false
	ext.savedToolFilter = nil
	ext.mu.Unlock()

	// Filter should be cleared (nil)
	assert.Nil(t, sdk.GetToolFilter())
}

func TestSkill_BodyTrustLabel(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	ext, err := NewAgentExtension(sdk.FilePathConfig(""), sdk.NoopPreferenceStore{}, CompactionConfig{})
	require.NoError(t, err)

	skill := Skill{
		Name:     "test-skill",
		FilePath: "/path/to/test-skill/SKILL.md",
		BaseDir:  "/path/to/test-skill",
	}
	skill.body = "# Instructions\nDo the thing."

	var published []sdk.Event

	b := &BusMock{
		PublishFunc: func(event sdk.Event) {
			published = append(published, event)
		},
	}

	handler := ext.makeSkillHandler(skill, b)
	require.NoError(t, handler(""))

	require.Len(t, published, 1)
	assert.Equal(t, TopicPrompt, published[0].Topic)

	payload := published[0].Payload.(string)
	assert.Contains(t, payload, `<skill_body trust="untrusted">`)
	assert.Contains(t, payload, "# Instructions\nDo the thing.")
	assert.Contains(t, payload, "</skill_body>")
	assert.Contains(t, payload, "</skill>")

	// Verify ordering: skill_body opens before body and closes before skill closes
	bodyOpen := strings.Index(payload, `<skill_body trust="untrusted">`)
	bodyClose := strings.Index(payload, "</skill_body>")
	skillClose := strings.Index(payload, "</skill>")
	assert.Less(t, bodyOpen, bodyClose, "skill_body open should come before close")
	assert.Less(t, bodyClose, skillClose, "skill_body close should come before skill close")
}
