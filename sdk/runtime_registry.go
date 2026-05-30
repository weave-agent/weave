package sdk

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/weave-agent/weave/sdk/model"
)

var (
	ErrRuntimeDuplicateName = errors.New("runtime duplicate name")
	ErrRuntimeNotFound      = errors.New("runtime item not found")
)

type ToolRequest struct {
	Name      string
	Call      ToolCall
	Arguments map[string]any
}

type RuntimeTool struct {
	Name       string
	Definition ToolDef
	Run        func(context.Context, ToolRequest) (ToolResult, error)
	Disabled   bool
}

type RuntimeToolInfo struct {
	Name       string
	Definition ToolDef
	Enabled    bool
}

type ToolDecorator func(Tool) Tool

type RuntimeProvider struct {
	Name    string
	Factory func(Config) (Provider, error)
}

type RuntimeProviderInfo struct {
	Name string
}

type ProviderMiddleware interface {
	BeforeProviderRequest(context.Context, ProviderRequest) (ProviderRequest, error)
	AfterProviderResponse(context.Context, ProviderEvent) error
	ObserveProviderStream(context.Context, ProviderEvent) error
}

type ProviderMiddlewareFuncs struct {
	BeforeProviderRequestFunc func(context.Context, ProviderRequest) (ProviderRequest, error)
	AfterProviderResponseFunc func(context.Context, ProviderEvent) error
	ObserveProviderStreamFunc func(context.Context, ProviderEvent) error
}

func (m ProviderMiddlewareFuncs) BeforeProviderRequest(ctx context.Context, req ProviderRequest) (ProviderRequest, error) {
	if m.BeforeProviderRequestFunc == nil {
		return req, nil
	}

	return m.BeforeProviderRequestFunc(ctx, req)
}

func (m ProviderMiddlewareFuncs) AfterProviderResponse(ctx context.Context, event ProviderEvent) error {
	if m.AfterProviderResponseFunc == nil {
		return nil
	}

	return m.AfterProviderResponseFunc(ctx, event)
}

func (m ProviderMiddlewareFuncs) ObserveProviderStream(ctx context.Context, event ProviderEvent) error {
	if m.ObserveProviderStreamFunc == nil {
		return nil
	}

	return m.ObserveProviderStreamFunc(ctx, event)
}

type RuntimeToolRegistry struct {
	mu         sync.RWMutex
	config     Config
	runtime    map[string]RuntimeTool
	disabled   map[string]bool
	decorators map[int]toolDecoratorEntry
	nextID     int
}

type toolDecoratorEntry struct {
	id        int
	seq       int
	owner     string
	decorator ToolDecorator
}

func NewRuntimeToolRegistry(cfg Config) *RuntimeToolRegistry {
	return &RuntimeToolRegistry{
		config:     ConfigOrDefault(cfg),
		runtime:    make(map[string]RuntimeTool),
		disabled:   make(map[string]bool),
		decorators: make(map[int]toolDecoratorEntry),
	}
}

func (r *RuntimeToolRegistry) Register(tool RuntimeTool) (HookHandle, error) {
	if tool.Name == "" {
		return noopHookHandle{}, errors.New("register runtime tool: empty name")
	}

	if tool.Run == nil {
		return noopHookHandle{}, fmt.Errorf("register runtime tool %q: nil run function", tool.Name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.runtime[tool.Name]; ok || ToolRegistered(tool.Name) {
		return noopHookHandle{}, fmt.Errorf("runtime tool %q: %w", tool.Name, ErrRuntimeDuplicateName)
	}

	r.runtime[tool.Name] = tool
	if tool.Disabled {
		r.disabled[tool.Name] = true
	}

	return newCloseHandle(func() error { return r.Unregister(tool.Name) }), nil
}

func (r *RuntimeToolRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.runtime[name]; !ok {
		return fmt.Errorf("runtime tool %q: %w", name, ErrRuntimeNotFound)
	}

	delete(r.runtime, name)
	delete(r.disabled, name)

	return nil
}

func (r *RuntimeToolRegistry) List() []RuntimeToolInfo {
	infos := make([]RuntimeToolInfo, 0)

	for _, name := range ListTools() {
		r.mu.RLock()
		disabled := r.disabled[name]
		r.mu.RUnlock()

		if disabled {
			continue
		}

		tool, ok := r.Get(name)
		if !ok {
			continue
		}

		infos = append(infos, RuntimeToolInfo{Name: name, Definition: tool.Definition, Enabled: true})
	}

	r.mu.RLock()

	for name, tool := range r.runtime {
		if !toolAllowed(name) {
			continue
		}

		infos = append(infos, RuntimeToolInfo{Name: name, Definition: tool.Definition, Enabled: !r.disabled[name]})
	}

	r.mu.RUnlock()

	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })

	return infos
}

func (r *RuntimeToolRegistry) Get(name string) (RuntimeTool, bool) {
	if !toolAllowed(name) {
		return RuntimeTool{}, false
	}

	r.mu.RLock()
	tool, ok := r.runtime[name]
	disabled := r.disabled[name]
	r.mu.RUnlock()

	if disabled {
		return RuntimeTool{}, false
	}

	if ok {
		return tool, true
	}

	legacy, err := GetTool(name, r.config)
	if err != nil {
		return RuntimeTool{}, false
	}

	legacy = r.decorate(legacy)

	return RuntimeTool{
		Name:       name,
		Definition: legacy.Definition(),
		Run: func(ctx context.Context, req ToolRequest) (ToolResult, error) {
			args := req.Arguments
			if args == nil {
				args = req.Call.Arguments
			}

			return legacy.Execute(ctx, args)
		},
	}, true
}

func (r *RuntimeToolRegistry) Enable(name string) error {
	if !ToolRegistered(name) {
		r.mu.RLock()
		_, ok := r.runtime[name]
		r.mu.RUnlock()

		if !ok {
			return fmt.Errorf("runtime tool %q: %w", name, ErrRuntimeNotFound)
		}
	}

	r.mu.Lock()
	delete(r.disabled, name)
	r.mu.Unlock()

	return nil
}

func (r *RuntimeToolRegistry) Disable(name string) error {
	if !ToolRegistered(name) {
		r.mu.RLock()
		_, ok := r.runtime[name]
		r.mu.RUnlock()

		if !ok {
			return fmt.Errorf("runtime tool %q: %w", name, ErrRuntimeNotFound)
		}
	}

	r.mu.Lock()
	r.disabled[name] = true
	r.mu.Unlock()

	return nil
}

func (r *RuntimeToolRegistry) Decorate(owner string, decorator ToolDecorator) HookHandle {
	if decorator == nil {
		return noopHookHandle{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	id := r.nextID
	r.decorators[id] = toolDecoratorEntry{id: id, seq: id, owner: owner, decorator: decorator}

	return newCloseHandle(r.closeDecorator(id))
}

func (r *RuntimeToolRegistry) closeDecorator(id int) func() error {
	return func() error {
		r.mu.Lock()
		defer r.mu.Unlock()

		delete(r.decorators, id)

		return nil
	}
}

func (r *RuntimeToolRegistry) decorate(tool Tool) Tool {
	for _, entry := range r.toolDecoratorSnapshot() {
		tool = entry.decorator(tool)
	}

	return tool
}

func (r *RuntimeToolRegistry) toolDecoratorSnapshot() []toolDecoratorEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := make([]toolDecoratorEntry, 0, len(r.decorators))
	for _, entry := range r.decorators {
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].seq < entries[j].seq })

	return entries
}

type RuntimeProviderRegistry struct {
	mu          sync.RWMutex
	config      Config
	runtime     map[string]RuntimeProvider
	middlewares map[int]providerMiddlewareEntry
	nextID      int
}

type providerMiddlewareEntry struct {
	id         int
	seq        int
	owner      string
	middleware ProviderMiddleware
}

func NewRuntimeProviderRegistry(cfg Config) *RuntimeProviderRegistry {
	return &RuntimeProviderRegistry{
		config:      ConfigOrDefault(cfg),
		runtime:     make(map[string]RuntimeProvider),
		middlewares: make(map[int]providerMiddlewareEntry),
	}
}

func (r *RuntimeProviderRegistry) Register(provider RuntimeProvider) (HookHandle, error) {
	if provider.Name == "" {
		return noopHookHandle{}, errors.New("register runtime provider: empty name")
	}

	if provider.Factory == nil {
		return noopHookHandle{}, fmt.Errorf("register runtime provider %q: nil factory", provider.Name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.runtime[provider.Name]; ok || ProviderRegistered(provider.Name) {
		return noopHookHandle{}, fmt.Errorf("runtime provider %q: %w", provider.Name, ErrRuntimeDuplicateName)
	}

	r.runtime[provider.Name] = provider

	return newCloseHandle(func() error { return r.Unregister(provider.Name) }), nil
}

func (r *RuntimeProviderRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.runtime[name]; !ok {
		return fmt.Errorf("runtime provider %q: %w", name, ErrRuntimeNotFound)
	}

	delete(r.runtime, name)

	return nil
}

func (r *RuntimeProviderRegistry) List() []RuntimeProviderInfo {
	names := ListProviders()

	r.mu.RLock()

	for name := range r.runtime {
		names = append(names, name)
	}

	r.mu.RUnlock()

	sort.Strings(names)

	infos := make([]RuntimeProviderInfo, 0, len(names))
	for _, name := range names {
		infos = append(infos, RuntimeProviderInfo{Name: name})
	}

	return infos
}

func (r *RuntimeProviderRegistry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	runtime, ok := r.runtime[name]
	r.mu.RUnlock()

	var (
		provider Provider
		err      error
	)
	if ok {
		provider, err = runtime.Factory(r.config)
	} else {
		provider, err = GetProvider(name, r.config)
	}

	if err != nil {
		return nil, false
	}

	return r.wrapProvider(provider), true
}

func (r *RuntimeProviderRegistry) UseMiddleware(owner string, middleware ProviderMiddleware) HookHandle {
	if middleware == nil {
		return noopHookHandle{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	id := r.nextID
	r.middlewares[id] = providerMiddlewareEntry{id: id, seq: id, owner: owner, middleware: middleware}

	return newCloseHandle(r.closeMiddleware(id))
}

func (r *RuntimeProviderRegistry) closeMiddleware(id int) func() error {
	return func() error {
		r.mu.Lock()
		defer r.mu.Unlock()

		delete(r.middlewares, id)

		return nil
	}
}

func (r *RuntimeProviderRegistry) wrapProvider(provider Provider) Provider {
	middlewares := r.providerMiddlewareSnapshot()
	if len(middlewares) == 0 {
		return provider
	}

	wrapped := runtimeProviderWithMiddleware{provider: provider, middlewares: middlewares}
	if counter, ok := provider.(TokenCounter); ok {
		return runtimeProviderWithMiddlewareAndTokenCounter{
			runtimeProviderWithMiddleware: wrapped,
			counter:                       counter,
		}
	}

	return wrapped
}

func (r *RuntimeProviderRegistry) providerMiddlewareSnapshot() []providerMiddlewareEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := make([]providerMiddlewareEntry, 0, len(r.middlewares))
	for _, entry := range r.middlewares {
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].seq < entries[j].seq })

	return entries
}

type runtimeProviderWithMiddleware struct {
	provider    Provider
	middlewares []providerMiddlewareEntry
}

func (p runtimeProviderWithMiddleware) Stream(ctx context.Context, req ProviderRequest, opts ...model.StreamOption) (<-chan ProviderEvent, error) {
	req, err := p.beforeProviderRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	streamCtx, cancel := context.WithCancel(ctx)

	events, err := p.provider.Stream(streamCtx, req, opts...)
	if err != nil {
		cancel()

		return nil, fmt.Errorf("provider stream: %w", err)
	}

	out := make(chan ProviderEvent, 1)
	go func() {
		defer close(out)
		defer cancel()

		for {
			var event ProviderEvent

			select {
			case <-streamCtx.Done():
				return
			case e, ok := <-events:
				if !ok {
					return
				}

				event = e
			}

			for _, entry := range p.middlewares {
				if err := entry.middleware.ObserveProviderStream(streamCtx, event); err != nil {
					_ = sendProviderEvent(ctx, out, ProviderEvent{Type: ProviderEventError, Content: err})

					return
				}
			}

			for _, entry := range p.middlewares {
				if err := entry.middleware.AfterProviderResponse(streamCtx, event); err != nil {
					_ = sendProviderEvent(ctx, out, ProviderEvent{Type: ProviderEventError, Content: err})

					return
				}
			}

			if !sendProviderEvent(ctx, out, event) {
				return
			}
		}
	}()

	return out, nil
}

func (p runtimeProviderWithMiddleware) beforeProviderRequest(ctx context.Context, req ProviderRequest) (ProviderRequest, error) {
	var err error
	for _, entry := range p.middlewares {
		req, err = entry.middleware.BeforeProviderRequest(ctx, req)
		if err != nil {
			return ProviderRequest{}, fmt.Errorf("provider middleware %q before request: %w", entry.owner, err)
		}
	}

	return req, nil
}

func sendProviderEvent(ctx context.Context, out chan<- ProviderEvent, event ProviderEvent) bool {
	select {
	case out <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

type runtimeProviderWithMiddlewareAndTokenCounter struct {
	runtimeProviderWithMiddleware
	counter TokenCounter
}

func (p runtimeProviderWithMiddlewareAndTokenCounter) CountTokens(ctx context.Context, req ProviderRequest, opts ...model.StreamOption) (TokenCount, error) {
	req, err := p.beforeProviderRequest(ctx, req)
	if err != nil {
		return TokenCount{}, err
	}

	count, err := p.counter.CountTokens(ctx, req, opts...)
	if err != nil {
		return TokenCount{}, fmt.Errorf("provider count tokens: %w", err)
	}

	return count, nil
}

type closeHandle struct {
	once sync.Once
	fn   func() error
	err  error
}

func newCloseHandle(fn func() error) HookHandle {
	if fn == nil {
		return noopHookHandle{}
	}

	return &closeHandle{fn: fn}
}

func (h *closeHandle) Close() error {
	h.once.Do(func() {
		h.err = h.fn()
	})

	return h.err
}
