# Simplify Extension Config — Auto-Discovery

## Overview

Radically simplify weave's extension configuration by moving from explicit compile-time selection to auto-discovery, similar to pi's extension system.

**Key changes:**
- Remove `extensions`, `ui_extensions`, and `core.providers` from `.weave.yaml`
- Auto-discover ALL extensions by recursively scanning for Go modules (dirs with `go.mod` + `.go` files)
- UI extensions detected at build time by parsing source for `RegisterUIExtension` calls
- UI extensions excluded from compilation when running headless
- All providers compiled in; runtime model selection via Ctrl+L / settings
- Minimal config: `core.agent_loop`, `ui`, `exclude_extensions`
- No backward compatibility (clean break)

**Problem it solves:** Users currently maintain explicit extension lists that get out of sync with what's installed. Built-in paths are hardcoded. Provider lists duplicate runtime settings.

**Key benefits:** Zero-config for standard setups. Extensions are independent modules. Adding/removing extensions is just file system operations.

## Context (from discovery)

**Files/components involved:**
- `config/config.go` — `File` struct, `CoreConfig`, `AllExtensions()`, `CoreExts()`, `TypedProviders()`
- `config/validation.go` — `ValidateWithConfigDir`, extension entry validation, provider validation
- `launcher/discovery.go` — `DiscoverWithBuiltins`, `findExtension`, `findBuiltin`, `collectGoFiles`
- `launcher/builder.go` — `BuildFunc`, `GenerateMainGo`, `GenerateGoMod`, `Build`
- `launcher/launcher.go` — `Launcher.Run`, `buildAndCache`, `exec`
- `sdk/wire/run.go` — `resolveExtensionsAndMode`, `ensurePresent`, provider env handling
- `sdk/wire/wire.go` — `WireWithCore`, `CoreWireConfig`, `mergeCoreAndOptional`
- `extensions/loop/loop.go` — provider selection via `WEAVE_PROVIDER` env var

**Related patterns found:**
- Extensions self-register via `init()` + `sdk.RegisterExtension/RegisterProvider/RegisterTool/RegisterUIExtension`
- Generated `main.go` blank-imports extension modules to trigger registration
- `WireWithCore` receives `agentLoop` + `providers` + `optExts`, sets `WEAVE_PROVIDER` env var
- UI extensions are separate from regular extensions — registered via `sdk.RegisterUIExtension()`
- Hash-based caching: `ComputeHash` SHA256 of extension contents invalidates binary cache
- Deduplication hierarchy: local (`.weave/extensions/`) > global (`~/.weave/extensions/`) > built-in (`moduleRoot/extensions/`)

**Dependencies identified:**
- All built-in extensions already have their own `go.mod` (verified)
- `sdk.ListProviders()` returns all registered providers at runtime
- Settings system (`~/.weave/settings.json`) already stores `provider` preference
- `cfg.Preferences()` interface method exists for reading settings

## Development Approach

- **Testing approach:** Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change

## Testing Strategy

- **Unit tests:** Required for every task
  - `launcher/discovery_test.go` — test `AutoDiscover` with mock directory trees
  - `launcher/builder_test.go` — test UI extension filtering in `GenerateMainGo`
  - `config/validation_test.go` — test simplified validation rules
  - `sdk/wire/wire_test.go` — test `WireWithCore` without providers list
- **Integration tests:** Run `go test ./...` from root and `cd` into extension modules
- **E2E:** Build and run weave to verify extensions load correctly

## Implementation Steps

### Task 1: Simplify config format and validation
- [x] Remove `Extensions`, `UIExtensions` fields from `config.File` struct
- [x] Remove `Providers` from `config.CoreConfig` struct
- [x] Add `ExcludeExtensions []string` to `config.File`
- [x] Remove `AllExtensions()`, `CoreExts()` methods from `config.File`
- [x] Update `ValidateWithConfigDir` — remove provider list validation, extension entry validation, ui_extensions validation
- [x] Keep `Providers map[string]any` field for per-provider settings (api_key, model, etc.)
- [x] Update `DefaultFile()` and `DefaultConfigJSON()` to reflect new format
- [x] Write tests for simplified `File` struct and validation
- [x] Run `go test ./config/...` — must pass before next task

### Task 2: Implement recursive AutoDiscover
- [x] Add `IsUIExt bool` field to `launcher.ExtensionInfo`
- [x] Implement `AutoDiscover(projectDir, homeDir, moduleRoot string, exclude []string)` in `launcher/discovery.go`
  - Recursively scan `.weave/extensions/`, `~/.weave/extensions/`, `moduleRoot/extensions/`
  - Detect Go modules: directory containing both `go.mod` and at least one non-test `.go` file
  - Name = directory basename
  - Collect `.go` files within module boundary (skip subdirs with their own `go.mod`)
  - Deduplicate by name: local > global > built-in
  - Apply `exclude` list
- [x] Implement `detectUIExtension(dir string, goFiles []string) bool` — scan `.go` files for `RegisterUIExtension(` substring
- [x] Remove old name-based discovery: `Discover`, `DiscoverWithBuiltins`, `findExtension`, `findBuiltin`, `checkBuiltinShadow`
- [x] Keep `collectGoFiles` but make it respect module boundaries
- [x] Write tests for `AutoDiscover` with temp directory trees
- [x] Write tests for `detectUIExtension` with sample Go source
- [x] Run `go test ./launcher/...` — must pass before next task

### Task 3: Update builder for headless UI filtering
- [ ] Update `BuildFunc` signature: remove `providers []string` parameter, add `headless bool`
  ```go
  type BuildFunc func(dir, moduleRoot, agentLoop string, headless bool, exts []ExtensionInfo) (string, error)
  ```
- [ ] Update `GenerateMainGo` — remove `providers` parameter, remove provider flag parsing from generated main.go
- [ ] Update `GenerateMainGo` — filter `IsUIExt` extensions when `headless` is true before generating blank imports
- [ ] Update `GenerateGoMod` — no changes needed (works with filtered list)
- [ ] Update `Build` — pass `headless` through, filter UI extensions before calling `GenerateMainGo`
- [ ] Update `ComputeHash` — hash must include headless flag so headless and interactive builds have different caches
- [ ] Write tests for UI extension filtering in generated main.go
- [ ] Write tests for hash difference between headless and interactive
- [ ] Run `go test ./launcher/...` — must pass before next task

### Task 4: Update launcher pipeline
- [ ] Update `Launcher.Run` — call `AutoDiscover` instead of `DiscoverWithBuiltins`
  - Pass `cf.ExcludeExtensions` as exclude list
  - Pass headless flag to `buildAndCache`
- [ ] Update `Launcher.buildAndCache` — pass headless to `l.Build`
- [ ] Update `Launcher.exec` — remove `--weave-providers=` flag
- [ ] Write tests for updated launcher pipeline
- [ ] Run `go test ./launcher/...` — must pass before next task

### Task 5: Update wire/run — remove provider list handling
- [ ] Remove `Providers` from `sdk/wire.CoreWireConfig`
- [ ] Update `WireWithCore` — remove provider validation, remove `WEAVE_PROVIDER` env var setting
- [ ] Update `mergeCoreAndOptional` — remove provider merging logic (only agent-loop + optExts now)
- [ ] Update `sdk/wire/run.go`:
  - Remove `resolveExtensionsAndMode` (replaced by auto-discovery)
  - `run()` calls `AutoDiscover` instead of `config.Load` + manual extension merging
  - Remove `ensurePresent` for skills/instructions (they're discovered automatically)
  - Remove provider env var handling (`WEAVE_PROVIDER`, `WEAVE_PROVIDER_AUTO`)
  - Keep headless detection and prompt file handling
- [ ] Update generated `main.go` template in `launcher/builder.go`:
  - Remove `providersFlag` parsing
  - Remove provider names from `wire.CoreWireConfig`
- [ ] Write tests for `WireWithCore` without providers
- [ ] Write tests for `mergeCoreAndOptional` simplification
- [ ] Run `go test ./sdk/wire/...` — must pass before next task

### Task 6: Update loop extension for settings-based provider selection
- [ ] Update `extensions/loop/loop.go` `init()`:
  - Read `cfg.Preferences(&prefs)` for default provider
  - Fall back to `sdk.ListProviders()[0]`
  - Fall back to `"anthropic"` if no providers registered
  - Still respect `WEAVE_PROVIDER` env var as explicit override
- [ ] Write tests for provider selection priority (env > settings > first registered > fallback)
- [ ] Run `cd extensions/loop && go test ./...` — must pass before next task

### Task 7: Update remaining references
- [ ] Search for `cf.Core.Providers` references across codebase and update/remove
- [ ] Search for `AllExtensions` references and update/remove
- [ ] Search for `WEAVE_PROVIDER` auto-set logic and clean up
- [ ] Update any integration tests that construct old config format
- [ ] Run `go test ./...` from root — must pass before next task

### Task 8: Verify acceptance criteria
- [ ] Verify `config.File` has only: `Prompt`, `UI`, `Core` (AgentLoop only), `Providers` (map for settings), `ExcludeExtensions`
- [ ] Verify `AutoDiscover` finds all built-in extensions recursively
- [ ] Verify UI extensions (`diff-viewer`) are excluded from headless builds
- [ ] Verify all providers are compiled in regardless of settings
- [ ] Verify runtime model selection (Ctrl+L) works with auto-discovered providers
- [ ] Run full test suite: `make test`
- [ ] Run linter: `make lint`
- [ ] Build weave binary and test interactive + headless modes

### Task 9: Update documentation
- [ ] Update `docs/design.md` if it references old config format
- [ ] Update example configs in any README or docs

## Technical Details

### Data structures

```go
// config.File — simplified
type File struct {
    Prompt            string         `short:"p"`
    UI                string         `default:"tui"`
    Core              CoreConfig     // only AgentLoop remains
    Providers         map[string]any // per-provider settings (api_key, model, etc.)
    ExcludeExtensions []string       `yaml:"exclude_extensions"`
}

type CoreConfig struct {
    AgentLoop string `default:"loop"`
}

// launcher.ExtensionInfo — with UI flag
type ExtensionInfo struct {
    Name       string
    Dir        string
    GoFiles    []string
    ModulePath string
    IsUIExt    bool
}

// sdk/wire.CoreWireConfig — without providers
type CoreWireConfig struct {
    AgentLoop  string
    SingleTurn bool
}
```

### AutoDiscover algorithm

```
for each root in [project/.weave/extensions, ~/.weave/extensions, moduleRoot/extensions]:
    walk directory tree recursively
    at each directory:
        if has go.mod AND has >= 1 non-test .go file:
            name = basename(dir)
            goFiles = collect .go files in dir, stopping at nested go.mod boundaries
            isUI = any goFile contains "RegisterUIExtension("
            add ExtensionInfo{name, dir, goFiles, modulePath, isUI}
            continue walking into subdirectories (for nested modules)

deduplicate by name: later roots override earlier (local > global > built-in)
remove entries whose name is in exclude list
return sorted list
```

### Hash computation

`ComputeHash` must include a `headless` marker so that:
- Headless build (excludes UI extensions) → hash A
- Interactive build (includes UI extensions) → hash B

This prevents cache collisions where a headless binary is reused for interactive mode (missing UI extensions).

### Provider selection at runtime

Priority (highest to lowest):
1. `WEAVE_PROVIDER` env var (explicit user override)
2. `settings.json` `"provider"` field (persisted user preference)
3. First registered provider (`sdk.ListProviders()[0]`)
4. `"anthropic"` (ultimate fallback)

## Post-Completion

**Manual verification:**
- Run `weave` interactively — verify all tools work, TUI loads, model selector shows all providers
- Run `weave -p "hello"` headlessly — verify output, no TUI dependencies
- Install a custom extension in `~/.weave/extensions/` — verify it's auto-discovered
- Add extension to `exclude_extensions` — verify it's skipped

**Configuration migration:**
- Users with old `.weave.yaml` will need to remove `extensions`, `ui_extensions`, `core.providers`
- Per-provider settings (`providers.anthropic.model`, etc.) stay in the same place
