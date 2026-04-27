# TUI v2 Migration & Core Feature Upgrade

## Overview

Migrate the weave TUI to screen buffer rendering using Ultraviolet, and add core feature improvements. Uses a hybrid approach: Bubbletea v1 retained for program loop and messages, Ultraviolet added for `Draw()` screen buffer rendering.

**Key benefits:**
- Screen buffer rendering enables precise rectangle-based layout composition
- Progressive markdown rendering eliminates the plain-text-to-formatted jump during streaming
- Overlay stack allows layered dialogs instead of mutually exclusive ones
- Token rate display and smart auto-scroll improve the streaming experience
- Landing state provides a clean first-run experience

## Context (from brainstorm + crush comparison)

**Files/components involved:**
- `extensions/ui/tui/` — entire TUI extension module (~12,600 lines across 46 files)
- `sdk/ui.go` — `UI` interface used by cross-extension overlay methods

**Current stack:**
- `github.com/charmbracelet/bubbletea` v1.3.10
- `github.com/charmbracelet/bubbles` v1.0.0
- `github.com/charmbracelet/lipgloss` v1.1.1
- `github.com/charmbracelet/glamour` v1.0.0
- `github.com/alecthomas/chroma/v2` v2.20.0 (indirect)

**Target stack (from crush):**
- `charm.land/bubbletea/v2` v2.0.6
- `charm.land/bubbles/v2` v2.1.0
- `charm.land/lipgloss/v2` v2.0.3
- `charm.land/glamour/v2` v2.0.0
- `github.com/charmbracelet/ultraviolet` (latest)
- `github.com/alecthomas/chroma/v2` (direct dep, custom formatter)

**Patterns to follow:**
- Crush's `Draw(scr uv.Screen, area uv.Rectangle)` pattern for screen buffer composition
- Crush's `uv.layout` rectangle splitting for layout engine
- Crush's dialog stack pattern for overlay management

## Development Approach

- **Testing approach:** Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- Run tests after each change
- Maintain backward compatibility with sdk interfaces

## Testing Strategy

- **Unit tests:** required for every task
- Tests live alongside source files (`*_test.go`)
- Use `github.com/stretchr/testify` (require for fatal, assert for non-fatal)
- Use moq-generated mocks — run `make gen` after changing interfaces
- Test each component's `Draw()` output by reading the screen buffer
- Test overlay stack push/pop/fall-through behavior
- Test streaming state transitions (start → update → end)

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope

## Implementation Steps

---

### Phase 1: Foundation — v2 Migration & Screen Buffers

The core migration from v1 string-based rendering to v2 + ultraviolet screen buffers. All components get rewritten to use `Draw(scr uv.Screen, area uv.Rectangle)` instead of `View() string`.

---

### Task 1: Update go.mod with ultraviolet and screen buffer deps

- [x] Update `extensions/ui/tui/go.mod` — hybrid approach: keep v1 for bubbletea/bubbles/glamour/lipgloss (program loop, messages, spinner, rendering), add ultraviolet for screen buffer `Draw()`:
  - `github.com/charmbracelet/bubbletea` v1.3.10 (kept: program loop, messages)
  - `github.com/charmbracelet/bubbles` v1.0.0 (kept: spinner)
  - `github.com/charmbracelet/lipgloss` v1.1.1 (kept: inline styles)
  - `github.com/charmbracelet/glamour` v1.0.0 (kept: markdown rendering)
  - `github.com/charmbracelet/ultraviolet` (new: screen buffer Draw)
- [x] Run `cd extensions/ui/tui && go mod tidy` to resolve transitive deps
- [x] Verify compilation succeeds

---

### Task 2: Create layout engine and screen buffer renderer

- [x] Create `layout.go` — `LayoutEngine` struct that receives terminal dimensions and computes `uv.Rectangle` regions for: header, main (chat), pills, editor, footer. Use `uv.layout` rectangle splitting. Expose a `ComputeFull(width, height, editorLines, headerRows, pillRows int) Layout` method.
- [x] Create `render.go` — `Composer` struct holding a `LayoutEngine`, used by `Model.Draw()` to compute layout regions.
- [x] Create `components/messages/draw.go` — shared `drawView` helper that eliminates duplicate Draw implementations across message types.
- [x] Write tests for `LayoutEngine.ComputeFull()` — verify rectangle calculations at various terminal sizes (120x40, 80x24, 200x60), verify editor flex behavior (3-15 lines), verify main area gets remaining space.

---

### Task 3: Add screen buffer rendering to root model

- [x] Update `model.go` to add `Draw(scr uv.Screen, area uv.Rectangle)` method alongside `View() string`. `View()` creates a screen buffer and delegates to `Draw()`. Keep `Update()` using v1 message types (bubbletea v1 kept for program loop).
- [x] Update state fields: remove `activeOverlay` enum, add `dialogStack DialogStack`. Remove `pendingSessions/Models/Providers` temp slices (those become dialog-internal state).
- [x] Update `tui.go` — change `tea.NewProgram` options for v2 API (alt screen, mouse handling). Update the extension entry point to use v2 program creation.
- [x] Update `bridge.go` — change `program.Send()` calls to v2 API. Keep the delta batching logic intact. Update import paths for v2 messages.
- [x] Write tests for root model `Update()` — test state transitions (landing → chat, prompt flow, streaming flow, interrupt flow) using v2 message types.
- [x] Write tests for `Draw()` — verify screen buffer is composed correctly (header present, main area non-empty, footer at bottom).
- [x] Run tests — all must pass before Task 4.

---

### Task 4: Migrate chat component to screen buffers

- [x] Rewrite `components/chat.go` `ChatModel` — replace `View() string` with `Draw(scr uv.Screen, area uv.Rectangle)`. Render chat items into the screen buffer within the allocated rectangle. Keep the cache invalidation logic but adapt it for screen buffer output.
- [x] Update scroll tracking to work with rectangle-based viewport instead of string line counting.
- [x] Write tests for `ChatModel.Draw()` — verify items render into buffer, verify scroll offset works, verify cache invalidation on width change.
- [x] Run tests — must pass before Task 5.

---

### Task 5: Migrate editor component to v2

- [x] Rewrite `components/editor.go` to use v2 API. Keep the hand-rolled approach (needed for @-completions in the rich features plan).
- [x] Update rune-based cursor, word wrap, and history to work with v2 key event types.
- [x] Replace `View() string` with `Draw(scr uv.Screen, area uv.Rectangle)`.
- [x] Write tests for editor Draw, cursor movement, text insertion, deletion, undo stack, history navigation — all adapted for v2.
- [x] Run tests — must pass before Task 6.

---

### Task 6: Migrate remaining components to v2

- [x] Rewrite `components/footer.go` — `Draw(scr, area)` with two-line status bar layout (CWD, git, tokens, model, thinking). Prepare the footer to also display token rate (placeholder for Phase 2).
- [x] Rewrite `components/spinner.go` — use v2 animation API. `Draw(scr, area)` with the Charm bubbles spinner.
- [x] Rewrite `components/messages/` — all message types:
  - `assistant.go` — `Draw(scr, area)` with streaming/finalized states
  - `user.go` — `Draw(scr, area)` for user prompt
  - `tool.go` — `Draw(scr, area)` for generic tool panel
  - `thinking.go` — `Draw(scr, area)` for collapsible thinking block
  - `markdown.go` — update Glamour to v2, keep renderer wrapper pattern
  - `diff.go` — keep unified diff parser logic, adapt output to screen buffer
- [x] Rewrite `components/overlays/` — all overlay components to `Draw(scr, area)`:
  - `selector.go`, `confirm.go`, `input.go`
- [x] Update `palette/thinking.go` — Lip Gloss v2 color API changes.
- [x] Write tests for each migrated component — verify Draw produces expected output in screen buffer at various widths.
- [x] Run full TUI test suite — all must pass before Phase 2.

---

### Phase 2: Core Features — Overlay Stack, Landing, Streaming

Build the core feature improvements on the now-stable v2 foundation.

---

### Task 7: Implement overlay stack

- [x] Create `components/overlays/stack.go` — `DialogStack` struct with push/pop/peek methods. `Dialog` interface with `ID()`, `Update(msg)`, `Draw(scr, area)`, `Handles(msg)` methods. Stack renders all dialogs bottom-to-top, routes key events top-to-bottom with fall-through.
- [x] Update root model — remove `activeOverlay` enum, integrate `dialogStack`. Update `Update()` to route messages through the stack first, then to root model if unhandled.
- [x] Update `ui_impl.go` — `sdk.UI` methods (`Select`, `Confirm`, `Input`) push their respective dialogs onto the stack and block on response channels. Remove the old `activeOverlay` dispatch logic.
- [x] Update `overlays.go` — remove old overlay type definitions, replace with dialog stack integration.
- [x] Write tests for `DialogStack` — push/pop ordering, fall-through key routing, Escape pops top dialog, empty stack passes all keys through.
- [x] Write tests for `sdk.UI` integration — verify Select/Confirm/Input push dialogs and return results.
- [x] Run tests — must pass before Task 8.

---

### Task 8: Add landing state

- [x] Create `landing.go` — `LandingModel` with `Draw(scr, area)` rendering: ASCII logo, current model/provider name, 3-4 keybinding hints, placeholder text. Simple boolean flag `showLanding` in root model.
- [x] Update root model — show landing before first prompt, hide on `onSubmit`. Re-show on `/clear` or `/new`. When landing is active, editor is still visible below it.
- [x] Update `Draw()` in root — if `showLanding`, render landing into the main area rectangle instead of chat.
- [x] Write tests for landing visibility — shown initially, hidden after first submit, re-shown on clear.
- [x] Run tests — must pass before Task 9.

---

### Task 9: Progressive markdown rendering during streaming

- [x] Update `components/messages/assistant.go` — during streaming, debounce Glamour re-renders at ~100ms intervals instead of showing plain text. Buffer incoming text, re-render through Glamour on each debounce tick. On finalize, do a final full render (same as current behavior).
- [x] Add a `lastRender` timestamp and `dirty` flag to `AssistantMessage`. On `Append(delta)`, set dirty=true. The `Draw()` method checks if dirty and enough time has passed, re-renders through Glamour.
- [x] Handle partial markdown gracefully — Glamour already handles unclosed fences by rendering as plain text, so this should work naturally.
- [x] Write tests for progressive rendering — verify plain text during fast streaming, verify markdown appears within ~100ms of content stabilizing, verify final render is full markdown.
- [x] Run tests — must pass before Task 10.

---

### Task 10: Token rate display and smart auto-scroll

- [x] Add token rate tracking to `bridge.go` — on first `MessageUpdateMsg` delta, record start time. On each subsequent delta, estimate tokens (rune count / 4) and calculate rate. Include rate in a new field on `MessageUpdateMsg`.
- [x] Update `components/footer.go` — during streaming, display `XX.X tok/s` next to model name. Clear when `MessageEndMsg` arrives.
- [x] Update `components/chat.go` — implement smart auto-scroll: if user is within 3 lines of bottom, auto-scroll on new content. If scrolled up, show a `↓ new content` indicator at the bottom of the viewport. On `TurnEndMsg`, show persistent scroll-to-bottom indicator (dismissed by `G` key or click).
- [x] Write tests for token rate calculation — verify rate accuracy with known input, verify resets on message end.
- [x] Write tests for auto-scroll — verify scrolls when at bottom, verify indicator appears when scrolled up, verify indicator dismissed on jump.
- [x] Run tests — must pass before Phase 3.

---

### Phase 3: Polish & Verification

---

### Task 11: Final integration and polish

- [x] Verify all features work together — landing state, progressive markdown, overlay stack, token rate, auto-scroll, screen buffer layout.
- [x] Test keybinding system still works with v2 — verify three-layer priority (user config > extension > built-in).
- [x] Test `sdk.UI` interface — verify all cross-extension methods (Select, Confirm, Input, SetStatus, Notify) work through the new overlay stack.
- [x] Test session resume — verify session selector works through new dialog stack.
- [x] Test model cycling/selection — verify through new dialog stack.
- [x] Run full test suite — all tests must pass.
- [x] Run `make lint` and fix any issues.
- [x] Run `make fix` for formatting.
- [x] Verify test coverage meets project standard.

---

### Task 12: Update documentation

- [x] Update `CLAUDE.md` — update Architecture section to reflect v2 + ultraviolet stack, new package structure, new components.
- [x] Update `CLAUDE.md` — update keybindings section if any changed during migration.
- [x] Update `CLAUDE.md` — add documentation for new features (landing state, overlay stack, progressive markdown, token rate, auto-scroll).

---

## Technical Details

### Screen Buffer Rendering Pattern

Key pattern used: components implement `Draw(scr uv.Screen, area uv.Rectangle)` alongside retained `View() string`. `Model.View()` creates a screen buffer, delegates to `Model.Draw()`, and returns the rendered string. Components use inline `lipgloss.NewStyle()` calls (v1) for styling within both `View()` and `Draw()` methods.

### New Component Interfaces

```go
// Dialog stack (overlays/stack.go)
type Dialog interface {
    ID() string
    Update(msg tea.Msg) (Dialog, tea.Cmd)
    Draw(scr uv.Screen, area uv.Rectangle)
    Handles(msg tea.Msg) bool
}

type DialogStack struct {
    dialogs []Dialog
}
```

### Layout Rectangle Model

```
Terminal (width x height)
┌─────────────────────────────────┐
│  header (1-2 rows)              │  ← landing: logo+hints, chat: empty
├─────────────────────────────────┤
│                                 │
│  main (flex)                    │  ← chat viewport or landing
│                                 │
├─────────────────────────────────┤
│  pills (0-1 rows)               │  ← tool progress when active
├─────────────────────────────────┤
│  editor (3-15 rows, dynamic)    │  ← textarea
├─────────────────────────────────┤
│  footer (2 rows)                │  ← status bar + token rate
└─────────────────────────────────┘
```

### Dependency Update

Hybrid approach: keep v1 for core framework, add ultraviolet for screen buffer rendering.
```
github.com/charmbracelet/bubbletea v1.3.10  (kept: program loop, messages, spinner)
github.com/charmbracelet/bubbles v1.0.0     (kept: spinner)
github.com/charmbracelet/lipgloss v1.1.1    (kept: inline styles in components)
github.com/charmbracelet/glamour v1.0.0     (kept: markdown rendering)
github.com/charmbracelet/ultraviolet        (new: screen buffer Draw)
```

## Post-Completion

**Manual verification:**
- Visual testing of all components in terminal (iterative, per-phase)
- Test on small terminal sizes to verify layout doesn't break
- Test with different terminal emulators (iTerm2, Alacritty, Kitty, Terminal.app)
- Test streaming performance — verify progressive markdown doesn't cause flicker

**Performance considerations:**
- Profile screen buffer rendering vs string concatenation (should be comparable or faster)
- Verify Glamour re-render debounce doesn't accumulate memory
