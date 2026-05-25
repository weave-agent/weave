# Auto-Populate Extension Defaults in settings.json

## Overview
When an extension's config section is missing from `settings.json` (or missing newly-added fields), automatically write the schema-declared defaults into the appropriate settings file. This gives users discoverability — they can see all available options and their defaults without reading source code — while preserving any values they've already customized.

The write is a side effect of `ExtensionConfig()`: runtime config loading continues to work exactly as before (defaults come from struct tags via `Loader`), but the source file gets populated so future edits are visible and self-documenting.

## Context
- **Schema registry** (`sdk/schema_registry.go`, `sdk/schema.go`): stores extracted `Schema` per `(scope, name)`; currently does not store the config struct's `reflect.Type`
- **Extension registration** (`sdk/registry.go`): `RegisterExtensionWithScope` extracts schema via reflection and stores it; needs to also store the type
- **Config loading** (`settings/config.go`): `ExtensionConfig()` loads from layered settings (global → project → local) via `getLayeredSettings()`; uses `Loader` which applies defaults → data → env → flags
- **Settings I/O** (`settings/settings.go`): `SaveSettings` writes JSON with indentation; no file-level locking currently
- **Defaults application** (`settings/loader.go`): `applyDefaults()` is package-private and reads `default` struct tags; we reuse it for populating the file

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- Run tests after each change
- Maintain backward compatibility

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope

## Implementation Steps

### Task 1: Store config struct type in schema registry
- [x] add `SchemaInfo` struct in `sdk/schema_registry.go` with `Schema Schema` and `Type reflect.Type` fields
- [x] change `schemas` map from `map[string]Schema` to `map[string]SchemaInfo`
- [x] update `storeSchema(scope, name, schema, typ)` signature to accept type
- [x] update `RegisterExtensionSchema` to extract and store the type
- [x] add `GetSchemaInfo(scope, name) *SchemaInfo` exported function
- [x] update `GetSchema` to return `SchemaInfo.Schema`
- [x] update `ListSchemas` to return `SchemaInfo.Schema`
- [x] update `ResetSchemas` and `ResetSchemasForScope` to clear the new map
- [x] update `sdk/registry.go`: pass `reflect.TypeOf(zero)` to `storeSchema` in both `RegisterExtensionWithScope` and `RegisterExtensionWithScopeAndWriter`
- [x] write tests for `GetSchemaInfo` (returns correct type, returns nil for missing)
- [x] write tests for schema registry reset/clear behavior with types
- [x] run `go test ./sdk/...` - must pass before task 2

### Task 2: Add file-level locking to SaveSettings
- [x] add `saveSettingsMu sync.Mutex` in `settings/settings.go` (package-level)
- [x] wrap `SaveSettings` body with `saveSettingsMu.Lock()` / `defer saveSettingsMu.Unlock()`
- [x] write test for concurrent `SaveSettings` calls (two goroutines, different settings, verify no corruption)
- [x] run `go test ./settings/...` - must pass before task 3

### Task 3: Build defaults-populate helpers
- [x] add `settings/populate.go` with `populateExtensionDefaults(sourcePath, scope, name string) error`
- [x] implement `buildDefaultsMap(schemaInfo *sdk.SchemaInfo) (map[string]any, error)`:
  - `reflect.New(schemaInfo.Type).Interface()` to create instance
  - call `applyDefaults(instance)` (package-private, accessible from `settings` package)
  - JSON marshal → unmarshal into `map[string]any` to get typed defaults
- [x] implement `mergeMissing(defaults, existing map[string]any) map[string]any`:
  - recursively merge: for each key in defaults, if not present in existing, add it
  - if both are maps, recurse; existing non-map values always win
- [x] implement `mapsEqual(a, b map[string]any) bool` for deep comparison
- [x] write tests for `buildDefaultsMap` with various struct types (string, int, bool, nested, pointers)
- [x] write tests for `mergeMissing` (missing keys added, existing keys preserved, nested merge)
- [x] write tests for `mapsEqual` (equal, different values, different keys, nested)
- [x] run `go test ./settings/...` - must pass before task 4

### Task 4: Determine source settings file and wire populate into ExtensionConfig
- [x] add `resolveSourcePath() (string, SettingsLayer, error)` method on `FullConfig`:
  - check if local settings (`.weave/settings.local.json`) exists via `loadLocalSettings` or `os.Stat`
  - if yes, return local path with `SettingsLocal` layer
  - else return `c.filePath` with appropriate layer (`SettingsProject` or `SettingsGlobal`)
- [x] modify `ExtensionConfig(scope, name, target)` in `settings/config.go`:
  - after the normal load (keep existing code), call `populateExtensionDefaults` with resolved source path
  - log warnings on populate failure (do not fail the load)
- [x] write tests for `resolveSourcePath` (local exists, only project exists, only global exists)
- [x] write integration test for `ExtensionConfig`:
  - create temp settings file without extension section
  - call `ExtensionConfig`, verify file now has defaults
  - call again, verify file unchanged (idempotent)
  - modify file with custom value, call again, verify custom value preserved
- [x] run `go test ./settings/...` - must pass before task 5

### Task 5: Verify acceptance criteria
- [x] verify all requirements from Overview are implemented
- [x] verify edge cases are handled (missing schema, nil type, empty defaults, concurrent calls)
- [x] run full test suite: `go test ./...`
- [x] run linter: `make lint`
- [x] verify test coverage meets project standard (80%+)

### Task 6: Update documentation
- [ ] update `sdk/config.go` interface doc or `sdk/registry.go` doc comment to mention auto-populate behavior
- [ ] add brief note in `settings/config.go` `ExtensionConfig` doc comment about side effect

## Technical Details

### Data structures and changes

**SchemaInfo** (`sdk/schema_registry.go`):
```go
type SchemaInfo struct {
    Schema Schema
    Type   reflect.Type
}
```

**Registry storage change**:
- `schemas map[string]Schema` → `schemas map[string]SchemaInfo`
- Key remains `scope + "/" + name`

### Parameters and formats

**populateExtensionDefaults**:
- `sourcePath`: absolute path to the settings file to write
- `scope`: config scope ("extensions", "tools", "providers", "ui", etc.)
- `name`: extension name within scope
- Returns error (logged but not propagated by caller)

### Processing flow

1. Extension calls `sdk.RegisterExtensionWithScope[T]("myext", "extensions", factory)`
2. Registry stores `SchemaInfo{Schema: extracted, Type: reflect.TypeOf((*T)(nil)).Elem()}`
3. Framework calls `cfg.ExtensionConfig("extensions", "myext", &target)` during wire init
4. `ExtensionConfig` loads config normally via `Loader` (defaults → data → env → flags)
5. `ExtensionConfig` then calls `populateExtensionDefaults(sourcePath, "extensions", "myext")`
6. Populate looks up `SchemaInfo`, builds defaults map, merges with existing file contents
7. If changed, writes back to source file with `SaveSettings` (which now has file locking)
8. `ExtensionConfig` returns — runtime config is unaffected by the file write

### Source path resolution

- Local (`SettingsLocal`): `.weave/settings.local.json` in project dir — checked first
- Project (`SettingsProject`): `.weave/settings.json` in project dir — used if no local
- Global (`SettingsGlobal`): `~/.weave/settings.json` — fallback

This respects the user's config hierarchy: per-developer overrides (local) get populated first, then project, then global.

## Post-Completion

**Manual verification**:
- Install an extension, check that its defaults appear in the active settings file
- Edit a default value, restart, verify edit is preserved
- Add a new field with a default to an extension's config struct, restart, verify new field appears in file

**No external system updates required** — this is an internal framework change with no breaking API changes.
