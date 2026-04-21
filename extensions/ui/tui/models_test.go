package tui

import (
	"testing"

	"weave/bus"
	"weave/ext/ui/tui/components/messages"
	"weave/ext/ui/tui/components/overlays"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelEntry_Display(t *testing.T) {
	e := ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-20250514"}
	assert.Equal(t, "anthropic/claude-sonnet-4-20250514", e.Display())
}

func TestCycleModel(t *testing.T) {
	entries := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-20250514"},
		{Provider: "openai", Model: "gpt-4o"},
		{Provider: "zai", Model: "glm-4"},
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
		{Provider: "anthropic", Model: "claude-sonnet-4-20250514"},
	}
	next := cycleModel(entries, entries[0])
	assert.Equal(t, "anthropic", next.Provider)
}

func TestCycleModel_Empty(t *testing.T) {
	cur := ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-20250514"}
	next := cycleModel(nil, cur)
	assert.Equal(t, cur, next)
}

func TestCurrentModel(t *testing.T) {
	entries := []ModelEntry{
		{Provider: "openai", Model: "gpt-4o"},
		{Provider: "zai", Model: "glm-4"},
	}
	cur := currentModel(entries)
	assert.Equal(t, "openai", cur.Provider)

	cur = currentModel(nil)
	assert.Equal(t, "anthropic", cur.Provider)
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
		{Provider: "anthropic", Model: "claude-sonnet-4-20250514"},
		{Provider: "openai", Model: "gpt-4o"},
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
		{Provider: "anthropic", Model: "claude-sonnet-4-20250514"},
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
		{Provider: "anthropic", Model: "claude-sonnet-4-20250514"},
		{Provider: "openai", Model: "gpt-4o"},
	}

	model, _ := m.Update(ModelListResultMsg{Models: models})
	m = model.(Model)
	require.Equal(t, overlayModel, m.activeOverlay)

	// Select the second model
	model, _ = m.Update(overlays.SelectorSelectedMsg{Index: 1, Item: overlays.SelectorItem{
		Title: "openai/gpt-4o", Subtitle: "openai",
	}})
	m = model.(Model)

	assert.Equal(t, overlayNone, m.activeOverlay)
	assert.Equal(t, "openai", m.currentModel.Provider)
	assert.Equal(t, "gpt-4o", m.currentModel.Model)
	assert.Equal(t, "gpt-4o", m.footer.ModelName())
	assert.Equal(t, "openai", m.footer.ProviderName())
}

func TestModel_ModelSelectorCancel(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	models := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-20250514"},
		{Provider: "openai", Model: "gpt-4o"},
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
		{Provider: "anthropic", Model: "claude-sonnet-4-20250514"},
		{Provider: "openai", Model: "gpt-4o"},
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
	m = model.(Model)

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

	// With no providers registered, cycle should be no-op
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = model.(Model)
	assert.Nil(t, cmd)
}

func TestModel_ModelChangedUpdatesFooter(t *testing.T) {
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	entry := ModelEntry{Provider: "openai", Model: "gpt-4o"}
	model, _ := m.Update(ModelChangedMsg{Entry: entry})
	m = model.(Model)

	assert.Equal(t, "gpt-4o", m.currentModel.Model)
	assert.Equal(t, "openai", m.currentModel.Provider)
	assert.Equal(t, "gpt-4o", m.footer.ModelName())
	assert.Equal(t, "openai", m.footer.ProviderName())
}

func TestModel_ModelChangedPublishesEvent(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := b.Subscribe(topicModelChange)

	m := newModel(b, nil, nil)
	m.width = 80
	m.height = 24

	entry := ModelEntry{Provider: "openai", Model: "gpt-4o"}
	model, cmd := m.Update(ModelChangedMsg{Entry: entry})
	m = model.(Model)

	require.NotNil(t, cmd)
	cmd()

	evt := <-ch
	assert.Equal(t, topicModelChange, evt.Topic)
	assert.Equal(t, "openai/gpt-4o", evt.Payload)
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
		{Provider: "anthropic", Model: "claude-sonnet-4-20250514"},
		{Provider: "openai", Model: "gpt-4o"},
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
		{Provider: "anthropic", Model: "claude-sonnet-4-20250514"},
		{Provider: "openai", Model: "gpt-4o"},
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
		{Provider: "anthropic", Model: "claude-sonnet-4-20250514"},
	}

	model, _ := m.Update(overlays.SelectorSelectedMsg{Index: -1, Item: overlays.SelectorItem{}})
	m = model.(Model)
	assert.Equal(t, overlayNone, m.activeOverlay)

	// Original model should be unchanged
	assert.NotEqual(t, "", m.currentModel.Provider)
}
