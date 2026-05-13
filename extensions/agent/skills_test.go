package agent

import (
	"os"
	"path/filepath"
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

		sdk.RegisterExtension("test-ext", func(cfg sdk.Config, _ struct{}) (sdk.Extension, error) {
			return stubExt{}, nil
		})

		projectDir := t.TempDir()
		extDir := filepath.Join(projectDir, ".weave", "extensions", "test-ext")
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

		sdk.RegisterExtension("global-ext", func(cfg sdk.Config, _ struct{}) (sdk.Extension, error) {
			return stubExt{}, nil
		})

		globalDir := t.TempDir()
		extDir := filepath.Join(globalDir, "extensions", "global-ext")
		skillsDir := filepath.Join(extDir, "skills")
		writeSkillMD(t, filepath.Join(skillsDir, "global-skill"), "global-skill", "Global skill", "body")

		paths := discoverExtensionSkills("", globalDir)
		require.Len(t, paths, 1)
		assert.Equal(t, skillsDir, paths[0])
	})

	t.Run("ignores extensions without skills dir", func(t *testing.T) {
		resetRegistries()
		defer resetRegistries()

		sdk.RegisterExtension("no-skills", func(cfg sdk.Config, _ struct{}) (sdk.Extension, error) {
			return stubExt{}, nil
		})

		projectDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(projectDir, ".weave", "extensions", "no-skills"), 0o755))

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

		sdk.RegisterExtension("proj-ext", func(cfg sdk.Config, _ struct{}) (sdk.Extension, error) {
			return stubExt{}, nil
		})
		sdk.RegisterExtension("glob-ext", func(cfg sdk.Config, _ struct{}) (sdk.Extension, error) {
			return stubExt{}, nil
		})

		projectDir := t.TempDir()
		globalDir := t.TempDir()

		projSkillsDir := filepath.Join(projectDir, ".weave", "extensions", "proj-ext", "skills")
		writeSkillMD(t, filepath.Join(projSkillsDir, "proj-skill"), "proj-skill", "Project", "body")

		globSkillsDir := filepath.Join(globalDir, "extensions", "glob-ext", "skills")
		writeSkillMD(t, filepath.Join(globSkillsDir, "glob-skill"), "glob-skill", "Global", "body")

		paths := discoverExtensionSkills(projectDir, globalDir)
		require.Len(t, paths, 2)
		assert.Contains(t, paths, projSkillsDir)
		assert.Contains(t, paths, globSkillsDir)
	})
}

// --- integration: full skill discovery with precedence ---

func TestDiscoverSkills_WithExtensionSkills(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	sdk.RegisterExtension("my-ext", func(cfg sdk.Config, _ struct{}) (sdk.Extension, error) {
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
	extSkillsDir := filepath.Join(projectDir, ".weave", "extensions", "my-ext", "skills")
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

// --- Skill.Body tests ---

func TestSkillBody(t *testing.T) {
	s := Skill{body: "test body"}
	assert.Equal(t, "test body", s.Body())
}
