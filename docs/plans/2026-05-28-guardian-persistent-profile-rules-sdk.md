# Guardian Persistent Profile Rules SDK Support

## Overview
- Add SDK and settings support required for Guardian to save approval-derived profile rules to the active configuration layer.
- Keep persistence details inside the settings package while exposing a narrow extension-facing writer API.
- Add a shared rule-scope value to Guardian approval resolutions so UI extensions can request exact-file, directory, project, command, host, or broad action rules without knowing Guardian persistence internals.

## Context (from discovery)
- Files/components involved:
  - `sdk/config.go` currently exposes `PreferenceWriter` with `SavePreferences`, which writes global settings only through `settings.FullConfig`.
  - `sdk/registry.go` already has `RegisterExtensionWithScopeAndWriter`, so privileged extensions can receive writer access.
  - `sdk/guardian.go` defines Guardian approvals, resolutions, profile rules, grants, snapshots, and topics.
  - `settings/config.go` already has `FullConfig.ExtensionConfig`, active-source resolution, default population, and deep merge helpers.
  - `settings/settings.go` has `saveSettingsMu` and layer-aware `SaveSettings` helpers.
- Related patterns found:
  - Singleton scopes such as `guardian`, `sandbox`, and `jsonl` are loaded directly from root settings keys.
  - Named scopes such as `tools`, `providers`, and `extensions` are loaded under `scope.name`.
  - Default population writes to `.weave/settings.local.json` when present, otherwise project `.weave/settings.json`, otherwise global `~/.weave/settings.json`.
  - SDK tests use `testify` and avoid raw `t.Error`/`t.Fatal`.
- Dependencies identified:
  - `weave-guardian` will consume the new active-layer scoped save API and new approval rule-scope field.
  - `weave-tui-guardian` will populate the new approval rule-scope field when publishing profile-scope resolutions.

## Development Approach
- **Testing approach**: Regular (code first, then tests in the same task)
- Complete each task fully before moving to the next.
- Make small, focused changes.
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task.
- **CRITICAL: all tests must pass before starting next task**.
- **CRITICAL: update this plan file when scope changes during implementation**.
- Run tests after each change.
- Maintain backward compatibility by adding optional interfaces rather than widening `PreferenceWriter` unless unavoidable.

## Testing Strategy
- **Unit tests**: required for every task.
- **E2E tests**: not applicable in this repo for this SDK/settings change.
- Use `make fmt`, `make test`, and targeted `go test ./sdk ./settings` during development.

## Progress Tracking
- Mark completed items with `[x]` immediately when done.
- Add newly discovered tasks with ➕ prefix.
- Document issues/blockers with ⚠️ prefix.
- Update plan if implementation deviates from original scope.
- Keep plan in sync with actual work done.

## What Goes Where
- **Implementation Steps** (`[ ]` checkboxes): tasks achievable within this codebase - code changes, tests, documentation updates.
- **Post-Completion** (no checkboxes): items requiring external action - manual testing, changes in consuming projects, deployment configs, third-party verifications.
- **Checkbox placement**: Checkboxes belong only in Task sections.

## Implementation Steps

### Task 1: Add shared Guardian profile rule scope to SDK
- [x] add `GuardianProfileRuleScope` type and constants in `sdk/guardian.go` for exact file, directory, project, exact command, command prefix, command family, network host, and action type scopes
- [x] add `RuleScope GuardianProfileRuleScope` to `sdk.GuardianResolution` with an `omitempty` JSON tag
- [x] update SDK tests in `sdk/guardian_test.go` for JSON compatibility and topic/type expectations
- [x] write tests for empty/default rule scope behavior
- [x] run `go test ./sdk` - must pass before next task

### Task 2: Add active-layer scoped config saving
- [x] add an optional SDK interface such as `ExtensionConfigWriter` with `SaveExtensionConfig(scope, name string, target any) error` in `sdk/config.go`
- [x] implement no-op behavior for `sdk.NoopPreferenceStore` if needed by tests and safe fallbacks
- [x] implement `(*settings.FullConfig).SaveExtensionConfig` in `settings/config.go` using `resolveSourcePath`, `saveSettingsMu`, `readSettingsMap`, deep merge, and existing write formatting
- [x] support singleton scopes (`guardian`, `sandbox`, `ui`, `jsonl`) and named scopes (`tools`, `providers`, `extensions`, `ui_extensions`) consistently with `ExtensionConfig`
- [x] reject unknown scopes with clear errors
- [x] write settings tests for saving singleton guardian config to local, project, and global active source paths
- [x] write settings tests for preserving unrelated fields and deep-merging nested profile rules
- [x] write settings tests for named-scope save behavior and unknown-scope errors
- [x] run `go test ./settings ./sdk` - must pass before next task

### Task 3: Verify acceptance criteria
- [x] verify SDK additions do not break existing read-only extension registration behavior
- [x] verify scoped config saving follows current default-population target selection rules
- [x] verify persisted JSON preserves unrelated settings and unknown fields where possible
- [x] run full root test suite with `make test`
- [x] run linter with `make lint` and fix all issues

### Task 4: Update documentation
- [ ] update `sdk/guardian.go` type comments if new exported values require clarification
- [ ] update project context docs if a new scoped writer API becomes a framework pattern

## Technical Details
- Prefer an optional writer interface instead of adding methods directly to `PreferenceWriter` to avoid breaking existing implementations.
- `SaveExtensionConfig` should accept the scoped subtree, not the full settings object.
- For `guardian` scope, callers pass a guardian config struct and the settings package writes it under the root `guardian` key.
- For named scopes, callers pass the config struct for the named item and the settings package writes it under `scope.name`.
- `GuardianResolution.RuleScope` is intent metadata only; Guardian remains responsible for converting that intent into safe, normalized persisted policy constraints.

## Post-Completion

**External repo follow-up**:
- Update `weave-guardian` to use `sdk.ExtensionConfigWriter` and `GuardianResolution.RuleScope`.
- Update `weave-tui-guardian` to populate `GuardianResolution.RuleScope` from a second approval dialog.
