package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"weave/sdk"
)

func TestNewSkillsExtension(t *testing.T) {
	ext, err := NewSkillsExtension(sdk.FilePathConfig(""))
	require.NoError(t, err)
	assert.Equal(t, "skills", ext.Name())
}

func TestSkillsExtension_Close(t *testing.T) {
	ext, err := NewSkillsExtension(sdk.FilePathConfig(""))
	require.NoError(t, err)
	assert.NoError(t, ext.Close())
}

func TestSkillsExtension_Subscribe_DiscoversSkills(t *testing.T) {
	root := t.TempDir()
	writeSkillMD(t, filepath.Join(root, "test-skill"), "test-skill", "A test skill", "# Do stuff")

	bus := &BusMock{
		PublishFunc: func(event sdk.Event) bool { return true },
	}

	ext, err := NewSkillsExtension(sdk.FilePathConfig(""))
	require.NoError(t, err)
	ext.discoveryPaths = []string{root}

	ext.Subscribe(bus)

	require.Len(t, bus.PublishCalls(), 1)
	call := bus.PublishCalls()[0]
	assert.Equal(t, TopicSkillsLoaded, call.Event.Topic)
	assert.Contains(t, call.Event.Payload.(string), "<name>test-skill</name>")
}

func TestSkillsExtension_Subscribe_RegistersCommands(t *testing.T) {
	root := t.TempDir()
	writeSkillMD(t, filepath.Join(root, "my-skill"), "my-skill", "Does things", "# Instructions")

	var registeredCmds []string
	ui := &UIMock{
		RegisterCommandFunc: func(name string, handler func(args string) error) {
			registeredCmds = append(registeredCmds, name)
		},
	}

	sdk.RegisterUI("tui", ui)
	t.Cleanup(sdk.ResetUIRegistry)

	bus := &BusMock{
		PublishFunc: func(event sdk.Event) bool { return true },
	}

	ext, err := NewSkillsExtension(sdk.FilePathConfig(""))
	require.NoError(t, err)
	ext.discoveryPaths = []string{root}

	ext.Subscribe(bus)

	assert.Contains(t, registeredCmds, "skill:my-skill")
}

func TestSkillsExtension_Subscribe_HeadlessMode(t *testing.T) {
	skillDir := filepath.Join(t.TempDir(), "headless-skill")
	writeSkillMD(t, skillDir, "headless-skill", "No UI", "# Instructions")

	bus := &BusMock{
		PublishFunc: func(event sdk.Event) bool { return true },
	}

	ext, err := NewSkillsExtension(sdk.FilePathConfig(""))
	require.NoError(t, err)

	ext.Subscribe(bus)

	require.Len(t, bus.PublishCalls(), 1)
	assert.Equal(t, TopicSkillsLoaded, bus.PublishCalls()[0].Event.Topic)
}

func TestSkillsExtension_Subscribe_ProjectLocalSkills(t *testing.T) {
	projectDir := t.TempDir()
	skillDir := filepath.Join(projectDir, ".weave", "skills", "proj-skill")
	writeSkillMD(t, skillDir, "proj-skill", "Project skill", "# Proj instructions")

	bus := &BusMock{
		PublishFunc: func(event sdk.Event) bool { return true },
	}

	configPath := filepath.Join(projectDir, ".weave.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("core:\n  agent_loop: loop\n"), 0o644))

	ext, err := NewSkillsExtension(sdk.FilePathConfig(configPath))
	require.NoError(t, err)

	t.Setenv("HOME", t.TempDir())

	ext.Subscribe(bus)

	require.Len(t, bus.PublishCalls(), 1)
	assert.Contains(t, bus.PublishCalls()[0].Event.Payload.(string), "<name>proj-skill</name>")
}

func TestMakeSkillHandler_Expansion(t *testing.T) {
	skill := Skill{
		Name:     "test-skill",
		FilePath: "/path/to/test-skill/SKILL.md",
		BaseDir:  "/path/to/test-skill",
	}
	skill.body = "# Instructions\nDo the thing."

	var published []sdk.Event
	bus := &BusMock{
		PublishFunc: func(event sdk.Event) bool {
			published = append(published, event)
			return true
		},
	}

	handler := makeSkillHandler(skill, bus)

	t.Run("with args", func(t *testing.T) {
		published = nil
		require.NoError(t, handler("do something extra"))

		require.Len(t, published, 1)
		payload := published[0].Payload.(string)
		assert.Equal(t, "agent.prompt", published[0].Topic)
		assert.Contains(t, payload, `<skill name="test-skill" location="/path/to/test-skill/SKILL.md">`)
		assert.Contains(t, payload, "References are relative to /path/to/test-skill.")
		assert.Contains(t, payload, "# Instructions\nDo the thing.")
		assert.Contains(t, payload, "</skill>")
		assert.Contains(t, payload, "do something extra")
	})

	t.Run("without args", func(t *testing.T) {
		published = nil
		require.NoError(t, handler(""))

		require.Len(t, published, 1)
		payload := published[0].Payload.(string)
		assert.Contains(t, payload, "</skill>")
		_, afterClosing, _ := strings.Cut(payload, "</skill>")
		assert.Empty(t, strings.TrimSpace(afterClosing))
	})
}

func TestMakeSkillHandler_FrontmatterStripped(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fm-skill")
	writeSkillMD(t, dir, "fm-skill", "Tests frontmatter stripping", "Real instructions here.")

	skill, err := loadSkillFromDir(dir)
	require.NoError(t, err)

	var published []sdk.Event
	bus := &BusMock{
		PublishFunc: func(event sdk.Event) bool {
			published = append(published, event)
			return true
		},
	}

	handler := makeSkillHandler(skill, bus)
	require.NoError(t, handler(""))

	payload := published[0].Payload.(string)
	assert.NotContains(t, payload, "---")
	assert.NotContains(t, payload, "name: fm-skill")
	assert.Contains(t, payload, "Real instructions here.")
}
