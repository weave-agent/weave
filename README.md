<p align="center">
  <img src="assets/logos/weave-monochrome.svg" alt="Weave logo" width="160">
</p>

# Weave

[![build](https://github.com/weave-agent/weave/actions/workflows/ci.yml/badge.svg)](https://github.com/weave-agent/weave/actions/workflows/ci.yml) [![Coverage Status](https://coveralls.io/repos/github/weave-agent/weave/badge.svg?branch=main)](https://coveralls.io/github/weave-agent/weave?branch=main) [![Go Report Card](https://goreportcard.com/badge/github.com/weave-agent/weave)](https://goreportcard.com/report/github.com/weave-agent/weave)

A lightweight, extensible coding agent written in Go. Minimal dependencies, standard library as much as possible, with every replaceable component built as an independent extension.

Most coding agents are monoliths — swapping the LLM provider, adding a tool, or replacing the TUI means forking the whole project. Weave makes every component an independent Go module with zero coupling between them. Extensions self-register via `init()`, communicate through a shared event bus, and get compiled into a single static binary on first run. Add an extension, restart, and it's there.

## Features

- **Everything is an extension** — agent loop, LLM providers, tools, and TUI are all independent modules
- **Event bus architecture** — extensions communicate through events, never import each other directly
- **Dynamic compilation** — extensions are discovered, compiled into a cached binary, and exec'd on launch
- **First-run bootstrap** — core extensions clone automatically from separate repos on first use
- **JSON configuration** — layered settings (global → local), with env var and CLI flag overrides
- **Declarative auth** — providers declare credential structs with `json`/`env` tags, framework handles the rest
- **Multiple providers** — OpenAI, Claude, and any OpenAI-compatible API out of the box
- **Sandboxed execution** — macOS Seatbelt, Linux bubblewrap, or configurable deny lists
- **Context compaction** — auto-triggers when token budget is exceeded, with configurable reserve
- **Session resume** — continue the last session or resume by ID

## Installation

### Homebrew (macOS / Linux)

```bash
brew tap weave-agent/tap
brew install weave-agent/tap/weave
```

### From source

```bash
go install github.com/weave-agent/weave/cmd/weave@latest
```

### From releases

Download the appropriate binary from [releases](https://github.com/weave-agent/weave/releases).

## How It Works

1. **Resolve config** — load settings from `.weave/settings.json` (walked up from cwd), fallback `~/.weave/settings.json`.
2. **Bootstrap** — on first run, clone core extensions into `~/.weave/extensions/`. Skip with `--skip-bootstrap`.
3. **Discover** — scan for extensions in project and home directories.
4. **Build** — generate a custom binary with blank imports for all discovered extensions, cached by hash so unchanged configurations start instantly.
5. **Exec** — run the compiled binary. Extensions self-register via `init()` and wire up through the event bus.

## Extension Management

```bash
weave install <source>              # git URL, GitHub shorthand, or local path
weave install <source> --name foo
weave list                          # name, source, module path, status
weave update [<name>]               # git pull --ff-only; no args = all
weave uninstall <name>
```

Use `/reload` at runtime to invalidate the cache, rebuild, and re-exec.

## Configuration

JSON-only. Main config: `.weave/settings.json` (walked up from cwd), fallback `~/.weave/settings.json`. Layered settings merge global then local. Auth stored separately in `~/.weave/auth.json`.

Config structs use tags: `json`, `default`, `env`, `flag`, `short`, `validate`, `description`. The loader applies: defaults → JSON → env vars → CLI flags → validation.

### Key Settings

| Setting | Description |
|---|---|
| Provider | Active LLM provider |
| Thinking level | `off`, `minimal`, `low`, `medium` (default), `high`, `xhigh` |
| Sandbox mode | `off`, `readonly`, `ask`, `auto` (default) |
| Step limit | Max tool calls per turn (default: 50) |

### Environment Variables

| Variable | Description |
|---|---|
| `WEAVE_PROVIDER` | Override active provider |
| `WEAVE_THINKING_LEVEL` | Control reasoning depth |
| `WEAVE_OFFLINE` | Offline mode |

Provider env vars resolve without the `WEAVE_` prefix. Tools and extensions use `WEAVE_<NAME>`.

### Session Resume

```bash
weave --continue       # or -c, resume last session
weave --resume <id>    # or -r <id>, resume specific session
```

### Keybindings

`.weave/keybindings.yaml`. Built-in: Esc=interrupt, Ctrl+C=double-press exit, Ctrl+L=model select, Ctrl+P=model cycle, Ctrl+N=new session, Shift+Tab=cycle thinking, Ctrl+S=cycle sandbox, Ctrl+O=expand output, Ctrl+G=external editor.

## Writing Extensions

Extensions are independent Go modules. Each one lives in its own repo with its own `go.mod`, owns a single concern, and self-registers via `init()`.

```go
package myextension

import "github.com/weave-agent/weave/sdk"

func init() {
    sdk.RegisterExtension("my-extension", NewMyExtension)
}
```

Register other types:

```go
sdk.RegisterProvider[MyConfig, MyAuth]("my-provider", NewProvider)
sdk.RegisterTool[MyConfig]("my-tool", NewTool)
sdk.RegisterUIExtension("my-ui", NewUI)
sdk.RegisterTUIExtension("my-tui", NewTUI)
```

### Rules

- Extensions communicate exclusively through **bus events** — never import or call each other directly
- `internal/` packages are not importable by extensions; anything extensions need must live in `sdk/`
- Event payload types must live in `sdk/`
- Never write to `stdout`/`stderr` — use `sdk.Logger(name)` for structured logging to `~/.weave/logs/weave.log`

### Declarative Provider Auth

Providers declare an auth struct with `json` and `env` tags, then register with `sdk.RegisterProvider[TConfig, TAuth]`. The framework loads credentials from `~/.weave/auth.json` and environment variables.

<details>
<summary>Key Packages</summary>

| Package | Description |
|---|---|
| `sdk/` | Public API — `Extension`, `Bus`, `Config`, `UI`, `Provider`, `Tool` interfaces; global registries; `Logger(name)`; auth helpers |
| `sdk/model/` | Model types, model registry, `StreamOptions` |
| `sdk/registry/` | Generic `Registry[T]` used by all registries |
| `sdk/retry/` | Shared retry with exponential backoff |
| `sdk/validate/` | JSON schema validator for tool arguments |
| `sdk/throttle.go` | Context-aware throttling helper for event streaming |
| `sdk/tool_events.go` | Tool progress events for streaming output over the bus |
| `bus/` | Callback-based event bus (`Publish`/`On`/`OnAll`/`Off`) with per-handler goroutines and panic recovery |
| `settings/` | JSON config system with `Loader` (defaults → JSON → env → CLI flags → validation) |
| `internal/wire/` | Composition root: `WireExtensions()`, `WireWithCore()`, `Run()` |
| `internal/launcher/` | Auto-discovery, hash-based caching, binary generation |
| `internal/auth/` | Provider credential storage (`~/.weave/auth.json`) |
| `internal/extmanage/` | Extension lifecycle and first-run bootstrap |
| `internal/log/` | Rotating file logging via `slog` |
| `internal/filemut/` | Per-file mutex for serializing concurrent edits |
| `internal/filetracker/` | Read-before-edit policy enforcement |
| `utils/openaicompat/` | Shared SSE parsing for OpenAI-compatible providers |
| `utils/ripgrep/` | Ripgrep binary detection |
| `utils/truncate/` | Output truncation (2000 lines / 50KB) |

</details>

## Development

```bash
make tools    # install dev tools (moq, golangci-lint)
make gen      # regenerate mocks (run after changing interfaces)
make fmt      # format code (gofumpt, goimports, go fix)
make fix      # auto-fix linter issues
make lint     # run golangci-lint
make test     # run root module tests
make bench    # run build benchmarks
make tidy     # run go mod tidy
```

### Testing

- **Assertions**: `github.com/stretchr/testify` — `require` for fatal, `assert` for non-fatal
- **Mocks**: moq-generated exclusively. Run `make gen` after changing interfaces. Mocks live in `*_mock_test.go` — never edit by hand

## Requirements

- Go 1.26+
- ripgrep (for search tools)

## License

MIT License - see [LICENSE](LICENSE) file for details.
