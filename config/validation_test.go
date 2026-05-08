package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validFile() *File {
	return &File{
		UI:   "tui",
		Core: CoreConfig{AgentLoop: "loop"},
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	err := Validate(validFile())
	assert.NoError(t, err)
}

func TestValidate_ValidNoneUI(t *testing.T) {
	f := validFile()
	f.UI = "none"
	err := Validate(f)
	assert.NoError(t, err)
}

func TestValidate_InvalidUI(t *testing.T) {
	f := validFile()
	f.UI = "web"
	err := Validate(f)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Equal(t, "ui", errs[0].Field)
	assert.Contains(t, errs[0].Message, "web")
	assert.Contains(t, errs[0].Message, `"tui" or "none"`)
}

func TestValidate_EmptyAgentLoop(t *testing.T) {
	f := validFile()
	f.Core.AgentLoop = ""
	err := Validate(f)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Equal(t, "core.agent_loop", errs[0].Field)
}

func TestValidate_InvalidAgentLoopChars(t *testing.T) {
	f := validFile()
	f.Core.AgentLoop = "my loop!"
	err := Validate(f)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Equal(t, "core.agent_loop", errs[0].Field)
	assert.Contains(t, errs[0].Message, "my loop!")
}

func TestValidate_ExcludeExtensionsValid(t *testing.T) {
	f := validFile()
	f.ExcludeExtensions = []string{"bash", "custom-ext"}
	err := Validate(f)
	assert.NoError(t, err)
}

func TestValidate_ExcludeExtensionsInvalidName(t *testing.T) {
	f := validFile()
	f.ExcludeExtensions = []string{"bad ext!"}
	err := Validate(f)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Equal(t, "exclude_extensions[0]", errs[0].Field)
	assert.Contains(t, errs[0].Message, "bad ext!")
}

func TestValidate_ProviderEntryValid(t *testing.T) {
	f := validFile()
	f.Providers = map[string]any{
		"anthropic": map[string]any{
			"api_key": "test-key",
			"model":   "claude-opus-4-7",
		},
	}

	err := Validate(f)
	assert.NoError(t, err)
}

func TestValidate_ProviderEntryInvalidType(t *testing.T) {
	f := validFile()
	f.Providers = map[string]any{
		"custom": "not-an-object",
	}

	err := Validate(f)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Equal(t, "providers.custom", errs[0].Field)
	assert.Contains(t, errs[0].Message, "expected object")
}

func TestValidate_MultipleErrors(t *testing.T) {
	f := &File{
		UI:                "web",
		Core:              CoreConfig{AgentLoop: ""},
		ExcludeExtensions: []string{"bad!name"},
	}

	err := Validate(f)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	assert.Len(t, errs, 3, "should collect all validation errors")

	fields := make([]string, len(errs))
	for i, e := range errs {
		fields[i] = e.Field
	}

	assert.Contains(t, fields, "ui")
	assert.Contains(t, fields, "core.agent_loop")
	assert.Contains(t, fields, "exclude_extensions[0]")
}

func TestValidationError_ErrorFormat(t *testing.T) {
	e := ValidationError{Field: "ui", Message: `invalid value "web"`}
	assert.Equal(t, `config.ui: invalid value "web"`, e.Error())
}

func TestValidationErrors_ErrorFormat(t *testing.T) {
	errs := ValidationErrors{
		{Field: "ui", Message: "bad ui"},
		{Field: "core.agent_loop", Message: "empty"},
	}
	assert.Equal(t, "config.ui: bad ui; config.core.agent_loop: empty", errs.Error())
}

func TestValidateWithConfigDir_IntegratedWithLoad(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".weave.yaml", "ui: web\n")

	_, _, _, err := LoadFromDir(dir, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validate config")
	assert.Contains(t, err.Error(), "ui")
}
