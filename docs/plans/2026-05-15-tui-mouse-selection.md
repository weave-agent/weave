# TUI Mouse Text Selection

## Overview

Implement in-app mouse text selection for the weave TUI chat. Currently, enabling `MouseModeCellMotion` for wheel scrolling broke native terminal text selection (the terminal sends all mouse events to the application). This feature provides click-and-drag text selection within the chat viewport, with auto-copy to clipboard on mouse release and a manual copy keybinding.

## Context

- **Files/components involved:**
  - `extensions/ui/tui/components/chat.go` — ChatModel (selection state, highlight rendering, text extraction)
  - `extensions/ui/tui/components/chat_test.go` — unit tests for selection logic
  - `extensions/ui/tui/model.go` — mouse event dispatch, layout helpers, copy command
  - `extensions/ui/tui/model_test.go` — integration tests for mouse selection flow
  - `extensions/ui/tui/keybindings.go` — copy action definition
  - `extensions/ui/tui/layout.go` — may need exported helper for chat area bounds
  - `extensions/ui/tui/go.mod` — add clipboard dependency

- **Related patterns found:**
  - Chat renders items to strings via `View(width)`, caches as `[]string` lines, draws via `uv.NewStyledString`
  - Mouse wheel already handled (`tea.MouseWheelMsg`) for scrolling
  - `uv.Screen` supports `CellAt(x,y)` and cell style mutation (`cell.Style.Attrs |= uv.AttrReverse`)
  - No clipboard integration currently exists

- **Dependencies identified:**
  - `github.com/atotto/clipboard` (already indirect via glamour, needs direct import)

## Development Approach

- **Testing approach:** Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change

## Testing Strategy

- **Unit tests:** required for every task
  - Selection state (normalize, clear, query)
  - Highlight rendering (line-level selection spans)
  - Text extraction (ANSI-aware, multi-line)
  - Mouse coordinate mapping (chat area, content positions)
- **Integration tests:** model-level mouse event handling
  - Click in chat area starts selection
  - Click outside chat area ignored
  - Drag extends selection
  - Key press clears selection

## Implementation Steps

### Task 1: Add selection state to ChatModel
- [x] Add selection fields to `ChatModel` (`selActive`, `selStartLine`, `selStartCol`, `selEndLine`, `selEndCol`, `mouseDown`)
- [x] Add methods: `StartSelection(line, col)`, `ExtendSelection(line, col)`, `EndSelection()`, `ClearSelection()`, `HasSelection()`, `MouseDown()`
- [x] Add `selectionForLine(globalLine)` helper returning start/end cols for a line
- [x] Add `lineToItem(contentLine)` helper mapping global content line to item index and line within item
- [x] write tests for selection state methods (normalize, clear, query)
- [x] write tests for `lineToItem` mapping
- [x] run tests — must pass before next task

### Task 2: Render selection highlight in ChatModel.Draw()
- [x] Modify `ChatModel.Draw()` to apply `uv.AttrReverse` to cells in selection range after drawing each line
- [x] Ensure highlight clips correctly at rectangle bounds
- [x] Handle case where selection extends beyond visible area
- [x] write tests for highlight rendering (selection within visible area, partially visible, empty selection)
- [x] run tests — must pass before next task

### Task 3: Extract selected text from ChatModel
- [x] Add `ExtractSelection() string` method that renders each selected line into a temp screen buffer and reads cell contents
- [x] Handle multi-line selection with proper newline insertion
- [x] Strip trailing whitespace from extracted text
- [x] write tests for text extraction (single line, multi-line, ANSI sequences, wide characters)
- [x] run tests — must pass before next task

### Task 4: Handle mouse events in model.Update()
- [x] Change `v.MouseMode = tea.MouseModeCellMotion` to `tea.MouseModeAllMotion` in `View()`
- [x] Add `tea.MouseClickMsg` case: start selection if in chat area and no dialog is open
- [x] Add `tea.MouseMotionMsg` case: extend selection if mouse is down and in chat area
- [x] Add `tea.MouseReleaseMsg` case: end selection, trigger auto-copy if selection exists
- [x] Add layout helpers: `chatArea() uv.Rectangle`, `chatContentPos(x, y, area) (line, col)`, `pointInArea(x, y, area) bool`
- [x] Clear selection on any key press (before existing key handling)
- [x] write tests for mouse click/drag/release handling
- [x] write tests for layout helpers
- [x] run tests — must pass before next task

### Task 5: Add clipboard integration and copy keybinding
- [x] Add `github.com/atotto/clipboard` as direct dependency
- [x] Add `ActionCopySelection` to `keybindings.go` with default `ctrl+shift+c`
- [x] Add `copySelectionCmd() tea.Cmd` using dual strategy (`tea.SetClipboard` + `clipboard.WriteAll`)
- [x] Add `dispatchBinding` handler for `ActionCopySelection`
- [x] Show typed notification on successful copy
- [x] write tests for copy keybinding dispatch
- [x] run tests — must pass before next task

### Task 6: Verify acceptance criteria
- [x] verify click-and-drag in chat area highlights text with inverted colors (manual test — skipped, not automatable)
- [x] verify click outside chat area does not start selection (covered by TestModel_MouseClick_OutsideChatArea_Ignored)
- [x] verify mouse release copies text to clipboard (covered by TestModel_MouseRelease_WithSelection_Copies)
- [x] verify `ctrl+shift+c` copies current selection (covered by TestModel_DispatchBinding_CopySelection)
- [x] verify key press clears selection (covered by TestModel_KeyPress_ClearsSelection)
- [x] verify selection clears on new message (added TestModel_MessageStart_ClearsSelection + impl in model.go)
- [x] run full test suite (unit tests) — all pass except 2 pre-existing failures in commands_test.go
- [x] run linter — 0 issues in modified files; 7 pre-existing issues in unrelated files
- [x] verify test coverage meets project standard — components package at 90.3%

### Task 7: Final cleanup and documentation
- [x] review all changes for code quality
- [x] ensure no debug prints or commented-out code remain
- [x] run `make test` from project root (all pass except 2 pre-existing failures in commands_test.go)
- [x] run `make lint` from project root (0 issues in modified files; 7 pre-existing issues in unrelated files)

## Technical Details

### Selection coordinate system

- **Global content line**: 0-indexed line across all rendered chat items including blank separators
- **Display column**: 0-indexed visual column within a line, accounting for ANSI sequences and wide characters
- **Normalization**: `StartSelection` stores raw start; `ExtendSelection` updates raw end; `selectionForLine` normalizes so start <= end

### Highlight rendering algorithm

During `ChatModel.Draw()`, for each visible line at index `i`:
1. Draw the line string normally via `uv.NewStyledString(line).Draw(scr, lineRect)`
2. Compute `globalLine = m.scroll + i`
3. If `sel := m.selectionForLine(globalLine)` is non-nil:
   - For `x` from `area.Min.X + sel.startCol` to `area.Min.X + sel.endCol`:
     - `cell := scr.CellAt(x, area.Min.Y+i)`
     - If cell != nil: `cell.Style.Attrs |= uv.AttrReverse`

### Text extraction algorithm

For each line in the selection range:
1. Get the rendered line string from cache
2. Create a 1-row `uv.ScreenBuffer` with width = line display width
3. Draw the line into the buffer
4. Read cells from `startCol` to `endCol`, concatenating `cell.Content`
5. Join lines with `\n`

### Mouse event flow

```
MouseClickMsg (left button) + in chat area
  → StartSelection(contentLine, col)
  → mouseDown = true

MouseMotionMsg (left button held) + in chat area
  → ExtendSelection(contentLine, col)

MouseReleaseMsg
  → EndSelection()
  → mouseDown = false
  → if HasSelection() → copySelectionCmd()

KeyPressMsg (any key)
  → ClearSelection()
  → proceed to normal key handling
```

## Post-Completion

**Manual verification:**
- Test in iTerm2: click-drag to select, paste elsewhere
- Test in Terminal.app: verify Shift+click still works for native selection bypass
- Test with multi-line assistant messages
- Test with tool panels and thinking blocks
- Test scrolling while selecting (edge case: drag beyond viewport)

**Edge cases to verify:**
- Empty chat (no items)
- Selection partially scrolled out of view
- Rapid click-drag-release (spam)
- Copy when nothing selected
- Selection across different message types (user + assistant + tool)
