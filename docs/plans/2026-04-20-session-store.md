# Session Store (JSONL Extension)

## Overview

Add session persistence to weave as an independent extension module. The JSONL store extension subscribes to bus events and automatically appends conversation entries to JSONL files under `~/.weave/sessions/`. No SDK interface changes required — the extension communicates entirely through the event bus.

Inspired by pi-coding-agent's `SessionManager`: append-only JSONL with id/parentId tree structure, session header as first line, one JSON object per line.

Implements the design.md Store interface (Create, Load, Append, History, List, Compact) internally within the extension — minus Fork.

## Context

- **No session storage exists** — the agent loop (`extensions/loop/`) keeps messages in-memory only (`var messages []sdk.Message`), lost on exit
- **Design doc** (`docs/design.md`) describes a planned `Store` interface and `jsonlstore` extension, but neither exists
- **pi-coding-agent** uses append-only JSONL trees with session headers, stored under `~/.pi/agent/sessions/<encoded-cwd>/`
- Extension follows existing patterns: independent Go module, self-registers via `sdk.RegisterExtension`, loads config via gonfig

### Files involved

- `extensions/store/jsonl/go.mod` — new independent Go module
- `extensions/store/jsonl/store.go` — extension + JSONL store logic
- `extensions/store/jsonl/store_test.go` — tests

### Dependencies

- `weave/sdk` — for Extension, Bus, Event, Message types
- `github.com/nniel-ape/gonfig` — config loading (via sdk pattern)
- Standard library only otherwise (`encoding/json`, `os`, `path/filepath`, `time`, `crypto/rand`)

## Development Approach

- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**
- Run tests after each change

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## Technical Details

### Event subscriptions

| Event | Action |
|-------|--------|
| `agent.prompt` | Create new session, write header + first user entry |
| `agent.turn_start` | Track turn counter |
| `agent.message_end` | Append assistant message entry |
| `agent.tool_result` | Append tool result entry |
| `agent.end` | Flush and close session file |

### JSONL file format

File: `~/.weave/sessions/{session-id}.jsonl`

```
{"type":"session","id":"a1b2c3","timestamp":"2026-04-20T10:00:00Z","cwd":"/path/to/project"}
{"id":"f4e5d6","type":"message","turn":1,"data":{"role":"user","content":"hello"},"created":"2026-04-20T10:00:01Z"}
{"id":"g7h8i9","parent_id":"f4e5d6","type":"message","turn":2,"data":{"role":"assistant","content":"hi!"},"created":"2026-04-20T10:00:05Z"}
```

- Line 1: session header (`type: "session"`)
- Subsequent lines: entries with `id`/`parent_id` forming a chain
- `data` carries the serialized event payload

### Data structures

```go
type Session struct {
    Header  SessionHeader
    Entries []Entry
}

type SessionHeader struct {
    Type      string    `json:"type"`     // "session"
    ID        string    `json:"id"`
    Timestamp time.Time `json:"timestamp"`
    CWD       string    `json:"cwd"`
}

type Entry struct {
    ID       string          `json:"id"`
    ParentID string          `json:"parent_id,omitempty"`
    Type     string          `json:"type"`     // "message", "summary"
    Turn     int             `json:"turn"`
    Data     json.RawMessage `json:"data"`
    Meta     map[string]any  `json:"meta,omitempty"`
    Created  time.Time       `json:"created"`
}

type SessionInfo struct {
    ID         string    `json:"id"`
    CWD        string    `json:"cwd"`
    EntryCount int       `json:"entry_count"`
    CreatedAt  time.Time `json:"created_at"`
    UpdatedAt  time.Time `json:"updated_at"`
}
```

### Config

```yaml
# .weave.yaml
extensions:
  - jsonl

jsonl:
  dir: ""                  # default: ~/.weave/sessions
  auto_compact: false
  compact_threshold: 100
```

### Store methods (internal)

- **Create(cwd string) \*Session** — generate UUID session ID, write header line to new JSONL file
- **Load(sessionID string) (\*Session, error)** — read JSONL file, parse header + entries
- **Append(sessionID string, entry Entry) error** — append one JSON line (O_APPEND, atomic-ish)
- **History(sessionID string) ([]Entry, error)** — read all entries, return root→leaf chain
- **List() ([]SessionInfo, error)** — scan `~/.weave/sessions/*.jsonl`, read headers + stat files
- **Compact(sessionID string, keepLast int) error** — read entries, summarize first N into `"summary"` entry, rewrite file

### ID generation

Use `crypto/rand` to generate 8-char hex IDs (no external dependency). Check for collisions against existing entry IDs.

## Implementation Steps

### Task 1: Create extension module scaffold
- [x] create `extensions/store/jsonl/` directory
- [x] create `go.mod` with module path `weave/extensions/store/jsonl`, require `weave/sdk`
- [x] create `store.go` with package declaration, `init()` registering `"jsonl-store"` extension via `sdk.RegisterExtension`
- [x] create `SessionHeader`, `Entry`, `Session`, `SessionInfo` data structures
- [x] create `Config` struct with gonfig tags (`dir`, `auto_compact`, `compact_threshold`)
- [x] verify module compiles with `go build`

### Task 2: Implement core store operations
- [x] implement `generateID()` — 8-char hex via `crypto/rand`
- [x] implement `sessionDir()` — resolve session directory (config or `~/.weave/sessions`)
- [x] implement `Create(cwd string) (*Session, error)` — generate session ID, write header line
- [x] implement `Append(sessionID string, entry Entry) error` — set ID/parentID/created, append JSON line
- [x] implement `loadFromFile(path string) (*Session, error)` — parse JSONL file into Session
- [x] implement `Load(sessionID string) (*Session, error)` — resolve path, delegate to loadFromFile
- [x] implement `History(sessionID string) ([]Entry, error)` — load session, return entries
- [x] implement `List() ([]SessionInfo, error)` — scan dir, read headers, stat files
- [x] write tests for Create (creates file, writes header)
- [x] write tests for Append (appends entry, sets ID/timestamp, chains parentId)
- [x] write tests for Load (roundtrip: Create + Append → Load)
- [x] write tests for History (returns entries in order)
- [x] write tests for List (multiple sessions, file info)
- [x] run tests — must pass before next task

### Task 3: Implement Compact
- [x] implement `Compact(sessionID string, keepLast int) error` — read all entries, keep last N, prepend summary entry, rewrite file
- [x] write tests for Compact (truncation, summary entry, keepLast > total entries)
- [x] run tests — must pass before next task

### Task 4: Wire extension to bus events
- [x] implement `Subscribe(bus Bus)` — subscribe to `agent.prompt`, `agent.message_end`, `agent.tool_result`, `agent.turn_start`, `agent.end`
- [x] implement event handler goroutine: on `agent.prompt` → Create session + Append user entry, on `agent.message_end` → Append assistant entry, on `agent.tool_result` → Append tool result entry
- [x] track current session state (session ID, last entry ID, turn counter) in the extension struct
- [x] implement `Close()` — close open file handle, clean up goroutine
- [x] write tests for Subscribe (publish events, verify JSONL file contents)
- [x] run tests — must pass before next task

### Task 5: Integration with launcher
- [x] verify extension is discovered by launcher's nested lookup (`extensions/store/jsonl/` → name `jsonl`)
- [x] add `jsonl-store` to default config or example `.weave.yaml`
- [x] run full test suite (`go test ./...`)
- [x] run linter (`make lint`)

### Task 6: Update documentation
- [ ] update `docs/design.md` Implementation Divergences section noting store is bus-only, no SDK interface
- [ ] update `CLAUDE.md` if architecture section needs store extension mention

## Post-Completion

**Future work** (no checkboxes):
- Session resume: store subscribes to `session.resume` event, reads JSONL, publishes history back to bus
- Fork support: copy entries from parent session into new session file
- Auto-compact on `agent.end` if entry count exceeds threshold
