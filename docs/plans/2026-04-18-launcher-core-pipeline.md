# Launcher Core Pipeline

## Overview
Implement the full launcher pipeline for Weave with a single extension type (Extension/Hook) to prove the architecture works end-to-end: config resolution, extension discovery, dynamic build system, binary caching, and exec.

Bottom-up approach: minimal SDK interfaces and infrastructure first, then the launcher itself. Additional extension types (Tool, Provider, Runner, Store) will be added later.

## Context
- Module name: `weave` (Go 1.26.2)
- Zero `.go` files exist — greenfield implementation
- `docs/design.md` is the architecture reference (inspiration, not verbatim spec)
- External deps: minimal OK (testify for tests OK, YAML parsing via stdlib or minimal lib)
- Package layout: flat structure, each directory = one package = one concern
- **Only Extension/Hook type for now** — other types (Tool, Provider, Runner, Store) deferred

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change

## Target Package Layout
```
weave/
  cmd/weave/main.go          -- thin CLI entry point
  sdk/                       -- only package extensions import
    event.go                 -- Event type
    config.go                -- Config interface
    extension.go             -- Extension interface
    registry.go              -- extension registration
    wire.go                  -- Wire() composition root
  bus/bus.go                 -- channel-based event bus
  cfg/cfg.go                 -- config loading + discovery
  launcher/
    discovery.go             -- find extension source dirs
    builder.go               -- hash, generate go.mod/main.go, compile
    cache.go                 -- per-hash binary cache
    launcher.go              -- orchestration: discover → build → cache → exec
```

## Testing Strategy
- **Unit tests**: required for every task
- Table-driven tests where applicable
- Test both success and error paths
- Use temp dirs for filesystem-dependent tests

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## Implementation Steps

### Task 1: SDK core types and interfaces (`weave/sdk/`)
- [x] create `sdk/event.go` — Event struct (Topic string, Payload any, Timestamp time.Time) and NewEvent(topic, payload) constructor
- [x] create `sdk/config.go` — Config interface with typed accessors (GetString, GetInt, GetBool, GetStringSlice, Sub)
- [x] create `sdk/extension.go` — Extension interface (Name() string, Subscribe(bus)) + ExtensionFunc convenience type
- [x] write tests: interface satisfaction checks, NewEvent constructor, Event fields
- [x] run tests — must pass before next task

### Task 2: Registry (`weave/sdk/registry.go`)
- [x] create global extensions map (name → factory func) with sync.RWMutex
- [x] implement RegisterExtension(name string, factory func() Extension)
- [x] implement GetExtension(name string) (Extension, error) — instantiate from factory
- [x] implement ListExtensions() []string — return registered names
- [x] write tests for register → retrieve, duplicate registration, missing extension
- [x] run tests — must pass before next task

### Task 3: Event bus (`weave/bus/`)
- [ ] create `bus/bus.go` — Bus struct with topic subscriptions map and all-subscriptions channel
- [ ] implement New() constructor with buffered channels (64 per topic, 256 for all)
- [ ] implement Subscribe(topics ...string) returning <-chan Event
- [ ] implement SubscribeAll() returning <-chan Event
- [ ] implement Publish(Event) — non-blocking send, drop if buffer full
- [ ] implement Close() — close all channels
- [ ] write tests for pub/sub, multiple subscribers, buffer overflow, close behavior
- [ ] run tests — must pass before next task

### Task 4: Config loading and discovery (`weave/cfg/`)
- [ ] define ConfigFile struct matching `.weave.yaml` schema (Extensions []string, Slots map[string]string)
- [ ] implement config discovery — walk up from cwd looking for `.weave.yaml` then `.weave/config.yaml`
- [ ] implement YAML parsing into ConfigFile
- [ ] implement the sdk.Config interface backed by parsed data
- [ ] write tests for discovery, parsing, accessors, missing config
- [ ] run tests — must pass before next task

### Task 5: Wire composition root (`weave/sdk/wire.go`)
- [ ] implement Wire(config Config, bus *bus.Bus) error — resolve and subscribe all extensions listed in config
- [ ] iterate config extensions list, instantiate each via registry, call Subscribe(bus)
- [ ] return descriptive error on missing extension
- [ ] write tests with mock extensions verifying Subscribe is called for each
- [ ] run tests — must pass before next task

### Task 6: Extension discovery (`weave/launcher/discovery.go`)
- [ ] define ExtensionInfo struct (Name, Dir string, GoFiles []string)
- [ ] implement search: `.weave/extensions/{name}/` (project-local) then `~/.weave/extensions/{name}/` (global)
- [ ] collect all .go files in extension directory
- [ ] return clear error for missing extensions
- [ ] write tests using temp dirs for local/global discovery, missing extension
- [ ] run tests — must pass before next task

### Task 7: Builder (`weave/launcher/builder.go`)
- [ ] implement hash computation: SHA256 of Go version + sorted extension .go file contents
- [ ] implement go.mod generation with replace directives for sdk and each extension
- [ ] implement main.go generation with blank imports for each extension + sdk.Wire call
- [ ] implement Build(dir, extensions []ExtensionInfo) (binaryPath string, err error) — write files, run go build
- [ ] write tests for hash determinism, generated file content, build with trivial extension
- [ ] run tests — must pass before next task

### Task 8: Cache (`weave/launcher/cache.go`)
- [ ] define cache root: `~/.weave/bin/`
- [ ] implement Lookup(hash string) (binaryPath string, found bool) — check `~/.weave/bin/{hash}/weave`
- [ ] implement Store(hash string, binaryPath string) error — copy binary to cache dir
- [ ] write tests using temp dirs for hit/miss/store scenarios
- [ ] run tests — must pass before next task

### Task 9: Launcher orchestration (`weave/launcher/launcher.go`)
- [ ] implement Launcher struct with Builder and Cache dependencies
- [ ] implement Run(ctx, configPath string, args []string) error
- [ ] pipeline: discover extensions → compute hash → cache lookup → build if miss → syscall.Exec
- [ ] handle build failures with clear error output
- [ ] write tests for full pipeline with mocked build step
- [ ] run tests — must pass before next task

### Task 10: CLI entry point (`cmd/weave/main.go`)
- [ ] implement main() with `run` as default subcommand
- [ ] parse flags: -c config, -e extension override, -p prompt
- [ ] wire config discovery → Launcher.Run
- [ ] write tests for flag parsing and subcommand routing
- [ ] run tests — must pass before next task

### Task 11: Integration verification
- [ ] create a minimal noop extension (registers itself, subscribes to events)
- [ ] verify full pipeline: config → discovery → build → cache hit on second run → exec
- [ ] verify exec'd binary runs Wire and subscribes extensions
- [ ] run full test suite — all must pass
- [ ] verify test coverage ≥ 80%

## Technical Details

### Config file format (`.weave.yaml`)
```yaml
extensions: [noop, logging]
slots: {}
```

### Hash computation
SHA256 of: Go version string + sorted concatenation of all extension .go file contents.

### Generated go.mod example
```
module weave-built

go 1.26.2

require weave/sdk v0.0.0

replace weave/sdk => /path/to/weave/sdk
replace weave/ext/noop => /path/to/.weave/extensions/noop
```

### Generated main.go example
```go
package main

import (
    _ "weave/sdk"
    _ "weave/ext/noop"
)

func main() {
    // sdk.Wire called via init chain
}
```

## Post-Completion
*Items requiring manual intervention or external systems — no checkboxes*

**Manual verification**:
- End-to-end test with a real extension on the user's machine
- Verify behavior when Go toolchain is missing
- Profile build times for extension sets of varying sizes

**Next steps** (future plans, not this one):
- Add Tool extension type
- Add Provider extension type
- Add Runner and Store types
- Implement reference extensions (bash tool, JSONL store, turn runner)
