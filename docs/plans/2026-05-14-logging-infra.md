# Logging Infrastructure for TUI Mode

## Overview
Introduce a unified, file-based logging system that prevents stdout/stderr output from corrupting the Bubble Tea TUI while preserving diagnostic logs. Extensions use a standard `sdk.Logger(name)` API; the framework routes all logs to a rotating file (`~/.weave/logs/weave.log`). In headless mode logs are mirrored to stderr.

## Context
- Current registry code (`sdk/registry.go`, `sdk/provider_registry.go`, `sdk/tool_registry.go`, `sdk/ui_registry.go`, `sdk/ui_ext_registry.go`) uses `log.New(os.Stderr, ...)` for duplicate-registration warnings — these leak to stderr.
- Subagent tool (`extensions/tools/subagent/`) uses `log.Printf` which writes to stderr.
- Sandbox and jsonl store already use `slog` — they will work automatically once the default handler is configured.
- The generated `main.go` in `internal/launcher/builder.go` loads config before wiring extensions and starting the TUI. The file logger must be set up after config load (to know the data directory) but before TUI start.
- Crush uses the same pattern: `lumberjack` for rotation, `slog` as the API, discard in TUI unless `--debug`.

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Testing Strategy
- **Unit tests**: required for every task
- `internal/log/setup.go` — test that Setup creates the log directory, configures the handler, and respects debug flag
- `sdk/log.go` — test that Logger returns a logger with the `ext` attribute
- `sdk/registry/registry.go` — test that WithWarn callback is invoked on duplicates
- `extensions/tools/subagent/*` — existing tests should still pass after log migration

## Implementation Steps

### Task 1: Add lumberjack dependency and create internal/log package
- [x] `go get gopkg.in/natefinch/lumberjack.v2` in root module
- [x] create `internal/log/setup.go` with:
  - `Setup(logFile string, debug bool, extraWriters ...io.Writer)` — configures `slog.Default()` with a JSON handler writing to lumberjack, plus optional extra handlers (e.g. stderr in headless mode)
  - `Initialized() bool` — atomic flag so callers know if logging is ready
  - default log level: `Info`; debug flag sets `Debug`
  - lumberjack config: MaxSize=10, MaxAge=30, Compress=false
- [x] create `internal/log/setup_test.go`:
  - test Setup creates `~/.weave/logs/` directory
  - test log file is written after Setup
  - test debug flag changes level
  - test Initialized() returns true after Setup
  - test second Setup call is ignored (sync.Once)
- [x] run root module tests — must pass

### Task 2: Create sdk.Logger helper
- [x] create `sdk/log.go` with:
  - `Logger(name string) *slog.Logger` — returns `slog.Default().With("ext", name)`
- [x] create `sdk/log_test.go`:
  - test Logger adds "ext" attribute
  - test Logger uses current slog default (not cached)
- [x] run root module tests — must pass

### Task 3: Migrate registry warnings from log.Logger to slog callback
- [x] modify `sdk/registry/registry.go`:
  - change `WithWarn` signature from `(*log.Logger, string)` to `(func(name string), string)`
  - `onDup` calls the callback directly instead of `logger.Printf`
- [x] modify `sdk/registry.go`, `sdk/provider_registry.go`, `sdk/tool_registry.go`, `sdk/ui_registry.go`, `sdk/ui_ext_registry.go`:
  - replace `log.New(os.Stderr, ...)` with a `func(name string)` that calls `slog.Warn("duplicate registration", "name", name, "kind", label)`
- [x] update `sdk/registry/registry_test.go` if tests reference `WithWarn` signature
- [x] run root module tests — must pass

### Task 4: Migrate subagent extension from log.Printf to slog
- [ ] in `extensions/tools/subagent/discovery.go`:
  - replace `log.Printf` warnings with `slog.Warn` (or `sdk.Logger("subagent").Warn`)
- [ ] in `extensions/tools/subagent/broker.go`:
  - replace `log.Printf` with `slog.Warn`/`slog.Error`
- [ ] in `extensions/tools/subagent/stdin_listener.go`:
  - replace `log.Printf` with `slog.Warn`/`slog.Error`
- [ ] run subagent extension tests (`cd extensions/tools/subagent && go test ./...`) — must pass

### Task 5: Wire file logger into launcher generated main.go
- [ ] modify `internal/launcher/builder.go`:
  - after config is loaded (around line 531), add generated code that:
    - derives log directory from `cfg.FilePath()` or falls back to `~/.weave/logs/`
    - calls `log.Setup(logFile, debug)` where `debug` comes from a new `--debug` flag or `WEAVE_DEBUG` env var
  - add `--debug` CLI flag parsing alongside existing flags
- [ ] ensure `log.Setup` is called before `wire.WireWithCore()` so extension wiring logs go to file
- [ ] run launcher tests (`go test ./internal/launcher/...`) — must pass

### Task 6: Add --debug flag support to stub main.go
- [ ] modify `cmd/weave/main.go`:
  - add `--debug` flag parsing before calling `wire.Run()`
  - pass debug value through to launcher/settings as needed
- [ ] run root module tests — must pass

### Task 7: Verify no stderr leakage during TUI runtime
- [ ] grep the codebase for remaining `log.Printf` and `fmt.Fprintf(os.Stderr, ...)` in extension code (not launcher fatal errors)
- [ ] document the standard: extensions must use `slog` or `sdk.Logger()`
- [ ] run full test suite (`make test-all`) — must pass

### Task 8: Update documentation
- [ ] update `CLAUDE.md` logging section (if exists) or add a note about `sdk.Logger()`
- [ ] run linter (`make lint`) — must pass

## Post-Completion
- **Manual verification**: run weave in TUI mode, check `~/.weave/logs/weave.log` contains structured logs; verify no stderr corruption during operation
- **Future work**: user-installed extensions may still write to stdout/stderr — this is a fundamental limitation of Bubble Tea and cannot be prevented at the framework level
