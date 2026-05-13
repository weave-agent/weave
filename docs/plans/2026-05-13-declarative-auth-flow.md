# Declarative Auth Flow for Providers

## Overview

Replace imperative API key resolution (`cfg.ResolveKey`, `model.RegisterProviderEnvVar`, `os.Getenv`) with a fully declarative auth system. Each provider declares an auth struct with `json`/`env` tags; the framework loads auth from structured `~/.weave/auth.json` + env vars automatically. Auth status is tracked by the model registry; the TUI and loop query it instead of calling `ResolveKey`.

## Context

- **Files/components involved:** `sdk/provider_registry.go`, `sdk/config.go`, `sdk/model/registry.go`, `sdk/model/env.go`, `internal/auth/`, `settings/config.go`, `settings/resolve.go`, all 4 provider extensions, `extensions/ui/tui/models.go`, `extensions/loop/loop.go`
- **Current pattern:** Providers call `model.RegisterProviderEnvVar` in `init()` and `cfg.ResolveKey` in factory. The TUI/loop use `model.ProviderEnvVar` + `cfg.ResolveKey` to check auth status.
- **New pattern:** Two-type `RegisterProvider[TConfig, TAuth]`. Auth struct loaded automatically. Model registry tracks `ProviderHasAuth`. No env var names hardcoded anywhere.
- **Breaking change:** `~/.weave/auth.json` moves from flat `{"provider": "key"}` to structured `{"provider": {"api_key": "key"}}`. No backward compatibility.

## Development Approach

- **Testing approach:** Regular (implement, then test)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change

## Testing Strategy

- **Unit tests:** Required for every task. New code gets tests; modified code gets updated tests.
- **Mock regeneration:** After `sdk.Config` interface changes, run `make gen` to regenerate moq mocks.
- **Module-by-module:** Extensions are separate Go modules. After root module changes, each extension module must compile and pass tests before proceeding.

## Implementation Steps

### Task 1: Update SDK provider registry to two-type generic

- [x] Change `RegisterProvider[T]` to `RegisterProvider[TConfig, TAuth]` in `sdk/provider_registry.go`
- [x] Store auth checker function alongside factory in registry
- [x] Add `sdk.CheckProviderAuth(name, cfg)` exported function
- [x] Update `sdk/provider_registry_test.go`
- [x] Run `go test ./sdk/...` — must pass

### Task 2: Remove ResolveKey from sdk.Config and delete ProviderEnvVar registry

- [x] Remove `ResolveKey` method from `sdk.Config` interface in `sdk/config.go`
- [x] Update `NoopConfig` and `HeadlessConfig` stubs
- [x] Regenerate `sdk/config_mock_test.go` via `make gen`
- [x] Delete `sdk/model/env.go` entirely
- [x] Update any tests that call `RegisterProviderEnvVar` or `ProviderEnvVar`
- [x] Run `go test ./sdk/...` — must pass

### Task 3: Add auth tracking to model registry

- [x] Add `SetProviderAuth` / `ProviderHasAuth` / `ListAvailableModels` to `sdk/model/registry.go`
- [x] Write tests for auth tracking functions
- [x] Run `go test ./sdk/model/...` — must pass

### Task 4: Create auth loading module

- [x] Create `internal/auth/config.go` with `LoadProviderAuth(providerName string, target any)`
- [x] Uses `settings.Loader` with `EnvPrefix: ""` on structured auth.json
- [x] Add `GetProviderConfig` helper to auth file for structured access
- [x] Write tests for auth loading (env var, auth.json, empty cases)
- [x] Run `go test ./internal/auth/...` — must pass

### Task 5: Clean up settings auth resolution code

- [x] Remove `ResolveKey`, `resolveProviderAPIKey`, `extractAPIKey` from `settings/config.go`
- [x] Delete `settings/resolve.go` entirely (or keep `ResolveValue` if used elsewhere)
- [x] Update `settings/config_test.go`, `settings/settings_config_test.go`
- [x] Run `go test ./settings/...` — must pass

### Task 6: Wire auth status during provider initialization

- [x] Update `sdk/wire/wire.go` to call `sdk.CheckProviderAuth` and `model.SetProviderAuth` for each provider during wiring
- [x] Update wire tests if they assert provider initialization behavior
- [x] Run `go test ./sdk/wire/...` — must pass

### Task 7: Migrate Anthropic provider

- [x] Create `extensions/providers/anthropic/auth.go` with `AnthropicAuth` struct
- [x] Update `extensions/providers/anthropic/anthropic.go` to use `RegisterProvider[AnthropicConfig, AnthropicAuth]`
- [x] Remove `model.RegisterProviderEnvVar` and `cfg.ResolveKey` calls
- [x] Update provider tests
- [x] Run `cd extensions/providers/anthropic && go test ./...` — must pass

### Task 8: Migrate OpenAI provider

- [x] Create `extensions/providers/openai/auth.go` with `OpenAIAuth` struct
- [x] Update `extensions/providers/openai/openai.go` to use `RegisterProvider[OpenAIConfig, OpenAIAuth]`
- [x] Remove `model.RegisterProviderEnvVar` and `cfg.ResolveKey` calls
- [x] Update provider tests
- [x] Run `cd extensions/providers/openai && go test ./...` — must pass

### Task 9: Migrate Kimi provider

- [x] Create `extensions/providers/kimi/auth.go` with `KimiAuth` struct
- [x] Update `extensions/providers/kimi/kimi.go` to use `RegisterProvider[KimiConfig, KimiAuth]`
- [x] Remove `model.RegisterProviderEnvVar` and `cfg.ResolveKey` calls
- [x] Update provider tests
- [x] Run `cd extensions/providers/kimi && go test ./...` — must pass

### Task 10: Migrate Z.ai provider

- [ ] Create `extensions/providers/zai/auth.go` with `ZaiAuth` struct
- [ ] Update `extensions/providers/zai/zai.go` to use `RegisterProvider[ZaiConfig, ZaiAuth]`
- [ ] Remove `model.RegisterProviderEnvVar` and `cfg.ResolveKey` calls
- [ ] Update provider tests
- [ ] Run `cd extensions/providers/zai && go test ./...` — must pass

### Task 11: Clean up TUI auth checks

- [ ] Update `extensions/ui/tui/models.go` — `providerHasKey` uses `model.ProviderHasAuth` instead of `cfg.ResolveKey`
- [ ] Update model selector to use `model.ListAvailableModels`
- [ ] Remove `ResolveKey` from mock config implementations in TUI tests
- [ ] Run `cd extensions/ui/tui && go test ./...` — must pass

### Task 12: Clean up loop auth checks

- [ ] Update `extensions/loop/loop.go` — `anyProviderHasKey` uses `model.ProviderHasAuth` instead of `cfg.ResolveKey`
- [ ] Remove `model.ProviderEnvVar` import from loop
- [ ] Update loop tests
- [ ] Run `cd extensions/loop && go test ./...` — must pass

### Task 13: Verify acceptance criteria

- [ ] All root module tests pass (`go test ./...` from root)
- [ ] All extension module tests pass (`cd extensions/* && go test ./...` for each)
- [ ] `make lint` passes
- [ ] `make fmt` produces no changes
- [ ] No `ResolveKey` calls remain in extension code
- [ ] No `RegisterProviderEnvVar` calls remain
- [ ] No `ProviderEnvVar` calls remain
- [ ] `--help` still shows provider flags correctly

## Technical Details

### Auth struct pattern

Each provider defines an auth struct in a separate `auth.go` file:

```go
package anthropic

type AuthConfig struct {
    APIKey string `json:"api_key" env:"ANTHROPIC_API_KEY" description:"API key"`
}
```

### Registration pattern

```go
sdk.RegisterProvider[AnthropicConfig, AuthConfig]("anthropic",
    func(cfg sdk.Config, c AnthropicConfig, a AuthConfig) (sdk.Provider, error) {
        if a.APIKey == "" {
            return nil, errors.New("anthropic: API key required")
        }
        // use c.Model, a.APIKey
    })
```

### Auth loading flow

```
RegisterProvider[TConfig, TAuth](name, factory)
  └─> wrapper := func(cfg Config) (Provider, error) {
        var tc TConfig
        cfg.ExtensionConfig("providers", name, &tc, "")  // model, base_url, etc.
        var ta TAuth
        auth.LoadProviderAuth(name, &ta)                  // api_key from auth.json + env
        return factory(cfg, tc, ta)
      }
```

### Auth file format

```json
{
    "anthropic": {"api_key": "sk-ant-..."},
    "openai": {"api_key": "sk-..."}
}
```

### Auth status tracking

During wiring, the framework calls `sdk.CheckProviderAuth(name, cfg)` for each registered provider and stores the result via `model.SetProviderAuth(name, hasAuth)`. The TUI and loop query `model.ProviderHasAuth(name)` instead of calling `cfg.ResolveKey`.

## Post-Completion

- Update CLAUDE.md to document the new auth pattern (declarative auth structs, auth.json format)
- Verify `weave` binary builds and runs correctly with real provider keys
