# Subagent Tool Extension

## Overview

Add a subagent system to Weave as a tool extension (`extensions/tools/subagent/`). Each subagent runs as an isolated `weave -p` subprocess with restricted tools, optional sandbox mode, and JSON streaming results. Agent definitions are markdown files with YAML frontmatter, discovered from `.weave/agents/` and `~/.weave/agents/`. Three built-in agents ship with the extension: general-purpose, explore, and plan.

**Key design decisions (from brainstorm):**
- Subagent = `weave -p --output json --tools <list> [--sandbox <mode>] --subagent-id <id>` subprocess
- No SDK interface changes — pure tool extension
- Three execution modes: single, parallel, chain
- Tool allowlist from agent definition, passed as `--tools` CLI flag
- Sandbox mode passthrough via `--sandbox` CLI flag
- Results stream as JSON lines on stdout
- **Full-mesh inter-agent communication** via parent-as-broker routing through stdin/stdout
- Agents discover each other via roster injection + `list_agents` tool

## Context (from discovery)

### Files/components involved

- `extensions/tools/grep/grep.go` — reference tool pattern (init + RegisterTool + Tool interface)
- `extensions/tools/bash/` — reference for subprocess execution
- `sdk/wire/run.go` — print/headless mode handling, `-p` flag logic (Line 76: `headless := cf.Prompt != ""`)
- `sdk/wire/wire.go` — tool wiring and filtering (Lines 24-59)
- `sdk/tool_registry.go` — RegisterTool/GetTool pattern (Lines 15-26)
- `config/config.go` — ToolConfig method for tool settings (Lines 383-403)
- `config/settings.go` — Settings struct, load/save
- `config/merge.go` — LoadLayeredSettings for project/global/local layers
- `extensions/sandbox/sandbox.go` — bus Subscribe pattern reference (Lines 80-145)
- `sdk/model/` — model registry for model validation in agent definitions

### Related patterns found

- All tool extensions follow identical pattern: `init()` → `sdk.RegisterTool()` → implement `Tool` interface
- Print mode passes prompt via `--weave-prompt-file` to child binary
- Sandbox already intercepts tool calls — subagent invocations go through same approval flow
- Tool config loaded via `cfg.ToolConfig(name, &target)` with JSON round-trip and `default` struct tags

### Dependencies identified

- No new external dependencies
- Uses stdlib `os/exec` for subprocess spawning
- Uses existing `sdk.Tool`, `sdk.ToolDef`, `sdk.ToolResult` types

## Development Approach

- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change

## Testing Strategy

- **Unit tests**: required for every task
- Tests cover agent definition parsing, discovery, subprocess invocation, and execution modes
- Mock subprocess execution where needed (test the protocol, not actual weave binary)
- Test edge cases: missing tools, invalid agent defs, abort handling

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## Implementation Steps

### Task 1: Agent definition parser and built-in agents
- [x] create `extensions/tools/subagent/` directory with `go.mod`
- [x] create `agent.go` with `AgentDef` struct (Name, Description, Tools, Model, Sandbox, System fields)
- [x] implement `ParseAgent(data []byte) (*AgentDef, error)` — parse YAML frontmatter + markdown body from agent definition files
- [x] create `agents/general.md` — full tools, any task, default model
- [x] create `agents/explore.md` — read/grep/find/ls tools, research agent
- [x] create `agents/plan.md` — read-only, implementation planning agent
- [x] write tests for `ParseAgent` — valid definitions, missing fields, invalid YAML, body as system prompt fallback
- [x] run tests — must pass before task 2

### Task 2: Agent discovery
- [x] create `discovery.go` with `DiscoverAgents(projectDir string) ([]*AgentDef, error)` — walk `.weave/agents/` then `~/.weave/agents/`, dedup by name (project wins over global)
- [x] load embedded built-in agents from `agents/` directory using `embed.FS`
- [x] merge built-in + discovered agents (user agents override built-ins with same name)
- [x] write tests for discovery — mock filesystem, dedup precedence, missing dirs, invalid files skipped with warning
- [x] run tests — must pass before task 3

### Task 3: Subagent tool registration
- [x] create `subagent.go` with `init()` calling `sdk.RegisterTool` for each discovered agent
- [x] implement `Tool` interface: `Name()` returns `subagent_<agentname>`, `Definition()` returns tool definition with description from agent def
- [x] tool parameters: `prompt` (string), `tasks` (array for parallel), `chain` (array for sequential), `background` (bool, default false), `cwd` (string override)
- [x] validate mutually exclusive params (exactly one of prompt/tasks/chain)
- [x] write tests for tool registration and parameter validation
- [x] run tests — must pass before task 4

### Task 4: Subprocess execution engine
- [x] create `execute.go` with `runSubagent(ctx context.Context, agent *AgentDef, prompt, cwd string) (string, error)`
- [x] build command: `weave -p --output json --tools <tools> [--sandbox <mode>] [--model <model>]` with prompt passed via `--weave-prompt-file`
- [x] spawn process via `exec.CommandContext`, pipe stdout
- [x] parse JSON lines from stdout — handle `message_start`, `message_update`, `tool_call`, `tool_result`, `message_end` event types
- [x] extract final content from `message_end` event as tool result
- [x] handle abort: ctx cancellation kills child process
- [x] write tests for JSON line parsing, command building, abort handling (mock exec.Command)
- [x] run tests — must pass before task 5

### Task 5: Execution modes (parallel and chain)
- [x] create `modes.go` with parallel and chain execution logic
- [x] parallel mode: spawn N processes concurrently via goroutines, collect results, aggregate into single ToolResult
- [x] chain mode: spawn processes sequentially, substitute `{previous}` placeholder in each step's prompt with prior result
- [x] error handling: partial failures in parallel (collect all, report which succeeded/failed), chain stops on first error
- [x] write tests for parallel mode (mock subprocesses, verify concurrency), chain mode ({previous} substitution, early stop)
- [x] run tests — must pass before task 6

### Task 6: Background execution
- [x] create `background.go` with background subagent management
- [x] when `background: true`, `Execute()` spawns the subprocess but returns immediately with `{"id":"subagent_explore_abc123","status":"running"}`
- [x] broker tracks background agents in `map[string]*backgroundAgent` (ID → process, status, result channel)
- [x] background agents emit `{"type":"agent_done","id":"...","status":"completed|failed","content":"..."}` when finished — broker receives and stores result
- [x] register `check_agent(id)` tool — returns current status and result (if done) of a background agent
- [x] register `await_agent(id)` tool — blocks until the specified background agent completes, then returns its result
- [x] broker publishes `subagent.done` event on parent bus when background agent finishes — agent loop can surface to LLM as injected context
- [x] write tests for background spawn (returns immediately), check_agent (pending/completed states), await_agent (blocks until done), agent_done notification
- [x] run tests — must pass before task 7

### Task 7: CLI flag support (--output json, --tools, --subagent-id)
- [x] add `--output` flag to weave CLI — values: `text` (default), `json`
- [x] modify print mode in `sdk/wire/run.go` to emit JSON lines when `--output json` is set
- [x] define JSON event types: `message_start`, `message_update`, `message_end`, `tool_call`, `tool_result` — reuse existing bus event payloads
- [x] add `--tools` flag — comma-separated allowlist that filters tool registry before wiring in `sdk/wire/wire.go`
- [x] add `--subagent-id` flag — when set, enables subagent mode in child process (registers inter-agent tools, reads stdin for messages)
- [x] ensure `--sandbox` flag already works (verify passthrough, add if missing)
- [x] write tests for `--output json` flag (verify JSON line format), `--tools` filtering (only allowed tools wired), `--subagent-id` flag behavior
- [x] run tests — must pass before task 8

### Task 8: Inter-agent communication — parent broker
- [x] create `broker.go` with `Broker` struct — holds registry of active subagent processes (`map[string]*subagentProc` where each has stdin pipe + stdout scanner)
- [x] implement routing: read `send` events from child stdout → find target by ID → write `agent_msg` to target's stdin
- [x] implement broadcast: read `broadcast` events from child stdout → write `agent_msg` to all active children's stdin
- [x] implement roster injection: when spawning a new subagent, inject current active agent list via stdin as initial context message
- [x] implement `inject` API: parent session can push context to any running child
- [x] implement `list_agents` request/response: child calls `list_agents` tool → broker writes roster back to child's stdin as tool result
- [x] write tests for broker routing (mock subagent processes), roster injection, broadcast fan-out, target-not-found handling
- [x] run tests — must pass before task 9

### Task 9: Inter-agent communication — child subagent tools
- [x] create `messaging.go` with `send_message`, `broadcast_message`, `list_agents` tool implementations
- [x] `send_message(to, content)` — writes `{"type":"send","to":"...","content":"..."}` to stdout, returns tool result confirming delivery
- [x] `broadcast_message(content)` — writes `{"type":"broadcast","content":"..."}` to stdout, returns tool result with recipient count
- [x] `list_agents()` — writes `{"type":"list_agents"}` to stdout, blocks until broker responds via stdin with roster, returns formatted list
- [x] register these tools only when `--subagent-id` flag is set AND agent definition has `messaging: true`
- [x] write tests for each inter-agent tool (mock stdout writing, stdin response parsing)
- [x] run tests — must pass before task 10

### Task 10: Inter-agent communication — stdin listener in child process
- [ ] add stdin listener goroutine in child's agent loop when `--subagent-id` is set — reads JSON lines from stdin in background
- [ ] handle `inject` messages: queue as user messages in agent loop's conversation
- [ ] handle `agent_msg` messages: queue as user messages with `[from <agent_id>]` prefix for context
- [ ] handle `cancel` message: trigger agent loop cancellation
- [ ] handle `list_agents_response`: deliver to waiting `list_agents` tool call
- [ ] graceful shutdown: close stdin listener on context cancellation, drain remaining messages
- [ ] write tests for stdin listener (mock stdin pipe, verify message queuing, cancel handling)
- [ ] run tests — must pass before task 11

### Task 11: Agent definition `messaging` field
- [ ] add `Messaging bool` field to `AgentDef` struct (`messaging` in YAML frontmatter, default: false)
- [ ] update `ParseAgent` to parse `messaging` field
- [ ] update `general.md` built-in to set `messaging: true`
- [ ] `explore.md` and `plan.md` keep `messaging: false` (simple agents don't need inter-agent comms)
- [ ] when `messaging: true`, pass `--subagent-id` flag to child process and register inter-agent tools
- [ ] write tests for messaging field parsing, tool registration conditional on messaging flag
- [ ] run tests — must pass before task 12

### Task 12: Integration and verification
- [ ] verify all requirements from Overview are implemented
- [ ] verify edge cases: no agents discovered, agent with invalid tools, child process crash, target agent not found for send_message
- [ ] run full test suite (`make test`)
- [ ] run linter (`make lint`) — all issues must be fixed
- [ ] verify test coverage for new extension module
- [ ] update CLAUDE.md with subagent extension documentation

## Technical Details

### Agent definition format (markdown + YAML frontmatter)

```markdown
---
name: explore
description: Fast codebase exploration for research and context gathering
tools: read,grep,find,ls
model: claude-haiku-4-5
sandbox: readonly
messaging: false
system: |
  You are a research agent. Explore the codebase to answer questions.
  Report findings concisely. Never modify any files.
---

Optional additional system prompt instructions in markdown body.
```

### Subagent invocation command

```
weave -p \
  --output json \
  --tools read,grep,find,ls \
  --sandbox readonly \
  --model claude-haiku-4-5 \
  --subagent-id subagent_explore_abc123 \
  --weave-prompt-file /tmp/weave-subagent-XXXX.txt
```

### JSON protocol — child stdout → parent (one JSON object per line)

```json
// Streaming events (always present):
{"type":"message_start","model":"claude-haiku-4-5"}
{"type":"message_update","content":"I found the issue in..."}
{"type":"tool_call","tool":"grep","args":{"pattern":"TODO"}}
{"type":"tool_result","tool":"grep","output":"..."}

// Inter-agent events (when messaging: true):
{"type":"send","to":"subagent_coder_def456","content":"Found auth logic at auth.go:42"}
{"type":"broadcast","content":"All: found circular dep in imports"}
{"type":"list_agents"}  // request roster from broker

// Terminal event (always present):
{"type":"message_end","content":"Final answer here","usage":{"input":150,"output":200}}

// Background completion (emitted by broker on parent bus):
{"type":"agent_done","id":"subagent_explore_abc123","status":"completed","content":"Final answer here"}
```

### JSON protocol — parent stdin → child (one JSON object per line)

```json
{"type":"inject","content":"User wants you to focus on tests instead"}
{"type":"agent_msg","from":"subagent_explore_abc123","content":"Auth logic is in auth.go:42-67"}
{"type":"list_agents_response","agents":[{"id":"subagent_explore_abc123","name":"explore","status":"running"},{"id":"subagent_coder_def456","name":"coder","status":"running"}]}
{"type":"cancel"}
```

### Inter-agent tool definitions (registered in child when messaging: true)

```json
[
  {
    "name": "send_message",
    "description": "Send a message to another running agent",
    "parameters": {
      "to": {"type": "string", "description": "Target agent ID"},
      "content": {"type": "string", "description": "Message content"}
    }
  },
  {
    "name": "broadcast_message",
    "description": "Send a message to all other running agents",
    "parameters": {
      "content": {"type": "string", "description": "Message content"}
    }
  },
  {
    "name": "list_agents",
    "description": "List all currently running agents and their IDs",
    "parameters": {}
  }
]
```

### Background agent tools (registered in parent when subagent extension loads)

```json
[
  {
    "name": "check_agent",
    "description": "Check the status and result of a background agent",
    "parameters": {
      "id": {"type": "string", "description": "Agent ID returned from background spawn"}
    }
  },
  {
    "name": "await_agent",
    "description": "Block until a background agent completes and return its result",
    "parameters": {
      "id": {"type": "string", "description": "Agent ID returned from background spawn"}
    }
  }
]
```
```

### Parent broker routing

```
┌─────────────────┐     stdout      ┌──────────────────┐
│ subagent_explore │ ──────────────→ │                  │
│   (abc123)       │ ←────────────── │                  │
└─────────────────┘     stdin        │   Parent Broker  │
                                    │  (subagent tool)  │
┌─────────────────┐     stdout      │                  │
│ subagent_coder  │ ──────────────→ │  Registry:       │
│   (def456)      │ ←────────────── │  abc123 → stdin  │
└─────────────────┘     stdin        │  def456 → stdin  │
                                    └──────────────────┘
```

- `send` from child stdout → lookup target ID → write `agent_msg` to target stdin
- `broadcast` from child stdout → write `agent_msg` to all active children stdin
- `list_agents` from child stdout → write `list_agents_response` to same child stdin
- Parent session inject via `inject` to any child stdin
- Roster injected at spawn: initial `agent_msg` with current active agents

### Tool definition exposed to LLM

```json
{
  "name": "subagent_explore",
  "description": "Fast codebase exploration for research and context gathering",
  "parameters": {
    "type": "object",
    "properties": {
      "prompt": {"type": "string", "description": "Task for single mode"},
      "tasks": {"type": "array", "items": {"type": "object"}, "description": "For parallel mode"},
      "chain": {"type": "array", "items": {"type": "object"}, "description": "For chain mode"},
      "background": {"type": "boolean", "description": "Run in background, return agent ID immediately"},
      "cwd": {"type": "string", "description": "Working directory override"}
    }
  }
}
```

### Discovery paths

1. Built-in agents: embedded in `extensions/tools/subagent/agents/` via `embed.FS`
2. User agents: `~/.weave/agents/*.md` (global)
3. Project agents: `.weave/agents/*.md` (project-local, wins over global)
4. User agents override built-ins with same name

### Module structure

```
extensions/tools/subagent/
  go.mod
  subagent.go       # init(), Tool interface, registration
  agent.go          # AgentDef struct, ParseAgent()
  discovery.go      # DiscoverAgents(), embedded built-ins
  execute.go        # runSubagent(), JSON line parsing
  modes.go          # parallel and chain execution
  background.go     # background execution, check_agent, await_agent
  broker.go         # parent-side message broker (routing, registry, roster)
  messaging.go      # child-side inter-agent tools (send_message, broadcast_message, list_agents)
  agents/
    general.md      # built-in: full tools, messaging: true
    explore.md      # built-in: read-only research
    plan.md         # built-in: read-only planning
```

## Post-Completion

**Manual verification:**
- Test subagent invocation end-to-end with actual weave binary
- Test parallel mode with 3+ concurrent subagents
- Test chain mode with multi-step pipeline
- Verify sandbox passthrough works with Seatbelt/bwrap
- Verify abort (Ctrl+C) kills child processes
- Test agent discovery from `.weave/agents/` and `~/.weave/agents/`
