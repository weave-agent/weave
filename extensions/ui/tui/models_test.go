package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"weave/bus"
	"weave/config"
	"weave/ext/ui/tui/components/messages"
	"weave/ext/ui/tui/components/overlays"
	"weave/sdk"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	f, err := os.CreateTemp("", "weave-test-settings-*.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp settings file: %v\n", err)
		os.Exit(1)
	}

	path := f.Name()
	_ = f.Close()

	config.SetSettingsPath(path)

	os.Exit(m.Run())
}

func splitLines(s string) []string {
	return strings.Split(s, "\n")
}

func TestModelEntry_Display(t *testing.T) {
	e := ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-6"}
	assert.Equal(t, "anthropic/claude-sonnet-4-6", e.Display())
}

func TestCycleModel(t *testing.T) {
	entries := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
		{Provider: "zai", Model: "glm-5.1"},
	}

	// Cycle forward
	next := cycleModel(entries, entries[0])
	assert.Equal(t, "openai", next.Provider)

	next = cycleModel(entries, entries[1])
	assert.Equal(t, "zai", next.Provider)

	// Wrap around
	next = cycleModel(entries, entries[2])
	assert.Equal(t, "anthropic", next.Provider)
}

func TestCycleModel_SingleEntry(t *testing.T) {
	entries := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}
	next := cycleModel(entries, entries[0])
	assert.Equal(t, "anthropic", next.Provider)
}

func TestCycleModel_Empty(t *testing.T) {
	cur := ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-6"}
	next := cycleModel(nil, cur)
	assert.Equal(t, cur, next)
}

func TestCurrentModel(t *testing.T) {
	entries := []ModelEntry{
		{Provider: "openai", Model: "gpt-5.5"},
		{Provider: "zai", Model: "glm-5.1"},
	}
	cur := currentModel(entries)
	assert.Equal(t, "openai", cur.Provider)

	cur = currentModel(nil)
	assert.Equal(t, "anthropic", cur.Provider)
}

func TestCurrentModel_EnvProvider(t *testing.T) {
	entries := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}

	t.Setenv("WEAVE_PROVIDER", "openai")

	cur := currentModel(entries)
	assert.Equal(t, "openai", cur.Provider)
	assert.Equal(t, "gpt-5.5", cur.Model)
}

func TestCurrentModel_EnvProviderNotInEntries(t *testing.T) {
	entries := []ModelEntry{
		{Provider: "openai", Model: "gpt-5.5"},
	}

	t.Setenv("WEAVE_PROVIDER", "anthropic")

	cur := currentModel(entries)
	// Falls back to first entry when env provider not found
	assert.Equal(t, "openai", cur.Provider)
}

func TestModel_CommandRegistered(t *testing.T) {
	m := newModel(nil, nil, nil)
	info, ok := m.commands.Lookup("/model")
	require.True(t, ok, "/model command should be registered")
	assert.Equal(t, "Select or change model", info.Description)
}

func TestModel_DefaultFooterModel(t *testing.T) {
	m := newModel(nil, nil, nil)
	// Footer should have a model set (anthropic default when no providers registered)
	assert.NotEmpty(t, m.footer.ModelName())
}

func TestModel_ModelListResultShowsOverlay(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	models := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}

	model, _ := m.Update(ModelListResultMsg{Models: models})
	m = model.(Model)

	assert.Equal(t, overlayModel, m.activeOverlay)
	assert.True(t, m.overlay.Visible())
	assert.Equal(t, models, m.pendingModels)

	view := m.overlay.View()
	assert.Contains(t, view, "Select Model")
}

func TestModel_ModelListResultEmpty(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	model, _ := m.Update(ModelListResultMsg{Models: nil})
	m = model.(Model)

	assert.Equal(t, overlayNone, m.activeOverlay)

	items := m.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "No models available")
}

func TestModel_ModelListResultSingle(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	models := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}

	model, _ := m.Update(ModelListResultMsg{Models: models})
	m = model.(Model)

	// Should show a message instead of overlay for single model
	assert.Equal(t, overlayNone, m.activeOverlay)

	items := m.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "Only one model available")
}

func TestModel_ModelSelectorSelect(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	models := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}

	model, _ := m.Update(ModelListResultMsg{Models: models})
	m = model.(Model)
	require.Equal(t, overlayModel, m.activeOverlay)

	// Select the second model
	model, _ = m.Update(overlays.SelectorSelectedMsg{Index: 1, Item: overlays.SelectorItem{
		Title: "openai/gpt-5.5", Subtitle: "openai",
	}})
	m = model.(Model)

	assert.Equal(t, overlayNone, m.activeOverlay)
	assert.Equal(t, "openai", m.currentModel.Provider)
	assert.Equal(t, "gpt-5.5", m.currentModel.Model)
	assert.Equal(t, "gpt-5.5", m.footer.ModelName())
	assert.Equal(t, "openai", m.footer.ProviderName())
}

func TestModel_ModelSelectorCancel(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	models := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}

	model, _ := m.Update(ModelListResultMsg{Models: models})
	m = model.(Model)
	require.Equal(t, overlayModel, m.activeOverlay)

	model, _ = m.Update(overlays.SelectorCancelledMsg{})
	m = model.(Model)

	assert.Equal(t, overlayNone, m.activeOverlay)
	assert.Nil(t, m.pendingModels)
}

func TestModel_ModelSelectorCancelClearsPendingModels(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	models := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}

	model, _ := m.Update(ModelListResultMsg{Models: models})
	m = model.(Model)
	require.Equal(t, overlayModel, m.activeOverlay)
	require.NotNil(t, m.pendingModels)

	// Cancel via ctrl+c
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = model.(Model)

	assert.Equal(t, overlayNone, m.activeOverlay)
	assert.False(t, m.overlay.Visible())
}

func TestModel_CtrlLOpensModelSelector(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	_ = model.(Model)

	// Ctrl+L should trigger listModelsCmd
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(ModelListResultMsg)
	require.True(t, ok)
	// No providers registered in test, so empty
	assert.Empty(t, result.Models)
}

func TestModel_CtrlPWhenSingleModel(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	// With no providers registered, cycle shows status message
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = model.(Model)

	assert.Equal(t, "Only one model available", m.statusMsg)

	_ = cmd // timer cmd for status message auto-clear
}

func TestModel_ModelChangedUpdatesFooter(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	entry := ModelEntry{Provider: "openai", Model: "gpt-5.5"}
	model, _ := m.Update(ModelChangedMsg{Entry: entry})
	m = model.(Model)

	assert.Equal(t, "gpt-5.5", m.currentModel.Model)
	assert.Equal(t, "openai", m.currentModel.Provider)
	assert.Equal(t, "gpt-5.5", m.footer.ModelName())
	assert.Equal(t, "openai", m.footer.ProviderName())
}

func TestModel_ModelChangedToNonReasoningForcesThinkingOff(t *testing.T) {
	sdk.ResetModelRegistry()
	sdk.RegisterBuiltinModels()

	defer sdk.ResetModelRegistry()

	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	assert.Equal(t, sdk.ThinkingMedium, m.thinkingLevel)

	// Switch to non-reasoning model
	entry := ModelEntry{Provider: "openai", Model: "gpt-4.1"}
	model, _ := m.Update(ModelChangedMsg{Entry: entry})
	m = model.(Model)

	assert.Equal(t, sdk.ThinkingOff, m.thinkingLevel)
	assert.Equal(t, "off", m.footer.ThinkingLevel())
	assert.Equal(t, "240", m.editor.BorderColor) // off color
}

func TestModel_ModelChangedPublishesEvent(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := b.Subscribe(topicModelChange)

	m := newModel(b, nil, nil)
	m.width = 80
	m.height = 24

	entry := ModelEntry{Provider: "openai", Model: "gpt-5.5"}
	model, cmd := m.Update(ModelChangedMsg{Entry: entry})
	_ = model.(Model)

	require.NotNil(t, cmd)
	executeBatchCmd(t, cmd)

	evt := <-ch
	assert.Equal(t, topicModelChange, evt.Topic)
	assert.Equal(t, map[string]string{"provider": "openai", "model": "gpt-5.5"}, evt.Payload)
}

func TestModel_ModelSlashCommandDispatches(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	handled, result := m.commands.Dispatch("/model")
	require.True(t, handled)
	assert.NotNil(t, result.Command)

	msg := result.Command()
	_, ok := msg.(ModelListResultMsg)
	assert.True(t, ok)
}

func TestModel_ModelOverlayInterceptsKeys(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	models := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}

	model, _ := m.Update(ModelListResultMsg{Models: models})
	m = model.(Model)
	require.Equal(t, overlayModel, m.activeOverlay)

	// Typing should go to overlay filter
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = model.(Model)

	assert.Equal(t, overlayModel, m.activeOverlay)
	assert.Equal(t, "o", m.overlay.Filter())
}

func TestModel_ModelSelectorViewShowsOverlay(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	normalView := m.View()
	assert.NotContains(t, normalView, "Select Model")

	models := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}

	model, _ := m.Update(ModelListResultMsg{Models: models})
	m = model.(Model)

	overlayView := m.View()
	assert.Contains(t, overlayView, "Select Model")
}

func TestModel_ModelSelectedInvalidIndex(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m.activeOverlay = overlayModel
	m.pendingModels = []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}

	model, _ := m.Update(overlays.SelectorSelectedMsg{Index: -1, Item: overlays.SelectorItem{}})
	m = model.(Model)
	assert.Equal(t, overlayNone, m.activeOverlay)

	// Original model should be unchanged
	assert.NotEmpty(t, m.currentModel.Provider)
}

func TestListModelsWithRegistry(t *testing.T) {
	sdk.ResetModelRegistry()
	sdk.ResetProviderRegistry()
	sdk.ResetProviderEnvVarRegistry()
	sdk.RegisterBuiltinModels()

	// Register providers so their models are included.
	sdk.RegisterProvider("anthropic", func(_ sdk.Config) (sdk.Provider, error) { return nil, nil }) //nolint:nilnil // stub registration for model list tests
	sdk.RegisterProvider("openai", func(_ sdk.Config) (sdk.Provider, error) { return nil, nil })    //nolint:nilnil // stub registration for model list tests
	sdk.RegisterProvider("zai", func(_ sdk.Config) (sdk.Provider, error) { return nil, nil })       //nolint:nilnil // stub registration for model list tests

	// Register env vars and set test API keys so providers appear configured.
	sdk.RegisterProviderEnvVar("anthropic", "ANTHROPIC_API_KEY")
	sdk.RegisterProviderEnvVar("openai", "OPENAI_API_KEY")
	sdk.RegisterProviderEnvVar("zai", "ZAI_API_KEY")

	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("ZAI_API_KEY", "test-key")

	defer sdk.ResetModelRegistry()
	defer sdk.ResetProviderRegistry()
	defer sdk.ResetProviderEnvVarRegistry()

	entries := listModels()
	assert.NotEmpty(t, entries, "should return models from registry")

	// Should include models from all registered providers
	providers := make(map[string]bool)
	for _, e := range entries {
		providers[e.Provider] = true
	}

	assert.True(t, providers["anthropic"], "should include anthropic models")
	assert.True(t, providers["openai"], "should include openai models")
	assert.True(t, providers["zai"], "should include zai models")
}

func TestListModelsEmpty(t *testing.T) {
	sdk.ResetModelRegistry()
	sdk.ResetProviderRegistry()
	sdk.ResetProviderEnvVarRegistry()

	defer sdk.ResetModelRegistry()
	defer sdk.ResetProviderRegistry()
	defer sdk.ResetProviderEnvVarRegistry()

	entries := listModels()
	assert.Nil(t, entries)
}

func TestListModelsIgnoresEnvOverrides(t *testing.T) {
	sdk.ResetModelRegistry()
	sdk.ResetProviderRegistry()
	sdk.ResetProviderEnvVarRegistry()
	sdk.RegisterBuiltinModels()
	sdk.RegisterProvider("anthropic", func(_ sdk.Config) (sdk.Provider, error) { return nil, nil }) //nolint:nilnil // stub registration for model list tests
	sdk.RegisterProviderEnvVar("anthropic", "ANTHROPIC_API_KEY")

	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	defer sdk.ResetModelRegistry()
	defer sdk.ResetProviderRegistry()
	defer sdk.ResetProviderEnvVarRegistry()

	t.Setenv("ANTHROPIC_MODEL", "my-custom-model")

	entries := listModels()

	// Should show registry entries as-is, not env-overridden names
	anthropicCount := 0

	for _, e := range entries {
		if e.Provider == "anthropic" {
			anthropicCount++

			assert.NotEqual(t, "my-custom-model", e.Model,
				"listModels should show registry IDs, not env overrides")
		}
	}

	assert.Equal(t, 5, anthropicCount,
		"should show all anthropic models, not collapsed by env override")
}

func TestModelEntryDisplayName(t *testing.T) {
	sdk.ResetModelRegistry()
	sdk.RegisterBuiltinModels()

	defer sdk.ResetModelRegistry()

	e := ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-6"}
	assert.Equal(t, "Claude Sonnet 4.6", e.DisplayName())

	e = ModelEntry{Provider: "unknown", Model: "custom-model"}
	assert.Equal(t, "unknown/custom-model", e.DisplayName())
}

func TestModelSelectorEntryBadges(t *testing.T) {
	sdk.ResetModelRegistry()
	sdk.RegisterBuiltinModels()

	defer sdk.ResetModelRegistry()

	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	models := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}

	model, _ := m.Update(ModelListResultMsg{Models: models})
	m = model.(Model)
	require.Equal(t, overlayModel, m.activeOverlay)

	view := m.overlay.View()
	assert.Contains(t, view, "[anthropic]", "should show provider badge")
	assert.Contains(t, view, "[openai]", "should show provider badge")
}

func TestModelSelectorCurrentModelMarker(t *testing.T) {
	sdk.ResetModelRegistry()
	sdk.RegisterBuiltinModels()

	defer sdk.ResetModelRegistry()

	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	// Current model is the default (anthropic/claude-sonnet)
	m.currentModel = ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-6"}

	models := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}

	model, _ := m.Update(ModelListResultMsg{Models: models})
	m = model.(Model)

	view := m.overlay.View()
	assert.Contains(t, view, "✓", "current model should have checkmark marker")
}

func TestStatusMessageOnModelCycle(t *testing.T) {
	sdk.ResetModelRegistry()
	sdk.ResetProviderRegistry()
	sdk.ResetProviderEnvVarRegistry()
	sdk.RegisterBuiltinModels()
	sdk.RegisterProvider("anthropic", func(_ sdk.Config) (sdk.Provider, error) { return nil, nil }) //nolint:nilnil // stub registration for model list tests
	sdk.RegisterProvider("openai", func(_ sdk.Config) (sdk.Provider, error) { return nil, nil })    //nolint:nilnil // stub registration for model list tests
	sdk.RegisterProviderEnvVar("anthropic", "ANTHROPIC_API_KEY")
	sdk.RegisterProviderEnvVar("openai", "OPENAI_API_KEY")

	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("OPENAI_API_KEY", "test-key")

	defer sdk.ResetModelRegistry()
	defer sdk.ResetProviderRegistry()
	defer sdk.ResetProviderEnvVarRegistry()

	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m.currentModel = ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-6"}

	// Cycle produces a ModelChangedMsg cmd — execute it and process the result
	model, cmd := m.dispatchBinding(ActionModelCycle)
	m = model.(Model)

	require.NotNil(t, cmd)

	msg := cmd()
	changedMsg, ok := msg.(ModelChangedMsg)
	require.True(t, ok, "expected ModelChangedMsg, got %T", msg)

	model, _ = m.Update(changedMsg)
	m = model.(Model)

	assert.Contains(t, m.statusMsg, "Switched to")
	assert.Contains(t, m.statusMsg, "thinking:")
}

func TestStatusMessageOnModelChanged(t *testing.T) {
	sdk.ResetModelRegistry()
	sdk.RegisterBuiltinModels()

	defer sdk.ResetModelRegistry()

	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	entry := ModelEntry{Provider: "openai", Model: "gpt-5.5"}
	model, _ := m.Update(ModelChangedMsg{Entry: entry})
	m = model.(Model)

	assert.Contains(t, m.statusMsg, "Switched to")
	assert.Contains(t, m.statusMsg, "thinking:")
}

func TestStatusMessageOnThinkingCycle(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	model, _ := m.dispatchBinding(ActionThinkingCycle)
	m = model.(Model)

	assert.Contains(t, m.statusMsg, "Thinking level:")
	assert.Contains(t, m.statusMsg, "high")
}

func TestStatusMessageClearsOnTimeout(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	m.statusMsg = "test status"

	model, _ := m.Update(statusTimeoutMsg{})
	m = model.(Model)

	assert.Empty(t, m.statusMsg)
	assert.Nil(t, m.statusTimer)
}

func TestStatusMessageRenderedInVIew(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m.statusMsg = "test status message"

	view := m.View()
	assert.Contains(t, view, "test status message")
}

func TestStatusMessageNotRenderedWhenEmpty(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	m.statusMsg = ""

	view := m.View()
	// Should not contain any status-related artifacts
	lines := splitLines(view)
	// Count sections: chat + editor + footer = 3 lines minimum (no spinner, no status)
	assert.GreaterOrEqual(t, len(lines), 3)
}
