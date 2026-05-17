# Subagent Panel Upgrade: Live Output, Progress, and Cancel

## Overview

Upgrade the subagent UI panel from a minimal status indicator to an informative, interactive view that shows live subagent output (tool calls, streaming text), task details (prompt, progress), and provides a cancel button to kill running subagent processes.

**Problem:** Currently the subagent panel shows only a status icon, agent name, mode, and elapsed time. Users have no visibility into what the agent is doing and no way to stop a runaway or misdirected subagent.

**Solution:** Stream child process JSON events via a new `subagent.output` bus event to a per-agent ring buffer in the UI extension. Render the last N events as a scrollable tool log in the expanded panel. Add per-agent cancellation via `subagent.cancel` bus event + context cancellation.

## Context (from discovery)

**Subagent tool extension** (`extensions/tools/subagent/`):
- `background.go` — `backgroundManager` tracks agents, uses single shared `context.Context`
- `execute.go` — `runSubagent` spawns child process, `parseJSONLines` discards all events except `message_end`
- `broker.go` — `monitorStdout` routes inter-agent messages, also discards tool/message events
- Child emits JSON lines with types: `message_start`, `message_update`, `message_end`, tool events

**Subagent UI extension** (`extensions/ui/tui/extensions/subagent-ui/`):
- `subagent_ui.go` — subscribes to `subagent.started`, `subagent.done`, `agent.end`
- `panel.go` — `agentPanelDrawer` renders status icon + name + elapsed (minimal)
- `tracker.go` — `AgentTracker` manages `TrackedAgent` state with grace-period timers
- `renderer.go` — rich renderer for chat cards

**Key constraint:** Extensions are separate Go modules. The subagent tool and UI extension communicate exclusively through bus events — no direct imports.

## Development Approach

- **Testing approach:** Regular (code first, then tests per task)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change

## Testing Strategy

- **Unit tests:** required for every task
- Tests for subagent tool extension: `cd extensions/tools/subagent && go test ./...`
- Tests for subagent UI extension: `cd extensions/ui/tui/extensions/subagent-ui && go test ./...`
- All tests use testify assertions and moq mocks per project conventions

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope

## Implementation Steps

### Task 1: Add output ring buffer to tracker
- [x] create `outputEntry` struct and `outputRing` type in `extensions/ui/tui/extensions/subagent-ui/tracker.go` with `Append(entry)`, `Snapshot() []outputEntry`, and configurable capacity (default 200)
- [x] add `Output *outputRing` and `Prompt string` fields to `TrackedAgent`
- [x] initialize `outputRing` in `AgentTracker.Start()`
- [x] write tests for ring buffer wrap-around, snapshot accuracy, and concurrent append/read in `tracker_test.go`
- [x] run tests - must pass before task 2

### Task 2: Add per-agent cancellation to backgroundManager
- [x] add `cancel context.CancelFunc` field to `backgroundAgent` in `extensions/tools/subagent/background.go`
- [x] create per-agent context via `context.WithCancel(bm.ctx)` in `spawn()`, store cancel func on `backgroundAgent`
- [x] add `cancelAgent(id string) error` method to `backgroundManager` that calls `ba.cancel()` for running agents
- [x] update `spawn()` to pass per-agent context to `runSubagent` instead of `bm.ctx`
- [x] write tests for cancel propagation: single agent cancel, manager shutdown cancels all agents, cancel of already-completed agent returns error
- [x] run tests - must pass before task 3

### Task 3: Plumb onEvent callback through stdout parsers
- [x] add `onEvent func(jsonEvent)` parameter to `parseJSONLines` in `extensions/tools/subagent/execute.go`, call it for each parsed JSON line
- [x] add `onEvent func(jsonEvent)` parameter to `monitorStdout` and `MonitorStdout` in `extensions/tools/subagent/broker.go`, call it for tool/message events (skip inter-agent routing types: send, broadcast, list_agents)
- [x] add `onEvent func(jsonEvent)` parameter to `runSubagent` in `execute.go`, pass through to both parser paths
- [x] update `testRunSubagent` signature in `execute.go` to match new parameter
- [x] update all callers and test files referencing the changed signatures
- [x] write tests verifying `onEvent` is called with correct event types for both parseJSONLines and broker.monitorStdout paths
- [x] run tests - must pass before task 4

### Task 4: Publish subagent.output and subscribe subagent.cancel in tool extension
- [x] add `notifyOutput(id string, evt jsonEvent)` method to `backgroundManager` that publishes `subagent.output` bus event with `{id, type, tool, content}` payload
- [x] wire `onEvent` callback in `spawn()` to call `bm.notifyOutput(subagentID, evt)`
- [x] add bus subscription for `subagent.cancel` in `backgroundManager.setBus()` or extension Subscribe, calling `bm.cancelAgent(payload["id"])`
- [x] write tests verifying `subagent.output` events are published for each parsed JSON line and `subagent.cancel` triggers cancelAgent
- [x] run tests - must pass before task 5

### Task 5: Subscribe to subagent.output in UI extension and populate ring buffer
- [x] add `subagent.output` case to the `OnAll` handler in `extensions/ui/tui/extensions/subagent-ui/subagent_ui.go`
- [x] parse payload fields (`id`, `type`, `tool`, `content`) and append `outputEntry` to the agent's ring buffer via tracker method
- [x] call `api.RequestRedraw()` after appending to trigger panel update
- [x] write tests for ring buffer population from bus events, handling of missing agent ID, and bad payload
- [x] run tests - must pass before task 6

### Task 6: Rewrite panel drawer with live output and cancel
- [x] update `agentPanelDrawer.Draw()` in `extensions/ui/tui/extensions/subagent-ui/panel.go` to render: header row (status + name + mode + elapsed + cancel button), prompt row, separator, scrollable tool log from ring buffer snapshot
- [x] update `agentPanelDrawer.Update()` to handle cancel keypress (Ctrl+X or Enter) — publish `subagent.cancel` bus event with agent ID
- [x] update `agentPanelDrawer.Handles()` to return true for `tea.KeyPressMsg`
- [x] change panel config in `handleStarted()` from `TrayOnly` to `AboveEditor` (or keep TrayOnly with expand-to-overlay on Enter)
- [x] update `TrackedAgent` to store `SubagentID` for cancel payload (extract from panel ID or store directly)
- [x] write tests for panel rendering with populated ring buffer, cancel keypress behavior, and empty/nil buffer handling
- [x] run tests - must pass before task 7

### Task 7: Update rich renderer for cancelled status
- [x] add `cancelled` status handling in `subagentRenderer.Render()` in `extensions/ui/tui/extensions/subagent-ui/renderer.go`
- [x] add `AgentCancelled` status to `AgentStatus` enum in `tracker.go` if not already covered
- [x] handle cancelled status in `statusIndicator()` in `panel.go` (e.g. `⊘` icon in `Warning` color)
- [x] update `backgroundManager.notifyDone()` to detect context cancellation and set status to `"cancelled"`
- [x] write tests for cancelled status rendering in both panel and rich renderer
- [x] run tests - must pass before task 8

### Task 8: Verify acceptance criteria
- [x] verify all requirements from Overview are implemented
- [x] verify edge cases: agent completes before panel expand, cancel non-existent agent, multiple agents simultaneously
- [x] run full test suite for subagent tool extension: `cd extensions/tools/subagent && go test ./...`
- [x] run full test suite for subagent UI extension: `cd extensions/ui/tui/extensions/subagent-ui && go test ./...`
- [x] run linter on both modules: `cd extensions/tools/subagent && golangci-lint run --timeout 2m ./...` and `cd extensions/ui/tui/extensions/subagent-ui && golangci-lint run --timeout 2m ./...`
- [x] fix any linter issues

### Task 9: Update documentation
- [x] update CLAUDE.md with new bus events (`subagent.output`, `subagent.cancel`) and per-agent cancellation
- [x] update CLAUDE.md subagent UI extension description with new panel capabilities

## Technical Details

**New bus events:**

`subagent.output` (published by subagent tool extension):
```go
sdk.NewEvent("subagent.output", map[string]any{
    "id":      "subagent_general_abc123",
    "type":    "tool_start",    // jsonEvent.Type
    "tool":    "read",          // jsonEvent.Tool
    "content": "...",           // jsonEvent.Content
})
```

`subagent.cancel` (published by subagent UI extension):
```go
sdk.NewEvent("subagent.cancel", map[string]string{
    "id": "subagent_general_abc123",
})
```

**Data structures:**

```go
type outputEntry struct {
    Type    string    // "tool_start", "tool_end", "message_start", "message_update", "message_end"
    Tool    string    // e.g. "read", "edit"
    Content string    // truncated
    Time    time.Time
}

type outputRing struct {
    mu    sync.RWMutex
    items []outputEntry
    cap   int
}
```

**Panel layout (expanded, 18 rows):**
```
─────────────────────────────────────────────────
  ● researcher  background  12s        [✕ cancel]
  Prompt: "Find all API endpoints in the codebase"
─────────────────────────────────────────────────
  ⚙ read    api/handler.go
  ✓ grep    "func.*Handler"
  ⚙ edit    api/handler.go
  → ...streaming response...
─────────────────────────────────────────────────
```

**Status indicators:** `●` running (Accent), `✓` completed (Success), `✗` failed (Error), `⊘` cancelled (Warning)

## Post-Completion

**Manual verification:**
- Test with multiple background agents running simultaneously
- Verify cancel kills the process and updates the panel
- Verify live output streams correctly in expanded panel
- Verify panel expands/collapses cleanly from tray
- Test graceful cleanup on `agent.end` with running agents
