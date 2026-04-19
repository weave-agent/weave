# Tools and Providers

## Overview
Implement basic tools (bash, read, edit, write, grep, find, ls) as individual Go modules under `extensions/tools/`, and three provider extensions (Anthropic, OpenAI, Z.ai) under `extensions/providers/`. OpenAI and Z.ai share an OpenAI-compatible base library. This gives Weave a working agent loop with real LLM backends and the tooling to act on the filesystem.

## Context
- **SDK registries**: `sdk.RegisterTool(name, factory)` and `sdk.RegisterProvider(name, factory)` are ready but empty
- **Agent-loop**: already calls `sdk.GetProvider()`, `sdk.ListTools()`, `sdk.GetTool()` at runtime
- **Extension pattern**: independent Go modules, self-register via `init()`, blank-imported by the built binary
- **Tool interface**: `Name() string`, `Definition() ToolDef`, `Execute(ctx, args) (ToolResult, error)`
- **Provider interface**: `Stream(ctx, ProviderRequest) (<-chan ProviderEvent, error)`
- **Pi reference**: 7 tools with pluggable operation interfaces, output truncation (2000 lines / 50KB), file mutation queues
- **Z.ai**: GLM-4 / ChatGLM platform, OpenAI-compatible API

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope
- Keep plan in sync with actual work done

## Implementation Steps

### Task 1: Shared truncation utility
Create a shared truncation package used by all tools for consistent output limiting (matching Pi's behavior: 2000 lines / 50KB, never partial lines).

- [ ] create `internal/truncate/truncate.go` with `Truncate(input string, maxLines int, maxBytes int) Result` and `Result` struct (content, truncated bool, lineCount, byteCount)
- [ ] set defaults: 2000 lines, 50KB
- [ ] write tests for Truncate (under limit, line limit, byte limit, empty input, single huge line, exact boundary)
- [ ] run `go test ./internal/truncate/...` — must pass before next task

### Task 2: bash tool
- [ ] create `extensions/tools/bash/go.mod` and `extensions/tools/bash/bash.go` with `init()` registering via `sdk.RegisterTool("bash", ...)`
- [ ] implement `Execute`: run command via `exec.CommandContext`, capture combined stdout+stderr, apply truncation, return `ToolResult`
- [ ] tool definition: `{ command: string, timeout?: number }` parameters
- [ ] write tests using table-driven cases (simple command, timeout, failure exit code, large output truncation, empty output)
- [ ] run `go test ./extensions/tools/bash/...` — must pass before next task

### Task 3: read tool
- [ ] create `extensions/tools/read/go.mod` and `extensions/tools/read/read.go` with `init()` registering via `sdk.RegisterTool("read", ...)`
- [ ] implement `Execute`: read file at path, apply offset/limit for line-based pagination, apply truncation, return content with line numbers
- [ ] tool definition: `{ path: string, offset?: number, limit?: number }` parameters
- [ ] write tests (read full file, with offset, with limit, nonexistent file, directory path, binary file)
- [ ] run `go test ./extensions/tools/read/...` — must pass before next task

### Task 4: edit tool
- [ ] create `extensions/tools/edit/go.mod` and `extensions/tools/edit/edit.go` with `init()` registering via `sdk.RegisterTool("edit", ...)`
- [ ] implement `Execute`: apply multiple `{ oldText, newText }` replacements to a file, return unified diff of changes
- [ ] tool definition: `{ path: string, edits: [{ oldText: string, newText: string }] }` parameters
- [ ] write tests (single edit, multiple edits, no match error, empty file, create new file)
- [ ] run `go test ./extensions/tools/edit/...` — must pass before next task

### Task 5: write tool
- [ ] create `extensions/tools/write/go.mod` and `extensions/tools/write/write.go` with `init()` registering via `sdk.RegisterTool("write", ...)`
- [ ] implement `Execute`: write content to file, create parent directories with `os.MkdirAll`
- [ ] tool definition: `{ path: string, content: string }` parameters
- [ ] write tests (write new file, overwrite existing, nested directory creation, permission error)
- [ ] run `go test ./extensions/tools/write/...` — must pass before next task

### Task 6: grep tool
- [ ] create `extensions/tools/grep/go.mod` and `extensions/tools/grep/grep.go` with `init()` registering via `sdk.RegisterTool("grep", ...)`
- [ ] implement `Execute`: search files using `regexp.Regexp`, support case-insensitive and literal modes, return matches with context lines, apply truncation
- [ ] tool definition: `{ pattern: string, path?: string, ignoreCase?: boolean, literal?: boolean, context?: number }` parameters
- [ ] write tests (simple match, no match, case-insensitive, literal mode, context lines, invalid regex)
- [ ] run `go test ./extensions/tools/grep/...` — must pass before next task

### Task 7: find tool
- [ ] create `extensions/tools/find/go.mod` and `extensions/tools/find/find.go` with `init()` registering via `sdk.RegisterTool("find", ...)`
- [ ] implement `Execute`: walk directory tree matching glob pattern via `filepath.Glob` / `filepath.Walk`, ignore common dirs (.git, node_modules), apply truncation
- [ ] tool definition: `{ pattern: string, path?: string }` parameters
- [ ] write tests (find by extension, find by name, nested match, no matches, nonexistent path)
- [ ] run `go test ./extensions/tools/find/...` — must pass before next task

### Task 8: ls tool
- [ ] create `extensions/tools/ls/go.mod` and `extensions/tools/ls/ls.go` with `init()` registering via `sdk.RegisterTool("ls", ...)`
- [ ] implement `Execute`: read directory entries with `os.ReadDir`, return names + type info, default to cwd
- [ ] tool definition: `{ path?: string }` parameters
- [ ] write tests (list cwd, list specific dir, nonexistent dir, empty dir, file path error)
- [ ] run `go test ./extensions/tools/ls/...` — must pass before next task

### Task 9: Anthropic provider
- [ ] create `extensions/providers/anthropic/go.mod` and `extensions/providers/anthropic/anthropic.go` with `init()` registering via `sdk.RegisterProvider("anthropic", ...)`
- [ ] implement `Stream`: call Anthropic Messages API with streaming, emit `ProviderEventTextDelta` and `ProviderEventToolCall` events, handle tool use blocks
- [ ] use official `github.com/anthropics/anthropic-sdk-go` SDK
- [ ] config: API key from `ANTHROPIC_API_KEY` env var, model from config
- [ ] write tests with mocked HTTP server (streaming response, tool calls, error handling)
- [ ] run `go test ./extensions/providers/anthropic/...` — must pass before next task

### Task 10: OpenAI-compat shared library
- [ ] create `extensions/providers/openai-compat/openai_compat.go` with shared types and streaming logic for OpenAI-compatible APIs
- [ ] parse SSE stream, emit `ProviderEventTextDelta` and `ProviderEventToolCall` events
- [ ] handle chat completion request/response format conversion to/from `sdk.ProviderRequest`/`ProviderEvent`
- [ ] write tests for SSE parsing, tool call extraction, error handling
- [ ] run `go test ./extensions/providers/openai-compat/...` — must pass before next task

### Task 11: OpenAI provider
- [ ] create `extensions/providers/openai/go.mod` and `extensions/providers/openai/openai.go` with `init()` registering via `sdk.RegisterProvider("openai", ...)`
- [ ] delegate to openai-compat shared library with `https://api.openai.com/v1` base URL
- [ ] config: API key from `OPENAI_API_KEY` env var, model from config
- [ ] write tests with mocked HTTP server
- [ ] run `go test ./extensions/providers/openai/...` — must pass before next task

### Task 12: Z.ai provider
- [ ] create `extensions/providers/zai/go.mod` and `extensions/providers/zai/zai.go` with `init()` registering via `sdk.RegisterProvider("zai", ...)`
- [ ] delegate to openai-compat shared library with Z.ai base URL
- [ ] config: API key from `ZAI_API_KEY` env var, model from config
- [ ] write tests with mocked HTTP server
- [ ] run `go test ./extensions/providers/zai/...` — must pass before next task

### Task 13: Integration verification
- [ ] verify all tool extensions register correctly via `sdk.ListTools()`
- [ ] verify all provider extensions register correctly via `sdk.ListProviders()`
- [ ] verify launcher discovery picks up nested extension directories (`extensions/tools/*`, `extensions/providers/*`)
- [ ] update launcher integration tests to include new extensions
- [ ] run `go test ./...` — full suite must pass
- [ ] run `make lint` — all issues must be fixed

### Task 14: Update documentation
- [ ] update CLAUDE.md if new patterns or conventions discovered
- [ ] update docs/design.md if architectural decisions diverged

## Technical Details

### Directory structure
```
extensions/
  tools/
    bash/
      go.mod          # module weave/ext/tools/bash
      bash.go
      bash_test.go
    read/
      go.mod          # module weave/ext/tools/read
      read.go
      read_test.go
    edit/
      go.mod          # module weave/ext/tools/edit
      edit.go
      edit_test.go
    write/
      go.mod          # module weave/ext/tools/write
      write.go
      write_test.go
    grep/
      go.mod          # module weave/ext/tools/grep
      grep.go
      grep_test.go
    find/
      go.mod          # module weave/ext/tools/find
      find.go
      find_test.go
    ls/
      go.mod          # module weave/ext/tools/ls
      ls.go
      ls_test.go
  providers/
    openai-compat/
      go.mod          # module weave/ext/providers/openai-compat
      openai_compat.go
      openai_compat_test.go
    anthropic/
      go.mod          # module weave/ext/providers/anthropic
      anthropic.go
      anthropic_test.go
    openai/
      go.mod          # module weave/ext/providers/openai
      openai.go
      openai_test.go
    zai/
      go.mod          # module weave/ext/providers/zai
      zai.go
      zai_test.go
  loop/               # existing
    ...
```

### Truncation defaults
- Max lines: 2000
- Max bytes: 50KB (51200 bytes)
- Never return partial lines
- Include metadata: truncated bool, line/byte counts

### Tool parameter schemas (JSON Schema format for ToolDef.Parameters)
All tools use `map[string]any` parameter schemas compatible with provider tool-use APIs.

### Provider streaming
- Anthropic: native SSE via official Go SDK
- OpenAI/Z.ai: SSE chat completions via shared openai-compat library
- Both emit `sdk.ProviderEventTextDelta` and `sdk.ProviderEventToolCall` events

### Config resolution
- Anthropic: `ANTHROPIC_API_KEY` env var, model from `.weave.yaml` or default
- OpenAI: `OPENAI_API_KEY` env var, model from config or default
- Z.ai: `ZAI_API_KEY` env var, model from config or default
- All use `gonfig` with `WithFile` + `WithEnvPrefix("WEAVE")` pattern

## Post-Completion
*Items requiring manual intervention or external systems — no checkboxes, informational only*

**Manual verification:**
- Test each provider with real API keys (anthropic, openai, z.ai)
- Test bash tool with real shell commands
- Test file tools against real filesystem

**External system updates:**
- API keys need to be set in environment for provider extensions to work
- Launcher hash computation will change — cached binaries will be rebuilt
- Launcher discovery may need update to handle nested extension directories (`tools/*`, `providers/*`)
