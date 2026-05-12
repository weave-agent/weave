# Kimi Provider

## Overview

Add a built-in provider for [Kimi](https://www.moonshot.cn/) (Moonshot AI) coding models, exposed via the Kimi Coding API at `https://api.kimi.com/coding`. This API uses the Anthropic Messages API format, so we reuse the official Anthropic Go SDK with a custom base URL and headers.

**Reference:** Pi coding agent defines this as provider `kimi-coding` with default model `kimi-for-coding`, API type `anthropic-messages`, base URL `https://api.kimi.com/coding`, and custom `User-Agent: KimiCLI/1.5` header.

**Model to register:**
- `kimi-for-coding` — default, 262K context, 32K max tokens, reasoning enabled

## Context (from discovery)

- **Files/components involved:**
  - New module: `extensions/providers/kimi/` (go.mod, kimi.go, models.go, kimi_test.go)
  - No core files need modification — auto-discovery picks up new extension modules
- **Related patterns found:**
  - Anthropic provider (`extensions/providers/anthropic/`) — identical API format, will be heavily referenced
  - ZAI provider (`extensions/providers/zai/`) — shows custom base URL + header pattern via `openaicompat`
  - Provider registration via `sdk.RegisterProvider` in `init()`
  - Model registration via `model.RegisterModel` in `init()`
  - Env var registration via `model.RegisterProviderEnvVar`
- **Dependencies identified:**
  - `github.com/anthropics/anthropic-sdk-go` — already a dependency of the anthropic provider
  - `weave/sdk`, `weave/sdk/model` — SDK interfaces

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

- **Unit tests:** Required for every task. Test provider initialization, config resolution, model registration, and streaming behavior.
- **Mock Anthropic API server:** Use `httptest` to simulate the Kimi Coding API (Anthropic Messages format) and verify request headers, body, and response parsing.
- **Test coverage:** Provider factory, Stream method, thinking level resolution, custom header injection.

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with `+` prefix
- Document issues/blockers with `⚠️` prefix
- Update plan if implementation deviates from original scope
- Keep plan in sync with actual work done

## Implementation Steps

### Task 1: Create Kimi provider module skeleton
- [x] Create `extensions/providers/kimi/go.mod` with module path `weave/ext/providers/kimi` and replace directive
- [x] Create `extensions/providers/kimi/models.go` with model registration (`kimi-for-coding`)
- [x] Create `extensions/providers/kimi/kimi.go` with provider struct, `init()`, config resolution, and `Stream()` delegation to Anthropic SDK
- [x] Configure Anthropic client with custom base URL (`https://api.kimi.com/coding`) and `User-Agent: KimiCLI/1.5` header
- [x] Add `KIMI_API_KEY` env var support via `model.RegisterProviderEnvVar`
- [x] Add `KIMI_MODEL` env var override support
- [x] Support `providers.kimi` config block (model, max_tokens, base_url overrides)
- [x] write tests for provider initialization (success + missing API key)
- [x] write tests for config resolution (env var, config file, defaults)
- [x] write tests for model registration
- [x] run tests — must pass before next task

### Task 2: Implement streaming with Anthropic SDK
- [x] Implement `Stream()` method reusing Anthropic SDK's `Messages.NewStreaming()`
- [x] Handle text deltas, thinking blocks, tool calls, and errors (same as anthropic provider)
- [x] Map thinking levels to Anthropic output config effort (low/medium/high/xhigh)
- [x] Handle thinking level clamping for models that don't support xhigh
- [x] write tests for streaming with mock Anthropic-compatible server
- [x] write tests for thinking block handling
- [x] write tests for tool call parsing
- [x] write tests for error handling (auth errors, network errors)
- [x] run tests — must pass before next task

### Task 3: Register built-in models and verify integration
- [x] Register `kimi-for-coding` as default model (262K context, 32K max tokens, reasoning enabled)
- [x] Add `KIMI_API_KEY` to provider env var registry
- [x] Verify provider appears in `sdk.ListProviders()` after build
- [x] Verify models appear in `model.ListModelsForProvider("kimi")`
- [x] write tests for model registry entries
- [x] write integration-style test: create provider → stream → verify events
- [x] run full test suite — must pass before next task

### Task 4: Verify acceptance criteria
- [x] verify provider `kimi` is registered and discoverable
- [x] verify `KIMI_API_KEY` resolves correctly (env → auth file → config file)
- [x] verify default model is `kimi-for-coding`
- [x] verify custom base URL and User-Agent header are sent
- [x] verify thinking levels work correctly
- [x] verify tool calls are parsed and emitted
- [x] run full test suite (all providers)
- [x] run linter — all issues must be fixed

### Task 5: Update documentation
- [x] update CLAUDE.md provider section with Kimi env var (`KIMI_API_KEY`) and model info
- [x] update settings.json example with `providers.kimi` config block

## Technical Details

### Provider config resolution (highest to lowest priority)
1. `KIMI_API_KEY` env var
2. `~/.weave/auth.json` `"kimi"` entry
3. `.weave/settings.json` `providers.kimi.api_key`
4. `KIMI_MODEL` env var for model override
5. `providers.kimi.model` in settings
6. Default: `kimi-for-coding`

### Anthropic SDK configuration
```go
client := anthropic.NewClient(
    option.WithAPIKey(apiKey),
    option.WithBaseURL("https://api.kimi.com/coding"),
    option.WithHeader("User-Agent", "KimiCLI/1.5"),
)
```

### Model definitions
| ID | Display Name | Context | Max Tokens | Reasoning | Default |
|---|---|---|---|---|---|
| `kimi-for-coding` | Kimi For Coding | 262144 | 32768 | yes | yes |
| `k2p6` | Kimi K2.6 | 262144 | 32768 | yes | no |
| `kimi-k2-thinking` | Kimi K2 Thinking | 262144 | 32768 | yes | no |

### Request format
Identical to Anthropic Messages API. Thinking enabled via `ThinkingConfigAdaptiveParam` + `OutputConfigParam{Effort: ...}`.

## Post-Completion

**Manual verification:**
- Obtain a Kimi API key and test live streaming
- Verify thinking blocks render correctly in TUI
- Verify tool calls execute correctly

**No external system updates required.**
