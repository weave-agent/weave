# Improve Context Window Tracking SDK Support

## Overview
- Add provider-neutral contracts and shared accounting primitives needed for accurate context window tracking.
- Replace ad-hoc usage metadata with reusable structures that providers and the agent can share.
- Extend OpenAI-compatible transport usage parsing so cached-token telemetry is preserved.

## Context (from discovery)
- Files/components involved:
  - `sdk/provider.go`
  - `sdk/model/types.go`
  - `utils/openaicompat/openai_compat.go`
  - related tests in `sdk/*_test.go` and `utils/openaicompat/openai_compat_test.go`
- Related patterns found:
  - Providers stream `sdk.ProviderEventUsage` with `sdk.ProviderUsage`.
  - Model metadata already exposes `ContextWindow` and `MaxTokens`.
  - OpenAI-compatible streaming already requests `stream_options.include_usage`.
- Dependencies identified:
  - Agent extension will consume new SDK accounting contracts.
  - OpenAI, Z.ai, and other OpenAI-compatible providers use `utils/openaicompat`.

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next.
- Make small, focused changes.
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task.
- **CRITICAL: all tests must pass before starting next task** - no exceptions.
- **CRITICAL: update this plan file when scope changes during implementation**.
- Run tests after each change.
- Maintain backward compatibility.

## Testing Strategy
- **Unit tests**: required for every task.
- **E2E tests**: not expected for SDK/transport-only changes.

## Progress Tracking
- Mark completed items with `[x]` immediately when done.
- Add newly discovered tasks with ➕ prefix.
- Document issues/blockers with ⚠️ prefix.
- Update plan if implementation deviates from original scope.
- Keep plan in sync with actual work done.

## What Goes Where
- **Implementation Steps** (`[ ]` checkboxes): tasks achievable within this codebase - code changes, tests, documentation updates.
- **Post-Completion** (no checkboxes): items requiring external action - manual testing, changes in consuming projects, deployment configs, third-party verifications.
- **Checkbox placement**: Checkboxes belong only in Task sections.

## Implementation Steps

### Task 1: Add provider-neutral token count contracts
- [x] add `TokenCounter`, `TokenCount`, and count source/confidence fields in `sdk/provider.go`
- [x] ensure new contracts do not require existing providers to change
- [x] write tests or compile-time assertions for optional interface compatibility
- [x] write tests for zero-value token count behavior if helper methods are added
- [x] run `go test ./sdk/...` - must pass before next task

### Task 2: Add shared request budget metadata
- [x] add SDK types for context budget snapshots including context window, input tokens, output reserve, safety margin, remaining tokens, and percent used
- [x] add helper logic only if it can stay provider-neutral and independent from agent policy
- [x] write tests for budget math including over-budget and zero/unknown window cases
- [x] write tests for percent/remaining rounding edge cases
- [x] run `go test ./sdk/...` - must pass before next task

### Task 3: Parse richer OpenAI-compatible usage data
- [x] extend `utils/openaicompat.Usage` to parse cached prompt token details when present
- [x] map cached prompt tokens into `sdk.ProviderUsage.CacheReadTokens`
- [x] preserve existing `prompt_tokens` and `completion_tokens` behavior
- [x] write tests for usage chunks with and without cached-token details
- [x] run `go test ./utils/openaicompat/...` - must pass before next task

### Task 4: Verify acceptance criteria
- [x] verify SDK additions are optional and backward compatible
- [x] verify OpenAI-compatible providers still stream normal text, tool calls, and usage
- [x] run `go test ./sdk/... ./utils/openaicompat/...`
- [x] run `make lint`
- [x] verify no provider-specific policy leaked into SDK abstractions

### Task 5: Update documentation
- [ ] update `CLAUDE.md` key package notes if SDK contracts change
- [ ] update relevant package docs or README sections if new public APIs need explanation

## Technical Details
- `TokenCounter` should be optional and implemented by providers that can preflight count full provider requests.
- Exact counts should use `Source: "exact"`; tokenizer estimates should use `Source: "tokenizer"`; fallback estimates should use `Source: "heuristic"`.
- Budget math should not decide compaction policy in SDK; agent owns policy.

## Post-Completion

**Manual verification**:
- Confirm downstream extension repos compile against the updated SDK.

**External system updates**:
- Provider repos implementing exact counting should update their SDK dependency after this lands.
