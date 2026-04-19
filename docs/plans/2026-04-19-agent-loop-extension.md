# Agent Loop Extension

## Overview
Implement the first core extension — the agent loop — that drives the LLM conversation cycle. Ports the two-level while-loop logic from pi-coding-agent (Node.js) to Go. Introduces the `core` config model (singleton agent-loop, multiple providers), tool/provider registries, and the streaming agent cycle.

## Context
- Existing codebase: `sdk/` (Extension, Bus, Event, Wire, Config, Registry), `bus/`, `config/`, `launcher/`
- Pi agent loop source: `/opt/homebrew/lib/node_modules/@mariozechner/pi-coding-agent/node_modules/@mariozechner/pi-agent-core/dist/agent-loop.js`
- `config/config.go` has a `Slots` field to replace with `Core`
- No provider or tool interfaces exist yet — these are added here
- Wire currently takes `[]string` extension names — needs core/extension merging

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change

## Design Decisions
- **Config**: `core` map with `agent-loop` (singleton, hardcoded default "loop") and `providers` (multiple, default "anthropic"). `extensions` list for optional extras.
- **Registries**: `sdk.RegisterProvider/GetProvider`, `sdk.RegisterTool/GetTool` — same pattern as existing `RegisterExtension/GetExtension`
- **Direct calls**: Loop calls provider and tools directly through registries (not bus-mediated)
- **Bus events for observability**: Loop publishes events at every stage
- **Wire**: Merges core defaults + config overrides + optional extensions. Validates: one agent-loop, at least one provider.

## Target Package Layout (new/changed files)
```
weave/
  sdk/
    provider.go          -- NEW: Provider interface, ProviderRequest, ProviderEvent
    provider_registry.go -- NEW: RegisterProvider/GetProvider
    tool.go              -- NEW: Tool interface, ToolDef, ToolResult
    tool_registry.go     -- NEW: RegisterTool/GetTool
    message.go           -- NEW: Message types (User, Assistant, ToolResult)
    wire.go              -- CHANGED: core/extension merging + validation
  config/
    config.go            -- CHANGED: replace Slots with Core struct
  extensions/
    agent-loop/
      loop.go            -- NEW: agent-loop extension (the pi-style cycle)
      loop_test.go
      go.mod             -- extension module file
```

## Testing Strategy
- **Unit tests**: required for every task
- Table-driven tests where applicable
- Mock providers and tools for agent-loop tests
- Test both success and error paths

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## Implementation Steps

### Task 1: Update config struct — replace Slots with Core
- [x] update `config/config.go`: replace `Slots map[string]string` with `Core` struct containing `AgentLoop string` (default "loop") and `Providers []string` (default ["anthropic"])
- [x] remove `Slots` initialization in `LoadFromDir`
- [x] add `Core() ([]string, []string)` method to File that returns (coreExts, optionalExts)
- [x] write tests for new config loading with core defaults
- [x] write tests for core override via yaml
- [x] run tests — must pass before next task

### Task 2: Add Provider interface and registry to sdk/
- [x] create `sdk/provider.go` — `Provider` interface with `Stream(ctx, ProviderRequest) (<-chan ProviderEvent, error)`, `ProviderRequest` struct (SystemPrompt, Messages, Tools), `ProviderEvent` struct (Type string, Content any)
- [x] create `sdk/provider_registry.go` — `RegisterProvider(name, factory)` / `GetProvider(name, cfg)` / `ListProviders()` / `ResetProviderRegistry()` — same pattern as extension registry
- [x] write tests for provider registration and retrieval
- [x] write tests for duplicate registration and missing provider
- [x] run tests — must pass before next task

### Task 3: Add Tool interface and registry to sdk/
- [x] create `sdk/tool.go` — `Tool` interface with `Name() string`, `Definition() ToolDef`, `Execute(ctx, args map[string]any) (ToolResult, error)`; `ToolDef` struct (Name, Description, Parameters any); `ToolResult` struct (Content string, IsError bool)
- [x] create `sdk/tool_registry.go` — `RegisterTool(name, factory)` / `GetTool(name, cfg)` / `ListTools()` / `ResetToolRegistry()` — same pattern as provider registry
- [x] write tests for tool registration and retrieval
- [x] write tests for duplicate registration and missing tool
- [x] run tests — must pass before next task

### Task 4: Add Message types to sdk/
- [ ] create `sdk/message.go` — `Message` struct with `Role` (enum: "user", "assistant", "toolResult"), `Content` (any), `ToolCallID` string, `ToolName` string, `Timestamp` time.Time; typed constructors `NewUserMessage`, `NewAssistantMessage`, `NewToolResultMessage`
- [ ] write tests for message constructors and role validation
- [ ] run tests — must pass before next task

### Task 5: Update Wire for core/extension merging
- [ ] update `sdk/wire.go` — `Wire` signature changes to accept core config: add core extension names to the front of the list, validate exactly one agent-loop extension and at least one provider
- [ ] Wire merges: core names first, then optional extensions, deduplicates
- [ ] validation errors for: missing agent-loop, duplicate agent-loop, no provider
- [ ] update `wire_test.go` for new Wire behavior
- [ ] write tests for core default merging
- [ ] write tests for validation errors
- [ ] run tests — must pass before next task

### Task 6: Implement agent-loop extension
- [ ] create `extensions/agent-loop/go.mod` — extension module with `require weave` and `replace` directive
- [ ] create `extensions/agent-loop/loop.go` — register as "loop" extension via `sdk.RegisterExtension("loop", ...)`
- [ ] implement `Subscribe(bus)`: subscribe to `agent.prompt`, `agent.steer`, `agent.followup` topics; launch goroutine for the main loop
- [ ] implement the two-level while loop: outer (follow-ups via bus), inner (steering via bus + tool calls)
- [ ] inner loop: inject steering messages → call `provider.Stream` → extract tool calls → look up tools in registry → execute → emit `agent.tool_result` → feed results back → repeat
- [ ] emit bus events: `agent.turn_start/end`, `agent.message_start/update/end`, `agent.tool_result`, `agent.end`
- [ ] implement `Close()`: cancel context, clean up goroutines
- [ ] create mock provider and mock tool for testing
- [ ] write tests for loop startup and shutdown
- [ ] write tests for single turn (prompt → response, no tools)
- [ ] write tests for tool call cycle (prompt → tool call → tool result → final response)
- [ ] write tests for steering message injection mid-loop
- [ ] write tests for follow-up message re-entry
- [ ] write tests for error/abort stop conditions
- [ ] run tests — must pass before next task

### Task 7: Update launcher to handle core config
- [ ] update `cmd/weave/main.go` — extract core and optional extensions from config, pass merged list to Wire
- [ ] update `launcher/launcher.go` — pass extension names from core + optional to discovery/build
- [ ] write tests for the updated main flow
- [ ] run full test suite — all tests must pass

### Task 8: Verify acceptance criteria
- [ ] verify all requirements from Overview are implemented
- [ ] verify edge cases handled (missing provider, no tools, empty prompt)
- [ ] run full test suite (`go test ./...`)
- [ ] run linter (`make lint`) — all issues fixed
- [ ] run formatter (`make fmt`)

## Technical Details

### Agent loop cycle (from pi)
```
runLoop():
  outer: for {
    inner: for hasToolCalls || hasSteering {
      inject steering messages → context
      emit agent.turn_start
      response = provider.Stream(ctx, req)
      emit agent.message_start/update/end (streaming)
      if error/abort → emit agent.end → return
      toolCalls = extractToolCalls(response)
      for each toolCall:
        tool = toolRegistry.Lookup(name)
        result = tool.Execute(ctx, args)
        emit agent.tool_result
        append result → context
      emit agent.turn_end
      hasSteering = check bus for agent.steer
    }
    followUps = check bus for agent.followup
    if no followUps → break
  }
  emit agent.end
```

### Bus topics
**Subscribes:** `agent.prompt`, `agent.steer`, `agent.followup`
**Publishes:** `agent.turn_start`, `agent.turn_end`, `agent.message_start`, `agent.message_update`, `agent.message_end`, `agent.tool_result`, `agent.end`

### Config yaml
```yaml
core:
  agent_loop: loop
  providers:
    - anthropic
extensions:
  - bash-tool
  - file-tool
```

## Post-Completion
*Items requiring manual intervention or external systems — no checkboxes, informational only*

**Next steps (separate plans):**
- Implement actual Anthropic provider extension
- Implement tool extensions (bash, file, grep, etc.)
- Implement session store extension
- Wire up compaction/summarization
