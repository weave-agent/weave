# SDK Core Package Restructure

## Overview
Split the flat `sdk/` package into focused sub-packages (`sdk/model/`, `sdk/registry/`, `sdk/wire/`) to improve organization, reduce coupling, and eliminate god-object files. Also fix bus concurrency issues and registry inconsistencies found during architectural review.

**Goals:**
- Extract `model.go` (296 lines, 4 concerns) into `sdk/model/` sub-package
- Create generic `Registry[T]` in `sdk/registry/` to eliminate 5 duplicated registry implementations
- Move wiring + entry-point logic to `sdk/wire/`, making `cmd/weave/main.go` a thin stub
- Fix bus race conditions, diagnostic deadlock risk, and duplicated subscriber logic
- Make `RegisterModel` warn on duplicates (like all other registries) instead of panicking

## Context

**Affected packages:**
- `sdk/` — source of all extracted code
- `bus/` — concurrency fixes
- `cmd/weave/main.go` (225 lines) — logic moves to `sdk/wire/run.go`
- `launcher/builder.go` — generates code referencing `sdk.WireWithCore`/`sdk.CoreWireConfig`
- `extensions/loop/`, `extensions/providers/{anthropic,openai,zai}/` — import `sdk.ModelDef`, `sdk.ThinkingLevel`, etc.
- `extensions/ui/tui/` — imports model types for model selector, thinking level display
- `utils/openaicompat/` — imports `sdk.ProviderEvent*`, `sdk.ToolCall`

**Import change scope (model types):**
- `sdk.ModelDef` → `model.ModelDef` (loop, providers, TUI)
- `sdk.ThinkingLevel` → `model.ThinkingLevel` (loop, TUI/palette, providers)
- `sdk.RegisterModel` → `model.RegisterModel` (providers, builtins)
- `sdk.StreamOptions` / `sdk.NewStreamOptions` / `sdk.WithModel` etc. → `model.*` (loop, providers)
- `sdk.ProviderAnthropic` / `sdk.ProviderOpenAI` → `model.ProviderAnthropic` / `model.ProviderOpenAI` (providers)
- `sdk.RegisterProviderEnvVar` / `sdk.ProviderEnvVar` → `model.*` (providers)
- `sdk.DefaultThinkingLevel` / `sdk.ParseThinkingLevel` → `model.*` (loop)

**Import change scope (wire types):**
- `sdk.WireWithCore` → `wire.WireWithCore` (launcher/builder.go generated code)
- `sdk.CoreWireConfig` → `wire.CoreWireConfig` (launcher/builder.go generated code)

**Dependency graph after restructure:**
```
sdk/registry/  ←  sdk/  ←  sdk/wire/
                           sdk/model/
```
No circular dependencies.

## Development Approach
- **Testing approach:** Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- Run tests after each change with `go test ./sdk/... ./bus/... ./cmd/...`
- Maintain backward compatibility within each task (no broken intermediate states)

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope

## Implementation Steps

### Task 1: Fix bus concurrency issues and cleanup
- [x] fix `Off()` race: hold `b.closeMu` write lock (not `b.mu`) through channel close, signal via `slot.done` channel instead of closing `slot.ch` while publisher may still write
- [x] add `recover()` in `publishDiagnostic()` to prevent deadlock if diagnostic handlers themselves panic
- [x] extract `collectSubscribers(topic string) []*handlerSlot` helper to deduplicate slot-copying logic between `Publish()` and `publishDiagnostic()`
- [x] add topic context to handler error log: `[bus] handler error on topic %q: %v` instead of `[bus] handler error: %v`
- [x] write/update tests: verify `Off()` safety under concurrent `Publish`, verify `publishDiagnostic` doesn't deadlock on handler panic, verify subscriber helper
- [x] run `go test ./bus/...` — must pass

### Task 2: Create `sdk/registry/` with generic `Registry[T]`
- [x] create `sdk/registry/registry.go` with generic `Registry[T any]` type supporting: `Register(name, T)`, `Get(name) (T, bool)`, `Exists(name) bool`, `List() []string`, `Reset()`
- [x] support configurable duplicate behavior via options: `WithWarn(*log.Logger, string)` (first-wins + warning) and `WithPanic(string)` (panic on dup)
- [x] write tests for `Registry[T]`: register/get, duplicate warn, duplicate panic, list, reset, concurrent access, empty name validation
- [x] run `go test ./sdk/registry/...` — must pass

### Task 3: Simplify sdk registry wrappers to use `sdk/registry.Registry[T]`
- [x] update `sdk/registry.go` (extension registry) to use `registry.New[func(Config) (Extension, error)]()` internally, keeping public API (`RegisterExtension`, `GetExtension`, `ListExtensions`, `ResetRegistry`) unchanged
- [x] update `sdk/tool_registry.go` similarly
- [x] update `sdk/provider_registry.go` similarly
- [x] update `sdk/ui_registry.go` similarly
- [x] update `sdk/ui_ext_registry.go` similarly
- [x] run existing `go test ./sdk/...` — all existing tests must pass with zero API changes
- [x] run `make gen` to regenerate mocks if needed

### Task 4: Create `sdk/model/` and migrate model code
- [x] create `sdk/model/types.go` — `ThinkingLevel` type + constants, `AllThinkingLevels`, `ModelDef` struct, `StreamOptions` struct, `StreamOption` func type, `NewStreamOptions`, `WithModel`/`WithThinkingLevel`/`WithMaxTokens`, `ClampForModel`, `DefaultThinkingLevel`, `ParseThinkingLevel`, `ProviderAnthropic`/`ProviderOpenAI` constants
- [x] create `sdk/model/registry.go` — model registry using `sdk/registry.Registry[ModelDef]` (with warn-on-dup behavior, matching other registries — this fixes the panic-on-duplicate review finding)
- [x] create `sdk/model/env.go` — provider env var registry (`RegisterProviderEnvVar`, `ProviderEnvVar`, `ResetProviderEnvVarRegistry`)
- [x] create `sdk/model/builtins.go` — `RegisterBuiltinModels()` with all hardcoded model entries + `init()` that calls it
- [x] move existing tests from `sdk/model_test.go` to `sdk/model/types_test.go` and `sdk/model/registry_test.go`, updating assertions for warn-instead-of-panic behavior change
- [x] run `go test ./sdk/model/...` — must pass

### Task 5: Update all consumers of model types
- [x] update `extensions/loop/loop.go` — change `sdk.ModelDef` → `model.ModelDef`, `sdk.ThinkingLevel` → `model.ThinkingLevel`, `sdk.NewStreamOptions` → `model.NewStreamOptions`, etc.; add `"weave/sdk/model"` import
- [x] update `extensions/providers/anthropic/anthropic.go` — `sdk.RegisterModel` → `model.RegisterModel`, `sdk.RegisterProviderEnvVar` → `model.RegisterProviderEnvVar`, `sdk.ProviderAnthropic` → `model.ProviderAnthropic`, etc.
- [x] update `extensions/providers/openai/openai.go` similarly
- [x] update `extensions/providers/zai/zai.go` similarly
- [x] update `utils/openaicompat/openai_compat.go` — `sdk.ProviderEvent*`/`sdk.ToolCall`/`sdk.SignedThinking`/`sdk.RedactedThinking` stay in `sdk` (not model), but check for any model imports
- [x] update `extensions/ui/tui/models.go`, `bridge.go`, `model.go` — `sdk.ThinkingLevel` → `model.ThinkingLevel`, `sdk.GetModel` → `model.GetModel`, etc.
- [x] update `extensions/ui/tui/palette/thinking.go` — `sdk.ThinkingLevel` → `model.ThinkingLevel`
- [x] delete `sdk/model.go` (all code extracted to `sdk/model/`)
- [x] run `go test ./sdk/... ./bus/...` — must pass (extension tests run separately)

### Task 6: Create `sdk/wire/` with Wire logic
- [ ] create `sdk/wire/wire.go` — move `Wire`, `WireWithCore`, `Wired`, `CoreWireConfig`, `validateCore`, `mergeCoreAndOptional` from `sdk/wire.go`
- [ ] update imports: `sdk/wire/` imports `weave/sdk` for interfaces and `weave/sdk/model` for thinking level constants
- [ ] move existing tests from `sdk/wire_test.go` to `sdk/wire/wire_test.go`, updating import paths
- [ ] delete `sdk/wire.go`
- [ ] update `launcher/builder.go` — change generated code to import `"weave/sdk/wire"` and use `wire.CoreWireConfig`/`wire.WireWithCore` instead of `sdk.*`
- [ ] run `go test ./sdk/wire/... ./launcher/...` — must pass

### Task 7: Extract cmd/weave/main.go logic into sdk/wire/run.go
- [ ] create `sdk/wire/run.go` — `Run(ctx context.Context, args []string) int` absorbing: `dispatch()`, `run()`, `resolveExtensionsAndMode()`, `writePromptFile()`, `resolveProjectDir()`, `ensurePresent()`, `findModuleRoot()`, `findModuleRootFrom()`, `isWeaveModule()` from `cmd/weave/main.go`
- [ ] `Run()` calls `config.Load`, `launcher.NewCache`, `launcher.NewLauncher`, `l.Run()` — same flow as current `main.go`
- [ ] `errNoInput` moves to `sdk/wire/run.go`
- [ ] handle `install` subcommand dispatch in `Run()` — move `runInstall` or delegate back to a separate install handler
- [ ] update `cmd/weave/main.go` to be a thin stub: `func main() { os.Exit(wire.Run(context.Background(), os.Args[1:])) }`
- [ ] write tests for `sdk/wire/run.go` covering: flag parsing, headless mode detection, extension resolution, prompt file creation, module root discovery
- [ ] run `go test ./sdk/wire/... ./cmd/...` — must pass

### Task 8: Final verification and cleanup
- [ ] run `make lint` — all issues must be fixed
- [ ] run `make fmt` — formatting must be clean
- [ ] run `go test ./sdk/... ./bus/... ./cmd/...` — all must pass
- [ ] run `cd extensions/loop && go test ./...` — must pass
- [ ] run `cd extensions/providers/anthropic && go test ./...` — must pass
- [ ] run `cd extensions/providers/openai && go test ./...` — must pass
- [ ] run `cd extensions/providers/zai && go test ./...` — must pass
- [ ] verify no stale references: `grep -r "sdk\.RegisterModel\|sdk\.GetModel\|sdk\.WireWithCore\|sdk\.CoreWireConfig\|sdk\.ThinkingLevel\|sdk\.ModelDef" --include="*.go" .` — should return zero results
- [ ] verify final package structure matches design: `sdk/model/`, `sdk/registry/`, `sdk/wire/`, clean `sdk/`
- [ ] update CLAUDE.md to reflect new package structure

## Technical Details

**Generic Registry[T] interface:**
```go
type Registry[T any] struct { ... }
func New[T any](opts ...Option[T]) *Registry[T]
func (r *Registry[T]) Register(name string, item T)
func (r *Registry[T]) Get(name string) (T, bool)
func (r *Registry[T]) Exists(name string) bool
func (r *Registry[T]) List() []string
func (r *Registry[T]) Reset()
```

**Bus subscriber helper:**
```go
func (b *Bus) collectSubscribers(topic string) []*handlerSlot {
    b.mu.RLock()
    slots := make([]*handlerSlot, 0, len(b.topicOn[topic])+len(b.allOn))
    slots = append(slots, b.topicOn[topic]...)
    slots = append(slots, b.allOn...)
    b.mu.RUnlock()
    return slots
}
```

**RegisterModel behavior change:** panics on duplicate → warns and keeps first (consistent with all other registries). The existing `TestModelRegistryDuplicatePanics` test must be updated to verify warn-instead-of-panic.

**cmd/weave/main.go becomes:**
```go
package main

import (
    "context"
    "os"
    "weave/sdk/wire"
)

func main() {
    os.Exit(wire.Run(context.Background(), os.Args[1:]))
}
```

## Post-Completion

**Manual verification:**
- Run `make bench` to verify build benchmarks still work with new package structure
- Test a full `weave` build and run to verify extension loading still works
- Verify `weave install` subcommand still works after cmd/weave/main.go simplification

**External system updates:**
- Any third-party extensions importing `sdk.ModelDef`, `sdk.ThinkingLevel`, etc. will need import path updates — this is a breaking API change for external extensions
- `go.mod` files in extension modules don't need changes (they import the root `weave` module)
