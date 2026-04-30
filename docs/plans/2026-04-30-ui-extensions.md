# UI Extensions System

## Overview

Add a UI extension category for TUI-specific plugins (custom overlays, tool renderers, slash commands, keybindings). UI extensions live under `extensions/ui/tui/extensions/{name}/`, are listed in config under `ui_extensions`, discovered by the launcher alongside core extensions, and wired by the TUI at startup via `sdk.GetUIExtensions()`.

**Problem it solves:** Currently, adding TUI customizations requires modifying the TUI module itself. UI extensions let users (and built-in features) add overlays, renderers, commands, and keybindings as independent modules that only build when TUI is included.

**How it integrates:** Reuses the existing launcher discovery and build pipeline. One new config field, one new SDK registry, one new discovery path, zero hardcoded imports.

## Context

- Files/components involved:
  - `sdk/ui.go` — existing `UI` interface with `RegisterRenderer`, `RegisterCommand`, `RegisterKeybinding`
  - `sdk/extension.go` — existing registry pattern to mirror
  - `config/config.go` — `File` struct, `CoreExts()` method
  - `launcher/discovery.go` — `Discover()` function, `ExtensionInfo` struct
  - `cmd/weave/main.go` — extension list assembly
  - `extensions/ui/tui/tui.go` — `init()` registration, `Subscribe()` wiring
- Related patterns: existing `RegisterExtension`/`RegisterProvider`/`RegisterTool`/`RegisterUI` registry pattern (mutex, panic on dup, sorted listing)
- Dependencies: UI extensions depend on the TUI being in the build; they are silently skipped in headless mode

## Development Approach

- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**

## Testing Strategy

- **Unit tests**: required for every task
- **E2E tests**: N/A (no UI e2e framework)

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): code changes, tests
- **Post-Completion** (no checkboxes): manual verification, docs updates

## Implementation Steps

### Task 1: Add UIExtension interface and registry to SDK
- [x] create `sdk/ui_ext.go` with `UIExtension` interface (`Name() string`, `Register(ui UI)`)
- [x] implement `RegisterUIExtension(ext UIExtension)` and `GetUIExtensions() []UIExtension` using existing registry pattern (sync.RWMutex, panic on dup, sorted listing)
- [x] write tests for `RegisterUIExtension` — success case and duplicate panic case
- [x] write tests for `GetUIExtensions` — empty registry, multiple extensions, sorted order
- [x] run `go test ./sdk/...` — must pass before task 2

### Task 2: Add ui_extensions config field and merge logic
- [x] add `UIExtensions []string` field to `File` struct in `config/config.go` with `yaml:"ui_extensions"` tag
- [x] add `AllExtensions()` method to `File` that merges `Extensions` + `UIExtensions` (only when `UI == "tui"`)
- [x] write tests for `AllExtensions()` — empty slices, tui mode includes ui extensions, non-tui mode excludes ui extensions
- [x] run `go test ./config/...` — must pass before task 3

### Task 3: Update entry point to use AllExtensions
- [x] replace `coreExts, optExts := cf.CoreExts()` usage in `cmd/weave/main.go` with `cf.AllExtensions()` merge
- [x] ensure skills and UI extension injection still works correctly with the new merged list
- [x] run `go build ./cmd/weave/...` — must compile before task 4

### Task 4: Add TUI extensions discovery path to launcher
- [x] in `launcher/discovery.go`, add fallback search in `extensions/ui/tui/extensions/{name}/` for names not found in standard built-in locations
- [x] verify module path is resolved correctly for TUI extension subdirectory
- [x] write tests for discovery — standard extension found, TUI extension found, unknown extension error
- [x] run `cd launcher && go test ./...` — must pass before task 5

### Task 5: Wire UI extensions in TUI Subscribe
- [x] in `extensions/ui/tui/tui.go` `Subscribe()` method, add `for _, ext := range sdk.GetUIExtensions() { ext.Register(t.ui) }` after `t.ui` initialization
- [x] write tests verifying UI extensions are called during Subscribe (mock UIExtension that records Register call)
- [x] run `cd extensions/ui/tui && go test ./...` — must pass before task 6

### Task 6: Create example UI extension (diff-viewer)
- [ ] create directory `extensions/ui/tui/extensions/diff-viewer/`
- [ ] implement `DiffViewer` struct with `Name()` and `Register(ui UI)` methods
- [ ] register in `init()` via `sdk.RegisterUIExtension`
- [ ] write tests for the extension — verify Name() and Register() calls UI methods
- [ ] run `cd extensions/ui/tui/extensions/diff-viewer && go test ./...` — must pass

### Task 7: Verify acceptance criteria
- [ ] verify UI extensions are discovered from `extensions/ui/tui/extensions/{name}/`
- [ ] verify `ui_extensions` config field works in `.weave.yaml`
- [ ] verify UI extensions are skipped in headless mode (`ui: none`)
- [ ] run full test suite (`make test`)
- [ ] run linter (`make lint`)

### Task 8: Update documentation
- [ ] update `CLAUDE.md` Architecture section with UI extensions description
- [ ] add UI extension directory convention to project structure docs

## Technical Details

**UIExtension interface:**
```go
type UIExtension interface {
    Name() string
    Register(ui UI)
}
```

**Config change:**
```yaml
ui_extensions:
  - diff-viewer
```

**Discovery order for UI extensions:**
1. `.weave/extensions/{name}/` (project-local)
2. `~/.weave/extensions/{name}/` (global)
3. `extensions/ui/tui/extensions/{name}/` (built-in, new fallback path)

**Wiring flow:**
1. Config lists `ui_extensions`
2. Launcher discovers + builds them (blank imports in generated `main.go`)
3. UI extension `init()` calls `sdk.RegisterUIExtension`
4. TUI `Subscribe()` iterates `sdk.GetUIExtensions()`, calls `ext.Register(ui)`

## Post-Completion

**Manual verification:**
- Test with a `.weave.yaml` containing `ui_extensions: [diff-viewer]`
- Verify diff-viewer renderer activates for edit tool output
- Verify headless mode (`weave -p "test"`) works without loading UI extensions
- Verify unknown UI extension name produces clear error from launcher

**Documentation updates:**
- Add UI extension authoring guide if needed
