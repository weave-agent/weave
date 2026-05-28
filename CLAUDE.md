# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A coding agent framework written in Go — event-driven, extension-based, with dynamic compilation of selected extensions at runtime. The framework core lives in this repo (`github.com/weave-agent/weave`). Extensions (agent loop, providers, tools, TUI, etc.) are developed in separate repos under `github.com/weave-agent/weave-<name>` and auto-installed to `~/.weave/extensions/` on first run.

## Commands

```bash
make lint          # Run golangci-lint
make fmt           # Format code (gofumpt, goimports, go fix) ALWAYS use make fix before manual fixing
make fix           # Auto-fix linter issues
make gen           # Regenerate mocks (moq)
make tools         # Install dev tools (moq, golangci-lint)
make bench         # Run launcher/build benchmarks
make test          # Run root module tests
make tidy          # Run go mod tidy
go test ./sdk/...  # Run tests for a single package
go test ./...      # Run all tests in this module
```

## Testing

- **Assertions**: Use `github.com/stretchr/testify` — `require` for fatal assertions, `assert` for non-fatal. Never use raw `t.Error`/`t.Fatal`.
- **Mocks**: Use moq-generated mocks exclusively. Run `make gen` after changing interfaces. Mocks live in `*_mock_test.go` files — never edit them by hand.
- **No hand-written mocks**: Set moq mock's `Func` fields for custom behavior — never create a new mock struct.

## Architecture

Standard library as much as possible. Every replaceable component is an extension. Extensions are independent Go modules that self-register via `init()`, each with its own `go.mod` in its own repo — test/lint by `cd`ing into that repo.

**Extension boundaries** — each extension owns one concern. Extensions communicate exclusively through bus events; they never import or call each other directly. If an extension needs data another extension owns, it listens for the event that publishes it.

**Fork territory** — Extensions needing deep structural changes (replacing the editor, rewiring focus lifecycle) should fork the TUI extension rather than shoehorning through `TUIExtAPI`.

**Module boundaries** — Root-module `internal/` packages are not importable by extensions. Anything extensions need to share must live in `sdk/`. Never place event payload types in `internal/` packages.

**Launcher pattern:** resolve config → handle help/no-input fast paths → bootstrap core extensions (first launch only) → auto-discover extensions → derive build inputs (headless excludes UI-only extensions) → build a custom binary (cached per hash) → exec it.

**Launcher cache:** Generated binaries are cached under `~/.weave/bin/<hash>/weave`. Cache keys include the Go runtime version, OS/arch, headless mode, agent loop, root module graph, extension Go files, embedded `//go:embed` resources, extension module files, selected core source directories, and local replace dependencies. Non-Go resource files only participate when referenced by `//go:embed`; unembedded `.md` files and assets do not invalidate the generated binary cache. The cache is capped at 1 GiB by default and evicts least-recently-used entries after successful stores, protecting the newly stored entry even if it exceeds the cap.

**Launcher cancellation:** Launcher build paths must propagate `context.Context`. `BuildFunc`, `Launcher.Run`, `buildAndCache`, `Build`, build locking, and build subprocesses share the same context; subprocesses must use `exec.CommandContext`.

**Provider context accounting:** Providers stream actual response usage with `sdk.ProviderUsage`. Providers that can preflight count fully rendered requests may optionally implement `sdk.TokenCounter`; existing providers are not required to. Use `TokenCountSourceExact`, `TokenCountSourceTokenizer`, or `TokenCountSourceHeuristic` to describe count quality. `ContextBudgetSnapshot` is arithmetic only; compaction and truncation policy stays in the agent extension. OpenAI-compatible usage parsing maps `prompt_tokens_details.cached_tokens` into `ProviderUsage.CacheReadTokens`.

## Key Packages

- `sdk/` — `Extension`, `Bus`, `Config`, `UI`, `PreferenceReader`/`Writer`, optional `ExtensionConfigWriter`, `SessionStore`, `FileTracker`, `FileMuter`, `Guardian`, `Sandboxer` interfaces; optional provider capabilities such as `TokenCounter`; shared provider accounting types (`ProviderUsage`, `TokenCount`, `ContextBudgetSnapshot`); global registries for extensions/providers/tools/UIs; schema registry with `SchemaInfo{Schema, Type}` metadata; `Logger(name)` for structured logging; `WithBus`/`BusFromContext` for context-based bus access; auth and OAuth helpers
- `sdk/model/` — model types, model registry, `StreamOptions` for per-request options
- `sdk/registry/` — generic `Registry[T]` used by all registries
- `sdk/providerhttp/` — provider HTTP transport config resolver and client factory with configurable timeouts
- `sdk/providerretry/` — provider retry config resolver with deep-merge support for defaults and per-provider overrides
- `sdk/retry/` — shared retry with exponential backoff and jitter support (`full`/`none` modes)
- `sdk/validate/` — JSON schema validator for tool arguments
- `sdk/throttle.go` — context-aware throttling helper for event streaming (first call immediate, subsequent calls deduplicated within interval)
- `sdk/guardian.go` — guardian policy contract, approval/profile/grant/overlay payloads, `GuardianSnapshot` including active overlays, and guardian event topic constants
- `sdk/sandbox.go` — sandbox containment contract, status/expansion payloads, and sandbox event topic constants
- `sdk/tool_events.go` — `ToolProgress` struct and event topic constants (`tool.start`, `tool.progress`, `tool.complete`, `tool.error`, `tool.interrupted`) for streaming tool output over the bus
- `bus/` — callback-based event bus (`Publish`/`On`/`OnAll`/`Off`) with per-handler goroutines and panic recovery
- `settings/` — JSON-only config system with `Loader` (defaults → JSON → env → CLI flags → validation), layered settings (global → local), `FullConfig` implementing `sdk.Config`, and settings writes coordinated by `saveSettingsMu`
- `internal/wire/` — composition root: `WireExtensions()`/`WireWithCore()`, `Run()` (full entry-point pipeline)
- `internal/launcher/` — auto-discovery, hash-based caching, size-bounded launcher binary cache, binary generation with blank imports
- `internal/auth/` — provider credential storage in `~/.weave/auth.json` (API keys + OAuth)
- `internal/extmanage/` — extension lifecycle (`install`/`list`/`update`/`uninstall`) and first-run bootstrap
- `internal/log/` — rotating file logging via `slog`
- `internal/filemut/` — per-file mutex for serializing concurrent edits
- `internal/filetracker/` — read-before-edit policy enforcement
- `utils/openaicompat/` — shared SSE parsing for OpenAI-compatible providers
- `utils/ripgrep/` — ripgrep binary detection
- `utils/truncate/` — output truncation (2000 lines / 50KB)

## Logging

Extensions must never write to `stdout` or `stderr` — output leaks corrupt the Bubble Tea TUI. Use `sdk.Logger(name)` to get a structured `*slog.Logger`. All logs go to `~/.weave/logs/weave.log`. In headless mode, logs also mirror to `stderr`.

## Agent Extension

The `agent` extension owns the conversation lifecycle: prompt assembly, turn loop, tool execution, skill discovery, context file loading.

**Prompt assembly** layers (in order): default prompt or SYSTEM.md → available tools → skills XML → context files (CLAUDE.md/AGENTS.md) → APPEND_SYSTEM.md → date + CWD.

**Turn loop** — outer loop waits for prompts; inner loop streams provider turns and executes tool calls. Read-only tools (`read`, `grep`, `find`, `ls`) execute concurrently; write tools (`edit`, `write`, `bash`, `subagent`) execute sequentially after reads. Step limit: 50 (configurable).

**Context compaction** — auto-triggers when token budget is exceeded, or manually via `/compact [instructions]`. Compaction summarizes old messages, preserving file operation awareness. Config: `CompactionConfig{Enabled, ReserveTokens, KeepRecentTokens, Model, MaxSteps}`.

**Extension lifecycle** — extensions call `sdk.RegisterExtension[T](name, factory)` in `init()`. Providers: `sdk.RegisterProvider[TConfig, TAuth]`. Tools: `sdk.RegisterTool[TConfig]`. UI/TUI: `sdk.RegisterUIExtension`/`RegisterTUIExtension`. Custom scope: `sdk.RegisterExtensionWithScope[T](name, scope, factory)`.

**Declarative provider auth** — Providers declare an auth struct with `json`/`env` tags, register with `sdk.RegisterProvider[TConfig, TAuth]`. Framework loads auth from `~/.weave/auth.json` + env vars (no `WEAVE_` prefix for providers).

## First-Run Bootstrap

When `~/.weave/extensions/` is empty, the framework clones the core extensions from `github.com/weave-agent/weave-<name>`. Triggered in `internal/wire/run.go` only for commands that proceed to launch; `--help` and no-input failures do not bootstrap or build. Skip with `--skip-bootstrap`.

## Configuration

JSON-only. Main config: `.weave/settings.json` (walked up from cwd), fallback `~/.weave/settings.json`. Layered settings merge global → local. Auth stored separately in `~/.weave/auth.json`.

Config structs use tags: `json`, `default`, `env`, `flag`, `short`, `validate`, `description`. Loader applies: defaults → JSON → env vars → CLI flags → validation.

Built-in config scopes: `tools`, `providers`, `ui`, `guardian`, `sandbox`, `jsonl`, `extensions`. Provider env vars resolve without `WEAVE_` prefix; tools/extensions use `WEAVE_<NAME>`.

Schema registration stores `SchemaInfo{Schema, Type}` per `(scope, name)`. Registration APIs pass the config struct type so settings can reconstruct typed defaults from `default` tags.

`ExtensionConfig(scope, name, target)` loads layered config, then best-effort populates missing schema defaults into the source settings file. Population failures are logged and do not fail config loading. Default population writes to `.weave/settings.local.json` if present, otherwise the active project `.weave/settings.json`, otherwise global `~/.weave/settings.json`. Settings writes that can race with default population must coordinate through `saveSettingsMu`; `SaveSettings` already holds it, and population holds it while read/merge/write is performed.

Privileged extensions that need to persist scoped configuration use the optional `sdk.ExtensionConfigWriter` capability, normally received through `RegisterExtensionWithScopeAndWriter`. `SaveExtensionConfig(scope, name, target)` writes only the scoped subtree to the same active settings layer used by default population: singleton scopes such as `guardian`, `sandbox`, `ui`, and `jsonl` write at the root key, while named scopes such as `tools`, `providers`, `extensions`, and `ui_extensions` write under `scope.name`. Keep persistence details in `settings`; extensions should pass typed config data rather than editing settings files directly.

Key env vars: `WEAVE_PROVIDER` (override active provider), `WEAVE_THINKING_LEVEL`, `WEAVE_OFFLINE`. Session resume: `--continue`/`-c`, `--resume <id>`/`-r`.

**Keybindings**: `.weave/keybindings.yaml`. Built-in: Esc=interrupt, Ctrl+C=double-press exit, Ctrl+L=model select, Ctrl+P=model cycle, Ctrl+N=new session, Shift+Tab=cycle thinking, Ctrl+S=cycle guardian profile, Ctrl+O=expand output, Ctrl+G=external editor.

**Thinking levels**: off, minimal, low, medium (default), high, xhigh. Models that don't support xhigh clamp to high.

**Guardian profiles**: `ask`, `auto`, `yolo`, or custom profile names. `ask` permits reads and harmless metadata automatically while prompting for writes, network, deletes, and unknown actions. `auto` permits normal coding actions and asks for risky or unknown actions. `yolo` runs most actions while retaining catastrophic blocks. Custom profiles live under `guardian.profiles`; select with `guardian.profile` or `--guardian-profile`.

**Guardian policy overlays**: trusted extensions can publish `sdk.GuardianPolicyOverlay` on `guardian.policy.overlay.push` to add or replace a runtime-only policy layer, and `sdk.GuardianPolicyOverlayPop` on `guardian.policy.overlay.pop` to remove one by ID. Active overlays appear in `GuardianSnapshot.Overlays` for display and diagnostics, but they do not create or appear in user-visible Guardian profile lists by default. `OverrideHardBlocks` must be an explicit overlay opt-in for trusted extensions; it is an architectural contract for cooperating extensions, not a security boundary.

**Sandbox containment**: sandbox is containment-only for approved shell commands. The guardian decides allow/ask/block before execution; sandbox wraps approved commands with OS-level filesystem and network boundaries. Expansion requests are ID-based and are handled through the guardian UI extension.

The old sandbox mode API is removed: no `--sandbox`, `--weave-sandbox-mode`, `WEAVE_SANDBOX_MODE`, `sandbox.mode`, `sandbox.writable`, `sandbox.deny_read`, or `sandbox.deny_write`. Use guardian profiles for policy and sandbox containment settings for boundaries.

Guardian env/CLI: `WEAVE_GUARDIAN_PROFILE`, `WEAVE_GUARDIAN_ASK_FALLBACK`, `--guardian-profile`, and generated extension flags such as `--guardian-ask_fallback`. Sandbox env/CLI: `WEAVE_SANDBOX_ENABLED`, `WEAVE_SANDBOX_FAIL_IF_UNAVAILABLE`, `WEAVE_SANDBOX_ALLOW_UNSANDBOXED_FALLBACK`, plus generated flags `--sandbox-enabled`, `--sandbox-fail_if_unavailable`, `--sandbox-allow_unsandboxed_fallback`.

**Extension management:**
- `weave install <source> [--name <name>]` — from git URL, GitHub shorthand, or local path
- `weave list` — show name, source, module path, status
- `weave update [<name>]` — `git pull --ff-only`; no args updates all
- `weave uninstall <name>` — remove from `~/.weave/extensions/`
- `weave cache clean` — remove launcher binary cache entries under `~/.weave/bin`
- `/reload` — invalidate cache, rebuild, re-exec

**Launcher benchmarks:** `internal/launcher/benchmark_test.go` covers cache-hit startup paths for no, one, and many extensions, plus phase metrics for discovery, hash computation, generated files, `go mod tidy`, `go build`, and cache store. Use `go test ./internal/launcher -run '^$' -bench 'Benchmark' -benchtime=1x` for a quick benchmark sanity check.
