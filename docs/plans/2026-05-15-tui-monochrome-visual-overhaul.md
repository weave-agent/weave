# TUI Monochrome Visual Overhaul

## Overview

A complete visual redesign of the weave terminal UI, moving from a generic purple-centered palette to a distinctive monochrome grayscale base with a dynamic activity-state accent color. This addresses all findings from the frontend design audit.

**Key changes:**
- Monochrome grayscale base (foreground, muted, borders, backgrounds)
- Dynamic accent color driven by agent activity state (idle/streaming/tool-running/error)
- Tool panels restyled as bordered cards instead of full-line background bands
- Chat messages gain left-edge role bars for stronger visual hierarchy
- Footer rebalanced with hidden zero-values and clearer information priority
- Blank chat separators replaced with subtle dot dividers
- Editor border pulse animation when agent is actively working
- Thinking level borders shifted to grayscale temperature variations

## Context (from discovery)

- **Files/components involved:** 39 Go files under `extensions/ui/tui/` reference `palette` package; 66 call sites to `palette.DefaultTheme()`; 40 test files
- **Related patterns found:** Components call `palette.DefaultTheme()` directly at render time; theme is not threaded through Draw()/View() methods; `ThemeInfo` in `sdk/ui.go` bridges to extensions via snapshot copy
- **Dependencies identified:** `charm.land/lipgloss/v2` for styling, `github.com/charmbracelet/ultraviolet` for screen buffer rendering, Bubble Tea v2 for message loop
- **Key gotchas:** Editor textarea styles set once at construction; diff renderer caches styles at creation; spinner color pulse is hardcoded; thinking colors ("141", "177") leak outside Theme struct

## Development Approach

- **Testing approach:** Regular (code first, then update tests)
- Make all visual changes in a single branch for cohesive result
- Complete each task fully before moving to the next
- Make small, focused changes within each task
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
- Maintain backward compatibility where possible

## Testing Strategy

- **Unit tests:** Required for every task (see Development Approach above)
- Tests use string assertions against rendered output (no golden files)
- Expected test updates: color code changes (e.g., "63" -> "250"), width changes from new prefixes, removed/added ANSI sequences
- Run `cd extensions/ui/tui && go test ./...` after each task
- Also run `make test` from root to catch cross-module impacts

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope
- Keep plan in sync with actual work done

## Implementation Steps

### Task 1: Rebuild theme system with state-aware accent

- [x] Update `palette/theme.go`: redefine Theme struct with grayscale base + Accent/AccentDim/AccentBright fields, remove Primary/PrimaryDim/PrimaryBright
- [x] Create `palette/state.go`: State enum (Idle, Streaming, ToolRunning, Error), AccentForState() helper
- [x] Update `palette/thinking.go`: grayscale thinking border mapping (240-250 range), remove hardcoded "141"/"177"
- [x] Update `palette/theme_test.go`: test new default color values
- [x] Update `palette/thinking_test.go`: test grayscale thinking border colors
- [x] Run tests: `cd extensions/ui/tui && go test ./palette/...` — must pass

### Task 2: Restyle message renderers

- [x] Update `messages/tool.go`: replace full-line BackgroundTint fills with bordered card style (rounded corners, accent-colored top-left), add flash timer for state transitions
- [x] Update `messages/assistant.go`: remove purple fade-in, use ForegroundDim -> Foreground, keep role indicator
- [x] Update `messages/user.go`: replace `❯` prefix with left-edge bar (`▐` in AccentDim), content after 1-space gap
- [x] Update `messages/thinking.go`: left border bar (`░` in Muted), content in ForegroundDim
- [x] Update `messages/diff.go`: update color references to new theme fields
- [x] Update `messages/notification.go`: update color references to new theme fields
- [x] Write/update tests for all message renderers
- [x] Run tests: `cd extensions/ui/tui && go test ./components/messages/...` — must pass

### Task 3: Restyle chat chrome and overlays

- [x] Update `components/chat.go`: replace blank separator lines with dot divider row (`·` in Muted)
- [x] Update `components/footer.go`: rebalance layout (CWD left, model name left with accent, stats right), hide zero values, replace `│` with `·`
- [x] Update `components/spinner.go`: use Accent instead of Primary for spinner color pulse
- [x] Update `components/editor.go`: update default border color to Accent, update tests
- [x] Update `landing.go`: use Accent for name style
- [x] Update `panel_tray.go`: update color references to new Accent theme fields
- [x] Update overlay components (`components/overlays/*.go`): update color references to Accent
- [x] Write/update tests for all chrome components
- [x] Run tests: `cd extensions/ui/tui && go test ./components/...` — must pass

### Task 4: Wire up activity state and agent pulse

- [x] Update `bridge.go`: publish `AgentStateChangeMsg` events based on bus events (turn_start -> Streaming, tool_result while pending -> ToolRunning, turn_end -> Idle, agent.end error -> Error)
- [x] Update `model.go`: handle `AgentStateChangeMsg`, update `m.theme.Accent`, trigger redraw; update `drawPills` to use accent; update header/status rendering
- [x] Update `components/editor.go`: implement pulse animation in Draw() based on pulse position and accent state
- [x] Update `tui_ext_api.go`: update `ThemeDef` and `toPaletteTheme()` to include new accent fields; update `Theme()` to return accent values
- [x] Update `sdk/ui.go`: update `ThemeInfo` struct with new accent fields
- [x] Write/update tests for state wiring and pulse animation
- [x] Run tests: `cd extensions/ui/tui && go test ./...` — must pass

### Task 5: Cross-module updates and root tests

- [x] Update `extensions/diff-viewer/diff_viewer.go` if it references old theme fields (already uses new Accent/AccentBright fields — no changes needed)
- [x] Run root tests: `make test` — must pass
- [x] Run linter: `make lint` — all issues fixed
- [x] Fix any cross-module test failures (none found)

### Task 6: Verify acceptance criteria

- [ ] Verify all audit recommendations are implemented
- [ ] Run full test suite: `make test`
- [ ] Run linter: `make lint`
- [ ] Manual terminal test: `go run cmd/weave/main.go`, verify landing screen, streaming, tool output, footer
- [ ] Verify test coverage meets project standard

## Technical Details

### New Theme Struct

```go
type Theme struct {
    // Base grayscale
    Foreground       string // "250" — primary text
    ForegroundDim    string // "245" — secondary text
    Muted            string // "240" — hints, borders, disabled
    MutedBright      string // "248" — hover states, active but unfocused
    Background       string // "16"  — pure black
    BackgroundTint   string // "234" — panels, pills, subtle surfaces
    BackgroundTint2  string // "236" — elevated surfaces, selected items

    // Structural colors
    Border           string // "240" — unfocused borders
    BorderFocused    string // "248" — focused borders (idle state)
    Success          string // "114" — muted green for success text
    Error            string // "167" — muted red for error text
    Warning          string // "172" — amber for warnings

    // Dynamic accent
    Accent           string // changes with agent state
    AccentDim        string // slightly dimmed variant
    AccentBright     string // slightly brighter variant
}
```

### Accent State Mapping

- `Idle`: Accent = ForegroundDim ("245") — UI rests in grayscale
- `Streaming`: Accent = Cyan ("45") — information flowing
- `ToolRunning`: Accent = Amber ("172") — work being done
- `Error`: Accent = Red ("167") — urgent, overrides everything

### Thinking Level Border Colors (grayscale)

- `Off`: "240", `Minimal`: "242", `Low`: "244`, `Medium`: "246`, `High`: "248`, `XHigh`: "250"

### Editor Pulse Animation

- Pulse position cycles 0-7 (4 corners + 4 edges)
- Updates every 500ms when agent is active
- Current position uses AccentBright, trailing uses Accent, rest uses BorderFocused
- When idle, border uses thinking-level grayscale color

## Post-Completion

**Manual verification:**
- Open TUI, verify landing screen is left-aligned with model info
- Send a prompt, verify streaming accent (cyan) appears in footer model name and spinner
- Trigger a tool call, verify tool panel renders as bordered card with amber accent
- Watch tool complete, verify brief flash then settled state
- Verify editor border pulses during active work
- Scroll up during streaming, verify dot separators between messages
- Verify footer hides zero-value stats (cost $0.0000, token rate 0)
- Interrupt streaming, verify error accent (red) appears briefly
- Press Ctrl+N for new session, verify clean landing screen returns

**External system updates:** None — this is purely visual within the TUI extension.
