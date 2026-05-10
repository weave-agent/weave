# rg-first approach for find and grep

## Overview

Switch the `find` and `grep` tool extensions from pure Go stdlib to an rg-first strategy: shell out to `ripgrep` when available (gets .gitignore, glob filtering, performance for free), fall back to the current stdlib implementation when rg is absent. Fill capability gaps in both tools (glob filter on grep, line truncation, binary file detection, `**/` patterns in find).

Add a `respect_gitignore` config key so users can toggle .gitignore honoring (on by default; rg path respects it natively via flags, fallback path uses hardcoded skip list).

## Context

- **Affected modules:** `extensions/tools/find/`, `extensions/tools/grep/`, `config/`, `sdk/`
- **Current state:** Both tools are pure Go stdlib with hardcoded skip dirs (`.git`, `node_modules`, `.hg`, `.svn`). No `.gitignore` support, no glob filtering in grep, no line truncation, no binary file detection.
- **Reference implementations:** crush (`rg --files --glob` for find, `rg --json` for grep, stdlib fallback), pi (`rg` + `fd` auto-downloaded)
- **Existing patterns:** bash tool uses `exec.CommandContext`; tools are independent Go modules with own `go.mod`; truncation via `utils/truncate/`
- **No new dependencies for fallback path** — stays pure Go stdlib

## Development Approach

- **Testing approach:** Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## Implementation Steps

### Task 1: Add rg binary detection helper

Create a shared rg detection utility in a new package that both find and grep can import.

- [x] create `utils/ripgrep/ripgrep.go` with `Find()` function (uses `sync.OnceValue` + `exec.LookPath("rg")` to cache result)
- [x] create `utils/ripgrep/ripgrep_test.go` — test that Find() returns empty string when rg absent, valid path when rg in PATH
- [x] run tests — must pass before next task

### Task 2: Add `respect_gitignore` config key

Wire the new setting through the config system so tools can read it.

- [x] add `RespectGitignore` field to the config/settings structs in `config/settings.go` (default: true)
- [x] add `RespectGitignore() bool` method to `sdk.Config` interface in `sdk/config.go`
- [x] implement the method on `config.FullConfig` in `config/config.go` — reads from settings, defaults to true
- [x] update `config/validation.go` if needed
- [x] write tests for the new config key (default value, explicit true/false)
- [x] run tests — must pass before next task

### Task 3: Refactor grep to rg-first

Replace the pure Go grep implementation with rg-first, stdlib-fallback.

- [x] add `include` parameter to grep tool definition (glob filter like `*.ts`, `{pattern}`)
- [x] implement `searchWithRipgrep()` — shells out to `rg --json -H -n` with pattern, path, include glob, ignoreCase/literal flags; parses structured JSON output into match structs; passes `--no-ignore` when `respect_gitignore` is false
- [x] refactor `Execute()` to try rg first, fall back to current stdlib `searchDir`/`searchFile` on rg failure
- [x] add line truncation to grep output (cap individual lines to ~500 chars)
- [x] add binary file detection to fallback path (skip files where first 512 bytes fail `http.DetectContentType` text check)
- [x] update grep tool description to mention rg and new capabilities
- [x] write tests for rg path (skip if rg not in PATH), fallback path, new `include` param, line truncation, binary skipping
- [x] run tests — must pass before next task

### Task 4: Refactor find to rg-first

Replace the pure Go find implementation with rg-first, stdlib-fallback.

- [ ] implement `findWithRipgrep()` — shells out to `rg --files --glob <pattern> --null` with path; passes `--no-ignore` when `respect_gitignore` is false; parses null-separated output
- [ ] add `**/` recursive pattern support: when pattern contains `/`, pass `--full-path` and prepend `**/` if needed (matches crush's approach)
- [ ] refactor `Execute()` to try rg first, fall back to current stdlib `filepath.WalkDir` on rg failure
- [ ] update find tool description to mention rg and glob support
- [ ] write tests for rg path (skip if rg not in PATH), fallback path, `**/` patterns, existing test cases still pass
- [ ] run tests — must pass before next task

### Task 5: Update go.mod for both tool modules

Add the `utils/ripgrep` dependency to both tool modules.

- [ ] update `extensions/tools/find/go.mod` — add replace directive for `utils/ripgrep`
- [ ] update `extensions/tools/grep/go.mod` — add replace directive for `utils/ripgrep`
- [ ] run `go mod tidy` in both directories
- [ ] verify both modules build and test cleanly

### Task 6: Verify acceptance criteria

- [ ] verify both tools use rg when available and fall back to stdlib when absent
- [ ] verify `respect_gitignore` config key works (true/false/default)
- [ ] verify grep has `include` glob filter parameter
- [ ] verify find supports `**/` patterns via rg
- [ ] verify binary files are skipped in grep fallback
- [ ] verify line truncation in grep output
- [ ] run full test suite for both modules
- [ ] run `make lint` — all issues must be fixed

## Technical Details

### rg commands

**grep:**
```
rg --json -H -n [-i] [-F] [--glob <include>] [--no-ignore] <pattern> <path>
```

**find:**
```
rg --files --glob <pattern> --null [--no-ignore] <path>
rg --files --glob <pattern> --null --full-path [--no-ignore] <path>  (when pattern contains /)
```

### Config key

```yaml
# .weave/settings.json
{
  "respect_gitignore": true
}
```

Defaults to `true`. When `false`, both tools pass `--no-ignore` to rg (or skip .gitignore dirs in fallback).

### Binary detection

`utils/ripgrep.Find()` returns the path to `rg` or empty string. Cached via `sync.OnceValue`. Both tools check once at execution time — no startup cost if rg absent.

## Post-Completion

**Manual verification:**
- Test both tools in a repo with `.gitignore` entries — verify ignored files are excluded
- Test with `respect_gitignore: false` — verify ignored files are included
- Test without rg installed — verify fallback works identically to current behavior
- Test grep `include` param with patterns like `*.go`, `*.{ts,tsx}`
- Test find with `src/**/*.go` pattern
