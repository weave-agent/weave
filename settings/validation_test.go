package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validSettings() *Settings {
	return &Settings{
		UIExtension: "tui",
		AgentLoop:   "agent",
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	err := Validate(validSettings())
	assert.NoError(t, err)
}

func TestValidate_ValidNoneUI(t *testing.T) {
	f := validSettings()
	f.UIExtension = "none"
	err := Validate(f)
	assert.NoError(t, err)
}

func TestValidate_AnyUIValueAccepted(t *testing.T) {
	f := validSettings()
	f.UIExtension = "web"
	err := Validate(f)
	assert.NoError(t, err)
}

func TestValidate_AnyAgentLoopAccepted(t *testing.T) {
	f := validSettings()
	f.AgentLoop = "my loop!"
	err := Validate(f)
	assert.NoError(t, err)
}

func TestValidate_EmptyAgentLoopAccepted(t *testing.T) {
	f := validSettings()
	f.AgentLoop = ""
	err := Validate(f)
	assert.NoError(t, err)
}

func TestValidate_ExcludeExtensionsValid(t *testing.T) {
	f := validSettings()
	f.ExcludeExtensions = []string{"bash", "custom-ext"}
	err := Validate(f)
	assert.NoError(t, err)
}

func TestValidate_ExcludeExtensionsAnyNameAccepted(t *testing.T) {
	f := validSettings()
	f.ExcludeExtensions = []string{"bad ext!"}
	err := Validate(f)
	assert.NoError(t, err)
}

func TestValidate_ProviderEntryAnyTypeAccepted(t *testing.T) {
	f := validSettings()
	f.Providers = map[string]any{
		"custom": "not-an-object",
	}

	err := Validate(f)
	assert.NoError(t, err)
}

func TestValidate_ProviderEntryObjectAccepted(t *testing.T) {
	f := validSettings()
	f.Providers = map[string]any{
		"anthropic": map[string]any{
			"api_key": "test-key",
			"model":   "claude-opus-4-7",
		},
	}

	err := Validate(f)
	assert.NoError(t, err)
}

func TestValidate_InvalidOutput(t *testing.T) {
	f := validSettings()
	f.Output = "xml"
	err := Validate(f)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Equal(t, "output", errs[0].Field)
	assert.Contains(t, errs[0].Message, "xml")
}

func TestValidate_MultipleOutputErrors(t *testing.T) {
	f := &Settings{
		UIExtension: "web",
		AgentLoop:   "",
		Output:      "yaml",
	}

	err := Validate(f)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Equal(t, "output", errs[0].Field)
}

func TestValidationError_ErrorFormat(t *testing.T) {
	e := ValidationError{Field: "ui_extension", Message: `invalid value "web"`}
	assert.Equal(t, `settings.ui_extension: invalid value "web"`, e.Error())
}

func TestValidationErrors_ErrorFormat(t *testing.T) {
	errs := ValidationErrors{
		{Field: "ui_extension", Message: "bad ui"},
		{Field: "agent_loop", Message: "empty"},
	}
	assert.Equal(t, "settings.ui_extension: bad ui; settings.agent_loop: empty", errs.Error())
}

func TestValidate_SandboxDefaultsValid(t *testing.T) {
	f := validSettings()
	// Empty/zero sandbox config is valid (defaults apply at runtime).
	err := Validate(f)
	assert.NoError(t, err)
}

func TestValidate_SandboxAnyModeAccepted(t *testing.T) {
	f := validSettings()
	f.Sandbox.Mode = "strict"
	err := Validate(f)
	assert.NoError(t, err)
}

func TestValidate_SandboxEmptyPathsAccepted(t *testing.T) {
	f := validSettings()
	f.Sandbox.Writable = []string{""}
	f.Sandbox.DenyWrite = []string{"  "}
	f.Sandbox.DenyRead = []string{" "}
	err := Validate(f)
	assert.NoError(t, err)
}

func TestValidateWithConfigDir_IntegratedWithLoad(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"web"}`)

	_, _, _, err := LoadFromDir(dir, nil)
	assert.NoError(t, err)
}

func TestValidate_ContinueOnly(t *testing.T) {
	f := validSettings()
	f.Continue = true
	err := Validate(f)
	assert.NoError(t, err)
}

func TestValidate_ResumeOnly(t *testing.T) {
	f := validSettings()
	f.Resume = "sess-abc123"
	err := Validate(f)
	assert.NoError(t, err)
}

func TestValidate_MutualExclusion(t *testing.T) {
	f := validSettings()
	f.Continue = true
	f.Resume = "sess-abc123"
	err := Validate(f)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Equal(t, "continue", errs[0].Field)
	assert.Contains(t, errs[0].Message, "mutually exclusive")
}
