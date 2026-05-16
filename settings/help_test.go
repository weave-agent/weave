package settings

import (
	"context"
	"flag"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"weave/sdk"
	"weave/sdk/model"
)

// --- Dummy implementations for registration ---

type dummyTool struct{ name string }

func (d dummyTool) Name() string { return d.name }

func (d dummyTool) Definition() sdk.ToolDef { return sdk.ToolDef{} }

func (d dummyTool) Execute(_ context.Context, _ map[string]any) (sdk.ToolResult, error) {
	return sdk.ToolResult{}, nil
}

type dummyProvider struct{}

func (d dummyProvider) Stream(_ context.Context, _ sdk.ProviderRequest, _ ...model.StreamOption) (<-chan sdk.ProviderEvent, error) {
	ch := make(chan sdk.ProviderEvent)
	close(ch)

	return ch, nil
}

type dummyExtension struct{}

func (d dummyExtension) Name() string { return "dummy" }

func (d dummyExtension) Subscribe(_ sdk.Bus) error { return nil }

func (d dummyExtension) Close() error { return nil }

// --- Config structs for testing ---

type testToolConfig struct {
	Timeout int    `json:"timeout" default:"120" description:"Command timeout" flag:"timeout" short:"t" env:"TIMEOUT"`
	Shell   string `json:"shell" default:"bash" description:"Shell to use"`
}

type testProviderConfig struct {
	Model   string `json:"model" default:"gpt-4" description:"Model name" flag:"model"`
	BaseURL string `json:"base_url" description:"API base URL" validate:"url"`
}

type testExtensionConfig struct {
	Enabled bool `json:"enabled" default:"true" description:"Enable extension"`
}

func resetAllRegistries(t *testing.T) {
	t.Helper()
	sdk.ResetToolRegistry()
	sdk.ResetProviderRegistry()
	sdk.ResetExtensionRegistry()
	sdk.ResetSchemas()
}

func TestHelpError_IsFlagErrHelp(t *testing.T) {
	he := &HelpError{Text: "test help"}
	require.ErrorIs(t, he, flag.ErrHelp, "HelpError should satisfy errors.Is(..., flag.ErrHelp)")
}

func TestHelpError_ErrorReturnsText(t *testing.T) {
	he := &HelpError{Text: "my help text"}
	assert.Equal(t, "my help text", he.Error())
}

func TestGenerateFullHelp_NoSchemas(t *testing.T) {
	resetAllRegistries(t)

	text := GenerateFullHelp()
	assert.Contains(t, text, "Usage: weave")
	assert.Contains(t, text, "Global flags:")
	assert.Contains(t, text, "--prompt")
	assert.Contains(t, text, "-p")
	assert.Contains(t, text, "--debug")
	assert.Contains(t, text, "Enable debug logging")
	assert.NotContains(t, text, "Tool options:")
}

func TestGenerateFullHelp_WithToolSchemas(t *testing.T) {
	resetAllRegistries(t)

	sdk.RegisterTool("bash", func(_ sdk.Config, _ sdk.PreferenceReader, _ testToolConfig) (sdk.Tool, error) {
		return dummyTool{name: "bash"}, nil
	})

	text := GenerateFullHelp()
	assert.Contains(t, text, "Tool options:")
	assert.Contains(t, text, "bash")
	assert.Contains(t, text, "--bash-timeout")
	assert.Contains(t, text, "-t,")
	assert.Contains(t, text, "Command timeout")
	assert.Contains(t, text, "default: 120")
	assert.Contains(t, text, "env: TIMEOUT")
	assert.Contains(t, text, "--bash-shell")
	assert.Contains(t, text, "default: bash")
}

func TestGenerateFullHelp_WithProviderSchemas(t *testing.T) {
	resetAllRegistries(t)

	sdk.RegisterProvider[testProviderConfig, struct{}]("openai", func(_ sdk.Config, _ testProviderConfig, _ struct{}) (sdk.Provider, error) {
		return dummyProvider{}, nil
	})

	text := GenerateFullHelp()
	assert.Contains(t, text, "Provider options:")
	assert.Contains(t, text, "openai")
	assert.Contains(t, text, "--openai-model")
	assert.Contains(t, text, "default: gpt-4")
	assert.Contains(t, text, "--openai-base_url")
	assert.Contains(t, text, "API base URL")
}

func TestGenerateFullHelp_AllScopes(t *testing.T) {
	resetAllRegistries(t)

	sdk.RegisterTool("bash", func(_ sdk.Config, _ sdk.PreferenceReader, _ testToolConfig) (sdk.Tool, error) {
		return dummyTool{name: "bash"}, nil
	})
	sdk.RegisterProvider[testProviderConfig, struct{}]("openai", func(_ sdk.Config, _ testProviderConfig, _ struct{}) (sdk.Provider, error) {
		return dummyProvider{}, nil
	})
	sdk.RegisterExtension("sandbox", func(_ sdk.Config, _ sdk.PreferenceReader, _ testExtensionConfig) (sdk.Extension, error) {
		return dummyExtension{}, nil
	})

	text := GenerateFullHelp()

	// Check all scope sections appear.
	assert.Contains(t, text, "Tool options:")
	assert.Contains(t, text, "Provider options:")
	assert.Contains(t, text, "Extension options:")

	// Check entries within each scope.
	assert.Contains(t, text, "bash")
	assert.Contains(t, text, "openai")
	assert.Contains(t, text, "sandbox")

	// Check ordering: tools before providers before extensions.
	toolsIdx := strings.Index(text, "Tool options:")
	providersIdx := strings.Index(text, "Provider options:")
	extensionsIdx := strings.Index(text, "Extension options:")

	require.Less(t, toolsIdx, providersIdx, "tools should come before providers")
	require.Less(t, providersIdx, extensionsIdx, "providers should come before extensions")
}

func TestGenerateFullHelp_SortsWithinScope(t *testing.T) {
	resetAllRegistries(t)

	sdk.RegisterTool("zed", func(_ sdk.Config, _ sdk.PreferenceReader, _ testToolConfig) (sdk.Tool, error) {
		return dummyTool{name: "zed"}, nil
	})
	sdk.RegisterTool("alpha", func(_ sdk.Config, _ sdk.PreferenceReader, _ testToolConfig) (sdk.Tool, error) {
		return dummyTool{name: "alpha"}, nil
	})

	text := GenerateFullHelp()
	zedIdx := strings.Index(text, "zed")
	alphaIdx := strings.Index(text, "alpha")
	require.Less(t, alphaIdx, zedIdx, "entries should be sorted alphabetically")
}

func TestGenerateFullHelp_WithUISchemas(t *testing.T) {
	resetAllRegistries(t)

	type testUIConfig struct {
		Theme string `json:"theme" default:"dark" description:"UI theme"`
	}

	sdk.RegisterExtensionWithScope("tui", "ui", func(_ sdk.Config, _ sdk.PreferenceReader, _ testUIConfig) (sdk.Extension, error) {
		return dummyExtension{}, nil
	})

	text := GenerateFullHelp()
	assert.Contains(t, text, "UI options:")
	assert.Contains(t, text, "tui")
	assert.Contains(t, text, "--tui-theme")
	assert.Contains(t, text, "UI theme")
}

func TestGenerateFullHelp_SkipsEmptySchemas(t *testing.T) {
	resetAllRegistries(t)

	// Register a tool with struct{} (no fields).
	sdk.RegisterTool("empty", func(_ sdk.Config, _ sdk.PreferenceReader, _ struct{}) (sdk.Tool, error) {
		return dummyTool{name: "empty"}, nil
	})

	text := GenerateFullHelp()
	assert.NotContains(t, text, "empty")
}

func TestLoadFromDir_HelpFlagPassesThrough(t *testing.T) {
	resetAllRegistries(t)

	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui"}`)

	// --help is no longer intercepted by LoadFromDir; it passes through to the generated binary.
	_, _, rest, err := LoadFromDir(dir, []string{"--help"})
	require.NoError(t, err)
	assert.Equal(t, []string{"--help"}, rest)
}

func TestLoadFromDir_HelpShortFlagPassesThrough(t *testing.T) {
	resetAllRegistries(t)

	dir := t.TempDir()
	writeFile(t, dir, ".weave/settings.json", `{"ui_extension":"tui"}`)

	// -h is no longer intercepted by LoadFromDir; it passes through to the generated binary.
	_, _, rest, err := LoadFromDir(dir, []string{"-h"})
	require.NoError(t, err)
	assert.Equal(t, []string{"-h"}, rest)
}
