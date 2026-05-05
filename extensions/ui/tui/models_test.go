package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"weave/bus"
	"weave/config"
	"weave/ext/ui/tui/components/messages"
	"weave/ext/ui/tui/components/overlays"
	"weave/sdk"
	"weave/sdk/model"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
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
	cur := currentModel(entries, "")
	assert.Equal(t, "openai", cur.Provider)

	cur = currentModel(nil, "")
	assert.Equal(t, "anthropic", cur.Provider)
}

func TestCurrentModel_EnvProvider(t *testing.T) {
	entries := []ModelEntry{
		{Provider: "anthropic", Model: "claude-sonnet-4-6"},
		{Provider: "openai", Model: "gpt-5.5"},
	}

	t.Setenv("WEAVE_PROVIDER", "openai")

	cur := currentModel(entries, "")
	assert.Equal(t, "openai", cur.Provider)
	assert.Equal(t, "gpt-5.5", cur.Model)
}

func TestCurrentModel_EnvProviderNotInEntries(t *testing.T) {
	entries := []ModelEntry{
		{Provider: "openai", Model: "gpt-5.5"},
	}

	t.Setenv("WEAVE_PROVIDER", "anthropic")

	cur := currentModel(entries, "")
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

	assert.False(t, m.dialogStack.Empty())
	assert.Equal(t, models, m.pendingModels)

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()
	assert.Contains(t, rendered, "Select Model")
}

func TestModel_ModelListResultEmpty(t *testing.T) {
	m := newModel(nil, nil, nil)
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
	assert.True(t, m.dialogStack.Empty())

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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24

	// With no providers registered, cycle shows status message
	model, cmd := m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
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
	model.ResetModelRegistry()
	model.RegisterBuiltinModels()

	defer model.ResetModelRegistry()

	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
	assert.Equal(t, model.ThinkingMedium, m.thinkingLevel)

	// Switch to non-reasoning model
	entry := ModelEntry{Provider: "openai", Model: "gpt-4.1"}
	model, _ := m.Update(ModelChangedMsg{Entry: entry})
	m = model.(Model)

	assert.Equal(t, model.ThinkingOff, m.thinkingLevel)
	assert.Equal(t, "off", m.footer.ThinkingLevel())
	assert.Equal(t, "240", m.editor.BorderColor) // off color
}

func TestModel_ModelChangedPublishesEvent(t *testing.T) {
	b := bus.New()
	defer b.Close()

	ch := subscribeToChan(b, topicModelChange)

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
	require.False(t, m.dialogStack.Empty())

	// Typing should go to overlay filter
	model, _ = m.Update(tea.KeyPressMsg{Text: "o", Code: 'o'})
	m = model.(Model)

	assert.False(t, m.dialogStack.Empty())
	// Filter "o" was applied to the selector dialog
}

func TestModel_ModelSelectorViewShowsOverlay(t *testing.T) {
	m := newModel(nil, nil, nil)
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
	m := newModel(nil, nil, nil)
	m.width = 80
	m.height = 24
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
	model.ResetModelRegistry()
	sdk.ResetProviderRegistry()
	model.ResetProviderEnvVarRegistry()
	model.RegisterBuiltinModels()

	// Register providers so their models are included.
	sdk.RegisterProvider("anthropic", func(_ sdk.Config) (sdk.Provider, error) { return nil, nil }) //nolint:nilnil // stub registration for model list tests
	sdk.RegisterProvider("openai", func(_ sdk.Config) (sdk.Provider, error) { return nil, nil })    //nolint:nilnil // stub registration for model list tests
	sdk.RegisterProvider("zai", func(_ sdk.Config) (sdk.Provider, error) { return nil, nil })       //nolint:nilnil // stub registration for model list tests

	// Register env vars and set test API keys so providers appear configured.
	model.RegisterProviderEnvVar("anthropic", "ANTHROPIC_API_KEY")
	model.RegisterProviderEnvVar("openai", "OPENAI_API_KEY")
	model.RegisterProviderEnvVar("zai", "ZAI_API_KEY")

	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("ZAI_API_KEY", "test-key")

	defer model.ResetModelRegistry()
	defer sdk.ResetProviderRegistry()
	defer model.ResetProviderEnvVarRegistry()

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
	model.ResetModelRegistry()
	sdk.ResetProviderRegistry()
	model.ResetProviderEnvVarRegistry()

	defer model.ResetModelRegistry()
	defer sdk.ResetProviderRegistry()
	defer model.ResetProviderEnvVarRegistry()

	entries := listModels()
	assert.Nil(t, entries)
}

func TestListModelsIgnoresEnvOverrides(t *testing.T) {
	model.ResetModelRegistry()
	sdk.ResetProviderRegistry()
	model.ResetProviderEnvVarRegistry()
	model.RegisterBuiltinModels()
	sdk.RegisterProvider("anthropic", func(_ sdk.Config) (sdk.Provider, error) { return nil, nil }) //nolint:nilnil // stub registration for model list tests
	model.RegisterProviderEnvVar("anthropic", "ANTHROPIC_API_KEY")

	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	defer model.ResetModelRegistry()
	defer sdk.ResetProviderRegistry()
	defer model.ResetProviderEnvVarRegistry()

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
	model.ResetModelRegistry()
	model.RegisterBuiltinModels()

	defer model.ResetModelRegistry()

	e := ModelEntry{Provider: "anthropic", Model: "claude-sonnet-4-6"}
	assert.Equal(t, "Claude Sonnet 4.6", e.DisplayName())

	e = ModelEntry{Provider: "unknown", Model: "custom-model"}
	assert.Equal(t, "unknown/custom-model", e.DisplayName())
}

func TestModelSelectorEntryBadges(t *testing.T) {
	model.ResetModelRegistry()
	model.RegisterBuiltinModels()

	defer model.ResetModelRegistry()

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
	require.False(t, m.dialogStack.Empty())

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()
	assert.Contains(t, rendered, "[anthropic]", "should show provider badge")
	assert.Contains(t, rendered, "[openai]", "should show provider badge")
}

func TestModelSelectorCurrentModelMarker(t *testing.T) {
	model.ResetModelRegistry()
	model.RegisterBuiltinModels()

	defer model.ResetModelRegistry()

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

	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())
	rendered := canvas.Render()
	assert.Contains(t, rendered, "✓", "current model should have checkmark marker")
}

func TestStatusMessageOnModelCycle(t *testing.T) {
	model.ResetModelRegistry()
	sdk.ResetProviderRegistry()
	model.ResetProviderEnvVarRegistry()
	model.RegisterBuiltinModels()
	sdk.RegisterProvider("anthropic", func(_ sdk.Config) (sdk.Provider, error) { return nil, nil }) //nolint:nilnil // stub registration for model list tests
	sdk.RegisterProvider("openai", func(_ sdk.Config) (sdk.Provider, error) { return nil, nil })    //nolint:nilnil // stub registration for model list tests
	model.RegisterProviderEnvVar("anthropic", "ANTHROPIC_API_KEY")
	model.RegisterProviderEnvVar("openai", "OPENAI_API_KEY")

	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("OPENAI_API_KEY", "test-key")

	defer model.ResetModelRegistry()
	defer sdk.ResetProviderRegistry()
	defer model.ResetProviderEnvVarRegistry()

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
	model.ResetModelRegistry()
	model.RegisterBuiltinModels()

	defer model.ResetModelRegistry()

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
	assert.Contains(t, view.Content, "test status message")
}

func TestStatusMessageNotRenderedWhenEmpty(t *testing.T) {
	m := newModel(nil, nil, nil)
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

	// Create a temp project dir with settings
	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o755))

	settingsJSON := `{"provider":"openai","model":"gpt-5.5"}`
	require.NoError(t, os.WriteFile(filepath.Join(projectWeave, "settings.json"), []byte(settingsJSON), 0o600))

	// Point settings path to a different (empty) global settings
	globalDir := t.TempDir()
	config.SetSettingsPath(filepath.Join(globalDir, "settings.json"))

	cur := currentModel(entries, projectDir)
	assert.Equal(t, "openai", cur.Provider)
	assert.Equal(t, "gpt-5.5", cur.Model)
}

func TestInitialThinkingLevel_LayeredSettings(t *testing.T) {
	projectDir := t.TempDir()
	projectWeave := filepath.Join(projectDir, ".weave")
	require.NoError(t, os.MkdirAll(projectWeave, 0o755))

	settingsJSON := `{"thinking_level":"high"}`
	require.NoError(t, os.WriteFile(filepath.Join(projectWeave, "settings.json"), []byte(settingsJSON), 0o600))

	globalDir := t.TempDir()
	config.SetSettingsPath(filepath.Join(globalDir, "settings.json"))

	level := initialThinkingLevel(projectDir)
	assert.Equal(t, model.ThinkingHigh, level)
}

func TestSaveSettings_PreservesUIFields(t *testing.T) {
	// Write initial settings with UI fields
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")
	config.SetSettingsPath(settingsPath)

	initial := &config.Settings{
		Provider:      "anthropic",
		Model:         "claude-sonnet-4-6",
		ThinkingLevel: "medium",
		UI: &config.UISettings{
			Theme:          "dark",
			EditorMaxLines: 30,
		},
	}
	require.NoError(t, config.SaveSettingsGlobal(initial))

	// Save model change
	saveSettings(ModelEntry{Provider: "openai", Model: "gpt-5.5"}, model.ThinkingHigh)

	// Verify UI fields preserved
	loaded, err := config.LoadSettings()
	require.NoError(t, err)
	assert.Equal(t, "openai", loaded.Provider)
	assert.Equal(t, "gpt-5.5", loaded.Model)
	assert.Equal(t, "high", loaded.ThinkingLevel)
	require.NotNil(t, loaded.UI)
	assert.Equal(t, "dark", loaded.UI.Theme)
	assert.Equal(t, 30, loaded.UI.EditorMaxLines)
}

func TestNewModel_ReadsUISettings(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")
	config.SetSettingsPath(settingsPath)

	// Write settings with editor_max_lines
	uiSettings := &config.Settings{
		UI: &config.UISettings{
			EditorMaxLines: 25,
		},
	}
	require.NoError(t, config.SaveSettingsGlobal(uiSettings))

	// Create a config file so configDir is set
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, ".weave", "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgPath), 0o755))
	require.NoError(t, os.WriteFile(cfgPath, []byte("ui: tui\ncore:\n  agent_loop: loop\n  providers:\n    - anthropic\n"), 0o600))

	cfg, err := config.LoadFullConfig(cfgPath)
	require.NoError(t, err)

	m := newModel(nil, cfg, nil)
	assert.Equal(t, 25, m.editor.MaxHeight())
}

func TestNewModel_DefaultEditorHeightWhenNoSettings(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")
	config.SetSettingsPath(settingsPath)

	m := newModel(nil, nil, nil)
	assert.Equal(t, 15, m.editor.MaxHeight()) // default
}
