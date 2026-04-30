# Editor Completions

## Overview
Add an inline completion popup to the TUI editor area with two triggers:
- **`/`** at line start shows slash command completions (filtered by typed text)
- **`@`** after whitespace shows file path completions (relative to CWD, with fuzzy filtering)

Tab/Enter selects, arrows navigate, Esc dismisses. Popup renders above the editor cursor using `uv.Screen`, following Crush's approach.

## Context
- **Editor**: `extensions/ui/tui/components/editor.go` — wraps bubbles/v2 textarea, no completion support
- **Command registry**: `extensions/ui/tui/commands.go` — has `Names()`, `Lookup()` with name + description
- **Reference**: Crush (`/Users/andrey/Projects/crush/internal/ui/completions/`) — mature completion system with FilterableList, fuzzy matching, cursor-relative positioning
- **Key routing**: `extensions/ui/tui/model.go` — Model.Update dispatches keys: dialog stack → binding resolver → editor. Completion interception goes between binding resolver and editor forward.
- **Rendering**: `extensions/ui/tui/model.go` Draw — computes layout regions via LayoutEngine, draws components into `uv.Screen`. Completion popup draws after editor, positioned at cursor.
- **Layout**: `extensions/ui/tui/layout.go` — LayoutEngine provides `lt.Editor` rectangle for positioning.

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- Every task includes new/updated tests
- All tests must pass before starting next task
- Maintain backward compatibility

## Testing Strategy
- **Unit tests**: required for every task
- No E2E tests in this project — TUI components tested via unit tests with tea.Program where needed

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## Implementation Steps

### Task 1: Create CompletionModel component
- [x] create `extensions/ui/tui/components/completion.go` with `CompletionModel` struct (visible, items, filtered, cursor, filter, width, maxVisible, kind)
- [x] define `CompletionKind` enum: `CompletionNone`, `CompletionSlash`, `CompletionFile`
- [x] define `CompletionItem` struct: Label, Description, Value string
- [x] implement `NewCompletionModel()`, `Show()`, `Hide()`, `Visible()`, `SetFilter()` (case-insensitive prefix filter on Label)
- [x] implement `CursorUp()`, `CursorDown()` (wrap-around), `SelectedItem()` methods
- [x] implement `View() string` — bordered box, selected item bold on purple bg, descriptions dimmed, max 8 visible items
- [x] implement `Draw(scr uv.Screen, area uv.Rectangle)` — render into screen buffer
- [x] write tests for filter logic, cursor navigation, Show/Hide, empty items edge case
- [x] run `cd extensions/ui/tui && go test ./components/...` — must pass before task 2

### Task 2: Add file path completion provider
- [x] create `extensions/ui/tui/components/path_completion.go` with `PathCompletions(baseDir, prefix string) []CompletionItem`
- [x] resolve prefix against baseDir, read parent directory, filter by final path component
- [x] directories get trailing `/` in Value, sorted alphabetically
- [x] write tests for: no prefix (list root files), partial match, nested paths, nonexistent dir
- [x] run `cd extensions/ui/tui && go test ./components/...` — must pass before task 3

### Task 3: Extend CommandInfo with file-accepting flag
- [x] add `AcceptsFiles bool` field to `CommandInfo` in `extensions/ui/tui/commands.go`
- [x] update `register()` to accept the new field (no current commands need it)
- [x] no existing code breaks — field defaults to false
- [x] write test verifying AcceptsFiles defaults to false for existing commands
- [x] run `cd extensions/ui/tui && go test ./...` — must pass before task 4

### Task 4: Add completion state to EditorModel
- [ ] add `completion CompletionModel` field to `EditorModel` in `extensions/ui/tui/components/editor.go`
- [ ] add `Completion() CompletionModel` getter
- [ ] add `ShowCompletion(kind CompletionKind, items []CompletionItem, filter string)` method
- [ ] add `HideCompletion()` method
- [ ] add `CompletionActive() bool` convenience method
- [ ] in `handleKey()`: when completion is visible, intercept Tab (CursorDown), Up (CursorUp), Down (CursorDown), Enter (apply + submit), Esc (Hide). Return `handled=true` for intercepted keys.
- [ ] implement `applyCompletion()` — replace trigger portion of textarea value with selected item Value, reposition cursor, hide popup
- [ ] on history navigation (Up/Down when navigating history), auto-hide completion popup
- [ ] write tests for key interception when completion visible vs not visible
- [ ] run `cd extensions/ui/tui && go test ./components/...` — must pass before task 5

### Task 5: Wire completion triggers into Model.Update
- [ ] in `extensions/ui/tui/model.go` `Update()` `tea.KeyPressMsg` case: after binding resolution, before editor forward, add completion key interception block
- [ ] add `handleCompletionKey(msg tea.KeyPressMsg)` method on Model — delegates to editor when completion active, returns (handled, model, cmd)
- [ ] add `refreshEditorCompletion()` method on Model that reads editor value and determines context:
  - Line 0 starts with `/`, no space yet → CompletionSlash with `m.commands.Names()` as items, filter = text after `/`
  - Line 0 starts with `/command ` where command has AcceptsFiles → CompletionFile with PathCompletions, filter = text after space
  - Detects `@` after whitespace → CompletionFile with PathCompletions, filter = text after `@`
  - Otherwise → HideCompletion
- [ ] after every editor key forward, call `refreshEditorCompletion()` to update popup state
- [ ] handle `slashCommandsUpdatedMsg` to refresh cached command names (if message exists)
- [ ] write tests for refreshEditorCompletion with various editor value patterns (empty, `/he`, `/help `, `text @`, plain text)
- [ ] run `cd extensions/ui/tui && go test ./...` — must pass before task 6

### Task 6: Render completion popup in Model.Draw
- [ ] in `extensions/ui/tui/model.go` `Draw()`: after editor rendering, before footer, add completion popup rendering
- [ ] compute popup position: above cursor in editor region (match Crush's approach — render above, not below)
- [ ] clamp popup to screen bounds (don't overflow left/top)
- [ ] popup width = `min(50, editorWidth)`, height = `min(filteredCount, maxVisible)`
- [ ] if not enough space above editor, render below instead
- [ ] write test verifying Draw doesn't panic with completion visible
- [ ] run `cd extensions/ui/tui && go test ./...` — must pass before task 7

### Task 7: Verify acceptance criteria
- [ ] run `make lint` from project root — all issues fixed
- [ ] run `make test` from project root — all pass
- [ ] run `cd extensions/ui/tui && go test ./...` — all pass
- [ ] manual build and test: type `/` → popup appears with all commands
- [ ] manual test: type `/he` → filters to `/help`
- [ ] manual test: Tab/Enter selects completion, inserts into editor
- [ ] manual test: Esc dismisses popup
- [ ] manual test: type `@` after whitespace → file completions appear
- [ ] manual test: type `@sr` → filters files matching prefix
- [ ] manual test: plain text without `/` or `@` → no popup

## Technical Details

**Completion triggers:**
- `/` at position 0 on line 0 → slash commands
- `@` after whitespace or at position 0 → file paths
- `/command ` for AcceptsFiles commands → file paths

**Key routing order when completion is active:**
1. Dialog stack (unchanged)
2. Ctrl+C / Escape handlers (unchanged)
3. Binding resolver (unchanged)
4. **Completion key interception** (NEW — Tab, Up, Down, Enter, Esc)
5. Editor forward (unchanged) + refreshEditorCompletion after

**Popup positioning (Crush approach):**
- Calculate cursor position relative to editor region
- Render popup above cursor (y = cursorY - popupHeight)
- If not enough space above, render below cursor instead
- Clamp x to not exceed screen width

**Text replacement on selection:**
- Slash: replace from `/` to cursor with `item.Value` (e.g., `/help `)
- File (@ trigger): replace from `@` to cursor with `item.Value`
- Path (after command): replace from last space to cursor with `item.Value`

## Post-Completion

**Manual verification:**
- Full interactive testing of both trigger types
- Verify no regressions in existing editor behavior (history nav, submit, keybindings)
- Test edge cases: multiline content, rapid typing, empty directory for file completions
- Verify popup doesn't flicker or cause layout jumps

**Future enhancements (not in scope):**
- Fuzzy matching (Crush uses `sahilm/fuzzy` library) — currently prefix-only
- MCP resource completions
- Match highlighting in completion items
- Configurable completion trigger characters
