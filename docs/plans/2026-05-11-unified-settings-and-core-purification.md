# Unified Settings System and Core Purification

## Overview

This plan addresses two architectural goals:

1. **Settings System Unification**: Merge the dual config system (`.weave.yaml` + `.weave/settings.json`) into a single JSON-only settings file. Rename `config/` package to `settings/`. Reduce from 3 layers to 2 (global + local).

2. **Core Purification**: Remove extension-specific business logic that has leaked into core modules (`sdk/`, `bus/`, `launcher/`, `sdk/wire/`, `utils/`, `sdk/model/`). Extensions define their own structs and parse opaque data; core only provides generic infrastructure.

## Context

- Current branch: `subagent-tool-extension` (uncommitted changes in `launcher/builder.go` and `launcher/builder_test.go`)
- 42 test files across affected packages
- `gonfig` library handles file parsing — will be reconfigured for JSON-only
- `sdk/` is imported by ~80 files — interface changes require mock regeneration

## Development Approach

- Regular approach (code first, then tests)
- Each task fully completed before next
- Small, focused changes
- Every task includes new/updated tests
- All tests must pass before next task
- Run linter after each task
- Update plan when scope changes

## Testing Strategy

- Unit tests for every task (required)
- MOQ mock regeneration when interfaces change (`make gen`)
- Run `make test` after each task
- Run `make lint` before final verification

## Implementation Steps

### Task 1: Move extension management out of sdk/wire/

Move `weave list/install/update/uninstall` commands from `sdk/wire/` to `cmd/weave/extmanage/`. Keep `sdk/wire/wire.go` and `sdk/wire/run.go` for runtime wiring only.

- [x] Create `cmd/weave/extmanage/` package
- [x] Move `sdk/wire/extmanage.go` → `cmd/weave/extmanage/list.go`, `cmd/weave/extmanage/update.go`, `cmd/weave/extmanage/uninstall.go`
- [x] Move `sdk/wire/install.go` → `cmd/weave/extmanage/install.go`
- [x] Move `sdk/wire/update_check.go` → `cmd/weave/extmanage/update_check.go`
- [x] Move `sdk/wire/extmanage_test.go` → `cmd/weave/extmanage/list_test.go` etc.
- [x] Move `sdk/wire/install_test.go` → `cmd/weave/extmanage/install_test.go`
- [x] Move `sdk/wire/update_check_test.go` → `cmd/weave/extmanage/update_check_test.go`
- [x] Move `OutdatedInfo`/`OutdatedEvent` from `sdk/event.go` to `cmd/weave/extmanage/`
- [x] Update `sdk/wire/run.go` to delegate subcommands to `cmd/weave/extmanage/`
- [x] Update imports across codebase
- [x] Run tests for moved packages
- [x] Run `make test` — must pass

### Task 2: Rename config/ → settings/ and drop YAML support

Rename package, update all imports, reconfigure gonfig for JSON-only, update file discovery.

- [x] Rename directory `config/` → `settings/`
- [x] Update package declaration in all files
- [x] Update all imports (`weave/config` → `weave/settings`) across ~16 files
- [x] Update `gonfig` usage to read JSON files
- [x] Update `FindConfigPath()` to look for `.weave/config.json` walked up from cwd
- [x] Update `EnsureGlobalConfig()` to create `~/.weave/config.json`
- [x] Remove YAML tag support from structs (keep JSON tags)
- [x] Update `launcher/builder.go` generated code to import `weave/settings`
- [x] Update `sdk/wire/run.go` to use `settings.LoadFromDir()`
- [x] Regenerate MOQ mocks if `sdk.Config` interface changes (no changes needed)
- [x] Update all tests referencing `config.` package
- [x] Run `make test` — must pass
- [x] Run `make lint` — must pass

### Task 3: Unify File and Settings structs

Merge `File` struct (from `.weave.yaml`) into `Settings` struct (from `.weave/settings.json`). Single struct, single load path.

- [x] Add `File` fields (`AgentLoop`, `UI`, `ExcludeExtensions`, `Providers`, `Sandbox`) to `Settings` struct
- [x] Merge `CoreConfig`, `ProviderEntry`, `SandboxFileConfig` into `Settings` or extension-owned structs
- [x] Update `FullConfig` implementation to use unified `Settings`
- [x] Update `LoadLayeredSettings()` to load the unified struct
- [x] Remove separate `File` loading path
- [x] Update `sdk.Config` interface to reflect unified structure
- [x] Update all callers of `config.File` or `config.FullConfig`
- [x] Write tests for unified settings loading
- [x] Write tests for 2-layer merge (global + local)
- [x] Run `make test` — must pass

### Task 4: Remove extension-specific validation from settings

Remove hardcoded extension names, sandbox modes, UI values, provider schemas from validation.

- [x] Remove `UIValueTUI`, `UIValueNone`, `DefaultAgentLoop`, `ExtBash` constants
- [x] Remove UI value validation (accept any string, runtime resolves via `sdk.GetUI()`)
- [x] Remove agent loop validation (accept any string, runtime resolves)
- [x] Remove `validateSandbox()` and `validSandboxModes`
- [x] Remove `validateProviderEntry()` — providers validate own config
- [x] Remove `exclude_extensions` entry validation (keep type check only)
- [x] Update validation tests
- [x] Run `make test` — must pass

### Task 5: Move auth out of settings

Move provider credential management to a dedicated location.

- [x] Move `settings/auth.go` → `internal/auth/`
- [x] Move `settings/auth_test.go` → `internal/auth/auth_test.go`
- [x] Remove `ProviderHasKey()` and `SetProviderKey()` from `sdk.Config` interface
- [x] Update `ResolveProviderKey()` to use auth package directly
- [x] Update all callers
- [x] Regenerate `sdk.Config` mock if needed
- [x] Run `make test` — must pass

### Task 6: Move UISettings to TUI extension

TUI extension defines its own config struct; core settings holds opaque data.

- [x] Remove `UISettings` from `settings/settings.go`
- [x] Create `extensions/ui/tui/settings.go` with TUI's own struct
- [x] Update TUI to use its local struct (already partially done via `uiSettings`)
- [x] Update `settings.Config.UIConfig()` to unmarshal opaque JSON into any target
- [x] Update merge logic to handle `ui.*` fields opaquely
- [x] Run `make test` — must pass

### Task 7: SDK cleanup — topics, sandbox modes, outdated types

Move extension-specific types out of `sdk/`.

- [x] Move `TopicSkillsLoaded` to `extensions/skills/`
- [x] Move `TopicInstructionsLoaded` to `extensions/instructions/`
- [x] Move sandbox mode constants (`SandboxOff`, etc.) and `NextSandboxMode()` to `extensions/sandbox/`
- [x] Move `SandboxModes` slice to `extensions/sandbox/`
- [x] Update all imports across codebase
- [x] Run `make test` — must pass

### Task 8: Launcher cleanup

Remove extension-specific logic from launcher generated code.

- [x] Create generic hook system in `sdk/` for output writer setters (`sdk.RegisterOutputWriterSetter`)
- [x] Update subagent extension to register via hook instead of named import
- [x] Remove subagent named import from `launcher/builder.go`
- [x] Remove `subagentext.SetStdoutWriter()` call from generated main
- [x] Move loop exclusion logic from `launcher/builder.go` to `sdk/wire/` or extension conflict metadata
- [x] Replace UI extension string scanning with `sdk.IsUIExtension(dir)` function
- [x] Update `launcher/builder_test.go`
- [x] Run `make test ./launcher/...` — must pass

### Task 9: Bus cleanup

Make diagnostic topics configurable instead of hardcoded.

- [x] Add `DiagnosticTopics []string` field to `Bus` struct
- [x] Update `invokeHandler()` to use configurable topics instead of hardcoded `extension.panic`/`extension.error`
- [x] Update `publishDiagnostic()` similarly
- [x] Provide defaults in `NewBus()` or constructor
- [x] Update bus tests to not hardcode topic names
- [x] Run `make test ./bus/...` — must pass

### Task 10: Utils and model cleanup

Remove provider-specific defaults from shared libraries.

- [x] Remove `defaultModel = "gpt-5.5"` from `utils/openaicompat/openai_compat.go`
- [x] Update callers to ensure `cfg.Model` is populated before calling `Stream()`
- [x] Move OpenAI-specific reasoning effort mapping from `openaicompat/` to `extensions/providers/openai/`
- [x] Move `DefaultThinkingLevel()` env var read from `sdk/model/types.go` to `settings/`
- [x] Update tests
- [x] Run `make test` — must pass

### Task 11: Wire cleanup

Remove extension-specific timing and CLI forwarding from wire.

- [x] Move TUI-specific update check timing to generic lifecycle event (`app.started`)
- [x] Update TUI extension to subscribe to lifecycle event
- [x] Replace hardcoded CLI flag forwarding with generic mechanism or extension self-parsing
- [x] Update `sdk/wire/run_test.go`
- [x] Run `make test ./sdk/wire/...` — must pass

### Task 12: Final verification

- [x] Run `make test` — full suite must pass
- [x] Run `make lint` — all issues fixed
- [x] Run `make fmt` — formatting clean
- [x] Verify no hardcoded extension names remain in core packages (remaining references are defaults in config structs and test data; business logic leakage removed)
- [x] Update README.md if settings file format changed (no README.md at root; design.md uses .agent.yaml as design concept, not implementation)
- [x] Update CLAUDE.md with new architecture boundaries

## Technical Details

### Settings file format (after unification)

`~/.weave/settings.json` (global):
```json
{
  "agent_loop": "loop",
  "ui": "tui",
  "exclude_extensions": [],
  "providers": {},
  "sandbox": { "mode": "auto", "writable": ["."] },
  "provider": "anthropic",
  "model": "claude-sonnet-4-6",
  "thinking_level": "medium",
  "respect_gitignore": true,
  "ui": { "editor_max_lines": 20 },
  "tools": { "bash": { "timeout": 120 } }
}
```

`<project>/.weave/settings.local.json` (project overrides):
Same schema, merged over global.

### Core/extension boundary (enforced)

| Core (`settings/`, `sdk/`, `bus/`, `launcher/`, `sdk/wire/`) | Extensions (`extensions/*`) |
|---|---|
| Generic interfaces, registries, types | Own config structs, validation, defaults |
| Generic event bus (no topic constants) | Own topic constants |
| Generic config loading (opaque JSON) | Parse own sections via `ToolConfig()`/`UIConfig()` |
| Generic launcher (discover, build, exec) | Self-register via `init()` |
| Extension management commands | — |

## Post-Completion

- Manual verification: test `weave install`, `weave list`, `weave update` commands
- Verify generated binary works with existing `.weave/` directories
- Check that existing user `settings.json` files load correctly
- Review with team for any missed leakage points
