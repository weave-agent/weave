package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	sandbox "github.com/weave-agent/weave-sandbox"
	"github.com/weave-agent/weave-tui/components"
	"github.com/weave-agent/weave-tui/components/attachments"
	"github.com/weave-agent/weave-tui/components/messages"
	"github.com/weave-agent/weave-tui/components/overlays"
	"github.com/weave-agent/weave-tui/palette"
	"github.com/weave-agent/weave/sdk"
	sdkmodel "github.com/weave-agent/weave/sdk/model"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	uv "github.com/charmbracelet/ultraviolet"
)

const doublePressWindow = 500 * time.Millisecond

const statusMessageTimeout = 2 * time.Second

const dockedOverlayHeight = 12

// statusTimeoutMsg is sent when the transient status message should be cleared.
type statusTimeoutMsg struct {
	gen int
}

// doublePressTimeoutMsg is sent when the double-press window expires.
type doublePressTimeoutMsg struct {
	kind int // 0 = ctrl+c, 1 = escape
	gen  int // generation counter to ignore stale timers
}

// toolFlashExpireMsg triggers a re-render after a tool panel flash expires.
type toolFlashExpireMsg struct{}

// pulseTickMsg advances the editor border pulse animation.
type pulseTickMsg struct {
	gen int // generation counter to ignore stale timers
}

const (
	doublePressCtrlC  = 0
	doublePressEscape = 1
)

func isEnterKey(code rune) bool {
	return code == tea.KeyEnter || code == tea.KeyReturn || code == tea.KeyKpEnter
}

// Model is the root Bubble Tea model for the TUI.
type Model struct {
	width  int
	height int
	bus    sdk.Bus
	cfg    sdk.Config
	ps     sdk.PreferenceStore

	chat       components.ChatModel
	editor     components.EditorModel
	footer     components.FooterModel
	spinner    components.SpinnerModel
	prompted   bool
	toolPanels map[string]*messages.ToolPanel // track pending tool panels by ID
	// pendingToolCalls tracks unresolved tool calls in provider order so the
	// UI can distinguish the currently-running tool from queued calls.
	pendingToolCalls map[string]string
	pendingToolOrder []string
	commands         *CommandRegistry
	bindings         *BindingRegistry
	ui               *TUIImpl
	layout           LayoutEngine

	pendingSessions        []SessionEntry
	pendingModels          []ModelEntry
	pendingProviders       []ProviderEntry
	pendingLoginProviders  []LoginProviderEntry
	pendingLogoutProviders []LogoutProviderEntry
	currentModel           ModelEntry
	prevModel              ModelEntry
	prevThinkingLevel      sdkmodel.ThinkingLevel
	dialogStack            overlays.DialogStack
	attach                 attachments.Model

	providerTarget string
	popupChans     map[string]chan overlayResponse
	popupSeq       int

	sessionDir string

	// double-press tracking
	ctrlCPressed   bool
	escapePressed  bool
	doublePressGen int

	// thinking level state
	thinkingLevel sdkmodel.ThinkingLevel

	// noConfigured is true when no provider has an API key set.
	noConfigured bool

	// oauthCancel cancels an in-flight OAuth flow when the user force-dismisses
	// the login dialog. Nil when no OAuth flow is active.
	oauthCtx    context.Context
	oauthCancel context.CancelFunc

	// oauthGen increments on each new OAuth flow start. Used to reject stale
	// LoginFlowResultMsg from previously canceled flows.
	oauthGen int

	// startup hints banner
	showHints bool

	// landing screen shown before first prompt
	showLanding bool
	landing     LandingModel

	// dockedOverlay is true when a popup dialog is in keep-content (docked) mode.
	dockedOverlay bool

	// transient status message
	statusMsg   string
	statusTimer tea.Cmd
	statusGen   int
	statusNew   bool // true for first frame after status is set (entrance animation)

	// theme is the active color theme for rendering.
	theme *palette.Theme

	// agentState tracks current agent activity for accent color and pulse.
	agentState palette.State

	// pulseGen increments on each pulse timer start to ignore stale ticks.
	pulseGen int

	// panel system
	panelManager    *PanelManager
	panelTray       PanelTray
	focus           FocusTarget
	expandedPanelID string

	// custom components set via TUIExtAPI
	customFooter TUIComponent
	customHeader TUIComponent

	// mouse selection state
	mouseRegion   int       // 0=none, 1=chat, 2=editor
	lastClickTime time.Time // double-click detection
	lastClickX    int
	lastClickY    int
}

// newModel creates a new root model.
// If ui is non-nil, it is reused (production path) so that renderers registered
// via sdk.UI are visible to the model. If nil, a fresh TUIImpl is created (tests).
//

func newModel(bus sdk.Bus, cfg sdk.Config, ps sdk.PreferenceStore, ui *TUIImpl) Model {
	return newModelWithConfig(bus, cfg, ps, ui, TUIConfig{})
}

// newModelWithConfig creates a new root model with explicit TUI configuration.
func newModelWithConfig(bus sdk.Bus, cfg sdk.Config, ps sdk.PreferenceStore, ui *TUIImpl, tuiCfg TUIConfig) Model {
	var cfgPath string
	if cfg != nil {
		cfgPath = cfg.FilePath()
	}

	sdir := resolveSessionDir(cfgPath)

	commands := NewCommandRegistry(bus, sdir)
	commands.register("/model", "Select or change model", false, func(_ string) CommandResult {
		return CommandResult{Command: listModelsCmd()}
	})

	commands.register("/providers", "Manage provider API keys", false, func(_ string) CommandResult {
		return CommandResult{Command: listProvidersCmd()}
	})

	commands.register("/thinking", "Set thinking level (off/minimal/low/medium/high/xhigh)", false, func(args string) CommandResult {
		if args == "" {
			return CommandResult{Notify: "Usage: /thinking <off|minimal|low|medium|high|xhigh>"}
		}

		level, err := sdkmodel.ParseThinkingLevel(args)
		if err != nil {
			return CommandResult{Notify: err.Error()}
		}

		return CommandResult{Command: func() tea.Msg {
			return ThinkingLevelSetMsg{Level: level}
		}}
	})

	editor := components.NewEditorModel()

	if tuiCfg.EditorMaxLines > 0 {
		editor = editor.SetMaxHeight(tuiCfg.EditorMaxLines)
	}

	models := listModels()
	cur := currentModel(models, ps)

	bindings := NewBindingRegistry()

	if cfg != nil && cfg.FilePath() != "" {
		if kbPath := loadKeybindings(cfg.FilePath()); kbPath != "" {
			_ = bindings.LoadUserConfig(kbPath)
		}
	}

	if ui == nil {
		ui = NewTUIImpl(commands, bindings)
	} else {
		ui.SetRegistries(commands, bindings)
	}

	messages.GetThemeInfo = ui.Theme

	m := Model{
		width:            80,
		height:           24,
		bus:              bus,
		cfg:              cfg,
		ps:               ps,
		chat:             components.NewChatModel(),
		editor:           editor,
		footer:           components.NewFooterModel(),
		spinner:          components.NewSpinnerModel(palette.DefaultTheme()),
		toolPanels:       make(map[string]*messages.ToolPanel),
		pendingToolCalls: make(map[string]string),
		commands:         commands,
		bindings:         bindings,
		ui:               ui,
		layout:           NewLayoutEngine(),
		currentModel:     cur,
		sessionDir:       sdir,
		thinkingLevel:    initialThinkingLevel(ps),
		noConfigured:     len(models) == 0,
		showHints:        true,
		showLanding:      true,
		landing:          NewLandingModel(cur.Model, cur.Provider, listLoadedComponents()),
		dialogStack:      overlays.NewDialogStack(),
		popupChans:       make(map[string]chan overlayResponse),
		theme:            palette.DefaultTheme(),
		panelManager:     ui.panelManager,
		panelTray:        NewPanelTray(),
		focus:            FocusEditor,
	}
	m.footer = m.footer.SetModel(cur.Model, cur.Provider)
	m.footer = m.footer.SetReasoning(modelReasoning(cur))
	m.footer = m.footer.SetThinkingLevel(string(m.thinkingLevel))
	m.editor = m.editor.SetBorderColor(palette.ThinkingBorderColor(m.thinkingLevel))

	if m.noConfigured {
		m.statusMsg = "No providers configured. Use /providers to set an API key."
	}

	return m
}

// listLoadedComponents returns a deduplicated, sorted list of all registered
// component names across extension, tool, provider, UI extension, and TUI
// extension registries.
func listLoadedComponents() []string {
	seen := make(map[string]bool)

	var names []string

	add := func(list []string) {
		for _, name := range list {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}

	add(sdk.ListExtensions())
	add(sdk.ListTools())
	add(sdk.ListProviders())
	add(sdk.ListUIExtensions())
	add(ListTUIExtensions())

	slices.Sort(names)

	return names
}

// Init returns the initial command.
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd

	if m.bus != nil {
		cmds = append(cmds, PublishModelChange(m.bus, m.currentModel))
	}

	// Flush status updates buffered during UI extension wiring
	// (before the event loop was running).
	if m.ui != nil {
		for _, s := range m.ui.DrainStatuses() {
			cmds = append(cmds, func() tea.Msg {
				return extStatusMsg(s)
			})
		}
	}

	// Wire TUI extensions via a command so registration happens after
	// the event loop is running. This prevents deadlock when extensions
	// call Send-based APIs like SetFooter or ShowPanel during RegisterTUI.
	if m.ui != nil {
		cmds = append(cmds, func() tea.Msg {
			for _, ext := range GetTUIExtensions(m.cfg) {
				ext.RegisterTUI(m.ui)
			}

			return nil
		})
	}

	return tea.Batch(cmds...)
}

// Update handles messages.
//
//nolint:gocyclo // central message dispatch for the TUI
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Clear status entrance animation flag after first frame
	m.statusNew = false

	// Keep panel tray in sync with panel manager state
	m.syncPanelTray()

	// Login flow authorization URL must be handled even when the login dialog is open.
	if authURLMsg, ok := msg.(LoginAuthURLMsg); ok {
		if m.oauthCancel == nil || authURLMsg.Gen != m.oauthGen {
			return m, nil
		}

		if top := m.dialogStack.Peek(); top != nil && top.ID() == dialogLoginOAuth {
			if dlg, ok := top.(*overlays.LoginDialog); ok {
				dlg.SetAuthURL(authURLMsg.URL)
			}
		}

		return m, completeOAuthFlowCmd(m.oauthCtx, authURLMsg.Handle, authURLMsg.Provider, authURLMsg.Gen)
	}

	// Login flow completion must be handled even when the login dialog is open.
	if loginResult, ok := msg.(LoginFlowResultMsg); ok {
		if m.oauthCancel == nil || loginResult.Gen != m.oauthGen {
			// Flow was canceled by the user or the result is from a previous flow;
			// ignore stale result.
			return m, nil
		}

		m.oauthCancel = nil
		m.oauthCtx = nil

		if top := m.dialogStack.Peek(); top != nil && top.ID() == dialogLoginOAuth {
			m.dialogStack, _ = m.dialogStack.Pop()
		}

		return m.onLoginFlowResult(loginResult)
	}

	// Dialog stack gets priority when non-empty.
	if !m.dialogStack.Empty() {
		// Ctrl+C force-dismisses the top dialog.
		if km, ok := msg.(tea.KeyPressMsg); ok && km.String() == "ctrl+c" {
			var d overlays.Dialog

			m.dialogStack, d = m.dialogStack.Pop()

			return m.handleDialogForceCancel(d)
		}

		newStack, cmd, completed := m.dialogStack.Update(msg)
		m.dialogStack = newStack

		// Handle dialogs that completed during fall-through (at most one).
		if len(completed) > 0 {
			return m.handleDialogDone(completed[0], cmd)
		}

		// Check if the top dialog completed.
		if top := m.dialogStack.Peek(); top != nil && top.Done() {
			var d overlays.Dialog

			m.dialogStack, d = m.dialogStack.Pop()

			return m.handleDialogDone(d, cmd)
		}

		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.chat = m.chat.SetSize(m.width, m.chatHeight(m.height))
		m.editor = m.editor.SetSize(m.width, m.editor.Height())
		m.footer = m.footer.SetSize(m.width)
		m.spinner = m.spinner.SetSize(m.width)
		m.dialogStack = m.dialogStack.Resize(m.width, m.height)
		m.panelTray = m.panelTray.SetSize(m.width)

		if m.ui != nil {
			m.ui.SetSize(m.width, m.height)
		}

		return m, nil

	case tea.KeyPressMsg:
		// Clear any active text selection on key press, unless the key
		// triggers the copy action (which needs to read the selection first).
		if action, ok := m.bindings.Resolve(keyString(msg)); !ok || action != ActionCopySelection {
			m.chat = m.chat.ClearSelection()
			m.editor = m.editor.ClearSelection()
		}

		// Dismiss startup hints on first keypress
		m.showHints = false

		// Call registered raw input handlers asynchronously to avoid
		// deadlocking the Bubble Tea event loop when handlers call TUI
		// APIs that send messages back into the loop (e.g. EditorText).
		if m.ui != nil {
			m.ui.mu.Lock()
			handlers := make([]func(KeyEvent), len(m.ui.inputHandlers))
			copy(handlers, m.ui.inputHandlers)
			m.ui.mu.Unlock()

			ev := KeyEvent{Code: msg.Code, Mod: int(msg.Mod), String: msg.String()}
			go func(handlers []func(KeyEvent), event KeyEvent) {
				for _, handler := range handlers {
					func(h func(KeyEvent)) {
						defer func() {
							if r := recover(); r != nil {
								slog.Error("panic in raw input handler", "recover", r)
							}
						}()

						h(event)
					}(handler)
				}
			}(handlers, ev)
		}

		// Attachment delete mode: intercept navigation keys
		if m.attach.InDeleteMode() {
			switch msg.String() {
			case "esc", "ctrl+c":
				m.attach = m.attach.ToggleDeleteMode()
				return m, nil
			case "up", "left":
				m.attach = m.attach.DeleteModePrev()
				return m, nil
			case "down", "right":
				m.attach = m.attach.DeleteModeNext()
				return m, nil
			case "enter":
				m.attach = m.attach.Remove(m.attach.DeleteIdx())

				if len(m.attach.Items()) == 0 {
					m.attach = m.attach.ToggleDeleteMode()
				}

				return m, nil
			}
			// Fall through to binding resolver (ctrl+r handled there)
		}

		// Handle ctrl+c with double-press: first clears editor, second quits
		if msg.String() == "ctrl+c" {
			return m.handleCtrlC()
		}

		// Handle escape with double-press: first interrupts, second clears editor
		if msg.Code == tea.KeyEsc {
			if m.expandedPanelID != "" {
				m.expandedPanelID = ""
				m.focus = FocusTray
				m.panelTray = m.panelTray.SetFocused(true)

				return m, nil
			}

			// If focus is not on editor, return focus to editor first
			if m.focus != FocusEditor {
				m.focus = FocusEditor
				m.panelTray = m.panelTray.SetFocused(false)

				return m, nil
			}

			// If completion is active, dismiss it instead
			if m.editor.CompletionActive() {
				m.editor = m.editor.HideCompletion()

				return m, nil
			}

			return m.handleEscape()
		}

		// Panel focus chain: Tab cycles editor → tray → panel (before keybinding resolver
		// so it takes priority over shift+tab thinking cycle when panels are visible)
		if msg.Code == tea.KeyTab && m.panelTray.Len() > 0 && !m.editor.CompletionActive() {
			return m.cycleFocus(msg.Mod == tea.ModShift)
		}

		// Blocking overlay panels intercept input before keybindings
		if m.focus == FocusPanel && m.panelManager.Active() != "" {
			if entry, ok := m.panelManager.Get(m.panelManager.Active()); ok && entry.Config.Blocking {
				if cmd, handled := m.panelManager.UpdateDrawer(m.panelManager.Active(), msg); handled {
					return m, cmd
				}
			}
		}

		// Tray key navigation when focused
		//nolint:nestif // Tray focus handles three key paths with shared model mutation.
		if m.focus == FocusTray {
			if msg.Code == tea.KeyRight {
				m.panelTray = m.panelTray.Next()
				if activeID := m.panelTray.ActiveID(); activeID != "" {
					m.panelManager.Show(activeID)
					m.syncPanelTray()
					m.syncChatViewport()
				}

				return m, nil
			}

			if msg.Code == tea.KeyLeft {
				m.panelTray = m.panelTray.Prev()
				if activeID := m.panelTray.ActiveID(); activeID != "" {
					m.panelManager.Show(activeID)
					m.syncPanelTray()
					m.syncChatViewport()
				}

				return m, nil
			}

			if isEnterKey(msg.Code) {
				activeID := m.panelTray.ActiveID()
				if activeID == "" {
					visible := m.panelManager.VisiblePanels()
					if len(visible) > 0 {
						activeID = visible[0]
					}
				}

				if activeID == "" {
					m.focus = FocusEditor
					m.panelTray = m.panelTray.SetFocused(false)
					m.expandedPanelID = ""

					return m, nil
				}

				m.panelManager.Show(activeID)
				m.syncPanelTray()
				m.syncChatViewport()
				m.activateSelectedPanel(activeID, true)

				return m, nil
			}
		}

		// Try keybinding resolver
		if action, ok := m.bindings.Resolve(keyString(msg)); ok {
			return m.dispatchBinding(action)
		}

		// Forward keys to active panel when focused
		if m.focus == FocusPanel && m.panelManager.Active() != "" {
			if cmd, ok := m.panelManager.UpdateDrawer(m.panelManager.Active(), msg); ok {
				return m, cmd
			}
		}

		// Completion key interception
		if handled, model, cmd := m.handleCompletionKey(msg); handled {
			return model, cmd
		}

		// Fall through to editor
		oldValue := m.editor.Value()
		oldLine := m.editor.CursorLine()
		oldCol := m.editor.CursorColumn()

		var cmd tea.Cmd

		m.editor, cmd = m.editor.Update(msg)

		// Refresh completion state when editor content or cursor position changed
		if m.editor.Value() != oldValue || m.editor.CursorLine() != oldLine || m.editor.CursorColumn() != oldCol {
			m = m.refreshEditorCompletion()
		}

		return m, cmd

	case tea.PasteMsg:
		// Clear active selections before paste changes editor content
		m.chat = m.chat.ClearSelection()
		m.editor = m.editor.ClearSelection()

		// Paste detection: auto-convert large pastes to file attachments
		if attachments.IsPastedContent(msg.Content) {
			m.attach = m.attach.AddPaste(msg.Content)
			m.showStatus(fmt.Sprintf("Pasted content added as attachment (%d lines)", m.attach.Items()[len(m.attach.Items())-1].Lines))

			return m, m.statusTimer
		}

		// Short paste: forward to editor
		var cmd tea.Cmd

		m.editor, cmd = m.editor.Update(msg)
		m = m.refreshEditorCompletion()

		return m, cmd

	case externalEditorMsg:
		if msg.err != nil {
			m.statusMsg = "Editor error: " + msg.err.Error()
			m.statusGen++
			m.syncChatViewport()
			gen := m.statusGen
			m.statusTimer = tea.Tick(statusMessageTimeout, func(_ time.Time) tea.Msg {
				return statusTimeoutMsg{gen: gen}
			})

			return m, m.statusTimer
		}

		if msg.text != "" {
			m.editor = m.editor.SetValue(msg.text)
		}

		return m, nil

	case components.SubmitMsg:
		return m.onSubmit(msg.Text)

	case components.SpinnerShowMsg:
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.SpinnerUpdate(msg)
		m.syncChatViewport()

		return m, cmd

	case components.SpinnerHideMsg:
		m.spinner, _ = m.spinner.SpinnerUpdate(msg)
		m.syncChatViewport()

		return m, nil

	case TurnStartMsg:
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.SpinnerUpdate(components.SpinnerShowMsg{})
		m.syncChatViewport()

		return m, cmd

	case MessageStartMsg:
		m.chat = m.chat.AddItem(messages.NewAssistantMessage())
		m.chat = m.chat.ClearSelection()

		// Keep render loop active so progressive renders show through
		if m.spinner.Visible() {
			return m, components.StartSpinner()
		}

		return m, nil

	case MessageUpdateMsg:
		m.onMessageUpdate(msg)

		return m, nil

	case MessageEndMsg:
		m.onMessageEnd(msg)

		return m, nil

	case ToolResultMsg:
		m.onToolResult(msg)

		// Schedule a tick to ensure the post-flash settled border is rendered
		// even if the TUI goes idle before the 800ms flash expires.
		return m, tea.Tick(800*time.Millisecond, func(time.Time) tea.Msg {
			return toolFlashExpireMsg{}
		})

	case toolFlashExpireMsg:
		return m, nil

	case TurnEndMsg:
		m.spinner = m.spinner.Hide()
		m.syncChatViewport()

		if !m.chat.AtBottom() {
			m.chat = m.chat.SetTurnEndPending(true)
		}

		return m, nil

	case AgentEndMsg:
		m.spinner = m.spinner.Hide()
		m.syncChatViewport()
		m.footer = m.footer.SetTokenRate(0)
		m.pendingToolCalls = make(map[string]string)
		m.pendingToolOrder = nil

		if msg.Payload != nil {
			if errStr, ok := msg.Payload.(string); ok && errStr != "" {
				am := messages.NewAssistantMessage()
				am.Finalize("[error] " + errStr)
				m.chat = m.chat.AddItem(am)
			}
		}

		return m, nil

	case TokenUsageMsg:
		m.footer = m.footer.SetTokenUsage(msg.InputTokens, msg.OutputTokens, 0).
			SetCacheTokens(msg.CacheCreationTokens, msg.CacheReadTokens)

		return m, nil

	case CompactedMsg:
		m.showLanding = false
		if msg.Error != "" {
			m.chat = m.chat.AddItem(messages.NewNotificationMessage(
				"Compaction failed: "+msg.Error, sdk.NotifyError))
		} else if msg.Summarized > 0 {
			m.chat = m.chat.AddItem(messages.NewNotificationMessage(
				fmt.Sprintf("Context compacted: %d messages summarized", msg.Summarized),
				sdk.NotifyInfo))
			m.chat = m.chat.AddItem(messages.NewCompactionEntry(
				msg.Summarized, msg.TokensBefore, msg.TokensAfter))
		}

		return m, nil

	case SessionResumedMsg:
		if msg.SessionID != "" {
			m.rebuildChatFromMessages(msg.Messages)
			m.showLanding = false
			m.prompted = true
		}

		return m, nil

	case SessionListResultMsg:
		return m.onSessionListResult(msg)

	case ModelListResultMsg:
		return m.onModelListResult(msg)

	case ModelChangedMsg:
		return m.onModelChanged(msg)

	case ModelChangeFailedMsg:
		return m.onModelChangeFailed(msg)

	case ProviderListResultMsg:
		return m.onProviderListResult(msg)

	case LoginListResultMsg:
		return m.onLoginListResult(msg)

	case LogoutListResultMsg:
		return m.onLogoutListResult(msg)

	case ShutdownMsg:
		return m, tea.Quit

	case reloadMsg:
		if err := handleReload(msg); err != nil {
			return m, func() tea.Msg { return notifyMsg{message: "/reload failed: " + err.Error()} }
		}

		return m, tea.Quit

	case popupPendingMsg:
		return m.handlePopupPending()

	case extStatusMsg:
		m.footer = m.footer.SetExtStatus(msg.key, msg.text)
		return m, nil

	case notifyMsg:
		m.showLanding = false
		m.chat = m.chat.AddItem(newNotifyAssistantMsg(msg.message))

		return m, nil

	case notifyTypedMsg:
		m.showLanding = false
		m.chat = m.chat.AddItem(messages.NewNotificationMessage(msg.message, msg.level))

		return m, nil

	case statusTimeoutMsg:
		if msg.gen == m.statusGen {
			m.statusMsg = ""
			m.statusTimer = nil
			m.syncChatViewport()
		}

		return m, nil

	case doublePressTimeoutMsg:
		if msg.gen == m.doublePressGen {
			switch msg.kind {
			case doublePressCtrlC:
				m.ctrlCPressed = false
			case doublePressEscape:
				m.escapePressed = false
			}
		}

		return m, nil

	case ThinkingLevelSetMsg:
		return m.applyThinkingLevel(msg.Level)

	case AgentStateChangeMsg:
		m.agentState = msg.State
		accent, accentDim, accentBright := palette.AccentForState(msg.State)
		newTheme := *m.theme
		newTheme.Accent = accent
		newTheme.AccentDim = accentDim
		newTheme.AccentBright = accentBright
		m.theme = &newTheme

		// When idle, border uses thinking-level grayscale color.
		borderColor := accent
		if msg.State == palette.StateIdle {
			borderColor = palette.ThinkingBorderColor(m.thinkingLevel)
		}

		m.editor = m.editor.SetBorderColor(borderColor)
		m.editor = m.editor.SetPulseColors(accent, accentBright)
		m.spinner = m.spinner.SetTheme(m.theme)

		// Enable/disable pulse animation based on active state
		active := msg.State == palette.StateStreaming || msg.State == palette.StateToolRunning
		m.editor = m.editor.SetPulseActive(active)

		if active {
			m.pulseGen++
			gen := m.pulseGen

			return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
				return pulseTickMsg{gen: gen}
			})
		}

		return m, nil

	case pulseTickMsg:
		if msg.gen != m.pulseGen {
			return m, nil // stale tick
		}

		if m.editor.PulseActive {
			m.editor = m.editor.SetPulsePos(m.editor.PulsePos + 1)

			return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
				return pulseTickMsg{gen: m.pulseGen}
			})
		}

		return m, nil

	case OutdatedNotificationMsg:
		return m.onOutdatedExtensions(msg)

	case slashCommandsUpdatedMsg:
		if m.editor.CompletionActive() {
			m = m.refreshEditorCompletion()
		}

		return m, nil

	case themeChangedMsg:
		if msg.theme != nil {
			m.theme = msg.theme
		}

		return m, nil

	case panelChangedMsg:
		m.syncPanelTray()
		m.clearInvalidExpandedPanel()
		m.syncChatViewport()

		if m.panelTray.Len() == 0 && (m.focus == FocusTray || m.focus == FocusPanel) {
			m.focus = FocusEditor
			m.panelTray = m.panelTray.SetFocused(false)
		} else if m.focus == FocusPanel && m.panelManager.Active() == "" {
			m.focus = FocusEditor
			m.panelTray = m.panelTray.SetFocused(false)
		}

		return m, nil

	case setEditorTextMsg:
		m.editor = m.editor.SetValue(msg.text)

		return m, nil

	case pasteToEditorMsg:
		m.chat = m.chat.ClearSelection()
		m.editor = m.editor.ClearSelection()

		var cmd tea.Cmd

		m.editor, cmd = m.editor.Update(tea.PasteMsg{Content: msg.text})
		m = m.refreshEditorCompletion()

		return m, cmd

	case editorTextRequestMsg:
		msg.response <- m.editor.Value()

		return m, nil

	case setFooterMsg:
		m.customFooter = msg.component

		return m, nil

	case setHeaderMsg:
		m.customHeader = msg.component

		return m, nil

	case setWorkingFramesMsg:
		m.spinner = m.spinner.SetCustomFrames(msg.frames, msg.interval)

		return m, nil

	case tea.MouseWheelMsg:
		switch msg.Button {
		case uv.MouseWheelUp:
			m.chat = m.chat.ScrollUp(1)
		case uv.MouseWheelDown:
			m.chat = m.chat.ScrollDown(1)
		}

		return m, nil

	case tea.MouseClickMsg:
		mouse := msg.Mouse()

		if mouse.Button != tea.MouseLeft || m.showLanding || !m.dialogStack.Empty() {
			return m, nil
		}

		now := time.Now()
		isDoubleClick := !m.lastClickTime.IsZero() &&
			now.Sub(m.lastClickTime) < doublePressWindow &&
			m.lastClickX == mouse.X && m.lastClickY == mouse.Y
		m.lastClickTime = now
		m.lastClickX = mouse.X
		m.lastClickY = mouse.Y

		// Clear existing selections from both regions
		if m.chat.HasSelection() {
			m.chat = m.chat.ClearSelection()
		}

		if m.editor.HasSelection() {
			m.editor = m.editor.ClearSelection()
		}

		chatRect := m.chatArea()
		editorRect := m.editorArea()

		if pointInArea(mouse.X, mouse.Y, chatRect) {
			return m.handleChatClick(chatRect, mouse, isDoubleClick)
		}

		if pointInArea(mouse.X, mouse.Y, editorRect) {
			return m.handleEditorClick(editorRect, mouse, isDoubleClick)
		}

		return m, nil

	case tea.MouseMotionMsg:
		mouse := msg.Mouse()

		if mouse.Button != tea.MouseLeft || m.showLanding || !m.dialogStack.Empty() {
			return m, nil
		}

		switch m.mouseRegion {
		case 1:
			if !m.chat.MouseDown() {
				return m, nil
			}

			area := m.chatArea()
			if !pointInArea(mouse.X, mouse.Y, area) {
				return m, nil
			}

			line, col := m.chatContentPos(mouse.X, mouse.Y, area)
			m.chat = m.chat.ExtendSelection(line, col)

		case 2:
			if !m.editor.MouseDown() {
				return m, nil
			}

			area := m.editorArea()
			if !pointInArea(mouse.X, mouse.Y, area) {
				return m, nil
			}

			line, col := m.editorContentPos(mouse.X, mouse.Y, area)
			m.editor = m.editor.ExtendSelection(line, col)
		}

		return m, nil

	case tea.MouseReleaseMsg:
		if !m.dialogStack.Empty() {
			return m, nil
		}

		switch m.mouseRegion {
		case 1:
			if !m.chat.MouseDown() {
				return m, nil
			}

			m.chat = m.chat.EndSelection()
			m.mouseRegion = 0

			if m.chat.HasSelection() {
				text := m.chat.ExtractSelection()
				if text != "" {
					return m, copySelectionCmd(text)
				}
			}

		case 2:
			if !m.editor.MouseDown() {
				return m, nil
			}

			m.editor = m.editor.EndSelection()
			m.mouseRegion = 0

			if m.editor.HasSelection() {
				text := m.editor.ExtractSelection()
				if text != "" {
					return m, copySelectionCmd(text)
				}
			}
		}

		return m, nil
	}

	// Forward spinner ticks to advance animation.
	if _, ok := msg.(spinner.TickMsg); ok && m.spinner.Visible() {
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.Update(msg)

		return m, cmd
	}

	return m, nil
}

// chatArea computes the screen rectangle for the chat viewport.
func (m Model) chatArea() uv.Rectangle {
	headerRows, pillRows := m.countLayoutRows()
	editorH := m.editor.Height() + m.attach.Height()
	trayRows, abovePanelRows, belowPanelRows := m.panelRows()
	lt := m.layout.ComputeWithPanels(
		m.width, m.height,
		editorH, headerRows, pillRows, m.dockedRows(),
		trayRows, abovePanelRows, belowPanelRows,
	)

	return lt.Main
}

// chatContentPos maps screen coordinates within the chat area to global
// content line and column indices.
func (m Model) chatContentPos(x, y int, area uv.Rectangle) (line, col int) {
	line = m.chat.ScrollOffset() + (y - area.Min.Y)
	col = x - area.Min.X

	return line, col
}

// pointInArea returns true if the given screen coordinates fall within area.
func pointInArea(x, y int, area uv.Rectangle) bool {
	return x >= area.Min.X && x < area.Max.X && y >= area.Min.Y && y < area.Max.Y
}

// editorArea computes the screen rectangle for the editor text area,
// accounting for attachments that push the editor down within its layout region.
func (m Model) editorArea() uv.Rectangle {
	headerRows, pillRows := m.countLayoutRows()
	editorH := m.editor.Height() + m.attach.Height()
	trayRows, abovePanelRows, belowPanelRows := m.panelRows()
	lt := m.layout.ComputeWithPanels(
		m.width, m.height,
		editorH, headerRows, pillRows, m.dockedRows(),
		trayRows, abovePanelRows, belowPanelRows,
	)

	attachH := m.attach.Height()
	if attachH > 0 && lt.Editor.Dy() > attachH {
		return uv.Rect(
			lt.Editor.Min.X,
			lt.Editor.Min.Y+attachH,
			lt.Editor.Dx(),
			lt.Editor.Dy()-attachH,
		)
	}

	return lt.Editor
}

// editorContentPos maps screen coordinates within the editor area to a
// logical line and rune column in the editor content.
func (m Model) editorContentPos(x, y int, editorRect uv.Rectangle) (line, col int) {
	visualRow := y - (editorRect.Min.Y + 1) // top border
	localCol := x - (editorRect.Min.X + 2)  // border(1) + padding(1)

	if visualRow < 0 {
		visualRow = 0
	}

	if localCol < 0 {
		localCol = 0
	}

	scrollOffset := m.editor.ScrollYOffset()
	globalVLine := scrollOffset + visualRow

	line, rowOffset := m.editor.VisualLineToLogical(globalVLine)
	col = m.editor.ColFromWrapped(line, rowOffset, localCol)

	return line, col
}

// handleChatClick processes a left-click in the chat area. Returns the model
// and an optional copy command if a double-click selected a word.
//
//nolint:dupl // similar structure but different component types
func (m Model) handleChatClick(chatRect uv.Rectangle, mouse tea.Mouse, isDoubleClick bool) (Model, tea.Cmd) {
	line, col := m.chatContentPos(mouse.X, mouse.Y, chatRect)

	if isDoubleClick {
		m.chat = m.chat.SelectWord(line, col)

		if m.chat.HasSelection() {
			if text := m.chat.ExtractSelection(); text != "" {
				return m, copySelectionCmd(text)
			}
		}

		return m, nil
	}

	m.mouseRegion = 1
	m.chat = m.chat.StartSelection(line, col)

	return m, nil
}

// handleEditorClick processes a left-click in the editor area. Returns the
// model and an optional copy command if a double-click selected a word.
//
//nolint:dupl // similar structure but different component types
func (m Model) handleEditorClick(editorRect uv.Rectangle, mouse tea.Mouse, isDoubleClick bool) (Model, tea.Cmd) {
	line, col := m.editorContentPos(mouse.X, mouse.Y, editorRect)

	if isDoubleClick {
		m.editor = m.editor.SelectWord(line, col)

		if m.editor.HasSelection() {
			if text := m.editor.ExtractSelection(); text != "" {
				return m, copySelectionCmd(text)
			}
		}

		return m, nil
	}

	m.mouseRegion = 2
	m.editor = m.editor.StartSelection(line, col)

	return m, nil
}

// handleCtrlC implements double-press ctrl+c: first press clears editor,
// second press within the window quits.
func (m Model) handleCtrlC() (tea.Model, tea.Cmd) {
	if m.editor.Value() != "" {
		m.editor = m.editor.SetValue("")
		m.doublePressGen++
		m.ctrlCPressed = true

		return m, tea.Tick(doublePressWindow, func(_ time.Time) tea.Msg {
			return doublePressTimeoutMsg{kind: doublePressCtrlC, gen: m.doublePressGen}
		})
	}

	// Editor is empty — check for double press
	if m.ctrlCPressed {
		m.ctrlCPressed = false
		return m, tea.Quit
	}

	m.doublePressGen++
	m.ctrlCPressed = true

	return m, tea.Tick(doublePressWindow, func(_ time.Time) tea.Msg {
		return doublePressTimeoutMsg{kind: doublePressCtrlC, gen: m.doublePressGen}
	})
}

// handleEscape implements double-press escape: first press interrupts streaming,
// second press within the window clears the editor.
func (m Model) handleEscape() (tea.Model, tea.Cmd) {
	// Check for double press — clear editor
	if m.escapePressed {
		m.editor = m.editor.SetValue("")
		m.escapePressed = false

		return m, nil
	}

	m.doublePressGen++
	m.escapePressed = true

	var cmd tea.Cmd

	activeTool := m.activeToolName()

	if activeTool == "await_agent" || strings.HasPrefix(activeTool, "subagent_") {
		if m.bus != nil {
			cmd = PublishInterrupt(m.bus)
		}
	} else {
		// First press — interrupt streaming if active, start timeout.
		var model tea.Model

		model, cmd = m.interruptStreaming()
		m = model.(Model)
	}

	return m, tea.Batch(cmd, tea.Tick(doublePressWindow, func(_ time.Time) tea.Msg {
		return doublePressTimeoutMsg{kind: doublePressEscape, gen: m.doublePressGen}
	}))
}

// dispatchBinding handles a resolved keybinding action.
//
//nolint:gocyclo // central keybinding dispatch
func (m Model) dispatchBinding(action BindingAction) (tea.Model, tea.Cmd) {
	switch action {
	case ActionExit:
		return m, tea.Quit
	case ActionModelSelect:
		return m, listModelsCmd()
	case ActionModelCycle:
		models := listModels()
		if len(models) <= 1 {
			m.showStatus("Only one model available")
			return m, m.statusTimer
		}

		next := cycleModel(models, m.currentModel)

		return m, func() tea.Msg { return ModelChangedMsg{Entry: next} }

	// Editor navigation
	case ActionCursorLineStart:
		m.editor = m.editor.CursorLineStart()
		return m, nil
	case ActionCursorLineEnd:
		m.editor = m.editor.CursorLineEnd()
		return m, nil
	case ActionCursorWordLeft:
		m.editor = m.editor.CursorWordLeft()
		return m, nil
	case ActionCursorWordRight:
		m.editor = m.editor.CursorWordRight()
		return m, nil
	case ActionEditorNewline:
		var cmd tea.Cmd

		m.editor, cmd = m.editor.InsertNewline()

		return m, cmd

	// Chat scroll
	case ActionScrollUp:
		m.chat = m.chat.ScrollUp(m.chatHeight(m.height))
		return m, nil
	case ActionScrollDown:
		m.chat = m.chat.ScrollDown(m.chatHeight(m.height))
		return m, nil
	case ActionScrollToBottom:
		m.chat = m.chat.JumpToBottom()
		return m, nil

	// Editor deletion
	case ActionDeleteWordBackward:
		m.editor = m.editor.DeleteWordBackward()
		return m, nil
	case ActionDeleteWordForward:
		m.editor = m.editor.DeleteWordForward()
		return m, nil
	case ActionDeleteToLineStart:
		m.editor = m.editor.DeleteToLineStart()
		return m, nil
	case ActionDeleteToLineEnd:
		m.editor = m.editor.DeleteToLineEnd()
		return m, nil

	// App control
	case ActionSuspend:
		return m, func() tea.Msg { return tea.SuspendMsg{} }
	case ActionExternalEditor:
		return m.openExternalEditor()

	// Display
	case ActionToggleToolOutput:
		m.toggleLastToolOutput()
		return m, nil
	case ActionThinkingCycle:
		return m.cycleThinkingLevel()

	// Session
	case ActionNewSession:
		m.chat = components.NewChatModel().SetSize(m.width, m.chatHeight(m.height))
		m.toolPanels = make(map[string]*messages.ToolPanel)
		m.pendingToolCalls = make(map[string]string)
		m.pendingToolOrder = nil
		m.prompted = false
		m.showLanding = true
		m.attach = m.attach.Clear()
		m.dockedOverlay = false
		m.dialogStack = overlays.NewDialogStack()

		// Clean up any pending popup channels to prevent goroutine leaks.
		for id, ch := range m.popupChans {
			select {
			case ch <- overlayResponse{err: errors.New("session reset")}:
			default:
			}

			delete(m.popupChans, id)
		}

		return m, nil

	// Attachments
	case ActionAttachDelete:
		if m.attach.InDeleteMode() {
			// ctrl+r while in delete mode: delete highlighted attachment
			m.attach = m.attach.Remove(m.attach.DeleteIdx())
		} else {
			m.attach = m.attach.ToggleDeleteMode()
		}

		return m, nil

	// Panels
	case ActionPanelPicker:
		if m.panelTray.Len() > 0 {
			m.focus = FocusTray
			m.panelTray = m.panelTray.SetFocused(true)
		} else {
			m.showStatus("No panels visible")
		}

		return m, m.statusTimer

	// Sandbox
	case ActionSandboxCycle:
		return m.cycleSandboxMode()

	// Copy selection
	case ActionCopySelection:
		if m.chat.HasSelection() {
			text := m.chat.ExtractSelection()
			if text != "" {
				return m, copySelectionCmd(text)
			}
		}

		if m.editor.HasSelection() {
			text := m.editor.ExtractSelection()
			if text != "" {
				return m, copySelectionCmd(text)
			}
		}

		m.showStatus("Nothing selected")

		return m, m.statusTimer

	default:
		return m, nil
	}
}

// toggleLastToolOutput expands or collapses the last tool output panel or skill message.
func (m *Model) toggleLastToolOutput() {
	items := m.chat.Items()
	for i, item := range slices.Backward(items) {
		if tp, ok := item.(*messages.ToolPanel); ok {
			tp.ToggleExpanded()
			m.chat = m.chat.UpdateItemByID(tp)

			return
		}

		if um, ok := item.(*messages.UserMessage); ok && um.IsSkillInvocation() {
			um.ToggleExpanded()
			m.chat = m.chat.UpdateItemAt(i, um)

			return
		}
	}
}

// copySelectionCmd returns a command that copies the given text to the clipboard
// using a dual strategy: OSC 52 escape sequences (for terminal-based copy) and
// the OS clipboard API (for direct system clipboard access).
func copySelectionCmd(text string) tea.Cmd {
	return tea.Batch(
		tea.SetClipboard(text),
		func() tea.Msg {
			if err := clipboard.WriteAll(text); err != nil {
				return notifyTypedMsg{message: "Copy failed: " + err.Error(), level: sdk.NotifyError}
			}

			return notifyTypedMsg{message: "Copied to clipboard", level: sdk.NotifySuccess}
		},
	)
}

// externalEditorMsg is sent when the external editor finishes.
type externalEditorMsg struct {
	text string
	err  error
}

// openExternalEditor opens the current editor content in an external editor.
func (m Model) openExternalEditor() (tea.Model, tea.Cmd) {
	text := m.editor.Value()

	tmpFile, err := os.CreateTemp("", "weave-editor-*.md")
	if err != nil {
		m.statusMsg = "Failed to create temp file: " + err.Error()
		m.statusGen++
		gen := m.statusGen
		m.statusTimer = tea.Tick(statusMessageTimeout, func(_ time.Time) tea.Msg {
			return statusTimeoutMsg{gen: gen}
		})

		return m, m.statusTimer
	}

	_ = tmpFile.Chmod(0o600)

	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(text); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)

		m.statusMsg = "Failed to write temp file: " + err.Error()
		m.statusGen++
		gen := m.statusGen
		m.statusTimer = tea.Tick(statusMessageTimeout, func(_ time.Time) tea.Msg {
			return statusTimeoutMsg{gen: gen}
		})

		return m, m.statusTimer
	}

	_ = tmpFile.Close()

	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}

	if editor == "" {
		editor = "vi"
	}

	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], tmpPath)...) //nolint:gosec,noctx // editor path comes from env

	return m, tea.ExecProcess(cmd, func(procErr error) tea.Msg {
		if procErr != nil {
			_ = os.Remove(tmpPath)

			return externalEditorMsg{err: procErr}
		}

		data, readErr := os.ReadFile(tmpPath)
		_ = os.Remove(tmpPath)

		if readErr != nil {
			return externalEditorMsg{err: readErr}
		}

		return externalEditorMsg{text: string(data)}
	})
}

// findStreamingAssistant scans chat items backwards for the active streaming assistant message.
// Returns the message and its index, or nil and -1 if not found.
func (m *Model) findStreamingAssistant() (*messages.AssistantMessage, int) {
	for i, item := range slices.Backward(m.chat.Items()) {
		if am, ok := item.(*messages.AssistantMessage); ok && am.IsStreaming() {
			return am, i
		}
	}

	return nil, -1
}

// onMessageUpdate appends a delta to the current assistant message and updates token rate.
func (m *Model) onMessageUpdate(msg MessageUpdateMsg) {
	if msg.TokenRate > 0 {
		m.footer = m.footer.SetTokenRate(msg.TokenRate)
	}

	am, idx := m.findStreamingAssistant()
	if am == nil {
		return
	}

	am.Append(msg.Content)
	m.chat = m.chat.UpdateItemAt(idx, am)
}

// onMessageEnd finalizes the current assistant message and creates pending tool panels.
func (m *Model) onMessageEnd(msg MessageEndMsg) {
	m.footer = m.footer.SetTokenRate(0)

	for _, tc := range msg.ToolCalls {
		m.trackPendingToolCall(tc)
	}

	am, idx := m.findStreamingAssistant()
	if am == nil {
		return
	}

	if msg.Thinking != "" {
		m.chat = m.chat.InsertItemAt(idx, messages.NewThinkingBlock(msg.Thinking))
		idx++
	}

	am.Finalize(msg.Content)
	m.chat = m.chat.UpdateItemAt(idx, am)

	for _, tc := range msg.ToolCalls {
		args, err := json.Marshal(tc.Arguments)

		argsStr := "{}"
		if err == nil {
			argsStr = string(args)
		}

		panel := messages.NewToolPanel(tc.ID, tc.Name, argsStr)
		if m.ui != nil {
			if r, ok := m.ui.GetRichRenderer(tc.Name); ok {
				panel.SetRenderer(&richRendererAdapter{renderer: r, themeFunc: m.ui.Theme})
			} else if r, ok := m.ui.GetRenderer(tc.Name); ok {
				panel.SetRenderer(r)
			} else {
				panel.SetDiffRenderer(messages.NewDiffRenderer())
			}
		} else {
			panel.SetDiffRenderer(messages.NewDiffRenderer())
		}

		m.toolPanels[tc.ID] = panel
		m.chat = m.chat.AddItem(panel)
	}
}

// onToolResult updates the tool panel with the result.
func (m *Model) onToolResult(msg ToolResultMsg) {
	m.clearPendingToolCall(msg.ToolID)

	panel, ok := m.toolPanels[msg.ToolID]
	if !ok {
		panel = messages.NewToolPanel(msg.ToolID, msg.Tool, "")
		m.toolPanels[msg.ToolID] = panel
		m.chat = m.chat.AddItem(panel)
	}

	panel.SetResult(msg.Result.Content, msg.Result.IsError)
	m.chat = m.chat.UpdateItemByID(panel)
}

func (m *Model) trackPendingToolCall(tc sdk.ToolCall) {
	if tc.ID == "" {
		return
	}

	if m.pendingToolCalls == nil {
		m.pendingToolCalls = make(map[string]string)
	}

	if _, exists := m.pendingToolCalls[tc.ID]; !exists {
		m.pendingToolOrder = append(m.pendingToolOrder, tc.ID)
	}

	m.pendingToolCalls[tc.ID] = tc.Name
}

func (m *Model) clearPendingToolCall(id string) {
	if id == "" || m.pendingToolCalls == nil {
		return
	}

	if _, exists := m.pendingToolCalls[id]; !exists {
		return
	}

	delete(m.pendingToolCalls, id)

	for len(m.pendingToolOrder) > 0 {
		if _, ok := m.pendingToolCalls[m.pendingToolOrder[0]]; ok {
			return
		}

		m.pendingToolOrder = m.pendingToolOrder[1:]
	}
}

func (m Model) activeToolName() string {
	for _, id := range m.pendingToolOrder {
		if name, ok := m.pendingToolCalls[id]; ok {
			return name
		}
	}

	return ""
}

// interruptStreaming finalizes the current streaming assistant message with
// an [interrupted] tag, hides the spinner, and publishes an interrupt event.
func (m Model) interruptStreaming() (tea.Model, tea.Cmd) {
	am, idx := m.findStreamingAssistant()
	if am == nil {
		return m, nil
	}

	am.Interrupt()
	m.chat = m.chat.UpdateItemAt(idx, am)
	m.spinner = m.spinner.Hide()
	m.syncChatViewport()
	m.footer = m.footer.SetTokenRate(0)

	var cmds []tea.Cmd
	if m.bus != nil {
		cmds = append(cmds, PublishInterrupt(m.bus))
	}

	return m, tea.Batch(cmds...)
}

// AddUserMessage adds a user message to the chat.
func (m *Model) AddUserMessage(content string) {
	m.chat = m.chat.AddItem(messages.NewUserMessage(content))
}

// onOutdatedExtensions renders a notification banner for outdated extensions.
func (m Model) onOutdatedExtensions(msg OutdatedNotificationMsg) (tea.Model, tea.Cmd) {
	if len(msg.Extensions) == 0 {
		return m, nil
	}

	names := make([]string, len(msg.Extensions))
	for i, ext := range msg.Extensions {
		names[i] = ext.Name
	}

	text := formatOutdatedBanner(names)

	m.showLanding = false
	m.chat = m.chat.AddItem(newNotifyAssistantMsg(text))

	return m, nil
}

// formatOutdatedBanner formats outdated extension names into a notification message.
func formatOutdatedBanner(names []string) string {
	hint := "Run `weave update` to update all, or `weave update <name>`"

	if len(names) == 1 {
		return fmt.Sprintf("Extension Updates Available\n%s has a newer version available.\n%s", names[0], hint)
	}

	nameList := strings.Join(names, ", ")

	return fmt.Sprintf("Extension Updates Available\n%s have newer versions available.\n%s", nameList, hint)
}

// onSubmit handles editor submit — routes slash commands or publishes prompt/followup.
func (m Model) onSubmit(text string) (tea.Model, tea.Cmd) {
	// Reject empty submissions without attachments.
	if text == "" && len(m.attach.Items()) == 0 {
		return m, nil
	}

	// Try slash command dispatch first.
	if handled, result := m.commands.Dispatch(text); handled { //nolint:nestif // command dispatch has multiple optional outcomes
		cmdName, cmdArgs := parseCommand(text)
		if skillName, ok := strings.CutPrefix(cmdName, "/skill:"); ok {
			xmlContent := fmt.Sprintf("<skill name=%q>\n%s\n</skill>", skillName, cmdArgs)

			m.chat = m.chat.AddItem(messages.NewUserMessage(xmlContent))
			m.prompted = true
			m.showLanding = false
		}

		if result.Quit {
			return m, tea.Quit
		}

		if result.ClearChat {
			m.chat = components.NewChatModel().SetSize(m.width, m.chatHeight(m.height))
			m.toolPanels = make(map[string]*messages.ToolPanel)
			m.attach = m.attach.Clear()
			m.showLanding = true
		}

		if result.ResetPrompt {
			m.prompted = false
		}

		if result.Notify != "" {
			m.showLanding = false

			m.chat = m.chat.AddItem(messages.NewAssistantMessage())

			items := m.chat.Items()
			if am, ok := items[len(items)-1].(*messages.AssistantMessage); ok {
				am.Finalize(result.Notify)
				m.chat = m.chat.UpdateItem(am)
			}
		}

		return m, result.Command
	}

	// Merge attachments into prompt text and clear them
	promptText := m.attach.RenderPrompt(text)
	m.attach = m.attach.Clear()

	m.AddUserMessage(promptText)

	if !m.prompted {
		m.prompted = true
		m.showLanding = false

		if m.bus != nil {
			m.bus.Publish(sdk.NewEvent(topicPrompt, promptText))
		}

		if !m.spinner.Visible() {
			var tickCmd tea.Cmd

			m.spinner, tickCmd = m.spinner.SpinnerUpdate(components.SpinnerShowMsg{})
			m.syncChatViewport()

			return m, tickCmd
		}

		return m, nil
	}

	if m.bus != nil {
		m.bus.Publish(sdk.NewEvent(topicFollowup, promptText))
	}

	if !m.spinner.Visible() {
		var tickCmd tea.Cmd

		m.spinner, tickCmd = m.spinner.SpinnerUpdate(components.SpinnerShowMsg{})
		m.syncChatViewport()

		return m, tickCmd
	}

	return m, nil
}

func (m Model) onSessionListResult(msg SessionListResultMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		am := messages.NewAssistantMessage()
		am.Finalize(fmt.Sprintf("Error listing sessions: %v", msg.Err))
		m.chat = m.chat.AddItem(am)

		return m, nil
	}

	if len(msg.Sessions) == 0 {
		am := messages.NewAssistantMessage()
		am.Finalize("No sessions found.")
		m.chat = m.chat.AddItem(am)

		return m, nil
	}

	items := make([]overlays.SelectorItem, len(msg.Sessions))
	for i, s := range msg.Sessions {
		items[i] = overlays.SelectorItem{
			Title:    shortenCWD(s.CWD),
			Subtitle: s.UpdatedAt.Format("2006-01-02 15:04"),
		}
	}

	m.pendingSessions = msg.Sessions
	sel := overlays.NewSelectorModel("Resume Session", items)
	sel = sel.SetSize(m.width, m.height).Show()
	m.dialogStack = m.dialogStack.Push(overlays.NewSelectorDialog(dialogSessionSelect, sel))

	return m, nil
}

func (m Model) onModelListResult(msg ModelListResultMsg) (tea.Model, tea.Cmd) {
	if len(msg.Models) == 0 {
		am := messages.NewAssistantMessage()
		am.Finalize("No models available.")
		m.chat = m.chat.AddItem(am)

		return m, nil
	}

	if len(msg.Models) == 1 {
		am := messages.NewAssistantMessage()
		am.Finalize("Only one model available: " + msg.Models[0].Display())
		m.chat = m.chat.AddItem(am)

		return m, nil
	}

	items := make([]overlays.SelectorItem, len(msg.Models))
	for i, model := range msg.Models {
		title := model.DisplayName()
		if model.Provider == m.currentModel.Provider && model.Model == m.currentModel.Model {
			title += " ✓"
		}

		items[i] = overlays.SelectorItem{
			Title:    title,
			Subtitle: "[" + model.Provider + "]",
		}
	}

	m.pendingModels = msg.Models
	sel := overlays.NewSelectorModel("Select Model", items)
	sel = sel.SetSize(m.width, m.height).Show()
	m.dialogStack = m.dialogStack.Push(overlays.NewSelectorDialog(dialogModelSelect, sel))

	return m, nil
}

func (m Model) onModelChanged(msg ModelChangedMsg) (tea.Model, tea.Cmd) {
	m.prevModel = m.currentModel
	m.prevThinkingLevel = m.thinkingLevel
	m.currentModel = msg.Entry
	m.footer = m.footer.SetModel(msg.Entry.Model, msg.Entry.Provider)
	m.footer = m.footer.SetReasoning(modelReasoning(msg.Entry))

	thinkingChanged := false

	if modelDef, ok := sdkmodel.GetModelForProvider(msg.Entry.Model, msg.Entry.Provider); ok {
		if !modelDef.Reasoning {
			thinkingChanged = m.thinkingLevel != sdkmodel.ThinkingOff
			m.thinkingLevel = sdkmodel.ThinkingOff
			m.footer = m.footer.SetThinkingLevel(string(sdkmodel.ThinkingOff))
			m.editor = m.editor.SetBorderColor(palette.ThinkingBorderColor(sdkmodel.ThinkingOff))
		} else if clamped := sdkmodel.ClampForModel(m.thinkingLevel, modelDef); clamped != m.thinkingLevel {
			thinkingChanged = true
			m.thinkingLevel = clamped
			m.footer = m.footer.SetThinkingLevel(string(clamped))
			m.editor = m.editor.SetBorderColor(palette.ThinkingBorderColor(clamped))
		}
	}

	displayName := msg.Entry.DisplayName()
	m.showStatus(fmt.Sprintf("Switched to %s (thinking: %s)", displayName, m.thinkingLevel))

	if m.bus != nil {
		var cmds []tea.Cmd

		cmds = append(cmds, PublishModelChange(m.bus, msg.Entry))

		if thinkingChanged {
			cmds = append(cmds, PublishThinkingChange(m.bus, m.thinkingLevel))
		}

		if m.cfg != nil {
			cmds = append(cmds, saveSettingsCmd(m.ps, m.currentModel, m.thinkingLevel))
		}

		cmds = append(cmds, m.statusTimer)

		return m, tea.Batch(cmds...)
	}

	return m, m.statusTimer
}

func (m Model) onModelChangeFailed(msg ModelChangeFailedMsg) (tea.Model, tea.Cmd) {
	m.currentModel = m.prevModel
	m.footer = m.footer.SetModel(m.prevModel.Model, m.prevModel.Provider)
	m.footer = m.footer.SetReasoning(modelReasoning(m.prevModel))

	m.thinkingLevel = m.prevThinkingLevel
	m.footer = m.footer.SetThinkingLevel(string(m.thinkingLevel))
	m.editor = m.editor.SetBorderColor(palette.ThinkingBorderColor(m.thinkingLevel))

	am := messages.NewAssistantMessage()
	am.Finalize("Failed to switch provider: " + msg.Error)
	m.chat = m.chat.AddItem(am)

	if m.bus != nil {
		return m, PublishThinkingChange(m.bus, m.thinkingLevel)
	}

	return m, nil
}

func (m Model) onProviderListResult(msg ProviderListResultMsg) (tea.Model, tea.Cmd) {
	if len(msg.Providers) == 0 {
		am := messages.NewAssistantMessage()
		am.Finalize("No providers available.")
		m.chat = m.chat.AddItem(am)

		return m, nil
	}

	items := make([]overlays.SelectorItem, len(msg.Providers))
	for i, p := range msg.Providers {
		statusText := "no key"
		if p.HasKey {
			statusText = "key set"
		}

		items[i] = overlays.SelectorItem{
			Title:    p.Name,
			Subtitle: statusText,
		}
	}

	m.pendingProviders = msg.Providers
	sel := overlays.NewSelectorModel("Manage Provider Keys", items)
	sel = sel.SetSize(m.width, m.height).Show()
	m.dialogStack = m.dialogStack.Push(overlays.NewSelectorDialog(dialogProviderSelect, sel))

	return m, nil
}

func (m Model) onLoginListResult(msg LoginListResultMsg) (tea.Model, tea.Cmd) {
	if len(msg.Providers) == 0 {
		am := messages.NewAssistantMessage()
		am.Finalize("No providers available for login.")
		m.chat = m.chat.AddItem(am)

		return m, nil
	}

	items := make([]overlays.SelectorItem, len(msg.Providers))
	for i, p := range msg.Providers {
		statusText := "not configured"
		if p.HasAuth {
			statusText = "configured"
		}

		if p.IsOAuth {
			statusText += " · OAuth"
		}

		items[i] = overlays.SelectorItem{
			Title:    p.Name,
			Subtitle: statusText,
		}
	}

	m.pendingLoginProviders = msg.Providers
	sel := overlays.NewSelectorModel("Login to Provider", items)
	sel = sel.SetSize(m.width, m.height).Show()
	m.dialogStack = m.dialogStack.Push(overlays.NewSelectorDialog(dialogLoginSelect, sel))

	return m, nil
}

func (m Model) onLogoutListResult(msg LogoutListResultMsg) (tea.Model, tea.Cmd) {
	if len(msg.Providers) == 0 {
		am := messages.NewAssistantMessage()
		am.Finalize("No providers currently authenticated.")
		m.chat = m.chat.AddItem(am)

		return m, nil
	}

	items := make([]overlays.SelectorItem, len(msg.Providers))
	for i, p := range msg.Providers {
		items[i] = overlays.SelectorItem{
			Title:    p.Name,
			Subtitle: "configured",
		}
	}

	m.pendingLogoutProviders = msg.Providers
	sel := overlays.NewSelectorModel("Logout from Provider", items)
	sel = sel.SetSize(m.width, m.height).Show()
	m.dialogStack = m.dialogStack.Push(overlays.NewSelectorDialog(dialogLogoutSelect, sel))

	return m, nil
}

func (m Model) onProviderDialogDone(result overlays.DialogResult, pendingCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if result.Err != nil || result.Index < 0 || result.Index >= len(m.pendingProviders) {
		m.pendingProviders = nil
		return m, pendingCmd
	}

	selected := m.pendingProviders[result.Index]
	m.providerTarget = selected.Name

	// Push key input dialog for entering the API key.
	input := overlays.NewInputModel("Enter API key for " + selected.Name)
	input = input.SetMask('*').SetSize(m.width, m.height).Show()
	m.dialogStack = m.dialogStack.Push(overlays.NewInputDialog(dialogKeyInput, input))

	return m, pendingCmd
}

func (m Model) onKeyInputDialogDone(result overlays.DialogResult, pendingCmd tea.Cmd) (tea.Model, tea.Cmd) {
	m.pendingProviders = nil
	providerName := m.providerTarget
	m.providerTarget = ""

	if result.Err != nil || providerName == "" {
		return m, pendingCmd
	}

	apiKey := strings.TrimSpace(result.Value)
	if apiKey == "" {
		return m, pendingCmd
	}

	if m.cfg == nil {
		am := messages.NewAssistantMessage()
		am.Finalize("No config available to save API key.")
		m.chat = m.chat.AddItem(am)

		return m, pendingCmd
	}

	if m.ps == nil {
		am := messages.NewAssistantMessage()
		am.Finalize("No preference store available to save API key.")
		m.chat = m.chat.AddItem(am)

		return m, pendingCmd
	}

	err := m.ps.SaveProviderKey(providerName, apiKey)

	am := messages.NewAssistantMessage()
	if err != nil {
		am.Finalize(fmt.Sprintf("Failed to save API key for %s: %v", providerName, err))
		m.chat = m.chat.AddItem(am)

		return m, pendingCmd
	}

	// Verify the saved key is actually usable by this provider.
	// OAuth-only providers (e.g. Codex) do not accept API keys and will
	// reject them; routing the user to /login instead.
	if sdk.ProviderRegistered(providerName) {
		hasAuth, checkErr := sdk.CheckProviderAuth(providerName)
		if checkErr != nil || !hasAuth {
			am.Finalize(fmt.Sprintf("API key not recognized for %s. This provider requires OAuth login via /login.", providerName))
			m.chat = m.chat.AddItem(am)

			return m, pendingCmd
		}
	}

	am.Finalize(fmt.Sprintf("API key saved for %s.", providerName))
	m.chat = m.chat.AddItem(am)

	// Update in-memory auth status so the provider is immediately usable.
	sdkmodel.SetProviderAuth(providerName, true)

	// If we were in noConfigured state, re-evaluate now that a key exists.
	if m.noConfigured {
		models := listModels()
		if len(models) > 0 {
			m.noConfigured = false
			cur := currentModel(models, m.ps)
			m.currentModel = cur
			m.footer = m.footer.SetModel(cur.Model, cur.Provider)
			m.footer = m.footer.SetReasoning(modelReasoning(cur))
		}
	}

	var cmds []tea.Cmd

	cmds = append(cmds, pendingCmd)

	if m.bus != nil {
		cmds = append(cmds, PublishAuthLoginSuccess(m.bus, providerName))

		// If we transitioned out of noConfigured, publish model.change so the
		// agent loop switches to the newly available provider.
		if !m.noConfigured {
			cmds = append(cmds, PublishModelChange(m.bus, m.currentModel))
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) onLoginDialogDone(result overlays.DialogResult, pendingCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if result.Err != nil || result.Index < 0 || result.Index >= len(m.pendingLoginProviders) {
		m.pendingLoginProviders = nil
		return m, pendingCmd
	}

	selected := m.pendingLoginProviders[result.Index]
	m.pendingLoginProviders = nil

	if selected.IsOAuth {
		return m.startOAuthLogin(selected, pendingCmd)
	}

	// API key flow: reuse the existing key input dialog.
	m.providerTarget = selected.ID
	input := overlays.NewInputModel("Enter API key for " + selected.Name)
	input = input.SetMask('*').SetSize(m.width, m.height).Show()
	m.dialogStack = m.dialogStack.Push(overlays.NewInputDialog(dialogKeyInput, input))

	return m, pendingCmd
}

// startOAuthLogin initiates the OAuth login flow for the selected provider.
// It handles both device code and authorization code flows.
func (m Model) startOAuthLogin(selected LoginProviderEntry, pendingCmd tea.Cmd) (tea.Model, tea.Cmd) {
	oauthProvider, ok := sdk.GetOAuthProvider(selected.ID)
	if !ok {
		am := messages.NewAssistantMessage()
		am.Finalize(fmt.Sprintf("OAuth provider %s not configured.", selected.Name))
		m.chat = m.chat.AddItem(am)

		return m, pendingCmd
	}

	// Create a cancellable context for the OAuth flow so force-canceling the
	// dialog can stop the background callback server / polling.
	m.oauthGen++
	ctx, cancel := context.WithCancel(context.Background())
	m.oauthCtx = ctx
	m.oauthCancel = cancel

	if oauthProvider.FlowType == sdk.DeviceCode {
		// Device code flow: request code synchronously, then poll asynchronously.
		reqCtx, reqCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer reqCancel()

		resp, err := sdk.RequestDeviceCode(reqCtx, oauthProvider.DeviceCodeURL, oauthProvider.ClientID, oauthProvider.Scopes)
		if err != nil {
			m.oauthCancel = nil

			cancel()

			am := messages.NewAssistantMessage()
			am.Finalize(fmt.Sprintf("Failed to start device code flow for %s: %v", selected.Name, err))
			m.chat = m.chat.AddItem(am)

			return m, pendingCmd
		}

		// Show login dialog with user code and verification URL.
		loginDlg := overlays.NewLoginModel(selected.Name, resp.VerificationURLOrURI())
		loginDlg = loginDlg.SetStatus("Enter code: "+resp.UserCode).SetSize(m.width, m.height).Show()
		m.dialogStack = m.dialogStack.Push(overlays.NewLoginDialog(dialogLoginOAuth, loginDlg))

		interval := resp.Interval
		if interval <= 0 {
			interval = 5
		}

		return m, tea.Batch(pendingCmd, pollDeviceCodeCmd(ctx, selected.ID, resp.DeviceCode, interval, oauthProvider.TokenURL, oauthProvider.ClientID, m.oauthGen))
	}

	// Authorization code flow: show dialog and run callback + browser flow.
	loginDlg := overlays.NewLoginModel(selected.Name, oauthProvider.AuthURL)
	loginDlg = loginDlg.SetSize(m.width, m.height).Show()
	m.dialogStack = m.dialogStack.Push(overlays.NewLoginDialog(dialogLoginOAuth, loginDlg))

	return m, tea.Batch(pendingCmd, runOAuthFlowCmd(ctx, oauthProvider, m.oauthGen))
}

func (m Model) onLogoutDialogDone(result overlays.DialogResult, pendingCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if result.Err != nil || result.Index < 0 || result.Index >= len(m.pendingLogoutProviders) {
		m.pendingLogoutProviders = nil
		return m, pendingCmd
	}

	selected := m.pendingLogoutProviders[result.Index]
	m.pendingLogoutProviders = nil

	if err := sdk.ClearProviderAuth(selected.ID); err != nil {
		am := messages.NewAssistantMessage()
		am.Finalize(fmt.Sprintf("Failed to clear auth for %s: %v", selected.Name, err))
		m.chat = m.chat.AddItem(am)

		return m, pendingCmd
	}

	// Update in-memory auth status so the provider is no longer usable.
	sdkmodel.SetProviderAuth(selected.ID, false)

	am := messages.NewAssistantMessage()
	am.Finalize(fmt.Sprintf("Logged out from %s.", selected.Name))
	m.chat = m.chat.AddItem(am)

	var cmds []tea.Cmd

	cmds = append(cmds, pendingCmd)

	if m.bus != nil {
		cmds = append(cmds, PublishAuthLogout(m.bus, selected.ID))
	}

	// Re-evaluate noConfigured state and switch to the next available provider.
	oldModel := m.currentModel

	models := listModels()
	if len(models) == 0 {
		m.noConfigured = true
	} else {
		m.noConfigured = false
		cur := currentModel(models, m.ps)
		m.currentModel = cur
		m.footer = m.footer.SetModel(cur.Model, cur.Provider)
		m.footer = m.footer.SetReasoning(modelReasoning(cur))
	}

	if m.bus != nil && !m.noConfigured && m.currentModel != oldModel {
		cmds = append(cmds, PublishModelChange(m.bus, m.currentModel))
	}

	return m, tea.Batch(cmds...)
}

// handleDialogDone processes a completed dialog from the stack.
func (m Model) handleDialogDone(d overlays.Dialog, pendingCmd tea.Cmd) (tea.Model, tea.Cmd) {
	id := d.ID()
	result := d.Result()

	switch id {
	case dialogSessionSelect:
		return m.onSessionDialogDone(result, pendingCmd)
	case dialogModelSelect:
		return m.onModelDialogDone(result, pendingCmd)
	case dialogProviderSelect:
		return m.onProviderDialogDone(result, pendingCmd)
	case dialogKeyInput:
		return m.onKeyInputDialogDone(result, pendingCmd)
	case dialogLoginSelect:
		return m.onLoginDialogDone(result, pendingCmd)
	case dialogLogoutSelect:
		return m.onLogoutDialogDone(result, pendingCmd)
	case dialogLoginOAuth:
		// Login OAuth dialog was dismissed (user canceled or flow completed).
		// The actual flow result is handled by LoginFlowResultMsg.
		if m.oauthCancel != nil {
			m.oauthCancel()
			m.oauthCancel = nil
			m.oauthCtx = nil
		}

		return m, pendingCmd
	default:
		// Popup dialogs: send result on channel.
		if ch, ok := m.popupChans[id]; ok {
			resp := overlayResponse{
				index:     result.Index,
				value:     result.Value,
				confirmed: result.Confirmed,
				selected:  result.Selected,
				err:       result.Err,
			}
			ch <- resp

			m.dockedOverlay = false
			delete(m.popupChans, id)
		}

		return m, tea.Batch(pendingCmd, checkNextPopupCmd(m.ui))
	}
}

// handleDialogForceCancel handles ctrl+c dismissal of the top dialog.
func (m Model) handleDialogForceCancel(d overlays.Dialog) (tea.Model, tea.Cmd) {
	id := d.ID()

	// Clean up pending data based on dialog purpose.
	switch id {
	case dialogSessionSelect:
		m.pendingSessions = nil
	case dialogModelSelect:
		m.pendingModels = nil
	case dialogProviderSelect:
		m.pendingProviders = nil
		m.providerTarget = ""
	case dialogKeyInput:
		m.pendingProviders = nil
		m.providerTarget = ""
	case dialogLoginSelect:
		m.pendingLoginProviders = nil
	case dialogLogoutSelect:
		m.pendingLogoutProviders = nil
	case dialogLoginOAuth:
		if m.oauthCancel != nil {
			m.oauthCancel()
			m.oauthCancel = nil
			m.oauthCtx = nil
		}
	default:
		// Popup dialog cancellation.
		if ch, ok := m.popupChans[id]; ok {
			ch <- overlayResponse{err: errors.New("canceled")}

			m.dockedOverlay = false
			delete(m.popupChans, id)
		}
	}

	return m, checkNextPopupCmd(m.ui)
}

func (m Model) onSessionDialogDone(result overlays.DialogResult, pendingCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if result.Err != nil || result.Index < 0 || result.Index >= len(m.pendingSessions) {
		m.pendingSessions = nil
		return m, pendingCmd
	}

	session := m.pendingSessions[result.Index]
	m.pendingSessions = nil

	if m.bus != nil {
		payload := sdk.SessionResumePayload{SessionID: session.ID}
		if store := sdk.GetSessionStore(); store != nil {
			history, err := store.LoadHistory(session.ID)
			if err != nil {
				return m, tea.Batch(
					pendingCmd,
					func() tea.Msg {
						return notifyTypedMsg{message: "Failed to load session: " + err.Error(), level: sdk.NotifyError}
					},
				)
			}

			payload.Messages = history
			m.rebuildChatFromMessages(history)
		} else {
			m.rebuildChatFromSession(session.ID)
		}

		m.showLanding = false
		m.prompted = true

		return m, tea.Batch(pendingCmd, PublishSessionResume(m.bus, payload))
	}

	return m, pendingCmd
}

func (m Model) onModelDialogDone(result overlays.DialogResult, pendingCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if result.Err != nil || result.Index < 0 || result.Index >= len(m.pendingModels) {
		m.pendingModels = nil
		return m, pendingCmd
	}

	selected := m.pendingModels[result.Index]
	m.pendingModels = nil

	model, cmd := m.onModelChanged(ModelChangedMsg{Entry: selected})

	return model, tea.Batch(pendingCmd, cmd)
}

func (m *Model) rebuildChatFromSession(sessionID string) {
	entries, err := loadSessionEntries(m.sessionDir, sessionID)
	if err != nil {
		m.chat = components.NewChatModel().SetSize(m.width, m.chatHeight(m.height))
		am := messages.NewAssistantMessage()
		am.Finalize(fmt.Sprintf("Error loading session: %v", err))
		m.chat = m.chat.AddItem(am)

		return
	}

	m.chat = components.NewChatModel().SetSize(m.width, m.chatHeight(m.height))
	m.toolPanels = make(map[string]*messages.ToolPanel)

	for _, entry := range entries {
		switch entry.Role {
		case sdk.RoleUser:
			m.chat = m.chat.AddItem(messages.NewUserMessage(entry.Content))
		case sdk.RoleAssistant:
			if entry.Thinking != "" {
				m.chat = m.chat.AddItem(messages.NewThinkingBlock(entry.Thinking))
			}

			am := messages.NewAssistantMessage()
			am.Finalize(entry.Content)
			m.chat = m.chat.AddItem(am)

			if len(entry.ToolCalls) > 0 {
				var tcs []sdk.ToolCall
				if err := json.Unmarshal(entry.ToolCalls, &tcs); err == nil {
					for _, tc := range tcs {
						args, _ := json.Marshal(tc.Arguments)
						panel := messages.NewToolPanel(tc.ID, tc.Name, string(args))
						m.toolPanels[tc.ID] = panel
						m.chat = m.chat.AddItem(panel)
					}
				}
			}
		case sdk.RoleToolResult:
			toolID, toolName, toolContent, toolIsError := parseToolEntry(entry.Tool)

			panel, ok := m.toolPanels[toolID]
			if !ok {
				panel = messages.NewToolPanel(toolID, toolName, "")
				m.toolPanels[toolID] = panel
				m.chat = m.chat.AddItem(panel)
			}

			panel.SetResult(toolContent, toolIsError)
			m.chat = m.chat.UpdateItemByID(panel)
		}
	}
}

// rebuildChatFromMessages rebuilds the chat display from sdk.Message slices.
// Used when Messages are available directly from the event payload.
func (m *Model) rebuildChatFromMessages(msgs []sdk.Message) {
	m.chat = components.NewChatModel().SetSize(m.width, m.chatHeight(m.height))
	m.toolPanels = make(map[string]*messages.ToolPanel)

	for _, msg := range msgs {
		content := ""

		if msg.Content != nil {
			if s, ok := msg.Content.(string); ok {
				content = s
			} else {
				content = fmt.Sprint(msg.Content)
			}
		}

		switch msg.Role {
		case sdk.RoleUser:
			m.chat = m.chat.AddItem(messages.NewUserMessage(content))
		case sdk.RoleAssistant:
			if len(msg.Thinking) > 0 {
				m.chat = m.chat.AddItem(messages.NewThinkingBlock(msg.Thinking[0].Thinking))
			}

			am := messages.NewAssistantMessage()
			am.Finalize(content)
			m.chat = m.chat.AddItem(am)

			for _, tc := range msg.ToolCalls {
				args, _ := json.Marshal(tc.Arguments)
				panel := messages.NewToolPanel(tc.ID, tc.Name, string(args))
				m.toolPanels[tc.ID] = panel
				m.chat = m.chat.AddItem(panel)
			}
		case sdk.RoleToolResult:
			panel, ok := m.toolPanels[msg.ToolCallID]
			if !ok {
				panel = messages.NewToolPanel(msg.ToolCallID, msg.ToolName, "")
				m.toolPanels[msg.ToolCallID] = panel
				m.chat = m.chat.AddItem(panel)
			}

			panel.SetResult(content, msg.IsError)
			m.chat = m.chat.UpdateItemByID(panel)
		}
	}
}

// parseToolEntry extracts tool call ID, name, content, and error flag from a stored tool entry.
func parseToolEntry(raw json.RawMessage) (id, name, content string, isError bool) {
	const defaultTool = "tool"

	if len(raw) == 0 {
		return "", defaultTool, "", false
	}

	var toolData struct {
		ID     string `json:"id"`
		Tool   string `json:"tool"`
		Result struct {
			Content string `json:"content"`
			IsError bool   `json:"is_error"`
		} `json:"result"`
	}

	if err := json.Unmarshal(raw, &toolData); err != nil {
		return "", defaultTool, "", false
	}

	id = toolData.ID

	name = toolData.Tool
	if name == "" {
		name = defaultTool
	}

	return id, name, toolData.Result.Content, toolData.Result.IsError
}

// cycleThinkingLevel returns the next distinct thinking level, skipping
// levels that would clamp to the same effective value as the current one.
func (m Model) cycleThinkingLevel() (tea.Model, tea.Cmd) {
	cur := m.thinkingLevel
	if modelDef, ok := sdkmodel.GetModelForProvider(m.currentModel.Model, m.currentModel.Provider); ok && modelDef.Reasoning {
		cur = sdkmodel.ClampForModel(cur, modelDef)
	}

	for i, lvl := range sdkmodel.AllThinkingLevels {
		if lvl != cur {
			continue
		}

		for j := 1; j <= len(sdkmodel.AllThinkingLevels); j++ {
			candidate := sdkmodel.AllThinkingLevels[(i+j)%len(sdkmodel.AllThinkingLevels)]

			var effective sdkmodel.ThinkingLevel

			if modelDef, ok := sdkmodel.GetModelForProvider(m.currentModel.Model, m.currentModel.Provider); ok {
				if modelDef.Reasoning {
					effective = sdkmodel.ClampForModel(candidate, modelDef)
				} else {
					effective = sdkmodel.ThinkingOff
				}
			} else {
				effective = candidate
			}

			if effective != cur {
				return m.applyThinkingLevel(candidate)
			}
		}

		return m, nil
	}

	return m, nil
}

// applyThinkingLevel applies a thinking level change.
// It clamps xhigh for models that don't support it, updates UI elements,
// shows a status message, and publishes the bus event.
func (m Model) applyThinkingLevel(level sdkmodel.ThinkingLevel) (tea.Model, tea.Cmd) {
	if modelDef, ok := sdkmodel.GetModelForProvider(m.currentModel.Model, m.currentModel.Provider); ok {
		if !modelDef.Reasoning {
			level = sdkmodel.ThinkingOff
		} else {
			level = sdkmodel.ClampForModel(level, modelDef)
		}
	}

	m.thinkingLevel = level
	m.footer = m.footer.SetThinkingLevel(string(level))
	m.editor = m.editor.SetBorderColor(palette.ThinkingBorderColor(level))

	m.showStatus(fmt.Sprintf("Thinking level: %s", level))

	var cmds []tea.Cmd

	if m.bus != nil {
		cmds = append(cmds, PublishThinkingChange(m.bus, level))

		if m.cfg != nil {
			cmds = append(cmds, saveSettingsCmd(m.ps, m.currentModel, level))
		}
	}

	if m.statusTimer != nil {
		cmds = append(cmds, m.statusTimer)
	}

	return m, tea.Batch(cmds...)
}

// cycleFocus moves focus through the editor → tray → panel cycle.
func (m Model) cycleFocus(reverse bool) (tea.Model, tea.Cmd) {
	if reverse {
		switch m.focus {
		case FocusEditor:
			if activeID := m.panelTray.ActiveID(); activeID != "" {
				m.panelManager.Show(activeID)
				m.syncChatViewport()
				m.focusSelectedPanel(activeID, true)
			}
		case FocusTray:
			m.focus = FocusEditor
			m.panelTray = m.panelTray.SetFocused(false)
			m.expandedPanelID = ""
		case FocusPanel:
			m.focus = FocusTray
			m.panelTray = m.panelTray.SetFocused(true)
			m.expandedPanelID = ""
		}
	} else {
		switch m.focus {
		case FocusEditor:
			m.focus = FocusTray
			m.panelTray = m.panelTray.SetFocused(true)
			m.expandedPanelID = ""
		case FocusTray:
			if activeID := m.panelTray.ActiveID(); activeID != "" {
				m.panelManager.Show(activeID)
				m.syncChatViewport()
				m.focusSelectedPanel(activeID, false)
			}
		case FocusPanel:
			m.focus = FocusEditor
			m.panelTray = m.panelTray.SetFocused(false)
			m.expandedPanelID = ""
		}
	}

	return m, nil
}

func (m *Model) activateSelectedPanel(panelID string, expandTrayOnly bool) {
	m.expandedPanelID = ""
	if entry, ok := m.panelManager.Get(panelID); ok && entry.Config.Placement == TrayOnly && expandTrayOnly {
		m.expandedPanelID = panelID
	}

	m.focus = FocusPanel
	m.panelTray = m.panelTray.SetFocused(false)
}

func (m *Model) focusSelectedPanel(panelID string, reverse bool) {
	if entry, ok := m.panelManager.Get(panelID); ok && entry.Config.Placement == TrayOnly {
		m.expandedPanelID = ""
		if reverse {
			m.focus = FocusTray
			m.panelTray = m.panelTray.SetFocused(true)

			return
		}

		m.focus = FocusEditor
		m.panelTray = m.panelTray.SetFocused(false)

		return
	}

	m.activateSelectedPanel(panelID, false)
}

func (m *Model) clearInvalidExpandedPanel() {
	if m.expandedPanelID == "" {
		return
	}

	if !m.panelManager.PanelVisible(m.expandedPanelID) {
		m.expandedPanelID = ""
		if m.focus == FocusPanel {
			m.focus = FocusEditor
			m.panelTray = m.panelTray.SetFocused(false)
		}
	}
}

func (m *Model) syncChatViewport() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	autoScroll := m.chat.AutoScroll()

	m.chat = m.chat.SetSize(m.width, m.chatHeight(m.height))
	if autoScroll {
		m.chat = m.chat.JumpToBottom()
	}
}

// showStatus sets a transient status message that clears after a timeout.
func (m *Model) showStatus(msg string) {
	m.statusMsg = msg
	m.statusNew = true
	m.statusGen++
	m.syncChatViewport()
	gen := m.statusGen
	m.statusTimer = tea.Tick(statusMessageTimeout, func(_ time.Time) tea.Msg {
		return statusTimeoutMsg{gen: gen}
	})
}

// cycleSandboxMode advances the sandbox to the next mode in the cycle order,
// updates the footer status pill.
func (m Model) cycleSandboxMode() (tea.Model, tea.Cmd) {
	sb := getSandboxer()
	if sb == nil {
		m.showStatus("Sandbox: not available")
		return m, m.statusTimer
	}

	current := sb.Mode()
	next := sandbox.NextSandboxMode(current)
	sb.SetMode(next)

	if m.bus != nil {
		m.bus.Publish(sdk.NewEvent("sandbox.mode.change", next))
	}

	m.footer = m.footer.SetExtStatus("sandbox", "SB:"+next)

	m.showStatus("Sandbox mode: " + next)

	return m, m.statusTimer
}

// handleCompletionKey processes keys when the completion popup is active.
// Returns true if the key was handled (intercepted for completion navigation).
func (m Model) handleCompletionKey(msg tea.KeyPressMsg) (bool, Model, tea.Cmd) {
	if !m.editor.CompletionActive() {
		return false, m, nil
	}

	switch msg.Code {
	case tea.KeyTab, tea.KeyUp, tea.KeyDown, tea.KeyEnter, tea.KeyKpEnter:
		// Alt+Enter or Shift+Enter inserts a newline, don't intercept
		if isEnterKey(msg.Code) && msg.Mod&(tea.ModAlt|tea.ModShift) != 0 {
			return false, m, nil
		}

		var cmd tea.Cmd

		m.editor, cmd = m.editor.Update(msg)

		return true, m, cmd
	}

	return false, m, nil
}

// refreshEditorCompletion reads the editor value and shows/hides the
// completion popup based on the current context. Uses the cursor's current
// line for multiline-aware completion.
func (m Model) refreshEditorCompletion() Model {
	value := m.editor.Value()

	lines := strings.Split(value, "\n")
	if len(lines) == 0 {
		m.editor = m.editor.HideCompletion()

		return m
	}

	lineIdx := m.editor.CursorLine()
	if lineIdx >= len(lines) {
		lineIdx = len(lines) - 1
	}

	if lineIdx < 0 {
		lineIdx = 0
	}

	line := lines[lineIdx]

	lineStart := 0
	for i := range lineIdx {
		lineStart += len(lines[i]) + 1
	}

	if lineIdx == 0 && strings.HasPrefix(line, "/") {
		m = m.slashCommandCompletion(line, lineStart)
	} else {
		m = m.atFileCompletion(line, lineStart)
	}

	if m.editor.CompletionActive() {
		return m
	}

	// Check registered autocomplete providers
	if m.ui != nil {
		m.ui.mu.Lock()
		providers := make([]AutocompleteProvider, len(m.ui.autocompleteProviders))
		copy(providers, m.ui.autocompleteProviders)
		m.ui.mu.Unlock()

		for _, provider := range providers {
			suggestions := provider.Suggestions(AutocompleteContext{
				Text:   value,
				Cursor: m.editor.CursorColumn(),
				Line:   line,
			})
			if len(suggestions) > 0 {
				items := make([]components.CompletionItem, len(suggestions))
				for i, s := range suggestions {
					items[i] = components.CompletionItem{
						Label:       s.Label,
						Description: s.Description,
						Value:       s.Value,
					}
				}

				comp := m.editor.ShowCompletion(components.CompletionCustom, items, "", lineStart)
				if comp.Completion().FilteredCount() > 0 {
					m.editor = comp

					return m
				}
			}
		}
	}

	m.editor = m.editor.HideCompletion()

	return m
}

// slashCommandCompletion handles "/" command and file completions at the start of a line.
func (m Model) slashCommandCompletion(line string, lineStart int) Model {
	cmdName, afterSpace, hasSpace := strings.Cut(line, " ")
	if !hasSpace {
		filter := strings.TrimPrefix(line, "/")
		names := m.commands.Names()

		items := make([]components.CompletionItem, 0, len(names))
		for _, name := range names {
			info, ok := m.commands.Lookup(name)
			if !ok {
				continue
			}

			items = append(items, components.CompletionItem{
				Label:       strings.TrimPrefix(name, "/"),
				Description: info.Description,
				Value:       name + " ",
			})
		}

		comp := m.editor.ShowCompletion(components.CompletionSlash, items, filter, lineStart)
		if comp.Completion().FilteredCount() == 0 {
			m.editor = m.editor.HideCompletion()
		} else {
			m.editor = comp
		}

		return m
	}

	if info, ok := m.commands.Lookup(cmdName); ok && info.AcceptsFiles {
		items := components.PathCompletions(".", afterSpace)
		triggerOffset := lineStart + len(cmdName) + 1
		// PathCompletions already filters by prefix, pass empty filter to avoid
		// double-filtering: SetFilter checks HasPrefix(Label, filter) but Label
		// is just the filename (e.g. "main.go"), not the full path.
		comp := m.editor.ShowCompletion(components.CompletionFile, items, "", triggerOffset)
		if comp.Completion().FilteredCount() == 0 {
			m.editor = m.editor.HideCompletion()
		} else {
			m.editor = comp
		}

		return m
	}

	m.editor = m.editor.HideCompletion()

	return m
}

// atFileCompletion handles "@" file path completions after whitespace.
func (m Model) atFileCompletion(line string, lineStart int) Model {
	atIdx := strings.LastIndex(line, "@")
	if atIdx < 0 || (atIdx > 0 && !isWhitespace(line[atIdx-1])) {
		m.editor = m.editor.HideCompletion()

		return m
	}

	cursorCol := m.editor.CursorColumn()
	atRunePos := len([]rune(line[:atIdx]))

	if cursorCol <= atRunePos {
		m.editor = m.editor.HideCompletion()

		return m
	}

	tokenLen := cursorCol - atRunePos - 1

	afterAt := []rune(line[atIdx+1:])
	if tokenLen > len(afterAt) {
		tokenLen = len(afterAt)
	}

	token := string(afterAt[:tokenLen])
	if strings.Contains(token, " ") {
		m.editor = m.editor.HideCompletion()

		return m
	}

	filter := token

	items := components.PathCompletions(".", filter)
	triggerOffset := lineStart + atIdx
	// PathCompletions already filters by prefix, pass empty filter to avoid
	// double-filtering: SetFilter checks HasPrefix(Label, filter) but Label
	// is just the filename (e.g. "main.go"), not the full path.
	comp := m.editor.ShowCompletion(components.CompletionFile, items, "", triggerOffset)
	if comp.Completion().FilteredCount() == 0 {
		m.editor = m.editor.HideCompletion()
	} else {
		m.editor = comp
	}

	return m
}

func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t'
}

// chatHeight returns the height allocated to the chat area.
func (m Model) chatHeight(totalHeight int) int {
	headerRows, pillRows := m.countLayoutRows()

	editorH := m.editor.Height() + m.attach.Height()
	trayRows, abovePanelRows, belowPanelRows := m.panelRows()
	lt := m.layout.ComputeWithPanels(m.width, totalHeight, editorH, headerRows, pillRows, m.dockedRows(), trayRows, abovePanelRows, belowPanelRows)

	return max(lt.Main.Dy(), 1)
}

// panelRows returns the tray, above-panel, and below-panel row counts.
func (m Model) panelRows() (trayRows, abovePanelRows, belowPanelRows int) {
	if m.panelManager == nil {
		return 0, 0, 0
	}

	visible := m.panelManager.VisiblePanels()
	if len(visible) > 0 {
		trayRows = 1
	}

	if m.panelManager.Active() != "" {
		placement := m.panelManager.ActivePanelPlacement()
		height := m.panelManager.ActivePanelHeight()

		switch placement {
		case AboveEditor:
			abovePanelRows = height
		case BelowEditor:
			belowPanelRows = height
		case AsOverlay:
			// overlay panels don't allocate fixed rows
		case TrayOnly:
			// tray-only panels are selected from the tray and rendered on demand
		}
	}

	return trayRows, abovePanelRows, belowPanelRows
}

// syncPanelTray updates the tray tabs from the panel manager's visible panels.
func (m *Model) syncPanelTray() {
	if m.panelManager == nil {
		return
	}

	visible := m.panelManager.VisiblePanels()
	tabs := make([]PanelTab, len(visible))
	activeIdx := -1

	for i, id := range visible {
		if entry, ok := m.panelManager.Get(id); ok {
			title := entry.Config.Title
			if title == "" {
				title = id
			}

			tabs[i] = PanelTab{ID: id, Title: title}
			if id == m.panelManager.Active() {
				activeIdx = i
			}
		}
	}

	m.panelTray = m.panelTray.SetTabs(tabs, activeIdx)
}

// dockedRows returns the number of rows to allocate for a docked overlay.
func (m Model) dockedRows() int {
	if m.dockedOverlay {
		return dockedOverlayHeight
	}

	return 0
}

// Draw renders the TUI into an ultraviolet screen buffer.
// It computes layout regions and delegates to each component.
func (m Model) Draw(scr uv.Screen, area uv.Rectangle) {
	// When dialogs are open, render the underlying UI first with dimming,
	// then overlay the dialog stack on top.
	if !m.dialogStack.Empty() {
		if m.dockedOverlay {
			lt := m.drawNormalUI(scr, area, dockedOverlayHeight)
			m.dialogStack.Draw(scr, lt.Docked)

			return
		}

		m.drawNormalUI(scr, area, 0)
		m.applyBackdropDimming(scr, area)
		m.dialogStack.Draw(scr, area)

		return
	}

	m.drawNormalUI(scr, area, 0)
}

// drawNormalUI renders the standard TUI layout without dialogs.
// When dockedRows > 0, allocates space for a docked overlay dialog.
// Returns the computed layout for caller use (e.g., dialog placement).
func (m Model) drawNormalUI(scr uv.Screen, area uv.Rectangle, dockedRows int) Layout {
	headerRows, pillRows := m.countLayoutRows()

	editorH := m.editor.Height() + m.attach.Height()
	trayRows, abovePanelRows, belowPanelRows := m.panelRows()
	lt := m.layout.ComputeWithPanels(
		area.Dx(), area.Dy(),
		editorH, headerRows, pillRows, dockedRows,
		trayRows, abovePanelRows, belowPanelRows,
	)

	if lt.Main.Dy() > 0 {
		m.chat = m.chat.SetSize(lt.Main.Dx(), lt.Main.Dy())
	}

	m.drawHeader(scr, lt.Header)
	m.drawMainContent(scr, lt.Main)
	m.drawPills(scr, lt.Pills)
	m.drawPanelTray(scr, lt.PanelTray)
	m.drawActivePanel(scr, lt.AbovePanel)
	editorArea := m.drawEditorWithAttachments(scr, lt.Editor)
	m.drawCompletionPopupIfActive(scr, editorArea)
	m.drawActivePanel(scr, lt.BelowPanel)
	m.drawFooter(scr, lt.Footer)
	m.drawOverlayPanel(scr, area)

	return lt
}

func (m Model) countLayoutRows() (headerRows, pillRows int) {
	if !m.showLanding && m.showHints && !m.prompted && len(m.chat.Items()) == 0 {
		headerRows = 1
	}

	if m.spinner.Visible() {
		pillRows++
	}

	if m.statusMsg != "" {
		pillRows++
	}

	if pillRows > 0 {
		pillRows++
	}

	return headerRows, pillRows
}

func (m Model) drawHeader(scr uv.Screen, area uv.Rectangle) {
	if area.Dy() == 0 {
		return
	}

	if m.customHeader != nil {
		m.customHeader.Draw(scr, area)

		return
	}

	hintsStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.Muted)).
		Background(lipgloss.Color(m.theme.BackgroundTint)).
		Padding(0, 1)
	uv.NewStyledString(hintsStyle.Render(
		"ctrl+p model · ctrl+l select · shift+tab thinking · ctrl+t toggle",
	)).Draw(scr, area)
}

func (m Model) drawFooter(scr uv.Screen, area uv.Rectangle) {
	if m.customFooter != nil {
		m.customFooter.Draw(scr, area)

		return
	}

	m.footer.Draw(scr, area, m.theme)
}

func (m Model) drawMainContent(scr uv.Screen, area uv.Rectangle) {
	if m.showLanding {
		m.landing = m.landing.SetSize(area.Dx(), area.Dy())
		m.landing.Draw(scr, area, m.theme)
	} else {
		m.chat.Draw(scr, area)
	}
}

func (m Model) drawPanelTray(scr uv.Screen, area uv.Rectangle) {
	if area.Dy() == 0 {
		return
	}

	m.panelTray.Draw(scr, area, m.theme)
}

func (m Model) drawActivePanel(scr uv.Screen, area uv.Rectangle) {
	if area.Dy() == 0 {
		return
	}

	if activeID := m.panelManager.Active(); activeID != "" {
		m.panelManager.DrawPanel(activeID, scr, area)
	}
}

func (m Model) drawOverlayPanel(scr uv.Screen, area uv.Rectangle) {
	activeID := m.panelManager.Active()
	if m.expandedPanelID != "" {
		m.drawPanelOverlay(scr, area, m.expandedPanelID, true)

		return
	}

	if activeID == "" || m.panelManager.ActivePanelPlacement() != AsOverlay {
		return
	}

	m.drawPanelOverlay(scr, area, activeID, false)
}

func (m Model) drawPanelOverlay(scr uv.Screen, area uv.Rectangle, panelID string, framed bool) {
	entry, ok := m.panelManager.Get(panelID)
	if !ok || !entry.Visible {
		return
	}

	width := entry.Config.Width
	if framed && width <= 0 {
		width = min(max(64, area.Dx()*4/5), area.Dx())
	} else if width <= 0 || width > area.Dx() {
		width = area.Dx()
	}

	height := entry.Config.Height
	if framed && height <= 0 {
		height = min(max(12, area.Dy()*3/4), area.Dy())
	} else if height <= 0 || height > area.Dy() {
		height = area.Dy()
	}

	x := area.Min.X + (area.Dx()-width)/2
	y := area.Min.Y + (area.Dy()-height)/2

	overlayArea := uv.Rect(x, y, width, height)
	if !framed {
		m.panelManager.DrawPanel(panelID, scr, overlayArea)

		return
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.theme.BorderFocused)).
		Foreground(lipgloss.Color(m.theme.Foreground)).
		Background(lipgloss.Color(m.theme.Background))

	title := entry.Config.Title
	if title == "" {
		title = entry.Config.ID
	}

	boxLines := make([]string, max(height-2, 1))
	for i := range boxLines {
		boxLines[i] = strings.Repeat(" ", max(width-2, 0))
	}

	box := boxStyle.Render(strings.Join(boxLines, "\n"))
	uv.NewStyledString(box).Draw(scr, overlayArea)

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.AccentBright)).
		Background(lipgloss.Color(m.theme.Background)).
		Bold(true)
	titleArea := uv.Rect(overlayArea.Min.X+2, overlayArea.Min.Y, max(overlayArea.Dx()-4, 0), 1)
	uv.NewStyledString(titleStyle.Render(" "+title+" ")).Draw(scr, titleArea)

	contentArea := uv.Rect(overlayArea.Min.X+2, overlayArea.Min.Y+1, max(overlayArea.Dx()-4, 0), max(overlayArea.Dy()-2, 0))
	m.panelManager.DrawPanel(panelID, scr, contentArea)
}

func (m Model) drawPills(scr uv.Screen, area uv.Rectangle) {
	if area.Dy() == 0 {
		return
	}

	y := area.Min.Y

	if m.spinner.Visible() {
		spArea := uv.Rect(area.Min.X+1, y, max(area.Dx()-1, 0), 1)
		m.spinner.Draw(scr, spArea)

		y++
	}

	if m.statusMsg != "" && y < area.Max.Y {
		statusColor := m.theme.Foreground
		if m.statusNew {
			statusColor = m.theme.Muted
		}

		statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor))
		stArea := uv.Rect(area.Min.X+1, y, max(area.Dx()-1, 0), 1)
		uv.NewStyledString(statusStyle.Render(m.statusMsg)).Draw(scr, stArea)
	}
}

func (m Model) drawEditorWithAttachments(scr uv.Screen, area uv.Rectangle) uv.Rectangle {
	attachH := m.attach.Height()

	if attachH > 0 && area.Dy() > attachH {
		attachArea := uv.Rect(area.Min.X, area.Min.Y, area.Dx(), attachH)
		editorArea := uv.Rect(area.Min.X, area.Min.Y+attachH, area.Dx(), area.Dy()-attachH)

		m.attach.Draw(scr, attachArea)
		m.editor.Draw(scr, editorArea)

		return editorArea
	}

	m.editor.Draw(scr, area)

	return area
}

func (m Model) drawCompletionPopupIfActive(scr uv.Screen, editorArea uv.Rectangle) {
	if !m.editor.CompletionActive() {
		return
	}

	comp := m.editor.Completion()
	if comp.FilteredCount() > 0 {
		m.drawCompletionPopup(scr, editorArea)
	}
}

// applyBackdropDimming sets the foreground color of all rendered cells to muted,
// creating a dimmed appearance for the underlying UI when dialogs are open.
func (m Model) applyBackdropDimming(scr uv.Screen, area uv.Rectangle) {
	mutedColor := lipgloss.Color(m.theme.Muted)

	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			cell := scr.CellAt(x, y)
			if cell == nil || cell.IsZero() {
				continue
			}

			newCell := cell.Clone()
			newCell.Style.Fg = mutedColor
			scr.SetCell(x, y, newCell)
		}
	}
}

// drawCompletionPopup renders the completion popup positioned relative to the
// editor cursor. It renders above the cursor when there's enough space,
// otherwise below.
func (m Model) drawCompletionPopup(scr uv.Screen, editorArea uv.Rectangle) {
	comp := m.editor.Completion()

	popupW := min(50, editorArea.Dx())
	visibleItems := min(comp.FilteredCount(), 8) // maxVisible default is 8
	popupH := visibleItems + 2                   // content rows + top/bottom border

	// Cursor position within editor content area.
	// Account for left border (1) + left padding (1).
	cursorX := editorArea.Min.X + 2 + m.editor.CursorColumn()
	cursorY := editorArea.Min.Y + 1 + m.editor.VisualCursorLine()

	// Default: render above cursor
	popupX := cursorX
	popupY := cursorY - popupH

	// If not enough space above, render below
	if popupY < 0 {
		popupY = cursorY + 1
	}

	// Clamp to screen bottom
	if popupY+popupH > m.height {
		popupH = m.height - popupY
	}

	// Clamp X so popup doesn't overflow right edge
	if popupX+popupW > m.width {
		popupX = m.width - popupW
	}

	if popupX < 0 {
		popupX = 0
	}

	if popupH <= 0 || popupW < 4 {
		return
	}

	// Sync popup width so View() renders at the correct size
	comp = comp.SetWidth(popupW)

	popupArea := uv.Rect(popupX, popupY, popupW, popupH)
	comp.Draw(scr, popupArea)
}

// View renders the TUI using ultraviolet screen buffers.
func (m Model) View() tea.View {
	canvas := uv.NewScreenBuffer(m.width, m.height)
	m.Draw(canvas, canvas.Bounds())

	v := tea.NewView(uv.TrimSpace(canvas.Render()))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	v.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
	v.KeyboardEnhancements.ReportAssociatedText = true

	return v
}
