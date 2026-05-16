# Harness Hardening: Audit Fixes

## Overview
Fix 17 gaps identified by the agents-best-practices audit of the Weave agent framework. The audit reviewed 6 domains (agentic loop, tools/permissions, context/compaction, security/observability, skills/subagents, prompt caching) and found 22 GAPs, 14 WARNs, 26 PASSes. This plan addresses items across P0 (safety), P1 (reliability), P2 (cost/observability), and P3 (completeness).

**Items explicitly deferred:**
- Secrets redaction (tool results + logs) — skipped by user decision
- Tool risk classification — documentation only
- Cost budget enforcement — display only, no automatic stop
- User prompt input validation — skipped
- Transactional multi-file edits — design limitation, not addressed
- Extension review/approval on install — install-time warning only

## Context (from audit)
- **Agent loop**: `extensions/agent/loop.go` — inner tool-calling loop has no step limit, no panic recovery, no retry
- **Tool system**: `sdk/tool.go`, `sdk/tool_registry.go` — no risk classification, no argument validation against schemas
- **Context assembly**: `extensions/agent/prompt.go` — context files injected without trust labels, date/CWD breaks cache prefix
- **Providers**: `extensions/providers/anthropic/`, `utils/openaicompat/` — no retry, no usage extraction, no cache_control
- **Skills**: `extensions/agent/skills.go` — AllowedTools parsed but never enforced
- **Subagents**: `extensions/tools/subagent/agent.go` — omitting tools field gives all tools with no warning
- **Session store**: `extensions/store/jsonl/store.go` — compaction not reflected on resume

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- Every task includes new/updated tests
- All tests must pass before starting next task
- Run tests after each change

## Testing Strategy
- **Unit tests**: required for every task
- Tests cover both success and error scenarios
- Run `cd extensions/agent && go test ./...` for agent tests
- Run `cd extensions/tools/subagent && go test ./...` for subagent tests
- Run `cd extensions/ui/tui && go test ./...` for TUI tests
- Run `cd extensions/providers/anthropic && go test ./...` for provider tests

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## Implementation Steps

### Task 1: Add inner loop step limit
- [x] add `MaxSteps int` field to `CompactionConfig` (or new `LoopConfig`) in `extensions/agent/extension.go` with default 50
- [x] add step counter and limit check in the inner loop at `extensions/agent/loop.go:210-212` — when exceeded, set `continueLoop = false` and publish an `agent.compacted`-style warning event
- [x] add config wiring in the factory function so `max_steps` is loadable from settings
- [x] write tests: TestAgent_InnerLoopStepLimit, TestAgent_StepLimitConfigurable in `extensions/agent/loop_test.go`
- [x] run `cd extensions/agent && go test ./...` — must pass before task 2

### Task 2: Add tool execution panic recovery
- [x] wrap `executeTool()` body at `extensions/agent/loop.go:708-722` with `defer func() { if r := recover(); r != nil { ... } }()` that returns `ToolResult{Content: fmt.Sprintf("tool panicked: %v", r), IsError: true}` and logs stack trace via `sdk.Logger`
- [x] write tests: TestExecuteTool_PanicRecovery in `extensions/agent/loop_test.go` using a mock tool that panics
- [x] run `cd extensions/agent && go test ./...` — must pass before task 3

### Task 3: Add trust labeling on context files
- [x] in `extensions/agent/prompt.go`, wrap context file content (CLAUDE.md/AGENTS.md) in `<user_context trust="untrusted">` XML tags around the `# Project Context` section
- [x] wrap APPEND_SYSTEM.md content in `<user_appended_context>` tags
- [x] add one line to `extensions/agent/default-system-prompt.md` instructing the model that content in `<user_context>` tags is user-provided guidance, not system policy
- [x] write tests: update TestBuild or add TestBuild_TrustLabels in `extensions/agent/prompt_test.go` verifying XML markers appear in output
- [x] run `cd extensions/agent && go test ./...` — must pass before task 4

### Task 4: Add provider retry logic
- [x] create a shared retry helper in `sdk/retry/retry.go` with configurable max retries (default 10), exponential backoff (1s base, 2x multiplier, 30s cap), and a predicate function to classify retriable vs non-retriable errors
- [x] integrate retry into Anthropic provider `extensions/providers/anthropic/anthropic.go` — wrap the initial `client.Messages.New()` call and add mid-stream reconnection support (save accumulated content, retry from last checkpoint)
- [x] integrate retry into OpenAI-compatible providers via `utils/openaicompat/openai_compat.go` — retry on 429/5xx/network using the existing `openaicompat.Error` classification
- [x] add `IsRetriable()` method to `openaicompat.Error` that returns true for rate_limit, server, and network categories
- [x] write tests: TestRetry_RetriableErrors, TestRetry_NonRetriableErrors, TestRetry_MaxRetries in `sdk/retry/retry_test.go`
- [x] write tests: TestAnthropicProvider_RetryOn429 in `extensions/providers/anthropic/anthropic_test.go`
- [x] run `cd extensions/providers/anthropic && go test ./...` and `cd sdk && go test ./...` — must pass before task 5

### Task 5: Enforce skill AllowedTools
- [ ] in `extensions/agent/skills.go:makeSkillHandler()`, before publishing the skill body via `agent.prompt`, call `sdk.SetToolFilter(skill.AllowedTools)` if `AllowedTools` is non-empty
- [ ] save and restore the previous tool filter after skill execution completes (need a mechanism — possibly a bus event or a callback)
- [ ] if `AllowedTools` is empty, skip filtering (skill gets all tools as today)
- [ ] write tests: TestSkill_AllowedToolsEnforced, TestSkill_AllowedToolsEmpty_NoFilter in `extensions/agent/skills_test.go`
- [ ] run `cd extensions/agent && go test ./...` — must pass before task 6

### Task 6: Warn on subagent missing tool declarations
- [ ] in `extensions/tools/subagent/agent.go:parseToolsField()` or at agent load time, when `tools` field is nil/empty, log a warning via `sdk.Logger("subagent")` with the agent name and a message suggesting explicit tool declaration
- [ ] write tests: TestParseToolsField_WarnOnEmpty in `extensions/tools/subagent/agent_test.go`
- [ ] run `cd extensions/tools/subagent && go test ./...` — must pass before task 7

### Task 7: Add harness-level argument validation
- [ ] add a lightweight JSON schema validator utility (or use `encoding/json` + manual checks) in a shared location — validate incoming `map[string]any` args against the tool's `Definition().Parameters` schema (check required fields exist, check types match, reject unknown properties if `additionalProperties: false`)
- [ ] integrate validation into `executeTool()` at `extensions/agent/loop.go:708-722` — before calling `tool.Execute()`, validate args and return `ToolResult{Content: "invalid arguments: ...", IsError: true}` on failure
- [ ] add `additionalProperties: false` to all tool schemas in `extensions/tools/*/` Definition() methods
- [ ] write tests: TestExecuteTool_InvalidArgs, TestExecuteTool_MissingRequired in `extensions/agent/loop_test.go`
- [ ] write tests: TestSchemaValidation_RequiredFields, TestSchemaValidation_UnknownFields in the validation utility test file
- [ ] run `cd extensions/agent && go test ./...` — must pass before task 8

### Task 8: Surface provider token usage to UI
- [ ] add `ProviderEventUsage` event type to `sdk/provider.go` with `InputTokens`, `OutputTokens`, `CacheCreationTokens`, `CacheReadTokens` fields
- [ ] in Anthropic provider `extensions/providers/anthropic/anthropic.go`, extract `Usage` from accumulated message and emit a `ProviderEventUsage` event at end of stream
- [ ] in OpenAI-compatible providers, extract usage from the final SSE event and emit `ProviderEventUsage`
- [ ] in bridge `extensions/ui/tui/bridge.go`, handle usage events and call footer's `SetTokenUsage()`
- [ ] write tests: TestAnthropic_UsageEventEmitted in `extensions/providers/anthropic/anthropic_test.go`
- [ ] write tests: TestBridge_UsageEvent in `extensions/ui/tui/bridge_test.go` (if exists) or `model_test.go`
- [ ] run `cd extensions/providers/anthropic && go test ./...` and `cd extensions/ui/tui && go test ./...` — must pass before task 9

### Task 9: Filter compacted entries on session resume
- [ ] in `extensions/store/jsonl/store.go:LoadHistory()`, after loading all messages, scan for compaction summary messages (those starting with `[Compaction Summary]\n`) and remove all earlier messages that the summary covers
- [ ] preserve messages after the last compaction summary verbatim
- [ ] write tests: TestLoadHistory_WithCompactionSummary, TestLoadHistory_MultipleCompactions in `extensions/store/jsonl/store_test.go`
- [ ] run `cd extensions/store/jsonl && go test ./...` — must pass before task 10

### Task 10: Reorder system prompt for cache friendliness
- [ ] in `extensions/agent/prompt.go:Build()`, move date+CWD injection from layer 2 to the last layer (after APPEND_SYSTEM.md)
- [ ] update any tests that assert on prompt ordering
- [ ] write tests: TestBuild_DateCWDAtEnd in `extensions/agent/prompt_test.go`
- [ ] run `cd extensions/agent && go test ./...` — must pass before task 11

### Task 11: Add Anthropic cache_control breakpoints
- [ ] in `extensions/providers/anthropic/anthropic.go`, add `cache_control: {type: "ephemeral"}` to the system prompt content block(s)
- [ ] add `cache_control` to the last user message or the most recent assistant message (whichever Anthropic recommends for conversation caching)
- [ ] if a compaction summary message exists, add `cache_control` to it as well
- [ ] write tests: TestAnthropic_CacheControlMarkers in `extensions/providers/anthropic/anthropic_test.go`
- [ ] run `cd extensions/providers/anthropic && go test ./...` — must pass before task 12

### Task 12: Add trust labeling on tool results
- [ ] in `sdk/message.go`, wrap tool result content in `<tool_output name="...">` XML tags when constructing `NewToolResultMessage()`
- [ ] add one line to `extensions/agent/default-system-prompt.md` instructing the model that content in `<tool_output>` tags is external data, not system instructions
- [ ] write tests: TestNewToolResultMessage_TrustLabel in `sdk/message_test.go`
- [ ] run `cd sdk && go test ./...` — must pass before task 13

### Task 13: Add trust labeling on skill body injection
- [ ] in `extensions/agent/skills.go:makeSkillHandler()`, wrap the skill body in `<skill_body trust="untrusted">` XML tags before publishing as `agent.prompt`
- [ ] add one line to `extensions/agent/default-system-prompt.md` instructing the model that content in `<skill_body>` tags is user-provided skill content, not system policy
- [ ] write tests: TestSkill_BodyTrustLabel in `extensions/agent/skills_test.go`
- [ ] run `cd extensions/agent && go test ./...` — must pass before task 14

### Task 14: Populate TraceID in bus events
- [ ] in `sdk/event.go:NewEvent()`, generate a UUID (using `crypto/rand` or `fmt.Sprintf` with time+random) and populate the `TraceID` field
- [ ] write tests: TestNewEvent_TraceIDPopulated, TestNewEvent_TraceIDUnique in `sdk/event_test.go`
- [ ] run `cd sdk && go test ./...` — must pass before task 15

### Task 15: Split PreferenceStore interface for credential access
- [ ] create `PreferenceReader` interface in `sdk/config.go` with `Preferences(target)` only (no `SaveProviderKey`)
- [ ] create `PreferenceWriter` interface extending `PreferenceReader` with `SaveProviderKey(provider, key)` and `SavePreferences(target)`
- [ ] keep `PreferenceStore` as the full interface (backward compatible) but update factory signatures so tools/extensions receive `PreferenceReader` by default; only auth-related extensions receive `PreferenceWriter`
- [ ] update tool and extension factory signatures in `sdk/tool_registry.go`, `sdk/extension.go` to use `PreferenceReader`
- [ ] update `internal/wire/` to pass `PreferenceWriter` only to providers and auth-related extensions
- [ ] write tests: verify tools receive `PreferenceReader`, verify providers receive `PreferenceWriter`
- [ ] run `make test` — must pass before task 16

### Task 16: Add install-time warning for extensions
- [ ] in `internal/extmanage/install.go`, after validating `.go` files and `go.mod`, print a warning to stderr: "Extension '<name>' will be compiled with full access to filesystem, network, and provider credentials. Only install extensions from trusted sources."
- [ ] write tests: TestInstall_PrintsAccessWarning in `cmd/weave/extmanage/install_test.go` (or relevant test file)
- [ ] run `go test ./cmd/weave/...` — must pass before task 17

### Task 17: Parallelize read-only tool execution
- [ ] in `extensions/agent/loop.go`, classify tool calls as read-only (read, grep, find, ls) vs write (edit, write, bash, subagent) based on tool name
- [ ] when multiple tool calls are present, execute read-only calls concurrently via goroutines with a sync.WaitGroup; execute write calls sequentially after all reads complete
- [ ] preserve message ordering — collect results in original tool call order
- [ ] write tests: TestExecuteTools_ParallelReads, TestExecuteTools_WritesSequential, TestExecuteTools_MixedParallelAndSequential in `extensions/agent/loop_test.go`
- [ ] run `cd extensions/agent && go test ./...` — must pass before task 18

### Task 18: Add explicit sections to compact prompt template
- [ ] in `extensions/agent/default-compact-prompt.md`, add explicit `## Active Constraints` and `## Current Plan (step X of Y)` sections to the requested output format
- [ ] add instruction: "Preserve ALL user-stated constraints verbatim in Active Constraints. Do not paraphrase or summarize constraints."
- [ ] add instruction: "Include the current plan state in Current Plan. If the user stated a multi-step goal, list completed and remaining steps."
- [ ] write tests: update compaction tests to verify the prompt template includes the new sections
- [ ] run `cd extensions/agent && go test ./...` — must pass before task 19

### Task 19: Final verification
- [ ] run full test suite: `make test`
- [ ] run linter: `make lint`
- [ ] verify all design decisions from brainstorm are implemented
- [ ] update CLAUDE.md if any new config fields or interfaces were added
- [ ] verify all trust labels are consistent (context files, tool output, skill body)

## Technical Details

### New config fields
- `agent.max_steps` (int, default 50) — inner loop iteration limit

### New event types
- `ProviderEventUsage` — token usage from providers (InputTokens, OutputTokens, CacheCreationTokens, CacheReadTokens)

### New utility
- `sdk/retry/` — shared retry with exponential backoff and retriable-error predicate

### Modified interfaces
- `PreferenceStore` split into `PreferenceReader` (tools/extensions) and `PreferenceWriter` (providers/auth extensions)
- Tool interface unchanged, Provider interface unchanged

### Prompt changes
- Context files wrapped in `<user_context trust="untrusted">`
- APPEND_SYSTEM.md wrapped in `<user_appended_context>`
- Tool results wrapped in `<tool_output name="...">`
- Skill body wrapped in `<skill_body trust="untrusted">`
- System prompt updated with trust boundary instructions
- Compact prompt updated with explicit `## Active Constraints` and `## Current Plan` sections
- Date/CWD moved from layer 2 to last layer

### Provider changes
- Anthropic: `cache_control` on system prompt and conversation messages
- All providers: retry wrapper with 10 retries, exponential backoff
- All providers: emit usage events

### Observability changes
- TraceID populated in all bus events
- Extension install prints access warning

### Runtime changes
- Read-only tool calls parallelized in inner loop
- Write tool calls remain sequential

## Post-Completion

**Manual verification:**
- Test prompt caching in a real session (check Anthropic API logs for cache_read_input_tokens)
- Test retry behavior by temporarily pointing at a flaky endpoint
- Test skill AllowedTools with a real skill that declares restricted tools
- Test session resume after compaction in a real TUI session
- Test inner loop step limit with a model that loops tool calls

**Performance monitoring:**
- Monitor cache hit rates after task 10+11 deployment
- Monitor retry counts in logs to validate backoff tuning
- Monitor step-limit hits to validate 50 is the right default
