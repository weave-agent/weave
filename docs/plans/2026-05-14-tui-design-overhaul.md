# TUI Design Overhaul

## Overview
Implement all recommendations from the TUI frontend design audit to transform the interface from a competent but forgettable default Bubble Tea app into a distinctive, cohesive, and memorable developer tool. The overhaul addresses: missing visual identity, lack of a centralized theme system, poor information hierarchy in messages and footer, invisible thinking blocks, generic overlays, and absent motion design.

Key benefits:
- Centralized `Theme` struct eliminates 20+ scattered hardcoded ANSI color codes
- Styled user messages with symbol prefix and role indicators create clear conversation hierarchy
- Redesigned thinking blocks give reasoning content the visual prominence it deserves
- Improved footer hierarchy surfaces the most important information (active model)
- Pill-shaped attachments and status messages replace debug-output aesthetics
- Subtle animations add polish without distraction

## Context (from discovery)
- **Files/components involved**: 15+ files with hardcoded colors across `extensions/ui/tui/`:
  - `components/attachments/attachments.go` (`63`, `196`, `240`)
  - `components/chat.go` (`220`)
  - `components/completion.go` (`240`, `99`, `15`, `245`)
  - `components/editor.go` (`63`, `240`)
  - `components/footer.go` (`245`, `196`, `220`, `82`)
  - `components/messages/diff.go` (`2`, `1`, `6`, `5`)
  - `components/messages/tool.go` (`1`, `8`, `2`)
  - `components/overlays/confirm.go`, `input.go`, `selector.go` (`63`, `15`, `243`, `252`, `99`)
  - `components/spinner.go` (`99`)
  - `extensions/diff-viewer/diff_viewer.go` (`6`, `5`, `2`, `1`)
  - `landing.go` (`63`, `242`, `243`)
  - `model.go` (`242`, `245`)
  - `palette/thinking.go` (`246`, `67`, `99`, `139`, `177`, `240`)
- **Related patterns found**: `components/messages/draw.go` provides a shared `drawView()` helper; `components/editor.go` has a `borderStyle()` helper; `palette/thinking.go` is the only existing palette file
- **Dependencies identified**: `charm.land/lipgloss/v2` (all styling); `github.com/alecthomas/chroma/v2` (syntax highlighting); `github.com/charmbracelet/glamour` (markdown); `github.com/charmbracelet/ultraviolet` (screen buffers)
- **Test files affected**: `styles_test.go` directly asserts color values (`63`, `15`, `82`, `1`, `2`) and will need updates. Most other tests strip ANSI or check text content only.

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - tests are not optional - they are a required part of the checklist
  - write unit tests for new functions/methods
  - write unit tests for modified functions/methods
  - add new test cases for new code paths
  - update existing test cases if behavior changes
  - tests cover both success and error scenarios
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Testing Strategy
- **Unit tests**: required for every task (see Development Approach above)
- **E2E tests**: Not applicable — this is a terminal UI with no browser-based e2e suite
- Visual rendering tests in `styles_test.go` must be updated when color values change
- Component tests for `chat.go`, `footer.go`, `thinking.go`, `tool.go`, `attachments.go` should verify rendered output content (most already strip ANSI)

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope
- Keep plan in sync with actual work done

## Implementation Steps

### Task 1: Create centralized Theme system
- [x] Create `extensions/ui/tui/palette/theme.go` with `Theme` struct containing semantic color slots: `Primary`, `PrimaryDim`, `PrimaryBright`, `Success`, `Error`, `Warning`, `Muted`, `MutedBright`, `Border`, `BorderFocused`, `BackgroundTint`, `Foreground`, `ForegroundBright`
- [x] Define default dark theme with a cohesive purple-blue centered palette (primary `63`, success `84`, error `204`, warning `221`, muted `245`)
- [x] Add `DefaultTheme()` constructor and make it accessible to all components
- [x] Refactor `palette/thinking.go` to use `Theme` primary family instead of arbitrary hue shifts
- [x] Refactor `components/editor.go`: replace hardcoded `63`/`240` with theme colors
- [x] Refactor `components/footer.go`: replace hardcoded `245`/`196`/`220`/`82` with theme semantic colors
- [x] Refactor `components/chat.go`: replace hardcoded `220` with theme warning color
- [x] Refactor `landing.go`: replace hardcoded `63`/`242`/`243` with theme colors
- [x] Refactor `model.go`: replace hardcoded `242`/`245` with theme colors
- [x] Refactor `components/spinner.go`: replace hardcoded `99` with theme primary
- [x] Update `styles_test.go` to use theme colors instead of hardcoded values
- [x] Write tests for `Theme` struct and `DefaultTheme()`
- [x] Run all TUI tests — must pass before next task

### Task 2: Redesign chat messages and conversation flow
- [x] Add 1 blank line between chat items in `components/chat.go` (`View()` and `Draw()`)
- [x] Style `UserMessage` in `components/messages/user.go`: add left border bar in primary color, `❯` symbol prefix in primary, muted content color
- [x] Add message role indicator for assistant messages (subtle `assistant` header or icon prefix in `components/messages/assistant.go` via the renderer)
- [x] Update `components/chat.go` scroll indicator: render as a styled pill with background instead of plain yellow text
- [x] Update hints banner in `model.go`: render with `BackgroundTint` background for visual separation
- [x] Write tests for user message styling (verify border and label presence in rendered output)
- [x] Write tests for chat spacing (verify blank lines between items)
- [x] Write tests for scroll indicator rendering
- [x] Run all TUI tests — must pass before next task

### Task 3: Redesign thinking blocks and tool panels
- [x] Redesign `ThinkingBlock` in `components/messages/thinking.go`: replace `.Faint(true)` with a styled header including a lightbulb icon and `BackgroundTint` background; expanded state uses indented content with left border
- [x] Update `FormatThinkingLabel()` to produce a more prominent collapsed label
- [x] Redesign `ToolPanel` in `components/messages/tool.go`: add subtle background tint per state (`22`-like for success, `52`-like for error, `235` for pending); improve header spacing and typography
- [x] Keep emoji state indicators (⏳/✓/✗) but improve their visual context
- [x] Update `components/messages/diff.go` to use theme colors for diff line kinds
- [x] Update `extensions/diff-viewer/diff_viewer.go` to use theme colors
- [x] Write tests for thinking block rendering (collapsed and expanded states)
- [x] Write tests for tool panel state styling
- [x] Write tests for diff renderer color usage
- [x] Run all TUI tests — must pass before next task

### Task 4: Redesign footer, landing screen, and editor
- [x] Restructure `FooterModel.renderLine2()` in `components/footer.go`: bold model name in primary color, thinking level as a subtle pill, token counts and cost muted, context percentage with theme threshold colors
- [x] Group footer information visually — model info on the right, stats on the left
- [x] Redesign `LandingModel` in `landing.go`: better vertical composition with a subtle horizontal rule (`Border` color) between logo and info; move placeholder into the editor textarea
- [x] Add `Placeholder` text to editor in `components/editor.go` ("Type a message...")
- [x] Improve blurred editor border distinction — use `Border` instead of `BorderFocused` dimmed
- [x] Write tests for footer rendering (verify model name prominence, separator grouping)
- [x] Write tests for landing page composition
- [x] Write tests for editor placeholder
- [x] Run all TUI tests — must pass before next task

### Task 5: Redesign attachments, overlays, and completion popup
- [x] Redesign `attachments.Model.Draw()` in `components/attachments/attachments.go`: replace bracketed text with pill-shaped chips using `BackgroundTint` background and rounded appearance; delete mode uses `Error` color with `×` indicator
- [x] Unify overlay border styles in `components/overlays/confirm.go`, `input.go`, `selector.go`: all use `RoundedBorder` consistently with `BorderFocused` color
- [x] Differentiate overlay types visually: selector uses primary accent, confirm uses warning accent for destructive actions
- [x] Update `components/completion.go`: use `Border` for popup border, improve selected item contrast
- [x] Write tests for attachment pill rendering
- [x] Write tests for overlay styling
- [x] Write tests for completion popup rendering
- [x] Run all TUI tests — must pass before next task

### Task 6: Add motion and animation polish
- [ ] Implement message fade-in in `components/messages/assistant.go`: first 2-3 frames render at progressively brighter foreground colors to create a subtle materializing effect; use a frame counter or timestamp on the message
- [ ] Add status message entrance animation in `model.go`: render at muted color for 1 frame, then full brightness — a quick "pop in"
- [ ] Implement dialog backdrop dimming: when `dialogStack` is non-empty, render underlying UI at muted foreground in `model.go` `Draw()`
- [ ] Add spinner color pulse: alternate between `Primary` and `PrimaryBright` every few frames
- [ ] Write tests for fade-in frame progression
- [ ] Write tests for backdrop dimming when dialogs are open
- [ ] Write tests for spinner color alternation
- [ ] Run all TUI tests — must pass before next task

### Task 7: Verify acceptance criteria and finalize
- [ ] Verify all audit recommendations are addressed:
  - [ ] Centralized theme system exists
  - [ ] User messages have visual styling
  - [ ] Message spacing added
  - [ ] Thinking blocks are visually prominent
  - [ ] Footer has information hierarchy
  - [ ] Landing screen is better composed
  - [x] Attachments use pill shapes
  - [x] Tool panels have state-specific backgrounds
  - [x] Overlays are visually differentiated
  - [ ] Motion/animation added
- [ ] Run full test suite (`make test-all`)
- [ ] Run linter (`make lint`) — all issues fixed
- [ ] Update any affected documentation

## Technical Details

### Theme struct design
```go
type Theme struct {
    Primary       string // 63 — main accent
    PrimaryDim    string // 60 — subdued accent
    PrimaryBright string // 69 — bright accent
    Success       string // 84 — mint green
    Error         string // 204 — rose
    Warning       string // 221 — amber
    Muted         string // 245 — gray
    MutedBright   string // 252 — light gray
    Border        string // 240 — borders
    BorderFocused string // 63 — focused borders
    BackgroundTint string // 234 — subtle panel backgrounds
    Foreground    string // 15 — main text
    ForegroundBright string // 15 — bright text (same on dark)
}
```

### Thinking level color mapping (revised)
Using brightness/intensity of the primary family instead of arbitrary hue shifts:
- Off: `240` (gray)
- Minimal: `60` (dim purple)
- Low: `63` (primary)
- Medium: `69` (bright purple)
- High: `141` (light purple)
- XHigh: `177` (pink-white)

### Message fade-in approach
Add a `createdAt time.Time` field to `AssistantMessage`. In `View()`, if `time.Since(createdAt) < 150ms`, render through a temporary style that maps elapsed time to foreground color brightness (`240` → `252` → `15`). This is subtle but perceptible.

## Post-Completion

**Manual verification**:
- Open weave TUI and verify visual appearance of each component
- Check that all colors render correctly in the user's terminal (iTerm2/ghostty/Terminal.app)
- Verify thinking level cycling updates editor border correctly
- Test dialog open/close with backdrop dimming
- Paste a large file to verify attachment pill rendering

**Future work (not in this plan)**:
- Light theme support (`WEAVE_THEME=light` env var)
- Terminal background color auto-detection
- User-customizable theme via settings.json
- More elaborate animations (slide-in dialogs, typewriter effect)
