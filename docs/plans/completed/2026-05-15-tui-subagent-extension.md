# TUI Subagent Extension

## Overview

A TUI extension that visualizes running subagents as per-agent panels in the panel tray. Each subagent appears as its own panel tab while running, showing live status and elapsed time. Panels auto-remove a few seconds after the agent completes.

**Problem it solves:** Background subagents run asynchronously with no visual feedback in the TUI. Users must invoke `check_agent` manually to see status. This extension provides at-a-glance visibility into active agents.

**Integration:** Purely event-driven via the bus. The extension subscribes to `subagent.started` and `subagent.done` events. No cross-module imports needed.

## Context (from discovery)

- **Files to modify:**
  - `extensions/tools/subagent/background.go` — add `subagent.started` event publication
  - `extensions/tools/subagent/execute.go` — possibly generate IDs for foreground agents
- **Files to create:**
  - `extensions/ui/tui/extensions/subagent/go.mod`
  - `extensions/ui/tui/extensions/subagent/subagent_ui.go` — main extension
  - `extensions/ui/tui/extensions/subagent/tracker.go` — agent state tracking
  - `extensions/ui/tui/extensions/subagent/panel.go` — panel drawer
  - `extensions/ui/tui/extensions/subagent/renderer.go` — rich tool renderer
  - `extensions/ui/tui/extensions/subagent/*_test.go` — tests
- **Related patterns found:**
  - diff-viewer (`extensions/ui/tui/extensions/diff-viewer/`) uses `tui.RegisterTUIExtension`
  - sandbox-ui uses `sdk.RegisterUIExtension` with bus subscription
  - Panel system: `PanelManager.Register/Show/Remove`, `PanelTray.syncPanelTray()`
- **Dependencies identified:**
  - `weave/ext/ui/tui` for TUIExtAPI
  - `weave` (sdk) for bus events and UI interfaces
- **Gaps:**
  - No `subagent.started` event exists — must add
  - No dynamic panel title API — not needed since panels are removed on completion
  - Panel-based TUI extensions are untested in codebase

## Development Approach

- **Testing approach:** Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Testing Strategy

- **Unit tests** for `AgentTracker` — start/done/lookup/removal/grace-period logic
- **Unit tests** for panel drawer — mock `uv.Screen` and verify rendered output
- **Unit tests** for rich renderer — verify it returns styled output for subagent tool calls
- **Integration tests** — verify panel lifecycle callbacks fire when bus events are published
- **Mock TUIExtAPI** — hand-rolled stub to verify `ShowPanel`/`RemovePanel` calls with correct args

## Implementation Steps

### Task 1: Add `subagent.started` event to subagent extension
- [x] Add `subagent.started` event publication in `background.go` when `spawn()` creates a new agent
- [x] Ensure payload includes `id`, `name`, `mode` keys as `map[string]string`
- [x] Consider adding `subagent.started` for foreground agents in `execute.go` (if straightforward) — skipped, foreground agents block synchronously and do not need the event; only background agents emit `subagent.started`, which is sufficient for panel visualization
- [x] Write tests verifying the event is published with correct payload
- [x] Run subagent extension tests: `cd extensions/tools/subagent && go test ./...`

### Task 2: Create subagent TUI extension module skeleton
- [x] Create `extensions/ui/tui/extensions/subagent/` directory
- [x] Create `go.mod` with proper module path (`weave/ext/ui/tui/extensions/subagent`) and replacements
- [x] Create main `subagent_ui.go` with `init()` registering via `tui.RegisterTUIExtension`
- [x] Implement stub `TUIExtension` interface (`Name()`, `RegisterTUI(api)`)
- [x] Verify module compiles: `cd extensions/ui/tui/extensions/subagent && go build ./...`

### Task 3: Implement `AgentTracker`
- [x] Create `tracker.go` with `AgentTracker` struct and `TrackedAgent` type
- [x] Methods: `Start(id, name, mode)`, `Done(id, status, result)`, `Get(id)`, `List()`, `Remove(id)`
- [x] Grace period handling: `Done()` marks status but `Remove()` is deferred (3-second timer or tick-based)
- [x] Thread-safe using `sync.RWMutex`
- [x] Write tests for all tracker methods (success + concurrent access cases)
- [x] Write tests for grace period logic

### Task 4: Implement bus subscription and panel lifecycle
- [x] In `RegisterTUI`, create `AgentTracker` instance
- [x] Access the event bus (may need dual registration as `sdk.UIExtensionWithBus` if bus not available via TUIExtAPI)
- [x] Subscribe to `subagent.started`: create `TrackedAgent`, call `api.ShowPanel()` with panel drawer
- [x] Subscribe to `subagent.done`: update agent status/result, schedule panel removal after grace period
- [x] Implement panel removal logic: after grace period, call `api.RemovePanel()` and `tracker.Remove()`
- [x] Write tests for panel lifecycle: start → done → removal

### Task 5: Implement panel drawer
- [x] Create `panel.go` with `agentPanelDrawer` implementing `tui.PanelDrawer`
- [x] `Draw()` renders: agent name, status indicator (●/✓/✗), mode, elapsed time, truncated result preview
- [x] `Update()` handles `tea.Msg` — on tick, trigger redraw for elapsed time updates
- [x] `Handles()` returns true for tick messages
- [x] Use `theme` colors for status indicators
- [x] Write tests verifying rendered output matches expected format

### Task 6: Implement rich tool renderer
- [x] Create `renderer.go` with `subagentRenderer` implementing `tui.RichToolRenderer`
- [x] Register renderer for all `subagent_*` tool names dynamically (discover from agent registry or use prefix matching if supported)
- [x] Render running state with spinner placeholder
- [x] Render completed state with compact result card
- [x] Write tests for renderer output

### Task 7: Final integration and verification
- [x] Wire all components together in `RegisterTUI`
- [x] Ensure no panels leak (all removals are accounted for)
- [x] Add graceful handling when TUI shuts down while agents are running
- [x] Run full TUI extension tests: `cd extensions/ui/tui && go test ./...`
- [x] Run subagent extension tests: `cd extensions/tools/subagent && go test ./...`
- [x] Run root tests: `go test ./...`
- [x] Run linter: `make lint`

### Task 8: Verify acceptance criteria
- [x] Verify `subagent.started` event is published when background agents spawn
- [x] Verify panel appears in tray when agent starts
- [x] Verify panel shows agent name, status, and elapsed time
- [x] Verify panel auto-removes a few seconds after agent completes/fails
- [x] Verify rich renderer shows subagent tool calls in chat
- [x] Verify no custom keybindings are needed (tray-only interaction)
- [x] Run full test suite (unit tests)
- [x] Run linter — all issues fixed

### Task 9: Update documentation
- [x] Add brief note to TUI section in project docs about subagent panel feature
- [x] Update this plan file with any deviations discovered during implementation

## Technical Details

### Data structures

```go
type AgentStatus int

const (
    AgentRunning AgentStatus = iota
    AgentCompleted
    AgentFailed
)

type TrackedAgent struct {
    ID        string
    Name      string
    Status    AgentStatus
    Mode      string
    SpawnedAt time.Time
    DoneAt    time.Time
    Result    string
    PanelID   string
}
```

### Event payloads

`subagent.started`:
```go
map[string]string{
    "id":   "subagent_researcher_a1b2c3",
    "name": "researcher",
    "mode": "background",
}
```

`subagent.done` (existing):
```go
map[string]string{
    "id":      "subagent_researcher_a1b2c3",
    "status":  "completed",
    "content": "...",
}
```

### Processing flow

1. User invokes `subagent_researcher` with `run_in_background: true`
2. Subagent extension spawns background goroutine, publishes `subagent.started`
3. TUI extension receives event, creates `TrackedAgent`, calls `ShowPanel`
4. Panel drawer renders live status with elapsed time ticking
5. Agent completes, subagent extension publishes `subagent.done`
6. TUI extension updates status, starts 3-second grace period
7. Grace period expires, `RemovePanel` called, agent removed from tracker
8. Tray tab disappears

## Post-Completion

**Manual verification:**
- Spawn a background subagent and verify panel appears in tray
- Spawn multiple background agents and verify multiple tabs
- Let agents complete and verify auto-removal
- Verify chat still shows rich renderer output for review

**Future enhancements (out of scope):**
- Interactive controls in panel (cancel agent, send message)
- Foreground agent panel support
- Persistent agent history panel
- Panel title badges showing status counts
