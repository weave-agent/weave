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

**Extension responsibility boundaries** — each extension owns exactly one concern and must not leak into another extension's domain. A tool runs commands, a provider calls an API, the agent loop drives turns, the TUI renders output — no overlap. Extensions communicate exclusively through bus events; they never import or call each other directly. If an extension needs data another extension owns, it listens for the event that publishes it rather than reaching into that extension's internals. When adding code to an extension, ask: is this this extension's job? If not, it belongs somewhere else.

**Launcher pattern:** resolve config → auto-discover extensions → build a custom binary (cached per hash) → exec it. `cmd/weave/main.go` is a thin stub that calls `wire.Run()`.

**Key packages:**
- `sdk/` — defines `Extension`, `Bus`, `Config`, `UI` interfaces; `Handler func(Event) error` type for callback-based bus handlers; `Config` includes `ToolConfig(name, target)`, `UIConfig(target)`, `IsHeadless() bool` for headless mode detection, and `RespectGitignore() bool` for .gitignore support in find/grep tools; `HeadlessConfig` wraps a `Config` and overrides `IsHeadless()` based on whether TUI is included; global registries for extensions, providers, tools, and UIs (`RegisterExtension`/`GetExtension`, `RegisterProvider`/`GetProvider`, `RegisterTool`/`GetTool`, `RegisterUI`/`GetUI`) with duplicate registration warnings (first wins, logs to stderr); `RegisterUIExtension` for TUI-specific plugins; `Sandboxer` interface (`WrapCommand`, `AllowWrite`, `AllowRead`) with package-level getter/setter (`SetSandboxer`/`GetSandboxer`, nil-safe); `Message` types; `NoopUI` stub for headless mode
- `sdk/model/` — model types (`ThinkingLevel`, `ModelDef`, `StreamOptions`), model registry (`RegisterModel`/`GetModel`/`ListModelsForProvider`/`ListAllModels`), provider env var registry (`RegisterProviderEnvVar`/`ProviderEnvVar`), and `RegisterBuiltinModels()` with all hardcoded entries. Model registry warns on duplicates (first wins, matching other registries)
- `sdk/registry/` — generic `Registry[T]` type used internally by all sdk registries; supports `WithWarn` (first-wins + warning) and `WithPanic` (panic on dup) options
- `sdk/wire/` — `Wire()` and `WireWithCore()` composition roots that resolve names and subscribe extensions to the bus; `Run()` absorbs the full entry-point pipeline (config loading, auto-discovery via `launcher.AutoDiscover`, launcher build); `CoreWireConfig` struct (AgentLoop + SingleTurn only, no providers); install subcommand logic
- `bus/` — callback-based event bus (`Publish`/`On`/`OnAll`/`Off`) with per-handler goroutines, non-blocking dispatch, `recover()` wrapping every handler invocation (panics trigger `extension.panic`, errors trigger `extension.error` diagnostic events), and graceful close via `sync.WaitGroup`
- `config/` — config discovery (walks up from cwd for `.weave.yaml` or `.weave/config.yaml`) and loading via gonfig. Config has a `core` section (agent_loop only) and `exclude_extensions` list. `FullConfig` implements `sdk.Config` with key resolution, typed tool/UI config via layered settings, and validation. `settings.go` — `Settings` struct with UI/Tools sections, load/save with layer support. `merge.go` — `MergeSettings` deep-merges global → project → local layers. `validation.go` — `ValidateWithConfigDir` checks config fields with structured field-path errors.
- `internal/truncate/` — shared output truncation (2000 lines / 50KB) used by all tools for consistent output limiting
- `utils/ripgrep/` — shared ripgrep binary detection (`Find()` returns `rg` path or empty string, cached via `sync.OnceValue`); used by find and grep tools for rg-first strategy
- `extensions/loop/` — core extension implementing the two-level while-loop agent cycle (outer: follow-ups, inner: steering + tool calls); subscribes to `agent.prompt`, `agent.steer`, `agent.followup`, `model.change`, `thinking.change`, `skills.loaded`, `instructions.loaded`; combines instructions + skills into system prompt; publishes `agent.turn_start/end`, `agent.message_start/update/end`, `agent.tool_result`, `agent.end`; selects initial provider via priority chain (`WEAVE_PROVIDER` env > settings provider > first registered > `"anthropic"` fallback)
- `extensions/instructions/` — discovers and loads context files (CLAUDE.md/AGENTS.md walked up from project root, then global `~/.weave/`) and system prompt files (SYSTEM.md/APPEND_SYSTEM.md from `.weave/` or `~/.weave/`); publishes assembled prompt on `TopicInstructionsLoaded`; project files override global
- `extensions/tools/{bash,read,edit,write,grep,find,ls}/` — individual tool extension modules, each an independent Go module self-registering via `sdk.RegisterTool`; find and grep use an rg-first pattern (shell out to `rg` when available for .gitignore support and faster searches, fall back to pure Go stdlib when absent), share binary detection via `utils/ripgrep.Find()`, and read `sdk.Config.RespectGitignore()` to toggle .gitignore honoring (default: true); grep supports an `include` glob filter parameter and per-line truncation; find supports `**/` recursive patterns via component-wise segment matching
- `extensions/providers/openai-compat/` — shared library for OpenAI-compatible providers (SSE parsing, message/tool conversion); reused by `openai` and `zai` providers; import as `openaicompat` package
- `extensions/providers/{anthropic,openai,zai}/` — provider extension modules; Anthropic uses official SDK, OpenAI and Z.ai delegate to `openai-compat`
- `extensions/store/jsonl/` — session persistence extension; subscribes to bus events and writes JSONL files to `~/.weave/sessions/`; implements Create, Append, Load, History, List, Compact internally with no SDK interface
- `extensions/sandbox/` — OS-level tool execution guard; wraps bash commands in Seatbelt (macOS) or bubblewrap (Linux) sandbox profiles; enforces path-based access policy on file tools via `Sandboxer` interface; four modes: `off` (no restrictions), `readonly` (no writes), `ask` (prompt per command), `auto` (sandbox wraps all commands); mandatory deny paths hardcoded (sensitive dotfiles, SSH keys, AWS credentials, .env files); subscribes to `sandbox.mode.change` for mid-session mode switching; publishes `sandbox.approve`/`sandbox.approved`/`sandbox.denied`/`sandbox.trust` events for ask-mode approval flow; self-registers via `sdk.RegisterExtension("sandbox", ...)`
- `extensions/ui/sandbox/` — TUI extension for sandbox mode indicator and ask-mode approval dialog; registers `Ctrl+S` keybinding to cycle sandbox modes; displays current mode in footer status pill (`SB:auto`, `SB:off`, etc.); implements `ApproveDialog` with approve/deny/trust-for-session options integrated into TUI `DialogStack` overlay system; self-registers via `sdk.RegisterUIExtension`
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
- `launcher/` — full pipeline: `AutoDiscover` recursively scans project-local `.weave/extensions/`, global `~/.weave/extensions/`, and built-in `extensions/` for Go modules (dirs with `go.mod` + `.go` files); UI extensions detected by `RegisterUIExtension(` in source; deduplicates by name (local > global > built-in); applies `exclude_extensions` list; `ComputeHash` of .go files (includes headless flag for different caches), `Cache` in `~/.weave/bin/{hash}/`, `Build` by generating go.mod+main.go with blank imports (filtering UI extensions for headless), then `syscall.Exec`; generated code imports `weave/sdk/wire` and uses `wire.CoreWireConfig`/`wire.WireWithCore`; passes `WEAVE_LAUNCHER_PATH`, `WEAVE_BUILD_HASH`, and `WEAVE_ORIG_ARGS` env vars for `/reload` support

**Extension lifecycle:** Extension packages call `sdk.RegisterExtension(name, factory)` in `init()`. Provider, tool, and UI extensions similarly call `sdk.RegisterProvider`, `sdk.RegisterTool`, and `sdk.RegisterUI`. Duplicate registrations log a warning (first registration wins). UI extensions (TUI-specific plugins) call `sdk.RegisterUIExtension` in `init()` and are wired by the TUI at startup via `sdk.GetUIExtensions()`. All auto-discovered extensions are blank-imported in the generated binary, triggering registration. `wire.WireWithCore()` (from `sdk/wire/`) resolves names from registries and calls `Subscribe(bus)` on each extension. Extensions register handlers via `bus.On(topic, handler)` or `bus.OnAll(handler)` to receive events; `bus.Off(handler)` removes a handler. Handler panics are caught by the bus and logged as `extension.panic` diagnostic events; errors are logged as `extension.error`. When no prompt is provided (`-p` flag unset), the TUI extension is included in the build for interactive mode; with `-p`, weave runs in print mode without TUI. `IsHeadless()` returns true in print mode, false in interactive mode. UI extensions are silently skipped in headless mode.

## Configuration

All config loading uses `github.com/nniel-ape/gonfig` (user-owned lib). No direct `yaml.v3` imports — gonfig handles all file parsing internally. Config files are `.weave.yaml` or `.weave/config.yaml`, discovered by walking up from cwd. Extensions define their own typed config structs with gonfig tags (`default`, `description`, `env`, `validate`) and call `gonfig.Load` themselves with `WithFile` + `WithEnvPrefix("WEAVE")`.

`sdk.Config` is the carrier interface (`FilePath()`, `ProviderConfig()`, `ResolveKey()`, `ToolConfig()`, `UIConfig()`, `IsHeadless()`) — extensions use it to load their own config via gonfig or read typed settings sections. `IsHeadless()` returns true when running without TUI (print mode); extensions use it to skip UI-dependent work. `HeadlessConfig` wraps a `Config` and overrides `IsHeadless()` based on whether TUI is included. `wire.WireWithCore` takes a `wire.CoreWireConfig` struct (AgentLoop + SingleTurn) without provider or extension lists.

Config yaml format (minimal — extensions are auto-discovered):
```yaml
core:
  agent_loop: loop       # default: "loop"
ui: tui                  # default: "tui" (interactive). "none" for headless.
exclude_extensions:      # optional: skip auto-discovered extensions by name
  - some-extension
providers:               # optional: per-provider settings (not provider selection)
  openai:
    model: gpt-5.5
    base_url: https://api.openai.com/v1
sandbox:                 # optional: sandbox configuration
  mode: auto             # off | readonly | ask | auto (default: auto)
  writable: ["."]        # paths allowed for writes (default: CWD)
  deny_write: []         # additional paths to block writes
  deny_read: []          # additional paths to block reads
  network: true          # allow network access in sandbox (default: true)
```

All extensions are auto-discovered by recursively scanning for Go modules (directories with `go.mod` + `.go` files) in project-local `.weave/extensions/`, global `~/.weave/extensions/`, and built-in `extensions/`. UI extensions (detected by `RegisterUIExtension(` in source) are excluded from headless builds. All providers are compiled in; runtime selection via settings or `WEAVE_PROVIDER` env var.

Corresponding `.weave/settings.json` (project-layer settings):
```json
{
  "model": "claude-sonnet-4-6",
  "thinking_level": "medium",
  "ui": { "editor_max_lines": 20 },
  "tools": { "bash": { "timeout": 60 } }
}
```

**Config validation** runs automatically on every `LoadFromDir` call via `ValidateWithConfigDir` in `config/validation.go`. It checks: `ui` must be `"tui"` or `"none"`, `core.agent_loop` non-empty, `exclude_extensions` entries valid, provider entries have valid structure, and `sandbox` section (if present) has valid `mode` (off/readonly/ask/auto), valid paths in `writable`/`deny_write`/`deny_read`, and boolean `network`. Errors include field paths (e.g. `config.exclude_extensions[0]: invalid extension name`).

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
  "respect_gitignore": true,
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
Built-in bindings: Escape=interrupt, Ctrl+C=double-press (first clears editor, second exits), Ctrl+D=exit, Ctrl+L=model selector, Ctrl+P=model cycle, Ctrl+N=new session, Ctrl+R=toggle attachment delete mode, Shift+Tab=cycle thinking level, Ctrl+T=toggle thinking blocks, Ctrl+O=expand tool output, Ctrl+G=open external editor, Ctrl+Z=suspend, Shift+G=scroll to bottom, Ctrl+S=cycle sandbox mode.

**Thinking levels** control reasoning depth for providers that support it. Six levels: off, minimal, low, medium (default), high, xhigh. Configured via:
- Shift+Tab cycles through levels (editor border color changes with level)
- `/thinking <level>` slash command sets a specific level
- `WEAVE_THINKING_LEVEL` environment variable for initial level
- Models that don't support xhigh (e.g. Sonnet) are automatically clamped to high

**Sandbox modes** control OS-level tool execution guard. Four modes: `off` (no restrictions), `readonly` (writes blocked, file tools denied), `ask` (prompt per bash command via TUI dialog, file tools use path policy, denied in headless), `auto` (default, sandbox wraps bash commands and enforces path policy). Configured via:
- Ctrl+S cycles through modes (footer status pill shows `SB:off`, `SB:readonly`, `SB:ask`, `SB:auto`)
- `.weave.yaml` `sandbox.mode` for initial mode
- Bus events: `sandbox.mode.change` (switch mode), `sandbox.approve`/`sandbox.approved`/`sandbox.denied` (ask-mode approval), `sandbox.trust` (session allowlist pattern)
- Mandatory deny paths are hardcoded (not configurable): writes to `~/.ssh/`, `~/.bashrc`, `~/.zshrc`, `~/.profile`, `~/.gitconfig`, `.git/hooks/`, `.git/config`, `.weave/`; reads from `~/.ssh/id_*`, `~/.aws/credentials`, `**/.env`, `**/.env.*`
- macOS uses Seatbelt (`sandbox-exec`), Linux uses bubblewrap (`bwrap`, must be installed separately)

**Model registry** (`sdk/model/`) provides curated model metadata (display name, reasoning support, context window, max tokens) via `model.RegisterModel`/`model.GetModel`/`model.ListModelsForProvider`/`model.ListAllModels`. Built-in models are registered by `model.RegisterBuiltinModels()`.

**StreamOptions** (`sdk/model/`) passes per-request options (model, thinking level, max tokens) to providers via functional options: `model.WithModel(model)`, `model.WithThinkingLevel(level)`, `model.WithMaxTokens(n)`. Providers read these instead of re-creating on model switch.

**Provider environment variables:**
- `ANTHROPIC_API_KEY` — required for Anthropic provider (default model: `claude-sonnet-4-6`, override with `ANTHROPIC_MODEL`)
- `OPENAI_API_KEY` — required for OpenAI provider (default model: `gpt-5.5`, override with `OPENAI_MODEL`)
- `ZAI_API_KEY` — required for Z.ai provider (default model: `glm-5.1`, override with `ZAI_MODEL`)
- `WEAVE_PROVIDER` — override the active provider at runtime (e.g., `openai`, `zai`); highest priority, overrides settings.json preference
- `WEAVE_THINKING_LEVEL` — initial thinking level (default: `medium`)
- `WEAVE_OFFLINE` — set to `1` to skip the startup extension update check (for offline/air-gapped environments)

**Provider selection priority** (highest to lowest):
1. `WEAVE_PROVIDER` env var (explicit user override)
2. `settings.json` `"provider"` field (persisted user preference)
3. Alphabetically first registered provider (`sdk.ListProviders()[0]`)
4. `"anthropic"` (ultimate fallback)

**Extension management:**
- `weave install <source> [--name <name>]` — install an extension from a git URL, GitHub shorthand, or local path into `~/.weave/extensions/<name>/`
  - `weave install github.com/user/weave-ext-mcp` — clone from GitHub shorthand
  - `weave install https://github.com/user/weave-ext-mcp` — clone from git URL
  - `weave install ./my-local-ext` — copy from local directory
  - `weave install github.com/user/repo --name mcp` — install with explicit name
  - Validates that target directory contains `.go` files; derives extension name from repo basename (without `.git`) unless `--name` is given
- `weave list` — list installed extensions with source type (git/local) and status (ok/outdated/static); checks git-sourced extensions for available updates
- `weave update [<name>]` — update git-sourced extensions via `git pull --ff-only`; no args updates all git-sourced extensions
- `weave uninstall <name>` — remove an extension from `~/.weave/extensions/`; validates extension name exists
- `/reload` — TUI slash command that invalidates the build cache and re-execs the launcher for a full rebuild, picking up extension changes without restarting the terminal

## Design Reference

`docs/design.md` is **strong inspiration, not direct instruction**. It captures the architectural intent and data flow, but implementation details will evolve. Treat it as a north star, not a spec to copy verbatim.
