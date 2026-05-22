# Launcher Optimisations

## Overview

Implement launcher correctness fixes, startup UX improvements, cache management, and performance measurement for the generated-binary launcher pipeline. The work reduces stale-cache risk, avoids unnecessary headless rebuilds, prevents help/no-input commands from doing build/bootstrap work, bounds launcher cache growth, and adds measurement before changing `go mod tidy` behavior.

The changes integrate with the existing launcher flow in `internal/wire/run.go` and `internal/launcher/`, preserving the generated binary model while making cache keys match actual build inputs more closely.

## Context (from discovery)

- Files/components involved:
  - `internal/wire/run.go` — CLI flow, bootstrap timing, help/no-input handling, launcher invocation
  - `internal/launcher/launcher.go` — discovery/hash/cache/build/exec pipeline, build locking, core hash inputs
  - `internal/launcher/builder.go` — hash computation, generated `go.mod`, generated `main.go`, build commands
  - `internal/launcher/cache.go` — binary cache lookup/store under `~/.weave/bin`
  - `internal/launcher/discovery.go` — extension discovery and `IsUIExt` metadata
  - `internal/launcher/ui_detect.go` — UI extension detection used for headless filtering
  - `internal/launcher/*_test.go` and `internal/launcher/benchmark_test.go` — tests and benchmarks
  - `internal/extmanage/` — subcommands and bootstrap support; new cache clean command should live alongside current extension management commands or a small adjacent cache command handler
- Related patterns found:
  - Launcher hash already includes Go version, headless flag, agent loop, extension `.go` files, extension `.md` files, extension module files, selected core dirs, and one-hop local replaces.
  - Build already filters UI extensions in headless mode, but hashing happens before that filter.
  - Generated `main.go` prints full help after build/exec; `internal/wire/run.go` only bypasses no-input validation for help.
  - Cache store uses temp file + atomic rename and has no eviction.
- Dependencies identified:
  - Generated binaries import `internal/wire` and `internal/log`; `internal/wire` imports `internal/filemut` and `internal/filetracker`.
  - `settings.GenerateFullHelp()` is the existing full help generator.
  - Go `//go:embed` matching semantics matter for cache invalidation.

## Development Approach

- **Testing approach**: Regular — code changes first, then tests in each task
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - tests are not optional - they are a required part of the checklist
  - write unit tests for new functions/methods
  - write unit tests for modified functions/methods
  - add new test cases for new code paths
  - update existing test cases if behavior changes
  - tests cover both success and error scenarios
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Testing Strategy

- **Unit tests**: required for every task that changes code
- **Integration tests**: update launcher integration tests where launcher flow changes
- **Benchmarks**: add phase/cache-hit metrics before changing `go mod tidy` behavior
- Run targeted tests after each task, then full launcher tests before final verification

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope
- Keep plan in sync with actual work done

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): tasks achievable within this codebase - code changes, tests, documentation updates
- **Post-Completion** (no checkboxes): items requiring external action - manual testing, changes in consuming projects, deployment configs, third-party verifications
- **Checkbox placement**: Checkboxes belong only in Task sections (`### Task N:` or `### Iteration N:`). Do not put checkboxes in Success criteria, Overview, or Context — they cause extra loop iterations.

## Implementation Steps

### Task 1: Make launcher cache keys cover real build inputs
- [x] update `internal/launcher/launcher.go` so `coreDirs()` hashes broader `internal/` instead of a partial manual list
- [x] add `runtime.GOOS` and `runtime.GOARCH` to `ComputeHash` in `internal/launcher/builder.go`
- [x] update hash tests in `internal/launcher/builder_test.go` for OS/arch cache-key participation
- [x] update launcher tests in `internal/launcher/launcher_test.go` for the broader `internal/` core-dir set
- [x] run `go test ./internal/launcher` - must pass before task 2

### Task 2: Match headless hash inputs to compiled inputs
- [x] add a small helper in `internal/launcher/launcher.go` to derive build inputs from discovered extensions and `headless`
- [x] use the derived build inputs for both `ComputeHash` and `buildAndCache`
- [x] keep `Build()` defensive UI filtering for direct callers
- [x] add/update tests proving headless hash ignores UI-only extension changes while interactive hash includes them
- [x] run `go test ./internal/launcher` - must pass before task 3

### Task 3: Hash embedded resources from `//go:embed`
- [x] replace `.md`-only resource hashing in `internal/launcher/builder.go` with `//go:embed` directive parsing
- [x] support single files, multiple patterns, glob patterns, and embedded directories deterministically
- [x] preserve module-boundary and deterministic relative-path hashing behavior
- [x] add tests for `.sbpl` embedded files, glob patterns, multiple patterns, directory embeds, and missing/unmatched patterns
- [x] run `go test ./internal/launcher` - must pass before task 4

### Task 4: Make launcher build cancellation context-aware
- [x] update `BuildFunc` in `internal/launcher/launcher.go` to accept `context.Context`
- [x] thread `ctx` through `Launcher.Run`, `buildAndCache`, and `Build`
- [x] use `exec.CommandContext(ctx, ...)` for `go mod tidy` and `go build` in `internal/launcher/builder.go`
- [x] update affected launcher tests/mocks for the new build signature
- [x] add a cancellation-focused test for build command context handling where practical
- [x] run `go test ./internal/launcher` - must pass before task 5

### Task 5: Add no-build help path and delay bootstrap
- [ ] restructure `internal/wire/run.go` so help/no-input early exits happen before `runBootstrap`
- [ ] implement `--help` / `-h` handling that prints full help without invoking `launcher.Run`
- [ ] preserve existing no-input behavior and normal-run bootstrap behavior
- [ ] add/update tests in `internal/wire/run_test.go` for help fast path, no-input no-bootstrap behavior, and normal bootstrap positioning where feasible
- [ ] run `go test ./internal/wire ./internal/launcher` - must pass before task 6

### Task 6: Add size-based launcher cache eviction and cache clean command
- [ ] add size-based LRU eviction support to `internal/launcher/cache.go` after successful cache store
- [ ] update cache access metadata on successful lookup/store without breaking existing cache layout
- [ ] add a `weave cache clean` subcommand in the CLI dispatch path, scoped to `~/.weave/bin`
- [ ] add tests for eviction ordering, size limit behavior, lookup access updates, and `cache clean`
- [ ] run `go test ./internal/launcher ./internal/wire ./internal/extmanage` - must pass before task 7

### Task 7: Add launcher performance measurement before changing tidy behavior
- [ ] add or update benchmarks for cache-hit startup paths: no extensions, one extension, many extensions
- [ ] add benchmark phase metrics for discovery, hash, generated files, `go mod tidy`, `go build`, and cache store
- [ ] make end-to-end launcher benchmarks hermetic by avoiding real repo `.weave/settings.json` and real `~/.weave/bin`
- [ ] keep `go mod tidy` behavior unchanged in this plan
- [ ] run `go test ./internal/launcher -run '^$' -bench 'Benchmark' -benchtime=1x` - must pass before task 8

### Task 8: Verify acceptance criteria
- [ ] verify stale dev-cache risk is reduced for generated-binary dependencies
- [ ] verify headless cache keys exclude UI-only extension content
- [ ] verify embedded non-`.md` resources invalidate cache when changed
- [ ] verify `--help` does not build/exec generated binary
- [ ] verify cache eviction and `weave cache clean` affect only launcher binary cache
- [ ] run `go test ./internal/launcher ./internal/wire ./internal/extmanage`
- [ ] run `go test ./...`
- [ ] run linter if available for the project

### Task 9: Update documentation
- [ ] update `CLAUDE.md` or relevant project docs if launcher/cache behavior documentation exists
- [ ] document `weave cache clean` usage if user-facing command documentation exists
- [ ] update benchmark notes if existing benchmark docs mention launcher behavior

## Technical Details

- Cache hash changes:
  - Keep `agentLoop` in the hash for safety.
  - Add OS/arch lines near current Go version line.
  - Broaden dev-mode internal hashing to avoid stale generated binaries after internal dependency edits.
- Headless build inputs:
  - Discovery still records all extensions.
  - Hash/build use the same filtered extension slice in headless mode.
  - `Build()` remains defensive for direct callers.
- Embedded resource hashing:
  - Parse `//go:embed` directives from discovered non-test Go files.
  - Resolve paths relative to the Go source file directory.
  - Hash matched files in deterministic order using paths relative to extension root.
- Help/bootstrap flow:
  - Help path must not compile or execute a generated binary.
  - Bootstrap should only happen for commands that proceed to launch.
- Cache eviction:
  - Default max cache size should be conservative, e.g. 1 GB unless implementation discovers an existing config convention.
  - Eviction must not delete outside `~/.weave/bin`.
  - `weave cache clean` should remove launcher binary cache entries only.
- Build context:
  - Context cancellation should stop dependency resolution/build subprocesses.

## Post-Completion

**Manual verification**:
- Run `weave --help` from a clean cache and confirm no generated binary build message appears.
- Run headless prompt mode after editing a UI extension and confirm no unnecessary rebuild.
- Run a normal prompt after changing an embedded `.sbpl` or other embedded asset and confirm cache invalidates.
- Fill `~/.weave/bin` with multiple entries or lower the size limit in a test build and confirm eviction behavior.

**Performance verification**:
- Capture before/after benchmark data with `go test ./internal/launcher -bench=. -benchmem -count=5 -run=NONE` and compare with `benchstat`.

**External system updates**:
- None expected.
