# TUI Rich Features — Tool Renderers, Completions, Attachments

## Overview

Add rich feature improvements to the TUI after the v2 + ultraviolet migration is complete. These features build on the stable v2 foundation and screen buffer rendering.

**Features:**
- Structured tool result types (`sdk/toolresult/`) with `RenderOutput` renderer interface
- Per-tool specialized TUI rendering with typed section dispatch
- Upgraded unified diff rendering with line numbers and syntax highlighting
- @-mention file completions in the editor
- Custom Chroma formatter for consistent code block highlighting
- File attachments with auto-detection of long pastes
- Pills bar for tool progress indicators

**Key design principle:** Tools publish structured data through `sdk/toolresult/`. Tools register `sdk.ToolRenderer` implementations that return `RenderOutput` — a UI-agnostic structured format. Each UI (TUI, headless, future web) interprets `RenderOutput` using its own framework. Tools never import UI packages.

## Context

**Prerequisites (must be complete before starting):**
- TUI v2 migration with ultraviolet screen buffers
- Overlay stack
- Landing state
- Progressive markdown, token rate, auto-scroll

**Files/components involved:**
- `sdk/toolresult/` — new shared sub-package for structured tool result types
- `sdk/toolrender.go` — new `ToolRenderer` interface, `RenderOutput` types, global registry
- `sdk/` — update `ToolResult` carrier to hold typed result data
- `extensions/tools/{bash,read,edit,write,grep,find,ls}/` — update to publish structured results + register renderers
- `extensions/ui/tui/components/messages/tools/` — new TUI rendering dispatch + per-tool renderers
- `extensions/ui/tui/components/messages/diff.go` — upgraded diff rendering
- `extensions/ui/tui/components/completions/` — new @-mention completion component
- `extensions/ui/tui/xchroma/` — new custom Chroma formatter
- `extensions/ui/tui/components/attachments/` — new file attachment component

## Development Approach

- **Testing approach:** Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- Run tests after each change

## Testing Strategy

- **Unit tests:** required for every task
- Use `github.com/stretchr/testify` (require for fatal, assert for non-fatal)
- Use moq-generated mocks — run `make gen` after changing interfaces
- Test tool result types: struct fields, JSON round-trip
- Test ToolRenderer registry: register, get, fallback
- Test RenderOutput interpretation per section type
- Test per-tool TUI renderer output in screen buffer
- Test completion source filtering, fuzzy matching, gitignore respect
- Test file attachment paste detection thresholds

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## Implementation Steps

---

### Task 1: Create sdk/toolresult/ package with structured result types

- [ ] Create `sdk/toolresult/` sub-package — shared contract between tool extensions and UI extensions. Both import SDK, so both can reference these types without creating directional dependencies.
- [ ] Define result types for each built-in tool:
  - `bash.go` — `BashResult{Command string, ExitCode int, Stdout string, Stderr string}`
  - `edit.go` — `EditResult{FilePath string, Diff string, Language string}`
  - `read.go` — `ReadResult{FilePath string, Content string, Language string, StartLine int, EndLine int}`
  - `write.go` — `WriteResult{FilePath string, Content string, Language string, Created bool}`
  - `grep.go` — `GrepResult{Pattern string, Matches []Match}` where `Match{File string, Line int, Text string}`
  - `find.go` — `FindResult{Entries []Entry}` where `Entry{Path string, IsDir bool}`
  - `ls.go` — `LsResult` type alias for `FindResult`
  - `generic.go` — `GenericResult{Text string}` — fallback for third-party tools
- [ ] Update `sdk/` — update `ToolResult` carrier: add `Data interface{}` field that carries typed result structs from `toolresult`.
- [ ] Update each tool extension to publish structured results — bash publishes `toolresult.BashResult`, edit publishes `toolresult.EditResult`, etc. Replace current plain-text result publishing.
- [ ] Write tests for each result type — verify struct fields, verify JSON round-trip for future web UI compatibility.
- [ ] Run tests — must pass before Task 2.

---

### Task 2: Create sdk/toolrender.go with RenderOutput and registry

- [ ] Create `sdk/toolrender.go` — defines `RenderOutput`, typed `Section` structs, `ToolRenderer` interface, and global registry.
- [ ] Define `SectionType` enum: `SectionText`, `SectionCode`, `SectionDiff`, `SectionTable`, `SectionTree`, `SectionError`.
- [ ] Define `Section` struct with typed metadata per section type:
  ```go
  type Section struct {
      Type     SectionType
      Content  string
      Language string       // for code/diff — file extension
      Code     CodeMeta     // SectionCode: ExitCode, Truncated, LineCount, StartLine
      Diff     DiffMeta     // SectionDiff: Adds, Deletes
      Table    TableMeta    // SectionTable: Headers, Widths
      Tree     TreeMeta     // SectionTree: DirCount, FileCount
      Error    ErrorMeta    // SectionError: ExitCode
  }
  ```
- [ ] Define `RenderOutput` struct: `Title string`, `Sections []Section`.
- [ ] Define `ToolRenderer` interface: `Render(data interface{}) RenderOutput`.
- [ ] Implement global registry: `RegisterToolRenderer(toolName, renderer)`, `GetToolRenderer(toolName) (ToolRenderer, bool)`.
- [ ] Write tests for registry — register, get, concurrent access, missing key returns false.
- [ ] Write tests for `RenderOutput` — verify section type discrimination, verify typed meta fields are populated correctly.
- [ ] Run tests — must pass before Task 3.

---

### Task 3: Register ToolRenderers in each tool extension

- [ ] Update `extensions/tools/bash/tool.go` — register `BashRenderer` in `init()` that converts `toolresult.BashResult` → `RenderOutput` with `SectionCode` + `CodeMeta{ExitCode, LineCount, Truncated}`.
- [ ] Update `extensions/tools/edit/tool.go` — register `EditRenderer` that converts `toolresult.EditResult` → `RenderOutput` with `SectionDiff` + `DiffMeta{Adds, Deletes}`.
- [ ] Update `extensions/tools/read/tool.go` — register `ReadRenderer` that converts `toolresult.ReadResult` → `RenderOutput` with `SectionCode` + `CodeMeta{StartLine, LineCount}`.
- [ ] Update `extensions/tools/write/tool.go` — register `WriteRenderer` that converts `toolresult.WriteResult` → `RenderOutput` with `SectionCode` + `CodeMeta`.
- [ ] Update `extensions/tools/grep/tool.go` — register `GrepRenderer` that converts `toolresult.GrepResult` → `RenderOutput` with `SectionTable` + `TableMeta{Headers}`.
- [ ] Update `extensions/tools/find/tool.go` — register `FindRenderer` that converts `toolresult.FindResult` → `RenderOutput` with `SectionTree` + `TreeMeta{DirCount, FileCount}`.
- [ ] Update `extensions/tools/ls/tool.go` — register `LsRenderer` (similar to find).
- [ ] Write tests for each renderer — verify correct `RenderOutput` produced from each typed result, verify section types and meta fields.
- [ ] Run tests — must pass before Task 4.

---

### Task 4: Create TUI tool rendering dispatch

- [ ] Create `components/messages/tools/dispatch.go` — `DrawToolResult()` function that:
  1. Calls `sdk.GetToolRenderer(result.Tool)` to look up the renderer
  2. Calls `renderer.Render(result.Data)` to get `RenderOutput`
  3. Passes `RenderOutput` to `drawRenderOutput(scr, area, output)`
  4. Falls back to generic panel if no renderer found
- [ ] Implement `drawRenderOutput()` — interprets each `SectionType` with full TUI screen buffer capabilities:
  - `SectionCode` → syntax-highlighted block with line numbers, borders, Chroma, collapse when `CodeMeta.Truncated`
  - `SectionDiff` → colored unified diff with line numbers, per-line highlighting, stats badge from `DiffMeta`
  - `SectionTable` → aligned columns with headers from `TableMeta`
  - `SectionTree` → indented listing with dir/file icons, counts from `TreeMeta`
  - `SectionError` → red-bordered error panel with exit code from `ErrorMeta`
  - `SectionText` → styled text paragraph
- [ ] Title rendered as bold header bar across the top of the tool panel.
- [ ] Update `components/messages/tool.go` — use `DrawToolResult()` instead of current generic rendering. Remove old `sdk.ToolRenderer` registration pattern.
- [ ] Write tests for dispatch — verify registry lookup, verify `RenderOutput` interpretation per section type, verify fallback to generic for unknown tools.
- [ ] Write tests for `drawRenderOutput` — verify each section type produces correct screen buffer output.
- [ ] Run tests — must pass before Task 5.

---

### Task 5: Upgraded unified diff rendering

- [ ] Upgrade `components/messages/diff.go` — add line numbers, Chroma syntax-highlighted lines (per-line, detect language from file extension in `---`/`+++` headers), configurable context lines.
- [ ] Add Chroma lexer caching — cache lexers by file extension to avoid re-detection on each render.
- [ ] Keep the color scheme: green added, red removed, cyan headers, magenta hunk markers, dim context.
- [ ] Integrate with the custom Chroma formatter from Task 7 for consistent styling.
- [ ] Write tests for upgraded diff — verify line numbers present, verify syntax highlighting applied, verify color coding, verify hunk detection.
- [ ] Run tests — must pass before Task 6.

---

### Task 6: @-mention file completions

- [ ] Create `components/completions/completions.go` — `CompletionsModel` with:
  - `visible`, `query`, `items []CompletionItem`, `selected int`
  - `Draw(scr, area)` renders popup below cursor position
  - `Update(msg)` handles typing filter, up/down navigation, tab/enter accept, escape dismiss
- [ ] Create `CompletionSource` interface — `Query(prefix string) []CompletionItem`. Implement `FileSource` that walks the project directory (respect `.gitignore`), caches results, fuzzy-filters by prefix.
- [ ] Integrate with editor — detect `@` trigger, track cursor position for popup placement, replace `@query` with selected item on accept.
- [ ] Write tests for `CompletionsModel` — trigger, filter, navigate, accept, dismiss.
- [ ] Write tests for `FileSource` — directory walking, gitignore respect, fuzzy matching.
- [ ] Run tests — must pass before Task 7.

---

### Task 7: Custom Chroma formatter

- [ ] Create `xchroma/formatter.go` — register a "weave" Chroma formatter in `init()`. Map Chroma token types (Keyword, String, Comment, Number, Operator, etc.) to Lip Gloss v2 styles from `styles/styles.go`. Force background color to match chat bubble background.
- [ ] Update `components/messages/markdown.go` — configure Glamour v2 renderer to use the "weave" Chroma formatter for code block syntax highlighting.
- [ ] Write tests for formatter — verify token type → style mapping, verify forced background, verify integration with Glamour rendering.
- [ ] Run tests — must pass before Task 8.

---

### Task 8: File attachments

- [ ] Create `components/attachments/attachments.go` — `AttachmentsModel` tracking `[]Attachment` with file path, line count, content preview. `Draw(scr, area)` renders inline indicators (`file.py (42 lines)`) above the editor.
- [ ] Add paste detection in editor — if paste exceeds threshold (>10 newlines or >1000 chars), auto-convert to temp file attachment. Show indicator in editor area.
- [ ] Add `ctrl+r` attachment delete mode — when attachments exist, `ctrl+r` toggles delete mode (highlight first attachment, press enter to remove).
- [ ] Update `onSubmit` — include attachment contents in the submitted message alongside text.
- [ ] Write tests for attachment model — add, remove, display indicators.
- [ ] Write tests for paste detection — verify short paste passes through, verify long paste converts to attachment.
- [ ] Run tests — must pass before Task 9.

---

### Task 9: Wire up pills bar for tool progress

- [ ] Add pills rendering to `render.go` — when tools are executing, show compact progress indicators in the pills row (between main and editor). Each pill shows tool name + spinner icon. Completed pills show checkmark/X briefly then fade.
- [ ] Update `model.go` — track active tool pills from `ToolResultMsg` events. Pills row height is 0 (hidden) when no tools are active, 1 when tools are running.
- [ ] Update `LayoutEngine.Compute()` — account for pills row (0 or 1 row).
- [ ] Write tests for pills bar — verify shows during tool execution, verify hides when complete, verify layout adjustment.
- [ ] Run tests — must pass before Task 10.

---

### Task 10: Final integration and verification

- [ ] Verify all features work together — tool renderers, @-completions, file attachments, pills, custom Chroma, upgraded diff.
- [ ] Verify `sdk.UI` interface still works — Select, Confirm, Input through overlay stack.
- [ ] Verify headless mode — tool results render as formatted text using `RenderOutput`.
- [ ] Verify third-party tool fallback — `GenericResult` renders as generic panel.
- [ ] Run full test suite — all tests must pass.
- [ ] Run `make lint` and fix any issues.
- [ ] Run `make fix` for formatting.

---

### Task 11: Update documentation

- [ ] Update `CLAUDE.md` — document `sdk/toolresult/` package and result types.
- [ ] Update `CLAUDE.md` — document `sdk.ToolRenderer` interface and `RenderOutput` format.
- [ ] Update `CLAUDE.md` — add new TUI components (completions, attachments, pills, tools dispatch).

---

## Technical Details

### Tool Result Data Flow

```
Tool Extension (e.g., bash)          SDK / Bus              UI Extension (e.g., TUI)
─────────────────────────            ─────────              ────────────────────────

bash.init()
  ├─ sdk.RegisterTool("bash", ...)
  └─ sdk.RegisterToolRenderer(
       "bash", BashRenderer{})

bash executes command
result := toolresult.BashResult{
    Command: cmd,
    ExitCode: code,
    Stdout: out,
}                                      bus.Publish(sdk.Event{
                                         Topic: "agent.tool_result",
                                         Payload: ToolResult{
                                           Tool: "bash",
                                           Data: result,
                                         },
                                       })
                                                                              TUI receives event
                                                                              sdk.GetToolRenderer("bash")
                                                                              → BashRenderer
                                                                              .Render(BashResult)
                                                                              → RenderOutput{
                                                                                  Title: "$ go test",
                                                                                  Sections: [...],
                                                                                }
                                                                              drawRenderOutput()
                                                                              → screen buffer

Headless receives same event
  sdk.GetToolRenderer("bash")
  → BashRenderer.Render(BashResult)
  → RenderOutput → print to stdout
```

### RenderOutput Type System

```go
// sdk/toolrender.go

type SectionType string

const (
    SectionText  SectionType = "text"
    SectionCode  SectionType = "code"
    SectionDiff  SectionType = "diff"
    SectionTable SectionType = "table"
    SectionTree  SectionType = "tree"
    SectionError SectionType = "error"
)

type Section struct {
    Type     SectionType
    Content  string
    Language string       // for code/diff

    Code  CodeMeta        // SectionCode
    Diff  DiffMeta        // SectionDiff
    Table TableMeta       // SectionTable
    Tree  TreeMeta        // SectionTree
    Error ErrorMeta       // SectionError
}

type CodeMeta struct {
    ExitCode  int
    Truncated bool
    LineCount int
    StartLine int
}

type DiffMeta struct {
    Adds    int
    Deletes int
}

type TableMeta struct {
    Headers []string
    Widths  []int
}

type TreeMeta struct {
    DirCount  int
    FileCount int
}

type ErrorMeta struct {
    ExitCode int
}

type RenderOutput struct {
    Title    string
    Sections []Section
}

type ToolRenderer interface {
    Render(data interface{}) RenderOutput
}
```

### New Files

| File | Owner | Purpose |
|------|-------|---------|
| `sdk/toolresult/bash.go` | SDK | `BashResult` struct |
| `sdk/toolresult/edit.go` | SDK | `EditResult` struct |
| `sdk/toolresult/read.go` | SDK | `ReadResult` struct |
| `sdk/toolresult/write.go` | SDK | `WriteResult` struct |
| `sdk/toolresult/grep.go` | SDK | `GrepResult` + `Match` structs |
| `sdk/toolresult/find.go` | SDK | `FindResult` + `Entry` structs |
| `sdk/toolresult/ls.go` | SDK | `LsResult` type alias |
| `sdk/toolresult/generic.go` | SDK | `GenericResult` fallback |
| `sdk/toolrender.go` | SDK | `RenderOutput`, `Section`, typed meta, `ToolRenderer`, registry |
| `extensions/ui/tui/components/messages/tools/dispatch.go` | TUI | `DrawToolResult`, `drawRenderOutput` per section type |
| `extensions/ui/tui/components/completions/completions.go` | TUI | @-mention file completion popup |
| `extensions/ui/tui/xchroma/formatter.go` | TUI | Custom "weave" Chroma formatter |
| `extensions/ui/tui/components/attachments/attachments.go` | TUI | File attachment model and display |

## Post-Completion

**Manual verification:**
- Visual testing of each tool renderer in terminal
- Test @-completions with large codebases (verify directory walk performance)
- Test file attachments with binary files and very large pastes
- Verify pills bar shows correct progress during multi-tool turns
- Verify diff rendering with various languages (Go, Python, YAML, Markdown)
- Test with different terminal emulators (iTerm2, Alacritty, Kitty, Terminal.app)

**Performance considerations:**
- Verify completion file walking cache is bounded
- Verify Chroma lexer caching avoids re-detection overhead
- Profile RenderOutput interpretation vs direct type-switch rendering
