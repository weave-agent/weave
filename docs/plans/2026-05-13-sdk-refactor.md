# SDK Refactor

## Overview

Address architectural, API design, and code quality findings from the SDK review. Twenty changes across `sdk/` to improve separation of concerns, interface ergonomics, and consistency. Some changes are blocked by or prerequisite to existing plans (declarative auth flow, rich UI extension API).

## Context (from discovery)

- **Files/components involved:** `sdk/*.go`, `sdk/wire/*.go`, `sdk/model/*.go`, `launcher/`, `settings/config.go`, all provider extensions, TUI, loop extension
- **Related plans:** `docs/plans/2026-05-13-declarative-auth-flow.md` (in progress), `docs/plans/2026-05-10-rich-ui-extension-api.md` (pending)
- **Dependencies identified:** Config split blocked by auth plan; UI split is pre-req to rich UI plan
- **Coverage:** `sdk/` 87.6%, `sdk/model/` 100%, `sdk/registry/` 100%, `sdk/wire/` 62.9%

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
- **Mock regeneration:** After interface changes, run `make gen` to regenerate moq mocks.
- **Module-by-module:** Extensions are separate Go modules. After root module changes, each extension module must compile before proceeding.

## Implementation Steps

### Task 1: Code quality batch — independent fixes

- [x] Extract `envPrefixFor(name string) string` helper; replace duplicated logic in `sdk/registry.go:37` and `sdk/tool_registry.go:30`
- [x] Add `const maxUIExtScanSize = 10 << 20` in `sdk/ui_extension.go`; replace magic number
- [x] Replace `//nolint:nilerr` suppressions in `sdk/ui_extension.go:19,47,52` with explicit logging of skipped files
- [x] Change `sync.Mutex` to `sync.RWMutex` in `sdk/lifecycle.go:10`
- [x] Remove redundant `active` bool in `sdk/tool_registry.go:45`; use `filter != nil`
- [x] Remove `type noopConfig = NoopConfig` alias in `sdk/config.go:35`
- [x] Extract helpers from `run()`: `loadConfig`, `buildLauncher`, `handleSubcommand` in `sdk/wire/run.go`
- [x] Split `Wire()` into `resolveExtensions()` + `subscribeExtensions()` in `sdk/wire/wire.go`
- [x] Write/update tests for all changes
- [x] Run `go test ./sdk/...` — must pass

### Task 2: Naming and consistency batch

- [x] Rename `ResetRegistry()` → `ResetExtensionRegistry()` in `sdk/registry.go:61`; update callers
- [x] Rename `sdk/ui_extension.go` → `sdk/ui_extension_detect.go`; update imports
- [x] Rename `wire.Wire()` → `WireExtensions()` in `sdk/wire/wire.go`; update callers
- [x] Extract `WEAVE_SINGLE_TURN` env var set from `WireWithCore` into explicit helper
- [x] Add `var ErrNotRegistered = errors.New("not registered")` in `sdk/`; use in provider and tool registries
- [x] Write/update tests for all changes
- [x] Run `go test ./sdk/...` — must pass

### Task 3: Move `sdk/wire/` to `internal/wire/`

- [x] Move `sdk/wire/*.go` to `internal/wire/*.go`
- [x] Update package declaration to `package wire` in all files
- [x] Update `cmd/weave/main.go` to import `weave/internal/wire` instead of `weave/sdk/wire`
- [x] Update launcher generated code template to import `weave/internal/wire`
- [x] Update all tests that import `weave/sdk/wire`
- [x] Write tests for moved code (tests moved with code)
- [x] Run `go test ./internal/wire/...` — must pass
- [x] Run `go test ./cmd/weave/...` — must pass

### Task 4: Move `IsUIExtension()` to `launcher/`

- [x] Move `IsUIExtension(dir string) bool` from `sdk/ui_extension_detect.go` to `launcher/ui_detect.go`
- [x] Update `launcher/auto_discover.go` to use local function instead of `sdk.IsUIExtension`
- [x] Remove `sdk/ui_extension_detect.go` if now empty
- [x] Update tests
- [x] Run `go test ./launcher/...` — must pass

### Task 5: Derive `envPrefix` inside `ExtensionConfig`

- [x] Remove `envPrefix` parameter from `ExtensionConfig(scope, name, target)` in `sdk/config.go`
- [x] Derive prefix inside `settings/config.go:445` based on scope: providers get `""`, tools/extensions get `"WEAVE_"+name`
- [x] Update `sdk/provider_registry.go:28` (remove `""` arg)
- [x] Update `sdk/tool_registry.go:30` (remove prefix arg)
- [x] Update `sdk/registry.go:37` (remove prefix arg)
- [x] Update `NoopConfig.ExtensionConfig` and `FilePathConfig.ExtensionConfig` stubs
- [x] Regenerate mocks via `make gen`
- [x] Write/update tests
- [x] Run `go test ./sdk/... ./settings/...` — must pass

### Task 6: Split `UI` interface

- [ ] Define `UIDialogs` interface in `sdk/ui.go` with `Select`, `Confirm`, `Input`, `MultiSelect`, `Editor`
- [ ] Define `UIStatus` interface in `sdk/ui.go` with `SetStatus`, `Notify`, `NotifyTyped`, `ShowError`, `SetWorking`, `ClearWorking`
- [ ] Define `UIRegistry` interface in `sdk/ui.go` with `RegisterCommand`, `RegisterRenderer`, `RegisterKeybinding`, `SetTheme`, `ListThemes`
- [ ] Compose `UI` interface from `UIDialogs` + `UIStatus` + `UIRegistry` (backward compat)
- [ ] Update `NoopUI` to implement all three sub-interfaces
- [ ] Regenerate mocks via `make gen`
- [ ] Write tests for sub-interface composition
- [ ] Run `go test ./sdk/...` — must pass

### Task 7: Replace global mutable state with bus events

- [ ] Add `sandbox.mode.change` subscription pattern for tools instead of `GetSandboxer()`
- [ ] Add `output.redirect` event type for output writer hooks instead of global setters
- [ ] Keep `app.started` for lifecycle (already event-based); remove `OnAppStarted` global
- [ ] Update tool extensions (bash, read, edit, write) to subscribe to bus for sandboxer
- [ ] Update TUI to publish `output.redirect` instead of calling `RegisterOutputWriterSetter`
- [ ] Remove `SetSandboxer`/`GetSandboxer`, `RegisterOutputWriterSetter`, `OnAppStarted` from `sdk/`
- [ ] Write tests for new bus events
- [ ] Run `go test ./sdk/...` — must pass

### Task 8: Make `RegisterUIExtension` generic with config

- [ ] Change `RegisterUIExtension(name string, ext UIExtension)` to `RegisterUIExtension[TConfig](name string, factory func(Config, TConfig) (UIExtension, error))`
- [ ] Add schema extraction for UI extension configs
- [ ] Update existing UI extensions (sandbox-ui, diff-viewer) to use new signature
- [ ] Update `sdk/wire/` to wire UI extensions with config
- [ ] Write tests
- [ ] Run `go test ./sdk/...` — must pass

### Task 9: Split `Config` into `Config` + `PreferenceStore`

**Blocked by:** `docs/plans/2026-05-13-declarative-auth-flow.md` (Task 2 removes `ResolveKey`)

- [ ] Remove `Preferences`, `SavePreferences`, `SaveProviderKey` from `sdk.Config` interface
- [ ] Define `PreferenceStore` interface with those three methods
- [ ] Update `FullConfig` to implement both interfaces
- [ ] Update all factory signatures: `func(Config, TConfig)` → `func(Config, PreferenceStore, TConfig)`
- [ ] Update TUI and loop to receive `PreferenceStore`
- [ ] Update `NoopConfig` — `PreferenceStore` methods go to `NoopPreferenceStore`
- [ ] Regenerate mocks via `make gen`
- [ ] Write/update tests
- [ ] Run `go test ./sdk/... ./settings/...` — must pass

### Task 10: Verify acceptance criteria

- [ ] All root module tests pass (`go test ./...`)
- [ ] All extension modules compile (`cd extensions/* && go test ./...` for each)
- [ ] `make lint` passes
- [ ] `make fmt` produces no changes
- [ ] No `ResetRegistry()` calls remain
- [ ] No `wire.Wire()` calls remain
- [ ] No `sdk.IsUIExtension` calls remain outside launcher
- [ ] No global `GetSandboxer()` calls remain in tools
- [ ] No global `OnAppStarted` calls remain
- [ ] `--help` still shows all extension flags correctly

## Technical Details

### Config split (after auth plan)

```go
type Config interface {
    FilePath() string
    ProjectDir() string
    ExtensionConfig(scope, name string, target any) error
    IsHeadless() bool
    RespectGitignore() bool
}

type PreferenceStore interface {
    Preferences(target any) error
    SavePreferences(target any) error
    SaveProviderKey(providerName, apiKey string) error
}
```

### UI split (pre-req to rich UI plan)

```go
type UIDialogs interface {
    Select(title string, items []string, opts ...SelectOption) (int, error)
    Confirm(message string, opts ...ConfirmOption) (bool, error)
    Input(prompt string, opts ...InputOption) (string, error)
}

type UIStatus interface {
    SetStatus(key, text string)
    Notify(message string)
    NotifyTyped(message string, level NotifyLevel)
    ShowError(message string)
    SetWorking(message string)
    ClearWorking()
}

type UIRegistry interface {
    RegisterCommand(name string, handler func(args string) error)
    RegisterRenderer(toolName string, renderer ToolRenderer)
    RegisterKeybinding(kb Keybinding)
    SetTheme(name string) error
    ListThemes() []string
}

type UI interface {
    UIDialogs
    UIStatus
    UIRegistry
}
```

### Wire move

```
sdk/wire/*.go → internal/wire/*.go
cmd/weave/main.go: weave/sdk/wire → weave/internal/wire
launcher/generate.go template: weave/sdk/wire → weave/internal/wire
```

## Post-Completion

- Update `CLAUDE.md` with new interface names and locations
- Verify `weave` binary builds and runs correctly
- Coordinate with declarative auth plan owner on Config split timing
- Coordinate with rich UI plan owner on UI split pre-req
