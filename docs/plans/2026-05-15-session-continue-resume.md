# Session Continue & Resume Support

## Overview

Add `--continue` / `-c` and `--resume` / `-r` CLI flags to weave that restore a previous session's message history into the agent loop, allowing conversations to be continued across process restarts. Supports both TUI (interactive) and headless (`-p`) modes.

**Problem**: Sessions are fully persisted to JSONL by the store extension, but the agent loop starts with an empty `messages` slice every time. The TUI has a `/resume` command that rebuilds the chat display visually, but the agent has no memory of restored messages.

**Key benefit**: Users can pick up where they left off — in the TUI they see their history and continue chatting; in headless mode they can chain `--continue` with `-p` to send follow-up prompts with full context.

## Context (from brainstorm)

- JSONL store (`extensions/store/jsonl/store.go`) already persists all messages with `Create`, `Load`, `History`, `List`, `Append`, `Compact`
- Store types (`Session`, `SessionHeader`, `SessionInfo`, `Entry`) are private to the store module
- SDK has no session types or interfaces — needs a `SessionStore` interface following the `FileTracker`/`FileMuter` injection pattern
- TUI already publishes `session.resume` event (defined in `bridge.go`) but agent loop ignores it
- TUI already has `rebuildChatFromSession()` for visual restoration
- Agent loop manages `messages []sdk.Message` (in-memory, reset on each `agent.prompt`)
- Wire phase (`internal/wire/`) resolves config and publishes startup events

**Approach chosen**: SDK interface injection (not bus round-trip). A `SessionStore` interface in `sdk/` with `ListSessions()` and `LoadHistory()` methods. The JSONL store implements it. Wire injects it via global getter/setter. This follows the existing `FileTracker`/`FileMuter` pattern exactly.

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**
- Run tests after each change

## Testing Strategy
- **Unit tests**: required for every task
- Test store's `ListSessions`/`LoadHistory` conversion logic
- Test agent loop's session resume handler
- Test wire's session resolution logic
- Test CLI flag parsing
- Cover both success and error scenarios (no session found, corrupt JSONL, invalid ID)

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## What Goes Where
- **Implementation Steps** (`[ ]` checkboxes): code changes, tests
- **Post-Completion** (no checkboxes): manual testing scenarios, external verification

## Implementation Steps

### Task 1: Add SessionStore interface to SDK
- [x] create `sdk/session.go` with `SessionStore` interface, `SessionInfo` struct, and global getter/setter (`SetSessionStore`/`GetSessionStore`) following the `FileTracker`/`FileMuter` pattern in `sdk/extension.go`
- [x] define `ListSessions() ([]SessionInfo, error)` and `LoadHistory(sessionID string) ([]Message, error)` methods on the interface
- [x] add `NoopSessionStore` zero-value stub (returns empty slice, nil error) for nil-safety
- [x] write tests for `SetSessionStore`/`GetSessionStore` and `NoopSessionStore` in `sdk/session_test.go`
- [x] run `go test ./sdk/...` — must pass before next task

### Task 2: Implement SessionStore on JSONL store
- [x] add `ListSessions() ([]sdk.SessionInfo, error)` method to `Store` in `extensions/store/jsonl/store.go` — wraps existing `List()` and converts `SessionInfo` → `sdk.SessionInfo`
- [x] add `LoadHistory(sessionID string) ([]sdk.Message, error)` method — wraps existing `History()`, unmarshals each `Entry.Data` (`json.RawMessage`) into `sdk.Message` structs (user, assistant, tool_result roles), skips entries that fail to unmarshal with a warning log
- [x] add message conversion helper: unmarshal `Entry.Data` → intermediate struct `{Role, Content, ToolCalls, ToolCallID, ToolName, IsError}` → `sdk.Message`
- [x] write tests for `ListSessions` (conversion, empty dir, corrupted header) in `extensions/store/jsonl/store_test.go`
- [x] write tests for `LoadHistory` (full history, partial corruption, empty session, tool results) in `extensions/store/jsonl/store_test.go`
- [x] run `cd extensions/store/jsonl && go test ./...` — must pass before next task

### Task 3: Add --continue and --resume CLI flags
- [x] add `Continue bool`, `Resume string` fields to `flagSet` in `settings/config.go` with appropriate tags (`flag:"continue" short:"c"`, `flag:"resume" short:"r"`)
- [x] add validation: `--continue` and `--resume` are mutually exclusive (return error if both set)
- [x] store resolved values on `Settings` struct (add `Continue bool` and `Resume string` fields)
- [x] write tests for flag parsing in `settings/config_test.go` — both flags, mutual exclusion, short flags
- [x] run `go test ./settings/...` — must pass before next task

### Task 4: Wire session resolution and store injection
- [ ] in `internal/wire/wire.go`, after creating the store extension, call `sdk.SetSessionStore(store)` (cast store to `sdk.SessionStore` interface)
- [ ] in `internal/wire/run.go`, add `resolveSession(cfg)` function that checks `--continue`/`--resume` flags, calls `sdk.GetSessionStore()`, resolves session ID, loads messages
- [ ] for `--continue`: call `ListSessions()`, pick most recent (sort by `UpdatedAt` desc, or most recent by CWD match if feasible)
- [ ] for `--resume <id>`: call `LoadHistory(id)` directly
- [ ] publish `session.resume` event with `SessionResumePayload{SessionID, Messages}` on the bus before `app.started`
- [ ] when `-p` is used with `--continue`/`--resume`: publish `agent.followup` instead of `agent.prompt` so messages are appended to the restored history
- [ ] add `SessionResumePayload` struct to `sdk/session.go` with `SessionID string` and `Messages []Message`
- [ ] write tests for `resolveSession` — no session found, invalid ID, successful continue, successful resume
- [ ] run `go test ./internal/wire/...` — must pass before next task

### Task 5: Agent loop session resume handler
- [ ] in `extensions/agent/loop.go`, add `resumed bool` and `sessionID string` fields to the agent struct
- [ ] subscribe to `session.resume` event in `Subscribe(bus)`: set `messages = payload.Messages`, `sessionID = payload.SessionID`, `resumed = true`
- [ ] ensure `agent.prompt` handler checks `resumed` flag — if true, treat the prompt as a follow-up (append to messages) rather than resetting; clear `resumed`
- [ ] write tests for session resume handler in `extensions/agent/loop_test.go` — messages restored, resumed flag set, subsequent prompt appends correctly
- [ ] run `cd extensions/agent && go test ./...` — must pass before next task

### Task 6: JSONL store session continuity on resume
- [ ] in `extensions/store/jsonl/store.go`, add handler for `session.resume` event: set internal `sessionID` to the resumed session ID so subsequent events (`agent.followup`, `agent.message_end`, `agent.tool_result`) append to the existing file instead of creating a new session
- [ ] write tests verifying that after `session.resume`, subsequent events are stored in the resumed session file
- [ ] run `cd extensions/store/jsonl && go test ./...` — must pass before next task

### Task 7: TUI integration for --continue/--resume
- [ ] in `extensions/ui/tui/bridge.go`, ensure `session.resume` event from wire triggers `rebuildChatFromSession()` (may already work via existing bus listener — verify)
- [ ] add `/resume` slash command to open interactive session picker (may already exist — verify and extend if needed)
- [ ] ensure TUI shows restored history on startup when `--continue` is used
- [ ] write tests for TUI session resume flow if testable via existing patterns in `extensions/ui/tui/`
- [ ] run `cd extensions/ui/tui && go test ./...` — must pass before next task

### Task 8: Update CLAUDE.md documentation
- [ ] add `--continue` / `-c` and `--resume` / `-r` flags to CLI documentation section
- [ ] document session-related env vars and bus events (`session.resume`, `SessionResumePayload`)
- [ ] document `SessionStore` interface in SDK section

## Technical Details

### New SDK types (`sdk/session.go`)
```go
type SessionStore interface {
    ListSessions() ([]SessionInfo, error)
    LoadHistory(sessionID string) ([]Message, error)
}

type SessionInfo struct {
    ID        string
    CWD       string
    CreatedAt time.Time
    UpdatedAt time.Time
}

type SessionResumePayload struct {
    SessionID string
    Messages  []Message
}
```

### CLI flags
```
--continue, -c    Resume most recent session
--resume, -r      Resume specific session (takes ID arg)
```

### Event flow
```
wire.Run()
  ├─ resolve --continue/--resume flags
  ├─ sdk.SetSessionStore(jsonlStore)
  ├─ resolveSession() → ListSessions/LoadHistory
  ├─ publish session.resume {SessionID, Messages}
  ├─ publish app.started
  └─ WireExtensions()
       ├─ agent.loop.Subscribe → receives session.resume, populates messages
       ├─ jsonl.Store.Subscribe → receives session.resume, sets sessionID
       └─ tui.Subscribe → receives session.resume, rebuilds chat display
```

### Headless mode with -p + --continue
```
wire publishes: session.resume → agent.followup (not agent.prompt)
agent loop: messages restored from session, then new prompt appended
```

### Error handling
- No session found → stderr message, exit 1 (headless) / notification + fresh start (TUI)
- Invalid session ID → same behavior
- Corrupt JSONL entries → skip with warning log, return partial history
- Orphaned tool_result entries → skip during LoadHistory

## Post-Completion

**Manual verification**:
- Start a TUI session, chat a few turns, exit. Run `weave --continue` and verify history appears, follow-up works with full context
- Run `weave -p "remember what we discussed?" --continue` in headless mode and verify the model has prior context
- Run `weave --resume <id>` with a valid and invalid session ID
- Test `/resume` slash command in TUI — interactive session picker
- Verify two concurrent `--continue` sessions don't crash (interleaved appends to same JSONL)

**External verification**:
- Update provider model if session resume affects token counting/display
- Verify JSONL files remain well-formed after resume + new turns
