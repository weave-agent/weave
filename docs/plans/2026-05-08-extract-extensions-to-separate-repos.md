# Extract Extensions to Separate Repos

## Overview

Move all extensions from the monorepo into independent repositories under `github.com/weave-agent`, enabling independent release cycles and community forks. The SDK stays in the main `weave` repo.

**Key changes:**
- Root module path: `weave` → `github.com/weave-agent/weave`
- Extension module paths: `weave/ext/...` → `github.com/weave-agent/weave-<name>`
- All imports updated across codebase and extensions
- Launcher builder generates temp go.mod with `replace` directives for SDK + extensions
- Core extensions auto-install on first run into `~/.weave/extensions/`
- Built-in discovery tier removed (extensions live in `~/.weave/extensions/` or project-local)

**Problem it solves:** Currently extensions are tightly coupled to the monorepo via `replace weave => ../../` directives. Users cannot fork/customize individual extensions without forking the entire repo.

**Key benefits:** Independent extension releases, community contributions, smaller main repo, fork-friendly architecture.

## Context (from discovery)

**Prerequisite work completed:**
- SDK refactor (`2026-05-13-sdk-refactor.md`) completed — `sdk/wire/` moved to `internal/wire/`, `launcher/` moved to `internal/launcher/`, extension management extracted to `internal/extmanage/`
- Loop, skills, and instructions extensions merged into single `agent` extension (`extensions/agent/`)

**Files/components involved:**
- `go.mod` — root module path change
- All `*.go` files — import path updates (`weave/...` → `github.com/weave-agent/weave/...`)
- `internal/wire/run.go` — `findModuleRoot()`, `isWeaveModule()` check `module weave`
- `internal/launcher/builder.go` — `GenerateGoMod`, `GenerateMainGo`, `ensureExtGoMod`, `extModulePath`, `Build`
- `internal/launcher/discovery.go` — `AutoDiscover` (already implemented, uses basename for name)
- `internal/launcher/launcher.go` — `BuildFunc` signature, `Run`, `buildAndCache`
- All `extensions/*/go.mod` — module path + dependency changes
- `internal/extmanage/install.go` — `weave install` command
- `internal/extmanage/extmanage.go` — extension listing/updating

**Related patterns found:**
- Extensions self-register via `init()` + `sdk.RegisterExtension/RegisterProvider/RegisterTool/RegisterUIExtension`
- Generated `main.go` blank-imports extension modules to trigger registration
- Launcher generates temp go.mod with `require` + `replace` for each extension
- `ensureExtGoMod` writes shim go.mods (with sentinel) for extensions lacking one
- Hash-based caching: `ComputeHash` SHA256 of extension contents invalidates binary cache
- Auto-discovery already implemented: scans `.weave/extensions/`, `~/.weave/extensions/`, `moduleRoot/extensions/`

**Dependencies identified:**
- Auto-discovery simplification plan (`2026-05-08-simplify-extension-config-auto-discovery.md`) is already implemented
- `openaicompat` is in `utils/openaicompat/` (part of main module, not a separate extension)
- TUI has nested UI extensions (`diff-viewer`) under `extensions/ui/tui/extensions/`
- All built-in extensions already have their own `go.mod`
- `sandbox` extension (`extensions/sandbox/`) and `sandbox-ui` (`extensions/ui/sandbox-ui/`) were added after initial plan

## Development Approach

- **Testing approach:** Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change

## Testing Strategy

- **Unit tests:** Required for every task
  - `internal/launcher/builder_test.go` — test `GenerateGoMod` with new module paths
  - `internal/launcher/builder_test.go` — test `GenerateMainGo` imports use full module path
  - `internal/launcher/discovery_test.go` — test `AutoDiscover` with updated module paths
  - `internal/wire/run_test.go` — test `findModuleRoot` with new module name
  - `config/*_test.go` — verify no regressions from import changes
  - Each extension module: `cd extensions/<name> && go test ./...`
- **Integration tests:** `make test` from root, build and exec binary
- **E2E:** Run `weave` interactively and headlessly to verify extensions load

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope
- Keep plan in sync with actual work done

## Implementation Steps

### Task 1: Update SDK module path in main repo

- [x] Update `go.mod`: change `module weave` to `module github.com/weave-agent/weave`
- [x] Update ALL imports in main repo (`sdk/`, `bus/`, `config/`, `internal/launcher/`, `internal/wire/`, `internal/extmanage/`, `utils/`, `cmd/`, `settings/`) from `weave/...` to `github.com/weave-agent/weave/...`
- [x] Update `internal/wire/run.go` `isWeaveModule()`: check for `module github.com/weave-agent/weave`
- [x] Update `internal/launcher/builder.go` `extModulePath` fallback: `"github.com/weave-agent/weave/ext/" + ext.Name`
- [x] Update `internal/launcher/builder.go` `GenerateGoMod`: use `github.com/weave-agent/weave` instead of `weave`
- [x] Update `internal/launcher/builder.go` `GenerateMainGo`: imports use `github.com/weave-agent/weave/...`
- [x] Update `internal/launcher/builder.go` `ensureExtGoMod` shim: `module github.com/weave-agent/weave/ext/<name>`, `require github.com/weave-agent/weave`, `replace github.com/weave-agent/weave => <moduleRoot>`
- [x] Run `make fmt` and `make fix`
- [x] Run `go test ./...` from root — must pass
- [x] Run `make test` — must pass

### Task 2: Update extension module paths

For each extension directory (`extensions/agent/`, `extensions/sandbox/`, `extensions/instructions/` [merged into agent], `extensions/loop/` [merged into agent], `extensions/skills/` [merged into agent], `extensions/store/jsonl/`, `extensions/tools/{bash,read,edit,write,grep,find,ls,subagent}/`, `extensions/providers/{anthropic,openai,zai,kimi}/`, `extensions/ui/tui/`, `extensions/ui/sandbox-ui/`, `extensions/ui/tui/extensions/diff-viewer/`):

- [x] Update `go.mod`: change `module weave/ext/...` to `module github.com/weave-agent/weave-<name>`
  - Naming: `bash` not `tools-bash`, `anthropic` not `providers-anthropic`, `tui` not `ui-tui`
  - `diff-viewer` → `github.com/weave-agent/weave-diff-viewer`
  - `sandbox-ui` → `github.com/weave-agent/weave-sandbox-ui`
  - Also added: `codex` → `github.com/weave-agent/weave-codex`, `subagent-ui` → `github.com/weave-agent/weave-subagent-ui`
- [x] Remove `replace weave => ...` line from go.mod
- [x] Add `require github.com/weave-agent/weave v0.0.0`
- [x] Update ALL imports in extension `.go` files from `weave/...` to `github.com/weave-agent/weave/...`
- [x] Run `go mod tidy` in each extension directory
- [x] Run `cd <ext-dir> && go test ./...` for each extension — must pass

### Task 3: Update builder tests for new module paths

- [x] Update `internal/launcher/builder_test.go` test cases that reference `weave` module path
- [x] Update `internal/launcher/builder_test.go` `GenerateGoMod` tests: verify output contains `github.com/weave-agent/weave`
- [x] Update `internal/launcher/builder_test.go` `GenerateMainGo` tests: verify imports use full module path
- [x] Update `internal/launcher/builder_test.go` `ensureExtGoMod` tests: verify shim uses new module path
- [x] Update `internal/launcher/discovery_test.go` test fixtures that write go.mod files with old module paths
- [x] Update `internal/launcher/launcher_test.go` if it references old module paths
- [x] Update `internal/wire/run_test.go` `findModuleRoot` tests with new module name
- [x] Run `go test ./internal/launcher/... ./internal/wire/...` — must pass

### Task 4: Verify end-to-end build

- [x] Run `go build ./cmd/weave` from root — must succeed (verified via `go build ./...`)
- [x] Run `./weave -p "hello"` headlessly — must work (skipped - requires API key, verified via `TestBuild_WithTrivialExtension`)
- [x] Run `./weave` interactively — must load TUI and all tools (skipped - requires interactive terminal + API key)
- [x] Verify `/reload` works after build (skipped - requires running TUI session)
- [x] Run `make lint` — must pass (0 issues across all packages)
- [x] Run `make test` — full suite must pass (all root + extension tests pass)

### Task 5: Create extension repo template and extract first extension

- [x] Push `github.com/weave-agent/weave-bash` repo
  - Copy `extensions/tools/bash/` contents to temp dir
  - `go.mod`: `module github.com/weave-agent/weave-bash`
  - README with fork/customize instructions
  - LICENSE same as main repo (skipped - no LICENSE in main repo)
  - `git init`, `git add`, `git commit`, `git push` to `git@github.com:weave-agent/weave-bash.git`
- [x] Verify `weave install github.com/weave-agent/weave-bash --name bash` works
- [x] Remove `extensions/tools/bash/` from main repo
- [x] Update main repo CI/docs to reference external extension repos
- [x] Run tests — must pass

### Task 6: Extract remaining core extensions

Repeat for each remaining extension:

- [ ] `github.com/weave-agent/weave-read`
- [ ] `github.com/weave-agent/weave-edit`
- [ ] `github.com/weave-agent/weave-write`
- [ ] `github.com/weave-agent/weave-grep`
- [ ] `github.com/weave-agent/weave-find`
- [ ] `github.com/weave-agent/weave-ls`
- [ ] `github.com/weave-agent/weave-subagent`
- [ ] `github.com/weave-agent/weave-anthropic`
- [ ] `github.com/weave-agent/weave-openai`
- [ ] `github.com/weave-agent/weave-zai`
- [ ] `github.com/weave-agent/weave-kimi`
- [ ] `github.com/weave-agent/weave-agent`
- [ ] `github.com/weave-agent/weave-sandbox`
- [ ] `github.com/weave-agent/weave-sandbox-ui`
- [ ] `github.com/weave-agent/weave-jsonl`
- [ ] `github.com/weave-agent/weave-tui`
- [ ] `github.com/weave-agent/weave-diff-viewer`

For each:
- Copy extension contents to temp dir, update go.mod/imports
- `git init`, `git add`, `git commit`, `git push` to `git@github.com:weave-agent/weave-<name>.git`
- Remove from main repo `extensions/`
- Verify `weave install github.com/weave-agent/weave-<name> --name <name>` works

### Task 6b: Push main repo

- [ ] Push main `weave` repo to `git@github.com:weave-agent/weave.git`
  - After all extension code is removed from `extensions/`
  - `git remote add origin git@github.com:weave-agent/weave.git` (or update existing)
  - `git push`

### Task 7: Implement first-run bootstrap

- [ ] Create `internal/extmanage/bootstrap.go` with `BootstrapCoreExtensions(homeDir string) error`
  - Checks if `~/.weave/extensions/` has core extensions installed
  - If not, runs equivalent of `weave install github.com/weave-agent/weave-<name> --name <name>` for each core extension
  - Core list: bash, read, edit, write, grep, find, ls, subagent, anthropic, openai, zai, kimi, agent, sandbox, sandbox-ui, jsonl, tui, diff-viewer
- [ ] Call bootstrap in `internal/wire/run.go` before launcher runs, if `~/.weave/extensions/` is empty
- [ ] Add `--skip-bootstrap` flag to skip auto-install
- [ ] Write tests for bootstrap logic
- [ ] Run tests — must pass

### Task 8: Update extension management commands

- [ ] Update `weave list` to show module path alongside name
- [ ] Update `weave update` to handle extensions with `github.com/weave-agent/` prefix
- [ ] Verify `weave uninstall` still works
- [ ] Write tests for updated commands
- [ ] Run tests — must pass

### Task 9: Verify acceptance criteria

- [ ] Verify root module path is `github.com/weave-agent/weave`
- [ ] Verify all extension module paths are `github.com/weave-agent/weave-<name>`
- [ ] Verify `go build ./cmd/weave` succeeds
- [ ] Verify `./weave -p "hello"` works headlessly
- [ ] Verify `./weave` interactive mode loads all tools and TUI
- [ ] Verify `weave install github.com/user/weave-bash --name bash` shadows official version
- [ ] Verify forked extension builds and runs correctly
- [ ] Run `make test` — full suite must pass
- [ ] Run `make lint` — must pass

### Task 10: Update documentation

- [ ] Update `docs/design.md` references to old module paths
- [ ] Update README with new extension development guide
- [ ] Document how to create a new extension repo (template, go.mod structure)
- [ ] Document fork/customize workflow

## Technical Details

### Module path mapping

| Extension Dir | Old Module Path | New Module Path |
|---------------|----------------|-----------------|
| `extensions/tools/bash` | `weave/ext/tools/bash` | `github.com/weave-agent/weave-bash` |
| `extensions/tools/read` | `weave/ext/tools/read` | `github.com/weave-agent/weave-read` |
| `extensions/tools/edit` | `weave/ext/tools/edit` | `github.com/weave-agent/weave-edit` |
| `extensions/tools/write` | `weave/ext/tools/write` | `github.com/weave-agent/weave-write` |
| `extensions/tools/grep` | `weave/ext/tools/grep` | `github.com/weave-agent/weave-grep` |
| `extensions/tools/find` | `weave/ext/tools/find` | `github.com/weave-agent/weave-find` |
| `extensions/tools/ls` | `weave/ext/tools/ls` | `github.com/weave-agent/weave-ls` |
| `extensions/tools/subagent` | `weave/ext/tools/subagent` | `github.com/weave-agent/weave-subagent` |
| `extensions/providers/anthropic` | `weave/ext/providers/anthropic` | `github.com/weave-agent/weave-anthropic` |
| `extensions/providers/openai` | `weave/ext/providers/openai` | `github.com/weave-agent/weave-openai` |
| `extensions/providers/zai` | `weave/ext/providers/zai` | `github.com/weave-agent/weave-zai` |
| `extensions/providers/kimi` | `weave/ext/providers/kimi` | `github.com/weave-agent/weave-kimi` |
| `extensions/agent` | `weave/ext/agent` | `github.com/weave-agent/weave-agent` |
| `extensions/sandbox` | `weave/extensions/sandbox` | `github.com/weave-agent/weave-sandbox` |
| `extensions/ui/sandbox-ui` | `weave/ext/ui/sandboxui` | `github.com/weave-agent/weave-sandbox-ui` |
| `extensions/store/jsonl` | `weave/ext/store/jsonl` | `github.com/weave-agent/weave-jsonl` |
| `extensions/ui/tui` | `weave/ext/ui/tui` | `github.com/weave-agent/weave-tui` |
| `extensions/ui/tui/extensions/diff-viewer` | `weave/ext/ui/tui/extensions/diff-viewer` | `github.com/weave-agent/weave-diff-viewer` |
| `extensions/providers/codex` | `weave/ext/providers/codex` | `github.com/weave-agent/weave-codex` |
| `extensions/ui/tui/extensions/subagent-ui` | `weave/ext/ui/tui/extensions/subagent-ui` | `github.com/weave-agent/weave-subagent-ui` |

### Generated temp go.mod structure

```go
module weave-built

go 1.26.2

require (
	github.com/weave-agent/weave v0.0.0
	github.com/weave-agent/weave-bash v0.0.0
	github.com/weave-agent/weave-anthropic v0.0.0
	// ... other extensions
)

replace github.com/weave-agent/weave => /path/to/main/repo
replace github.com/weave-agent/weave-bash => /Users/.../.weave/extensions/bash
replace github.com/weave-agent/weave-anthropic => /Users/.../.weave/extensions/anthropic
// ... etc
```

### Import path changes

All files in main repo:
```go
// Before
import "weave/sdk"
import "weave/bus"
import "weave/config"

// After
import "github.com/weave-agent/weave/sdk"
import "github.com/weave-agent/weave/bus"
import "github.com/weave-agent/weave/config"
```

All files in extensions:
```go
// Before
import "weave/sdk"
import "weave/sdk/model"
import "weave/bus"
import "weave/utils/truncate"

// After
import "github.com/weave-agent/weave/sdk"
import "github.com/weave-agent/weave/sdk/model"
import "github.com/weave-agent/weave/bus"
import "github.com/weave-agent/weave/utils/truncate"
```

### Bootstrap behavior

On first run when `~/.weave/extensions/` is empty:
```
$ weave
weave: installing core extensions...
  → bash
  → read
  → edit
  → ...
```

With `--skip-bootstrap`:
```
$ weave --skip-bootstrap
weave: no extensions found. Run without --skip-bootstrap to install core extensions.
```

## Developer Workflow After Migration

### Where extensions live

**For end users:**
- Core extensions auto-install to `~/.weave/extensions/` on first run
- Custom extensions installed via `weave install <url> --name <name>`
- No `extensions/` directory in the main repo

**For weave core development** (working on SDK, launcher, bus):
```bash
# Clone main repo only
git clone git@github.com:weave-agent/weave.git
cd weave

# Run weave — bootstrap installs core extensions to ~/.weave/extensions/
go run ./cmd/weave

# Or pre-install specific extensions for testing
weave install github.com/weave-agent/weave-bash --name bash
```

**For extension development** (working on a specific extension):
```bash
# Clone extension repo
git clone git@github.com:weave-agent/weave-bash.git
cd weave-bash

# Add temporary replace for local SDK (don't commit this)
echo 'replace github.com/weave-agent/weave => /path/to/local/weave' >> go.mod

# Develop and test the extension standalone
go test ./...
```

**For full-stack development** (weave + extension changes together):
```bash
# In your weave project, clone extensions locally
cd /path/to/weave
mkdir -p .weave/extensions
git clone git@github.com:weave-agent/weave-bash.git .weave/extensions/bash
git clone git@github.com:weave-agent/weave-anthropic.git .weave/extensions/anthropic
# ... etc

# Project-local extensions shadow ~/.weave/extensions/ automatically
go run ./cmd/weave
```

Auto-discovery order ensures `.weave/extensions/` (project-local) takes precedence over `~/.weave/extensions/` (global), so local forks are used automatically.

## Post-Completion

**Manual verification:**
- Clone `weave` fresh, build, run — verify bootstrap installs extensions
- Fork `weave-bash`, install fork, verify it shadows official
- Test on clean machine (no `~/.weave/` directory)

**External system updates:**
- Create GitHub repos under `github.com/weave-agent` for each extension
- Set up CI for each extension repo (run tests on PR)
- Update `weave` repo CI to handle missing `extensions/` directory

**Configuration migration:**
- Existing `~/.weave/extensions/` clones will continue to work (names unchanged)
- Users with old configs don't need to change anything
- Extension developers need to update their forks' go.mod and imports
