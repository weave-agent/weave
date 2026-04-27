# Skills Support

## Overview
Add Agent Skills support following the [Agent Skills specification](https://agentskills.io/specification). Skills are markdown-based capability packages (SKILL.md directories) discovered from the filesystem. The agent loads skill descriptions into the system prompt (~100 tokens each), reads full instructions on demand (<5000 tokens), and accesses bundled scripts/references as needed (progressive disclosure).

**Key features:**
- Filesystem discovery from `~/.weave/skills/` (global) and `.weave/skills/` (project-local)
- `<available_skills>` XML injection into the system prompt
- `/skill:name` slash command expansion in TUI
- TUI autocomplete for skill commands
- TUI rendering of skill invocations

## Context
- **New extension module**: `extensions/skills/` — independent Go module following existing extension pattern
- **Existing registries**: `sdk.RegisterExtension`, `sdk.GetUI`, `sdk.GetTool` — skills extension uses these to integrate
- **System prompt injection point**: `extensions/loop/loop.go:419` — `ProviderRequest.SystemPrompt` field exists but is never populated
- **Command dispatch**: `extensions/ui/tui/commands.go:101-119` — `Dispatch()` uses exact map lookup; `/skill:name` works as-is since colon is part of the name
- **Editor autocomplete**: `extensions/ui/tui/components/editor.go:59-62` — `SetSlashCommands()` exists but called only once during model creation; needs dynamic refresh mechanism
- **UI command registration**: `extensions/ui/tui/ui_impl.go:148-168` — `RegisterCommand()` adds to registry but doesn't update editor autocomplete
- **Agent Skills spec**: SKILL.md with YAML frontmatter (`name`, `description`, optional `license`, `compatibility`, `metadata`, `allowed-tools`)

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Testing Strategy
- **Unit tests**: required for every task
- No e2e tests in this project — unit tests only

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## What Goes Where
- **Implementation Steps** (`[ ]` checkboxes): code changes, tests
- **Post-Completion** (no checkboxes): manual testing, external verification

## Implementation Steps

### Task 1: Skill types and discovery
Core skill loading logic — types, filesystem walker, YAML frontmatter parser.

- [ ] create `extensions/skills/` Go module with `go.mod`
- [ ] define `Skill` struct with fields: `Name`, `Description`, `FilePath`, `BaseDir`, `DisableModelInvocation`, `License`, `Compatibility`, `Metadata`, `AllowedTools`
- [ ] define `SkillFrontmatter` struct for YAML parsing
- [ ] implement `loadSkillFromDir(dir string) (Skill, error)` — reads `SKILL.md`, parses frontmatter, validates name matches directory, validates description presence
- [ ] implement `discoverSkills(paths ...string) ([]Skill, error)` — walks `~/.weave/skills/` and `.weave/skills/`, deduplicates by name (first found wins), returns sorted list
- [ ] implement `formatSkillsPrompt(skills []Skill) string` — generates `<available_skills>` XML block with name, description, location per skill
- [ ] write tests for `loadSkillFromDir` — valid skill, missing SKILL.md, invalid frontmatter, name/directory mismatch
- [ ] write tests for `discoverSkills` — multiple paths, deduplication, empty directories
- [ ] write tests for `formatSkillsPrompt` — XML structure, empty skills list, special chars in description
- [ ] run `cd extensions/skills && go test ./...` — must pass before next task

### Task 2: Skills extension module
Extension self-registration, bus integration, command registration.

- [ ] implement `init()` with `sdk.RegisterExtension("skills", factory)`
- [ ] implement `SkillsExtension` struct with `Name()`, `Subscribe(bus)`, `Close()` methods
- [ ] in `Subscribe`: discover skills, publish `skills.loaded` event with skill list on bus
- [ ] in `Subscribe`: get UI via `sdk.GetUI("tui")`, register `/skill:name` command for each discovered skill that reads full SKILL.md and publishes as `agent.prompt` with expanded content
- [ ] implement skill command handler — reads SKILL.md body (strips frontmatter), wraps in `<skill name="..." location="...">` XML, appends user args
- [ ] handle missing UI gracefully (headless mode — no commands registered, only bus events)
- [ ] write tests for extension factory, Subscribe lifecycle, command handler with mocked UI and bus
- [ ] write tests for skill command expansion — frontmatter stripping, XML wrapping, args appending
- [ ] run tests — must pass before next task

### Task 3: System prompt injection in loop
Loop subscribes to `skills.loaded` and injects skills XML into the system prompt.

- [ ] add `availableSkills string` field to `Loop` struct in `extensions/loop/loop.go`
- [ ] subscribe to `skills.loaded` topic in `Subscribe()` — store formatted skills prompt
- [ ] modify `streamTurn` to populate `ProviderRequest.SystemPrompt` with the skills prompt when non-empty
- [ ] write tests for system prompt injection — with skills (prompt contains XML), without skills (empty string), skills update via bus event
- [ ] run `cd extensions/loop && go test ./...` — must pass before next task

### Task 4: TUI autocomplete refresh
Make dynamically registered skill commands appear in the editor autocomplete.

- [ ] add `slashCommandsUpdatedMsg` message type to `extensions/ui/tui/model.go`
- [ ] in `TUIImpl.RegisterCommand()`, after registering the command, send `slashCommandsUpdatedMsg` to the tea.Program
- [ ] in `Model.Update()`, handle `slashCommandsUpdatedMsg` by calling `m.editor = m.editor.SetSlashCommands(m.commands.Names())`
- [ ] write tests for autocomplete refresh — verify slashCmds list grows after RegisterCommand
- [ ] run `cd extensions/ui/tui && go test ./...` — must pass before next task

### Task 5: TUI rendering of skill invocations
Render skill command invocations with `[skill]` prefix in the chat view.

- [ ] detect skill blocks in user messages (XML pattern `<skill name="..."`)
- [ ] render matched messages with `[skill]` prefix and skill name, collapsible to show full content
- [ ] write tests for skill block detection and rendering
- [ ] run tests — must pass before next task

### Task 6: Config integration
Add skills to config file format and auto-include the skills extension.

- [ ] add `Skills map[string]any` field to `config.File` struct (optional, for per-skill config overrides)
- [ ] in `cmd/weave/main.go`, auto-include `"skills"` in the extension list (always loaded, like loop)
- [ ] write tests for config parsing with skills section
- [ ] run tests — must pass before next task

### Task 7: Verify acceptance criteria
- [ ] verify all requirements from Overview are implemented
- [ ] verify progressive disclosure works — descriptions in system prompt, full content loaded on demand
- [ ] verify `/skill:name` commands work in TUI with autocomplete
- [ ] verify skill invocation rendering in chat
- [ ] run full test suite (`make test`) — all must pass
- [ ] run linter (`make lint`) — all issues must be fixed

## Technical Details

### SKILL.md format (from specification)
```markdown
---
name: my-skill
description: What this skill does and when to use it. Max 1024 chars.
license: Apache-2.0
compatibility: Requires python3
metadata:
  author: example
  version: "1.0"
allowed-tools: bash read write
---

# Skill Instructions

Step-by-step instructions for the agent...
```

### Name validation rules
- 1-64 chars, lowercase a-z, 0-9, hyphens only
- No leading/trailing hyphens, no consecutive hyphens
- Must match parent directory name
- Cannot contain "anthropic" or "claude"

### Discovery paths
1. `~/.weave/skills/<skill-name>/SKILL.md` — global user skills
2. `.weave/skills/<skill-name>/SKILL.md` — project-local skills
3. First found wins on name collision

### System prompt XML format
```xml
<available_skills>
<skill>
<name>my-skill</name>
<description>What this skill does</description>
<location>/absolute/path/to/my-skill/SKILL.md</location>
</skill>
</available_skills>
```

### Skill command expansion
Input: `/skill:my-skill do something`
Expanded to user message:
```
<skill name="my-skill" location="/path/to/my-skill/SKILL.md">
References are relative to /path/to/my-skill.

[Full SKILL.md body content, frontmatter stripped]

</skill>

do something
```

### Bus events
- `skills.loaded` — published by skills extension after discovery, payload: `[]Skill` formatted as XML string

## Post-Completion

**Manual verification:**
- Create a test skill in `~/.weave/skills/test-skill/SKILL.md` and verify it appears in autocomplete
- Test progressive disclosure: verify descriptions appear in system prompt, full content loads on `/skill:name` invocation
- Test project-local skills override global skills with same name
- Test headless mode (`-p` flag) — skills should still inject into system prompt without TUI commands

**Documentation:**
- Update `CLAUDE.md` with skills architecture section
- Add example skill to a `skills/` directory in repo (or docs)
