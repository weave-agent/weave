# Tool Improvements

## Overview
Upgrade weave's 5 core tools (bash, read, edit, write, ls) with features identified by comparing against pi-coding-agent and crush implementations. 12 improvements across all tools — background jobs, streaming output, temp file overflow, read-before-edit tracking, macOS path normalization, replace-all mode, file mutation queue, line ending preservation, hierarchical tree output, entry limits, ignore patterns, sorted output, and no-op write detection.

## Context
- **Affected files**: `extensions/tools/{bash,read,edit,write,ls}/*.go`, `internal/truncate/truncate.go`, `sdk/event.go`
- **Registration pattern**: All tools use `sdk.RegisterTool(name, func(cfg sdk.Config) (sdk.Tool, error))` in `init()`
- **Tool interface**: `Name()`, `Definition()`, `Execute(ctx, args map[string]any) (ToolResult, error)` — results are string content
- **Bus events**: `bus.Publish(sdk.Event{Topic, Payload, Timestamp})` — tools can publish events for streaming
- **Truncation**: Shared `internal/truncate/truncate.go` — `DefaultMaxLines=2000`, `DefaultMaxBytes=50KB`
- **Config**: `cfg.ToolConfig("name", &target)` with JSON struct tags + `default` tags

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Each task includes tests as separate checklist items
- All tests must pass before moving to next task
- Run `cd extensions/tools/<tool> && go test ./...` for per-tool tests

## Testing Strategy
- Unit tests per tool module — test the `Execute` method directly with various inputs
- Use `testdata/` directories for file-based tests where needed
- Test both success and error paths

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## Implementation Steps

### Task 1: Bash — Streaming output
Add real-time output streaming via bus events so the agent loop can show progress during long-running commands.

- [x] Add bus field to bash tool struct — pass `sdk.Bus` through a new parameter or via context value
- [x] Refactor `Execute` to stream stdout/stderr line-by-line via `bus.Publish` with topic `tool.bash.output` (payload: `{Command, Line, Stream: "stdout"|"stderr"}`)
- [x] Buffer output for final `ToolResult` while streaming — keep current truncation behavior for the summary
- [x] Write tests for streaming: verify events published with correct topics and payloads
- [x] Write tests for streaming + timeout: verify partial output returned on timeout
- [x] Run `cd extensions/tools/bash && go test ./...`

### Task 2: Bash — Background jobs
Add `run_in_background` and `auto_background_after` params. Background jobs run detached, output retrieved separately.

- [x] Add `BackgroundManager` type in `extensions/tools/bash/` — manages `map[string]*BackgroundJob` with mutex
  - `BackgroundJob` struct: `ID, Command, StartTime, ctx+cancel, stdout/stderr syncBuffer, done chan, exitErr`
  - Methods: `Start(), Output(id), Kill(id), List()`
- [x] Add `run_in_background` (bool) and `auto_background_after` (int, seconds, default 0=disabled) params to bash tool definition
- [x] Implement background execution path: start command, return job ID immediately with instructions
- [x] Implement auto-background: start sync, move to background after timeout or completion
- [x] Add `tool.bash.background_start` and `tool.bash.background_done` bus events
- [x] Write tests for immediate background (`run_in_background=true`)
- [x] Write tests for auto-background (`auto_background_after=2`)
- [x] Write tests for background job output retrieval and kill
- [x] Run `cd extensions/tools/bash && go test ./...`

### Task 3: Bash — Temp file overflow
When output exceeds truncation limits, save full output to a temp file and include the path in the result.

- [x] In bash `Execute`, after truncation, if `Result.Truncated` — write full output to `os.CreateTemp("", "weave-bash-*.log")`
- [x] Append temp file path to `ToolResult.Content`: `\n\nFull output saved to: /tmp/weave-bash-xxx.log`
- [x] Write tests verifying temp file created with correct content when output exceeds limits
- [x] Write tests verifying no temp file when output is within limits
- [x] Run `cd extensions/tools/bash && go test ./...`

### Task 4: Read — macOS path normalization
Handle macOS-specific path quirks: NFD Unicode normalization, curly quotes, and Unicode spaces.

- [x] Create `internal/pathutil/normalize.go` with `NormalizePath(path string) string` function
  - Replace curly quotes (`“`, `”`, `‘`, `’`) with straight quotes
  - Replace Unicode spaces (` `, ` `) with regular space
  - Apply NFD normalization using `golang.org/x/text/unicode/nf` (check if already a dep)
- [x] Integrate `NormalizePath` into read tool — apply before file access, try normalized path if original not found
- [x] Write tests for each normalization case
- [x] Write test for path that needs no normalization (passthrough)
- [x] Run `cd extensions/tools/read && go test ./...`

### Task 5: Read — Read-before-edit tracking (bus events)
Track which files have been read so the edit tool can enforce read-before-edit.

- [x] Add `tool.read.done` bus event in read tool — payload: `{Path, ModTime}` published after successful read
- [x] Create `internal/filetracker/tracker.go` with in-memory `FileTracker` type
  - `RecordRead(path string, modTime time.Time)`
  - `WasRead(path string) bool`
  - `GetReadTime(path string) (time.Time, bool)`
  - Thread-safe via `sync.RWMutex`
- [x] FileTracker subscribes to `tool.read.done` events and records path + mod time
- [x] Expose FileTracker through SDK or pass to edit tool via config/context
- [x] Write tests for FileTracker: record, query, concurrent access
- [x] Write tests for read tool: verify event published on successful read
- [x] Run `cd extensions/tools/read && go test ./...`

### Task 6: Edit — Read-before-edit enforcement
Edit tool checks FileTracker before applying edits. Rejects if file not read or modified since read.

- [x] Integrate FileTracker into edit tool struct
- [x] Add pre-edit check: if `!tracker.WasRead(path)` → return error `"file must be read before editing"`
- [x] Add staleness check: if file mod time > recorded read time → return error with details
- [x] Write tests: edit without prior read fails, edit after read succeeds, edit after external modification fails
- [x] Run `cd extensions/tools/edit && go test ./...`

### Task 7: Edit — Replace all mode
Add `replace_all` flag to replace every occurrence of oldText.

- [x] Add `replace_all` (bool) to edit tool params schema
- [x] When `replace_all=true`: use `strings.ReplaceAll(content, oldText, newText)` instead of single-match logic
- [x] When `replace_all=false` (default): keep existing exact-single-match validation
- [x] Update tool definition description to mention replace_all
- [x] Write tests: replace_all with multiple occurrences, replace_all=false with multiple occurrences (error), replace_all with no matches (error)
- [x] Run `cd extensions/tools/edit && go test ./...`

### Task 8: Edit — File mutation queue
Serialize concurrent edits to the same file to prevent race conditions.

- [x] Create `internal/filemut/mutex.go` with `FileMutex` type
  - `Lock(path string) func()` — returns unlock function, uses `sync.Map` of `*sync.Mutex` per path
  - Per-path mutexes created lazily, never cleaned (bounded by number of unique files in session)
- [x] Integrate FileMutex into edit tool: `defer fm.Lock(path)()` at start of Execute
- [x] Also integrate into write tool for consistency
- [x] Write tests: concurrent edits to same file are serialized, edits to different files run in parallel
- [x] Run `cd extensions/tools/edit && go test ./...`

### Task 9: Edit — Line ending preservation
Detect and preserve original line endings (CRLF/LF) across edits.

- [x] Create `internal/fileutil/endings.go` with helpers:
  - `DetectLineEndings(content []byte) string` — returns "\r\n" or "\n"
  - `NormalizeToLF(content []byte) ([]byte, string)` — returns LF content + original ending
  - `RestoreLineEndings(content []byte, ending string) []byte`
- [x] Integrate into edit tool: detect endings before edit, normalize to LF, apply edits, restore original endings
- [x] Write tests: CRLF preserved, LF preserved, mixed endings normalized to first-detected
- [x] Run `cd extensions/tools/edit && go test ./...`

### Task 10: LS — Sorted output + entry limit + ignore patterns
Add alphabetical sorting, configurable entry limit, and ignore glob patterns.

- [x] Sort `os.ReadDir` results alphabetically (case-insensitive via `strings.ToLower`)
- [x] Add `limit` param (default 500) to tool definition — truncate after N entries with notice
- [x] Add `ignore` param (`[]string` glob patterns) — filter entries matching any pattern using `filepath.Match`
- [x] Update tool definition with new params
- [x] Write tests: sorted output verified, limit truncation, ignore patterns filter correctly, combined limit+ignore
- [x] Run `cd extensions/tools/ls && go test ./...`

### Task 11: LS — Hierarchical tree output
Add `depth` param and render output as a tree structure instead of flat list.

- [x] Add `depth` param (int, default 0=unlimited) to tool definition
- [x] When `depth > 0` or path is single directory: use recursive `filepath.WalkDir` with depth limit
- [x] Build tree structure: sort entries, indent with `├──`/`└──`/`│   ` tree characters
- [x] Keep flat list mode for backward compat when depth=0 and no subdirectory traversal needed
- [x] Write tests: tree output format, depth limiting, deeply nested structure
- [x] Run `cd extensions/tools/ls && go test ./...`

### Task 12: Write — No-op detection
Skip writing if content is identical to existing file content.

- [ ] At start of write Execute: if file exists, read current content and compare to new content
- [ ] If identical: return `ToolResult{Content: "file already contains the exact content, no changes made"}` without writing
- [ ] Write tests: identical content returns no-op message, different content writes normally, new file writes normally
- [ ] Run `cd extensions/tools/write && go test ./...`

### Task 13: Final verification
- [ ] Run `make lint` — all issues fixed
- [ ] Run `make test` — all tests pass
- [ ] Verify all 12 features work end-to-end in an interactive session
- [ ] Run `make fix` if any formatting issues

## Technical Details

### Bash streaming event format
```go
sdk.Event{
    Topic: "tool.bash.output",
    Payload: BashOutputPayload{
        Command: command,
        Line:    line,
        Stream:  "stdout", // or "stderr"
    },
}
```

### Bash background job result format
```
Background job started: <id>
Command: <command>
Use job output tools or wait for completion event.
```

### Read tracking event format
```go
sdk.Event{
    Topic: "tool.read.done",
    Payload: ReadDonePayload{
        Path:    path,
        ModTime: fileInfo.ModTime(),
    },
}
```

### FileMutex API
```go
fm := filemut.New()
unlock := fm.Lock("/path/to/file.go")
defer unlock()
```

### LS tree output format
```
dir/
├── file1.go
├── file2.go
├── subdir/
│   ├── nested.go
│   └── deep/
│       └── file.go
└── another.go
```

## Post-Completion
*Items requiring manual intervention or external systems — no checkboxes, informational only*

**Manual verification:**
- Test streaming output in TUI — verify progressive display works
- Test background jobs in TUI — verify job status and output retrieval
- Test macOS path normalization with real macOS paths (screenshots, Unicode filenames)
- Test tree output readability in terminal

**External considerations:**
- `golang.org/x/text` dependency for NFD normalization — check if already in go.mod, add if not
- TUI extension may need updates to render streaming bash output and background job status
- New bus event topics should be documented in CLAUDE.md architecture section
