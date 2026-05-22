# Provider Resilience SDK Support

## Overview
Add shared SDK support for provider HTTP transport configuration, provider retry configuration, jittered retry backoff, OpenAI-compatible retry plumbing, and capped provider error-body reads. This creates a stable extension-facing foundation that provider repos can adopt without duplicating config parsing or transport setup.

The first pass configures transport deadlines only. It intentionally does not add total stream timeout or stream-idle timeout.

## Context (from discovery)
- Files/components involved:
  - `sdk/retry/retry.go`
  - new `sdk/providerhttp` package
  - new `sdk/providerretry` package
  - `utils/openaicompat/openai_compat.go`
  - `settings/config.go` and `settings.FullConfig.ExtensionConfig`
- Related patterns found:
  - Provider configs are loaded through `cfg.ExtensionConfig("providers", name, &target)`
  - Provider env vars use exact env tags without `WEAVE_` prefix
  - `sdk/retry` currently has deterministic exponential backoff without jitter
  - `utils/openaicompat` currently owns package-global retry defaults and reads non-OK response bodies fully
- Dependencies identified:
  - `net/http`
  - `net`
  - `time`
  - existing `sdk.Config`
  - existing `sdk/retry`

## Development Approach
- **Testing approach**: Regular
- Complete each task fully before moving to the next
- Make small, focused changes
- Every task that changes code includes new or updated tests
- All tests must pass before starting the next task
- Update this plan file when scope changes during implementation
- Maintain backward compatibility where possible

## Testing Strategy
- Unit tests for `sdk/providerhttp` config merge, duration parsing, validation, and generated transport settings
- Unit tests for `sdk/providerretry` config merge, duration parsing, validation, and mapping into `sdk/retry.Config`
- Unit tests for `sdk/retry` jitter behavior
- Unit tests for `utils/openaicompat` explicit retry config and 64 KiB error-body cap
- Run `go test ./sdk/retry ./sdk/providerhttp ./sdk/providerretry ./utils/openaicompat` after focused changes
- Run `go test ./sdk/... ./utils/openaicompat` before finalizing

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Keep plan in sync with implementation

## What Goes Where
- Implementation Steps contain automatable code/test/doc tasks
- Post-Completion contains manual verification and external coordination
- Checkboxes belong only in task sections

## Implementation Steps

### Task 1: Add provider HTTP config model and parsing
- [ ] create `sdk/providerhttp` package with exported `Config` type using duration string fields
- [ ] define code defaults for `dial_timeout`, `tls_handshake_timeout`, `response_header_timeout`, and `idle_conn_timeout`
- [ ] add parsing into an internal resolved duration struct
- [ ] write tests for code defaults and successful duration parsing
- [ ] write tests for invalid and negative duration values
- [ ] run `go test ./sdk/providerhttp` - must pass before next task

### Task 2: Implement provider HTTP deep-merge resolution
- [ ] implement resolution order: code defaults, `providers.defaults.http`, `providers.<name>.http`
- [ ] support deep partial merge so provider overrides inherit unspecified default fields
- [ ] return clear errors that include provider name and invalid field
- [ ] write tests for global defaults merge
- [ ] write tests for provider-specific partial override merge
- [ ] write tests for invalid override failure
- [ ] run `go test ./sdk/providerhttp` - must pass before next task

### Task 3: Build explicit provider HTTP client factory
- [ ] add factory returning `*http.Client` with configured `http.Transport`
- [ ] set `DialContext`, `TLSHandshakeTimeout`, `ResponseHeaderTimeout`, and `IdleConnTimeout`
- [ ] keep `http.Client.Timeout` unset for streaming compatibility
- [ ] write tests verifying transport timeout fields
- [ ] write tests verifying client total timeout remains zero
- [ ] run `go test ./sdk/providerhttp` - must pass before next task

### Task 4: Add jitter support to generic retry
- [ ] extend `sdk/retry.Config` with jitter mode support for `full` and `none`
- [ ] keep deterministic behavior available through `none`
- [ ] implement full jitter as random delay in `[0, calculatedDelay]`
- [ ] write tests proving `none` preserves existing calculated delays
- [ ] write tests proving `full` returns delays inside the expected range
- [ ] run `go test ./sdk/retry` - must pass before next task

### Task 5: Add provider retry config resolver
- [ ] create `sdk/providerretry` package with provider-facing config type
- [ ] define defaults for `max_retries`, `base_delay`, `max_delay`, `multiplier`, and `jitter`
- [ ] implement duration string parsing for retry delays
- [ ] implement resolution order: code defaults, `providers.defaults.retry`, `providers.<name>.retry`
- [ ] expose `ForProvider(cfg sdk.Config, provider string)` returning `retry.Config` plus raw config if useful
- [ ] write tests for defaults, deep merge, invalid jitter, invalid durations, and invalid retry values
- [ ] run `go test ./sdk/providerretry` - must pass before next task

### Task 6: Make OpenAI-compatible retry explicit
- [ ] update `utils/openaicompat.Stream` to accept retry config or an options/runtime struct
- [ ] remove runtime dependency on package-global retry config while preserving testability
- [ ] add structured debug logging for retry attempts with provider/operation-safe fields only
- [ ] update existing retry tests to pass explicit retry config
- [ ] write tests for jitter-disabled deterministic retry path
- [ ] run `go test ./utils/openaicompat` - must pass before next task

### Task 7: Cap OpenAI-compatible error response bodies
- [ ] add 64 KiB cap for non-OK response body reads in `utils/openaicompat`
- [ ] append or expose a truncation marker when the error body exceeds the cap
- [ ] preserve existing structured API error parsing when body is under cap
- [ ] write tests for capped oversized error body
- [ ] write tests for normal structured error parsing after cap change
- [ ] run `go test ./utils/openaicompat` - must pass before next task

### Task 8: Verify acceptance criteria
- [ ] verify config shape supports `providers.defaults.http` and `providers.defaults.retry`
- [ ] verify invalid HTTP/retry config fails fast through resolver errors
- [ ] verify no total stream timeout is introduced
- [ ] verify retry supports only `full` and `none` jitter modes
- [ ] verify OpenAI-compatible error bodies are capped at 64 KiB
- [ ] run `go test ./sdk/... ./utils/openaicompat`
- [ ] run `make lint`

### Task 9: Update documentation if needed
- [ ] update provider configuration documentation if an appropriate docs location exists
- [ ] document `providers.defaults.http`, `providers.defaults.retry`, and provider override examples
- [ ] document that no stream-idle timeout and no `max_elapsed` are supported in this pass

## Technical Details
Config shape:

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

Target APIs:

```go
func providerhttp.ForProvider(cfg sdk.Config, provider string) (*http.Client, providerhttp.Config, error)
func providerretry.ForProvider(cfg sdk.Config, provider string) (retry.Config, providerretry.Config, error)
```

`openaicompat` should not read project config directly. Providers resolve runtime dependencies once and pass them to the shared streaming utility.

## Post-Completion
Manual verification:
- Start weave with valid provider default HTTP/retry overrides and confirm provider initialization succeeds
- Start weave with invalid duration or jitter config and confirm provider reports a clear initialization error
- Confirm retry debug logs do not contain secrets, prompts, headers, request bodies, or response bodies
