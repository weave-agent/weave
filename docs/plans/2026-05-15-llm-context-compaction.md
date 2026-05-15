# LLM Context Compaction

## Overview
Replace the existing storage-only JSONL compaction with runtime context management that auto-compacts conversations when they approach the model's context window. Uses LLM-generated structured summaries with turn-boundary-aware cut points and cumulative file tracking.

**Problem**: Conversations grow unbounded until the provider rejects them. The current `/compact` command only trims JSONL entries on disk — it doesn't reduce the messages sent to the provider.

**Solution**: Estimate tokens before each API call, and when the budget is exceeded, summarize old messages via an LLM call, replace them with a structured summary, and continue.

## Context

### Files involved
- `extensions/agent/extension.go` — config struct, registration, NewAgentExtension
- `extensions/agent/loop.go` — turn loop (inner loop lines 136-189), streamTurn, message assembly
- `extensions/agent/prompt.go` — system prompt builder
- `extensions/agent/context.go` — context file discovery (pattern to follow for COMPACT.md)
- `extensions/ui/tui/commands.go` — `/compact` command handler (line 73, currently ignores args)
- `sdk/message.go` — Message struct, constructors
- `sdk/model/types.go` — ModelDef with ContextWindow
- `sdk/model/registry.go` — GetModel, model lookup
- `extensions/store/jsonl/store.go` — existing Compact() (storage-level only)

### Key integration points
- Inner loop at `loop.go:136-189` — compaction check goes between draining steering (line 137) and streaming the turn (line 143)
- `AgentExtension` currently registers with empty `struct{}` config — needs a real config struct for compaction settings
- `model.GetModel(name).ContextWindow` provides context window size
- `drainSteering()` at line 137 already handles the "compact" steering event but just adds it as a user message — needs to actually trigger compaction
- `sdk.NewAssistantMessage()` can carry the summary as a regular message with a marker field

### Dependencies
- `weave/sdk` — Message types, Bus, Provider interface (already imported)
- `weave/sdk/model` — ModelDef, StreamOptions (already imported)
- No new external dependencies — token estimation uses `chars/4` heuristic

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**

## Testing Strategy
- **Unit tests**: required for every task
- Table-driven tests for token estimation, cut point detection, serialization
- Mock provider for summary generation tests (use moq-generated mock)
- Integration test for full compaction flow within agent extension

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## Implementation Steps

### Task 1: Add compaction config struct and registration
- [ ] create `CompactionConfig` struct in `extensions/agent/extension.go` with fields: `Enabled bool` (default true), `ReserveTokens int` (default 16384), `KeepRecentTokens int` (default 20000), `Model string` (default empty = use current model)
- [ ] update `init()` registration from `_ struct{}` to use `CompactionConfig` with json/default/description tags
- [ ] store `CompactionConfig` on `AgentExtension` struct
- [ ] write tests verifying config defaults are applied correctly
- [ ] run tests — `cd extensions/agent && go test ./...`

### Task 2: Implement token estimation
- [ ] create `extensions/agent/compaction.go` with `estimateTokens(msgs []sdk.Message) int` using `chars/4` heuristic
- [ ] handle all message content types: string content via `fmt.Sprint()`, tool calls (name + args), thinking content, images (4800 chars each)
- [ ] write table-driven tests for: empty messages, user text, assistant with tool calls, mixed conversation, single oversized message
- [ ] run tests — must pass before Task 3

### Task 3: Implement turn-boundary-aware cut point detection
- [ ] add `findCutPoint(msgs []sdk.Message, keepRecentTokens int) int` in `compaction.go`
- [ ] walk backwards from newest message, accumulate tokens
- [ ] valid cut points: user messages, assistant messages (without pending tool calls)
- [ ] never cut at tool_result messages — they must stay with their parent tool call
- [ ] return index of first message to summarize (everything before index gets summarized)
- [ ] write tests for: cut in middle of conversation, all messages fit (no cut needed), oversized single turn, tool result boundary preservation
- [ ] run tests — must pass before Task 4

### Task 4: Implement COMPACT.md discovery and message serialization
- [ ] add `discoverCompactPrompt(projectDir, globalDir string) string` in `context.go` — follows same pattern as `loadSystemPrompt()`, looks for `.weave/COMPACT.md` (project) then `~/.weave/COMPACT.md` (global), project overrides global
- [ ] add default summarization system prompt as embedded `default-compact-prompt.md` in `extensions/agent/`
- [ ] add `resolveCompactPrompt(customInstructions string, projectDir, globalDir string) string` that returns: custom instructions if non-empty, else COMPACT.md content if found, else default embedded prompt
- [ ] add `serializeForSummary(msgs []sdk.Message, previousSummary string, fileOps fileOperations) string` in `compaction.go`
- [ ] format messages as `[User]: content`, `[Assistant]: content`, `[Tool call]: name(args)`, `[Tool result]: content (truncated to 2000 chars)`
- [ ] prepend previous summary if present (from prior compaction)
- [ ] append cumulative file operation lists (`<read-files>` and `<modified-files>` sections)
- [ ] write tests for: COMPACT.md discovery (project vs global vs default), serialization with empty messages, single turn, multi-turn with tools, truncation of long tool results, previous summary inclusion
- [ ] run tests — must pass before Task 5

### Task 5: Implement file operation tracking
- [ ] add `fileOperations` struct in `compaction.go` with `readFiles map[string]bool` and `modifiedFiles map[string]bool`
- [ ] add `trackFileOp(msgs []sdk.Message, ops *fileOperations)` that scans messages for read/edit/write tool calls and tool results
- [ ] integrate into `AgentExtension` as a field initialized on new conversation
- [ ] accumulate across compactions — when summary replaces messages, preserved file lists become the baseline
- [ ] write tests for: no file ops, read tracking, edit/write tracking, accumulation across compaction
- [ ] run tests — must pass before Task 6

### Task 6: Implement compaction execution (LLM summary generation)
- [ ] add `compact(ctx context.Context, bus sdk.Bus, provider sdk.Provider, msgs []sdk.Message, cfg CompactionConfig, model string, ops *fileOperations) ([]sdk.Message, error)` in `compaction.go`
- [ ] call `findCutPoint` to determine what to summarize
- [ ] call `serializeForSummary` to build the summarization prompt
- [ ] call `provider.Stream()` with a summarization system prompt and the serialized conversation
- [ ] parse the response into a summary string
- [ ] return new message slice: [summary message] + [kept messages]
- [ ] write tests using mock provider that returns a canned summary
- [ ] run tests — must pass before Task 7

### Task 7: Integrate compaction into agent turn loop
- [ ] update `/compact` command in `commands.go` to pass `args` through: `PublishSteer(bus, "compact "+args)` — allows `/compact focus on the auth refactor`
- [ ] add `shouldCompact(messages []sdk.Message, systemPrompt string, cfg CompactionConfig, modelName string) bool` helper
- [ ] add `compactionCheck` step in inner loop at `loop.go:137` — after `drainSteering`, before `streamTurn`
- [ ] handle the "compact" steering event specially: parse args from payload, pass to `compact()` as `customInstructions`, trigger compaction immediately instead of adding as user message
- [ ] publish `agent.compacted` bus event with metadata (messages removed, tokens saved)
- [ ] reset `fileOperations` tracker on new conversation (`/new`)
- [ ] write tests for: steering event with custom instructions, auto-trigger flow, manual trigger without args
- [ ] run tests — must pass before Task 8

### Task 8: Add TUI compaction notification
- [ ] handle `agent.compacted` bus event in `extensions/ui/tui/bridge.go`
- [ ] show `NotifyTyped` with `NotifyInfo`: "Context compacted: N messages summarized"
- [ ] add a visual chat entry for the compaction summary (similar to thinking block styling with `BackgroundTint`)
- [ ] run tests — must pass before Task 9

### Task 9: Verify acceptance criteria
- [ ] verify auto-compaction triggers when tokens approach context window
- [ ] verify manual `/compact` triggers immediate compaction
- [ ] verify turn boundaries are respected (no split tool call/result)
- [ ] verify file operations accumulate across compactions
- [ ] verify config overrides work (disabled, custom reserve tokens, custom model)
- [ ] verify `/compact focus on X` passes custom instructions to summarization
- [ ] verify COMPACT.md (project or global) is used when no inline instructions provided
- [ ] run full test suite — `cd extensions/agent && go test ./...` and `cd extensions/ui/tui && go test ./...`
- [ ] run linter — `cd extensions/agent && make lint`

## Technical Details

### CompactionConfig struct
```go
type CompactionConfig struct {
    Enabled          bool   `json:"enabled" default:"true" description:"Enable auto-compaction"`
    ReserveTokens    int    `json:"reserve_tokens" default:"16384" description:"Tokens reserved for model response"`
    KeepRecentTokens int    `json:"keep_recent_tokens" default:"20000" description:"Recent tokens to keep (not summarized)"`
    Model            string `json:"model" default:"" description:"Model for summary generation (empty = current model)"`
}
```

Config scope: agent (`agent_loop.compaction.*` in settings JSON).

### Custom compaction prompt resolution (priority order)
1. **`/compact [instructions]`** — inline args from the slash command override everything
2. **`.weave/COMPACT.md`** (project-local) — persistent project-specific summarization instructions
3. **`~/.weave/COMPACT.md`** (global) — user-wide summarization instructions
4. **Embedded default** (`default-compact-prompt.md`) — built-in structured summary prompt

Discovery follows the same pattern as `SYSTEM.md`/`APPEND_SYSTEM.md` in `context.go`. Project-local overrides global.

Example `COMPACT.md`:
```markdown
Focus the summary on:
- Architecture decisions and their rationale
- Files that were modified and why
- Any unresolved issues or trade-offs discussed
- The current state of the implementation plan
```

### Summary message format
The compaction summary is stored as an assistant message with a sentinel prefix:
```
[Compaction Summary]

## Goal
...

## Progress
...

<read-files>
...
</read-files>

<modified-files>
...
</modified-files>
```

### Token estimation
- Text content: `len(content) / 4`
- Tool calls: sum of `len(name) + len(args)` / 4 per call
- Tool results: `len(content) / 4`
- Images: 1200 tokens each (4800 chars)
- System prompt: `len(systemPrompt) / 4`

### shouldCompact check
```go
func shouldCompact(messages []sdk.Message, systemPrompt string, cfg CompactionConfig, modelName string) bool {
    if !cfg.Enabled {
        return false
    }
    contextWindow := contextWindowSize(modelName)
    if contextWindow == 0 {
        return false
    }
    total := len(systemPrompt)/4 + estimateTokens(messages)
    return total > contextWindow - cfg.ReserveTokens
}
```

### Inner loop integration point (loop.go)
```
line 137: messages, _ = drainSteering(steerCh, messages)
+ NEW: check for "compact" steering, extract custom instructions from payload
+ NEW: trigger compaction with resolveCompactPrompt(instructions, projectDir, globalDir)
+ NEW: if shouldCompact(), trigger auto-compaction (no custom instructions)
line 139: bus.Publish(sdk.NewEvent(TopicTurnStart, turn))
line 143: resp, toolCalls, err := streamTurn(...)
```

## Post-Completion

**Manual verification**:
- Test with a long conversation that exceeds context window — verify auto-compaction fires
- Test `/compact` manually — verify summary quality
- Test with different models — verify context window detection works per-model
- Test with compaction disabled — verify no auto-triggering

**Future considerations** (not in scope):
- Extension hooks (`session_before_compact`, `session_compact` events)
- Branch summarization for conversation tree navigation
- Oversized single-turn splitting (dual summaries)
