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
make test          # Run root + all extension module tests
make tidy          # Run go mod tidy in root and all extension modules
go test ./launcher/...  # Run tests for a single package
cd extensions/agent && go test ./...  # Run tests for a single extension module (must cd first)
```

## Testing

- **Assertions**: Use `github.com/stretchr/testify` — `require` for fatal assertions (prerequisite failures, nil deref risk), `assert` for non-fatal checks. Never use raw `t.Error`/`t.Fatal`.
- **Mocks**: Use moq-generated mocks exclusively. Run `make gen` after changing interfaces. Mocks live in `*_mock_test.go` files — never edit them by hand.
- **go:generate**: Each SDK interface file has a `//go:generate moq ...` directive. Cross-package mocks use `-skip-ensure -pkg <pkg>`.
- **No hand-written mocks**: If a mock needs custom behavior (scripted responses, call recording), set the mock's `Func` fields or write a helper function that configures a moq mock — never create a new mock struct.
- **Config interface changes**: When `sdk.Config` interface changes, update all test config stubs across extension modules (common locations: `extensions/tools/grep/grep_test.go`, `extensions/tools/find/find_test.go`, `extensions/ui/tui/models_test.go`). Run `make gen` to regenerate moq mocks after interface changes.

## Architecture

Standard library as much as possible. Every replaceable component is an extension (runner, provider, tools, store, hooks). Extensions are independent Go modules that self-register via `init()`. Extension modules have their own `go.mod` — test/lint them by `cd`ing into the directory, not via path from root (e.g. `go test ./extensions/agent/...` won't work).

**Extension responsibility boundaries** — each extension owns exactly one concern and must not leak into another extension's domain. A tool runs commands, a provider calls an API, the agent loop drives turns, the TUI renders output — no overlap. Extensions communicate exclusively through bus events; they never import or call each other directly. If an extension needs data another extension owns, it listens for the event that publishes it rather than reaching into that extension's internals. When adding code to an extension, ask: is this this extension's job? If not, it belongs somewhere else.

**Fork territory** — the extension API covers common operations. Extensions that need deep structural changes (replacing the entire editor component, rewiring focus lifecycle, replacing the main layout engine) should fork the TUI module directly rather than shoehorning through `TUIExtAPI`. The API is designed for panels, custom renderers, theme tweaks, and status/footer/header overlays — not for gutting core components. If an extension's needs exceed what `TUIExtAPI` provides, maintain a separate fork of `extensions/ui/tui/` instead of adding one-off escape hatches to the shared API.

**Launcher pattern:** resolve config → auto-discover extensions → build a custom binary (cached per hash) → exec it. `cmd/weave/main.go` is a thin stub that calls `wire.Run()`. The generated `main.go` sets up file logging (`internal/log.Setup`) before calling `wire.WireWithCore()`, ensuring all extension wiring logs are captured to `~/.weave/logs/weave.log`.

**Key packages:**
- `sdk/` — defines `Extension`, `Bus`, `Config`, `UI` interfaces; `Handler func(Event) error` type for callback-based bus handlers; `Config` includes `FilePath()`, `ProjectDir()`, `ExtensionConfig(scope, name, target)`, `IsHeadless()`, and `RespectGitignore() bool`; `PreferenceStore` interface with `Preferences(target)`, `SavePreferences(target)`, `SaveProviderKey(provider, key)`; `HeadlessConfig` wraps a `Config` and overrides `IsHeadless()`; `NoopConfig` is a nil-safe Config stub that returns empty/zero values; `NoopPreferenceStore` is a nil-safe PreferenceStore stub; `configOrDefault(cfg)` returns the given Config or a `NoopConfig` stub if nil; `preferenceStoreFrom(cfg)` extracts `PreferenceStore` from a `Config` or returns `NoopPreferenceStore`; global registries for extensions, providers, tools, and UIs (`RegisterExtension`/`GetExtension`, `RegisterProvider`/`GetProvider`, `RegisterTool`/`GetTool`, `RegisterUI`/`GetUI`) with duplicate registration warnings (first wins, logs to stderr); `RegisterUIExtension`/`GetUIExtensions`/`ResetUIExtensionRegistry` for TUI-specific plugin detection; `Sandboxer` interface (`WrapCommand`, `AllowWrite`, `AllowRead`); `OnBusReady(fn)` / `InvokeBusSubscribers(bus)` for tool bus registration; `OutputRedirectPayload` event payload for `output.redirect` bus events; `ErrNotRegistered` sentinel error; `Message` types; `NoopUI` stub for headless mode; `FileTracker` interface (`RecordRead`, `WasRead`, `GetReadTime`) with global getter/setter (`SetFileTracker`/`GetFileTracker`) for read-before-edit enforcement; `FileMuter` interface (`Lock(path) func()`) with global getter/setter (`SetFileMutex`/`GetFileMutex`) for per-file mutation serialization; `WithBus(ctx, bus)`/`BusFromContext(ctx)` to attach/retrieve the event bus from context for tool streaming; `ReadDonePayload`, `BashOutputPayload`, `BackgroundStartPayload`, `BackgroundDonePayload` event payload types for tool bus events; `Logger(name)` returns a structured `*slog.Logger` pre-tagged with `"ext": name` for extension diagnostic output; `SessionStore` interface (`ListSessions()`, `LoadHistory(sessionID)`) with `SessionInfo{ID, CWD, CreatedAt, UpdatedAt}` struct and global getter/setter (`SetSessionStore`/`GetSessionStore`) following the `FileTracker`/`FileMuter` pattern; `SessionResumePayload{SessionID, Messages}` for `session.resume` bus events; `NoopSessionStore` zero-value stub
- `sdk/ui.go` — `UI` interface composed of three sub-interfaces: `UIDialogs` (`Select`, `Confirm`, `Input`, `MultiSelect`, `Editor` — all with variadic functional options), `UIStatus` (`SetStatus`, `Notify`, `NotifyTyped`, `ShowError`, `SetWorking`, `ClearWorking`), and `UIRegistry` (`RegisterCommand`, `RegisterRenderer`, `RegisterKeybinding`, `SetTheme`, `ListThemes`). Functional options: `SelectOption`, `ConfirmOption`, `InputOption`, `EditorOption`, each with a docked-placement variant (`WithKeepContent()` for selects, `WithKeepContentConfirm()` for confirms, `WithKeepContentInput()` for inputs, `WithKeepContentEditor()` for editors) that renders the overlay at the bottom of the chat area (12 rows) instead of a centered modal, keeping chat history visible. `NotifyLevel` enum (`NotifyInfo`, `NotifyWarning`, `NotifyError`, `NotifySuccess`) for typed notifications. `ThemeInfo` struct provides read-only theme colors for extensions, including dynamic accent fields (`Accent`, `AccentDim`, `AccentBright`) and state-specific background tints (`BackgroundTintPending`, `BackgroundTintSuccess`, `BackgroundTintError`). `NoopUI` provides zero-value defaults for all methods (returns first item for selects, `true` for confirms, empty strings/zeros for everything else)
- `sdk/schema.go` — `Schema` and `SchemaField` types describing extracted config struct metadata (json name, default, description, env, flag, short, validate tags). `extractSchema` reflects on a struct type to build the schema; `JSONFieldName` extracts the JSON key from a struct tag
- `sdk/schema_registry.go` — thread-safe schema storage per extension for help generation; `storeSchema`/`GetSchema`/`ListSchemas`/`ResetSchemas`
- `sdk/model/` — model types (`ThinkingLevel`, `ModelDef`, `StreamOptions`), model registry (`RegisterModel`/`GetModel`/`ListModelsForProvider`/`ListAllModels`/`ListAvailableModels`/`DefaultModelForProvider`/`SetProviderAuth`/`ProviderHasAuth`/`ResetAuthRegistry`), and `RegisterBuiltinModels()` with all hardcoded entries. Model registry warns on duplicates (first wins, matching other registries). Auth status tracking: `SetProviderAuth`/`ProviderHasAuth` are populated during wiring by `sdk/wire/wire.go` and queried by the TUI and loop to filter available models
- `sdk/registry/` — generic `Registry[T]` type used internally by all sdk registries; supports `WithWarn` (first-wins + warning) and `WithPanic` (panic on dup) options
- `internal/log/` — `Setup(logFile, debug, extraWriters...)` configures `slog.Default()` with a JSON handler writing to a rotating file via lumberjack. `Initialized()` reports whether logging has been set up. Called by the launcher-generated `main.go` before wiring extensions so all extension logs are captured
- `internal/wire/` — `WireExtensions()` and `WireWithCore()` composition roots that resolve names and subscribe extensions to the bus; `Run()` absorbs the full entry-point pipeline (config loading, auto-discovery via `launcher.AutoDiscover`, launcher build) and delegates extension management subcommands (`install`, `list`, `update`, `uninstall`) to `cmd/weave/extmanage/`; `CoreWireConfig` struct (AgentLoop, SingleTurn, Continue, Resume); `WireWithCore()` calls `sdk.InvokeBusSubscribers(bus)` before resolving extensions, calls `resolveSession()` when Continue/Resume are set, publishes `app.started` after wiring, sets `WEAVE_SINGLE_TURN` env var when `SingleTurn` is true, then calls `WireExtensions()`; `WireExtensions()` silently skips tools, providers, and UI extensions that appear in the extension names list; `WireExtensions()` checks auth status for all registered providers via `sdk.CheckProviderAuth` and stores results in the model registry via `model.SetProviderAuth`
- `bus/` — callback-based event bus (`Publish`/`On`/`OnAll`/`Off`) with per-handler goroutines, non-blocking dispatch, `recover()` wrapping every handler invocation (panics trigger diagnostic events via configurable topics), and graceful close via `sync.WaitGroup`
- `internal/auth/` — provider credential storage in `~/.weave/auth.json`; supports both API keys (`api_key`) and OAuth tokens (`access_token`, `refresh_token`, `expires_at`, `token_type`). `Load()`/`Save()`/`SetProviderKey()`/`GetProviderKey()`/`SetOAuthCredential()`/`GetOAuthCredential()`/`ClearProviderAuth()`/`RefreshOAuthToken()`; `LoadProviderAuth(providerName, target)` loads auth into a typed struct from auth.json + env vars (using `env` tags with no prefix). Used by the `sdk.RegisterProvider` wrapper for automatic auth injection into provider factories. Auth file permissions are `0600`
- `settings/` — unified JSON-only settings system. Discovers `.weave/settings.json` by walking up from cwd, falling back to `~/.weave/settings.json`. `Settings` struct holds all config fields (agent_loop, ui_extension, providers, sandbox, jsonl, extensions, plus user preferences like provider, model, thinking_level, ui, tools). `FullConfig` implements `sdk.Config` with key resolution and typed extension config via `ExtensionConfig()`. `FullConfig.SetArgs(args)` stores remaining CLI args for extension-specific flag parsing. `FullConfig.SetProjectDir(dir)` overrides the project directory for layered settings resolution. `loader.go` — custom `Loader` struct with `Load(target)` that applies defaults → JSON data → env vars → CLI flags → validation, using struct tags (`default`, `env`, `flag`, `short`, `validate`, `description`). `merge.go` — `MergeSettings` deep-merges global → local layers for tool/UI preferences. `validation.go` — generic field-path validation (no extension-specific values). `ProjectDirFromConfig()` derives project root from config file path for auto-discovery from subdirectories. `help.go` — `GenerateFullHelp()` produces full `--help` output from registered extension schemas.
- `internal/truncate/` — shared output truncation (2000 lines / 50KB) used by all tools for consistent output limiting
- `utils/ripgrep/` — shared ripgrep binary detection (`Find()` returns `rg` path or empty string, cached via `sync.OnceValue`); used by find and grep tools for rg-first strategy
- `internal/filemut/` — per-file mutex for serializing concurrent edits/writes to the same path; uses `sync.Map` of `*sync.Mutex` with lazy creation
- `internal/filetracker/` — in-memory tracker for read-before-edit policy enforcement; stores path → mod time mappings; safe for concurrent use
- `extensions/tools/edit/endings.go` — line ending detection and preservation (`DetectLineEndings`, `NormalizeToLF`, `RestoreLineEndings`); handles CRLF/LF and mixed endings (moved from `internal/fileutil/` to avoid cross-module `internal/` visibility issues)
- `extensions/tools/edit/pathutil.go`, `extensions/tools/read/pathutil.go`, `extensions/tools/write/pathutil.go` — macOS path normalization (`NormalizePath`) for curly quotes, Unicode spaces, and NFD normalization to match filesystem behavior (duplicated across tool modules to avoid cross-module `internal/` imports)
- `extensions/agent/` — core extension implementing the full agent conversation lifecycle. See "Agent Extension" section below for details. Subscribes to `agent.prompt`, `agent.steer`, `agent.followup`, `model.change`, `thinking.change`; publishes `agent.turn_start/end`, `agent.message_start/update/end`, `agent.tool_result`, `agent.compacted`, `agent.end`. Key files: `extension.go` (config, lifecycle), `loop.go` (turn loop, compaction integration), `compaction.go` (token estimation, cut point detection, serialization, compaction execution), `prompt.go` (system prompt assembly, default-compact-prompt.md embedding), `context.go` (context file discovery, COMPACT.md resolution), `skills.go` (skill discovery).
- `extensions/tools/{bash,read,edit,write,grep,find,ls}/` — individual tool extension modules, each an independent Go module self-registering via `sdk.RegisterTool`; find and grep use an rg-first pattern (shell out to `rg` when available for .gitignore support and faster searches, fall back to pure Go stdlib when absent), share binary detection via `utils/ripgrep.Find()`, and read `sdk.Config.RespectGitignore()` to toggle .gitignore honoring (default: true); grep supports an `include` glob filter parameter and per-line truncation; find supports `**/` recursive patterns via component-wise segment matching; bash supports streaming output via `tool.bash.output` bus events, background execution (`run_in_background`, `auto_background_after` params), and temp file overflow for truncated output; read publishes `tool.read.done` events for read-before-edit tracking, normalizes macOS paths (curly quotes, Unicode spaces, NFD), and formats output with aligned line numbers (`N | content` with dynamic padding width); edit enforces read-before-edit via `FileTracker`, supports `replace_all` mode, serializes concurrent edits via `FileMuter`, and preserves original line endings (CRLF/LF); write detects no-op writes when content is identical and serializes via `FileMuter`; ls supports sorting, `limit` and `ignore` glob params, and hierarchical tree output with `depth`
- `extensions/tools/subagent/` — subagent tool extension; spawns isolated `weave -p --output json` subprocesses with restricted tool allowlists and optional sandbox mode. Agent definitions are markdown files with YAML frontmatter (name, description, tools, model, sandbox, messaging, system) discovered from embedded built-ins (`agents/general.md`, `explore.md`, `plan.md`), `.weave/agents/`, and `~/.weave/agents/` (project > global > built-in precedence). Each discovered agent registers as `subagent_<name>` tool. Three execution modes: single (`prompt`), parallel (`tasks`), chain (`chain` with `{previous}` substitution). Background mode spawns without blocking; `check_agent(id)` and `await_agent(id)` query results. Publishes `subagent.started` (payload: `map[string]string` with `id`, `name`, `mode`) when a background agent spawns and `subagent.done` (payload: `map[string]string` with `id`, `status`, `content`) on background agent completion. Inter-agent communication via parent `Broker` routes `send_message`, `broadcast_message`, and `list_agents` between children when `messaging: true`. Broker registers agents by ID, monitors stdout for JSON routing events, and writes `agent_msg`/`inject`/`list_agents_response` to target stdin pipes. Child-side `stdin_listener.go` reads stdin JSON lines and queues them as user messages when `--weave-subagent-id` is set. Subagent uses `testRunSubagent` hook for mocking in tests
- `extensions/providers/openai-compat/` — shared library for OpenAI-compatible providers (SSE parsing, message/tool conversion); reused by `openai` and `zai` providers; import as `openaicompat` package
- `extensions/providers/{anthropic,openai,zai,kimi}/` — provider extension modules; Anthropic and Kimi use official SDKs (Kimi uses Anthropic SDK with custom base URL), OpenAI and Z.ai delegate to `openai-compat`
- `extensions/store/jsonl/` — session persistence extension; subscribes to bus events and writes JSONL files to `~/.weave/sessions/`; implements Create, Append, Load, History, List, Compact internally with no SDK interface
- `extensions/sandbox/` — OS-level tool execution guard; wraps bash commands in Seatbelt (macOS) or bubblewrap (Linux) sandbox profiles; enforces path-based access policy on file tools via `Sandboxer` interface; four modes: `off` (no restrictions), `readonly` (no writes), `ask` (prompt per command), `auto` (sandbox wraps all commands); mandatory deny paths hardcoded (sensitive dotfiles, SSH keys, AWS credentials, .env files); cache paths (`~/.cache`, `~/Library/Caches`, `~/.npm`, `~/.local/share`) are always writable and auto-created if missing on Linux; macOS-specific: `TMPDIR` is redirected to `/private/tmp` in sandboxed commands because Seatbelt denies writes to `/var/folders/...` temp paths; subscribes to `sandbox.mode.change` for mid-session mode switching; publishes `sandbox.approve`/`sandbox.approved`/`sandbox.denied`/`sandbox.trust` events for ask-mode approval flow; self-registers via `sdk.RegisterExtension("sandbox", ...)`
- `extensions/ui/sandbox/` — TUI extension for sandbox mode indicator and ask-mode approval dialog; registers `Ctrl+S` keybinding to cycle sandbox modes; displays current mode in footer status pill (`SB:auto`, `SB:off`, etc.); implements `ApproveDialog` with approve/deny/trust-for-session options integrated into TUI `DialogStack` overlay system; uses `NotifyTyped` for mode change notifications; self-registers via `sdk.RegisterUIExtension`
- `extensions/ui/tui/` — interactive terminal UI extension built with Bubble Tea v2 (`charm.land/bubbletea/v2`) + Ultraviolet screen buffers for `Draw()` rendering. Key handling uses `tea.KeyPressMsg` (not v1 `tea.KeyMsg`). Mouse support uses `tea.MouseModeAllMotion` to capture click, drag, and release events for in-app text selection; wheel events scroll the chat viewport. Components use `lipgloss.NewStyle()` from `charm.land/lipgloss/v2`. Screen buffer rendering pattern: `Model.View()` creates an `uv.NewScreenBuffer(w,h)`, delegates to `Model.Draw()` which computes layout via `LayoutEngine` then draws each component, and returns `uv.TrimSpace(canvas.Render())`. Components implement `Draw(scr uv.Screen, area uv.Rectangle)` alongside retained `View() string` (used internally for string-to-buffer conversion via `uv.NewStyledString`). Uses `github.com/charmbracelet/ultraviolet` for `uv.Screen`, `uv.Rectangle`, `uv.layout` splitting. `github.com/atotto/clipboard` provides OS clipboard access for text selection copy. Self-registers via `sdk.RegisterExtension("tui", ...)` and `sdk.RegisterUI("tui", ...)`; implements `sdk.UI` interface for cross-extension integration (popups, status bar, slash commands, keybindings); bridge goroutine translates bus events to Bubble Tea messages; includes: streaming chat with progressive markdown rendering (Glamour with custom xchroma formatter), dialog stack for layered overlays, landing state before first prompt, smart auto-scroll with new-content indicator, token rate display in footer, tool output panels, thinking blocks, diff highlighting, multi-line editor (bubbles/v2 textarea) with history, file attachments with paste detection, pills bar for tool progress, slash commands, session/model selectors, and configurable keybindings. UI extensions (universal, via `RegisterUIExtension`) are wired at startup via `sdk.GetUIExtensions()`; TUI-specific extensions (via `RegisterTUIExtension` in the `tui` package, see below) are wired via `sdk.GetTUIExtensions()`
  - `palette/theme.go` — centralized `Theme` struct with semantic color slots. Grayscale base (`Foreground` "250", `ForegroundDim` "245", `Muted` "240", `BackgroundTint` "234") plus structural colors (`Border`, `BorderFocused`, `Success`, `Error`, `Warning`) and dynamic accent fields (`Accent`, `AccentDim`, `AccentBright`). State-specific background tints (`BackgroundTintPending` "235", `BackgroundTintSuccess` "22", `BackgroundTintError` "52") for tool panel cards. `DefaultTheme()` returns an independent copy. All components consume theme colors instead of hardcoded ANSI values.
  - `palette/thinking.go` — thinking level border color mapping using grayscale temperature progression (`240` → `242` → `244` → `246` → `248` → `250`)
  - `palette/state.go` — `State` enum (`StateIdle`, `StateStreaming`, `StateToolRunning`, `StateError`) representing agent activity state; `AccentForState()` maps states to accent color triplets (Accent/AccentDim/AccentBright). Idle uses grayscale ("245"), Streaming uses cyan ("45"), ToolRunning uses amber ("172"), Error uses red ("167")
  - `components/chat.go` — chat viewport with auto-scroll tracking, scroll indicator as a styled pill with `BackgroundTint` background, dot divider rows (`·` in `Muted`) between chat items, and mouse text selection (click-and-drag with `AttrReverse` highlight, text extraction via temporary screen buffer rendering, auto-copy to clipboard on release)
  - `components/completion.go` — autocomplete popup with slash command, file path, and custom provider completion support
  - `components/editor.go` — multi-line editor wrapping `bubbles/v2 textarea.Model` with history navigation, external editor support (`Ctrl+G`), dynamic height (3–15 lines), placeholder text ("Type a message..."), blurred border using `theme.Border` for clear focus distinction, and mouse text selection (click-and-drag with `AttrReverse` highlight, word selection via double-click, coordinate mapping via `VisualLineToLogical`/`ColFromWrapped` for wrapped lines, auto-copy to clipboard on release)
  - `components/footer.go` — two-line status bar with CWD, git, extension status pills. Line 2 groups stats (tokens, cost, context pct with threshold colors) on the left and model info (bold accent-colored model name, thinking level as a pill, token rate) on the right, separated by padding. Zero values (cost $0.0000, token rate 0) are hidden. Separators use `·` in muted.
  - `components/spinner.go` — streaming activity indicator (bubbles/v2 spinner) with color pulse between `Accent` and `AccentBright` every 3 ticks (6-tick cycle)
  - `components/attachments/` — file attachment model: tracks `[]Attachment` with paste detection (>10 newlines or >1000 chars auto-converts to temp file), pill-shaped display above editor with `BackgroundTint` background, `Ctrl+R` delete mode with `Error` color and `×` indicator
  - `components/messages/` — message renderers (assistant with progressive markdown and grayscale fade-in `ForegroundDim` → `Foreground` via `fadeColor()`, user with left-edge bar (`▐` in `AccentDim`) and 1-space gap, tool panels as bordered cards with rounded corners and state-dependent border colors plus 800ms flash on state transition, thinking with `░` left border bar in `Muted` and `ForegroundDim` content, diff with theme colors, typed notifications with level-based borders, compaction entry with `◫ Compacted` header and token statistics in `Muted` color) with shared `drawView` helper for screen buffer output; code blocks use custom "weave" Chroma formatter from `xchroma/`
  - `components/overlays/` — dialog components (selector with primary accent, confirm with warning accent for destructive actions, input, multiselect, editor) with `RoundedBorder` and `BorderFocused` color, `Dialog` interface (`ID`/`Update`/`Draw`/`Handles`/`Done`/`Result`/`SetSize`), adapter types (`SelectorDialog`, `ConfirmDialog`, `InputDialog`, `MultiSelectDialog`, `EditorDialog`), and `DialogStack` for layered overlay management. All overlay dialogs support docked placement via `WithKeepContent` functional options — chat viewport shrinks and dialog renders at bottom (12 rows) instead of a centered modal, keeping chat history visible
  - `xchroma/` — custom Chroma formatter registered as "weave"; maps token types (Keyword, String, Comment, Number, Operator) to Lip Gloss v2 styles with forced background matching chat bubble
  - `layout.go` — `LayoutEngine` computes `uv.Rectangle` regions (header, main, pills, editor, footer, panel tray, panel content) via `uv.layout` splitting; stored directly on Model. Panel regions are allocated when an active panel with `AboveEditor` or `BelowEditor` placement is visible
  - `landing.go` — landing screen with ASCII logo, horizontal rule in `Border` color, model info, and keybinding hints (shown before first prompt, re-shown on `/clear`/`/new`). Placeholder text moved to editor textarea.
  - `keybindings.go` — key resolution using v2 API (`keyString(msg tea.KeyPressMsg)`, `msg.Key.String()`)
  - `overlays.go` — overlay request/response types, dialog push logic; routes `sdk.UI` overlay calls (Select, Confirm, Input, MultiSelect, Editor) through blocking channels to the dialog stack
  - `bridge.go` — bus-to-Tea message translation with delta batching, token rate calculation, and agent state tracking. `agentStateTracker` derives `palette.State` from turn lifecycle events (`agent.turn_start` → Streaming, `agent.message_end` with tool calls → ToolRunning, `agent.tool_result` decrements pending count, `agent.turn_end` → Idle, `agent.end` with error → Error) and emits `AgentStateChangeMsg` to update theme accent colors dynamically.
  - `panel.go` — `PanelManager` tracks registered panels (`Show`/`Hide`/`Remove`/`PanelVisible`/`Active`); `PanelConfig` holds `ID`, `Placement` (`AsOverlay` renders as floating overlay without consuming layout rows; `AboveEditor`/`BelowEditor` reserve height in layout), `Blocking`, `Width`, `Height`, `Title` (displayed in panel tray tab, falls back to ID if empty); `PanelDrawer` interface (`Draw`, `Update`, `Handles`) for panel content rendering
  - `panel_tray.go` — `PanelTray` renders a tab strip for visible panels with active highlighting; keyboard focusable; cycles through tabs with Next/Prev; hidden when no panels are visible
  - `tui_ext_api.go` — `TUIExtAPI` interface (17 methods): panel management (`ShowPanel`, `HidePanel`, `RemovePanel`, `PanelVisible`, `PanelTray`), read-only queries (`Theme` returning `sdk.ThemeInfo`, `Size`), editor integration (`EditorText`, `SetEditorText`, `PasteToEditor`), rich rendering (`RegisterRichRenderer`), header/footer replacement (`SetFooter`, `SetHeader` accepting `TUIComponent` — a single-method interface with `Draw(scr uv.Screen, area uv.Rectangle)`), raw input (`OnTerminalInput` with `KeyEvent{Code rune, Mod int, String string}`), autocomplete (`AddAutocomplete` with `AutocompleteProvider` interface `Name() string` and `Suggestions(AutocompleteContext) []AutocompleteSuggestion`), cosmetic (`SetWorkingFrames`, `RegisterTheme`). Tool renderer precedence: rich renderer (`RichToolRenderer`) → plain renderer (`ToolRenderer`) → default diff renderer. Supporting types: `TUIExtension` interface (`Name`, `RegisterTUI`), `PanelTrayAPI` (`SetOrder(ids []string)`, `GetOrder() []string`), `RichToolRenderer`, `TUIComponent`, `KeyEvent`, `AutocompleteProvider`/`AutocompleteContext`/`AutocompleteSuggestion`, `ThemeDef` (includes `BackgroundTint`, `BackgroundTintPending`, `BackgroundTintSuccess`, `BackgroundTintError` fields not present in `ThemeInfo`)
  - `tui_ext_registry.go` — `RegisterTUIExtension[TConfig](name, factory)` registers TUI-specific extensions in the `tui` package (not `sdk`); `GetTUIExtensions(cfg)` instantiates all registered TUI extensions. Config scope is `"tui_extensions"` (e.g. `tui_extensions.diff-viewer.*` in settings JSON), distinct from the TUI's own `"ui"` scope. Uses same generic config-population pattern as other extension registries (settings → env → flags)
  - `extensions/ui/tui/extensions/subagent/` — TUI extension for subagent panel visualization; subscribes to `subagent.started`, `subagent.done`, and `agent.end` bus events via `sdk.OnBusReady`; creates a `BelowEditor` panel for each running background agent showing name, status indicator (●/✓/✗), mode, and elapsed time; panels auto-remove after 3-second grace period on completion; `agent.end` triggers `Close()` to cancel grace-period timers and clear tracked state; registers a `RichToolRenderer` for `subagent_*` tool names to render compact result cards in chat; uses `AgentTracker` for thread-safe agent state with timer-based grace period removal; self-registers via `tui.RegisterTUIExtension("subagent", ...)`

**Animation patterns:**
- Message fade-in: `AssistantMessage` has a `createdAt` timestamp. `fadeColor()` returns progressively brighter foreground colors (`ForegroundDim` "245" → `Foreground` "250" → `ForegroundBright` "15") for the first 150ms, creating a subtle materializing effect via `lipgloss.NewStyle().Foreground(...).Render(content)`.
- Status entrance animation: `Model.statusNew` flag set by `showStatus()`, cleared on first `Update()` frame. Status renders at `Muted` color when `statusNew` is true, then `Foreground` on subsequent frames.
- Dialog backdrop dimming: When `dialogStack` is non-empty, `Draw()` renders the normal UI then calls `applyBackdropDimming()` which iterates all screen cells and sets their foreground to `Muted`.
- Spinner color pulse: `SpinnerModel.tickCount` increments on each `spinner.TickMsg`. Color alternates between `Accent` and `AccentBright` every 3 ticks (6-tick cycle).
- Editor border pulse: When agent is active (Streaming or ToolRunning), the editor border cycles a pulse animation across 8 positions (4 corners + 4 edge midpoints). Current position uses `AccentBright`, trailing position uses `Accent`, rest uses `BorderFocused`. Position updates every 500ms from the model. Disabled when idle or in error state. Implemented in `components/editor.go` `drawPulse()`.

The model selector (`Ctrl+L`) and `/model` slash command filter to only providers with valid auth credentials using `model.ListAvailableModels()`. The `/providers` slash command shows key status via `model.ProviderHasAuth()`. The `/login` slash command opens the auth selector for interactive API key or OAuth login. The `/logout` slash command shows configured providers and clears selected auth.

- `launcher/` — full pipeline: `AutoDiscover` recursively scans project-local `.weave/extensions/`, global `~/.weave/extensions/`, and built-in `extensions/` for Go modules (dirs with `go.mod` + `.go` files); UI extensions detected internally (scans `.go` files for `RegisterUIExtension(`, `RegisterUI(`, or `RegisterTUIExtension(` calls); deduplicates by name (local > global > built-in); applies `exclude_extensions` list; `ComputeHash` of .go files (includes headless flag for different caches), `Cache` in `~/.weave/bin/{hash}/`, `Build` by generating go.mod+main.go with blank imports (filtering UI extensions for headless), then `syscall.Exec`; generated code imports `weave/internal/wire` and uses `wire.CoreWireConfig`/`wire.WireWithCore`; passes `WEAVE_LAUNCHER_PATH`, `WEAVE_BUILD_HASH`, and `WEAVE_ORIG_ARGS` env vars for `/reload` support

## Logging

Extensions must never write to `stdout` or `stderr` directly — output leaks corrupt the Bubble Tea TUI. The framework provides a unified file-based logging system for all diagnostics.

- **API**: Use `sdk.Logger(name) *slog.Logger` to obtain a structured logger pre-tagged with `"ext": name`. Write logs via `slog.Info`, `slog.Warn`, `slog.Error`, or the logger returned by `sdk.Logger`.
- **Destination**: All logs route to a rotating file at `~/.weave/logs/weave.log` (JSON format, 10 MB max size, 30 day retention). In headless/print mode logs are also mirrored to `stderr`.
- **Setup**: The launcher-generated `main.go` calls `log.Setup(logFile, debug)` before wiring extensions. Debug level is controlled by `--debug` or `WEAVE_DEBUG`.
- **Migration**: Replace `log.Printf`, `fmt.Fprintf(os.Stderr, ...)`, and `log.New(os.Stderr, ...)` with `slog` or `sdk.Logger()` calls.

## Agent Extension

The `agent` extension owns the entire conversation lifecycle: prompt assembly, turn loop, tool execution, skill discovery, and context file loading. It subscribes to `agent.prompt`, `agent.steer`, `agent.followup`, `model.change`, and `thinking.change` events. It publishes `agent.turn_start/end`, `agent.message_start/update/end`, `agent.tool_result`, `agent.compacted`, and `agent.end` events.

### Prompt Assembly

The system prompt is built once per conversation (rebuilt on `/new`) from multiple layers, in order:

1. **Default prompt** (`default-system-prompt.md`, embedded) **or SYSTEM.md** (if found). SYSTEM.md replaces the default entirely.
2. **Date + CWD** — always injected (`Current date: YYYY-MM-DD`, `Current working directory: /path`).
3. **Available tools** — dynamic list from `sdk.ListTools()` with descriptions.
4. **Skills XML** — `<available_skills>` block with name, description, and file path for each discovered skill.
5. **Skills usage instructions** — `<skills_usage>` block instructing the model to read matching skills via the read tool before acting.
6. **Context files** — `# Project Context` section with CLAUDE.md/AGENTS.md content.
7. **APPEND_SYSTEM.md** — always appended last, regardless of whether SYSTEM.md or the default prompt was used as the base.

### Context File Discovery

Context files (`CLAUDE.md` and `AGENTS.md`) are discovered by walking up from the project directory toward the filesystem root. The first matching file per directory is used. Project-local files take precedence over global files (`~/.weave/`). Multiple files from different directories in the hierarchy are all included, with parent directories listed before child directories.

### SYSTEM.md and APPEND_SYSTEM.md

- **SYSTEM.md**: Placed in `<project>/.weave/SYSTEM.md` or `~/.weave/SYSTEM.md`. When present, it replaces the default system prompt entirely. Project-local SYSTEM.md overrides the global one.
- **APPEND_SYSTEM.md**: Placed in `<project>/.weave/APPEND_SYSTEM.md` or `~/.weave/APPEND_SYSTEM.md`. When present, it is always appended as the final layer of the system prompt, after context files. Project-local APPEND_SYSTEM.md overrides the global one.

### Skill Discovery

Skills are discovered from three sources, in precedence order (first wins by name):

1. **Project skills**: `<project>/.weave/skills/<name>/SKILL.md`
2. **Global skills**: `~/.weave/skills/<name>/SKILL.md`
3. **Extension-bundled skills**: `<ext-dir>/skills/<name>/SKILL.md` for each registered extension

Each skill directory contains a `SKILL.md` with YAML frontmatter (`name`, `description`, `disable-model-invocation`, `license`, `compatibility`, `metadata`, `allowed-tools`) followed by a `---` delimiter and the skill body. The skill name in frontmatter must match the directory name. Names must be lowercase alphanumeric with hyphens (e.g. `go-test`, `refactor`), 1-64 characters, and cannot be reserved words (`anthropic`, `claude`).

### Extension-Bundled Skills

Each extension can ship skills in a `skills/` subdirectory. The agent extension discovers these by scanning registered extension directories in both project-local `.weave/extensions/` and global `~/.weave/extensions/`. Only directories matching currently registered extension names are checked. User skills (project or global) override extension-bundled skills by name.

### Skill Self-Invocation

Skills are listed in the system prompt as XML with their name, description, and file location. The model is instructed to read matching skills using the read tool before taking other action. When a user invokes a skill via the `/skill:<name>` slash command, the skill body is pre-loaded into the conversation as an `agent.prompt` event wrapped in `<skill>` XML.

### Turn Loop

The agent implements a two-level loop:
- **Outer loop**: waits for follow-up prompts or new conversations (`agent.followup` / `agent.prompt`). A new `agent.prompt` resets messages and rebuilds the system prompt.
- **Inner loop**: streams a turn from the provider, executes any tool calls, and repeats while tools remain (or steering messages arrive). Each turn runs in its own cancellable context; `agent.interrupt` cancels the current turn without ending the session.

The provider is lazily instantiated on the first prompt, giving the TUI time to show "No providers configured" and letting the user set an API key. Model and thinking level changes are drained before each turn to ensure the active provider matches user selection.

`drainSteering` parses steering events and detects `compact` / `compact <instructions>` payloads, returning them separately from regular steering messages so the loop can trigger compaction instead of appending them as user messages.

### Context Compaction

Context compaction summarizes old conversation messages to stay within the model's context window. It operates in two modes:

- **Auto-compaction**: triggered at the start of each inner loop iteration when `estimateTokens(messages) + len(systemPrompt)/4 > contextWindow - reserveTokens`. Uses `shouldCompact()` to check the token budget.
- **Manual compaction**: triggered via `/compact [instructions]` slash command, which sends an `agent.steer` event with `"compact"` prefix. Both manual and auto-compaction errors are non-fatal — they publish an `agent.compacted` event with an `error` field and the session continues.

**Cut point detection** (`findCutPoint`): walks backward from the latest messages, accumulating tokens until `keepRecentTokens` is exceeded. Then `findValidBoundary` ensures the cut never splits tool call/result pairs — it skips past assistant messages with pending tool results. `tool_result` messages are never valid boundaries.

**Summary generation** (`compact`): messages before the cut point are serialized to text via `serializeForSummary`, which includes previous compaction summaries (wrapped in `<previous-summary>` XML) and cumulative file operation lists (`<read-files>`, `<modified-files>`). The serialized text is sent to the provider as a single user message with the compact prompt as system prompt, thinking off, and no tools. The summary is stored as an assistant message with the exact prefix `[Compaction Summary]\n` (defined as `compactionSummaryPrefix`). Previous summaries are detected by scanning for this prefix, and only the first matching message is extracted as the `<previous-summary>` context.

**Manual compaction in waitForInput**: When `/compact` is invoked between turns (in `waitForInput` state), `handleManualCompact` runs compaction with a dedicated cancellable context that respects interrupt signals. Errors are published as `agent.compacted` events with an error field and are non-fatal.

**File operation tracking**: `fileOperations` accumulates read/modified file paths across compactions. Before messages are summarized, `trackFileOps` extracts file paths from read/edit/write tool calls. Additionally, file ops are tracked after each tool execution cycle in the inner loop (`loop.go`), ensuring current-turn operations are captured even before compaction runs. The accumulated lists are included in the serialization so the LLM can produce a summary that preserves awareness of which files were accessed and modified. The tracker resets on new conversations (`agent.prompt`).

**Unknown model fallback**: If the current model is not in the registry or has no context window defined, `shouldCompact` uses a conservative default of 200,000 tokens. This ensures compaction still fires for unregistered or custom models.

**COMPACT.md size limit**: Files larger than 100KB (`maxCompactPromptSize`) are silently skipped to prevent oversized prompts from being loaded into the summarization context.

**Compact prompt resolution** (priority order):
1. `/compact [instructions]` inline args (highest priority)
2. `.weave/COMPACT.md` (project-local)
3. `~/.weave/COMPACT.md` (global)
4. Embedded `default-compact-prompt.md` (fallback)

**`CompactionConfig`** struct: `Enabled` (bool, default true), `ReserveTokens` (int, default 16384), `KeepRecentTokens` (int, default 20000), `Model` (string, empty = current model). Loaded from extension config scope.

**`agent.compacted` bus event payload**: `map[string]any` with `summarized` (int, messages replaced), `tokens_before` (int), `tokens_after` (int). On error: `{"error": "message"}`.

**TUI integration**: `CompactedMsg` in bridge translates the event. `CompactionEntry` component renders a `◫ Compacted` header with token statistics in `Muted` color. `/compact` slash command registered in `commands.go`.

**Extension lifecycle:** Extension packages call `sdk.RegisterExtension[T](name, factory)` in `init()`, where `T` is the extension's typed config struct and `factory` has signature `func(Config, PreferenceStore, T) (Extension, error)`. Provider factories receive three arguments: `func(Config, TConfig, TAuth) (Provider, error)`. Tool factories: `func(Config, PreferenceStore, TConfig) (Tool, error)`. UI extension factories: `func(Config, PreferenceStore, TConfig) (UIExtension, error)`. TUI-specific extension factories: `func(Config, PreferenceStore, TConfig) (TUIExtension, error)` registered via `sdk.RegisterTUIExtension[TConfig](name, factory)` in `init()`. For extensions needing a custom config scope (e.g. TUI uses `"ui"`, sandbox uses `"sandbox"`), use `sdk.RegisterExtensionWithScope[T](name, scope, factory)`. Duplicate registrations log a warning (first registration wins). The SDK extracts the config schema via reflection at registration time and stores it for help generation. UI extensions (universal plugins) call `sdk.RegisterUIExtension[TConfig](name, factory)` and are wired by the TUI at startup via `sdk.GetUIExtensions()`. TUI extensions (TUI-only plugins) call `sdk.RegisterTUIExtension[TConfig](name, factory)` and are wired by the TUI at startup via `sdk.GetTUIExtensions()`, receiving a `TUIExtAPI` implementation. All auto-discovered extensions are blank-imported in the generated binary, triggering registration. `wire.WireWithCore()` (from `internal/wire/`) calls `sdk.InvokeBusSubscribers(bus)` before resolving extensions, then resolves names from registries and calls `Subscribe(bus)` on each extension. Before subscribing, it publishes `app.started` on the bus. Extensions register handlers via `bus.On(topic, handler)` or `bus.OnAll(handler)` to receive events; `bus.Off(handler)` removes a handler. Handler panics are caught by the bus and published as diagnostic events via configurable topics (`extension.panic` and `extension.error` by default); errors from handlers are similarly published as diagnostic events. When no prompt is provided (`-p` flag unset), the TUI extension is included in the build for interactive mode; with `-p`, weave runs in print mode without TUI. `FullConfig.IsHeadless()` always returns `false`; headless mode is determined at the entry point (`internal/wire/run.go`) by whether `-p` was provided. `HeadlessConfig` wraps a `Config` and overrides `IsHeadless()` for specific scenarios. UI extensions and TUI extensions are silently skipped in headless mode.

**Declarative provider auth** — Providers declare an auth struct with `json`/`env` tags in a separate `auth.go` file (e.g., `AuthConfig{APIKey string \`json:"api_key" env:"ANTHROPIC_API_KEY"\`, OAuthToken string \`json:"oauth_token"\`}`). They register with `sdk.RegisterProvider[TConfig, TAuth](name, factory)`. The framework automatically loads auth from `~/.weave/auth.json` (structured `{"providers": {"name": {"api_key": "..."}}}` or with OAuth token fields) and environment variables (no prefix — `ANTHROPIC_API_KEY` not `WEAVE_ANTHROPIC_API_KEY`), then passes the populated auth struct to the factory. Auth availability is tracked by the model registry (`model.SetProviderAuth`/`model.ProviderHasAuth`), set during `wire.Wire()` for all registered providers. The TUI and agent extension query `model.ProviderHasAuth` instead of calling `cfg.ResolveKey`.

**Module boundaries** — Every extension has its own `go.mod` (e.g., `weave/ext/ui/tui`), making it a separate Go module. Go's `internal/` visibility rule applies at the module boundary, so root-module `internal/` packages are **not importable by extensions**. Anything extensions need to share — especially bus event payload types that cross producer/consumer boundaries — must live in a public shared module (typically `sdk/`). Never place event payload types in `internal/` packages if any extension needs to type-assert them.

## Configuration

Config files are JSON-only. The main config file (`.weave/settings.json`) is discovered by walking up from cwd; falls back to `~/.weave/settings.json`. Extensions declare typed config structs with struct tags (`json`, `default`, `description`, `env`, `flag`, `short`, `validate`) and the framework populates them automatically via the generic registration system.

`sdk.Config` is the carrier interface (`FilePath()`, `ProjectDir()`, `ExtensionConfig(scope, name, target)`, `IsHeadless()`, `RespectGitignore()`) — extensions receive populated config structs in their factory functions. `sdk.PreferenceStore` provides `Preferences(target)`, `SavePreferences(target)`, and `SaveProviderKey(provider, key)`; factories that need preference access receive it as a separate argument. `IsHeadless()` returns true when running without TUI (print mode); extensions use it to skip UI-dependent work. `HeadlessConfig` wraps a `Config` and overrides `IsHeadless()`. `wire.WireWithCore` takes a `wire.CoreWireConfig` struct (AgentLoop + SingleTurn) without provider or extension lists.

All extensions are auto-discovered by recursively scanning for Go modules (directories with `go.mod` + `.go` files) in project-local `.weave/extensions/`, global `~/.weave/extensions/`, and built-in `extensions/`. UI extensions (detected internally by the launcher) are excluded from headless builds. All providers are compiled in; runtime selection via settings or `WEAVE_PROVIDER` env var.

Unified settings JSON format (single file — project `~/.weave/settings.json` or global):
```json
{
  "agent_loop": "agent",
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
  "jsonl": { "dir": "~/.weave/sessions" },
  "extensions": {
    "myext": { "key": "value" },
    "agent": {
      "enabled": true,
      "reserve_tokens": 16384,
      "keep_recent_tokens": 20000,
      "model": ""
    }
  },
  "provider": "anthropic",
  "model": "claude-sonnet-4-6",
  "thinking_level": "medium",
  "respect_gitignore": true,
  "ui": { "editor_max_lines": 20 },
  "tools": { "bash": { "timeout": 120 } }
}
```

Auth credentials are stored separately in `~/.weave/auth.json` (not in `settings.json`):
```json
{
  "providers": {
    "anthropic": { "api_key": "sk-ant-..." },
    "openai": {
      "access_token": "sk-...",
      "refresh_token": "rt-...",
      "expires_at": "2026-05-16T12:00:00Z",
      "token_type": "bearer"
    }
  }
}
```
Both API keys and OAuth tokens are valid authentication methods. The framework loads whichever is present and passes it to the provider factory. OAuth tokens are refreshed automatically when near expiry if a `refresh_token` is available.

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
- Providers: `sdk.RegisterProvider[KimiConfig, AuthConfig]("kimi", factory)` — config loaded from `providers.kimi.*`, auth loaded from auth.json + env vars via `env` tags on `AuthConfig`
- Extensions: `sdk.RegisterExtension[MyConfig]("name", factory)` — config loaded from `extensions.name.*`
- Custom scope: `sdk.RegisterExtensionWithScope[TUIConfig]("tui", "ui", factory)` — config loaded from `ui.*`

The config struct uses tags: `json` (field name), `default` (default value), `env` (env var suffix), `flag` (CLI flag name, auto-generated from `json` tag if omitted), `short` (short flag), `validate` (validation rules), `description` (help text). The `settings.Loader` applies sources in priority order: defaults → JSON data → env vars → CLI flags → validation. Config schemas are extracted via reflection at registration time and used by `settings.GenerateFullHelp()` for auto-generated `--help` output.

Supported validation rules: `required`, `gt=N`, `lt=N`, `min=N`, `max=N`, `oneof=a b c`, `url`. Structs can implement `Validate() error` for custom validation. CLI flags are generated only for: `string`, `int`, `int64`, `uint`, `uint64`, `float64`, `bool`, and `[]string`. Other types receive values from JSON and env vars but not CLI flags. Slice fields (`[]string`) accept comma-separated values: `--ext-names a,b,c`.

Built-in config scopes are hardcoded in `FullConfig.ExtensionConfig`: `tools`, `providers`, `ui`, `sandbox`, `jsonl`, `extensions`. To add a new scope, update the switch statement in `settings/config.go`.

**Provider env vars are a special case:** providers receive an empty `envPrefix`, so both their config env tags (e.g. `ANTHROPIC_MODEL`, `KIMI_MODEL`) and auth env tags (e.g. `ANTHROPIC_API_KEY`, `KIMI_API_KEY`) resolve directly without a `WEAVE_` prefix. Tools and extensions use `WEAVE_<NAME>` as prefix.

Agent extension env vars (using `WEAVE_AGENT_` prefix):
```bash
WEAVE_AGENT_ENABLED=false              # Disable auto-compaction
WEAVE_AGENT_RESERVE_TOKENS=8192        # Reserve tokens for model response
WEAVE_AGENT_KEEP_RECENT_TOKENS=10000   # Recent tokens to preserve
WEAVE_AGENT_MODEL=claude-haiku-4-5     # Model for summary generation
```

**Extension-specific CLI flags** are prefixed with the extension name: `--bash-timeout 60`, `--kimi-model kimi-for-coding`. The framework strips the prefix before passing args to the extension's loader. Boolean flags support `--flag=true`/`--flag=false` syntax. Unknown flags are silently ignored by the loader, so extensions don't need to defensively parse args.

Agent extension compaction flags:
```bash
--agent-enabled=false                  # Disable auto-compaction
--agent-reserve-tokens=8192            # Reserve tokens for model response
--agent-keep-recent-tokens=10000       # Recent tokens to preserve
--agent-model=claude-haiku-4-5         # Model for summary generation
```

Session resume flags:
```bash
--continue, -c                         # Resume the most recent session (by UpdatedAt)
--resume <id>, -r <id>                 # Resume a specific session by ID
```
`--continue` and `--resume` are mutually exclusive. When used with `-p` (headless mode), the prompt is published as `agent.followup` so it appends to the restored history rather than starting a new conversation.

**Extension configs are resolved from layered settings** (global → project → local), not just the project config file. This means a tool's timeout can be set globally and overridden per-project.

**UI config** — TUI defines its own config struct in `extensions/ui/tui/settings.go` and registers with `sdk.RegisterExtensionWithScope[TUIConfig]("tui", "ui", factory)`. The framework passes populated `ui.*` settings; the TUI unmarshals into its local struct. The editor height clamp is set from `editor_max_lines` when present.

**Keybindings** are configured in `.weave/keybindings.yaml` and override built-in defaults (priority: user config > extension registrations > built-in):
```yaml
keybindings:
  app.model.cycle: ["ctrl+p"]
  app.model.select: ["ctrl+l"]
```
Built-in bindings: Escape=interrupt (or return focus to editor from panel/tray), Ctrl+C=double-press (first clears editor, second exits), Ctrl+D=exit, Ctrl+L=model selector, Ctrl+P=model cycle, Ctrl+N=new session, Ctrl+R=toggle attachment delete mode, Ctrl+Shift+C=copy selection to clipboard, Shift+Tab=cycle thinking level, Ctrl+T=toggle thinking blocks, Ctrl+O=expand tool output, Ctrl+G=open external editor, Ctrl+Z=suspend, Shift+G=scroll to bottom, Ctrl+S=cycle sandbox mode, F6=open panel picker. Tab cycles focus through editor → panel tray → active panel when panels are visible; Esc returns focus to editor.

**Thinking levels** control reasoning depth for providers that support it. Six levels: off, minimal, low, medium (default), high, xhigh. Configured via:
- Shift+Tab cycles through levels (editor border color changes with level)
- `/thinking <level>` slash command sets a specific level
- `WEAVE_THINKING_LEVEL` environment variable for initial level
- Models that don't support xhigh (e.g. Sonnet) are automatically clamped to high

Thinking level color mapping (grayscale temperature progression):
- Off: `240` (gray, theme.Border)
- Minimal: `242`
- Low: `244`
- Medium: `246`
- High: `248`
- XHigh: `250` (brightest gray)

**Sandbox modes** control OS-level tool execution guard. Four modes: `off` (no restrictions), `readonly` (writes blocked, file tools denied), `ask` (prompt per bash command via TUI dialog, file tools use path policy, denied in headless), `auto` (default, sandbox wraps bash commands and enforces path policy). Configured via:
- Ctrl+S cycles through modes (footer status pill shows `SB:off`, `SB:readonly`, `SB:ask`, `SB:auto`)
- `settings.json` `sandbox.mode` for initial mode
- Bus events: `sandbox.mode.change` (switch mode), `sandbox.approve`/`sandbox.approved`/`sandbox.denied` (ask-mode approval), `sandbox.trust` (session allowlist pattern)
- Mandatory deny paths are hardcoded (not configurable): writes to `~/.ssh/`, `~/.bashrc`, `~/.zshrc`, `~/.profile`, `~/.gitconfig`, `.git/hooks/`, `.git/config`, `.weave/`; reads from `~/.ssh/id_*`, `~/.aws/credentials`, `**/.env`, `**/.env.*`
- macOS uses Seatbelt (`sandbox-exec`), Linux uses bubblewrap (`bwrap`, must be installed separately)

**Model registry** (`sdk/model/`) provides curated model metadata (display name, reasoning support, context window, max tokens) via `model.RegisterModel`/`model.GetModel`/`model.ListModelsForProvider`/`model.ListAllModels`. Built-in models are registered by `model.RegisterBuiltinModels()`.

**StreamOptions** (`sdk/model/`) passes per-request options (model, thinking level, max tokens) to providers via functional options: `model.WithModel(model)`, `model.WithThinkingLevel(level)`, `model.WithMaxTokens(n)`. Providers read these instead of re-creating on model switch.

**Provider environment variables:**
- `ANTHROPIC_API_KEY` — API key for Anthropic provider (default model: `claude-sonnet-4-6`, override with `ANTHROPIC_MODEL`). OAuth token is also accepted if set via `/login`.
- `OPENAI_API_KEY` — API key for OpenAI provider (default model: `gpt-5.5`, override with `OPENAI_MODEL`). OAuth token is also accepted if set via `/login`.
- `OPENAI_BASE_URL` — override the default API base URL for OpenAI provider
- `ZAI_API_KEY` — API key for Z.ai provider (default model: `glm-5.1`, override with `ZAI_MODEL`). OAuth token is also accepted if set via `/login`.
- `ZAI_BASE_URL` — override the default API base URL for Z.ai provider
- `KIMI_API_KEY` — API key for Kimi provider (default model: `kimi-for-coding`, override with `KIMI_MODEL`). OAuth token is also accepted if set via `/login`.
- `KIMI_MAX_TOKENS` — override the default max tokens (32768) for Kimi provider
- `KIMI_BASE_URL` — override the default API base URL for Kimi provider
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

## Authentication and Login

Weave supports two authentication methods: API keys and OAuth. Both are stored in `~/.weave/auth.json` (permissions `0600`). Environment variables take precedence over file-based auth.

### `/login` slash command (TUI only)

Type `/login` in the TUI to open the auth selector. It shows all registered providers with their current auth status:
- **OAuth providers** (e.g., OpenAI, GitHub Copilot): launches the OAuth flow — browser redirect for authorization code flows, or device code + verification URL for device code flows.
- **API key providers** (all providers): opens a masked input dialog to enter the API key.

On successful login, `auth.json` is updated, auth status is refreshed in the model registry (`model.SetProviderAuth`), and a success notification is shown. The model selector (`Ctrl+L`) will include the newly authenticated provider's models.

### `/logout` slash command (TUI only)

Type `/logout` to see all providers with configured auth. Selecting a provider clears its credentials from `auth.json` and updates the model registry. The provider's models are removed from the model selector.

### OAuth flows

- **Authorization code** (OpenAI): PKCE verifier is generated, browser opens to the provider's auth URL with redirect to `localhost`, a temporary callback server receives the code, and tokens are exchanged via HTTP POST.
- **Device code** (GitHub Copilot): requests a device code from the provider, displays the user code and verification URL, polls the token endpoint at interval until authorized or timeout.

### OAuth provider registry (`sdk/`)

OAuth providers are registered in the SDK via `sdk.RegisterOAuthProvider(id, provider)`. Each provider defines its flow type (`AuthorizationCode` or `DeviceCode`), auth URL, token URL, device code URL (for device code flows), scopes, and optional client ID. `sdk.ListOAuthProviders()` and `sdk.GetOAuthProvider(id)` query the registry.

### Token refresh

When a provider uses an OAuth token, it checks expiry before each request. If the token is near expiry and a `refresh_token` is available, `auth.RefreshOAuthToken()` is called to exchange the refresh token for new access/refresh tokens, update `auth.json`, and retry the request. If refresh fails, the provider returns an auth-expired error.

### Headless mode

In headless mode (`-p` flag), `/login` and `/logout` are unavailable. Authentication must be set via environment variables or pre-populated `auth.json`.

## Session Resume

The session resume feature restores message history from the JSONL store into the agent loop when weave starts with `--continue` or `--resume <id>`. This works in both TUI (interactive) and headless (`-p`) modes.

### SessionStore interface (`sdk/session.go`)

```go
type SessionStore interface {
    ListSessions() ([]SessionInfo, error)
    LoadHistory(sessionID string) ([]Message, error)
}

type SessionInfo struct {
    ID        string
    CWD       string
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

The JSONL store (`extensions/store/jsonl/`) implements `SessionStore`. Wire injects it via the global getter/setter (`sdk.SetSessionStore`/`sdk.GetSessionStore`) following the same pattern as `FileTracker` and `FileMuter`. A `NoopSessionStore` zero-value stub is provided for nil-safety.

### Event flow

```
wire.Run()
  ├─ resolve --continue/--resume flags from CLI/config
  ├─ WireExtensions() wires all extensions (including jsonl store)
  ├─ resolveSession() → ListSessions/LoadHistory
  ├─ publish session.resume {SessionID, Messages}
  ├─ publish app.started
  └─ Extensions receive events
       ├─ agent.loop.Subscribe → receives session.resume, populates messages, sets resumed=true
       ├─ jsonl.Store.Subscribe → receives session.resume, sets internal sessionID so appends go to existing file
       └─ tui.Subscribe → receives session.resume, rebuilds chat display (preserves tool_calls and thinking blocks)
```

### Agent loop resume behavior

When the agent loop receives `session.resume`:
- `messages` is populated from `SessionResumePayload.Messages`
- `resumed` flag is set to `true`
- On the next `agent.prompt`, if `resumed` is true, the new user message is appended to the restored history instead of resetting messages (which would happen for a normal `agent.prompt`); `resumed` is then cleared

### Error handling

- No session found (`--continue` with empty store) → stderr message, exit 1 (headless) / warning log + fresh start (TUI)
- Invalid session ID (`--resume <id>` not found) → same behavior
- Corrupt JSONL entries during `LoadHistory` → skipped with warning log, partial history returned
- Headless errors from `resolveSession` propagate as fatal; TUI errors are logged as warnings and the session continues fresh

### Bus events

- `session.resume` — published by `internal/wire/` before `app.started`. Payload: `sdk.SessionResumePayload{SessionID string, Messages []sdk.Message}`. Subscribed by agent loop, JSONL store, and TUI.

## Design Reference

`docs/design.md` is **strong inspiration, not direct instruction**. It captures the architectural intent and data flow, but implementation details will evolve. Treat it as a north star, not a spec to copy verbatim.
