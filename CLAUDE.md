# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A coding agent framework written in Go — event-driven, extension-based, with dynamic compilation of selected extensions at runtime. Agent-loop, providers (Anthropic, OpenAI, Z.ai), and tools (bash, read, edit, write, grep, find, ls) are implemented as independent extension modules.

## Commands

```bash
make lint          # Run golangci-lint
make fmt           # Format code (gofumpt, goimports, go fix) ALWAYS use make fix before manual fixing
make fix           # Auto-fix linter issues
make gen           # Regenerate mocks (moq)
make tools         # Install dev tools (moq, golangci-lint)
go test ./...      # Run all tests
go test ./launcher/...  # Run tests for a single package
```

## Testing

- **Assertions**: Use `github.com/stretchr/testify` — `require` for fatal assertions (prerequisite failures, nil deref risk), `assert` for non-fatal checks. Never use raw `t.Error`/`t.Fatal`.
- **Mocks**: Use moq-generated mocks exclusively. Run `make gen` after changing interfaces. Mocks live in `*_mock_test.go` files — never edit them by hand.
- **go:generate**: Each SDK interface file has a `//go:generate moq ...` directive. Cross-package mocks (e.g., in `extensions/loop/`) use `-skip-ensure -pkg <pkg>`.
- **No hand-written mocks**: If a mock needs custom behavior (scripted responses, call recording), set the mock's `Func` fields or write a helper function that configures a moq mock — never create a new mock struct.

## Architecture

Standard library as much as possible. Every replaceable component is an extension (runner, provider, tools, store, hooks). Extensions are independent Go modules that self-register via `init()`.

**Launcher pattern:** resolve config → pick extensions → build a custom binary (cached per hash) → exec it. The `cmd/weave/main.go` entry point orchestrates this pipeline.

**Key packages:**
- `sdk/` — defines `Extension`, `Bus`, `Config` interfaces; global registries for extensions, providers, and tools (`RegisterExtension`/`GetExtension`, `RegisterProvider`/`GetProvider`, `RegisterTool`/`GetTool`); `Message` types; `Wire()` and `WireWithCore()` composition roots that resolve names and subscribe extensions to the bus
- `bus/` — channel-based pub/sub event bus (`Publish`/`Subscribe`/`SubscribeAll`) with buffered channels and graceful close
- `config/` — config discovery (walks up from cwd for `.weave.yaml` or `.weave/config.yaml`) and loading via gonfig. Config has a `core` section (agent_loop + providers) and `extensions` list.
- `internal/truncate/` — shared output truncation (2000 lines / 50KB) used by all tools for consistent output limiting
- `extensions/agent-loop/` — core extension implementing the two-level while-loop agent cycle (outer: follow-ups, inner: steering + tool calls); subscribes to `agent.prompt`, `agent.steer`, `agent.followup`; publishes `agent.turn_start/end`, `agent.message_start/update/end`, `agent.tool_result`, `agent.end`
- `extensions/tools/{bash,read,edit,write,grep,find,ls}/` — individual tool extension modules, each an independent Go module self-registering via `sdk.RegisterTool`
- `extensions/providers/openai-compat/` — shared library for OpenAI-compatible providers (SSE parsing, message/tool conversion); reused by `openai` and `zai` providers; import as `openaicompat` package
- `extensions/providers/{anthropic,openai,zai}/` — provider extension modules; Anthropic uses official SDK, OpenAI and Z.ai delegate to `openai-compat`
- `launcher/` — full pipeline: `Discover` extensions (project-local `.weave/extensions/{name}/`, global `~/.weave/extensions/{name}/`, then built-in under `extensions/{category}/{name}/` with nested lookup), `ComputeHash` of .go files, `Cache` in `~/.weave/bin/{hash}/`, `Build` by generating go.mod+main.go with blank imports, then `syscall.Exec`

**Extension lifecycle:** Extension packages call `sdk.RegisterExtension(name, factory)` in `init()`. Provider and tool extensions similarly call `sdk.RegisterProvider` and `sdk.RegisterTool`. The built binary blank-imports selected extensions, triggering registration. `sdk.Wire()` or `sdk.WireWithCore()` resolves names from registries and subscribes each to the bus.

## Configuration

All config loading uses `github.com/nniel-ape/gonfig` (user-owned lib). No direct `yaml.v3` imports — gonfig handles all file parsing internally. Config files are `.weave.yaml` or `.weave/config.yaml`, discovered by walking up from cwd. Extensions define their own typed config structs with gonfig tags (`default`, `description`, `env`, `validate`) and call `gonfig.Load` themselves with `WithFile` + `WithEnvPrefix("WEAVE")`.

`sdk.Config` is a thin carrier interface (`FilePath() string`) — extensions use the path to load their own config via gonfig. `sdk.Wire` takes `[]string` (extension names) directly. `sdk.WireWithCore` takes a `CoreWireConfig` struct (AgentLoop + Providers) alongside optional extension names, merging and deduplicating them. Config yaml format:

```yaml
core:
  agent_loop: loop       # default: "loop"
  providers:             # default: ["anthropic"]
    - anthropic
extensions:
  - bash-tool
```

**Provider environment variables:**
- `ANTHROPIC_API_KEY` — required for Anthropic provider (default model: `claude-sonnet-4-20250514`, override with `ANTHROPIC_MODEL`)
- `OPENAI_API_KEY` — required for OpenAI provider (default model: `gpt-4o`, override with `OPENAI_MODEL`)
- `ZAI_API_KEY` — required for Z.ai provider (default model: `glm-4`, override with `ZAI_MODEL`)

```yaml
core:
  agent_loop: loop       # default: "loop"
  providers:             # default: ["anthropic"]
    - anthropic
extensions:
  - bash-tool
```

## Design Reference

`docs/design.md` is **strong inspiration, not direct instruction**. It captures the architectural intent and data flow, but implementation details will evolve. Treat it as a north star, not a spec to copy verbatim.
