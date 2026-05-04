package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validFile() *File {
	return &File{
		UI: "tui",
		Core: CoreConfig{
			AgentLoop: "loop",
			Providers: []string{"anthropic"},
		},
		Extensions:   []string{"bash"},
		UIExtensions: []string{},
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

func TestValidate_EmptyProviders(t *testing.T) {
	f := validFile()
	f.Core.Providers = nil
	err := Validate(f)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Equal(t, "core.providers", errs[0].Field)
	assert.Contains(t, errs[0].Message, "at least one provider")
}

func TestValidate_InvalidProviderName(t *testing.T) {
	f := validFile()
	f.Core.Providers = []string{"anthropic", "bad name!"}
	err := Validate(f)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Equal(t, "core.providers[1]", errs[0].Field)
	assert.Contains(t, errs[0].Message, "bad name!")
}

func TestValidate_InvalidExtensionName(t *testing.T) {
	f := validFile()
	f.Extensions = []string{"bad ext!"}
	err := Validate(f)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Equal(t, "extensions[0]", errs[0].Field)
	assert.Contains(t, errs[0].Message, "bad ext!")
}

func TestValidate_InvalidUIExtensionName(t *testing.T) {
	f := validFile()
	f.UIExtensions = []string{"bad!"}
	err := Validate(f)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Equal(t, "ui_extensions[0]", errs[0].Field)
}

func TestValidate_PathExtensionExists(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "my-ext")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	writeFile(t, extDir, "main.go", "package main")

	f := validFile()
	f.Extensions = []string{"./my-ext"}

	err := ValidateWithConfigDir(f, dir)
	assert.NoError(t, err)
}

func TestValidate_PathExtensionNotExist(t *testing.T) {
	dir := t.TempDir()

	f := validFile()
	f.Extensions = []string{"./missing-ext"}

	err := ValidateWithConfigDir(f, dir)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	require.Len(t, errs, 1)
	assert.Equal(t, "extensions[0]", errs[0].Field)
	assert.Contains(t, errs[0].Message, "does not exist")
}

func TestValidate_PathExtensionNotDirectory(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "not-a-dir", "content")

	f := validFile()
	f.Extensions = []string{"./not-a-dir"}

	err := ValidateWithConfigDir(f, dir)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	assert.Contains(t, errs[0].Message, "not a directory")
}

func TestValidate_PathExtensionNoGoFiles(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "empty-ext")
	require.NoError(t, os.MkdirAll(extDir, 0o750))
	writeFile(t, extDir, "readme.md", "# hello")

	f := validFile()
	f.Extensions = []string{"./empty-ext"}

	err := ValidateWithConfigDir(f, dir)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	assert.Contains(t, errs[0].Message, "no .go files")
}

func TestValidate_PathSkippedWithoutConfigDir(t *testing.T) {
	f := validFile()
	f.Extensions = []string{"./nonexistent"}

	err := Validate(f)
	assert.NoError(t, err, "path entries should be skipped when configDir is empty")
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
		UI: "web",
		Core: CoreConfig{
			AgentLoop: "",
			Providers: []string{},
		},
		Extensions: []string{"bad!name"},
	}

	err := Validate(f)
	require.Error(t, err)

	var errs ValidationErrors
	require.ErrorAs(t, err, &errs)
	assert.Len(t, errs, 4, "should collect all validation errors")

	fields := make([]string, len(errs))
	for i, e := range errs {
		fields[i] = e.Field
	}

	assert.Contains(t, fields, "ui")
	assert.Contains(t, fields, "core.agent_loop")
	assert.Contains(t, fields, "core.providers")
	assert.Contains(t, fields, "extensions[0]")
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
	writeFile(t, dir, ".weave.yaml", "ui: web\nextensions: []\n")

	_, _, _, err := LoadFromDir(dir, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validate config")
	assert.Contains(t, err.Error(), "ui")
}
