# Guardian and Sandbox Redesign Umbrella

## Overview
Replace the current sandbox-mode model with a guardian-led safety architecture. The root module will define typed SDK contracts, settings shapes, bootstrap metadata, and event topics for a new `guardian` extension, rewritten `sandbox` extension, and `tui-guardian` UI extension. Backward compatibility with `off`, `readonly`, `ask`, and `auto` sandbox modes is intentionally dropped.

The new model separates policy from containment:

- `guardian` decides whether a tool action may run: `allow`, `ask`, or `block`.
- `sandbox` constrains approved bash processes with OS-level boundaries and supports expansion requests.
- `tui-guardian` presents approvals, expansion prompts, profile controls, and decision history.

## Context (from discovery)
- Files/components involved:
  - `sdk/sandbox.go`
  - new `sdk/guardian.go`
  - `settings/config.go`
  - `settings/settings.go`
  - `settings/merge.go`
  - `settings/help.go`
  - `internal/extmanage/bootstrap.go`
  - `internal/extmanage/bootstrap_test.go`
  - launcher generated binary flag propagation in `internal/launcher/builder.go`
- Related patterns found:
  - Extensions register through SDK registries and communicate over bus events.
  - Tools subscribe with `sdk.OnBusReady` before extensions publish registration events.
  - Existing sandbox API mixes approval, file policy, and command wrapping.
  - Bootstrap currently installs `sandbox` and `tui-sandbox` but not `guardian` or `tui-guardian`.
- Dependencies identified:
  - Core extension repos under `~/.weave/extensions/`.
  - `make gen` after SDK interface changes.
  - Root tests and independent extension tests.

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next.
- Make small, focused changes.
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task.
- **CRITICAL: all tests must pass before starting next task**.
- **CRITICAL: update this plan file when scope changes during implementation**.
- Run tests after each change.
- Backward compatibility with old sandbox modes is not required.

## Testing Strategy
- **Unit tests**: required for every task.
- Root SDK/settings/bootstrap tests must cover new contracts and removed old mode assumptions.
- Extension-specific behavior is tested in each extension repo plan.

## Progress Tracking
- Mark completed items with `[x]` immediately when done.
- Add newly discovered tasks with ➕ prefix.
- Document issues/blockers with ⚠️ prefix.
- Update plan if implementation deviates from original scope.
- Keep plan in sync with actual work done.

## What Goes Where
- **Implementation Steps** (`[ ]` checkboxes): tasks achievable within this codebase.
- **Post-Completion**: manual or external checks only.
- **Checkbox placement**: Checkboxes belong only in Task sections.

## Implementation Steps

### Task 1: Add guardian SDK contracts
- [ ] create `sdk/guardian.go` with `Guardian`, request, decision, approval, profile, grant, and snapshot types
- [ ] add typed guardian event topic constants for registration, decisions, approval requests, resolutions, profile changes, snapshots, and grant clearing
- [ ] add `//go:generate moq` directive for `Guardian`
- [ ] write SDK tests for guardian mock generation and event topic constants
- [ ] run `make gen` and `go test ./sdk/...` - must pass before task 2

### Task 2: Replace sandbox SDK contract with containment-only API
- [ ] update `sdk/sandbox.go` to remove mode, file read/write, and string approval concepts
- [ ] define containment-only `Sandboxer` API with command wrapping, status, and expansion-related types
- [ ] add typed sandbox event topic constants for registration, status, expansion request, and expansion resolution
- [ ] update existing sandbox SDK tests for the new interface
- [ ] run `make gen` and `go test ./sdk/...` - must pass before task 3

### Task 3: Add guardian and sandbox configuration shapes
- [ ] add `GuardianFileConfig` with `profile`, `ask_fallback`, and custom profile map fields in `settings/config.go`
- [ ] replace old sandbox file config fields with containment settings: `enabled`, `fail_if_unavailable`, `allow_unsandboxed_fallback`, filesystem, and network
- [ ] update settings merge behavior for guardian profiles and new sandbox settings
- [ ] write settings tests for JSON loading, layering, env/CLI overrides, and generated config propagation
- [ ] run `go test ./settings/...` - must pass before task 4

### Task 4: Update launcher and bootstrap metadata
- [ ] update core extension bootstrap list to include `guardian` and `tui-guardian`
- [ ] remove `tui-sandbox` from the core bootstrap list
- [ ] update bootstrap tests for the new core extension set
- [ ] update launcher generated argument/env propagation for guardian profile and sandbox settings if needed
- [ ] run `go test ./internal/extmanage ./internal/launcher ./internal/wire` - must pass before task 5

### Task 5: Remove root documentation references to old sandbox modes
- [ ] update project context/docs that mention `off`, `readonly`, `ask`, and `auto` sandbox modes
- [ ] document the new guardian profiles: `ask`, `auto`, `yolo`, and custom profiles
- [ ] document that sandbox is containment-only and expansions are handled through guardian UI
- [ ] update help output tests if help text changes
- [ ] run `make test` - must pass before task 6

### Task 6: Verify root acceptance criteria
- [ ] verify SDK has no remaining `AllowRead`, `AllowWrite`, `Mode`, or `SetMode` sandbox contract requirements
- [ ] verify settings no longer expose old sandbox mode semantics
- [ ] verify bootstrap includes `guardian` and `tui-guardian` and excludes `tui-sandbox`
- [ ] run `make lint` - all issues must be fixed
- [ ] run `make test` - must pass

## Technical Details

### Guardian decision model
```go
type Guardian interface {
    Decide(ctx context.Context, req GuardianRequest) (GuardianDecision, error)
    Resolve(ctx context.Context, decisionID string, resolution GuardianResolution) error
    Snapshot(ctx context.Context) (GuardianSnapshot, error)
}
```

### Built-in profiles
- `ask`: reads and harmless metadata run automatically; writes, deletes, network, and unknown actions ask; hard-danger actions block.
- `auto`: normal coding actions run automatically; risky/destructive/unknown actions ask; hard-danger actions block.
- `yolo`: almost everything runs, with catastrophic circuit breakers retained.

### Event model
All approval and expansion flows are ID-based. Matching by command string is forbidden.

## Post-Completion

**Manual verification**:
- Start Weave after clearing extension cache and verify bootstrap expects `guardian`, `sandbox`, and `tui-guardian`.
- Confirm old `--sandbox readonly` style behavior is not documented as supported.

**External system updates**:
- New `github.com/weave-agent/weave-guardian` and `github.com/weave-agent/weave-tui-guardian` repositories need to be created upstream before bootstrap works from GitHub.
