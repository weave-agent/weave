# Pi Extension Runtime Migration - Root SDK and Launcher

## Overview
- Add the root SDK/runtime contracts required for 100% Pi capability parity by rewrite.
- Keep the existing bus and static registries as compatibility foundations while introducing typed hooks, runtime services, and stable extension context APIs.
- This repo is the dependency root for all extension repos.

## Context (from discovery)
- Files/components involved: `sdk/extension.go`, `sdk/tool.go`, `sdk/provider.go`, `sdk/session.go`, `sdk/model`, `sdk/guardian.go`, `sdk/sandbox.go`, `internal/wire`, `internal/launcher`, `settings`.
- Related patterns found: extensions currently self-register through `init()`, communicate over the bus, and use typed config schemas.
- Dependencies identified: all extension repos consume this SDK; agent/tui/jsonl/bash/guardian/sandbox are the first downstream adopters.

## Compatibility Context
Backward compatibility means existing extensions must continue to compile and run while the new runtime is introduced. The first root change should add adapters, not force every extension repo to migrate in the same commit.

Existing extension style that must keep working:

```go
func init() {
	sdk.RegisterTool[Config]("read", func(cfg sdk.Config, prefs sdk.PreferenceReader, toolCfg Config) (sdk.Tool, error) {
		return &Tool{cfg: toolCfg}, nil
	})
}

type Tool struct{}

func (t *Tool) Name() string { return "read" }
func (t *Tool) Definition() sdk.ToolDef { return sdk.ToolDef{Name: "read"} }
func (t *Tool) Execute(ctx context.Context, args map[string]any) (sdk.ToolResult, error) {
	return sdk.ToolResult{Content: "ok"}, nil
}
```

New runtime-aware style should be additive:

```go
type Extension struct{}

func (e *Extension) Register(ctx sdk.ExtensionContext) error {
	ctx.Hooks().ToolCall().Use("my-ext", func(ctx context.Context, call sdk.ToolCallRequest) (sdk.ToolCallResult, error) {
		return sdk.ToolCallResult{Continue: true, Call: call}, nil
	})

	ctx.Tools().Register(sdk.RuntimeTool{
		Name: "my_tool",
		Run: func(ctx context.Context, req sdk.ToolRequest) (sdk.ToolResult, error) {
			return sdk.ToolResult{Content: "ok"}, nil
		},
	})

	return nil
}
```

Concrete compatibility requirements:
- Existing `RegisterExtension`, `RegisterTool`, `RegisterProvider`, `RegisterUIExtension`, and model registration calls keep their public signatures.
- Existing `sdk.Extension.Subscribe(bus)`, `sdk.Tool.Execute(ctx,args)`, and `sdk.Provider.Stream(ctx,req,opts...)` implementations are adapted into runtime services.
- Existing bus topics continue to publish for observers even when typed hooks become the source of truth.
- Existing config schema/default/env/flag loading continues to work for both old and new registration styles.
- Existing launcher generated blank imports still link extension packages so their `init()` registration runs.

## API Addition Examples
These examples are directional sketches, not final API decisions. Prefer small, deep interfaces that hide ordering, cleanup, and bus-bridging details.

Typed hook primitive:

```go
type Hook[TReq any, TRes any] interface {
	Use(owner string, fn HookFunc[TReq, TRes], opts ...HookOption) HookHandle
	Run(ctx context.Context, req TReq) (TRes, error)
}

type HookHandle interface {
	Close() error
}
```

Runtime context:

```go
type ExtensionContext interface {
	Bus() Bus
	Hooks() Hooks
	Tools() ToolRegistry
	Providers() ProviderRegistry
	Session() SessionController
	Resources() ResourceRegistry
	Models() ModelController
	Exec(ctx context.Context, req ExecRequest) (ExecResult, error)
	Config(scope, name string, target any) error
}
```

Session controller examples:

```go
type SessionController interface {
	SendUserMessage(ctx context.Context, content any) error
	AppendEntry(ctx context.Context, entry SessionEntry) (string, error)
	SetName(ctx context.Context, name string) error
	Name(ctx context.Context) (string, error)
	SetLabel(ctx context.Context, entryID string, label string) error
	Compact(ctx context.Context, req CompactRequest) (CompactResult, error)
}
```

Provider middleware examples:

```go
type ProviderMiddleware interface {
	BeforeProviderRequest(context.Context, ProviderRequest) (ProviderRequest, error)
	AfterProviderResponse(context.Context, ProviderResponse) error
}
```

Exec service examples:

```go
type ExecRequest struct {
	Command string
	Args    []string
	Dir     string
	Env     []string
	Reason  string
}

type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}
```

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
  - tests are not optional - they are a required part of the checklist
  - write unit tests for new functions/methods
  - write unit tests for modified functions/methods
  - add new test cases for new code paths
  - update existing test cases if behavior changes
  - tests cover both success and error scenarios
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility while the migration is staged across repos

## Testing Strategy
- **Unit tests**: required for every task that changes code
- **Integration tests**: required for runtime registration, hook execution, and bus compatibility
- **E2E tests**: add/update if the repo already has UI or launcher-level e2e coverage

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope
- Keep plan in sync with actual work done

## What Goes Where
- **Implementation Steps** (`[ ]` checkboxes): tasks achievable within this codebase - code changes, tests, documentation updates
- **Post-Completion** (no checkboxes): items requiring external action - manual testing, changes in consuming projects, deployment configs, third-party verifications
- **Checkbox placement**: Checkboxes belong only in Task sections. Do not put checkboxes in Success criteria, Overview, or Context.

## Implementation Steps

### Task 1: Add typed runtime hook primitives to `sdk`
- [x] add generic hook/interceptor types for ordered handlers with typed request/result payloads; example hook families include input, prompt, context, provider request/response, tool call/result, message, turn, and session lifecycle
- [x] add hook registration handles that support deterministic removal and close cleanup; handlers registered by an extension should be removable when the extension closes
- [x] add compatibility helpers for publishing observer events on the existing bus; example: `ToolCall` hook execution still emits the current tool-call bus topic for observers
- [x] write tests for ordering, mutation, veto, error propagation, and unregister behavior
- [x] run `go test ./sdk/...` - must pass before task 2

### Task 2: Introduce extension runtime context
- [x] add `sdk.ExtensionContext` with access to hooks, tools, session, models, resources, exec, config, and bus services; keep TUI-specific framework types out of this root interface
- [x] add backward-compatible registration paths for existing `sdk.Extension` implementations; old `Subscribe(bus)` extensions should be wrapped as observer-only runtime extensions
- [x] update extension registry and schema registration to expose runtime-aware factories without breaking existing factories; old and new factories should share config default population
- [x] write tests for legacy extension registration and runtime-context extension registration
- [x] run `go test ./sdk/... ./internal/...` - must pass before task 3

### Task 3: Add runtime tool and provider registry contracts
- [x] add session-scoped runtime tool registry interfaces in `sdk`; examples include register, unregister, list, get, enable, disable, and decorate existing tools
- [x] add provider middleware contracts for before-request, after-response, and stream observation; middleware belongs in runtime/agent layers, not provider implementations
- [x] add compatibility adapters for existing static `RegisterTool` and `RegisterProvider` APIs; static tools/providers should appear in runtime registries automatically
- [x] write tests for runtime registration, duplicate names, activation filters, and static compatibility
- [x] run `go test ./sdk/...` - must pass before task 4

### Task 4: Add session, resource, model, and exec service contracts
- [ ] add session controller interfaces for send message, append entry, set name, get name, set label, compact, fork, switch, and tree operations; unsupported operations should return typed errors
- [ ] add resource discovery interfaces for prompts, skills, context, themes, and embedded assets; resource providers should be deterministic and failure-isolated
- [ ] add model/thinking controller interfaces over existing model registry and thinking-level behavior; preserve current model selection events and preference writes
- [ ] add guardian/sandbox-aware exec request/result contracts without embedding shell implementation details; root SDK defines the port, bash/sandbox/guardian provide adapters
- [ ] write tests for zero-value/noop service behavior and service lookup failures
- [ ] run `go test ./sdk/...` - must pass before task 5

### Task 5: Wire runtime services through launcher and composition root
- [ ] update `internal/wire` to construct and pass runtime services to extensions
- [ ] update launcher-generated imports/wiring only as needed for new registration shapes
- [ ] preserve first-run bootstrap, headless filtering, and cache hash behavior
- [ ] write tests for generated wiring compatibility and cache-key stability where affected
- [ ] run `go test ./internal/... ./sdk/...` - must pass before task 6

### Task 6: Maintain event bus compatibility layer
- [ ] map new typed lifecycle hooks to existing bus topics for observers
- [ ] keep existing bus topics stable unless a plan explicitly migrates a repo away from them
- [ ] add deprecation notes only where a typed replacement exists
- [ ] write tests proving existing bus subscribers still receive expected events
- [ ] run `go test ./...` - must pass before task 7

### Task 7: Verify acceptance criteria
- [ ] verify this repo implements its part of the Pi parity migration
- [ ] verify backward compatibility for existing public APIs where required
- [ ] run `go test ./...`
- [ ] run repo lint command if available
- [ ] update this plan with any remaining blockers or follow-up tasks

### Task 8: Update documentation
- [ ] update README.md or package docs if public API changed
- [ ] update examples or usage snippets if registration/runtime behavior changed
- [ ] document migration notes for extension authors if applicable

## Technical Details
- New runtime APIs should be deep modules: simple caller-facing interfaces hiding hook ordering, cleanup, bus bridging, and compatibility adapters.
- Bus remains the notification mechanism; hooks/services become the behavior-changing mechanism.
- New contracts must avoid importing extension-specific packages or TUI framework types into root SDK.
- Runtime API additions should model stable capabilities, not Pi method names directly. For example, Pi `context` maps to a Weave context/resource contributor, and Pi `tool_call` maps to a typed tool-call interceptor.
- Prefer adapters over duplicate execution paths: old static tools/providers should be lifted into runtime registries rather than maintained as a separate code path.
- Use typed errors for unsupported runtime capabilities so downstream repos can degrade clearly during staged migration.

## Post-Completion
*Items requiring manual intervention or external systems - no checkboxes, informational only*

**Manual verification**:
- Run a normal `weave` launch with existing extensions before any downstream repo is migrated.
- Verify first-run bootstrap and `weave cache clean` still behave as expected.

**External system updates**:
- Downstream extension repos must update `github.com/weave-agent/weave` dependency after this lands.
