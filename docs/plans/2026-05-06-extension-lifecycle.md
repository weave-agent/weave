# Extension Lifecycle Commands (list, update, uninstall) + Outdated Notifications

## Overview
Add missing extension management commands (`list`, `update`, `uninstall`) and an automatic startup check that notifies users when installed extensions have newer versions available on their git remote. All new logic lives in core (`sdk/wire/`), with the TUI acting only as a presentation layer for outdated notifications.

**Problem**: Extensions can be installed but never updated, listed, or removed. Users have no visibility into which extensions are outdated.

**Key design decisions**:
- No manifest/metadata files — filesystem is the source of truth
- Git HEAD comparison (local vs remote) for outdated detection
- `git pull --ff-only` for safe updates (won't overwrite local changes)
- Core owns all logic, TUI is just a subscriber
- Startup check fires from `sdk/wire/` after wiring, not from any extension

## Context
- **Subcommand dispatch**: `sdk/wire/run.go` — `Run()` uses manual arg parsing (`args[0] == "install"`)
- **Install implementation**: `sdk/wire/install.go` — staging/swap pattern, `parseSource()`, `validateGoFiles()`
- **Discovery system**: `launcher/discovery.go` — `ExtensionInfo{Dir, GoFiles}`, three-tier discovery
- **Event bus**: `bus/bus.go` — `Publish(Event)`, `On(topic, Handler)`, `Event{Topic, Payload, Timestamp}`
- **TUI bridge**: `extensions/ui/tui/bridge.go` — `translateEvent()` maps bus events to Tea messages
- **Extensions dir**: `~/.weave/extensions/` for user-installed, `.weave/extensions/` for project-local

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- Every task includes new/updated tests
- All tests must pass before starting next task
- Update this plan if scope changes during implementation

## Testing Strategy
- **Unit tests**: required for every task
- Tests for success and error scenarios
- Mock filesystem operations where needed (temp dirs)

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## Implementation Steps

### Task 1: Add shared extension management helpers in `sdk/wire/extmanage.go`
- [x] create `sdk/wire/extmanage.go` with `extensionStatus` type: `{Name, Dir, SourceType(git/local), LocalHead, RemoteHead, Outdated bool}`
- [x] implement `listExtensionsDir() []extensionStatus` — scan `~/.weave/extensions/` and `.weave/extensions/`, detect `.git/` presence for source type
- [x] implement `checkOutdated(ext *extensionStatus) error` — compare `git rev-parse HEAD` vs `git ls-remote origin HEAD` with 10s timeout
- [x] implement `updateExtension(name string) error` — run `git pull --ff-only` in extension dir, return descriptive error on failure
- [x] implement `uninstallExtension(name string) error` — remove extension dir from `~/.weave/extensions/`, validate name exists
- [x] write tests for `listExtensionsDir` using temp dirs with git repos and plain dirs
- [x] write tests for `checkOutdated` (up-to-date, outdated, non-git, network error)
- [x] write tests for `updateExtension` (success, non-git, diverged, not found)
- [x] write tests for `uninstallExtension` (success, not found)
- [x] run tests — must pass before task 2

### Task 2: Add CLI subcommands (`list`, `update`, `uninstall`)
- [x] add `runList(args []string) int` in `sdk/wire/extmanage.go` — call `listExtensionsDir`, check outdated for git-sourced extensions, print formatted table to stdout
- [x] add `runUpdate(args []string) int` in `sdk/wire/extmanage.go` — with optional name arg (no name = update all git-sourced), call `updateExtension`
- [x] add `runUninstall(args []string) int` in `sdk/wire/extmanage.go` — require name arg, call `uninstallExtension`, warn if extension is referenced in active config
- [x] add subcommand dispatch branches in `sdk/wire/run.go` `Run()` function (matching existing `install` pattern)
- [x] write tests for `runList` output formatting
- [x] write tests for `runUpdate` (single, all, not found, no git extensions)
- [x] write tests for `runUninstall` (success, missing arg, not found)
- [x] run tests — must pass before task 3

### Task 3: Add startup update check in core
- [x] create `sdk/wire/update_check.go` with `OutdatedInfo{Name, LocalHead, RemoteHead}` struct
- [x] define event payload struct: `OutdatedEvent{Extensions []OutdatedInfo}`
- [x] implement `fireUpdateCheck(bus Bus)` — goroutine that iterates `~/.weave/extensions/`, compares HEAD for git-sourced extensions, publishes `extension.outdated` event on bus with list of outdated names
- [x] add `go fireUpdateCheck(bus)` call in `run()` (in `sdk/wire/run.go`) after wiring completes, before agent loop starts
- [x] respect offline mode — skip check if `WEAVE_OFFLINE=1` env var is set
- [x] log check start/completion to stderr for diagnostics
- [x] write tests for `fireUpdateCheck` using mock bus and temp git repos
- [x] write tests for offline mode skip behavior
- [x] run tests — must pass before task 4

### Task 4: Add TUI notification for outdated extensions
- [ ] add `extension.outdated` topic handling in `extensions/ui/tui/bridge.go` `translateEvent()` — return new `OutdatedNotificationMsg`
- [ ] define `OutdatedNotificationMsg` struct in bridge with `Extensions []OutdatedInfo`
- [ ] add handler in TUI model's `Update()` for `OutdatedNotificationMsg` — render styled banner in chat area listing outdated extension names with hint to run `weave update`
- [ ] style the notification banner (warning color, bordered box) matching existing diagnostic event styling
- [ ] write tests for bridge event translation
- [ ] write tests for TUI notification message handling
- [ ] run tests — must pass before task 5

### Task 5: Verify acceptance criteria and cleanup
- [ ] verify `weave list` shows installed extensions with correct status
- [ ] verify `weave update <name>` updates git-sourced extensions
- [ ] verify `weave update` (no args) updates all git-sourced extensions
- [ ] verify `weave uninstall <name>` removes extension
- [ ] verify startup check fires and publishes `extension.outdated` event
- [ ] verify TUI shows notification banner when outdated extensions detected
- [ ] verify headless mode does not show notification (no subscriber)
- [ ] verify `WEAVE_OFFLINE=1` skips update check
- [ ] run full test suite (`make test`)
- [ ] run linter (`make lint`) — all issues fixed

## Technical Details

**Extension status detection**:
```
~/.weave/extensions/<name>/.git/ exists → git-sourced, updatable
~/.weave/extensions/<name>/ no .git/    → local copy, not updatable
```

**Outdated check** (per extension):
```
localHead  = git rev-parse HEAD          (in extension dir)
remoteHead = git ls-remote origin HEAD   (10s timeout, suppress stderr)
outdated   = localHead != remoteHead
```

**Update command flow**:
```
weave update <name> → validate dir exists → validate .git/ exists → git pull --ff-only → log result
weave update        → iterate all extensions → skip non-git → update each → log results
```

**Uninstall command flow**:
```
weave uninstall <name> → validate name → check ~/.weave/extensions/<name>/ exists → os.RemoveAll → log result
```

**Startup check flow**:
```
run() → Wire() → go fireUpdateCheck(bus) → start agent loop
fireUpdateCheck:
  → list ~/.weave/extensions/
  → for each with .git/: compare HEAD vs remote
  → if any outdated: bus.Publish(Event{Topic: "extension.outdated", Payload: OutdatedEvent{...}})
```

**TUI notification rendering**:
```
┌─ Extension Updates Available ──────────────────────────────┐
│  mcp, diff-viewer have newer versions available.            │
│  Run `weave update` to update all, or `weave update <name>` │
└────────────────────────────────────────────────────────────┘
```

## Post-Completion
*Manual verification items — no checkboxes*

- Test with real git-sourced extensions installed via `weave install`
- Verify update check doesn't slow down TUI startup (should be async)
- Test with network offline to verify graceful degradation
- Test uninstall of extension referenced in `.weave.yaml` config
