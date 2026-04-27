package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSkillMD(t *testing.T, dir, name, desc, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))

	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n" + body
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))
}

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
		{"contains anthropic", "anthropic-tool", true},
		{"contains claude", "claude-helper", true},
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
}

func TestDiscoverSkills(t *testing.T) {
	t.Run("multiple paths with dedup", func(t *testing.T) {
		globalDir := t.TempDir()
		projectDir := t.TempDir()

		writeSkillMD(t, filepath.Join(globalDir, "alpha"), "alpha", "Global alpha", "instructions")
		writeSkillMD(t, filepath.Join(globalDir, "beta"), "beta", "Global beta", "instructions")
		writeSkillMD(t, filepath.Join(projectDir, "beta"), "beta", "Project beta", "project instructions")
		writeSkillMD(t, filepath.Join(projectDir, "gamma"), "gamma", "Project gamma", "instructions")

		skills, err := discoverSkills(globalDir, projectDir)
		require.NoError(t, err)
		require.Len(t, skills, 3)

		assert.Equal(t, "alpha", skills[0].Name)
		assert.Equal(t, "beta", skills[1].Name)
		assert.Equal(t, "gamma", skills[2].Name)

		assert.Equal(t, "Global beta", skills[1].Description, "first path should win on dedup")
	})

	t.Run("empty directories", func(t *testing.T) {
		emptyDir := t.TempDir()
		skills, err := discoverSkills(emptyDir)
		require.NoError(t, err)
		assert.Empty(t, skills)
	})

	t.Run("nonexistent paths are skipped", func(t *testing.T) {
		skills, err := discoverSkills("/nonexistent/path/skills")
		require.NoError(t, err)
		assert.Empty(t, skills)
	})

	t.Run("invalid skills are skipped", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "bad"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "bad", "SKILL.md"), []byte("no frontmatter"), 0o644))

		skills, err := discoverSkills(dir)
		require.NoError(t, err)
		assert.Empty(t, skills)
	})

	t.Run("sorted output", func(t *testing.T) {
		dir := t.TempDir()
		writeSkillMD(t, filepath.Join(dir, "zeta"), "zeta", "Z skill", "z")
		writeSkillMD(t, filepath.Join(dir, "alpha"), "alpha", "A skill", "a")
		writeSkillMD(t, filepath.Join(dir, "middle"), "middle", "M skill", "m")

		skills, err := discoverSkills(dir)
		require.NoError(t, err)
		require.Len(t, skills, 3)
		assert.Equal(t, "alpha", skills[0].Name)
		assert.Equal(t, "middle", skills[1].Name)
		assert.Equal(t, "zeta", skills[2].Name)
	})
}

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

	t.Run("special chars in description", func(t *testing.T) {
		skills := []Skill{
			{Name: "xml-skill", Description: "Uses <tags> & \"quotes\"", FilePath: "/path"},
		}
		result := formatSkillsPrompt(skills)
		assert.Contains(t, result, "Uses <tags> & \"quotes\"")
	})
}
