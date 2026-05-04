# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A coding agent framework written in Go — event-driven, extension-based, with dynamic compilation of selected extensions at runtime. Agent-loop, providers (Anthropic, OpenAI, Z.ai), tools (bash, read, edit, write, grep, find, ls), and a terminal UI (TUI) are implemented as independent extension modules.

## Commands

```bash
make lint          # Run golangci-lint
make fmt           # Format code (gofumpt, goimports, go fix) ALWAYS use make fix before manual fixing
make fix           # Auto-fix linter issues
make gen           # Regenerate mocks (moq)
make tools         # Install dev tools (moq, golangci-lint)
make bench         # Run build benchmarks (cold/warm/partial, with and without TUI)
make test          # Run all tests (root + all extension modules)
go test ./launcher/...  # Run tests for a single package
cd extensions/loop && go test ./...  # Run tests for a single extension module (must cd first)
```

## Testing

- **Assertions**: Use `github.com/stretchr/testify` — `require` for fatal assertions (prerequisite failures, nil deref risk), `assert` for non-fatal checks. Never use raw `t.Error`/`t.Fatal`.
- **Mocks**: Use moq-generated mocks exclusively. Run `make gen` after changing interfaces. Mocks live in `*_mock_test.go` files — never edit them by hand.
- **go:generate**: Each SDK interface file has a `//go:generate moq ...` directive. Cross-package mocks (e.g., in `extensions/loop/`) use `-skip-ensure -pkg <pkg>`.
- **No hand-written mocks**: If a mock needs custom behavior (scripted responses, call recording), set the mock's `Func` fields or write a helper function that configures a moq mock — never create a new mock struct.

## Architecture

Standard library as much as possible. Every replaceable component is an extension (runner, provider, tools, store, hooks). Extensions are independent Go modules that self-register via `init()`. Extension modules have their own `go.mod` — test/lint them by `cd`ing into the directory, not via path from root (e.g. `go test ./extensions/loop/...` won't work).

**Launcher pattern:** resolve config → pick extensions → build a custom binary (cached per hash) → exec it. The `cmd/weave/main.go` entry point orchestrates this pipeline.

**Key packages:**
- `sdk/` — defines `Extension`, `Bus`, `Config`, `UI` interfaces; `Config` includes `ToolConfig(name, target)` and `UIConfig(target)` for typed settings population; global registries for extensions, providers, tools, and UIs (`RegisterExtension`/`GetExtension`, `RegisterProvider`/`GetProvider`, `RegisterTool`/`GetTool`, `RegisterUI`/`GetUI`); `Message` types; `Wire()` and `WireWithCore()` composition roots that resolve names and subscribe extensions to the bus; `NoopUI` stub for headless mode
- `bus/` — channel-based pub/sub event bus (`Publish`/`Subscribe`/`SubscribeAll`) with buffered channels and graceful close
- `config/` — config discovery (walks up from cwd for `.weave.yaml` or `.weave/config.yaml`) and loading via gonfig. Config has a `core` section (agent_loop + providers) and `extensions` list. `FullConfig` implements `sdk.Config` with key resolution, typed tool/UI config via layered settings, and validation. `settings.go` — `Settings` struct with UI/Tools sections, load/save with layer support. `merge.go` — `MergeSettings` deep-merges global → project → local layers. `validation.go` — `ValidateWithConfigDir` checks config fields with structured field-path errors.
- `internal/truncate/` — shared output truncation (2000 lines / 50KB) used by all tools for consistent output limiting
- `extensions/loop/` — core extension implementing the two-level while-loop agent cycle (outer: follow-ups, inner: steering + tool calls); subscribes to `agent.prompt`, `agent.steer`, `agent.followup`, `model.change`, `thinking.change`; publishes `agent.turn_start/end`, `agent.message_start/update/end`, `agent.tool_result`, `agent.end`
- `extensions/tools/{bash,read,edit,write,grep,find,ls}/` — individual tool extension modules, each an independent Go module self-registering via `sdk.RegisterTool`
- `extensions/providers/openai-compat/` — shared library for OpenAI-compatible providers (SSE parsing, message/tool conversion); reused by `openai` and `zai` providers; import as `openaicompat` package
- `extensions/providers/{anthropic,openai,zai}/` — provider extension modules; Anthropic uses official SDK, OpenAI and Z.ai delegate to `openai-compat`
- `extensions/store/jsonl/` — session persistence extension; subscribes to bus events and writes JSONL files to `~/.weave/sessions/`; implements Create, Append, Load, History, List, Compact internally with no SDK interface
- `extensions/ui/tui/` — interactive terminal UI extension built with Bubble Tea v2 (`charm.land/bubbletea/v2`) + Ultraviolet screen buffers for `Draw()` rendering. Key handling uses `tea.KeyPressMsg` (not v1 `tea.KeyMsg`). Components use `lipgloss.NewStyle()` from `charm.land/lipgloss/v2`. Screen buffer rendering pattern: `Model.View()` creates an `uv.NewScreenBuffer(w,h)`, delegates to `Model.Draw()` which computes layout via `LayoutEngine` then draws each component, and returns `uv.TrimSpace(canvas.Render())`. Components implement `Draw(scr uv.Screen, area uv.Rectangle)` alongside retained `View() string` (used internally for string-to-buffer conversion via `uv.NewStyledString`). Uses `github.com/charmbracelet/ultraviolet` for `uv.Screen`, `uv.Rectangle`, `uv.layout` splitting. Self-registers via `sdk.RegisterExtension("tui", ...)` and `sdk.RegisterUI("tui", ...)`; implements `sdk.UI` interface for cross-extension integration (popups, status bar, slash commands, keybindings); bridge goroutine translates bus events to Bubble Tea messages; includes: streaming chat with progressive markdown rendering (Glamour with custom xchroma formatter), dialog stack for layered overlays, landing state before first prompt, smart auto-scroll with new-content indicator, token rate display in footer, tool output panels, thinking blocks, diff highlighting, multi-line editor (bubbles/v2 textarea) with history, file attachments with paste detection, pills bar for tool progress, slash commands, session/model selectors, and configurable keybindings. UI extensions (see below) are wired at startup via `sdk.GetUIExtensions()`
  - `components/chat.go` — chat viewport with auto-scroll tracking and scroll indicator
  - `components/editor.go` — multi-line editor wrapping `bubbles/v2 textarea.Model` with history navigation, external editor support (`Ctrl+G`), and dynamic height (3–15 lines)
  - `components/footer.go` — two-line status bar with CWD, git, tokens, model, thinking level, token rate
  - `components/spinner.go` — streaming activity indicator (bubbles/v2 spinner)
  - `components/attachments/` — file attachment model: tracks `[]Attachment` with paste detection (>10 newlines or >1000 chars auto-converts to temp file), inline display above editor (`file.py (42 lines)`), `Ctrl+R` delete mode
  - `components/messages/` — message renderers (assistant with progressive markdown, user, tool, thinking, diff, markdown) with shared `drawView` helper for screen buffer output; code blocks use custom "weave" Chroma formatter from `xchroma/`
  - `components/overlays/` — dialog components (selector, confirm, input), `Dialog` interface (`ID`/`Update`/`Draw`/`Handles`/`Done`/`Result`/`SetSize`), adapter types (`SelectorDialog`, `ConfirmDialog`, `InputDialog`), and `DialogStack` for layered overlay management
  - `xchroma/` — custom Chroma formatter registered as "weave"; maps token types (Keyword, String, Comment, Number, Operator) to Lip Gloss v2 styles with forced background matching chat bubble
  - `layout.go` — `LayoutEngine` computes `uv.Rectangle` regions (header, main, pills, editor, footer) via `uv.layout` splitting; stored directly on Model
  - `landing.go` — landing screen with ASCII logo, model info, keybinding hints (shown before first prompt, re-shown on `/clear`/`/new`)
  - `keybindings.go` — key resolution using v2 API (`keyString(msg tea.KeyPressMsg)`, `msg.Key.String()`)
  - `overlays.go` — overlay request routing, dialog stack integration for `sdk.UI` methods
  - `bridge.go` — bus-to-Tea message translation with delta batching and token rate calculation
- `launcher/` — full pipeline: `Discover` extensions (project-local `.weave/extensions/{name}/`, global `~/.weave/extensions/{name}/`, then built-in under `extensions/{category}/{name}/` with nested lookup, plus TUI extensions at `extensions/ui/tui/extensions/{name}/`); path-based entries (`./`, `../`, `/`, `~` prefixed) resolve directly instead of going through name-based discovery; `ComputeHash` of .go files, `Cache` in `~/.weave/bin/{hash}/`, `Build` by generating go.mod+main.go with blank imports, then `syscall.Exec`

**Extension lifecycle:** Extension packages call `sdk.RegisterExtension(name, factory)` in `init()`. Provider, tool, and UI extensions similarly call `sdk.RegisterProvider`, `sdk.RegisterTool`, and `sdk.RegisterUI`. UI extensions (TUI-specific plugins) call `sdk.RegisterUIExtension` in `init()` and are wired by the TUI at startup via `sdk.GetUIExtensions()`. The built binary blank-imports selected extensions, triggering registration. `sdk.Wire()` or `sdk.WireWithCore()` resolves names from registries and subscribes each to the bus. When no prompt is provided (`-p` flag unset), the TUI extension is included in the build for interactive mode; with `-p`, weave runs in print mode without TUI. UI extensions are silently skipped in headless mode.

## Configuration

All config loading uses `github.com/nniel-ape/gonfig` (user-owned lib). No direct `yaml.v3` imports — gonfig handles all file parsing internally. Config files are `.weave.yaml` or `.weave/config.yaml`, discovered by walking up from cwd. Extensions define their own typed config structs with gonfig tags (`default`, `description`, `env`, `validate`) and call `gonfig.Load` themselves with `WithFile` + `WithEnvPrefix("WEAVE")`.

`sdk.Config` is the carrier interface (`FilePath()`, `ProviderConfig()`, `ResolveKey()`, `ToolConfig()`, `UIConfig()`) — extensions use it to load their own config via gonfig or read typed settings sections. `sdk.Wire` takes `[]string` (extension names) directly. `sdk.WireWithCore` takes a `CoreWireConfig` struct (AgentLoop + Providers) alongside optional extension names, merging and deduplicating them.

Config yaml format:
```yaml
core:
  agent_loop: loop       # default: "loop"
  providers:             # default: ["anthropic"]
    - anthropic
ui: tui                  # default: "tui" (interactive). "none" for headless.
extensions:
  - bash
ui_extensions:           # only loaded when ui: tui
  - diff-viewer
```

Example `.weave.yaml` with path-based extensions and per-provider config:
```yaml
core:
  agent_loop: loop
  providers:
    - anthropic
    - openai
ui: tui
extensions:
  - bash
  - jsonl
  - ./my-custom-tool          # relative path: resolved from this file's directory
  - ~/weave-extensions/debug  # tilde: resolved from home directory
ui_extensions:
  - diff-viewer
providers:
  openai:
    model: gpt-5.5
    base_url: https://api.openai.com/v1
```

Corresponding `.weave/settings.json` (project-layer settings):
```json
{
  "model": "claude-sonnet-4-6",
  "thinking_level": "medium",
  "ui": { "editor_max_lines": 20 },
  "tools": { "bash": { "timeout": 60 } }
}
```

**Extension entries** can be bare names or filesystem paths:
- Bare names (`bash`) — resolved through the three-tier discovery hierarchy (project-local `.weave/extensions/`, global `~/.weave/extensions/`, built-in `extensions/`)
- Path entries (`./my-ext`, `../shared/ext`, `/opt/weave-ext`, `~/exts/custom`) — resolved directly: `~` expands to home dir, relative paths resolve from the config file's directory, absolute paths used as-is. Path entries derive their extension name from the directory's base name.

**Config validation** runs automatically on every `LoadFromDir` call via `ValidateWithConfigDir` in `config/validation.go`. It checks: `ui` must be `"tui"` or `"none"`, `core.agent_loop` non-empty, `core.providers` non-empty with valid names, extension entries valid (bare names match `[a-zA-Z0-9_-]+`, paths exist as directories with `.go` files), and provider entries have valid structure. Errors include field paths (e.g. `config.extensions[2]: path "./missing" does not exist`).

**Layered settings** provide persistent user preferences with three layers merged in order (global → project → local):
- Global: `~/.weave/settings.json` — user-wide defaults
- Project: `.weave/settings.json` — walked up from project dir, shared via version control
- Local: `.weave/settings.local.json` — per-developer overrides, auto-added to `.git/info/exclude`

Settings JSON format:
```json
{
  "provider": "anthropic",
  "model": "claude-sonnet-4-6",
  "thinking_level": "medium",
  "ui": {
    "theme": "dark",
    "editor_max_lines": 20
  },
  "tools": {
    "bash": { "timeout": 120 }
  }
}
```

Merge semantics: nested objects merge recursively, primitive values and maps override, later layers take precedence. `LoadLayeredSettings(projectDir)` in `config/merge.go` loads and merges all three. `SaveSettings` accepts a `SettingsLayer` parameter to control which file is written.

**Typed tool config** — tools define config structs and read them via `sdk.Config.ToolConfig(name, &target)`. The config system reads `tools.{name}` from merged settings, JSON round-trips into the target struct, and applies `default` struct tags for zero-value fields. Example: bash tool reads `BashConfig{Timeout int \`json:"timeout" default:"120"\`}` from `tools.bash.timeout` in settings.

**UI config** — TUI reads `UISettings` (theme, editor_max_lines) via `sdk.Config.UIConfig(&target)`. The editor height clamp is set from `editor_max_lines` when present.

**Keybindings** are configured in `.weave/keybindings.yaml` and override built-in defaults (priority: user config > extension registrations > built-in):
```yaml
keybindings:
  app.model.cycle: ["ctrl+p"]
  app.model.select: ["ctrl+l"]
```
Built-in bindings: Escape=interrupt, Ctrl+C=double-press (first clears editor, second exits), Ctrl+D=exit, Ctrl+L=model selector, Ctrl+P=model cycle, Ctrl+N=new session, Ctrl+R=toggle attachment delete mode, Shift+Tab=cycle thinking level, Ctrl+T=toggle thinking blocks, Ctrl+O=expand tool output, Ctrl+G=open external editor, Ctrl+Z=suspend, Shift+G=scroll to bottom.

**Thinking levels** control reasoning depth for providers that support it. Six levels: off, minimal, low, medium (default), high, xhigh. Configured via:
- Shift+Tab cycles through levels (editor border color changes with level)
- `/thinking <level>` slash command sets a specific level
- `WEAVE_THINKING_LEVEL` environment variable for initial level
- Models that don't support xhigh (e.g. Sonnet) are automatically clamped to high

**Model registry** (`sdk/model.go`) provides curated model metadata (display name, reasoning support, context window, max tokens) via `RegisterModel`/`GetModel`/`ListModelsForProvider`/`ListAllModels`. Built-in models are registered by `RegisterBuiltinModels()`.

**StreamOptions** (`sdk/provider.go`) passes per-request options (model, thinking level, max tokens) to providers via functional options: `WithModel(model)`, `WithThinkingLevel(level)`, `WithMaxTokens(n)`. Providers read these instead of re-creating on model switch.

**Provider environment variables:**
- `ANTHROPIC_API_KEY` — required for Anthropic provider (default model: `claude-sonnet-4-6`, override with `ANTHROPIC_MODEL`)
- `OPENAI_API_KEY` — required for OpenAI provider (default model: `gpt-5.5`, override with `OPENAI_MODEL`)
- `ZAI_API_KEY` — required for Z.ai provider (default model: `glm-5.1`, override with `ZAI_MODEL`)
- `WEAVE_THINKING_LEVEL` — initial thinking level (default: `medium`)

## Design Reference

`docs/design.md` is **strong inspiration, not direct instruction**. It captures the architectural intent and data flow, but implementation details will evolve. Treat it as a north star, not a spec to copy verbatim.
