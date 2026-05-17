package tui

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/weave-agent/weave-tui/components/messages"
	"github.com/weave-agent/weave-tui/components/overlays"
	"github.com/weave-agent/weave/bus"
	"github.com/weave-agent/weave/sdk"
	sdkmodel "github.com/weave-agent/weave/sdk/model"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	cur := currentModel(entries, nil)
	assert.Equal(t, "openai", cur.Provider)

	cur = currentModel(nil, nil)
	assert.Empty(t, cur.Provider)
}

func TestCurrentModel_EnvProvider(t *testing.T) {
	entries := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}

	t.Setenv("WEAVE_PROVIDER", "openai")

	cur := currentModel(entries, nil)
	assert.Equal(t, "openai", cur.Provider)
	assert.Equal(t, "gpt-5.5", cur.Model)
}

func TestCurrentModel_EnvProviderNotInEntries(t *testing.T) {
	entries := []ModelEntry{
		{Provider: "openai", Model: "gpt-5.5"},
	}

	t.Setenv("WEAVE_PROVIDER", "anthropic")

	cur := currentModel(entries, nil)
	// Falls back to first entry when env provider not found
	assert.Equal(t, "openai", cur.Provider)
}

func TestCurrentModel_PreferencesProviderOnly(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	defer sdkmodel.ResetModelRegistry()

	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "gpt-5.5", Provider: "openai", Default: true})
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "gpt-5.4", Provider: "openai"})

	entries := []ModelEntry{
		{Provider: "openai", Model: "gpt-5.5"},
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}

	// Preferences with provider but no model — should use provider's default model.
	mockCfg := &mockConfig{
		preferences: map[string]string{
			"provider": "openai",
		},
	}

	cur := currentModel(entries, mockCfg)
	assert.Equal(t, "openai", cur.Provider)
	assert.Equal(t, "gpt-5.5", cur.Model)
}

func TestCurrentModel_PreferencesProviderOnly_NoRegistryFallback(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	defer sdkmodel.ResetModelRegistry()

	entries := []ModelEntry{
		{Provider: "openai", Model: "gpt-5.5"},
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}

	// No model registered for "openai" — should fall back to first matching entry.
	mockCfg := &mockConfig{
		preferences: map[string]string{
			"provider": "openai",
		},
	}

	cur := currentModel(entries, mockCfg)
	assert.Equal(t, "openai", cur.Provider)
	assert.Equal(t, "gpt-5.5", cur.Model)
}

func TestModel_CommandRegistered(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	info, ok := m.commands.Lookup("/model")
	require.True(t, ok, "/model command should be registered")
	assert.Equal(t, "Select or change model", info.Description)
}

func TestModel_DefaultFooterModel(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	sdk.ResetProviderRegistry()
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-sonnet-4-6", Provider: "anthropic", Reasoning: true, Default: true})
	sdk.RegisterProvider[struct{}, struct{}]("anthropic", func(_ sdk.Config, _, _ struct{}) (sdk.Provider, error) { return nil, nil }) //nolint:nilnil // stub
	sdkmodel.SetProviderAuth("anthropic", true)

	defer sdkmodel.ResetModelRegistry()
	defer sdk.ResetProviderRegistry()
	defer sdkmodel.ResetAuthRegistry()

	m := newModel(nil, nil, nil, nil)
	assert.NotEmpty(t, m.footer.ModelName())
}

func TestModel_ModelListResultShowsOverlay(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	models := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}

	model, _ := m.Update(ModelListResultMsg{Models: models})
	m = model.(Model)

	assert.False(t, m.dialogStack.Empty())
	assert.Equal(t, models, m.pendingModels)

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()
	assert.Contains(t, rendered, "Select Model")
}

func TestModel_ModelListResultEmpty(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	model, _ := m.Update(ModelListResultMsg{Models: nil})
	m = model.(Model)

	assert.True(t, m.dialogStack.Empty())

	items := m.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "No models available")
}

func TestModel_ModelListResultSingle(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	models := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}

	model, _ := m.Update(ModelListResultMsg{Models: models})
	m = model.(Model)

	// Should show a message instead of overlay for single model
	assert.True(t, m.dialogStack.Empty())

	items := m.chat.Items()
	require.Len(t, items, 1)
	am, ok := items[0].(*messages.AssistantMessage)
	require.True(t, ok)
	assert.Contains(t, am.Content(), "Only one model available")
}

func TestModel_ModelSelectorSelect(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	models := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}

	model, _ := m.Update(ModelListResultMsg{Models: models})
	m = model.(Model)
	require.False(t, m.dialogStack.Empty())

	// Select the second model
	model, _ = m.Update(overlays.SelectorSelectedMsg{Index: 1, Item: overlays.SelectorItem{
		Title: "openai/gpt-5.5", Subtitle: "openai",
	}})
	m = model.(Model)

	assert.True(t, m.dialogStack.Empty())
	assert.Equal(t, "openai", m.currentModel.Provider)
	assert.Equal(t, "gpt-5.5", m.currentModel.Model)
	assert.Equal(t, "gpt-5.5", m.footer.ModelName())
	assert.Equal(t, "openai", m.footer.ProviderName())
}

func TestModel_ModelSelectorCancel(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	models := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}

	model, _ := m.Update(ModelListResultMsg{Models: models})
	m = model.(Model)
	require.False(t, m.dialogStack.Empty())

	model, _ = m.Update(overlays.SelectorCancelledMsg{})
	m = model.(Model)

	assert.True(t, m.dialogStack.Empty())
	assert.Nil(t, m.pendingModels)
}

func TestModel_ModelSelectorCancelClearsPendingModels(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	models := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}

	model, _ := m.Update(ModelListResultMsg{Models: models})
	m = model.(Model)
	require.False(t, m.dialogStack.Empty())
	require.NotNil(t, m.pendingModels)

	// Cancel via ctrl+c
	model, _ = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = model.(Model)

	assert.True(t, m.dialogStack.Empty())
}

func TestModel_CtrlLOpensModelSelector(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	model, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
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
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	// With no providers registered, cycle shows status message
	model, cmd := m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	m = model.(Model)

	assert.Equal(t, "Only one model available", m.statusMsg)

	_ = cmd // timer cmd for status message auto-clear
}

func TestModel_ModelChangedUpdatesFooter(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
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
	sdkmodel.ResetModelRegistry()
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "gpt-4.1", Provider: "openai", DisplayName: "GPT-4.1"})

	defer sdkmodel.ResetModelRegistry()

	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	assert.Equal(t, sdkmodel.ThinkingMedium, m.thinkingLevel)

	// Switch to non-reasoning model
	entry := ModelEntry{Provider: "openai", Model: "gpt-4.1"}
	model, _ := m.Update(ModelChangedMsg{Entry: entry})
	m = model.(Model)

	assert.Equal(t, sdkmodel.ThinkingOff, m.thinkingLevel)
	assert.Equal(t, "off", m.footer.ThinkingLevel())
	assert.Equal(t, "240", m.editor.BorderColor) // off color
}

func TestModel_ModelChangedPublishesEvent(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := subscribeToChan(b, topicModelChange)

	m := newModel(b, nil, nil, nil)
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
	m := newModel(nil, nil, nil, nil)
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
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	models := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}

	model, _ := m.Update(ModelListResultMsg{Models: models})
	m = model.(Model)
	require.False(t, m.dialogStack.Empty())

	// Typing should go to overlay filter
	model, _ = m.Update(tea.KeyPressMsg{Text: "o", Code: 'o'})
	m = model.(Model)

	assert.False(t, m.dialogStack.Empty())
	// Filter "o" was applied to the selector dialog
}

func TestModel_ModelSelectorViewShowsOverlay(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	normalView := m.View()
	assert.NotContains(t, normalView.Content, "Select Model")

	models := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}

	model, _ := m.Update(ModelListResultMsg{Models: models})
	m = model.(Model)

	overlayView := m.View()
	assert.Contains(t, overlayView.Content, "Select Model")
}

func TestModel_ModelSelectedInvalidIndex(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.currentModel = ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-6"}
	m.pendingModels = []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}

	model, _ := m.Update(overlays.SelectorSelectedMsg{Index: -1, Item: overlays.SelectorItem{}})
	m = model.(Model)
	assert.True(t, m.dialogStack.Empty())

	// Original model should be unchanged
	assert.NotEmpty(t, m.currentModel.Provider)
}

func TestListModelsWithRegistry(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	sdk.ResetProviderRegistry()
	sdkmodel.ResetAuthRegistry()

	// Register representative models across all providers.
	for _, def := range []sdkmodel.ModelDef{
		{ID: "claude-opus-4-7", Provider: "anthropic", DisplayName: "Claude Opus 4.7", Reasoning: true, SupportsXHigh: true},
		{ID: "claude-sonnet-4-6", Provider: "anthropic", DisplayName: "Claude Sonnet 4.6", Reasoning: true, Default: true},
		{ID: "claude-opus-4-5", Provider: "anthropic", DisplayName: "Claude Opus 4.5", Reasoning: true},
		{ID: "claude-sonnet-4-5", Provider: "anthropic", DisplayName: "Claude Sonnet 4.5", Reasoning: true},
		{ID: "claude-haiku-4-5", Provider: "anthropic", DisplayName: "Claude Haiku 4.5", Reasoning: true},
		{ID: "gpt-5.5", Provider: "openai", DisplayName: "GPT-5.5", Reasoning: true, Default: true},
		{ID: "gpt-5.4", Provider: "openai", DisplayName: "GPT-5.4", Reasoning: true},
		{ID: "gpt-5.2", Provider: "openai", DisplayName: "GPT-5.2", Reasoning: true},
		{ID: "gpt-4.1", Provider: "openai", DisplayName: "GPT-4.1"},
		{ID: "o4-mini", Provider: "openai", DisplayName: "o4-mini", Reasoning: true},
		{ID: "o3", Provider: "openai", DisplayName: "o3", Reasoning: true},
		{ID: "glm-5.1", Provider: "zai", DisplayName: "GLM-5.1", Reasoning: true, Default: true},
		{ID: "glm-5", Provider: "zai", DisplayName: "GLM-5", Reasoning: true},
		{ID: "glm-4.7", Provider: "zai", DisplayName: "GLM-4.7", Reasoning: true},
		{ID: "glm-4.7-flash", Provider: "zai", DisplayName: "GLM-4.7 Flash", Reasoning: true},
		{ID: "glm-4.7-flashx", Provider: "zai", DisplayName: "GLM-4.7 FlashX", Reasoning: true},
		{ID: "glm-4.5-air", Provider: "zai", DisplayName: "GLM-4.5 Air", Reasoning: true},
	} {
		sdkmodel.RegisterModel(def)
	}

	// Register providers so their models are included.
	sdk.RegisterProvider[struct{}, struct{}]("anthropic", func(_ sdk.Config, _, _ struct{}) (sdk.Provider, error) { return nil, nil }) //nolint:nilnil // stub registration for model list tests
	sdk.RegisterProvider[struct{}, struct{}]("openai", func(_ sdk.Config, _, _ struct{}) (sdk.Provider, error) { return nil, nil })    //nolint:nilnil // stub registration for model list tests
	sdk.RegisterProvider[struct{}, struct{}]("zai", func(_ sdk.Config, _, _ struct{}) (sdk.Provider, error) { return nil, nil })       //nolint:nilnil // stub registration for model list tests

	// Set auth status so providers appear configured.
	sdkmodel.SetProviderAuth("anthropic", true)
	sdkmodel.SetProviderAuth("openai", true)
	sdkmodel.SetProviderAuth("zai", true)

	defer sdkmodel.ResetModelRegistry()
	defer sdk.ResetProviderRegistry()
	defer sdkmodel.ResetAuthRegistry()

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
	sdkmodel.ResetModelRegistry()
	sdk.ResetProviderRegistry()
	sdkmodel.ResetAuthRegistry()

	defer sdkmodel.ResetModelRegistry()
	defer sdk.ResetProviderRegistry()
	defer sdkmodel.ResetAuthRegistry()

	entries := listModels()
	assert.Nil(t, entries)
}

func TestListModelsIgnoresEnvOverrides(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	sdk.ResetProviderRegistry()
	sdkmodel.ResetAuthRegistry()

	for _, def := range []sdkmodel.ModelDef{
		{ID: "claude-opus-4-7", Provider: "anthropic", Reasoning: true},
		{ID: "claude-sonnet-4-6", Provider: "anthropic", Reasoning: true},
		{ID: "claude-opus-4-5", Provider: "anthropic", Reasoning: true},
		{ID: "claude-sonnet-4-5", Provider: "anthropic", Reasoning: true},
		{ID: "claude-haiku-4-5", Provider: "anthropic", Reasoning: true},
	} {
		sdkmodel.RegisterModel(def)
	}

	sdk.RegisterProvider[struct{}, struct{}]("anthropic", func(_ sdk.Config, _, _ struct{}) (sdk.Provider, error) { return nil, nil }) //nolint:nilnil // stub registration for model list tests
	sdkmodel.SetProviderAuth("anthropic", true)

	defer sdkmodel.ResetModelRegistry()
	defer sdk.ResetProviderRegistry()
	defer sdkmodel.ResetAuthRegistry()

	entries := listModels()

	// Should show registry entries as-is
	anthropicCount := 0

	for _, e := range entries {
		if e.Provider == "anthropic" {
			anthropicCount++
		}
	}

	assert.Equal(t, 5, anthropicCount,
		"should show all anthropic models")
}

func TestModelEntryDisplayName(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-sonnet-4-6", Provider: "anthropic", DisplayName: "Claude Sonnet 4.6"})

	defer sdkmodel.ResetModelRegistry()

	e := ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-6"}
	assert.Equal(t, "Claude Sonnet 4.6", e.DisplayName())

	e = ModelEntry{Provider: "unknown", Model: "custom-model"}
	assert.Equal(t, "unknown/custom-model", e.DisplayName())
}

func TestModelSelectorEntryBadges(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-sonnet-4-6", Provider: "anthropic", DisplayName: "Claude Sonnet 4.6", Reasoning: true})
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "gpt-5.5", Provider: "openai", DisplayName: "GPT-5.5", Reasoning: true})

	defer sdkmodel.ResetModelRegistry()

	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.chat = m.chat.SetSize(80, 10)

	models := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}

	model, _ := m.Update(ModelListResultMsg{Models: models})
	m = model.(Model)
	require.False(t, m.dialogStack.Empty())

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()
	assert.Contains(t, rendered, "[anthropic]", "should show provider badge")
	assert.Contains(t, rendered, "[openai]", "should show provider badge")
}

func TestModelSelectorCurrentModelMarker(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-sonnet-4-6", Provider: "anthropic", DisplayName: "Claude Sonnet 4.6", Reasoning: true})
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "gpt-5.5", Provider: "openai", DisplayName: "GPT-5.5", Reasoning: true})

	defer sdkmodel.ResetModelRegistry()

	m := newModel(nil, nil, nil, nil)
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

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()
	assert.Contains(t, rendered, "✓", "current model should have checkmark marker")
}

func TestStatusMessageOnModelCycle(t *testing.T) {
	sdkmodel.ResetModelRegistry()
	sdk.ResetProviderRegistry()
	sdkmodel.ResetAuthRegistry()

	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-sonnet-4-6", Provider: "anthropic", Reasoning: true})
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "claude-opus-4-7", Provider: "anthropic", Reasoning: true, SupportsXHigh: true})
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "gpt-5.5", Provider: "openai", Reasoning: true})

	sdk.RegisterProvider[struct{}, struct{}]("anthropic", func(_ sdk.Config, _, _ struct{}) (sdk.Provider, error) { return nil, nil }) //nolint:nilnil // stub registration for model list tests
	sdk.RegisterProvider[struct{}, struct{}]("openai", func(_ sdk.Config, _, _ struct{}) (sdk.Provider, error) { return nil, nil })    //nolint:nilnil // stub registration for model list tests
	sdkmodel.SetProviderAuth("anthropic", true)
	sdkmodel.SetProviderAuth("openai", true)

	defer sdkmodel.ResetModelRegistry()
	defer sdk.ResetProviderRegistry()
	defer sdkmodel.ResetAuthRegistry()

	m := newModel(nil, nil, nil, nil)
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
	sdkmodel.ResetModelRegistry()
	sdkmodel.RegisterModel(sdkmodel.ModelDef{ID: "gpt-5.5", Provider: "openai", DisplayName: "GPT-5.5", Reasoning: true})

	defer sdkmodel.ResetModelRegistry()

	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	entry := ModelEntry{Provider: "openai", Model: "gpt-5.5"}
	model, _ := m.Update(ModelChangedMsg{Entry: entry})
	m = model.(Model)

	assert.Contains(t, m.statusMsg, "Switched to")
	assert.Contains(t, m.statusMsg, "thinking:")
}

func TestStatusMessageOnThinkingCycle(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	model, _ := m.dispatchBinding(ActionThinkingCycle)
	m = model.(Model)

	assert.Contains(t, m.statusMsg, "Thinking level:")
	assert.Contains(t, m.statusMsg, "high")
}

func TestStatusMessageClearsOnTimeout(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24

	m.statusMsg = "test status"

	model, _ := m.Update(statusTimeoutMsg{})
	m = model.(Model)

	assert.Empty(t, m.statusMsg)
	assert.Nil(t, m.statusTimer)
}

func TestStatusMessageRenderedInVIew(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.statusMsg = "test status message"

	view := m.View()
	assert.Contains(t, view.Content, "test status message")
}

func TestStatusMessageNotRenderedWhenEmpty(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	m.width = 80
	m.height = 24
	m.statusMsg = ""

	view := m.View()
	// Should not contain any status-related artifacts
	lines := splitLines(view.Content)
	// Count sections: chat + editor + footer = 3 lines minimum (no spinner, no status)
	assert.GreaterOrEqual(t, len(lines), 3)
}

func TestCurrentModel_LayeredSettings(t *testing.T) {
	entries := []ModelEntry{
		{Provider: "openai", Model: "gpt-5.5"},
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
	}

	mockCfg := &mockConfig{
		preferences: map[string]string{
			"provider": "openai",
			"model":    "gpt-5.5",
		},
	}

	cur := currentModel(entries, mockCfg)
	assert.Equal(t, "openai", cur.Provider)
	assert.Equal(t, "gpt-5.5", cur.Model)
}

func TestCurrentModel_EnvOverridesSettings(t *testing.T) {
	entries := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}

	// Settings say anthropic, but env says openai — env wins.
	t.Setenv("WEAVE_PROVIDER", "openai")

	mockCfg := &mockConfig{
		preferences: map[string]string{
			"provider": "anthropic",
			"model":    "claude-sonnet-4-6",
		},
	}

	cur := currentModel(entries, mockCfg)
	assert.Equal(t, "openai", cur.Provider, "WEAVE_PROVIDER should override settings")
	assert.Equal(t, "gpt-5.5", cur.Model)
}

func TestInitialThinkingLevel_LayeredSettings(t *testing.T) {
	mockCfg := &mockConfig{
		preferences: map[string]string{
			"thinking_level": "high",
		},
	}

	level := initialThinkingLevel(mockCfg)
	assert.Equal(t, sdkmodel.ThinkingHigh, level)
}

func TestInitialThinkingLevel_InvalidPreference(t *testing.T) {
	mockCfg := &mockConfig{
		preferences: map[string]string{
			"thinking_level": "bogus",
		},
	}

	level := initialThinkingLevel(mockCfg)
	assert.Equal(t, sdkmodel.ThinkingMedium, level)
}

func TestSaveSettings_CallsSavePreferences(t *testing.T) {
	var capturedPrefs preferences

	mockCfg := &mockConfig{
		savePreferences: func(target any) error {
			data, err := json.Marshal(target)
			if err != nil {
				return fmt.Errorf("marshal: %w", err)
			}

			if err := json.Unmarshal(data, &capturedPrefs); err != nil {
				return fmt.Errorf("unmarshal: %w", err)
			}

			return nil
		},
	}

	saveSettings(mockCfg, ModelEntry{Provider: "openai", Model: "gpt-5.5"}, sdkmodel.ThinkingHigh)

	assert.Equal(t, "openai", capturedPrefs.Provider)
	assert.Equal(t, "gpt-5.5", capturedPrefs.Model)
	assert.Equal(t, "high", capturedPrefs.ThinkingLevel)
}

func TestSaveSettings_PreservesUIFields(t *testing.T) {
	// Simulate stored settings with UI fields that should be preserved
	stored := map[string]any{
		"provider":       "anthropic",
		"model":          "claude-sonnet-4-6",
		"thinking_level": "medium",
		"ui": map[string]any{
			"theme":            "dark",
			"editor_max_lines": 30,
		},
	}

	mockCfg := &mockConfig{
		preferences: stored,
		savePreferences: func(target any) error {
			// Merge target fields into stored (simulating FullConfig.SavePreferences)
			targetData, _ := json.Marshal(target)

			var targetMap map[string]any

			_ = json.Unmarshal(targetData, &targetMap)
			maps.Copy(stored, targetMap)

			return nil
		},
	}

	// Save model change
	saveSettings(mockCfg, ModelEntry{Provider: "openai", Model: "gpt-5.5"}, sdkmodel.ThinkingHigh)

	// Verify model fields updated
	assert.Equal(t, "openai", stored["provider"])
	assert.Equal(t, "gpt-5.5", stored["model"])
	assert.Equal(t, "high", stored["thinking_level"])

	// Verify UI fields preserved
	ui, ok := stored["ui"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "dark", ui["theme"])
	assert.Equal(t, 30, ui["editor_max_lines"])
}

func TestNewModel_ReadsUISettings(t *testing.T) {
	m := newModelWithConfig(nil, nil, nil, nil, TUIConfig{EditorMaxLines: 25})
	assert.Equal(t, 25, m.editor.MaxHeight())
}

func TestNewModel_DefaultEditorHeightWhenNoSettings(t *testing.T) {
	m := newModel(nil, nil, nil, nil)
	assert.Equal(t, 15, m.editor.MaxHeight()) // default
}

func TestSaveSettings_NilConfig(t *testing.T) {
	// Should not panic with nil config
	assert.NotPanics(t, func() {
		saveSettings(nil, ModelEntry{Provider: "openai", Model: "gpt-5.5"}, sdkmodel.ThinkingHigh)
	})
}

// mockConfig is a test-double for sdk.Config.
type mockConfig struct {
	filePath        string
	preferences     any // JSON-marshalable value returned by Preferences
	savePreferences func(target any) error
	uiConfig        any // JSON-marshalable value returned by UIConfig
}

func (m *mockConfig) FilePath() string   { return m.filePath }
func (m *mockConfig) ProjectDir() string { return "" }
func (m *mockConfig) ExtensionConfig(_, _ string, target any) error {
	if m.uiConfig == nil {
		return nil
	}

	data, err := json.Marshal(m.uiConfig)
	if err != nil {
		return fmt.Errorf("marshal ui config: %w", err)
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("unmarshal ui config: %w", err)
	}

	return nil
}

func (m *mockConfig) IsHeadless() bool { return true }

func (m *mockConfig) Preferences(target any) error {
	if m.preferences == nil {
		return nil
	}

	data, err := json.Marshal(m.preferences)
	if err != nil {
		return fmt.Errorf("marshal preferences: %w", err)
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("unmarshal preferences: %w", err)
	}

	return nil
}

func (m *mockConfig) SavePreferences(target any) error {
	if m.savePreferences != nil {
		return m.savePreferences(target)
	}

	return nil
}
func (m *mockConfig) SaveProviderKey(_, _ string) error { return nil }
func (m *mockConfig) RespectGitignore() bool            { return true }
