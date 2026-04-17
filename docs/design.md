# Agent Core — Design Document

## Overview

A coding agent framework written in Go. Event-driven, extension-based, with dynamic compilation of selected extensions at runtime. Inspired by [Pi Mono](https://github.com/badlogic/pi-mono).

**Principles:**
- Standard library as much as possible
- Every replaceable component is an extension (runner, provider, tools, store, hooks)
- Extensions are independent Go modules, installed globally
- Per-project config selects which extensions to activate via explicit slots
- Channel-based event bus with string topics, extensible by extensions
- Fast recompile via Go build cache + per-hash binary caching
- Single external dependency: gonfig

---

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│  User Project (~/projects/my-app/)                      │
│  ┌──────────────┐                                       │
│  │ .agent.yaml  │  ← per-project config + slot mapping  │
│  └──────────────┘                                       │
│  ┌──────────────────────┐                               │
│  │ .agent/extensions/   │  ← project-local extensions   │
│  └──────────────────────┘                               │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│  Global (~/.agent/)                                     │
│  ┌──────────────────────┐                               │
│  │ extensions/           │  ← installed extensions      │
│  │   llmprovider/        │     (Go modules)             │
│  │   sandbox-bash/       │                              │
│  │   lint/               │                              │
│  ├──────────────────────┤                               │
│  │ bin/{hash}/           │  ← compiled binaries         │
│  │   agent               │                              │
│  │   meta.json           │  ← build metadata            │
│  │ tmp/{hash}/           │  ← generated build dirs      │
│  └──────────────────────┘                               │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│  Agent Binary (per-hash compiled)                       │
│  ┌───────────┐ ┌───────┐ ┌───────────────────────────┐ │
│  │ Runner    │ │ Event │ │ Extensions                │ │
│  │ (turn-    │ │ Bus   │ │ (Provider, Tools, Store,  │ │
│  │  based)   │ │       │ │  Hooks)                   │ │
│  └───────────┘ └───────┘ └───────────────────────────┘ │
│  All are extensions. Runner is resolved via slot.       │
└─────────────────────────────────────────────────────────┘
```

---

## Package Structure

```
agent/
├── cmd/
│   └── agent/
│       └── main.go               # thin CLI: run, ext, builds, gc
│
├── sdk/                          # ONLY package extensions import
│   ├── provider.go               # Provider, ChatRequest, ChatResponse, Message
│   ├── runner.go                 # Runner interface, RunOpts
│   ├── tool.go                   # Tool, ToolResult, ToolCall
│   ├── store.go                  # Store interface, Entry, Session
│   ├── extension.go              # Extension interface (hooks)
│   ├── event.go                  # Event struct, topic constants
│   ├── registry.go               # Register functions, Wire()
│   └── config.go                 # Config interface
│
├── bus/                          # event bus (infrastructure, not extension)
│   └── bus.go
│
├── cfg/                          # config loading (infrastructure)
│   └── cfg.go
│
├── launcher/                     # build management
│   ├── launcher.go               # config resolution, orchestration
│   ├── builder.go                # generate go.mod/main.go, compile
│   ├── cache.go                  # file-based build cache
│   └── install.go                # ext install/remove/list
│
├── extensions/                   # built-in extensions = reference examples
│   ├── tools/                    # ALL built-in tools in one package
│   │   ├── tools.go              # init() registers all tools
│   │   ├── bash.go
│   │   ├── read.go
│   │   ├── write.go
│   │   ├── edit.go
│   │   ├── grep.go
│   │   └── glob.go
│   ├── llmprovider/              # LLM provider
│   │   └── provider.go
│   ├── jsonlstore/               # JSONL session persistence
│   │   └── store.go
│   ├── turnrunner/               # turn-based agent loop
│   │   └── runner.go
│   └── lint/                     # example hook extension
│       └── lint.go
│
└── go.mod
```

**Why flat, not `pkg/` or `internal/`:**
- Each top-level directory is one package, one concern
- Extensions only import `sdk/` — everything else is irrelevant to them
- `bus/` and `cfg/` are infrastructure, not replaceable (created before extensions)
- `launcher/` is used only by `cmd/agent/`, testable independently
- Go favors flat structures — no artificial grouping

---

## SDK Interfaces

### Runner (the agent loop)

```go
// sdk/runner.go
package sdk

import "context"

type Runner interface {
    Run(ctx context.Context, opts RunOpts) error
}

type RunOpts struct {
    Prompt    string
    ResumeID  string               // empty = new session
    Config    Config
    Bus       *Bus
    Providers map[string]Provider  // all active providers, keyed by name
    Tools     map[string]Tool
    Store     Store
}
```

### Provider (LLM)

```go
// sdk/provider.go
package sdk

import "context"

type Provider interface {
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
    Models() []string
}

type ChatRequest struct {
    Messages []Message     `json:"messages"`
    Model    string        `json:"model"`
    Tools    []ToolDef     `json:"tools,omitempty"`
    Stream   bool          `json:"stream"`
}

type ChatResponse struct {
    Content   string       `json:"content"`
    Message   Message      `json:"message"`
    ToolCalls []ToolCall   `json:"tool_calls,omitempty"`
    Done      bool         `json:"done"`
}

type Message struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

type ToolDef struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    Parameters  json.RawMessage `json:"parameters"`
}

type ToolCall struct {
    ID    string          `json:"id"`
    Tool  string          `json:"tool"`
    Input json.RawMessage `json:"input"`
}
```

### Tool

```go
// sdk/tool.go
package sdk

import "context"

type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage
    Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}

type ToolResult struct {
    ID     string          `json:"id"`
    Result json.RawMessage `json:"result"`
    Error  string          `json:"error,omitempty"`
}
```

### Store (session persistence)

```go
// sdk/store.go
package sdk

import "context"

type Store interface {
    Create(ctx context.Context, opts StoreOpts) (*Session, error)
    Load(ctx context.Context, id string) (*Session, error)
    Append(ctx context.Context, sessionID string, entry Entry) error
    History(ctx context.Context, sessionID string) ([]Entry, error)
    Fork(ctx context.Context, sessionID string, fromEntryID string) (string, error)
    List(ctx context.Context) ([]SessionInfo, error)
    Compact(ctx context.Context, sessionID string, keepLast int) error
}

type StoreOpts struct {
    Name string
}

type Entry struct {
    ID       string          `json:"id"`
    ParentID string          `json:"parent_id,omitempty"`
    Type     string          `json:"type"`
    Turn     int             `json:"turn"`
    Data     json.RawMessage `json:"data"`
    Meta     map[string]any  `json:"meta,omitempty"`
    Created  time.Time       `json:"created"`
}

type Session struct {
    ID      string
    Entries []Entry
}

type SessionInfo struct {
    ID        string
    Name      string
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

### Extension (hooks)

```go
// sdk/extension.go
package sdk

type Extension interface {
    Name() string
    Subscribe(bus *Bus)
}
```

### Config

```go
// sdk/config.go
package sdk

type Config interface {
    GetString(key string) string
    GetInt(key string) int
    GetDuration(key string) time.Duration
    GetBool(key string) bool
    GetStringSlice(key string) []string
    GetStringMap(key string) map[string]any
    Sub(key string) Config
}
```

---

## Registry + Wire

Extensions self-register via `init()` using factory functions. No priority numbers. Resolution is config-driven:
- **Slots** (runner, store) — config explicitly maps which extension fills the role
- **Providers** — all listed providers are instantiated, runner picks by model name
- **Tools** — last-wins by config `extensions` list order, name from `Tool.Name()`
- **Extensions** (hooks) — all are instantiated and subscribed to the event bus

```go
// sdk/registry.go
package sdk

import "sync"

type Factory func(cfg Config) (any, error)

type Registration struct {
    Name    string
    Factory Factory
}

type toolWrapper func(name string, tool Tool) Tool

var (
    mu           sync.Mutex
    runners      map[string]Registration
    providers    map[string]Registration
    stores       map[string]Registration
    tools        map[string]toolRegistration // key: extName
    extensions   map[string]Registration
    toolWrappers []toolWrapper
)

type toolRegistration struct {
    Factory Factory
}

func init() {
    runners = make(map[string]Registration)
    providers = make(map[string]Registration)
    stores = make(map[string]Registration)
    tools = make(map[string]toolRegistration)
    extensions = make(map[string]Registration)
}

func RegisterRunner(name string, factory Factory) {
    mu.Lock()
    runners[name] = Registration{Name: name, Factory: factory}
    mu.Unlock()
}

func RegisterProvider(name string, factory Factory) {
    mu.Lock()
    providers[name] = Registration{Name: name, Factory: factory}
    mu.Unlock()
}

func RegisterStore(name string, factory Factory) {
    mu.Lock()
    stores[name] = Registration{Name: name, Factory: factory}
    mu.Unlock()
}

// RegisterTool registers a tool factory. The tool name is determined
// by Tool.Name() after instantiation — no hardcoded tool names.
// extName identifies the providing extension package.
func RegisterTool(extName string, factory Factory) {
    mu.Lock()
    tools[extName] = toolRegistration{Factory: factory}
    mu.Unlock()
}

func RegisterExtension(name string, factory Factory) {
    mu.Lock()
    extensions[name] = Registration{Name: name, Factory: factory}
    mu.Unlock()
}

func RegisterToolWrapper(w toolWrapper) {
    mu.Lock()
    toolWrappers = append(toolWrappers, w)
    mu.Unlock()
}

// Wire is the composition root. Resolves slots from config,
// instantiates extensions, returns an assembled Runner.
func Wire(config Config, bus *Bus) (Runner, error) {
    extOrder := config.GetStringSlice("extensions")

    // --- resolve slots (runner, store) ---

    runnerReg, ok := runners[config.GetString("slots.runner")]
    if !ok {
        return nil, fmt.Errorf("slot runner: %q not registered", config.GetString("slots.runner"))
    }

    storeReg, ok := stores[config.GetString("slots.store")]
    if !ok {
        return nil, fmt.Errorf("slot store: %q not registered", config.GetString("slots.store"))
    }
    storeInstance, err := storeReg.Factory(config)
    if err != nil {
        return nil, fmt.Errorf("wire store: %w", err)
    }

    // --- resolve providers: all listed providers are instantiated ---

    resolvedProviders := map[string]Provider{}
    for _, extName := range extOrder {
        reg, ok := providers[extName]
        if !ok {
            continue
        }
        instance, err := reg.Factory(config.Sub("provider"))
        if err != nil {
            return nil, fmt.Errorf("wire provider %s: %w", extName, err)
        }
        resolvedProviders[extName] = instance.(Provider)
    }

    // --- resolve tools: instantiate, get name from Tool.Name(), last-wins by config order ---

    resolvedTools := map[string]Tool{}
    for _, extName := range extOrder {
        reg, ok := tools[extName]
        if !ok {
            continue
        }
        instance, err := reg.Factory(config.Sub("tools"))
        if err != nil {
            return nil, fmt.Errorf("wire tools from %s: %w", extName, err)
        }
        // one extension can provide multiple tools
        if t, ok := instance.(Tool); ok {
            resolvedTools[t.Name()] = t
        }
        // extension can also return a []Tool
        if ts, ok := instance.([]Tool); ok {
            for _, t := range ts {
                resolvedTools[t.Name()] = t
            }
        }
    }

    // apply tool wrappers
    for _, wrap := range toolWrappers {
        for name, t := range resolvedTools {
            resolvedTools[name] = wrap(name, t)
        }
    }

    // --- instantiate all hook extensions ---

    for _, extName := range extOrder {
        reg, ok := extensions[extName]
        if !ok {
            continue
        }
        instance, err := reg.Factory(config)
        if err != nil {
            return nil, fmt.Errorf("wire extension %s: %w", extName, err)
        }
        if ext, ok := instance.(Extension); ok {
            ext.Subscribe(bus)
        }
    }

    // --- instantiate runner with assembled dependencies ---

    runnerInstance, err := runnerReg.Factory(config)
    if err != nil {
        return nil, fmt.Errorf("wire runner %s: %w", runnerName, err)
    }

    return &wiredRunner{
        runner: runnerInstance.(Runner),
        opts: RunOpts{
            Config:    config,
            Bus:       bus,
            Providers: resolvedProviders,
            Tools:     resolvedTools,
            Store:     storeInstance.(Store),
        },
    }, nil
}

// wiredRunner wraps the resolved runner with pre-wired opts
type wiredRunner struct {
        runner Runner
        opts   RunOpts
}

func (w *wiredRunner) Run(ctx context.Context, prompt string) error {
    w.opts.Prompt = prompt
    return w.runner.Run(ctx, w.opts)
}
```

---

## Event Bus

Channel-based pub/sub with string topics. Infrastructure, not an extension — created before any extension is wired.

```go
// bus/bus.go
package bus

import "sync"

type Event struct {
    Topic   string         `json:"topic"`
    Turn    int            `json:"turn"`
    Payload any            `json:"payload"`
    Meta    map[string]any `json:"meta,omitempty"`
    Error   error          `json:"-"`
}

type Bus struct {
    mu      sync.RWMutex
    subs    map[string][]chan Event
    allSubs []chan Event
}

func New() *Bus {
    return &Bus{subs: make(map[string][]chan Event)}
}

func (b *Bus) Subscribe(topics ...string) <-chan Event {
    ch := make(chan Event, 64)
    b.mu.Lock()
    for _, t := range topics {
        b.subs[t] = append(b.subs[t], ch)
    }
    b.mu.Unlock()
    return ch
}

func (b *Bus) SubscribeAll() <-chan Event {
    ch := make(chan Event, 256)
    b.mu.Lock()
    b.allSubs = append(b.allSubs, ch)
    b.mu.Unlock()
    return ch
}

func (b *Bus) Publish(evt Event) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    for _, ch := range b.subs[evt.Topic] {
        select {
        case ch <- evt:
        default:
        }
    }
    for _, ch := range b.allSubs {
        select {
        case ch <- evt:
        default:
        }
    }
}
```

### Built-in Event Topics

```go
// sdk/event.go
package sdk

const (
    AgentStart      = "agent.start"
    AgentEnd        = "agent.end"
    TurnStart       = "turn.start"
    TurnEnd         = "turn.end"
    MessageStart    = "message.start"
    MessageUpdate   = "message.update"
    MessageEnd      = "message.end"
    ToolCallStart   = "tool.call_start"
    ToolCallUpdate  = "tool.call_update"
    ToolCallEnd     = "tool.call_end"
    ToolResult      = "tool.result"
    Error           = "error"
)
// Extensions may define additional topics freely.
```

---

## Built-in Extensions

### Turn Runner (default agent loop)

```go
// extensions/turnrunner/runner.go
package turnrunner

import (
    "context"
    "agent/sdk"
)

func init() {
    sdk.RegisterRunner("turn-based", func(cfg sdk.Config) (any, error) {
        return &TurnRunner{}, nil
    })
}

type TurnRunner struct{}

func (r *TurnRunner) Run(ctx context.Context, opts sdk.RunOpts) error {
    bus := opts.Bus

    session, err := opts.Store.Create(ctx, sdk.StoreOpts{})
    if err != nil {
        return err
    }

    bus.Publish(sdk.Event{Topic: sdk.AgentStart, Payload: session.ID})

    messages := []sdk.Message{{Role: "user", Content: opts.Prompt}}
    turn := 0

    // build model→provider index from all providers
    modelIndex := buildModelIndex(opts.Providers)

    for {
        turn++
        bus.Publish(sdk.Event{Topic: sdk.TurnStart, Turn: turn})

        model := opts.Config.GetString("provider.model")
        provider := modelIndex.providerFor(model)

        resp, err := provider.Chat(ctx, sdk.ChatRequest{
            Messages: messages,
            Model:    model,
            Tools:    toolDefs(opts.Tools),
            Stream:   true,
        })
        if err != nil {
            bus.Publish(sdk.Event{Topic: sdk.Error, Error: err})
            return err
        }

        bus.Publish(sdk.Event{Topic: sdk.MessageEnd, Turn: turn, Payload: resp.Content})
        messages = append(messages, resp.Message)

        opts.Store.Append(ctx, session.ID, sdk.Entry{
            Type: "assistant", Turn: turn, Data: marshal(resp),
        })

        bus.Publish(sdk.Event{Topic: sdk.TurnEnd, Turn: turn})

        if len(resp.ToolCalls) == 0 {
            break
        }

        for _, tc := range resp.ToolCalls {
            bus.Publish(sdk.Event{Topic: sdk.ToolCallStart, Turn: turn, Payload: tc})

            tool, ok := opts.Tools[tc.Tool]
            if !ok {
                bus.Publish(sdk.Event{Topic: sdk.ToolResult, Turn: turn,
                    Payload: sdk.ToolResult{ID: tc.ID, Error: "unknown tool: " + tc.Tool}})
                continue
            }

            result, err := tool.Execute(ctx, tc.Input)
            tr := sdk.ToolResult{ID: tc.ID, Result: result}
            if err != nil {
                tr.Error = err.Error()
            }

            bus.Publish(sdk.Event{Topic: sdk.ToolResult, Turn: turn, Payload: tr})
            messages = append(messages, sdk.Message{Role: "tool", Content: string(result)})
            opts.Store.Append(ctx, session.ID, sdk.Entry{
                Type: "tool_result", Turn: turn, Data: marshal(tr),
            })
        }
    }

    bus.Publish(sdk.Event{Topic: sdk.AgentEnd})
    return nil
}

func toolDefs(tools map[string]sdk.Tool) []sdk.ToolDef {
    var defs []sdk.ToolDef
    for _, t := range tools {
        defs = append(defs, sdk.ToolDef{
            Name: t.Name(), Description: t.Description(), Parameters: t.Parameters(),
        })
    }
    return defs
}

// modelIndex maps model names to their provider
type modelIndex struct {
    models     map[string]sdk.Provider // model name → provider
    providers  map[string]sdk.Provider // provider name → provider (fallback)
}

func buildModelIndex(providers map[string]sdk.Provider) *modelIndex {
    idx := &modelIndex{
        models:    make(map[string]sdk.Provider),
        providers: providers,
    }
    for name, p := range providers {
        for _, model := range p.Models() {
            idx.models[model] = p
        }
    }
    return idx
}

func (idx *modelIndex) providerFor(model string) sdk.Provider {
    if p, ok := idx.models[model]; ok {
        return p
    }
    // fallback: first provider
    for _, p := range idx.providers {
        return p
    }
    return nil
}
```

### Built-in Tools (all in one package)

```go
// extensions/tools/tools.go
package tools

import "agent/sdk"

func init() {
    sdk.RegisterTool("tools", func(cfg sdk.Config) (any, error) {
        return []sdk.Tool{
            newBash(cfg),
            newRead(cfg),
            newWrite(cfg),
            newEdit(cfg),
            newGrep(cfg),
            newGlob(cfg),
        }, nil
    })
}
```

```go
// extensions/tools/bash.go
package tools

import (
    "context"
    "os/exec"
    "time"

    "agent/sdk"
)

type bash struct {
    timeout time.Duration
}

func newBash(cfg sdk.Config) *bash {
    return &bash{timeout: cfg.GetDuration("bash.timeout")}
}

func (b *bash) Name() string        { return "bash" }
func (b *bash) Description() string { return "Execute shell commands" }

func (b *bash) Parameters() json.RawMessage {
    return json.RawMessage(`{
        "type": "object",
        "properties": {
            "command": {"type": "string", "description": "The command to execute"}
        },
        "required": ["command"]
    }`)
}

func (b *bash) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
    var args struct{ Command string `json:"command"` }
    json.Unmarshal(input, &args)

    ctx, cancel := context.WithTimeout(ctx, b.timeout)
    defer cancel()

    cmd := exec.CommandContext(ctx, "bash", "-c", args.Command)
    out, err := cmd.CombinedOutput()

    result, _ := json.Marshal(map[string]string{"output": string(out)})
    return result, err
}
```

```go
// extensions/tools/read.go
package tools

import "context"

type read struct{}

func newRead(cfg sdk.Config) *read { return &read{} }

func (r *read) Name() string        { return "read" }
func (r *read) Description() string { return "Read file contents" }

func (r *read) Parameters() json.RawMessage {
    return json.RawMessage(`{
        "type": "object",
        "properties": {
            "path": {"type": "string", "description": "File path to read"}
        },
        "required": ["path"]
    }`)
}

func (r *read) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
    var args struct{ Path string `json:"path"` }
    json.Unmarshal(input, &args)
    data, err := os.ReadFile(args.Path)
    if err != nil {
        return nil, err
    }
    result, _ := json.Marshal(map[string]string{"content": string(data)})
    return result, nil
}
```

### JSONL Store (default session persistence)

```go
// extensions/jsonlstore/store.go
package jsonlstore

import (
    "context"
    "agent/sdk"
)

func init() {
    sdk.RegisterStore("jsonl", func(cfg sdk.Config) (any, error) {
        dir := cfg.GetString("session.dir")
        if dir == "" {
            dir = filepath.Join(os.UserHomeDir(), ".agent", "sessions")
        }
        os.MkdirAll(dir, 0755)
        return &JSONLStore{dir: dir}, nil
    })
}

type JSONLStore struct {
    dir string
}

func (s *JSONLStore) Create(ctx context.Context, opts sdk.StoreOpts) (*sdk.Session, error) {
    id := nanoid()
    return &sdk.Session{ID: id}, nil
}

func (s *JSONLStore) Append(ctx context.Context, sid string, entry sdk.Entry) error {
    entry.ID = nanoid()
    entry.Created = time.Now()
    data, _ := json.Marshal(entry)
    f, err := os.OpenFile(
        filepath.Join(s.dir, sid+".jsonl"),
        os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644,
    )
    if err != nil {
        return err
    }
    defer f.Close()
    _, err = f.Write(append(data, '\n'))
    return err
}

func (s *JSONLStore) Load(ctx context.Context, id string) (*sdk.Session, error) {
    entries, err := s.readAll(id)
    if err != nil {
        return nil, err
    }
    return &sdk.Session{ID: id, Entries: entries}, nil
}

func (s *JSONLStore) History(ctx context.Context, sid string) ([]sdk.Entry, error) {
    session, err := s.Load(ctx, sid)
    if err != nil {
        return nil, err
    }
    return linearPath(session.Entries), nil
}

func (s *JSONLStore) Fork(ctx context.Context, sid string, fromEntryID string) (string, error) {
    newID := nanoid()
    entries, _ := s.readAll(sid)
    for _, e := range entries {
        if e.ID == fromEntryID {
            e.Meta = map[string]any{"forked_from": sid}
        }
        data, _ := json.Marshal(e)
        f, _ := os.Create(filepath.Join(s.dir, newID+".jsonl"))
        f.Write(append(data, '\n'))
        f.Close()
    }
    return newID, nil
}

func (s *JSONLStore) Compact(ctx context.Context, sid string, keepLast int) error {
    entries, _ := s.readAll(sid)
    if len(entries) <= keepLast {
        return nil
    }
    old := entries[:len(entries)-keepLast]
    summary := summarizeOldEntries(old)
    s.Append(ctx, sid, sdk.Entry{
        Type: "summary",
        Data: summary,
        Meta: map[string]any{"compacted_count": len(old)},
    })
    return nil
}

func (s *JSONLStore) List(ctx context.Context) ([]sdk.SessionInfo, error) {
    entries, _ := os.ReadDir(s.dir)
    var infos []sdk.SessionInfo
    for _, e := range entries {
        if strings.HasSuffix(e.Name(), ".jsonl") {
            info, _ := e.Info()
            infos = append(infos, sdk.SessionInfo{
                ID:        strings.TrimSuffix(e.Name(), ".jsonl"),
                UpdatedAt: info.ModTime(),
            })
        }
    }
    return infos, nil
}
```

### LLM Provider

```go
// extensions/llmprovider/provider.go
package llmprovider

import (
    "context"
    "net/http"

    "agent/sdk"
)

func init() {
    sdk.RegisterProvider("llm-provider", func(cfg sdk.Config) (any, error) {
        return &Provider{
            apiKey: cfg.GetString("provider.api_key"),
            model:  cfg.GetString("provider.model"),
            client: &http.Client{},
        }, nil
    })
}

type Provider struct {
    apiKey string
    model  string
    client *http.Client
}

func (p *Provider) Chat(ctx context.Context, req sdk.ChatRequest) (*sdk.ChatResponse, error) {
    // call Messages API
    return &sdk.ChatResponse{Content: "", Done: true}, nil
}

func (p *Provider) Models() []string {
    return []string{"claude-opus-4-7", "claude-sonnet-4-6", "claude-haiku-4-5"}
}
```

### Lint Hook (example extension)

```go
// extensions/lint/lint.go
package lint

import (
    "agent/bus"
    "agent/sdk"
)

func init() {
    sdk.RegisterExtension("lint", func(cfg sdk.Config) (any, error) {
        return &LintHook{strict: cfg.GetBool("lint.strict")}, nil
    })
}

type LintHook struct {
    strict bool
}

func (l *LintHook) Name() string { return "lint" }

func (l *LintHook) Subscribe(b *bus.Bus) {
    ch := b.Subscribe(sdk.ToolCallStart)
    go func() {
        for evt := range ch {
            tc, ok := evt.Payload.(sdk.ToolCall)
            if !ok {
                continue
            }
            if tc.Tool == "bash" && l.isDangerous(tc.Input) {
                b.Publish(bus.Event{
                    Topic: "lint.violation",
                    Payload: map[string]any{
                        "tool":  tc.Tool,
                        "input": tc.Input,
                        "rule":  "dangerous_command",
                    },
                })
            }
        }
    }()
}

func (l *LintHook) isDangerous(input json.RawMessage) bool { return false }
```

---

## Launcher

The `agent` CLI with subcommands. Handles extension discovery, build caching, and execution.

### CLI Commands

```
agent run [-c config.yaml] [-e ext1,ext2,...] [-p prompt] [--resume id]
agent ext install <source>
agent ext remove <name>
agent ext list
agent builds
agent gc
```

### Config Resolution

```go
// launcher/launcher.go
package launcher

type ResolvedConfig struct {
    Extensions []string
    ConfigPath string
    Config     sdk.Config
}

func ResolveConfig(configFlag string, extFlag string) (*ResolvedConfig, error) {
    cfgPath := configFlag
    if cfgPath == "" {
        cfgPath = findConfigFile() // walk up from cwd: .agent.yaml, .agent/config.yaml
    }

    var cfg Config
    if cfgPath != "" {
        gonfig.Load(&cfg, gonfig.WithFile(cfgPath), gonfig.WithEnvPrefix("AGENT"))
    }

    var exts []string
    if extFlag != "" {
        exts = strings.Split(extFlag, ",")
    } else {
        exts = cfg.Extensions
    }

    return &ResolvedConfig{
        Extensions: exts,
        ConfigPath: cfgPath,
        Config:     wrapConfig(cfg),
    }, nil
}
```

### Extension Discovery

```go
func resolveExtensions(names []string) ([]Extension, error) {
    var exts []Extension
    for _, name := range names {
        // search order:
        //   1. .agent/extensions/{name}/     (project-local)
        //   2. ~/.agent/extensions/{name}/   (global)
        path, err := findExtension(name)
        if err != nil {
            return nil, fmt.Errorf("extension %q not found", name)
        }
        exts = append(exts, Extension{Name: name, Path: path})
    }
    return exts, nil
}

func findExtension(name string) (string, error) {
    local := filepath.Join(".agent", "extensions", name)
    if isGoModule(local) {
        return filepath.Abs(local)
    }
    global := filepath.Join(homeDir(), ".agent", "extensions", name)
    if isGoModule(global) {
        return global, nil
    }
    return "", fmt.Errorf("not found")
}
```

### Builder

```go
// launcher/builder.go
package launcher

func Build(exts []Extension, cfg *ResolvedConfig, cache *FileCache) (string, error) {
    hash := computeHash(exts)

    // check cache
    cached, _ := cache.GetBuild(hash)
    if cached != nil {
        binPath := filepath.Join(cache.binDir, hash, "agent")
        if _, err := os.Stat(binPath); err == nil {
            cache.TouchBuild(hash)
            return binPath, nil
        }
    }

    // create temp build dir
    buildDir := filepath.Join(homeDir(), ".agent", "tmp", hash)
    os.MkdirAll(buildDir, 0755)

    // generate go.mod with replace directives
    generateGoMod(buildDir, exts)

    // generate main.go with blank imports
    generateMain(buildDir, exts)

    // compile
    binDir := filepath.Join(homeDir(), ".agent", "bin", hash)
    os.MkdirAll(binDir, 0755)
    binaryPath := filepath.Join(binDir, "agent")

    cmd := exec.Command("go", "build", "-o", binaryPath, ".")
    cmd.Dir = buildDir
    if output, err := cmd.CombinedOutput(); err != nil {
        return "", fmt.Errorf("build failed: %s: %w", output, err)
    }

    // save to cache
    cache.SaveBuild(BuildMeta{
        Hash:       hash,
        Extensions: extNames(exts),
        GoVersion:  runtime.Version(),
        BuiltAt:    time.Now(),
        LastUsed:   time.Now(),
        ConfigPath: cfg.ConfigPath,
    })

    return binaryPath, nil
}

func computeHash(exts []Extension) string {
    h := sha256.New()
    fmt.Fprintf(h, "go:%s\n", runtime.Version())
    fmt.Fprintf(h, "sdk:%s\n", SDKVersion)
    for _, e := range exts {
        filepath.WalkDir(e.Path, func(path string, d fs.DirEntry, err error) error {
            if !d.IsDir() && strings.HasSuffix(path, ".go") {
                data, _ := os.ReadFile(path)
                h.Write(data)
            }
            return nil
        })
    }
    return hex.EncodeToString(h.Sum(nil))[:16]
}
```

### Generated Files

```go
// ~/.agent/tmp/{hash}/go.mod (generated)
module agent-build

go 1.24

require (
    agent v0.0.0
    agent-ext-llmprovider v0.0.0
    agent-ext-sandbox v0.0.0
    agent-ext-lint v0.0.0
)

replace (
    agent => /opt/agent
    agent-ext-llmprovider => ~/.agent/extensions/llmprovider
    agent-ext-sandbox => ~/.agent/extensions/sandbox-bash
    agent-ext-lint => ~/.agent/extensions/lint
)
```

```go
// ~/.agent/tmp/{hash}/main.go (generated)
// Code generated by agent launcher. DO NOT EDIT.
package main

import (
    // built-in extensions
    _ "agent/extensions/llmprovider"
    _ "agent/extensions/tools"
    _ "agent/extensions/jsonlstore"
    _ "agent/extensions/turnrunner"

    // external extensions
    _ "agent-ext-sandbox"              # overrides bash from tools (last wins)
    _ "agent-ext-lint"

    "agent/bus"
    "agent/cfg"
    "agent/sdk"
)

func main() {
    config := cfg.Load()
    eventBus := bus.New()

    runner, err := sdk.Wire(config, eventBus)
    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }

    runner.Run(context.Background(), readPrompt())
}
```

### Build Cache (file-based, stdlib only)

Each build gets its own directory with a `meta.json` next to the binary.

```
~/.agent/
  bin/
    a1b2c3/
      agent          # compiled binary
      meta.json      # build metadata
    d4e5f6/
      agent
      meta.json
```

```go
// launcher/cache.go
package launcher

type BuildMeta struct {
    Hash       string    `json:"hash"`
    Extensions []string  `json:"extensions"`
    GoVersion  string    `json:"go_version"`
    BuiltAt    time.Time `json:"built_at"`
    LastUsed   time.Time `json:"last_used"`
    ConfigPath string    `json:"config_path"`
}

type ExtMeta struct {
    Name        string    `json:"name"`
    Path        string    `json:"path"`
    Source      string    `json:"source"`       // "git", "local"
    Version     string    `json:"version"`
    InstalledAt time.Time `json:"installed_at"`
}

type FileCache struct {
    baseDir string
    binDir  string
    extDir  string
}

func NewFileCache(baseDir string) *FileCache {
    return &FileCache{
        baseDir: baseDir,
        binDir:  filepath.Join(baseDir, "bin"),
        extDir:  filepath.Join(baseDir, "extensions"),
    }
}

func (c *FileCache) GetBuild(hash string) (*BuildMeta, error) {
    data, err := os.ReadFile(filepath.Join(c.binDir, hash, "meta.json"))
    if os.IsNotExist(err) {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }
    var meta BuildMeta
    if err := json.Unmarshal(data, &meta); err != nil {
        return nil, err
    }
    return &meta, nil
}

func (c *FileCache) SaveBuild(meta BuildMeta) error {
    dir := filepath.Join(c.binDir, meta.Hash)
    os.MkdirAll(dir, 0755)
    data, _ := json.MarshalIndent(meta, "", "  ")
    return os.WriteFile(filepath.Join(dir, "meta.json"), data, 0644)
}

func (c *FileCache) TouchBuild(hash string) error {
    meta, err := c.GetBuild(hash)
    if err != nil || meta == nil {
        return err
    }
    meta.LastUsed = time.Now()
    return c.SaveBuild(*meta)
}

func (c *FileCache) FindByExtensions(exts []string) (*BuildMeta, error) {
    target := sortedJoined(exts)
    builds, _ := c.ListBuilds()
    for i := range builds {
        if sortedJoined(builds[i].Extensions) == target {
            binPath := filepath.Join(c.binDir, builds[i].Hash, "agent")
            if _, err := os.Stat(binPath); err == nil {
                return &builds[i], nil
            }
        }
    }
    return nil, nil
}

func (c *FileCache) ListBuilds() ([]BuildMeta, error) {
    entries, err := os.ReadDir(c.binDir)
    if os.IsNotExist(err) {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }
    var builds []BuildMeta
    for _, e := range entries {
        if !e.IsDir() {
            continue
        }
        meta, _ := c.GetBuild(e.Name())
        if meta != nil {
            builds = append(builds, *meta)
        }
    }
    sort.Slice(builds, func(i, j int) bool {
        return builds[i].LastUsed.After(builds[j].LastUsed)
    })
    return builds, nil
}

func (c *FileCache) GC(maxAge time.Duration) (int, error) {
    builds, err := c.ListBuilds()
    if err != nil {
        return 0, err
    }
    removed := 0
    for _, b := range builds {
        if time.Since(b.LastUsed) > maxAge {
            os.RemoveAll(filepath.Join(c.binDir, b.Hash))
            removed++
        }
    }
    return removed, nil
}

func (c *FileCache) ListExtensions() ([]ExtMeta, error) {
    entries, err := os.ReadDir(c.extDir)
    if os.IsNotExist(err) {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }
    var exts []ExtMeta
    for _, e := range entries {
        if !e.IsDir() {
            continue
        }
        data, err := os.ReadFile(filepath.Join(c.extDir, e.Name(), "meta.json"))
        if err != nil {
            continue
        }
        var meta ExtMeta
        if err := json.Unmarshal(data, &meta); err != nil {
            continue
        }
        exts = append(exts, meta)
    }
    return exts, nil
}

func (c *FileCache) SaveExtension(meta ExtMeta) error {
    dir := filepath.Join(c.extDir, meta.Name)
    data, _ := json.MarshalIndent(meta, "", "  ")
    return os.WriteFile(filepath.Join(dir, "meta.json"), data, 0644)
}

func (c *FileCache) RemoveExtension(name string) error {
    return os.RemoveAll(filepath.Join(c.extDir, name))
}

func sortedJoined(items []string) string {
    s := make([]string, len(items))
    copy(s, items)
    sort.Strings(s)
    return strings.Join(s, ",")
}
```

### Run Command

```go
// cmd/agent/main.go
package main

func main() {
    if len(os.Args) < 2 {
        fmt.Fprintln(os.Stderr, "usage: agent <command> [args]")
        os.Exit(1)
    }

    switch os.Args[1] {
    case "run":
        cmdRun(os.Args[2:])
    case "ext":
        cmdExt(os.Args[2:])
    case "builds":
        cmdBuilds()
    case "gc":
        cmdGC()
    }
}

func cmdRun(args []string) {
    flags := parseRunFlags(args)

    cfg := launcher.ResolveConfig(flags.Config, flags.Extensions)
    exts := launcher.ResolveExtensions(cfg.Extensions)
    cache := launcher.NewFileCache(filepath.Join(os.UserHomeDir(), ".agent"))
    binary, err := launcher.Build(exts, cfg, cache)
    if err != nil {
        log.Fatal(err)
    }

    argv := []string{binary}
    if flags.Config != "" {
        argv = append(argv, "-c", flags.Config)
    }
    if flags.Prompt != "" {
        argv = append(argv, "-p", flags.Prompt)
    }
    if flags.Resume != "" {
        argv = append(argv, "--resume", flags.Resume)
    }
    syscall.Exec(binary, argv, os.Environ())
}
```

---

## Project Config (.agent.yaml)

```yaml
extensions:
  - llmprovider              # Anthropic (claude-opus-4-7, claude-sonnet-4-6, ...)
  - openai-provider          # OpenAI (gpt-5, gpt-4o, ...)
  - tools                    # registers bash, read, write, edit, grep, glob
  - sandbox                  # also registers bash → overrides tools/bash (last wins)
  - jsonlstore
  - turnrunner
  - lint

slots:
  runner: turnrunner
  store: jsonl

provider:
  model: claude-sonnet-4-6   # resolved to llmprovider via model index
  fallback_model: gpt-5      # resolved to openai-provider

llmprovider:
  api_key: ${ANTHROPIC_API_KEY}

openai-provider:
  api_key: ${OPENAI_API_KEY}

tools:
  bash:
    timeout: 30s
    allowed_commands: ["git", "go", "npm", "cargo"]
  lint:
    strict: true

session:
  dir: .agent/sessions
  auto_compact: true
  compact_threshold: 50

extensions_config:
  llmprovider:
    max_tokens: 8192
```

---

## Data Flow

```
User types prompt
       │
       ▼
  ┌──────────┐
  │ Launcher │  reads .agent.yaml, resolves extensions
  └────┬─────┘
       │ cache hit? → exec binary
       │ cache miss → generate main.go + go.mod → build → cache → exec
       ▼
  ┌──────────────────────────────────────────┐
  │ Agent Binary                             │
  │                                          │
  │  init() chain fires                      │
  │    → sdk.RegisterRunner("turn-based")    │
  │    → sdk.RegisterProvider("llm-provider")│
  │    → sdk.RegisterProvider("openai")      │
  │    → sdk.RegisterStore("jsonl")          │
  │    → sdk.RegisterTool("tools")            │
  │      (returns []Tool{Name: "bash", ...})  │
  │    → sdk.RegisterExtension("lint")       │
  │                                          │
  │  main() → cfg.Load() → bus.New()         │
  │         → sdk.Wire(config, bus)          │
  │           → slots: runner, store         │
  │           → providers: all instantiated  │
  │           → tools: last-wins by config   │
  │           → hooks: all subscribed to bus │
  │                                          │
  │  ┌──── Turn Runner Loop ────┐            │
  │  │ turn++                    │            │
  │  │ → provider.Chat()         │──→ events │
  │  │ → tool calls?             │            │
  │  │   → tool.Execute()        │──→ events │
  │  │   → store.Append()        │            │
  │  │ → no tools? break         │            │
  │  └───────────────────────────┘            │
  │                                          │
  │  Events flow through Bus                 │
  │    → extensions react on their channels  │
  │    → store appends entries               │
  └──────────────────────────────────────────┘
```

---

## Build Performance

| Change | What recompiles | Time |
|--------|----------------|------|
| Edit 1 extension source | 1 Go package + link | 1-3s |
| Change active extensions (-e flag) | New hash → full build | 5-10s |
| Same extension set, already built | Nothing — cache hit | 0s (exec) |
| SDK interface change | All packages importing SDK | 3-8s |
| Go version upgrade | All hashes invalidated → full rebuild | 10-20s |

---

## Replaceable Slots

| Slot/Type | Built-in | Could be replaced with |
|------|----------|----------------------|
| `runner` | `turnrunner` | Parallel runner, multi-agent runner, REPL runner |
| `providers` | `llmprovider` (Anthropic) | `openai-provider`, local vLLM, any LLM API |
| `store` | `jsonlstore` | SQLite store, PostgreSQL store, in-memory |
| `tools` | `tools` (bash,read,write,edit,grep,glob) | `sandbox` overrides bash via last-wins order |
| hooks | `lint` | Any extension subscribing to events |

---

## Dependencies

| Package | Purpose | External? |
|---------|---------|:---------:|
| `std/*` | HTTP, JSON, sync, channels, crypto, os, filepath | No |
| `github.com/nniel-ape/gonfig` | Multi-source config loading | Yes |

Single external dependency. Everything else is stdlib.
