# Declarative Extension Config with Generic Registration

## Overview

Replace the imperative config loading pattern (`cfg.ToolConfig("name", &target)`) with declarative generic registration. Extensions declare config structs at `init()` time; the framework extracts JSON subtrees, applies defaults/env/validation, and passes populated structs to factories.

Key changes:
- Generic registration: `RegisterTool[T]`, `RegisterProvider[T]`, `RegisterExtension[T]`
- Custom config loader in `settings/` replacing gonfig for extension configs
- Per-provider typed config structs (no more hardcoded `ProviderEntry`)
- Full `--help` tree auto-generated from registered schemas
- Drop gonfig dependency entirely

## Context

- **Files/components involved:** `sdk/*`, `settings/*`, all 7 tool extensions, all 4 provider extensions, `extensions/ui/tui`, `extensions/loop`, `extensions/sandbox`, `extensions/store/jsonl`, `extensions/skills`, `extensions/instructions`, plus test mocks across modules
- **Pattern:** Extensions currently call `cfg.ToolConfig("bash", &bc)` or `cfg.ProviderConfig("kimi")` inside their factory functions. The new pattern removes these calls entirely.
- **Dependency:** `github.com/nniel-ape/gonfig` is used for top-level Settings loading + CLI flags. Will be replaced by custom loader + standard `flag` package.

## Development Approach

- **Testing approach:** Regular (implement, then test)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change

## Testing Strategy

- **Unit tests:** Required for every task. New code gets tests; modified code gets updated tests.
- **Mock regeneration:** After `sdk.Config` interface changes, run `make gen` to regenerate moq mocks.
- **Module-by-module:** Extensions are separate Go modules. After root module changes, each extension module must compile and pass tests before proceeding.

## Implementation Steps

### Task 1: Build custom config loader

- [x] Create `settings/loader.go` with `Loader` struct and `Load(target any)` method
- [x] Implement `applyDefaults(target)` using `default` struct tags
- [x] Implement `applyData(target, data map[string]any)` using JSON tag matching
- [x] Implement `applyEnv(target, prefix string)` using `env` struct tags
- [x] Implement `applyFlags(target, args []string)` using `flag`/`short` struct tags
- [x] Implement validation: `required`, `gt`, `lt`, `min`, `max`, `oneof`, `url`
- [x] Support custom `Validate() error` interface
- [x] Write tests for loader (defaults, data, env, flags, validation, nested structs)
- [x] Run `go test ./settings/...` — must pass

### Task 2: Update SDK Config interface

- [x] Remove `ToolConfig`, `UIConfig`, `ProviderConfig` from `sdk.Config` interface
- [x] Add `ExtensionConfig(scope, name string, target any, envPrefix string) error`
- [x] Update `noopConfig` and `FilePathConfig` to match new interface
- [x] Regenerate `sdk/config_mock_test.go` via `make gen`
- [x] Update `sdk/config_headless_test.go` if it asserts deleted methods
- [x] Write tests for `ExtensionConfig` delegation pattern
- [x] Run `go test ./sdk/...` — must pass

### Task 3: Add generic registration + schema capture

- [x] Change `RegisterTool` to generic: `RegisterTool[T any](name string, factory func(Config, T) (Tool, error))`
- [x] Change `RegisterProvider` to generic: `RegisterProvider[T any](name string, factory func(Config, T) (Provider, error))`
- [x] Change `RegisterExtension` to generic: `RegisterExtension[T any](name string, factory func(Config, T) (Extension, error))`
- [x] Add schema extraction at registration time (`extractSchema` reflection)
- [x] Store schemas in `sdk` registry for help generation
- [x] Update `GetTool`/`GetProvider`/`GetExtension` to populate config via `ExtensionConfig` before calling factory
- [x] Update `sdk/tool_registry_test.go`, `sdk/provider_registry_test.go`, `sdk/registry_test.go`
- [x] Run `go test ./sdk/...` — must pass

### Task 4: Settings cleanup + loader integration

- [x] Remove `ProviderEntry` struct from `settings/config.go`
- [x] Remove `TypedProviders`, `ProviderConfig` from `Settings`
- [x] Remove `ProviderConfig`, `providerEntryToSDK`, `ToolConfig`, `UIConfig` from `FullConfig`
- [x] Remove `populateConfig`, `applyDefaults` (old implementations) from `settings/config.go`
- [x] Implement `FullConfig.ExtensionConfig` using new `Loader`
- [x] Simplify `Settings` struct: keep `Providers`, `UI`, `Tools` as `map[string]any` raw containers
- [x] Update `settings/config_test.go`, `settings/settings_config_test.go`
- [x] Run `go test ./settings/...` — must pass

### Task 5: Top-level flag parsing without gonfig

- [x] Replace `gonfig.Load(&flags, WithFlags(args))` in `LoadFromDir` with custom flag parsing using `flag` package
- [x] Parse `-p`, `--output`, `--tools`, `--model`, `--sandbox`, `--subagent-id`, `--ui` flags
- [x] Keep `--help` / `-h` detection
- [x] Update `flagSet` struct tags if needed
- [x] Remove gonfig from `settings/config.go` imports
- [x] Run `go test ./settings/...` — must pass

### Task 6: Full-tree help generation

- [x] Create `settings/help.go` with `GenerateFullHelp()`
- [x] Walk all registered schemas and format flag lines
- [x] Group by scope: Global, Tools, Providers, Extensions, UI
- [x] Prefix extension flags with extension name (`--bash-timeout`, `--kimi-model`)
- [x] Show defaults and descriptions
- [x] Wire `--help` / `-h` in `LoadFromDir` to print help and exit
- [x] Write tests for help generation
- [x] Run `go test ./settings/...` — must pass

### Task 7: Provider extensions

- [x] **Anthropic:** Define `AnthropicConfig`, update registration to generic, remove `cfg.ProviderConfig` call
- [x] **OpenAI:** Define `OpenAIConfig`, update registration, remove `cfg.ProviderConfig` call
- [x] **Kimi:** Define `KimiConfig`, update registration, remove `cfg.ProviderConfig` call
- [x] **Z.ai:** Define `ZaiConfig`, update registration, remove `cfg.ProviderConfig` call
- [x] Update provider test mocks (remove `ProviderConfig` from mock `Config` implementations)
- [x] Run tests for each provider module (`cd extensions/providers/<name> && go test ./...`)

### Task 8: Tool extensions

- [x] **Bash:** Update to `RegisterTool[BashConfig]`, remove `cfg.ToolConfig` call
- [x] **Grep:** Update to `RegisterTool[struct{}]` (or define `GrepConfig` if needed), keep `cfg` for `RespectGitignore`
- [x] **Find:** Update to `RegisterTool[struct{}]`
- [x] **Read:** Update to `RegisterTool[struct{}]`
- [x] **Edit:** Update to `RegisterTool[struct{}]`
- [x] **Write:** Update to `RegisterTool[struct{}]`
- [x] **LS:** Update to `RegisterTool[struct{}]`
- [x] **Subagent:** Update dynamic tool registrations to generic
- [x] Update tool test mocks (remove `ToolConfig`, `UIConfig`, `ProviderConfig` from mock `Config` implementations)
- [x] Run tests for each tool module

### Task 9: Other extensions

- [x] **TUI:** Define `TUIConfig`, update to `RegisterExtension[TUIConfig]`, remove `cfg.UIConfig` call
- [x] **Loop:** Update to `RegisterExtension[struct{}]` (or define `LoopConfig` if needed)
- [x] **Sandbox:** Define `SandboxConfig`, update to `RegisterExtension[SandboxConfig]`, remove direct `gonfig.Load`
- [x] **Store/JSONL:** Define `JSONLOpts`, update to `RegisterExtension[JSONLOpts]`, remove direct `gonfig.Load`
- [x] **Skills:** Update to `RegisterExtension[struct{}]`
- [x] **Instructions:** Update to `RegisterExtension[struct{}]`
- [x] Update extension test mocks
- [x] Run tests for each extension module

### Task 10: Remove gonfig dependency

- [ ] Remove `github.com/nniel-ape/gonfig` from `go.mod`
- [ ] Run `go mod tidy`
- [ ] Verify no gonfig imports remain in codebase
- [ ] Run `make test` — must pass

### Task 11: Verify acceptance criteria

- [ ] All root module tests pass (`go test ./...` from root)
- [ ] All extension module tests pass (`cd extensions/* && go test ./...` for each)
- [ ] `make lint` passes
- [ ] `make fmt` produces no changes
- [ ] `--help` outputs full tree with all extension flags
- [ ] Extension configs load from JSON + env + defaults correctly
- [ ] Provider configs are typed per-provider (no `ProviderEntry`)
- [ ] No `ToolConfig`/`UIConfig`/`ProviderConfig` calls remain in extension code

## Technical Details

### Registration flow

```
extension init()
  └─> sdk.RegisterTool[BashConfig]("bash", factory)
        └─> captures: name="bash", type=BashConfig, factory
        └─> extracts schema via reflection
        └─> stores in registry

wire.GetTool("bash", cfg)
  └─> looks up "bash" in registry
  └─> creates zero BashConfig
  └─> calls cfg.ExtensionConfig("tools", "bash", &bc, "WEAVE_BASH")
        └─> FullConfig extracts settings.Tools["bash"]
        └─> Loader.Load(&bc) with subtree + env prefix
  └─> calls factory(cfg, bc)
```

### Config struct tags

| Tag | Purpose | Example |
|---|---|---|
| `json` | JSON key in settings file | `json:"timeout"` |
| `default` | Default value if unset | `default:"120"` |
| `validate` | Validation rules | `validate:"gt=0,lt=3600"` |
| `env` | Env var override name | `env:"TIMEOUT"` |
| `flag` | CLI flag name | `flag:"timeout"` |
| `short` | Short CLI flag | `short:"t"` |
| `description` | Help text | `description:"Command timeout"` |

### Priority order

defaults → JSON data → env vars → CLI flags → validation

## Post-Completion

- Update CLAUDE.md to document the new declarative config pattern
- Verify extension development docs mention config struct tags
- Consider adding a `weave config --schema` command to dump registered schemas as JSON (future enhancement)
