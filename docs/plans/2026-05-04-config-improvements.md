# Config & Settings Improvements

## Overview

Improve weave's configuration system with four focused changes:
1. **Path-based extension resolution** — allow `./`, `../`, `/`, `~` prefixed paths in extension lists
2. **Layered settings files** — global → project → local merge with deep merge semantics
3. **Typed tool config schemas** — tools register typed config structs auto-populated from settings
4. **Config validation** — validate config on load with clear error messages

**Why**: Current system is rigid — extensions only by name, settings only persist provider/model/thinking, no validation, no project-local overrides.

## Context

### Files involved
- `config/config.go` — main config File struct, Load, discovery
- `config/settings.go` — Settings struct, LoadSettings/SaveSettings
- `config/validation.go` — new: config validation logic
- `config/merge.go` — new: settings layer merge logic
- `launcher/discovery.go` — extension discovery, validExtName
- `sdk/config.go` — Config interface, ProviderConfigEntry
- `sdk/tool_registry.go` — RegisterTool factory signature
- `extensions/tools/bash/` — example tool for typed config integration
- `extensions/ui/tui/` — consumer of settings (editor_max_lines, theme)

### Current patterns
- Config loaded via gonfig (file + env vars + flags)
- Settings: flat JSON struct with 3 fields, single file at `~/.weave/settings.json`
- Extension discovery: name-only, three-tier search (local → global → built-in)
- Tools receive `sdk.Config` but ignore it (no per-tool config)

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**
- Run tests after each change
- Maintain backward compatibility — existing `.weave.yaml` files must continue to work

## Testing Strategy
- **Unit tests**: required for every task
- `config/` tests run with `go test ./config/...`
- `launcher/` tests run with `go test ./launcher/...`
- Extension module tests run with `cd extensions/tools/bash && go test ./...`

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope

## Implementation Steps

### Task 1: Path-based extension resolution
- [ ] add `isPath(s string) bool` helper in `launcher/discovery.go` — detects `./`, `../`, `/`, `~` prefixes
- [ ] add `resolveExtensionPath(entry, configDir string) (string, error)` — expands `~`, resolves relative from configDir
- [ ] modify `DiscoverCustomHomeWithBuiltins` to branch: path-like entries resolve directly, bare names use existing discovery
- [ ] update `ExtensionInfo.Name` derivation for path entries — use directory base name
- [ ] write tests for `isPath` (all prefixes, bare name negative cases)
- [ ] write tests for `resolveExtensionPath` (relative, absolute, tilde, error cases)
- [ ] write tests for `DiscoverCustomHomeWithBuiltins` with path entries mixed with bare names
- [ ] run `go test ./launcher/...` — must pass before next task

### Task 2: Layered settings — data structures and merge
- [ ] create `config/settings.go` rewrite: expand `Settings` struct with `UI` and `Tools` sections
  ```go
  type Settings struct {
      Provider      string                 `json:"provider,omitempty"`
      Model         string                 `json:"model,omitempty"`
      ThinkingLevel string                 `json:"thinking_level,omitempty"`
      UI            *UISettings            `json:"ui,omitempty"`
      Tools         map[string]interface{} `json:"tools,omitempty"`
  }
  type UISettings struct {
      Theme         string `json:"theme,omitempty"`
      EditorMaxLines int   `json:"editor_max_lines,omitempty"`
  }
  ```
- [ ] create `config/merge.go` with `MergeSettings(layers ...*Settings) *Settings` — deep merge: nested objects merge recursively, primitives override, maps merge by key
- [ ] create `LoadLayeredSettings(projectDir string) (*Settings, error)` — loads global → project → local, merges
  - global: `~/.weave/settings.json`
  - project: `.weave/settings.json` (walk up from projectDir)
  - local: `.weave/settings.local.json` (same dir as project settings)
- [ ] update `SaveSettings` to accept a layer parameter (global vs project)
- [ ] write tests for `MergeSettings` (empty layers, override order, deep merge, nil handling)
- [ ] write tests for `LoadLayeredSettings` (temp dirs with multiple layers)
- [ ] run `go test ./config/...` — must pass before next task

### Task 3: Typed tool config schemas
- [ ] add `SettingsConfig` interface to `sdk/config.go`:
  ```go
  type SettingsConfig interface {
      ToolConfig(name string, target interface{}) error
      UIConfig(target interface{}) error
  }
  ```
- [ ] add `RegisterToolWithConfig` to `sdk/tool_registry.go` — factory receives config struct type, auto-populated from settings
- [ ] implement `ToolConfig` on `FullConfig` — reads from merged settings `tools.{name}` block, JSON round-trips into target struct, applies defaults from struct tags
- [ ] implement `UIConfig` on `FullConfig` — reads from merged settings `ui` block
- [ ] update bash tool as reference implementation:
  ```go
  type BashConfig struct {
      Timeout int `json:"timeout" default:"120"`
  }
  // in init(): sdk.RegisterTool("bash", func(cfg sdk.Config) (sdk.Tool, error) {
  //     var bc BashConfig; cfg.ToolConfig("bash", &bc); return &tool{timeout: bc.Timeout}, nil
  // })
  ```
- [ ] write tests for `ToolConfig` (populated struct, defaults, missing section)
- [ ] write tests for `UIConfig` (populated struct, defaults, missing section)
- [ ] run `go test ./config/... ./sdk/...` — must pass before next task
- [ ] run `cd extensions/tools/bash && go test ./...` — must pass

### Task 4: Config validation
- [ ] create `config/validation.go` with `Validate(f *File) error` function
- [ ] validate `ui` field: must be `"tui"` or `"none"`
- [ ] validate `core.agent_loop`: non-empty
- [ ] validate `core.providers`: non-empty slice, valid names
- [ ] validate extension entries: bare names match `validExtName`, paths are resolvable (dir exists with .go files)
- [ ] validate provider entries: known provider names have required fields
- [ ] return structured errors with field paths (e.g. `config.ui: invalid value "web", must be "tui" or "none"`)
- [ ] integrate `Validate` into `LoadFromDir` — call after gonfig load, return validation errors
- [ ] write tests for `Validate` (valid config, each invalid case, multiple errors)
- [ ] run `go test ./config/...` — must pass before next task

### Task 5: Wire settings into TUI and existing consumers
- [ ] update TUI to read `UISettings` from layered settings (editor_max_lines, theme)
- [ ] update TUI settings persistence (model/thinking level changes) to save to correct layer
- [ ] add `.weave/settings.local.json` to `.git/info/exclude` if it doesn't exist (prompt user, don't force)
- [ ] write tests for TUI settings integration
- [ ] run `go test ./config/... ./launcher/...` — full suite must pass

### Task 6: Verify acceptance criteria
- [ ] verify path-based extensions work: bare names, relative paths, absolute paths, tilde
- [ ] verify layered settings: global → project → local merge, deep merge semantics
- [ ] verify typed tool config: bash reads timeout from settings with defaults
- [ ] verify validation: invalid config produces clear field-level errors
- [ ] verify backward compatibility: existing `.weave.yaml` with no settings files still works
- [ ] run full test suite (`go test ./config/... ./launcher/... ./sdk/...`)
- [ ] run `make lint` — all issues fixed
- [ ] run `cd extensions/tools/bash && go test ./...`

### Task 7: Update documentation
- [ ] update `CLAUDE.md` Configuration section with new settings format and path-based extension syntax
- [ ] add example `.weave.yaml` showing path-based extensions and settings integration

## Technical Details

### Path resolution rules
- `./foo`, `../foo` — relative to config file's directory
- `/foo` — absolute path
- `~/foo` — expand via `os.UserHomeDir()`
- `foo` — bare name, existing discovery hierarchy

### Settings merge semantics
```json
// Global: {"thinking_level": "medium", "ui": {"theme": "dark"}}
// Project: {"model": "claude-opus-4-7"}
// Local: {"ui": {"editor_max_lines": 20}}
// Merged: {"thinking_level": "medium", "model": "claude-opus-4-7", "ui": {"theme": "dark", "editor_max_lines": 20}}
```

### Typed config population
1. Read `tools.{name}` from merged settings
2. JSON marshal the map
3. JSON unmarshal into target struct (struct tags for field mapping)
4. Apply defaults from `default` struct tags for zero-value fields

### Validation error format
```
config.ui: invalid value "web", must be "tui" or "none"
config.extensions[2]: path "./missing-ext" does not exist
config.core.providers[0]: unknown provider "anthrpocli"
```

## Post-Completion
*Items requiring manual intervention — no checkboxes, informational only*

**Manual verification**:
- Test with a real `.weave.yaml` using path-based extensions
- Test settings layering across global/project/local
- Verify TUI picks up `editor_max_lines` and `theme` from settings
- Verify bash tool respects `timeout` from settings

**Future considerations** (not in scope):
- Settings migration system (version field, auto-migrate old formats)
- `/settings` command in TUI for interactive editing
- Hot reload of settings without restart
- Per-extension config sections in main `.weave.yaml` (beyond tools/ui)
