# Add Login and OAuth Support to Weave

## Overview

Add interactive login support to Weave's TUI, including both API key entry and OAuth browser flows. Currently, Weave only supports API keys via environment variables or manual `~/.weave/auth.json` editing. This feature adds `/login` and `/logout` slash commands that let users authenticate interactively within the TUI.

**Key benefits:**
- No more manual auth.json editing or env var management
- OAuth support for subscription-billed providers (GitHub Copilot, OpenAI ChatGPT)
- Visual auth status in the TUI (which providers are configured)
- Token refresh handled automatically

**Initial OAuth providers:** GitHub Copilot (device code flow) and OpenAI (browser-based OAuth).

## Context (from discovery)

- **Auth storage**: `internal/auth/auth.go` (`File` struct with `Providers map[string]json.RawMessage`), `config.go` (`LoadProviderAuth`, `applyEnvToStruct`)
- **Provider auth**: All providers use `AuthConfig{APIKey string}` with `json:"api_key" env:"..." validate:"required"` tags
- **Auth wiring**: `internal/wire/wire.go` calls `sdk.CheckProviderAuth` then `model.SetProviderAuth`
- **TUI commands**: `extensions/ui/tui/commands.go` (`CommandRegistry`), `model.go` registers built-ins
- **TUI overlays**: `components/overlays/stack.go` (`DialogStack`), `input.go` (`InputModel` — no password masking yet)
- **TUI overlay system**: `overlays.go` (`overlayRequest`/`overlayResponse`, `pushPopupDialog`)
- **Model registry**: `sdk/model/registry.go` (`SetProviderAuth`, `ProviderHasAuth`, `ListAvailableModels`)

## Development Approach

- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility (existing API key and env var auth continues to work)

## Testing Strategy

- **Unit tests**: required for every task
  - `internal/auth/`: Test OAuth credential storage, refresh, clearing
  - `sdk/`: Test OAuth provider registry
  - `extensions/ui/tui/`: Test slash command registration, dialog state
- **Integration tests**: OAuth callback server, token exchange (mocked HTTP)
- No E2E tests (project doesn't use Playwright/Cypress)

## Implementation Steps

### Task 1: Extend auth storage for OAuth credentials
- [x] Add `OAuthCredential` struct and helpers to `internal/auth/auth.go`
- [x] Add `GetOAuthCredential`, `SetOAuthCredential`, `ClearProviderAuth` methods to `File`
- [x] Update `LoadProviderAuth` in `config.go` to handle OAuth token fields in provider auth structs
- [x] Ensure auth file backward compatibility (old files without OAuth fields still work)
- [x] Write tests for OAuth credential storage (success + error cases)
- [x] Write tests for `ClearProviderAuth` and backward compatibility
- [x] Run `make test` — must pass before Task 2

### Task 2: Add OAuth provider registry to SDK
- [ ] Define `OAuthProvider` struct (`AuthURL`, `TokenURL`, `DeviceCodeURL`, `Scopes`, `ClientID`, `FlowType`)
- [ ] Add `RegisterOAuthProvider` / `GetOAuthProvider` / `ListOAuthProviders` to `sdk/`
- [ ] Define OAuth flow types: `AuthorizationCode`, `DeviceCode`
- [ ] Register GitHub Copilot (`DeviceCode` flow)
- [ ] Register OpenAI (`AuthorizationCode` flow)
- [ ] Update `model.SetProviderAuth` / `CheckProviderAuth` to consider OAuth tokens as valid auth
- [ ] Write tests for OAuth provider registry
- [ ] Write tests for auth status with OAuth credentials
- [ ] Run `make test` — must pass before Task 3

### Task 3: Add password masking to TUI input dialog
- [ ] Add `WithMask` or `mask` field to `InputModel` in `components/overlays/input.go`
- [ ] Render masked characters (`*`) when mask is enabled
- [ ] Update `sdk.UI` interface `InputOption` to support password masking
- [ ] Update `NoopUI` stub
- [ ] Write tests for input masking
- [ ] Run `make test` — must pass before Task 4

### Task 4: Add `/login` and `/logout` slash commands
- [ ] Register `/login` command in `commands.go` — shows OAuth + API key provider selector
- [ ] Register `/logout` command — shows configured providers, clears selected auth
- [ ] Add `AuthSelectorComponent` (lists providers with status indicators)
- [ ] Wire auth selector into TUI model (`model.go`)
- [ ] Add `auth.login.success` and `auth.logout` bus events
- [ ] Write tests for slash command registration
- [ ] Write tests for auth selector state
- [ ] Run `make test` — must pass before Task 5

### Task 5: Implement API key login flow
- [ ] API key selection from auth selector triggers `InputDialog` with password masking
- [ ] On submit: call `auth.SetProviderKey(provider, key)`
- [ ] Update `model.SetProviderAuth(provider, true)`
- [ ] Show success notification via `SetNotify`
- [ ] Write tests for API key flow
- [ ] Run `make test` — must pass before Task 6

### Task 6: Implement OAuth callback server
- [ ] Create `internal/auth/callback_server.go` — temporary HTTP server on localhost random port
- [ ] Handle single `/callback` request, extract authorization code
- [ ] 2-minute timeout with context cancellation
- [ ] Return code via channel, then auto-shutdown
- [ ] Write tests for callback server (mock HTTP requests)
- [ ] Run `make test` — must pass before Task 7

### Task 7: Implement authorization code OAuth flow (OpenAI)
- [ ] PKCE verifier/challenge generation
- [ ] Build authorization URL with redirect URI, state, PKCE
- [ ] Open browser via `xdg-open` / `open` / `start`
- [ ] Exchange code for tokens via HTTP POST to token URL
- [ ] Store tokens via `auth.SetOAuthCredential`
- [ ] Add `LoginDialogComponent` — shows URL, waiting state, success/error
- [ ] Write tests for PKCE generation, URL building, token exchange (mocked HTTP)
- [ ] Run `make test` — must pass before Task 8

### Task 8: Implement device code OAuth flow (GitHub Copilot)
- [ ] Request device code from device authorization endpoint
- [ ] Show user code and verification URL in `LoginDialogComponent`
- [ ] Poll token endpoint at interval until authorized or timeout
- [ ] Store tokens on success
- [ ] Write tests for device code flow (mocked HTTP)
- [ ] Run `make test` — must pass before Task 9

### Task 9: Add lazy token refresh
- [ ] Add `RefreshOAuthToken` to `internal/auth/` — uses refresh token, updates `auth.json`
- [ ] Provider factories call refresh before using OAuth token if near expiry
- [ ] Graceful fallback to "auth expired" error if refresh fails
- [ ] Write tests for token refresh (mocked HTTP)
- [ ] Write tests for expiry check and fallback
- [ ] Run `make test` — must pass before Task 10

### Task 10: Update provider auth structs for OAuth
- [ ] Update `extensions/providers/anthropic/auth.go` to include `OAuthToken` field
- [ ] Update `extensions/providers/openai/auth.go` to include `OAuthToken` field
- [ ] Update `extensions/providers/kimi/auth.go` to include `OAuthToken` field
- [ ] Update `extensions/providers/zai/auth.go` to include `OAuthToken` field
- [ ] Update provider factories to use API key or OAuth token (whichever present)
- [ ] Write tests for provider auth struct changes
- [ ] Run `make test` — must pass before Task 11

### Task 11: Verify acceptance criteria
- [ ] `/login` shows provider selector with OAuth + API key options
- [ ] `/logout` shows configured providers and clears selected auth
- [ ] API key entry works end-to-end (input → storage → provider auth)
- [ ] OAuth flows work for OpenAI (authorization code) and Copilot (device code)
- [ ] Token refresh happens automatically on expiry
- [ ] Auth status correctly shows in model selector (only configured providers/models)
- [ ] Headless mode unchanged (no TUI login, env vars still work)
- [ ] Backward compatibility: existing auth.json files work
- [ ] Run full test suite: `make test`
- [ ] Run linter: `make lint`
- [ ] Verify test coverage for new code

### Task 12: Update documentation
- [ ] Update CLAUDE.md auth section with `/login` and `/logout` usage
- [ ] Update provider docs to mention OAuth support
- [ ] Add example: `weave /login` → select provider → enter key or OAuth flow

## Technical Details

### Data Structures

```go
// internal/auth/auth.go additions
type OAuthCredential struct {
    AccessToken  string    `json:"access_token"`
    RefreshToken string    `json:"refresh_token"`
    ExpiresAt    time.Time `json:"expires_at,omitempty"`
    TokenType    string    `json:"token_type,omitempty"`
}

// sdk/ oauth registry additions
type OAuthFlowType string
const (
    AuthorizationCode OAuthFlowType = "authorization_code"
    DeviceCode        OAuthFlowType = "device_code"
)

type OAuthProvider struct {
    ID           string
    Name         string
    AuthURL      string
    TokenURL     string
    DeviceCodeURL string    // optional, for device code flow
    Scopes       []string
    ClientID     string    // optional, may be provider-specific
    FlowType     OAuthFlowType
}
```

### Auth File Format (Backward Compatible)

```json
{
  "providers": {
    "anthropic": {
      "api_key": "sk-ant-..."
    },
    "openai": {
      "access_token": "sk-...",
      "refresh_token": "rt-...",
      "expires_at": "2026-05-16T12:00:00Z",
      "token_type": "bearer"
    },
    "github-copilot": {
      "access_token": "ghu_...",
      "refresh_token": "ghr_...",
      "expires_at": "2026-05-16T12:00:00Z"
    }
  }
}
```

### Processing Flow

1. **TUI `/login`**: Show `AuthSelectorComponent` → user selects provider →
   - If OAuth: spawn `LoginDialogComponent` → browser/device code → callback server (auth code) or polling (device code) → exchange for tokens → `auth.SetOAuthCredential` → `model.SetProviderAuth`
   - If API key: show `InputDialog` (masked) → `auth.SetProviderKey` → `model.SetProviderAuth`

2. **Provider wiring**: `wire.go` → `LoadProviderAuth` → auth struct has `APIKey` or `OAuthToken` → factory uses whichever is present

3. **Token refresh**: Provider makes request → checks token expiry → if expired + has refresh token → `RefreshOAuthToken` → retry request

4. **Logout**: `/logout` → show configured providers → `auth.ClearProviderAuth` → `model.SetProviderAuth(provider, false)`

## Post-Completion

**Manual verification:**
- Test `/login` with each provider (OpenAI, Copilot, API key for any)
- Test `/logout` clears auth and removes provider from model selector
- Test token expiry: set expiry to past, verify refresh on next request
- Test headless mode: ensure `-p` flag still works with env vars
- Test backward compatibility: old auth.json without OAuth fields

**Security considerations:**
- Auth file remains `0600` permissions
- OAuth state parameter prevents CSRF
- PKCE prevents authorization code interception
- Refresh tokens never logged
- Callback server binds only to localhost
