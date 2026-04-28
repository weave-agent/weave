# TUI Bubble Tea v2 Migration + Rich Features

## Overview

Migrate the weave TUI from Bubble Tea v1 to v2 (`charm.land/bubbletea/v2`), replace the hand-rolled editor with bubbles/v2 textarea, and add rich feature improvements (custom Chroma formatter, file attachments, pills bar).

**Key changes:**
- Bubble Tea v1 → v2 (import paths, key handling API)
- Lip Gloss v1 → v2 (import paths, `Place` signature)
- Bubbles v1 → v2 (spinner API, new textarea component)
- Replace hand-rolled `EditorModel` with bubbles/v2 `textarea.Model`
- Custom Chroma formatter for consistent code block highlighting
- File attachments with auto-detection of long pastes
- Pills bar for tool progress indicators

**Reference:** `/Users/andrey/Projects/crush` — production Bubble Tea v2 TUI with textarea, completions, and ultraviolet screen buffers.

## Context

**Files/components involved:**
- `extensions/ui/tui/go.mod` — dependency version upgrades
- `extensions/ui/tui/model.go` — main model, Update/View/Draw, key dispatch
- `extensions/ui/tui/bridge.go` — bus-to-Tea message translation
- `extensions/ui/tui/keybindings.go` — key resolution (keyString, Resolve)
- `extensions/ui/tui/layout.go` — LayoutEngine regions
- `extensions/ui/tui/landing.go` — landing screen (lipgloss.Place)
- `extensions/ui/tui/components/editor.go` — **replaced** by bubbles/v2 textarea
- `extensions/ui/tui/components/chat.go` — chat viewport (lipgloss imports)
- `extensions/ui/tui/components/footer.go` — status bar (lipgloss imports)
- `extensions/ui/tui/components/spinner.go` — spinner (bubbles import)
- `extensions/ui/tui/components/messages/` — message renderers (lipgloss imports)
- `extensions/ui/tui/components/overlays/` — dialog components (tea.KeyMsg usage)
- All `_test.go` files in `extensions/ui/tui/` — import path updates

**Architecture reference (crush):**
- `internal/ui/model/ui.go` — main v2 model with textarea integration
- `internal/ui/completions/` — @-mention completion system (future reference)
- `internal/ui/styles/` — v2 lipgloss style definitions

## Development Approach

- **Testing approach:** Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- Run tests after each change

## Testing Strategy

- **Unit tests:** required for every task
- Use `github.com/stretchr/testify` (require for fatal, assert for non-fatal)
- Use moq-generated mocks — run `make gen` after changing interfaces
- Test key handling: verify correct action dispatched per key
- Test textarea integration: submit, history, resize, external editor
- Test layout adjustments: pills row visibility, textarea height changes
- Test attachment model: add, remove, paste detection
- Test Chroma formatter: token type → style mapping

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## Implementation Steps

---

### Task 1: Upgrade go.mod dependencies to v2

- [x] Update `extensions/ui/tui/go.mod` — replace bubbletea v1 with `charm.land/bubbletea/v2 v2.0.6`, lipgloss v1 with `charm.land/lipgloss/v2 v2.0.3`, bubbles v1 with `charm.land/bubbles/v2 v2.1.0`
- [x] Run `go mod tidy` to resolve transitive deps — fix any version conflicts
- [x] Verify `go build ./...` compiles (will fail on import paths, that's expected — just verify go.mod is consistent)
- [x] Run tests — must pass before Task 2

---

### Task 2: Migrate import paths (mechanical)

- [x] Replace all `github.com/charmbracelet/bubbletea` imports with `charm.land/bubbletea/v2` across all `.go` files in `extensions/ui/tui/`
- [x] Replace all `github.com/charmbracelet/lipgloss` imports with `charm.land/lipgloss/v2` across all `.go` files
- [x] Replace all `github.com/charmbracelet/bubbles` imports with `charm.land/bubbles/v2` across all `.go` files
- [x] Update `bubbles/spinner` sub-imports to `charm.land/bubbles/v2/spinner`
- [x] Run `go mod tidy` and verify `go build ./...` compiles (may fail on API changes, that's expected)
- [x] Run tests — skipped: tests require Task 3 API changes (msg.Type/tea.KeyRunes/msg.Runes) to compile

---

### Task 3: Migrate key handling to v2 API

Key changes: `tea.KeyMsg` → `tea.KeyPressMsg`, `msg.String()` → `msg.Key.String()`, `msg.Type == tea.KeyCtrlX` → `key.Matches(msg, ...)`.

- [x] Add `charm.land/bubbles/v2/key` import where needed
- [x] Update `keybindings.go` — change `keyString(msg tea.KeyMsg)` to `keyString(msg tea.KeyPressMsg)`, update `msg.String()` to `msg.Key.String()` (or appropriate v2 string representation)
- [x] Update `model.go` Update() — change `case tea.KeyMsg:` to `case tea.KeyPressMsg:`, update all key type checks (`tea.KeyCtrlC`, `tea.KeyEsc`, etc.) to use `key.Matches()` or `msg.Key.String()` comparisons
- [x] Update `model.go` Init() — remove or update any v1-specific key commands
- [x] Update `components/overlays/selector.go` — change `tea.KeyMsg` to `tea.KeyPressMsg`, update all key matching
- [x] Update `components/overlays/confirm.go` — change `tea.KeyMsg` to `tea.KeyPressMsg`, update all key matching
- [x] Update `components/overlays/input.go` — change `tea.KeyMsg` to `tea.KeyPressMsg`, update all key matching
- [x] Run `go build ./...` to verify compilation
- [x] Write tests for key dispatch — verify correct actions returned for key presses via v2 API
- [x] Run tests — must pass before Task 4

---

### Task 4: Migrate lipgloss and bubbles API changes

- [x] Update all `lipgloss.Place()` calls — v2 signature changed (check crush for new signature). Files: `landing.go`, `model.go`, `components/footer.go`
- [x] Update spinner initialization in `components/spinner.go` — v2 bubbles spinner API may differ (check constructor options)
- [x] Update any `lipgloss.NewStyle()` calls that use v1-only methods — scan all files for deprecated API
- [x] Verify all `View()` string rendering still works — lipgloss v2 style rendering should be compatible but verify
- [x] Run `go build ./...` and `go vet ./...` to verify compilation
- [x] Write tests for lipgloss style rendering — verify Place output, style composition
- [x] Run tests — must pass before Task 5

---

### Task 5: Replace hand-rolled editor with bubbles/v2 textarea

Reference: crush `internal/ui/model/ui.go` textarea setup at line ~270.

- [x] Remove `components/editor.go` (hand-rolled `EditorModel`) — replaced with textarea wrapper
- [x] Add `textarea` field to `Model` struct in `model.go` — type `textarea.Model` from `charm.land/bubbles/v2/textarea`
- [x] Initialize textarea in `NewModel()` or equivalent constructor — configure: `DynamicHeight(true)`, `MinHeight(3)`, `MaxHeight(15)`, `CharLimit(-1)`, `ShowLineNumbers(false)`, `SetVirtualCursor(false)`, `Focus()`
- [x] Implement textarea styling — create focused/blurred styles using lipgloss v2, match existing editor border colors
- [x] Forward `tea.KeyPressMsg` and `tea.WindowSizeMsg` to textarea in Update() — `ta, cmd := m.textarea.Update(msg)` with height change tracking (see crush `updateTextareaWithPrevHeight`)
- [x] Implement submit — `m.textarea.Value()` on enter, `m.textarea.Reset()` after send
- [x] Implement command history — maintain `[]string` history slice, navigate on up/down when textarea is at first/last line
- [x] Implement external editor (`ctrl+g`) — write textarea value to temp file, open `$EDITOR`, read back
- [x] Update `LayoutEngine.Compute()` — textarea height drives editor region size (dynamic, not fixed)
- [x] Update `Draw()` — render textarea into the editor region using textarea's `Draw()` or `View()` via screen buffer
- [x] Write tests for textarea submit — verify value retrieval and reset
- [x] Write tests for history — verify up/down navigation cycles through entries
- [x] Write tests for dynamic height — verify layout adjusts when textarea grows/shrinks
- [x] Run tests — must pass before Task 6

---

### Task 6: Custom Chroma formatter (xchroma)

- [x] Create `extensions/ui/tui/xchroma/formatter.go` — register a "weave" Chroma formatter in `init()`. Map Chroma token types (Keyword, String, Comment, Number, Operator, etc.) to Lip Gloss v2 styles. Force background color to match chat bubble background.
- [x] Update `components/messages/markdown.go` — configure Glamour renderer to use the "weave" Chroma formatter for code block syntax highlighting
- [x] Write tests for formatter — verify token type → style mapping, verify forced background, verify integration with Glamour rendering
- [x] Run tests — must pass before Task 7

---

### Task 7: File attachments

- [x] Create `components/attachments/attachments.go` — `AttachmentsModel` tracking `[]Attachment` with file path, line count, content preview. `Draw(scr, area)` renders inline indicators (`file.py (42 lines)`) above the editor.
- [x] Add paste detection in editor — if paste exceeds threshold (>10 newlines or >1000 chars), auto-convert to temp file attachment. Show indicator in editor area.
- [x] Add `ctrl+r` attachment delete mode — when attachments exist, `ctrl+r` toggles delete mode (highlight first attachment, press enter to remove).
- [x] Update `onSubmit` — include attachment contents in the submitted message alongside text.
- [x] Write tests for attachment model — add, remove, display indicators.
- [x] Write tests for paste detection — verify short paste passes through, verify long paste converts to attachment.
- [x] Run tests — must pass before Task 8

---

### Task 8: Final integration and verification

- [ ] Verify all features work together — v2 APIs, textarea, attachments, pills, custom Chroma
- [ ] Verify `sdk.UI` interface still works — Select, Confirm, Input through overlay stack
- [ ] Verify headless mode still works — tool results render as formatted text
- [ ] Run full test suite — all tests must pass
- [ ] Run `make lint` and fix any issues
- [ ] Run `make fix` for formatting

---

### Task 9: Update documentation

- [ ] Update `CLAUDE.md` — document Bubble Tea v2 migration, new import paths, textarea component
- [ ] Update `CLAUDE.md` — add new TUI components (attachments, pills, xchroma)
- [ ] Update `CLAUDE.md` — update component descriptions for v2 API changes

---

## Technical Details

### v1 → v2 Import Path Changes

| v1 | v2 |
|----|-----|
| `github.com/charmbracelet/bubbletea` | `charm.land/bubbletea/v2` |
| `github.com/charmbracelet/lipgloss` | `charm.land/lipgloss/v2` |
| `github.com/charmbracelet/bubbles` | `charm.land/bubbles/v2` |
| `github.com/charmbracelet/bubbles/spinner` | `charm.land/bubbles/v2/spinner` |
| `github.com/charmbracelet/bubbles/textarea` | `charm.land/bubbles/v2/textarea` |
| `github.com/charmbracelet/bubbles/key` | `charm.land/bubbles/v2/key` |

### v1 → v2 API Changes

| v1 | v2 |
|----|-----|
| `tea.KeyMsg` | `tea.KeyPressMsg` |
| `msg.String()` | `msg.Key.String()` |
| `msg.Type == tea.KeyCtrlC` | `key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c")))` or string comparison |
| `spinner.New()` | `spinner.New(spinner.WithStyle(...))` |
| `lipgloss.Place(w, h, pos, pos, lipgloss.WithWhitespaceChars(" "))` | `lipgloss.Place(w, h, pos, pos, " ")` |

### Textarea Integration (from crush reference)

```go
ta := textarea.New()
ta.SetStyles(textarea.Styles{
    Focused: textarea.StyleState{
        Base:    baseStyle,
        Text:    baseStyle,
        Prompt:  promptStyle,
    },
    Blurred: textarea.StyleState{
        Base:    blurredStyle,
        Text:    blurredStyle,
    },
})
ta.ShowLineNumbers = false
ta.CharLimit = -1
ta.SetVirtualCursor(false)
ta.DynamicHeight = true
ta.MinHeight = 3
ta.MaxHeight = 15
ta.Focus()
```

### Files Affected by Import Changes (38 files)

**bubbletea imports (22 files):** model.go, model_test.go, bridge.go, bridge_test.go, tui.go, ui_impl.go, ui_impl_test.go, overlays.go, keybindings.go, keybindings_test.go, commands.go, commands_test.go, sessions.go, models.go, models_test.go, integration_test.go, components/editor.go, components/editor_test.go, components/spinner.go, components/spinner_test.go, components/overlays/selector.go, components/overlays/stack.go

**lipgloss imports (13 files):** model.go, landing.go, components/chat.go, components/footer.go, components/spinner.go, components/editor.go, components/messages/user.go, components/messages/diff.go, components/messages/thinking.go, components/messages/tool.go, components/overlays/selector.go, components/overlays/confirm.go, components/overlays/input.go

**bubbles imports (3 files):** model.go, components/spinner.go, components/spinner_test.go

## Post-Completion

**Manual verification:**
- Visual testing of v2 rendering in terminal — verify no visual regressions
- Test textarea dynamic height with long multi-line inputs
- Test external editor integration (`ctrl+g`)
- Test file attachments with binary files and very large pastes
- Verify pills bar shows correct progress during multi-tool turns
- Verify diff rendering with Chroma highlighting
- Test with different terminal emulators (iTerm2, Alacritty, Kitty, Terminal.app)
- Performance: verify v2 rendering is not slower than v1

**Future work (not in scope):**
- @-mention file completions (reference: crush `internal/ui/completions/`)
- Structured tool result types and renderers
- Upgraded unified diff rendering with line numbers and syntax highlighting
