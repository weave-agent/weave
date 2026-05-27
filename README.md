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
- **Dynamic compilation** — extensions are discovered, compiled into a hash-cached binary, and exec'd on launch
- **First-run bootstrap** — core extensions clone automatically from separate repos on first use
- **JSON configuration** — layered settings (global → local), with env var and CLI flag overrides
- **Declarative auth** — providers declare credential structs with `json`/`env` tags, framework handles the rest
- **Multiple providers** — OpenAI, Claude, and any OpenAI-compatible API out of the box
- **Guardian policy + sandbox containment** — guardian approvals decide whether actions run; sandbox constrains approved shell commands
- **Context compaction** — auto-triggers when token budget is exceeded, with configurable reserve
- **Session resume** — continue the last session or resume by ID

## Installation

### Homebrew (macOS / Linux)

```bash
brew tap weave-agent/tap
brew install weave-agent/tap/weave
weave --version
```

The Homebrew formula installs `go` and `ripgrep` as dependencies.

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
4. **Build** — generate a custom binary with blank imports for discovered build inputs, cached by hash so unchanged configurations start instantly. Headless prompt runs exclude UI-only extensions from build inputs.
5. **Exec** — run the compiled binary. Extensions self-register via `init()` and wire up through the event bus.

Help and no-input error paths return before first-run bootstrap or launcher build work. Use `weave --help` or `weave -h` to print global launcher help without bootstrapping extensions or building a launcher binary. Extension-specific help is available from generated binaries that have imported those extensions.

## Extension Management

```bash
weave install <source>              # git URL, GitHub shorthand, or local path
weave install <source> --name foo
weave list                          # name, source, module path, status
weave update [<name>]               # git pull --ff-only; no args = all
weave uninstall <name>
weave cache clean                    # remove launcher binary cache entries
```

Generated launcher binaries live under `~/.weave/bin/<hash>/weave`. Cache keys include the Go runtime version, OS/arch, headless mode, agent loop, root module graph, extension Go files, embedded `//go:embed` resources, extension module files, selected core source directories, and local replace dependencies. The cache is capped at 1 GiB by default and evicts least-recently-used entries after successful stores; the newly stored entry is protected even if it exceeds the cap. `weave cache clean` removes only launcher binary cache entries under `~/.weave/bin`.

Use `/reload` at runtime to invalidate the current cache entry, rebuild, and re-exec.

## Configuration

JSON-only. Main config: `.weave/settings.json` (walked up from cwd), fallback `~/.weave/settings.json`. Layered settings merge global then local. Auth stored separately in `~/.weave/auth.json`.

Config structs use tags: `json`, `default`, `env`, `flag`, `short`, `validate`, `description`. The loader applies: defaults → JSON → env vars → CLI flags → validation.

When a registered extension, tool, provider, UI extension, or built-in scoped config is loaded, Weave writes missing `default` tag values into the selected settings file for discoverability. Existing user values are preserved. If `.weave/settings.local.json` exists, it is populated first; otherwise Weave writes to the active project `.weave/settings.json` or global `~/.weave/settings.json`.

### Key Settings

| Setting | Description |
|---|---|
| Provider | Active LLM provider |
| Thinking level | `off`, `minimal`, `low`, `medium` (default), `high`, `xhigh` |
| Guardian profile | `ask`, `auto`, `yolo`, or a custom profile |
| Sandbox containment | OS-level boundaries for approved shell commands |
| Step limit | Max tool calls per turn (default: 50) |

Guardian profiles control policy decisions. `ask` permits reads and harmless
metadata automatically while prompting for writes, network, deletes, and
unknown actions. `auto` permits normal coding actions and asks for risky or
unknown actions. `yolo` runs most actions while retaining hard catastrophic
blocks. Custom profiles live under `guardian.profiles` and can be selected with
`guardian.profile` or `--guardian-profile`.

Trusted extensions may apply runtime Guardian policy overlays through SDK
events. These overlays are session-runtime controls layered over the active
profile, are visible in Guardian snapshots for display and diagnostics, and do
not appear as selectable profiles.

Sandbox configuration is containment-only. It does not decide whether a tool
action is allowed; the guardian does that first. Once an approved shell command
is ready to run, the sandbox extension wraps it with filesystem and network
boundaries. Requests to expand those boundaries are surfaced through the
guardian UI extension for approval and history.

The old sandbox-mode API is removed. `--sandbox`, `--weave-sandbox-mode`,
`WEAVE_SANDBOX_MODE`, `sandbox.mode`, `sandbox.writable`,
`sandbox.deny_read`, and `sandbox.deny_write` are no longer supported. Use
`--guardian-profile` for policy selection and the containment settings below
for sandbox boundaries.

Guardian settings:

| Field | Type | Description |
|---|---|---|
| `guardian.profile` | string | Active profile: `ask`, `auto`, `yolo`, or a custom profile name |
| `guardian.ask_fallback` | bool | Ask instead of blocking when no policy matches |
| `guardian.profiles` | object | Custom profile definitions keyed by profile name |

Generated binaries also accept scoped CLI overrides for guardian settings,
including `--guardian-ask_fallback`. The launcher-level
`--guardian-profile` flag forwards the active profile into generated binaries.

Sandbox settings:

| Field | Type | Description |
|---|---|---|
| `sandbox.enabled` | bool | Enable OS-level containment for approved shell commands |
| `sandbox.fail_if_unavailable` | bool | Fail commands when sandbox containment is unavailable |
| `sandbox.allow_unsandboxed_fallback` | bool | Allow approved commands to run without containment when sandboxing is unavailable |
| `sandbox.filesystem.read_only` | string[] | Paths mounted read-only inside the sandbox |
| `sandbox.filesystem.read_write` | string[] | Paths mounted read-write inside the sandbox |
| `sandbox.filesystem.blocked` | string[] | Paths blocked inside the sandbox |
| `sandbox.network.enabled` | bool | Allow network access from sandboxed processes |
| `sandbox.network.allow_hosts` | string[] | Hosts allowed from sandboxed processes |
| `sandbox.network.allow_ports` | string[] | Ports allowed from sandboxed processes |
| `sandbox.network.block_hosts` | string[] | Hosts blocked from sandboxed processes |
| `sandbox.network.allow_listen` | bool | Allow sandboxed processes to listen on local ports |

Generated binaries also accept scoped CLI overrides for sandbox settings,
including `--sandbox-enabled`, `--sandbox-fail_if_unavailable`, and
`--sandbox-allow_unsandboxed_fallback`.

### Provider HTTP and Retry Configuration

Providers support per-provider HTTP transport and retry overrides through the `providers.defaults` and `providers.<name>` config keys.

#### HTTP Transport

Set under `providers.defaults.http` or `providers.<name>.http`:

| Field | Type | Default | Description |
|---|---|---|---|
| `dial_timeout` | duration string | `10s` | TCP dial timeout |
| `tls_handshake_timeout` | duration string | `10s` | TLS handshake timeout |
| `response_header_timeout` | duration string | `60s` | Wait for response headers |
| `idle_conn_timeout` | duration string | `90s` | Idle connection timeout |

#### Retry

Set under `providers.defaults.retry` or `providers.<name>.retry`:

| Field | Type | Default | Description |
|---|---|---|---|
| `max_retries` | int | `5` | Maximum retry attempts |
| `base_delay` | duration string | `1s` | Initial retry delay |
| `max_delay` | duration string | `30s` | Maximum retry delay |
| `multiplier` | float | `2.0` | Exponential backoff multiplier |
| `jitter` | string | `full` | Jitter mode: `full` or `none` |

#### Example

```json
{
  "providers": {
    "defaults": {
      "http": {
        "dial_timeout": "10s",
        "tls_handshake_timeout": "10s",
        "response_header_timeout": "60s",
        "idle_conn_timeout": "90s"
      },
      "retry": {
        "max_retries": 5,
        "base_delay": "1s",
        "max_delay": "30s",
        "multiplier": 2.0,
        "jitter": "full"
      }
    },
    "openai": {
      "http": {
        "response_header_timeout": "120s"
      },
      "retry": {
        "max_retries": 3,
        "jitter": "none"
      }
    }
  }
}
```

Provider-specific values override defaults. Unspecified fields inherit from defaults. Invalid duration strings or jitter values cause a clear initialization error.

Note: This pass does not support a per-stream idle timeout or a `max_elapsed` retry limit.

### Environment Variables

| Variable | Description |
|---|---|
| `WEAVE_PROVIDER` | Override active provider |
| `WEAVE_THINKING_LEVEL` | Control reasoning depth |
| `WEAVE_OFFLINE` | Offline mode |
| `WEAVE_GUARDIAN_PROFILE` | Override active guardian profile |
| `WEAVE_GUARDIAN_ASK_FALLBACK` | Override guardian ask fallback behavior |
| `WEAVE_SANDBOX_ENABLED` | Enable or disable sandbox containment |
| `WEAVE_SANDBOX_FAIL_IF_UNAVAILABLE` | Fail commands when containment is unavailable |
| `WEAVE_SANDBOX_ALLOW_UNSANDBOXED_FALLBACK` | Allow unsandboxed fallback when containment is unavailable |

Provider env vars resolve without the `WEAVE_` prefix. Tools and extensions use `WEAVE_<NAME>`.

### Session Resume

```bash
weave --continue       # or -c, resume last session
weave --resume <id>    # or -r <id>, resume specific session
```

### Keybindings

`.weave/keybindings.yaml`. Built-in: Esc=interrupt, Ctrl+C=double-press exit, Ctrl+L=model select, Ctrl+P=model cycle, Ctrl+N=new session, Shift+Tab=cycle thinking, Ctrl+S=cycle guardian profile, Ctrl+O=expand output, Ctrl+G=external editor.

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

Config structs registered through `sdk.RegisterExtension`, `sdk.RegisterTool`, `sdk.RegisterProvider`, or `sdk.RegisterUIExtension` should use `json`, `default`, `env`, `flag`, and `description` tags. `default` values are loaded at runtime and are also auto-written into settings files when absent. `sdk.GetSchemaInfo(scope, name)` returns the registered schema plus the original config `reflect.Type`; `sdk.GetSchema` remains available when only the schema is needed.

### Provider Context Accounting

Providers stream response usage through `sdk.ProviderUsage` on `sdk.ProviderEventUsage`.
Providers that can count a fully rendered request before streaming may also implement
`sdk.TokenCounter`:

```go
type TokenCounter interface {
	CountTokens(ctx context.Context, req ProviderRequest, opts ...model.StreamOption) (TokenCount, error)
}
```

Use `TokenCount.Source` to describe count quality: `TokenCountSourceExact` for
provider or canonical tokenizer counts, `TokenCountSourceTokenizer` for
compatible tokenizer estimates, and `TokenCountSourceHeuristic` for fallback
estimates. `sdk.NewContextBudgetSnapshot` provides provider-neutral budget
arithmetic; policy decisions such as compaction stay in the agent extension.

### Rules

- Extensions communicate exclusively through **bus events** — never import or call each other directly
- `internal/` packages are not importable by extensions; anything extensions need must live in `sdk/`
- Event payload types must live in `sdk/`
- Guardian policy integrations use `sdk.Guardian` and guardian event topics; sandbox containment integrations use `sdk.Sandboxer` and sandbox event topics
- Guardian policy overlays are runtime-only policy layers for trusted extensions. Publish `sdk.GuardianPolicyOverlay` on `guardian.policy.overlay.push` to add or replace an overlay, and `sdk.GuardianPolicyOverlayPop` on `guardian.policy.overlay.pop` to remove one by ID. Active overlays are exposed through `GuardianSnapshot.Overlays`. Overlays do not create user-visible profiles; `OverrideHardBlocks` is an explicit trusted-extension contract, not a security boundary
- Provider context accounting should use shared SDK contracts: providers stream response totals with `sdk.ProviderUsage`, may optionally implement `sdk.TokenCounter` for preflight request counts, and should expose provider-neutral budget math through `sdk.ContextBudgetSnapshot`
- Approval and sandbox expansion flows are ID-based; do not match requests or resolutions by command string
- `sdk.Guardian` exposes `Decide`, `Resolve`, and `Snapshot`; key topics include `guardian.registered`, `guardian.approval.request`, `guardian.approval.resolution`, and `guardian.profile.change`
- `sdk.Sandboxer` exposes `WrapCommand`, `Status`, `RequestExpansion`, and `ResolveExpansion`; key topics include `sandbox.registered`, `sandbox.status`, `sandbox.expansion.request`, and `sandbox.expansion.resolution`
- Non-Go resource files only invalidate the launcher cache when referenced by `//go:embed`; unembedded `.md` files and assets do not affect generated binary cache keys
- Never write to `stdout`/`stderr` — use `sdk.Logger(name)` for structured logging to `~/.weave/logs/weave.log`

### Declarative Provider Auth

Providers declare an auth struct with `json` and `env` tags, then register with `sdk.RegisterProvider[TConfig, TAuth]`. The framework loads credentials from `~/.weave/auth.json` and environment variables.

<details>
<summary>Key Packages</summary>

| Package | Description |
|---|---|
| `sdk/` | Public API — `Extension`, `Bus`, `Config`, `UI`, `Provider`, `Tool`, `Guardian`, `Sandboxer` interfaces; optional `TokenCounter`; provider usage, token count, and context budget accounting types; global registries and schema metadata; guardian/sandbox event topics; `Logger(name)`; auth helpers |
| `sdk/model/` | Model types, model registry, `StreamOptions` |
| `sdk/registry/` | Generic `Registry[T]` used by all registries |
| `sdk/providerhttp/` | Provider HTTP transport config and client factory |
| `sdk/providerretry/` | Provider retry config resolver with deep-merge |
| `sdk/retry/` | Shared retry with exponential backoff and jitter support |
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
| `utils/openaicompat/` | Shared SSE parsing for OpenAI-compatible providers, including usage chunks and cached prompt token mapping to `sdk.ProviderUsage.CacheReadTokens` |
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
make bench    # run launcher/build benchmarks
make tidy     # run go mod tidy
```

Launcher benchmarks live in `internal/launcher/benchmark_test.go`. They cover cache-hit startup paths for no, one, and many extensions, and report phase metrics for discovery, hash computation, generated files, `go mod tidy`, `go build`, and cache store. For a quick sanity pass, run:

```bash
go test ./internal/launcher -run '^$' -bench 'Benchmark' -benchtime=1x
```

### Testing

- **Assertions**: `github.com/stretchr/testify` — `require` for fatal, `assert` for non-fatal
- **Mocks**: moq-generated exclusively. Run `make gen` after changing interfaces. Mocks live in `*_mock_test.go` — never edit by hand

## Requirements

- Go 1.26+
- ripgrep (installed automatically by Homebrew)

## License

MIT License - see [LICENSE](LICENSE) file for details.
