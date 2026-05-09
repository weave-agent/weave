# Sandbox Extension — OS-Level Tool Execution Guard

## Overview

Add a sandbox extension that wraps bash tool execution with OS-level sandboxing (Seatbelt on macOS, bubblewrap on Linux) and enforces path-based access policy on all file tools (read, write, edit, grep, find, ls). The sandbox constrains writes to CWD, blocks access to sensitive paths, and provides configurable network isolation. A companion TUI extension adds a mode indicator, mode cycling keybinding, and ask-mode approval dialog.

Four sandbox modes: `off` → `readonly` → `ask` → `auto` (default). The design is "soft guard" — freedom to act inside the project, but catastrophic mistakes (writing to `~/.ssh`, rewriting `.bashrc`, `rm -rf /`) are caught by the OS sandbox profile.

## Context

### Files/components involved
- `sdk/sandbox.go` — new `Sandboxer` interface (`WrapCommand`, `AllowWrite`, `AllowRead`) + package-level getter/setter
- `extensions/sandbox/` — new extension module (core sandbox logic + path policy)
- `extensions/ui/sandbox/` — new TUI extension (mode indicator, keybinding, ask dialog)
- `extensions/tools/bash/bash.go` — modify `Execute()` to check `Sandboxer.WrapCommand()`
- `extensions/tools/write/write.go` — modify `Execute()` to check `Sandboxer.AllowWrite()`
- `extensions/tools/edit/edit.go` — modify `Execute()` to check `Sandboxer.AllowWrite()`
- `extensions/tools/read/read.go` — modify `Execute()` to check `Sandboxer.AllowRead()`
- `extensions/tools/grep/grep.go` — modify `Execute()` to check `Sandboxer.AllowRead()`
- `extensions/tools/find/find.go` — modify `Execute()` to check `Sandboxer.AllowRead()`
- `extensions/tools/ls/ls.go` — modify `Execute()` to check `Sandboxer.AllowRead()`
- `sdk/config.go` — `noopConfig` unchanged (no Sandboxer on Config interface)

### Key patterns
- Tool registration: `sdk.RegisterTool(name, factory)` in `init()`
- UI extension registration: `sdk.RegisterUIExtension(&struct{})` with `Name()`, `Register(ui)`
- Footer status: `ui.SetStatus(key, text)` adds indicator pills
- Bus events: `bus.Publish(sdk.NewEvent(topic, payload))` / `bus.On(topic, handler)`
- Config loading: `cfg.ToolConfig(name, &target)` with JSON struct tags
- Testing: `testify` assertions, `moq`-generated mocks

### Dependencies
- macOS: `sandbox-exec` (built-in, no install needed)
- Linux: `bwrap` (bubblewrap) — must be installed. Detected at runtime, sandbox disabled if missing.

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope
- Keep plan in sync with actual work done

## Implementation Steps

### Task 1: Add Sandboxer interface to SDK
- [x] create `sdk/sandbox.go` with `Sandboxer` interface:
  ```go
  type Sandboxer interface {
      WrapCommand(cmd, dir string) (string, error) // wraps bash commands in OS sandbox
      AllowWrite(path string) bool                 // checks if file tool can write to path
      AllowRead(path string) bool                  // checks if file tool can read from path
  }
  ```
- [x] add package-level `sandboxer` var with `SetSandboxer()` / `GetSandboxer()` (nil-safe, no registry)
- [x] add `//go:generate moq` directive for `Sandboxer`
- [x] write tests for `SetSandboxer`/`GetSandboxer` (set/get, nil default, overwrite)
- [x] run `make gen` to generate mocks
- [x] run `go test ./sdk/...` — must pass before task 2

### Task 2: Create sandbox extension scaffold
- [x] create `extensions/sandbox/` directory with `go.mod` (module: `weave/extensions/sandbox`)
- [x] create `extensions/sandbox/sandbox.go` with `SandboxConfig` struct (JSON tags: mode, writable, deny_write, deny_read, network)
- [x] implement `init()` with `sdk.RegisterExtension("sandbox", factory)`
- [x] factory loads config via `cfg.ToolConfig("sandbox", &sc)`, creates `Sandbox` struct, calls `sdk.SetSandboxer(s)`
- [x] implement `Subscribe(bus)` — listen for `sandbox.mode.change` to switch modes mid-session
- [x] implement `Close()` — cleanup
- [x] write tests for config loading, mode switching, extension lifecycle
- [x] run `cd extensions/sandbox && go test ./...` — must pass before task 3

### Task 3: Implement macOS Seatbelt profile generation
- [x] create `extensions/sandbox/darwin.go` with build tag `//go:build darwin`
- [x] implement `generateSeatbeltProfile(config SandboxConfig, dir string) string` — generates Seatbelt v1 profile
- [x] mandatory deny paths: `~/.ssh/`, `~/.bashrc`, `~/.zshrc`, `~/.profile`, `~/.gitconfig`, `.git/hooks/`, `.git/config`, `.weave/`
- [x] mandatory read denies: `~/.ssh/id_*`, `~/.aws/credentials`, `**/.env`, `**/.env.*`
- [x] write rules: `(allow file-write* (subpath <writable>))` for CWD by default
- [x] network rules: allow all if `network: true`, proxy-based if `network: false`
- [x] implement `WrapCommand(cmd, dir) (string, error)` — returns `sandbox-exec -p '<profile>' bash -c '<cmd>'`
- [x] write tests for profile generation (mandatory denies present, writable paths correct, network rules)
- [x] run `cd extensions/sandbox && go test ./...` — must pass before task 4

### Task 4: Implement Linux bwrap profile generation
- [x] create `extensions/sandbox/linux.go` with build tag `//go:build linux`
- [x] implement `buildBwrapArgs(config SandboxConfig, dir string) []string`
- [x] read-only root: `--ro-bind / /`
- [x] writable paths: `--bind <path> <path>` for each writable entry
- [x] mandatory deny paths: `--ro-bind /dev/null <deny-path>` for files, `--tmpfs <dir>` for directories
- [x] network: `--unshare-net` when `network: false`
- [x] PID isolation: `--unshare-pid --proc /proc`
- [x] implement `WrapCommand(cmd, dir) (string, error)` — returns `bwrap <args> -- bash -c '<cmd>'`
- [x] add `bwrapAvailable()` check — return error if bwrap not installed
- [x] write tests for bwrap arg construction (mandatory denies, writable mounts, network flags)
- [x] run `cd extensions/sandbox && go test ./...` — must pass before task 5

### Task 5: Implement sandbox modes
- [x] add mode constants: `ModeOff`, `ModeReadonly`, `ModeAsk`, `ModeAuto`
- [x] `auto` mode: `WrapCommand` wraps every bash command in sandbox profile
- [x] `readonly` mode: profile has no writable paths, built-in read-only command allowlist passes through
- [x] `ask` mode: publishes `sandbox.approve` event on bus with command details, waits for `sandbox.approved`/`sandbox.denied` response
- [x] `off` mode: `WrapCommand` returns command unchanged
- [x] handle headless `ask` mode: deny with message "command requires approval (headless mode)"
- [x] write tests for each mode's `WrapCommand` behavior (off returns unchanged, readonly has no writable paths, ask publishes event, auto wraps)
- [x] run `cd extensions/sandbox && go test ./...` — must pass before task 6

### Task 6: Integrate sandbox into bash tool
- [x] modify `extensions/tools/bash/bash.go` `Execute()` — check `sdk.GetSandboxer()` before building command
- [x] if sandboxer present, call `sandboxer.WrapCommand(command, t.dir)` and use wrapped command
- [x] if sandboxer returns error, return `ToolResult{Content: "sandbox: ...", IsError: true}`
- [x] if sandboxer nil, current behavior unchanged
- [x] add `dir` field to bash tool struct, populated from `cfg.FilePath()` or CWD
- [x] write tests for bash tool with mock sandboxer (wraps command, returns error, nil sandboxer)
- [x] run `cd extensions/tools/bash && go test ./...` — must pass before task 7

### Task 7: Integrate sandbox policy into file tools (write, edit)
- [x] modify `extensions/tools/write/write.go` `Execute()` — check `sdk.GetSandboxer().AllowWrite(path)` before writing
- [x] if denied, return `ToolResult{Content: "sandbox: write denied — path is protected", IsError: true}`
- [x] modify `extensions/tools/edit/edit.go` `Execute()` — check `sdk.GetSandboxer().AllowWrite(path)` before editing
- [x] if sandboxer nil, current behavior unchanged for both tools
- [x] write tests for write tool with mock sandboxer (allowed write, denied write, nil sandboxer)
- [x] write tests for edit tool with mock sandboxer (allowed edit, denied edit, nil sandboxer)
- [x] run `cd extensions/tools/write && go test ./...` and `cd extensions/tools/edit && go test ./...` — must pass before task 8

### Task 8: Integrate sandbox policy into file tools (read, grep, find, ls)
- [ ] modify `extensions/tools/read/read.go` `Execute()` — check `sdk.GetSandboxer().AllowRead(path)` before reading
- [ ] if denied, return `ToolResult{Content: "sandbox: read denied — path is protected", IsError: true}`
- [ ] modify `extensions/tools/grep/grep.go` `Execute()` — check `AllowRead()` for search paths
- [ ] modify `extensions/tools/find/find.go` `Execute()` — check `AllowRead()` for search paths
- [ ] modify `extensions/tools/ls/ls.go` `Execute()` — check `AllowRead()` for listed paths
- [ ] if sandboxer nil, current behavior unchanged for all tools
- [ ] write tests for each tool with mock sandboxer (allowed read, denied read, nil sandboxer)
- [ ] run tests for all four tool modules — must pass before task 9

### Task 9: Create TUI sandbox extension
- [ ] create `extensions/ui/sandbox/` directory with `go.mod` (module: `weave/extensions/ui/sandbox`)
- [ ] implement `init()` with `sdk.RegisterUIExtension(&SandboxUI{})`
- [ ] `Register(ui)`: call `ui.SetStatus("sandbox", "SB:auto")` for initial mode display
- [ ] `Register(ui)`: call `ui.RegisterKeybinding(sdk.Keybinding{Name: "sandbox.cycle", Keys: []string{"ctrl+s"}, Description: "Cycle sandbox mode"})`
- [ ] listen for `sandbox.mode.change` events to update footer status pill
- [ ] write tests for UI extension registration and status updates
- [ ] run `cd extensions/ui/sandbox && go test ./...` — must pass before task 10

### Task 10: Implement ask-mode approval dialog in TUI
- [ ] create `extensions/ui/sandbox/dialog.go` with `ApproveDialog` implementing TUI dialog interface
- [ ] dialog shows command text, approve/deny/trust-for-session options
- [ ] "trust for session" publishes `sandbox.trust` event — sandbox extension adds pattern to session allowlist
- [ ] dialog integrates with existing `DialogStack` overlay system
- [ ] listen for `sandbox.approve` bus events to trigger dialog display
- [ ] write tests for dialog construction and approval/denial flows
- [ ] run `cd extensions/ui/sandbox && go test ./...` — must pass before task 11

### Task 11: Add sandbox config to .weave.yaml validation
- [ ] update `config/validation.go` to validate `sandbox` section if present
- [ ] validate `mode` is one of: off, readonly, ask, auto
- [ ] validate `writable`, `deny_write`, `deny_read` entries are valid paths
- [ ] validate `network` is boolean
- [ ] write tests for config validation (valid configs, invalid mode, invalid paths)
- [ ] run `go test ./config/...` — must pass before task 12

### Task 12: Update CLAUDE.md and documentation
- [ ] update `CLAUDE.md` Architecture section with sandbox extension description
- [ ] update `CLAUDE.md` Configuration section with sandbox config format
- [ ] add sandbox mode descriptions and keybinding to Configuration section
- [ ] run `make lint` — all issues must be fixed
- [ ] run full test suite: `make test`

## Technical Details

### Sandboxer interface
```go
type Sandboxer interface {
    WrapCommand(cmd, dir string) (string, error) // wraps bash commands in OS sandbox
    AllowWrite(path string) bool                 // checks if file tool can write to path
    AllowRead(path string) bool                  // checks if file tool can read from path
}
```

### File tool integration pattern
Each file tool checks the sandboxer at the top of `Execute()`:
```go
// write/edit tools
if s := sdk.GetSandboxer(); s != nil && !s.AllowWrite(path) {
    return sdk.ToolResult{Content: "sandbox: write denied — path is protected", IsError: true}, nil
}

// read/grep/find/ls tools
if s := sdk.GetSandboxer(); s != nil && !s.AllowRead(path) {
    return sdk.ToolResult{Content: "sandbox: read denied — path is protected", IsError: true}, nil
}
```
If sandboxer is nil (extension not loaded), tools work exactly as before.

### Sandbox config (YAML → JSON settings)
```yaml
# .weave.yaml
sandbox:
  mode: auto        # off | readonly | ask | auto
  writable: ["."]   # paths allowed for writes
  deny_write: []    # additional paths to block
  deny_read: []     # paths to block from reading
  network: true     # allow network in sandbox
```

### Mandatory deny paths (hardcoded, not configurable)
Write denies: `~/.ssh/`, `~/.bashrc`, `~/.zshrc`, `~/.profile`, `~/.gitconfig`, `.git/hooks/`, `.git/config`, `.weave/`
Read denies: `~/.ssh/id_*`, `~/.aws/credentials`, `**/.env`, `**/.env.*`

### Mode behavior matrix
| Mode | Bash reads | Bash writes | Write/Edit tools | Read/Grep/Find/Ls tools | Headless fallback |
|---|---|---|---|---|---|
| `off` | Free | Free | Free | Free | N/A |
| `readonly` | Read-only cmds only | Blocked | Blocked (`AllowWrite` → false) | Free | Same |
| `ask` | Prompt each | Prompt each | Prompt (`AllowWrite` via event) | Free | Deny all writes |
| `auto` | Free | Sandbox wraps | Policy check (`AllowWrite`) | Policy check (`AllowRead`) | Same |

### Bus events
- `sandbox.mode.change` — payload: `{mode: "auto"}` — switches active mode
- `sandbox.approve` — payload: `{command: "..."}` — ask-mode requesting approval
- `sandbox.approved` — payload: `{command: "...", trust: false}` — approval response
- `sandbox.denied` — payload: `{command: "..."}` — denial response
- `sandbox.trust` — payload: `{pattern: "npm *"`} — trust pattern for session

### Platform detection
```go
func (s *Sandbox) detectPlatform() (string, error) {
    switch runtime.GOOS {
    case "darwin":
        return "darwin", nil // sandbox-exec always available
    case "linux":
        if _, err := exec.LookPath("bwrap"); err != nil {
            return "", fmt.Errorf("bubblewrap not installed")
        }
        return "linux", nil
    default:
        return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
    }
}
```

## Post-Completion

**Manual verification**:
- Test sandbox on macOS with `sandbox-exec` — verify writes blocked outside CWD
- Test sandbox on Linux with `bwrap` — verify filesystem and network isolation
- Test mode cycling in TUI — verify footer indicator updates
- Test ask-mode approval dialog — verify approve/deny/trust flows
- Test headless mode with ask — verify commands are denied
- Test `rm -rf /` in auto mode — verify blocked by mandatory deny

**External system updates**:
- `bwrap` (bubblewrap) must be installed on Linux — document in README
- Windows support not included — can be added as a future task
