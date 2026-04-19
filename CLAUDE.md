# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A coding agent framework written in Go — event-driven, extension-based, with dynamic compilation of selected extensions at runtime. Core framework implemented with agent-loop extension; provider and tool extensions are next.

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

## Architecture

Standard library as much as possible. Every replaceable component is an extension (runner, provider, tools, store, hooks). Extensions are independent Go modules that self-register via `init()`.

**Launcher pattern:** resolve config → pick extensions → build a custom binary (cached per hash) → exec it. The `cmd/weave/main.go` entry point orchestrates this pipeline.

**Key packages:**
- `sdk/` — defines `Extension`, `Bus`, `Config` interfaces; global registries for extensions, providers, and tools (`RegisterExtension`/`GetExtension`, `RegisterProvider`/`GetProvider`, `RegisterTool`/`GetTool`); `Message` types; `Wire()` and `WireWithCore()` composition roots that resolve names and subscribe extensions to the bus
- `bus/` — channel-based pub/sub event bus (`Publish`/`Subscribe`/`SubscribeAll`) with buffered channels and graceful close
- `config/` — config discovery (walks up from cwd for `.weave.yaml` or `.weave/config.yaml`) and loading via gonfig. Config has a `core` section (agent_loop + providers) and `extensions` list.
- `extensions/agent-loop/` — core extension implementing the two-level while-loop agent cycle (outer: follow-ups, inner: steering + tool calls); subscribes to `agent.prompt`, `agent.steer`, `agent.followup`; publishes `agent.turn_start/end`, `agent.message_start/update/end`, `agent.tool_result`, `agent.end`
- `launcher/` — full pipeline: `Discover` extensions (project-local `.weave/extensions/{name}/` then global `~/.weave/extensions/{name}/`), `ComputeHash` of .go files, `Cache` in `~/.weave/bin/{hash}/`, `Build` by generating go.mod+main.go with blank imports, then `syscall.Exec`

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

## Design Reference

`docs/design.md` is **strong inspiration, not direct instruction**. It captures the architectural intent and data flow, but implementation details will evolve. Treat it as a north star, not a spec to copy verbatim.
