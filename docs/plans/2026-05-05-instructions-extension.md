# Instructions Extension — Context Files & Custom System Prompts

## Overview

Add an `instructions` extension that discovers and loads CLAUDE.md/AGENTS.md context files and SYSTEM.md/APPEND_SYSTEM.md custom prompts, assembling them into the system prompt alongside the existing skills XML. This brings weave to parity with pi's context loading system while using weave-native conventions.

**Problem:** Weave currently has no mechanism for loading project-level instructions or customizing the system prompt. The system prompt is solely derived from discovered skills (SKILL.md files) as XML.

**Outcome:** Users can place CLAUDE.md or AGENTS.md in project directories (walked up to root) for project-specific instructions, and SYSTEM.md/APPEND_SYSTEM.md in `.weave/` or `~/.weave/` for system prompt customization.

## Context

### Current system prompt flow
1. Skills extension discovers SKILL.md files in `~/.weave/skills/` and `{project}/.weave/skills/`
2. Publishes XML-formatted skills on `sdk.TopicSkillsLoaded`
3. Loop extension stores it as `availableSkills`
4. Loop passes `availableSkills` as `SystemPrompt` to provider via `streamTurn()`

### Key files
- `sdk/event.go` — shared topic constants (currently only `TopicSkillsLoaded`)
- `sdk/config.go` — `Config` interface with `FilePath()` for project root resolution
- `extensions/skills/extension.go` — pattern to follow for new extension
- `extensions/skills/discover.go` — skill discovery pattern
- `extensions/loop/loop.go:42-46` — stores `availableSkills`, passes as system prompt
- `extensions/loop/loop.go:492-497` — `streamTurn()` sends `SystemPrompt` to provider
- `config/config.go:416-423` — `projectDirFromConfig()` resolves project root from config path

### Existing utilities to reuse
- `sdk.RegisterExtension()` / `sdk.GetExtension()` — extension registration pattern
- `sdk.NewEvent()` — event creation
- `config.GlobalConfigDir()` — returns `~/.weave/`
- Project root resolution pattern from `extensions/skills/extension.go:67-75`

## Development Approach
- **Testing approach:** Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- Every task includes new/updated tests
- All tests must pass before starting next task

## File Locations & Naming Conventions

**Context files** (CLAUDE.md/AGENTS.md) — pi-compatible:
- Walk up from project root (resolved from config `FilePath()`) looking for `CLAUDE.md` or `AGENTS.md`
- Global: `~/.weave/CLAUDE.md` or `~/.weave/AGENTS.md`
- Same dedup semantics as pi: closest to project root takes precedence, `seenPaths` set

**System prompt files** — weave-native:
- Project: `.weave/SYSTEM.md`, `.weave/APPEND_SYSTEM.md`
- Global: `~/.weave/SYSTEM.md`, `~/.weave/APPEND_SYSTEM.md`
- Project overrides global

## Implementation Steps

### Task 1: Create instructions extension skeleton
- [x] create `extensions/instructions/` directory with `go.mod` (module `weave/ext/instructions`, same pattern as `extensions/skills/go.mod`)
- [x] create `extension.go` with `InstructionsExtension` struct, `init()` registering via `sdk.RegisterExtension("instructions", ...)`, `Name()`, `Subscribe()`, `Close()` methods
- [x] create `discover.go` with `discoverContextFiles(projectDir, globalDir string) []ContextFile` — walks up from `projectDir` to root, then checks `globalDir`, returns ordered list of `{Path, Content}` structs
- [x] create `system.go` with `loadSystemPrompt(projectDir, globalDir string) (base string, append string)` — checks for SYSTEM.md and APPEND_SYSTEM.md in project `.weave/` dir and `~/.weave/`, project overrides global
- [x] create `prompt.go` with `formatInstructionsPrompt(contextFiles []ContextFile, systemBase, systemAppend string) string` — assembles all parts into a single string with clear section headers
- [x] add `TopicInstructionsLoaded` constant to `sdk/event.go`
- [x] run `go build` from within `extensions/instructions/` to verify compilation

### Task 2: Implement context file discovery and system prompt loading
- [x] implement walk-up algorithm in `discover.go`: iterate from `projectDir` upward to filesystem root, check for `CLAUDE.md` then `AGENTS.md` in each dir, dedup by absolute path, collect in order (closest first)
- [x] add global context file loading: check `~/.weave/CLAUDE.md` and `~/.weave/AGENTS.md`
- [x] implement `loadSystemPrompt` in `system.go`: check `.weave/SYSTEM.md` (project) → `~/.weave/SYSTEM.md` (global), same for APPEND_SYSTEM.md
- [x] implement `formatInstructionsPrompt` in `prompt.go`: if context files exist, add "# Project Context" section with each file as `## {path}\n\n{content}`; if systemBase exists, prepend; if systemAppend exists, append
- [x] wire `Subscribe()` to call discover + load + format, then publish on `TopicInstructionsLoaded`
- [x] run `go build` from within `extensions/instructions/` to verify compilation

### Task 3: Write tests for instructions extension
- [x] create `extension_test.go` with tests for `discoverContextFiles`: no files found, single file found, walk-up precedence (closest wins), deduplication, global fallback
- [x] add tests for `loadSystemPrompt`: no files, project override, global fallback, both project and global
- [x] add tests for `formatInstructionsPrompt`: empty input, context files only, system prompt only, all combined
- [x] add integration test for `Subscribe()` publishing correct event on bus
- [x] run `cd extensions/instructions && go test ./...` — all tests must pass

### Task 4: Integrate instructions with loop extension
- [x] add `instructionsLoaded string` field to `Loop` struct in `extensions/loop/loop.go`
- [x] subscribe to `TopicInstructionsLoaded` in the select loop (same pattern as skills at line 191-194)
- [x] add drain helper for instructions channel (same pattern as `drainSkills`)
- [x] modify `streamTurn` call site to combine instructions + skills into the final system prompt: if both non-empty, concatenate with separator; if only one, use it
- [x] update `streamTurn` signature if needed, or assemble prompt before calling it
- [x] run `cd extensions/loop && go test ./...` — all tests must pass

### Task 5: Wire instructions into default config and launcher
- [x] add `"instructions"` to the default extensions list in `config/config.go:DefaultFile()` and `DefaultConfigJSON()`
- [x] verify launcher discovery picks up the new extension (it's a built-in under `extensions/instructions/`)
- [x] update `CLAUDE.md` Architecture section to document the new extension and its file locations
- [x] run `make test` from root — all tests must pass
- [x] run `make lint` — no new issues

### Task 6: Verify acceptance criteria
- [x] verify CLAUDE.md files are discovered by walking up from project directory
- [x] verify AGENTS.md files are discovered (same logic, alternative name)
- [x] verify `.weave/SYSTEM.md` overrides the system prompt base
- [x] verify `.weave/APPEND_SYSTEM.md` appends to the system prompt
- [x] verify global `~/.weave/SYSTEM.md` and `~/.weave/APPEND_SYSTEM.md` work as fallback
- [x] verify project files take precedence over global files
- [x] verify instructions + skills are both present in the final system prompt
- [x] run full test suite — all pass
- [x] run linter — clean

## Technical Details

### New bus topic
```go
// sdk/event.go
TopicInstructionsLoaded = "instructions.loaded"
```

### Context file struct
```go
type ContextFile struct {
    Path    string
    Content string
}
```

### Prompt assembly order (in `formatInstructionsPrompt`)
1. System base prompt (from SYSTEM.md) — if present
2. Project context files (CLAUDE.md/AGENTS.md) — "# Project Context" section
3. System append prompt (from APPEND_SYSTEM.md) — if present

The loop extension then combines: `instructions + "\n\n" + skills` as the final system prompt.

### Discovery algorithm
```
projectDir = resolve from config.FilePath() (same as skills extension)
globalDir  = ~/.weave/

contextFiles = []
seenPaths = set{}

// Walk up from project to root
dir = projectDir
while dir != "/" and dir != parent(dir):
    for name in ["CLAUDE.md", "AGENTS.md"]:
        path = dir/name
        if exists(path) and path not in seenPaths:
            contextFiles.prepend({path, content})  // closest first
            seenPaths.add(path)
            break  // only first match per directory
    dir = parent(dir)

// Global fallback
for name in ["CLAUDE.md", "AGENTS.md"]:
    path = globalDir/name
    if exists(path) and path not in seenPaths:
        contextFiles.append({path, content})
        seenPaths.add(path)
        break
```

### System prompt files
```
base = readFile(".weave/SYSTEM.md") or readFile("~/.weave/SYSTEM.md") or ""
append = readFile(".weave/APPEND_SYSTEM.md") or readFile("~/.weave/APPEND_SYSTEM.md") or ""
```

## Post-Completion
*Items requiring manual intervention or external systems*

**Manual verification:**
- Create a test project with nested CLAUDE.md files and verify walk-up discovery
- Test with SYSTEM.md and APPEND_SYSTEM.md in `.weave/` directory
- Verify behavior in headless mode (`-p` flag)
- Test with skills + instructions both active

**Documentation:**
- Update `CLAUDE.md` with new extension docs
- Update `docs/design.md` if it references the system prompt flow
