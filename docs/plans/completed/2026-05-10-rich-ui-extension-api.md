# Rich UI Extension API

## Overview

Expand weave's UI extension system from a minimal set of 8 fixed methods to a comprehensive API covering overlays, panels, widgets, themes, editor integration, custom rendering, and notifications. The design follows a two-layer approach:

1. **`sdk.UI` (universal)** — methods any UI backend (TUI, web, IDE) can implement. Blocking overlay methods accept functional options for flexibility (e.g., `WithKeepContent` to dock dialog at bottom instead of centered overlay).
2. **TUI-specific (`TUIExtAPI`)** — deeper capabilities (panels, custom overlays, theme registration, footer/header replacement, autocomplete, raw key events). Extensions import the TUI package directly. Launcher excludes them from non-TUI builds.

Balance principle: the extension API covers common operations. Extensions that need deep structural changes (replacing the editor component, rewiring focus lifecycle) fork the TUI module directly.

## Context (from brainstorm)

### Files/components involved

- `sdk/ui.go` — `UI` interface (8 methods), `Keybinding`, `ToolRenderer` types
- `sdk/ui_ext.go` — `UIExtension`, `UIExtensionWithBus` interfaces
- `sdk/ui_ext_registry.go` — `RegisterUIExtension` function
- `sdk/noop_ui.go` — `NoopUI` stub for headless mode
- `extensions/ui/tui/overlays.go` — overlay request/response types, dialog push logic
- `extensions/ui/tui/components/overlays/stack.go` — `Dialog` interface, `DialogStack`, adapter types
- `extensions/ui/tui/components/overlays/selector.go` — `SelectorModel`, `SelectorDialog`
- `extensions/ui/tui/components/overlays/confirm.go` — `ConfirmModel`, `ConfirmDialog`
- `extensions/ui/tui/components/overlays/input.go` — `InputModel`, `InputDialog`
- `extensions/ui/tui/layout.go` — `LayoutEngine` for screen region computation
- `extensions/ui/tui-sandbox/tui_sandbox.go` — existing UI extension using `RegisterUIExtension`
- `extensions/ui/tui/extensions/diff-viewer/diff_viewer.go` — existing UI extension using `RegisterRenderer`
- `launcher/auto_discover.go` — extension discovery, UI extension detection

### Related patterns found

- `Dialog` interface in `overlays/stack.go` is the core overlay abstraction — `ID()`, `Update()`, `Draw()`, `Handles()`, `Done()`, `Result()`, `SetSize()`
- `DialogStack` manages layered overlays with top-down key routing and fall-through
- Overlay requests use channels for blocking semantics — `overlayRequest.result chan overlayResponse`
- `NoopUI` provides zero-value defaults for all `UI` methods
- Launcher detects UI extensions by scanning for `RegisterUIExtension(` in source

### Dependencies identified

- Bubble Tea v2 (`charm.land/bubbletea/v2`) — TUI framework
- Ultraviolet (`github.com/charmbracelet/ultraviolet`) — screen buffers, layout
- Lipgloss v2 (`charm.land/lipgloss/v2`) — styling
- No new external dependencies needed

## Development Approach

- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- Maintain backward compatibility — existing `sdk.UI` method signatures change only with functional options (backward-compatible via variadic params)

## Testing Strategy

- **Unit tests**: required for every task
- Test `NoopUI` stubs for all new methods
- Test `DialogStack` with new panel/dialog types
- Test layout engine with panel tray regions
- Test functional options parsing
- Test launcher detection of TUI-dependent extensions

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope

## Design Summary

### `sdk.UI` — Universal API (8 existing + 8 new)

| Method | Status | Signature |
|---|---|---|
| `Select` | Updated | `Select(title string, items []string, opts ...SelectOption) (int, error)` |
| `Confirm` | Updated | `Confirm(message string, opts ...ConfirmOption) (bool, error)` |
| `Input` | Updated | `Input(prompt string, opts ...InputOption) (string, error)` |
| `SetStatus` | Existing | `SetStatus(key, text string)` |
| `Notify` | Existing | `Notify(message string)` |
| `RegisterCommand` | Existing | `RegisterCommand(name string, handler func(args string) error)` |
| `RegisterRenderer` | Existing | `RegisterRenderer(toolName string, renderer ToolRenderer)` |
| `RegisterKeybinding` | Existing | `RegisterKeybinding(kb Keybinding)` |
| `MultiSelect` | **New** | `MultiSelect(title string, items []string, defaults []bool, opts ...SelectOption) ([]int, error)` |
| `Editor` | **New** | `Editor(title string, prefill string, opts ...EditorOption) (string, error)` |
| `NotifyTyped` | **New** | `NotifyTyped(message string, level NotifyLevel)` |
| `SetWorking` | **New** | `SetWorking(message string)` |
| `ClearWorking` | **New** | `ClearWorking()` |
| `ShowError` | **New** | `ShowError(message string)` |
| `SetTheme` | **New** | `SetTheme(name string) error` |
| `ListThemes` | **New** | `ListThemes() []string` |

### New `sdk/` types

```go
// NotifyLevel for typed notifications
type NotifyLevel int
const (
    NotifyInfo NotifyLevel = iota
    NotifyWarning
    NotifyError
)

// Functional options for overlay methods
type SelectOption func(*SelectConfig)
type ConfirmOption func(*ConfirmConfig)
type InputOption func(*InputConfig)
type EditorOption func(*EditorConfig)

// WithKeepContent docks the dialog at bottom instead of centered overlay
func WithKeepContent() SelectOption { ... }
func WithKeepContent() ConfirmOption { ... }
func WithKeepContent() InputOption { ... }
func WithKeepContent() EditorOption { ... }
```

### TUI-specific — `TUIExtAPI` (20 methods)

| Method | Description |
|---|---|
| **Panels** | |
| `ShowPanel(config PanelConfig, drawer PanelDrawer)` | Show or update panel by ID |
| `HidePanel(id string)` | Hide panel (stays registered) |
| `RemovePanel(id string)` | Fully remove panel |
| `PanelVisible(id string) bool` | Check visibility |
| `PanelTray() PanelTrayAPI` | Access tray for tab ordering |
| **Read-only** | |
| `Theme() ThemeInfo` | Current theme info |
| `Size() (int, int)` | Terminal dimensions |
| **Editor** | |
| `EditorText() string` | Read editor content |
| `SetEditorText(text string)` | Write editor content |
| `PasteToEditor(text string)` | Insert at cursor |
| **Rendering** | |
| `RegisterRichRenderer(tool string, renderer RichToolRenderer)` | Tool output with theme |
| `RegisterMessageRenderer(msgType string, renderer MessageRenderer)` | Custom message types |
| **Footer/Header** | |
| `SetFooter(component)` | Replace footer |
| `SetHeader(component)` | Replace header |
| **Input** | |
| `OnTerminalInput(handler func(KeyEvent))` | Raw key events |
| `AddAutocomplete(provider AutocompleteProvider)` | Editor autocomplete |
| **Cosmetic** | |
| `SetWorkingFrames(frames []string, interval time.Duration)` | Custom spinner |
| `RegisterTheme(name string, theme ThemeDef) error` | Register custom theme |

### Panel system

```go
type PanelConfig struct {
    ID         string
    Placement  PanelPlacement  // AsOverlay | AboveEditor | BelowEditor
    Blocking   bool            // true = modal, false = non-blocking
    Width      int             // 0 = auto
    Height     int             // 0 = auto
}

type PanelPlacement int
const (
    AsOverlay   PanelPlacement = iota
    AboveEditor
    BelowEditor
)

type PanelDrawer interface {
    Draw(scr uv.Screen, area uv.Rectangle)
    Update(msg tea.Msg) (PanelDrawer, tea.Cmd)
    Handles(msg tea.Msg) bool
}
```

### Panel tray

- Tab strip between chat and editor (hidden when no panels)
- Each panel is a tab — user selects which is active
- Keybinding to open panel picker overlay (like Ctrl+P for models)
- Tab cycles focus: editor → tray → active panel → editor
- Esc returns focus to editor

### `WithKeepContent` option

When active, overlay dialogs dock at bottom of chat area instead of centered overlay. Chat viewport shrinks, dialog renders below it. Both fully visible simultaneously.

### Launcher integration

- TUI-dependent extensions detected by `RegisterTUIExtension(` in source scan
- Excluded from builds when active UI is not TUI (same pattern as current `RegisterUIExtension` headless exclusion)
- `TUIExtension` interface: `Name() string` + `RegisterTUI(api TUIExtAPI)`

### Fork territory (not in API)

- `SetEditor(component)` — replacing entire input editor
- `Focus(component)` — low-level focus management
- Deep structural layout changes

## Implementation Steps

### Task 1: Add new types and functional options to `sdk/`

- [x] add `NotifyLevel` type and constants to `sdk/ui.go`
- [x] add `SelectConfig`, `ConfirmConfig`, `InputConfig`, `EditorConfig` structs to `sdk/ui.go`
- [x] add functional option types: `SelectOption`, `ConfirmOption`, `InputOption`, `EditorOption`
- [x] add `WithKeepContent()` option constructors for each overlay type
- [x] update `UI` interface with new method signatures (variadic opts on existing methods + 8 new methods)
- [x] update `NoopUI` with stubs for all new methods
- [x] write tests for functional options parsing
- [x] write tests for `NoopUI` new method stubs
- [x] run `make test` — must pass before next task

### Task 2: Implement Editor overlay in TUI

- [x] create `EditorModel` in `extensions/ui/tui/components/overlays/editor.go` — multi-line text editor using `bubbles/v2 textarea`
- [x] create `EditorDialog` adapter implementing `Dialog` interface
- [x] add `requestEditor` to `overlayRequestKind` in `overlays.go`
- [x] implement `Editor()` method on `TUIImpl` with blocking channel pattern
- [x] write tests for `EditorModel` update/draw behavior
- [x] write tests for `EditorDialog` adapter lifecycle
- [x] run `cd extensions/ui/tui && go test ./...` — must pass before next task

### Task 3: Implement MultiSelect overlay in TUI

- [x] create `MultiSelectModel` in `extensions/ui/tui/components/overlays/multiselect.go` — checkbox list with toggle on Enter, confirm on Ctrl+Enter
- [x] create `MultiSelectDialog` adapter implementing `Dialog` interface
- [x] add `requestMultiSelect` to `overlayRequestKind`
- [x] implement `MultiSelect()` method on `TUIImpl`
- [x] write tests for `MultiSelectModel` toggle/filter/confirm behavior
- [x] write tests for `MultiSelectDialog` adapter
- [x] run `cd extensions/ui/tui && go test ./...` — must pass before next task

### Task 4: Implement NotifyTyped, ShowError, SetWorking/ClearWorking in TUI

- [x] add `notifyTypedMsg` with `level NotifyLevel` to `overlays.go`
- [x] update notification rendering to style by level (info=default, warning=yellow, error=red)
- [x] add `ShowError()` on `TUIImpl` — renders as error notification
- [x] add `SetWorking()` / `ClearWorking()` on `TUIImpl` — toggles working indicator with custom message
- [x] write tests for typed notification rendering
- [x] write tests for working indicator show/hide
- [x] run `cd extensions/ui/tui && go test ./...` — must pass before next task

### Task 5: Implement WithKeepContent option — docked overlay mode

- [x] add `KeepContent bool` to overlay request struct
- [x] update layout engine to split chat viewport when docked overlay is active (chat shrinks, dialog docks at bottom)
- [x] update `pushPopupDialog` to respect `KeepContent` flag — use docked placement instead of centered overlay
- [x] update `Select`, `Confirm`, `Input`, `Editor` implementations to pass `KeepContent` from options
- [x] write tests for layout splitting with docked overlay
- [x] write tests that overlay options correctly flow to request struct
- [x] run `cd extensions/ui/tui && go test ./...` — must pass before next task

### Task 6: Implement theme support

- [x] define `ThemeInfo` struct in `sdk/` with color/style fields (read-only info for extensions)
- [x] define `ThemeDef` struct in TUI for theme registration
- [x] implement `SetTheme(name string) error` on `TUIImpl` — switch active theme
- [x] implement `ListThemes() []string` on `TUIImpl` — return available theme names
- [x] implement `RegisterTheme(name string, theme ThemeDef) error` on `TUIExtAPI`
- [x] implement `Theme() ThemeInfo` on `TUIExtAPI`
- [x] write tests for theme switching
- [x] write tests for theme registration and listing
- [x] run `make test` — must pass before next task

### Task 7: Implement Panel system and Panel tray

- [x] define `PanelConfig`, `PanelPlacement`, `PanelDrawer` types in `extensions/ui/tui/`
- [x] create `PanelManager` to track registered panels (show/hide/remove/visible)
- [x] create `PanelTray` component — tab strip rendering active panels
- [x] update `LayoutEngine` to include panel regions (AboveEditor, BelowEditor, tray)
- [x] implement panel focus chain: editor → tray → active panel → editor (Tab/Shift+Tab)
- [x] implement Esc to return focus to editor from panel
- [x] register keybinding for panel picker overlay
- [x] write tests for `PanelManager` show/hide/remove lifecycle
- [x] write tests for layout engine with panel regions
- [x] write tests for focus chain cycling
- [x] run `cd extensions/ui/tui && go test ./...` — must pass before next task

### Task 8: Define TUIExtAPI and TUIExtension registration

- [x] define `TUIExtAPI` interface in `extensions/ui/tui/` with all 20 methods
- [x] define `TUIExtension` interface (`Name() string`, `RegisterTUI(api TUIExtAPI)`)
- [x] add `RegisterTUIExtension` function and registry
- [x] wire `TUIExtAPI` in TUI startup — create impl, pass to registered TUI extensions
- [x] update launcher `AutoDiscover` to detect `RegisterTUIExtension(` in source for build exclusion
- [x] write tests for TUI extension registration and wiring
- [x] write tests for launcher detection of TUI-dependent extensions
- [x] run `make test` — must pass before next task

### Task 9: Implement remaining TUIExtAPI methods

- [x] implement `EditorText()` / `SetEditorText()` / `PasteToEditor()` — bridge to editor component
- [x] implement `RegisterRichRenderer(tool, renderer)` — tool output with theme access
- [x] implement `RegisterMessageRenderer(msgType, renderer)` — custom message type rendering
- [x] implement `SetFooter(component)` / `SetHeader(component)` — component replacement
- [x] implement `OnTerminalInput(handler)` — raw key event subscription
- [x] implement `AddAutocomplete(provider)` — editor autocomplete provider registration
- [x] implement `SetWorkingFrames(frames, interval)` — custom spinner animation
- [x] write tests for each TUIExtAPI method
- [x] run `cd extensions/ui/tui && go test ./...` — must pass before next task

### Task 10: Update existing extensions to use new API

- [x] update `tui-sandbox` extension to use `NotifyTyped` for mode change messages
- [x] update `diff-viewer` extension to use `RegisterRichRenderer` for themed diff output
- [x] write tests verifying updated extensions still register correctly
- [x] run `make test` — must pass before next task

### Task 11: Verify acceptance criteria

- [x] verify all `sdk.UI` new methods work through TUI implementation
- [x] verify all `TUIExtAPI` methods work through extension registration
- [x] verify `NoopUI` stubs return sensible defaults for all new methods
- [x] verify launcher excludes TUI-dependent extensions from non-TUI builds
- [x] verify `WithKeepContent` docks overlays at bottom with chat visible
- [x] verify panel tray shows/hides correctly
- [x] verify focus chain works (editor → tray → panel → editor)
- [x] verify theme switching and registration
- [x] run full test suite (`make test`)
- [x] run linter (`make lint`) — all issues must be fixed

### Task 12: Update CLAUDE.md documentation

- [x] update `sdk/` section with new `UI` methods, `NotifyLevel`, functional options
- [x] update `extensions/ui/tui/` section with `TUIExtAPI`, panel system, panel tray
- [x] update launcher section with `RegisterTUIExtension` detection
- [x] add fork territory guidance to architecture section

## Technical Details

### Functional options pattern

Each overlay method gets its own option type for type safety:

```go
type SelectConfig struct {
    KeepContent bool
}
type SelectOption func(*SelectConfig)

func WithKeepContent() SelectOption {
    return func(c *SelectConfig) { c.KeepContent = true }
}
```

The `TUIImpl` checks `config.KeepContent` to decide centered overlay vs docked bottom placement.

### Panel system architecture

```
PanelManager
├── panels map[string]*panelEntry  // all registered panels
├── order []string                  // tray tab order
├── active string                   // currently visible panel ID
└── visible map[string]bool         // visibility state

PanelTray (component)
├── draws tab strip for visible panels
├── highlights active panel tab
└── handles tab selection via keyboard

LayoutEngine (updated)
├── adds AbovePanel and BelowPanel regions
├── adds TrayRegion between chat and editor
└── computes panel content area based on active panel
```

### Docked overlay layout

When `KeepContent` is active:
```
[Header]
[Chat viewport (height = original - dockHeight)]
[──────── Docked dialog border ────────]
[  > Option 1                          ]
[  > Option 2                          ]
[  > Option 3                          ]
[──────────────────────────────────────]
[Pills bar]
[Editor]
[Footer]
```

### TUI extension wiring flow

```
1. TUI startup
2. sdk.GetUIExtensions() → wire universal extensions (Register/UIExtension)
3. sdk.GetTUIExtensions() → wire TUI-specific extensions (RegisterTUI/TUIExtAPI)
4. TUIExtAPI impl created with references to layout, editor, theme, dialog stack
5. Each TUIExtension.RegisterTUI(api) called with the impl
```

### Message renderer integration

```
1. Extension calls api.RegisterMessageRenderer("jira-ticket", renderer)
2. TUI stores in messageRendererRegistry map[string]MessageRenderer
3. Chat component checks registry before default markdown render
4. If renderer found for message type, use it; otherwise default
```

## Post-Completion

**Manual verification:**
- Test panel tray rendering in terminal with different panel configurations
- Test docked overlay with chat scrolling (KeepContent mode)
- Test theme switching visually — verify all components update
- Test focus chain feels natural (Tab cycling between editor, tray, panels)
- Test TUI extension build exclusion with `--headless` flag

**External system updates:**
- Update extension developer documentation with new API reference
- Update extension template/scaffold to show TUIExtension pattern
- Consider publishing example TUI extension (e.g., panel-based file browser)
