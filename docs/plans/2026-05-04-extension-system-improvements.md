# Extension System Improvements

## Overview

Harden weave's extension system with four pi-inspired improvements:
1. **Callback-based bus** — replace channel-based `Subscribe`/`Unsubscribe` with `On`/`Off`/`OnAll` handler registration. Bus manages goroutine lifecycle and wraps every dispatch with `recover()`, making extension panics non-fatal.
2. **Collision diagnostics** — warn when extensions shadow built-ins (discovery-time) or register duplicate tools/providers (registration-time). First registration wins, collision is logged.
3. **Headless mode detection** — add `IsHeadless() bool` to `sdk.Config` so extensions can skip UI-dependent work in print mode.
4. **Install + Reload** — `weave install <source>` CLI subcommand to clone extensions into `~/.weave/extensions/`. `/reload` slash command invalidates cache and re-execs the launcher for a full rebuild.

## Context

### Files/components involved
- `sdk/extension.go` — Bus interface, Handler type, Extension interface
- `bus/bus.go` — Bus implementation (channel-based dispatch)
- `sdk/config.go` — Config interface (adding IsHeadless)
- `sdk/wire.go` — Wire/WireWithCore (uses Bus, needs migration)
- `launcher/discovery.go` — extension discovery (collision warnings)
- `launcher/launcher.go` — exec pipeline (pass env vars for reload)
- `sdk/registry.go` — tool/provider registration (duplicate warnings)
- Every extension's `Subscribe(bus Bus)` method — migrate from channels to callbacks
- New: `cmd/weave/install.go` — install subcommand

### Related patterns
- Extensions self-register via `init()` + `sdk.Register*()`
- `ExtensionFunc` wraps simple `func(Bus)` closures
- Bus uses buffered channels (topicBufSize=64, allBufSize=256) with non-blocking sends
- Launcher pipeline: discover → hash → cache → build → syscall.Exec

### Dependencies
- No new external dependencies
- Changes are internal to the SDK and bus packages
- All existing extensions must migrate (mechanical change)

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Maintain backward compatibility during migration (keep old API temporarily if needed)

## Testing Strategy
- **Unit tests**: required for every task
- Bus tests: verify callback dispatch, panic recovery, error logging
- Discovery tests: verify collision detection across tiers
- Config tests: verify IsHeadless propagation
- Integration: verify end-to-end flow after all tasks complete

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope

## Implementation Steps

### Task 1: Redesign Bus interface to callback model
- [ ] update `sdk.Bus` interface: replace `Subscribe`/`SubscribeAll`/`Unsubscribe` with `On(topic string, h Handler)`/`OnAll(h Handler)`/`Off(h Handler)` and add `Handler func(Event) error` type
- [ ] implement callback-based `bus.Bus` with per-handler goroutine management, non-blocking dispatch, and `recover()` wrapping every handler invocation
- [ ] on panic: log stack trace and publish `extension.panic` diagnostic event; on error return: log and publish `extension.error` diagnostic event
- [ ] write tests for callback dispatch (single handler, multiple handlers on same topic, OnAll)
- [ ] write tests for panic recovery (handler panics, process survives, diagnostic event published)
- [ ] write tests for error handling (handler returns error, diagnostic event published)
- [ ] write tests for Off (handler removed, stops receiving events)
- [ ] write tests for Close (all handlers stopped, pending events drained or dropped)
- [ ] run `cd bus && go test ./...` - must pass before task 2

### Task 2: Migrate Wire and ExtensionFunc to callbacks
- [ ] update `sdk.Wire()` and `sdk.WireWithCore()` to use new Bus interface — no channel consumption, just extension.Subscribe(bus) calls remain unchanged
- [ ] update `ExtensionFunc.Subscribe` — the closure receives the new Bus, extension authors use bus.On internally
- [ ] update all bus mocks (`make gen` to regenerate moq mocks from updated interface)
- [ ] write tests verifying Wire still correctly resolves and subscribes extensions
- [ ] run `go test ./sdk/...` - must pass before task 3

### Task 3: Migrate all built-in extensions to callback bus
- [ ] migrate `extensions/loop/` — replace channel loops with `bus.On()` handler registrations
- [ ] migrate `extensions/store/jsonl/` — replace channel loops with `bus.On()` handler registrations
- [ ] migrate `extensions/ui/tui/` bridge goroutine — replace channel subscriptions with `bus.On()` / `bus.OnAll()` handlers
- [ ] migrate `extensions/providers/anthropic/`, `extensions/providers/openai/`, `extensions/providers/zai/` — update any bus usage
- [ ] migrate all tool extensions (`bash`, `read`, `edit`, `write`, `grep`, `find`, `ls`) — update any bus usage
- [ ] migrate UI extensions (`diff-viewer`) — update any bus usage
- [ ] write/update tests for each migrated extension verifying handler registration and event processing
- [ ] run `make test` (root + all extension modules) - must pass before task 4

### Task 4: Add collision diagnostics
- [ ] update `DiscoverCustomHomeWithBuiltins` return signature to `([]ExtensionInfo, []string, error)` — warnings as second return value
- [ ] add `checkBuiltinShadow(moduleRoot, name)` that checks if a resolved local/global extension also exists as built-in, returns warning string
- [ ] update `DiscoverCustomHome` to also return `[]string` (empty, for API consistency)
- [ ] update `Discover` and `DiscoverWithBuiltins` wrappers to pass warnings through
- [ ] update `launcher.Run` to log warnings to stderr
- [ ] add duplicate registration warnings in `sdk.RegisterTool`, `sdk.RegisterProvider`, `sdk.RegisterUI`, `sdk.RegisterUIExtension` — log when name already exists, first registration wins
- [ ] write tests for discovery collision detection (local shadows built-in, global shadows built-in, no shadow)
- [ ] write tests for registration duplicate warnings (registering same tool name twice)
- [ ] run `go test ./launcher/... ./sdk/...` - must pass before task 5

### Task 5: Add headless mode detection
- [ ] add `IsHeadless() bool` to `sdk.Config` interface
- [ ] update `noopConfig.IsHeadless()` to return `true` (default: headless)
- [ ] update `FilePathConfig.IsHeadless()` to return `true`
- [ ] add `HeadlessConfig` struct that wraps a `Config` and overrides `IsHeadless()` based on whether TUI is included in the extension set
- [ ] in launcher pipeline, wrap config with `HeadlessConfig{headless: ui != "tui"}` before passing to Wire
- [ ] update moq mocks (`make gen`)
- [ ] write tests for HeadlessConfig wrapping (headless=true when ui=none, headless=false when ui=tui)
- [ ] run `go test ./sdk/... ./launcher/...` - must pass before task 6

### Task 6: Add weave install subcommand
- [ ] create `cmd/weave/install.go` with `runInstall` function that handles `weave install <source>` where source is a git URL, local path, or GitHub shorthand
- [ ] parse source: detect git URL (https://, git://), GitHub shorthand (github.com/user/repo), local path (./, /)
- [ ] derive extension name from repo name (basename without .git) or require `--name` flag
- [ ] clone/copy into `~/.weave/extensions/<name>/`, validate .go files exist
- [ ] add install subcommand wiring in `cmd/weave/main.go` (or wherever subcommands are dispatched)
- [ ] write tests for source parsing (git URL, GitHub shorthand, local path, invalid source)
- [ ] write tests for name derivation and validation
- [ ] run `go test ./cmd/weave/...` - must pass before task 7

### Task 7: Add /reload slash command
- [ ] in `launcher/launcher.go` exec function, pass `WEAVE_LAUNCHER_PATH` (path to original weave binary via `os.Args[0]`) and `WEAVE_BUILD_HASH` (current cache hash) as environment variables
- [ ] add `/reload` slash command handler in TUI that reads `WEAVE_LAUNCHER_PATH` and `WEAVE_BUILD_HASH`, removes the cache directory for the current hash, then calls `syscall.Exec` with the launcher path and original args
- [ ] store original args in an env var (`WEAVE_ORIG_ARGS`) at exec time so /reload can reconstruct the command
- [ ] write tests for env var passing in launcher exec
- [ ] write tests for cache invalidation logic
- [ ] run `make test` - must pass before task 8

### Task 8: Verify acceptance criteria
- [ ] verify all four improvements work end-to-end: callback bus, collision warnings, headless detection, install + reload
- [ ] verify extension panics are caught and don't crash the process
- [ ] verify collision warnings appear on stderr when local/global shadows built-in
- [ ] verify IsHeadless returns correct value in both interactive and print modes
- [ ] verify `weave install` clones an extension and it's discoverable on next run
- [ ] verify `/reload` invalidates cache and re-execs with fresh build
- [ ] run full test suite (`make test`)
- [ ] run linter (`make lint`) — all issues must be fixed

### Task 9: Update documentation
- [ ] update CLAUDE.md extension architecture section to reflect callback-based bus
- [ ] update extension interface docs (On/Off/OnAll instead of Subscribe/Unsubscribe)
- [ ] add `weave install` usage to docs

## Technical Details

### Callback Bus dispatch model
```
Extension calls bus.On("topic", handler)
  → Bus stores handler in map[topic][]Handler
  → When Publish(event) is called:
    → For each handler on event.Topic + all OnAll handlers:
      → launch goroutine with recover()
      → call handler(event)
      → on panic: log + publish "extension.panic" event
      → on error: log + publish "extension.error" event
```

### Handler identity for Off
Handlers are matched by interface value (function pointer identity). Each `bus.On` call with the same closure variable is the same handler. Extensions store their handler references if they need to call Off.

### Migration pattern (every extension)
```go
// Before:
func (e *Ext) Subscribe(bus sdk.Bus) {
    ch := bus.Subscribe("topic")
    go func() {
        for ev := range ch {
            e.handle(ev)
        }
    }()
}

// After:
func (e *Ext) Subscribe(bus sdk.Bus) {
    bus.On("topic", func(ev sdk.Event) error {
        return e.handle(ev)
    })
}
```

### Reload flow
```
/weave → launcher → build → exec → weave-built-abc123 (running)
  → user types /reload
  → delete ~/.weave/bin/abc123/
  → syscall.Exec(WEAVE_LAUNCHER_PATH, origArgs, env)
  → launcher re-discovers → new hash xyz789 → build → exec → weave-built-xyz789
```

### Install sources
- `weave install github.com/user/weave-ext-mcp` → git clone to `~/.weave/extensions/weave-ext-mcp/`
- `weave install https://github.com/user/weave-ext-mcp` → same
- `weave install ./my-local-ext` → copy to `~/.weave/extensions/my-local-ext/`
- `weave install github.com/user/repo --name mcp` → clone as `~/.weave/extensions/mcp/`

## Post-Completion

**Manual verification:**
- Test `/reload` in interactive TUI with a modified extension to verify rebuild picks up changes
- Test `weave install` with a real git repository
- Verify collision warnings appear in terminal output during startup
- Verify extension panic is caught and TUI shows diagnostic message

**External system updates:**
- Update any third-party extension repos with the new `bus.On` pattern
- Communicate breaking Bus interface change to extension authors
