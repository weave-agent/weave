package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"weave/bus"
	"weave/sdk"
	"weave/sdk/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetRegistries() {
	sdk.ResetRegistry()
	sdk.ResetProviderRegistry()
	sdk.ResetToolRegistry()
	sdk.ResetUIRegistry()
	model.ResetAuthRegistry()
	model.ResetModelRegistry()
}

func TestNewAgentExtension(t *testing.T) {
	ext, err := NewAgentExtension(sdk.FilePathConfig(""))
	require.NoError(t, err)
	assert.Equal(t, "agent", ext.Name())
}

func TestAgentExtension_Close(t *testing.T) {
	ext, err := NewAgentExtension(sdk.FilePathConfig(""))
	require.NoError(t, err)
	assert.NoError(t, ext.Close())
}

func TestAgentExtension_SubscribeAndClose(t *testing.T) {
	ext, err := NewAgentExtension(sdk.FilePathConfig(""))
	require.NoError(t, err)

	b := bus.New()
	defer b.Close()

	require.NoError(t, ext.Subscribe(b))
	require.NoError(t, ext.Close())
}

func TestAgentExtension_SubscribeTwiceWithoutClose(t *testing.T) {
	ext, err := NewAgentExtension(sdk.FilePathConfig(""))
	require.NoError(t, err)

	b := bus.New()
	defer b.Close()

	require.NoError(t, ext.Subscribe(b))

	err = ext.Subscribe(b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Subscribe called twice without Close")

	require.NoError(t, ext.Close())
}

func TestAgentExtension_ReSubscribeAfterClose(t *testing.T) {
	ext, err := NewAgentExtension(sdk.FilePathConfig(""))
	require.NoError(t, err)

	b := bus.New()
	defer b.Close()

	require.NoError(t, ext.Subscribe(b))
	require.NoError(t, ext.Close())

	require.NoError(t, ext.Subscribe(b))
	require.NoError(t, ext.Close())
}

func TestAgentExtension_RegisterAsExtension(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	sdk.RegisterExtension("agent", func(cfg sdk.Config, _ struct{}) (sdk.Extension, error) {
		return NewAgentExtension(cfg)
	})

	ext, err := sdk.GetExtension("agent", nil)
	require.NoError(t, err, "GetExtension(agent)")
	assert.Equal(t, "agent", ext.Name())

	_, ok := ext.(*AgentExtension)
	require.True(t, ok, "expected *AgentExtension, got %T", ext)
}

func TestAgentExtension_ProjectDir(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, ".weave", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	require.NoError(t, os.WriteFile(configPath, []byte(`{}`), 0o644))

	ext, err := NewAgentExtension(sdk.FilePathConfig(configPath))
	require.NoError(t, err)

	assert.Equal(t, projectDir, ext.projectDir())
}

func TestAgentExtension_ProjectDir_FromFilePath(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), ".weave", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o755))
	require.NoError(t, os.WriteFile(configPath, []byte(`{}`), 0o644))

	cfg := sdk.FilePathConfig(configPath)
	ext, err := NewAgentExtension(cfg)
	require.NoError(t, err)

	// FilePathConfig returns empty ProjectDir, so it falls back to deriving from FilePath.
	// Since config is inside .weave/, projectDir strips .weave/ and returns its parent.
	assert.Equal(t, filepath.Dir(filepath.Dir(configPath)), ext.projectDir())
}

func TestGlobalConfigDir(t *testing.T) {
	dir := globalConfigDir()
	require.NotEmpty(t, dir)
	assert.Contains(t, dir, ".weave")
}

func TestAgentExtension_Subscribe_RegistersSkillCommands(t *testing.T) {
	resetRegistries()
	defer resetRegistries()

	skillDir := filepath.Join(t.TempDir(), "my-skill")
	writeSkillMD(t, skillDir, "my-skill", "Does things", "# Instructions")

	var registeredCmds []string

	ui := &UIMock{
		RegisterCommandFunc: func(name string, handler func(args string) error) {
			registeredCmds = append(registeredCmds, name)
		},
	}

	sdk.RegisterUI("tui", ui)

	defer sdk.ResetUIRegistry()

	ext, err := NewAgentExtension(sdk.FilePathConfig(""))
	require.NoError(t, err)

	ext.skillDiscoveryPaths = []string{filepath.Dir(skillDir)}

	b := bus.New()
	defer b.Close()

	require.NoError(t, ext.Subscribe(b))

	require.NoError(t, ext.Close())

	assert.Contains(t, registeredCmds, "/skill:my-skill")
}

func TestMakeSkillHandler(t *testing.T) {
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

	handler := makeSkillHandler(skill, b)

	require.NoError(t, handler(""))
	require.Len(t, published, 1)
	assert.Equal(t, TopicPrompt, published[0].Topic)

	payload := published[0].Payload.(string)
	assert.Contains(t, payload, `<skill name="test-skill" location="/path/to/test-skill/SKILL.md">`)
	assert.Contains(t, payload, "References are relative to /path/to/test-skill.")
	assert.Contains(t, payload, "# Instructions\nDo the thing.")
	assert.Contains(t, payload, "</skill>")
}

func TestMakeSkillHandler_WithArgs(t *testing.T) {
	skill := Skill{
		Name:     "test-skill",
		FilePath: "/path/to/test-skill/SKILL.md",
		BaseDir:  "/path/to/test-skill",
	}
	skill.body = "# Instructions"

	var published []sdk.Event

	b := &BusMock{
		PublishFunc: func(event sdk.Event) {
			published = append(published, event)
		},
	}

	handler := makeSkillHandler(skill, b)

	require.NoError(t, handler("extra args"))
	require.Len(t, published, 1)

	payload := published[0].Payload.(string)
	assert.Contains(t, payload, "</skill>")
	assert.Contains(t, payload, "extra args")

	_, afterClosing, _ := strings.Cut(payload, "</skill>")
	assert.Contains(t, strings.TrimSpace(afterClosing), "extra args")
}

func TestMakeSkillHandler_ArgsNotXMLEscaped(t *testing.T) {
	skill := Skill{
		Name:     "test-skill",
		FilePath: "/path/to/test-skill/SKILL.md",
		BaseDir:  "/path/to/test-skill",
	}
	skill.body = "# Instructions"

	var published []sdk.Event

	b := &BusMock{
		PublishFunc: func(event sdk.Event) {
			published = append(published, event)
		},
	}

	handler := makeSkillHandler(skill, b)

	require.NoError(t, handler(`<div class="x">`))
	require.Len(t, published, 1)

	payload := published[0].Payload.(string)
	_, afterClosing, _ := strings.Cut(payload, "</skill>")
	assert.Contains(t, afterClosing, `<div class="x">`)
	assert.NotContains(t, afterClosing, "&lt;")
}

func TestMakeSkillHandler_FrontmatterStripped(t *testing.T) {
	skillDir := filepath.Join(t.TempDir(), "fm-skill")
	writeSkillMD(t, skillDir, "fm-skill", "Tests frontmatter stripping", "Real instructions here.")

	skill, err := loadSkillFromDir(skillDir)
	require.NoError(t, err)

	var published []sdk.Event

	b := &BusMock{
		PublishFunc: func(event sdk.Event) {
			published = append(published, event)
		},
	}

	handler := makeSkillHandler(skill, b)
	require.NoError(t, handler(""))

	require.Len(t, published, 1)
	payload := published[0].Payload.(string)
	assert.NotContains(t, payload, "---")
	assert.NotContains(t, payload, "name: fm-skill")
	assert.Contains(t, payload, "Real instructions here.")
}
