# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A coding agent framework written in Go — event-driven, extension-based, with dynamic compilation of selected extensions at runtime. Agent-loop, providers (Anthropic, OpenAI, Z.ai, Kimi), tools (bash, read, edit, write, grep, find, ls, subagent), and a terminal UI (TUI) are implemented as independent extension modules.

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
- `sdk/` — defines `Extension`, `Bus`, `Config`, `UI` interfaces; `Handler func(Event) error` type for callback-based bus handlers; `Config` includes `FilePath()`, `ProjectDir()`, `ResolveKey(provider, envVar)`, `ExtensionConfig(scope, name, target, envPrefix)`, `IsHeadless()`, `Preferences(target)`, `SavePreferences(target)`, `SaveProviderKey(provider, key)`, and `RespectGitignore() bool`; `HeadlessConfig` wraps a `Config` and overrides `IsHeadless()`; global registries for extensions, providers, tools, and UIs (`RegisterExtension`/`GetExtension`, `RegisterProvider`/`GetProvider`, `RegisterTool`/`GetTool`, `RegisterUI`/`GetUI`) with duplicate registration warnings (first wins, logs to stderr); `RegisterUIExtension`/`GetUIExtensions`/`IsUIExtension` for TUI-specific plugin detection; `Sandboxer` interface (`WrapCommand`, `AllowWrite`, `AllowRead`) with package-level getter/setter (`SetSandboxer`/`GetSandboxer`, nil-safe); `RegisterOutputWriterSetter`/`SetOutputWriters` for generic output writer hooks; `OnAppStarted`/`AppStartedHandlers` for lifecycle event registration; `Message` types; `NoopUI` stub for headless mode
- `sdk/model/` — model types (`ThinkingLevel`, `ModelDef`, `StreamOptions`), model registry (`RegisterModel`/`GetModel`/`ListModelsForProvider`/`ListAllModels`), provider env var registry (`RegisterProviderEnvVar`/`ProviderEnvVar`), and `RegisterBuiltinModels()` with all hardcoded entries. Model registry warns on duplicates (first wins, matching other registries)
- `sdk/registry/` — generic `Registry[T]` type used internally by all sdk registries; supports `WithWarn` (first-wins + warning) and `WithPanic` (panic on dup) options
- `sdk/wire/` — `Wire()` and `WireWithCore()` composition roots that resolve names and subscribe extensions to the bus; `Run()` absorbs the full entry-point pipeline (config loading, auto-discovery via `launcher.AutoDiscover`, launcher build) and delegates extension management subcommands (`install`, `list`, `update`, `uninstall`) to `cmd/weave/extmanage/`; `CoreWireConfig` struct (AgentLoop + SingleTurn only); `WireWithCore()` publishes `app.started` after wiring, sets `WEAVE_SINGLE_TURN` env var when `SingleTurn` is true, and calls `sdk.AppStartedHandlers()` before subscribing extensions; `Wire()` silently skips tools, providers, and UI extensions that appear in the extension names list
- `bus/` — callback-based event bus (`Publish`/`On`/`OnAll`/`Off`) with per-handler goroutines, non-blocking dispatch, `recover()` wrapping every handler invocation (panics trigger diagnostic events via configurable topics), and graceful close via `sync.WaitGroup`
- `internal/auth/` — provider credential storage in `~/.weave/auth.json`; `Load()`/`Save()`/`SetProviderKey()`/`GetProviderKey()`; used by `settings.ResolveProviderKey()` for the full resolution chain (env var → auth file → config file)
- `settings/` — unified JSON-only settings system. Discovers `.weave/settings.json` by walking up from cwd, falling back to `~/.weave/settings.json`. `Settings` struct holds all config fields (agent_loop, ui_extension, providers, sandbox, plus user preferences like provider, model, thinking_level, ui, tools). `FullConfig` implements `sdk.Config` with key resolution and typed extension config via `ExtensionConfig()`. `loader.go` — custom `Loader` struct with `Load(target)` that applies defaults → JSON data → env vars → CLI flags → validation, using struct tags (`default`, `env`, `flag`, `short`, `validate`, `description`). `merge.go` — `MergeSettings` deep-merges global → local layers for tool/UI preferences. `validation.go` — generic field-path validation (no extension-specific values). `ProjectDirFromConfig()` derives project root from config file path for auto-discovery from subdirectories. `help.go` — `GenerateFullHelp()` produces full `--help` output from registered extension schemas.
- `internal/truncate/` — shared output truncation (2000 lines / 50KB) used by all tools for consistent output limiting
- `utils/ripgrep/` — shared ripgrep binary detection (`Find()` returns `rg` path or empty string, cached via `sync.OnceValue`); used by find and grep tools for rg-first strategy
- `extensions/loop/` — core extension implementing the two-level while-loop agent cycle (outer: follow-ups, inner: steering + tool calls); subscribes to `agent.prompt`, `agent.steer`, `agent.followup`, `model.change`, `thinking.change`, `skills.loaded`, `instructions.loaded`; combines instructions + skills into system prompt; publishes `agent.turn_start/end`, `agent.message_start/update/end`, `agent.tool_result`, `agent.end`; selects initial provider via priority chain (`WEAVE_PROVIDER` env > settings provider > first registered > `"anthropic"` fallback)
- `extensions/instructions/` — discovers and loads context files (CLAUDE.md/AGENTS.md walked up from project root, then global `~/.weave/`) and system prompt files (SYSTEM.md/APPEND_SYSTEM.md from `.weave/` or `~/.weave/`); publishes assembled prompt on `TopicInstructionsLoaded`; project files override global
- `extensions/tools/{bash,read,edit,write,grep,find,ls}/` — individual tool extension modules, each an independent Go module self-registering via `sdk.RegisterTool`; find and grep use an rg-first pattern (shell out to `rg` when available for .gitignore support and faster searches, fall back to pure Go stdlib when absent), share binary detection via `utils/ripgrep.Find()`, and read `sdk.Config.RespectGitignore()` to toggle .gitignore honoring (default: true); grep supports an `include` glob filter parameter and per-line truncation; find supports `**/` recursive patterns via component-wise segment matching
- `extensions/tools/subagent/` — subagent tool extension; spawns isolated `weave -p --output json` subprocesses with restricted tool allowlists and optional sandbox mode. Agent definitions are markdown files with YAML frontmatter (name, description, tools, model, sandbox, messaging, system) discovered from embedded built-ins (`agents/general.md`, `explore.md`, `plan.md`), `.weave/agents/`, and `~/.weave/agents/` (project > global > built-in precedence). Each discovered agent registers as `subagent_<name>` tool. Three execution modes: single (`prompt`), parallel (`tasks`), chain (`chain` with `{previous}` substitution). Background mode spawns without blocking; `check_agent(id)` and `await_agent(id)` query results. Inter-agent communication via parent `Broker` routes `send_message`, `broadcast_message`, and `list_agents` between children when `messaging: true`. Broker registers agents by ID, monitors stdout for JSON routing events, and writes `agent_msg`/`inject`/`list_agents_response` to target stdin pipes. Child-side `stdin_listener.go` reads stdin JSON lines and queues them as user messages when `--weave-subagent-id` is set. Subagent uses `testRunSubagent` hook for mocking in tests
- `extensions/providers/openai-compat/` — shared library for OpenAI-compatible providers (SSE parsing, message/tool conversion); reused by `openai` and `zai` providers; import as `openaicompat` package
- `extensions/providers/{anthropic,openai,zai,kimi}/` — provider extension modules; Anthropic and Kimi use official SDKs (Kimi uses Anthropic SDK with custom base URL), OpenAI and Z.ai delegate to `openai-compat`
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
- `launcher/` — full pipeline: `AutoDiscover` recursively scans project-local `.weave/extensions/`, global `~/.weave/extensions/`, and built-in `extensions/` for Go modules (dirs with `go.mod` + `.go` files); UI extensions detected via `sdk.IsUIExtension(dir)` (scans `.go` files for `RegisterUIExtension(` or `RegisterUI(` calls); deduplicates by name (local > global > built-in); applies `exclude_extensions` list; `ComputeHash` of .go files (includes headless flag for different caches), `Cache` in `~/.weave/bin/{hash}/`, `Build` by generating go.mod+main.go with blank imports (filtering UI extensions for headless), then `syscall.Exec`; generated code imports `weave/sdk/wire` and uses `wire.CoreWireConfig`/`wire.WireWithCore`; passes `WEAVE_LAUNCHER_PATH`, `WEAVE_BUILD_HASH`, and `WEAVE_ORIG_ARGS` env vars for `/reload` support

**Extension lifecycle:** Extension packages call `sdk.RegisterExtension[T](name, factory)` in `init()`, where `T` is the extension's typed config struct. Provider, tool, and UI extensions similarly call `sdk.RegisterProvider[T]`, `sdk.RegisterTool[T]`, and `sdk.RegisterUI`. For extensions needing a custom config scope (e.g. TUI uses `"ui"`, sandbox uses `"sandbox"`), use `sdk.RegisterExtensionWithScope[T](name, scope, factory)`. Duplicate registrations log a warning (first registration wins). The SDK extracts the config schema via reflection at registration time and stores it for help generation. UI extensions (TUI-specific plugins) call `sdk.RegisterUIExtension` in `init()` and are wired by the TUI at startup via `sdk.GetUIExtensions()`. All auto-discovered extensions are blank-imported in the generated binary, triggering registration. `wire.WireWithCore()` (from `sdk/wire/`) resolves names from registries and calls `Subscribe(bus)` on each extension. Before subscribing, it publishes `app.started` on the bus and invokes any handlers registered via `sdk.OnAppStarted()`. Extensions register handlers via `bus.On(topic, handler)` or `bus.OnAll(handler)` to receive events; `bus.Off(handler)` removes a handler. Handler panics are caught by the bus and published as diagnostic events via configurable topics (`extension.panic` and `extension.error` by default); errors from handlers are similarly published as diagnostic events. When no prompt is provided (`-p` flag unset), the TUI extension is included in the build for interactive mode; with `-p`, weave runs in print mode without TUI. `IsHeadless()` returns true in print mode, false in interactive mode. UI extensions are silently skipped in headless mode.

**Module boundaries** — Every extension has its own `go.mod` (e.g., `weave/ext/ui/tui`), making it a separate Go module. Go's `internal/` visibility rule applies at the module boundary, so root-module `internal/` packages are **not importable by extensions**. Anything extensions need to share — especially bus event payload types that cross producer/consumer boundaries — must live in a public shared module (typically `sdk/`). Never place event payload types in `internal/` packages if any extension needs to type-assert them.

## Configuration

Config files are JSON-only. The main config file (`.weave/settings.json`) is discovered by walking up from cwd; falls back to `~/.weave/settings.json`. Extensions declare typed config structs with struct tags (`json`, `default`, `description`, `env`, `flag`, `short`, `validate`) and the framework populates them automatically via the generic registration system.

`sdk.Config` is the carrier interface (`FilePath()`, `ProjectDir()`, `ResolveKey(provider, envVar)`, `ExtensionConfig(scope, name, target, envPrefix)`, `IsHeadless()`, `Preferences()`, `SavePreferences()`, `SaveProviderKey()`, `RespectGitignore()`) — extensions receive populated config structs in their factory functions. `IsHeadless()` returns true when running without TUI (print mode); extensions use it to skip UI-dependent work. `HeadlessConfig` wraps a `Config` and overrides `IsHeadless()`. `wire.WireWithCore` takes a `wire.CoreWireConfig` struct (AgentLoop + SingleTurn) without provider or extension lists.

All extensions are auto-discovered by recursively scanning for Go modules (directories with `go.mod` + `.go` files) in project-local `.weave/extensions/`, global `~/.weave/extensions/`, and built-in `extensions/`. UI extensions (detected via `sdk.IsUIExtension(dir)`) are excluded from headless builds. All providers are compiled in; runtime selection via settings or `WEAVE_PROVIDER` env var.

Unified settings JSON format (single file — project `~/.weave/settings.json` or global):
```json
{
  "agent_loop": "loop",
  "ui_extension": "tui",
  "exclude_extensions": [],
  "providers": {
    "kimi": {
      "model": "kimi-for-coding",
      "max_tokens": 32768,
      "base_url": "https://api.kimi.com/coding"
    }
  },
  "sandbox": { "mode": "auto", "writable": ["."] },
  "provider": "anthropic",
  "model": "claude-sonnet-4-6",
  "thinking_level": "medium",
  "respect_gitignore": true,
  "ui": { "editor_max_lines": 20 },
  "tools": { "bash": { "timeout": 120 } }
}
```

**Config validation** runs automatically on every `LoadFromDir` call via `ValidateWithConfigDir` in `settings/validation.go`. Generic checks only — no extension-specific value validation (runtime resolves names via registries). Currently validates `output` format (`"text"` or `"json"`). Extension names, sandbox modes, UI values, and agent loop names are not validated at load time.

**Help handling** — `settings.HelpError` wraps help text and implements `errors.Is(flag.ErrHelp)` for standard help detection. When `--help` or `-h` is passed, `LoadFromDir` returns a `HelpError` with the full auto-generated help text from all registered extension schemas (global flags + per-extension flags with defaults, env vars, and descriptions).

**Layered settings** provide persistent user preferences with two layers merged in order (global → local):
- Global: `~/.weave/settings.json` — user-wide defaults
- Local: `.weave/settings.local.json` — per-developer overrides, auto-added to `.git/info/exclude`

The main config file (`.weave/settings.json` discovered by walking up from cwd, or `~/.weave/settings.json` as fallback) provides project-level settings. Tool and UI preferences are resolved via `LoadLayeredSettings` which merges global + local for `tools.*` and `ui.*` fields.

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
  },
  "providers": {
    "kimi": {
      "model": "kimi-for-coding",
      "max_tokens": 32768,
      "base_url": "https://api.kimi.com/coding"
    }
  }
}
```

Merge semantics: nested objects merge recursively, primitive values and maps override, later layers take precedence. `LoadLayeredSettings(projectDir)` in `settings/merge.go` loads and merges global + local. `SaveSettings` accepts a `SettingsLayer` parameter to control which file is written.

**Declarative extension config** — extensions declare typed config structs and register them via the generic SDK registration functions. The framework automatically populates the config struct from settings, env vars, and CLI flags before calling the factory:
- Tools: `sdk.RegisterTool[BashConfig]("bash", factory)` — config loaded from `tools.bash.*`
- Providers: `sdk.RegisterProvider[KimiConfig]("kimi", factory)` — config loaded from `providers.kimi.*`
- Extensions: `sdk.RegisterExtension[MyConfig]("name", factory)` — config loaded from `extensions.name.*`
- Custom scope: `sdk.RegisterExtensionWithScope[TUIConfig]("tui", "ui", factory)` — config loaded from `ui.*`

The config struct uses tags: `json` (field name), `default` (default value), `env` (env var suffix), `flag` (CLI flag name), `short` (short flag), `validate` (validation rules), `description` (help text). The `settings.Loader` applies sources in priority order: defaults → JSON data → env vars → CLI flags → validation. Config schemas are extracted via reflection at registration time and used by `settings.GenerateFullHelp()` for auto-generated `--help` output.

**UI config** — TUI defines its own config struct in `extensions/ui/tui/settings.go` and registers with `sdk.RegisterExtensionWithScope[TUIConfig]("tui", "ui", factory)`. The framework passes populated `ui.*` settings; the TUI unmarshals into its local struct. The editor height clamp is set from `editor_max_lines` when present.

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
- `settings.json` `sandbox.mode` for initial mode
- Bus events: `sandbox.mode.change` (switch mode), `sandbox.approve`/`sandbox.approved`/`sandbox.denied` (ask-mode approval), `sandbox.trust` (session allowlist pattern)
- Mandatory deny paths are hardcoded (not configurable): writes to `~/.ssh/`, `~/.bashrc`, `~/.zshrc`, `~/.profile`, `~/.gitconfig`, `.git/hooks/`, `.git/config`, `.weave/`; reads from `~/.ssh/id_*`, `~/.aws/credentials`, `**/.env`, `**/.env.*`
- macOS uses Seatbelt (`sandbox-exec`), Linux uses bubblewrap (`bwrap`, must be installed separately)

**Model registry** (`sdk/model/`) provides curated model metadata (display name, reasoning support, context window, max tokens) via `model.RegisterModel`/`model.GetModel`/`model.ListModelsForProvider`/`model.ListAllModels`. Built-in models are registered by `model.RegisterBuiltinModels()`.

**StreamOptions** (`sdk/model/`) passes per-request options (model, thinking level, max tokens) to providers via functional options: `model.WithModel(model)`, `model.WithThinkingLevel(level)`, `model.WithMaxTokens(n)`. Providers read these instead of re-creating on model switch.

**Provider environment variables:**
- `ANTHROPIC_API_KEY` — required for Anthropic provider (default model: `claude-sonnet-4-6`, override with `ANTHROPIC_MODEL`)
- `OPENAI_API_KEY` — required for OpenAI provider (default model: `gpt-5.5`, override with `OPENAI_MODEL`)
- `ZAI_API_KEY` — required for Z.ai provider (default model: `glm-5.1`, override with `ZAI_MODEL`)
- `KIMI_API_KEY` — required for Kimi provider (default model: `kimi-for-coding`, override with `KIMI_MODEL`)
- `KIMI_MAX_TOKENS` — override the default max tokens (32768) for Kimi provider
- `WEAVE_PROVIDER` — override the active provider at runtime (e.g., `openai`, `zai`); highest priority, overrides settings.json preference
- `WEAVE_THINKING_LEVEL` — initial thinking level (default: `medium`)
- `WEAVE_OFFLINE` — set to `1` to skip the startup extension update check (for offline/air-gapped environments)

**Provider selection priority** (highest to lowest):
1. `WEAVE_PROVIDER` env var (explicit user override)
2. `settings.json` `"provider"` field (persisted user preference)
3. Alphabetically first registered provider (`sdk.ListProviders()[0]`)
4. `"anthropic"` (ultimate fallback)

**Kimi models:**
| ID | Display Name | Context | Max Tokens | Reasoning | Default |
|---|---|---|---|---|---|
| `kimi-for-coding` | Kimi For Coding | 262144 | 32768 | yes | yes |

The Kimi API uses `kimi-for-coding` as the stable model identifier; the backend maps it to the current model version.

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
