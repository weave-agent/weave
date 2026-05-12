# Merge loop, skills, instructions into agent extension

## Overview

Merge three separate extensions (`loop`, `skills`, `instructions`) into a single `agent` extension that owns the entire conversation lifecycle: prompt assembly, turn loop, tool execution, skill discovery, and context file loading.

**Key improvements:**
- Default system prompt with dynamic tool descriptions (Crush-style)
- SYSTEM.md override support (replaces default base)
- Skills the model can invoke itself via read tool (not just slash commands)
- Extension-bundled skills (each extension can ship skills in a `skills/` subdirectory)
- Eliminates bus indirection between loop/skills/instructions

## Context (from discovery)

**Files/components involved:**
- `extensions/loop/` — turn loop, provider selection, tool execution, event publishing
- `extensions/skills/` — skill discovery from `~/.weave/skills/` and `<project>/.weave/skills/`, slash command registration
- `extensions/instructions/` — context file discovery (CLAUDE.md/AGENTS.md), SYSTEM.md/APPEND_SYSTEM.md loading
- `sdk/wire/wire.go` — extension wiring (needs update for new extension name)
- `extensions/ui/tui/` — slash command handlers for `/skill:<name>` (may need update)

**Related patterns found:**
- All three extensions are independent Go modules with their own `go.mod`
- Loop subscribes to `skills.loaded` and `instructions.loaded` events today
- Skills extension registers `/skill:<name>` commands via `sdk.GetUI("tui")`
- Crush uses template-based system prompt with `{{.AvailSkillXML}}` injection
- Crush discovers skills from extension directories + user directories
- Weave extensions can ship skills in a `skills/` subdirectory (discovered by agent extension)
- Precedence: project `.weave/skills/` > global `~/.weave/skills/` > extension `skills/`

**Dependencies identified:**
- `sdk/` package for Extension, Bus, Config, UI interfaces
- `sdk/model/` for StreamOptions, ThinkingLevel
- `sdk/wire/` for CoreWireConfig
- Tool registry (`sdk.GetTool`) for dynamic tool descriptions
- TUI UI interface for slash command registration

## Development Approach

- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Maintain backward compatibility where possible

## Testing Strategy

- **Unit tests**: required for every task
- Prompt builder tests with various combinations of default/SYSTEM.md/context/skills
- Skill discovery tests (same coverage as current skills extension)
- Context file discovery tests (same coverage as current instructions extension)
- Loop behavior tests (same coverage as current loop extension)
- Integration test: agent extension wires correctly via `sdk.RegisterExtension`

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## Implementation Steps

### Task 1: Create agent extension skeleton

- [ ] Create `extensions/agent/` directory with `go.mod`
- [ ] Create `extension.go` with `AgentExtension` struct, `Subscribe`, `Close`, `Name`
- [ ] Register via `sdk.RegisterExtension("agent", ...)` in `init()`
- [ ] Create minimal `prompt.go` with `promptBuilder` struct
- [ ] Write tests for extension registration and basic Subscribe/Close
- [ ] Run tests — must pass before next task

### Task 2: Port context file discovery (from instructions extension)

- [ ] Port `discover.go` logic to `extensions/agent/context.go`
- [ ] `discoverContextFiles(projectDir, globalDir)` — walks up for CLAUDE.md/AGENTS.md
- [ ] `loadSystemPrompt(projectDir, globalDir)` — loads SYSTEM.md
- [ ] `loadAppendSystemPrompt(projectDir, globalDir)` — loads APPEND_SYSTEM.md
- [ ] Port `formatInstructionsPrompt` logic into prompt builder
- [ ] Write tests for context discovery (project override, global fallback, dedup)
- [ ] Write tests for SYSTEM.md/APPEND_SYSTEM.md loading
- [ ] Run tests — must pass before next task

### Task 3: Port skill discovery (from skills extension)

- [ ] Port `discover.go` and `skill.go` logic to `extensions/agent/skills.go`
- [ ] `discoverSkills(paths...)` — scans skill directories
- [ ] `loadSkillFromDir(dir)` — parses SKILL.md frontmatter + body
- [ ] `validateName(name)` — skill name validation
- [ ] Add extension-bundled skills: discover from each extension's `skills/` subdirectory
- [ ] `discoverExtensionSkills()` — iterates registered extensions, checks for `skills/` dir
- [ ] Precedence: project > global > extension (user overrides extension skills by name)
- [ ] `formatSkillsPrompt(skills)` — XML formatting for system prompt
- [ ] Write tests for skill discovery, parsing, validation, dedup
- [ ] Write tests for extension-bundled skills discovery
- [ ] Run tests — must pass before next task

### Task 4: Build prompt builder with default system prompt

- [ ] Create `default-system-prompt.md` file in `extensions/agent/`
- [ ] Embed via `//go:embed default-system-prompt.md`
- [ ] Include: identity, date, CWD, critical rules, tool usage basics
- [ ] `buildToolDescriptions()` — dynamic list from `sdk.ListTools()`
- [ ] `PromptBuilder.Build()` — assembles layers:
  1. Default prompt OR SYSTEM.md (if found)
  2. Date + CWD (always injected)
  3. Available tools (dynamic)
  4. Skills XML + `<skills_usage>` instructions
  5. Context files (CLAUDE.md/AGENTS.md)
  6. APPEND_SYSTEM.md
- [ ] Write tests for full prompt assembly with all combinations
- [ ] Write tests for SYSTEM.md override (replaces default base)
- [ ] Run tests — must pass before next task

### Task 5: Port turn loop (from loop extension)

- [ ] Port `loop.go` to `extensions/agent/loop.go`
- [ ] Remove `skills.loaded` and `instructions.loaded` channel handling
- [ ] Loop calls `promptBuilder.Build()` directly for system prompt
- [ ] Keep all turn logic: prompt/steer/followup/interrupt handling
- [ ] Keep tool execution via `executeTool`
- [ ] Keep streaming and event publishing
- [ ] Port `stream.go` and `execute.go` helpers
- [ ] Write tests for loop behavior (same coverage as current loop tests)
- [ ] Run tests — must pass before next task

### Task 6: Register slash commands for skills

- [ ] In `Subscribe`, get TUI via `sdk.GetUI("tui")`
- [ ] Register `/skill:<name>` commands for each discovered skill
- [ ] Command handler publishes `agent.prompt` with skill body pre-loaded (existing behavior)
- [ ] Write tests for command registration
- [ ] Run tests — must pass before next task

### Task 7: Wire new agent extension and remove old extensions

- [ ] Update `sdk/wire/wire.go` — add "agent" to core extensions or document it replaces loop
- [ ] Update launcher auto-discovery to skip `extensions/loop/`, `extensions/skills/`, `extensions/instructions/`
- [ ] Delete `extensions/loop/`, `extensions/skills/`, `extensions/instructions/` directories
- [ ] Update any references in `CLAUDE.md` or docs
- [ ] Verify `make test` passes for root and `extensions/agent/`
- [ ] Run full test suite — must pass before next task

### Task 8: Verify acceptance criteria

- [ ] Agent extension discovers and loads CLAUDE.md/AGENTS.md correctly
- [ ] Agent extension discovers and loads skills correctly
- [ ] Default system prompt includes dynamic tool descriptions
- [ ] SYSTEM.md replaces default base when present
- [ ] APPEND_SYSTEM.md is always last
- [ ] Skills XML includes usage instructions for model self-invocation
- [ ] `/skill:<name>` slash commands work
- [ ] Extension-bundled skills are discovered from extension `skills/` subdirectories
- [ ] User skills override extension skills by name
- [ ] Run full test suite (root + all extensions)
- [ ] Run linter — all issues fixed

### Task 9: Update documentation

- [ ] Update `CLAUDE.md` references to old extensions
- [ ] Document new `agent` extension behavior
- [ ] Document SYSTEM.md override behavior
- [ ] Document extension-bundled skills

## Technical Details

**Prompt layering (top to bottom):**

```
SYSTEM.md (if found) OR defaultSystemPrompt
--- injected ---
Current date: YYYY-MM-DD
Current working directory: /path

Available tools:
- read: Read file contents
- bash: Execute shell commands
...

<available_skills>
  <skill>...</skill>
</available_skills>

<skills_usage>
When a skill matches the current task, load it using the read tool
on its <location> before taking any other action.
</skills_usage>

# Project Context
## /path/to/CLAUDE.md
<content>

APPEND_SYSTEM.md (if found)
```

**Skill self-invocation flow:**
1. Skills listed in system prompt with name, description, location
2. Model instructed to `read` matching skill before acting
3. Skill body loaded into conversation as user message
4. Model follows skill instructions

**Embedded files:**
- `default-system-prompt.md` — embedded via `//go:embed`, used when no SYSTEM.md found
- Subagent agent definitions — already embedded in `extensions/tools/subagent/`

**Extension-bundled skills:**
- Each extension can include a `skills/` subdirectory with `SKILL.md` files
- Discovered by scanning registered extension directories
- Precedence: project > global > extension (user skills override extension skills by name)
- No `//go:embed` — skills are regular files, not binary-embedded

## Post-Completion

**Manual verification:**
- Test with project that has CLAUDE.md — verify context appears
- Test with project that has SYSTEM.md — verify override works
- Test with skills in `~/.weave/skills/` — verify discovery
- Test `/skill:<name>` — verify pre-loading works
- Test without any context files — verify default prompt works

**External references:**
- Pi coding agent: `/opt/homebrew/lib/node_modules/@mariozechner/pi-coding-agent/`
- Crush: `/Users/andrey/Projects/crush/internal/skills/`, `/Users/andrey/Projects/crush/internal/agent/templates/`
