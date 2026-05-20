# Tool Streaming & Interruption — SDK Foundation

## Overview
Add the event types and utilities that enable tools to stream partial output and the framework to track tool lifecycle. This is the foundation that the agent, TUI, and tool extensions will build on.

## Context
- `sdk/provider.go:52` — `ToolCall` already has `ID string`, no change needed
- `sdk/bus.go` — bus context attachment already exists (`BusFromContext`)
- `sdk/tool.go` — `Tool` interface stays unchanged; streaming is opt-in via bus events

## Development Approach
- Regular approach (code first, then tests)
- Every task includes tests before moving to next

## Implementation Steps

### Task 1: Add tool event types and topics
- [ ] Create `sdk/tool_events.go` with `ToolProgress` struct and event topic constants
- [ ] Topics: `tool.start`, `tool.progress`, `tool.complete`, `tool.error`, `tool.interrupted`
- [ ] Write tests for ToolProgress JSON marshal/unmarshal
- [ ] Run `go test ./sdk/...` — must pass

### Task 2: Add throttle helper
- [ ] Create `sdk/throttle.go` with `Throttle(fn func(), interval time.Duration)` helper
- [ ] First call fires immediately; subsequent calls deduplicated within interval
- [ ] Goroutine-safe; stops scheduling when context is canceled
- [ ] Write tests for immediate first call, deduplication, cancellation, concurrency safety
- [ ] Run `go test ./sdk/...` — must pass

### Task 3: Verify integration
- [ ] Run `make lint` — fix any issues
- [ ] Run `go test ./...` in root module — must pass

## Technical Details

```go
// sdk/tool_events.go
type ToolProgress struct {
    ToolCallID string `json:"tool_call_id"`
    ToolName   string `json:"tool_name"`
    Content    string `json:"content,omitempty"`
    IsError    bool   `json:"is_error,omitempty"`
}

const (
    TopicToolStart       = "tool.start"
    TopicToolProgress    = "tool.progress"
    TopicToolComplete    = "tool.complete"
    TopicToolError       = "tool.error"
    TopicToolInterrupted = "tool.interrupted"
)
```

## Post-Completion
- Agent extension and TUI extension plans depend on this SDK release
- No manual verification needed — pure library addition
