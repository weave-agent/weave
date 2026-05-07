# Remove `weave/config` import from TUI extension

## Overview
The TUI extension (`extensions/ui/tui/`) directly imports `weave/config` to check provider auth status, load/save user preferences, and manage git exclusion. This violates the architecture rule: extensions communicate through `sdk.Config` and bus events, never by importing other packages directly.

All other extensions (loop, tools, providers, store, instructions) are clean. This fix brings the TUI in line by extending `sdk.Config` with 4 new methods, following the existing `UIConfig(any)` / `ToolConfig(name, any)` JSON-round-trip pattern.

## Context
- **Files importing `weave/config`**: `extensions/ui/tui/models.go`, `extensions/ui/tui/providers.go`, `extensions/ui/tui/model.go`
- **Config functions used**: `LoadAuth`, `LoadLayeredSettings`, `LoadSettings`, `SaveSettingsGlobal`, `SetProviderKey`, `EnsureLocalSettingsExcluded`
- **Config types used**: `AuthFile`, `ProviderAuth`, `Settings`, `UISettings`
- **Existing pattern**: `UIConfig(target any)` and `ToolConfig(name, target any)` already JSON-round-trip settings into any target struct — we extend this pattern
- **Interface impl**: `config.FullConfig` in `config/config.go` implements `sdk.Config`
- **Stub impls**: `noopConfig` and `FilePathConfig` in `sdk/config.go` need stubs for new methods

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- Run tests after each change

## Testing Strategy
- **Unit tests**: required for every task — new interface methods on `FullConfig`, updated TUI functions
- **Mock regeneration**: `make gen` after interface changes

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## Implementation Steps

### Task 1: Extend `sdk.Config` interface with 4 new methods
- [x] add `Preferences(target any) error`, `SavePreferences(target any) error`, `ProviderHasKey(providerName string) bool`, `SetProviderKey(providerName, apiKey string) error` to `sdk.Config` interface in `sdk/config.go`
- [x] add stub implementations to `noopConfig` and `FilePathConfig` in `sdk/config.go`
- [x] run `make gen` to regenerate `sdk/config_mock_test.go`
- [x] run `go test ./sdk/...` — SDK tests must pass
- [x] run `make lint` — no new warnings

### Task 2: Implement new methods on `config.FullConfig`
- [ ] implement `Preferences(target any) error` in `config/config.go` — wraps `LoadLayeredSettings(projectDir)` with JSON round-trip into target
- [ ] implement `SavePreferences(target any) error` in `config/config.go` — loads existing global settings, JSON-merges target fields, saves via `SaveSettingsGlobal`
- [ ] implement `ProviderHasKey(providerName string) bool` in `config/config.go` — checks env var via `model.ProviderEnvVar()` + `LoadAuth().GetProviderKey()`
- [ ] implement `SetProviderKey(providerName, apiKey string) error` in `config/config.go` — delegates to existing `config.SetProviderKey()`
- [ ] write tests for all 4 new methods in `config/config_test.go` (or extend existing)
- [ ] run `cd config && go test ./...` — config tests must pass

### Task 3: Update `extensions/ui/tui/models.go` — remove config import
- [ ] define local `preferences` struct with `Provider`, `Model`, `ThinkingLevel` fields (json-tagged)
- [ ] update `listModels()` to accept `cfg sdk.Config` param, replace `config.LoadAuth()` + `providerHasKey()` with `cfg.ProviderHasKey()`
- [ ] update `currentModel()` to accept `cfg sdk.Config` param, replace `config.LoadLayeredSettings()` with `cfg.Preferences(&prefs)`
- [ ] update `initialThinkingLevel()` to accept `cfg sdk.Config` param, replace `config.LoadLayeredSettings()` with `cfg.Preferences(&prefs)`
- [ ] update `saveSettings()` to accept `cfg sdk.Config` param, replace `config.LoadSettings()` + `config.SaveSettingsGlobal()` with `cfg.SavePreferences(&prefs)`
- [ ] delete `providerHasKey()` helper — no longer needed
- [ ] remove `import "weave/config"`
- [ ] update all callers of these functions in `model.go` to pass `m.cfg`

### Task 4: Update `extensions/ui/tui/providers.go` — remove config import
- [ ] update `listProviders()` to accept `cfg sdk.Config` param, replace `config.LoadAuth()` + `auth.GetProviderKey()` with `cfg.ProviderHasKey()`
- [ ] remove `import "weave/config"`
- [ ] update callers in `model.go` to pass `m.cfg`

### Task 5: Update `extensions/ui/tui/model.go` — remove config import
- [ ] define local `uiSettings` struct with `Theme string` and `EditorMaxLines int` (json-tagged) to replace `config.UISettings`
- [ ] replace `config.UISettings` usage with local struct
- [ ] replace `config.SetProviderKey()` with `m.cfg.SetProviderKey()`
- [ ] remove `config.EnsureLocalSettingsExcluded(configDir)` call (moved to builder in Task 7)
- [ ] remove `import "weave/config"`

### Task 6: Move `EnsureLocalSettingsExcluded` to generated binary
- [ ] add `config.EnsureLocalSettingsExcluded(filepath.Dir(cfgPath))` call in `launcher/builder.go` generated code, after `cfg = fullCfg` line (~441)
- [ ] update builder test if affected
- [ ] run `cd launcher && go test ./...` — launcher tests must pass

### Task 7: Update TUI tests
- [ ] update `extensions/ui/tui/models_test.go` — remove direct `config.SaveSettingsGlobal`/`config.LoadSettings` calls, use mock `sdk.Config` instead
- [ ] update mock config implementations in TUI tests to satisfy new interface methods
- [ ] run `cd extensions/ui/tui && go test ./...` — TUI tests must pass
- [ ] run `make lint` — no new warnings

### Task 8: Final verification
- [ ] run `grep -r '"weave/config"' extensions/ui/tui/*.go` — must return no results
- [ ] run `make gen` — mocks regenerate cleanly
- [ ] run `make lint` — no warnings
- [ ] run `go test ./sdk/... ./config/... ./launcher/...` — all pass
- [ ] run `cd extensions/ui/tui && go test ./...` — TUI tests pass

## Technical Details

**New `sdk.Config` methods:**
```go
Preferences(target any) error               // reads merged settings into target via JSON round-trip
SavePreferences(target any) error           // merges target fields into existing global settings, saves
ProviderHasKey(providerName string) bool    // checks env var + auth file
SetProviderKey(providerName, apiKey string) error
```

**TUI local types (defined in models.go):**
```go
type preferences struct {
    Provider      string `json:"provider,omitempty"`
    Model         string `json:"model,omitempty"`
    ThinkingLevel string `json:"thinking_level,omitempty"`
}
```

**TUI local types (defined in model.go):**
```go
type uiSettings struct {
    Theme          string `json:"theme,omitempty"`
    EditorMaxLines int    `json:"editor_max_lines,omitempty"`
}
```

**`SavePreferences` implementation** — loads existing global settings, marshals target to map, merges fields, saves back. Preserves UI/Tools sections.

**`EnsureLocalSettingsExcluded` move** — from TUI's `newModel()` to generated binary's main() in `launcher/builder.go`. The generated code already imports `weave/config`.

## Post-Completion
*Update CLAUDE.md if the interface description needs updating.*
