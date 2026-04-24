# Provider System v2

## Overview

Comprehensive overhaul of the provider, model, thinking level, and model switching systems inspired by pi's reference implementation. Fixes the critical model switching bug (model name ignored), adds thinking level support with per-request provider options, introduces a model registry with metadata, and refreshes the TUI with thinking level display, editor border colors, and improved model selector.

**Key benefits:**
- Model switching actually works (including within same provider)
- Thinking levels (off/minimal/low/medium/high/xhigh) with visual feedback
- Per-request model and thinking options — no more re-creating providers on switch
- Rich model metadata for future features (cost tracking, capability detection)
- TUI parity with pi for core provider/model interactions

## Context

**Files involved:**
- `sdk/provider.go` — Provider interface, ProviderRequest, ProviderEvent (will extend)
- `sdk/provider_registry.go` — Provider factory registry (minor update)
- `sdk/config.go` — Config interface (add thinking level)
- `extensions/loop/loop.go` — Agent loop, model change handling (fix bug, pass options)
- `extensions/providers/anthropic/anthropic.go` — Anthropic provider (add thinking)
- `extensions/providers/openai/openai.go` — OpenAI provider (add reasoning_effort)
- `extensions/providers/zai/zai.go` — ZAI provider (add enable_thinking)
- `extensions/providers/openai-compat/openai_compat.go` — Shared compat layer (add thinking)
- `extensions/ui/tui/models.go` — Model list/cycling (use registry)
- `extensions/ui/tui/model.go` — TUI root model (thinking actions, status messages)
- `extensions/ui/tui/bridge.go` — Bus event translation (thinking events)
- `extensions/ui/tui/components/footer.go` — Footer (thinking level display)
- `extensions/ui/tui/components/editor.go` — Editor (border color)
- `extensions/ui/tui/keybindings.go` — Add thinking cycle binding
- `extensions/ui/tui/commands.go` — Add /thinking command

**New files:**
- `sdk/model.go` — ModelDef, ThinkingLevel, model registry

**Patterns to reuse:**
- `sdk.RegisterProvider` / `sdk.GetProvider` pattern for model registry
- Bus event publish/subscribe for thinking level changes (same as model.change)
- `BindingRegistry` for new `app.thinking.cycle` action
- `SelectorModel` overlay for enhanced model selector
- `FooterModel` immutable setter pattern for new fields

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**
- Maintain backward compatibility — existing configs must work unchanged
- Run `make fix` before manual fixes, `make gen` after interface changes

## Implementation Steps

### Task 1: Add model types and registry to SDK
Create the foundational types: `ModelDef` (model metadata), `ThinkingLevel` (6 levels with budgets), `StreamOptions` (per-request options passed to providers). Add a model registry with curated entries for the 3 existing providers.

- [ ] create `sdk/model.go` with: `ModelDef` struct (ID, Provider, DisplayName, Reasoning bool, SupportsXHigh bool, ContextWindow int, MaxTokens int, Cost *ModelCost); `ThinkingLevel` type with 6 levels (off/minimal/low/medium/high/xhigh); `ThinkingBudgets` map (minimal:1024, low:2048, medium:8192, high:16384, xhigh:32768); `StreamOptions` struct (Model string, ThinkingLevel, MaxTokens int64); `ProviderEventThinking = "thinking_delta"` event type; model registry functions `RegisterModel`, `GetModel`, `ListModelsForProvider`, `ListAllModels`, `DefaultModelForProvider`; curated models: anthropic (claude-opus-4-20250514: Reasoning=true, SupportsXHigh=true; claude-sonnet-4-20250514: Reasoning=true, SupportsXHigh=false), openai (gpt-4o, gpt-4o-mini), zai (glm-4, glm-4-flash)
- [ ] add `ThinkingLevel` field to `ProviderConfigEntry` in `sdk/config.go`
- [ ] write tests for model registry (register, get, list, default), ThinkingLevel budget lookup, StreamOptions defaults, SupportsXHigh clamping
- [ ] run tests - must pass before task 2

### Task 2: Extend Provider interface with StreamOptions
Change `Provider.Stream()` to accept `StreamOptions`. Update all 3 provider implementations and the loop to pass options. This is the core decoupling that fixes model switching.

- [ ] update `Provider` interface in `sdk/provider.go`: change `Stream(ctx, ProviderRequest)` to `Stream(ctx, ProviderRequest, ...StreamOption)` using variadic functional options pattern (`type StreamOption func(*StreamOptions)`); add `NewStreamOptions(opts ...StreamOption) *StreamOptions` that defaults Model from provider, ThinkingLevel to ThinkingOff, MaxTokens from provider config; add `WithModel(model)`, `WithThinkingLevel(level)`, `WithMaxTokens(n)` option constructors
- [ ] update `extensions/providers/anthropic/anthropic.go`: accept model from StreamOptions (fallback to p.model), add thinking config to `anthropic.MessageNewParams` — adaptive thinking with effort mapping (minimal→"low", low→"low", medium→"medium", high→"high", xhigh→"xhigh"), emit `ProviderEventThinking` for thinking content blocks from streaming events; clamp xhigh→high if model doesn't support xhigh
- [ ] update `extensions/providers/openai-compat/openai_compat.go`: accept model from StreamOptions, add `reasoning_effort` field to `ChatRequest` when thinking is enabled (effortMap: minimal→"low", low→"low", medium→"medium", high→"high"), emit thinking content from `delta.reasoning_content` or `delta.reasoning` fields
- [ ] update `extensions/providers/openai/openai.go` and `extensions/providers/zai/zai.go`: pass StreamOptions through to openaicompat.Stream, ZAI uses `enable_thinking` compat flag
- [ ] update `extensions/loop/loop.go` `streamTurn()`: accept and pass StreamOptions with current thinking level and model
- [ ] run `make gen` to regenerate mocks, fix compile errors
- [ ] write tests for StreamOptions defaults, functional option pattern, anthropic thinking level → SDK param mapping (including xhigh clamp for non-supporting models)
- [ ] write tests for openaicompat thinking → request body mapping
- [ ] run tests - must pass before task 3

### Task 3: Fix model switching and add thinking level propagation
Fix the critical bug where `applyModelChange()` ignores the model name. Add thinking level state to the loop and propagate changes via bus events.

- [ ] fix `applyModelChange()` in `extensions/loop/loop.go`: extract both `provider` and `model` from event payload, store current model name in Loop struct, pass it via StreamOptions (no longer re-creating provider for same-provider model changes)
- [ ] add thinking level state to Loop struct (`thinkingLevel ThinkingLevel`, default `ThinkingMedium`), add `TopicThinkingChange = "thinking.change"` bus topic, subscribe in `run()`, apply thinking level changes and drain like model changes
- [ ] update `streamTurn()` to extract thinking content from `ProviderEventThinking` events and include `"thinking"` key in `TopicMsgEnd` payload
- [ ] write tests for `applyModelChange` with model key (was the bug), thinking level application, drain functions, xhigh clamping on model switch
- [ ] run tests - must pass before task 4

### Task 4: TUI — thinking level display and editor border colors
Add thinking level to the TUI footer, implement pi-style editor border colors that change with thinking level, and add the Shift+Tab cycling keybinding.

- [ ] create `extensions/ui/tui/palette/thinking.go` with `ThinkingBorderColor(level ThinkingLevel) string` mapping: off→"240" (dark gray), minimal→"246" (medium gray), low→"67" (steel blue), medium→"99" (light blue-purple), high→"139" (muted purple), xhigh→"177" (bright magenta)
- [ ] update `extensions/ui/tui/components/footer.go`: add `thinkingLevel string` field, `SetThinkingLevel(level string) FooterModel` setter, append ` · {level}` after model in `renderLine2()` — e.g. `anthropic/claude-sonnet · medium`; show level only when model has `Reasoning=true`
- [ ] update `extensions/ui/tui/components/editor.go`: add `BorderColor string` field (default `"63"`), `SetBorderColor(color string) EditorModel` setter, use `BorderColor` instead of hardcoded `"63"` in `View()`'s `borderStyle`
- [ ] add `ActionThinkingCycle BindingAction = "app.thinking.cycle"` to `extensions/ui/tui/keybindings.go` with default keys `["shift+tab"]`, description "Cycle thinking level"
- [ ] update `extensions/ui/tui/model.go`: add `thinkingLevel sdk.ThinkingLevel` field (init from `WEAVE_THINKING_LEVEL` env var, default medium); handle `ActionThinkingCycle` in `dispatchBinding()` — cycle through levels (clamp xhigh to high for models without SupportsXHigh), update footer, update editor border color, publish `TopicThinkingChange`, show status message `"Thinking level: {level}"`
- [ ] write tests for thinking level cycling logic, border color mapping (all 6 levels), footer rendering with thinking level, xhigh clamping for non-supporting models
- [ ] run tests - must pass before task 5

### Task 5: TUI — model selector enhancement and status messages
Improve the model selector with [provider] badges and current model marker. Add status messages for model cycling. Update model list to use the registry.

- [ ] update `extensions/ui/tui/models.go`: replace `knownModels` hardcoded map with `sdk.ListAllModels()`, update `listModels()` to build entries from registry with ModelDef metadata, update `resolveModelName()` to look up default from registry
- [ ] update model selector in `extensions/ui/tui/model.go`: build `SelectorItem` entries with `Title = modelDef.DisplayName + " ✓"` for current model and `modelDef.DisplayName` otherwise, `Subtitle = "[" + provider + "]"` for all entries
- [ ] add transient status message system to root Model: `statusMsg string` field, `statusTimer tea.Cmd`, render status line above editor for 2s after model cycle or thinking cycle (e.g. `"Switched to Claude Opus (thinking: medium)"`); show `"Only one model available"` if cycling with single model
- [ ] write tests for model list from registry, status message display, selector entry formatting with badges
- [ ] run tests - must pass before task 6

### Task 6: TUI — startup hints and /thinking command
Add a startup keybinding hints banner and a `/thinking` slash command.

- [ ] add `/thinking` command to `extensions/ui/tui/commands.go`: description "Set thinking level (off/minimal/low/medium/high/xhigh)", parse arg as ThinkingLevel, validate, apply change same as cycling
- [ ] add startup hints banner in `extensions/ui/tui/model.go` `View()`: on first render (before first user message), show single dim line: `"ctrl+p cycle model · ctrl+l select model · shift+tab cycle thinking · ctrl+t toggle thinking"`, dismiss on first keypress
- [ ] update `CLAUDE.md` to document ThinkingLevel, StreamOptions, model registry, new keybindings
- [ ] run full test suite (`go test ./...`)
- [ ] run linter (`make lint`) — fix any issues
- [ ] run `make fix` for formatting

## Technical Details

### ThinkingLevel enum
```go
type ThinkingLevel string

const (
    ThinkingOff     ThinkingLevel = "off"
    ThinkingMinimal ThinkingLevel = "minimal"
    ThinkingLow     ThinkingLevel = "low"
    ThinkingMedium  ThinkingLevel = "medium"
    ThinkingHigh    ThinkingLevel = "high"
    ThinkingXHigh   ThinkingLevel = "xhigh"
)

var ThinkingBudgets = map[ThinkingLevel]int{
    ThinkingMinimal: 1024,
    ThinkingLow:     2048,
    ThinkingMedium:  8192,
    ThinkingHigh:    16384,
    ThinkingXHigh:   32768,
}

var AllThinkingLevels = []ThinkingLevel{
    ThinkingOff, ThinkingMinimal, ThinkingLow,
    ThinkingMedium, ThinkingHigh, ThinkingXHigh,
}

// ClampForModel returns the level capped to what the model supports.
// If level is xhigh and model doesn't support it, returns high.
func ClampForModel(level ThinkingLevel, model ModelDef) ThinkingLevel {
    if level == ThinkingXHigh && !model.SupportsXHigh {
        return ThinkingHigh
    }
    return level
}
```

### StreamOptions functional options
```go
type StreamOption func(*StreamOptions)

type StreamOptions struct {
    Model         string
    ThinkingLevel ThinkingLevel
    MaxTokens     int64
}

func WithModel(model string) StreamOption {
    return func(o *StreamOptions) { o.Model = model }
}

func WithThinkingLevel(level ThinkingLevel) StreamOption {
    return func(o *StreamOptions) { o.ThinkingLevel = level }
}
```

### Anthropic thinking integration
```go
// In Stream(), based on ThinkingLevel:
effortMap := map[ThinkingLevel]string{
    ThinkingMinimal: "low",
    ThinkingLow:     "low",
    ThinkingMedium:  "medium",
    ThinkingHigh:    "high",
    ThinkingXHigh:   "xhigh",
}
// For adaptive thinking (Opus 4.6+, Sonnet 4.6+):
params.Thinking = anthropic.ThinkingParam{
    Type: "adaptive",
    // effort mapped from level
}
```

### Editor border color ramp
```
off     → 240 (dark gray)
minimal → 246 (medium gray)
low     → 67  (steel blue)
medium  → 99  (light blue-purple) — current default
high    → 139 (muted purple)
xhigh   → 177 (bright magenta)
```

### Bus event additions
- `TopicThinkingChange = "thinking.change"` — payload: `{"level": "medium"}`
- `ProviderEventThinking = "thinking_delta"` — thinking content from provider

### Model registry curated entries
```go
// Anthropic
{"claude-opus-4-20250514", DisplayName: "Claude Opus 4", Reasoning: true, SupportsXHigh: true, ContextWindow: 200000, MaxTokens: 16384}
{"claude-sonnet-4-20250514", DisplayName: "Claude Sonnet 4", Reasoning: true, SupportsXHigh: false, ContextWindow: 200000, MaxTokens: 16384}
// OpenAI
{"gpt-4o", DisplayName: "GPT-4o", Reasoning: false, ContextWindow: 128000, MaxTokens: 16384}
{"gpt-4o-mini", DisplayName: "GPT-4o Mini", Reasoning: false, ContextWindow: 128000, MaxTokens: 16384}
// ZAI
{"glm-4", DisplayName: "GLM-4", Reasoning: false, ContextWindow: 128000, MaxTokens: 8192}
{"glm-4-flash", DisplayName: "GLM-4 Flash", Reasoning: false, ContextWindow: 128000, MaxTokens: 8192}
```

## Post-Completion

**Manual verification:**
- Launch TUI and verify footer shows thinking level after model name
- Cycle thinking level with Shift+Tab, confirm border color changes through all 6 levels and status message appears
- Switch models with Ctrl+P, confirm status message shows new model + thinking level
- Switch models with Ctrl+L selector, confirm [provider] badges and ✓ marker work
- Test `/thinking high` and `/thinking xhigh` commands
- Verify startup hints banner appears and dismisses on first keypress
- Test model switching within same provider (e.g. anthropic/sonnet → anthropic/opus) — was the bug
- Test xhigh clamping: set xhigh, switch to Sonnet (no xhigh support), confirm drops to high
- Test with Anthropic extended thinking — confirm thinking blocks appear in chat
- Test with OpenAI reasoning_effort — confirm no errors
- Test backward compat: existing `.weave.yaml` with no thinking config should work unchanged (defaults to medium)
