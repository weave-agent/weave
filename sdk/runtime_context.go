package sdk

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/weave-agent/weave/sdk/model"
)

var ErrRuntimeCapabilityUnsupported = errors.New("runtime capability unsupported")

// RuntimeExtension is implemented by extensions that register against typed
// runtime services instead of subscribing directly to bus events.
type RuntimeExtension interface {
	Register(ExtensionContext) error
}

// ExtensionContext exposes root runtime services to extensions without pulling
// UI-specific framework types into the SDK surface.
type ExtensionContext interface {
	Bus() Bus
	Hooks() Hooks
	Tools() ToolRegistry
	Providers() ProviderRegistry
	Session() SessionController
	Resources() ResourceRegistry
	Models() ModelController
	Exec(context.Context, ExecRequest) (ExecResult, error)
	Config(scope, name string, target any) error
}

// ToolRegistry exposes session-scoped runtime tools while adapting legacy
// static tool registrations into the same lookup path.
type ToolRegistry interface {
	Register(RuntimeTool) (HookHandle, error)
	Unregister(name string) error
	List() []RuntimeToolInfo
	Get(name string) (RuntimeTool, bool)
	Enable(name string) error
	Disable(name string) error
	Decorate(owner string, decorator ToolDecorator) HookHandle
}

// ProviderRegistry exposes runtime providers and provider middleware while
// adapting legacy static provider registrations into the same lookup path.
type ProviderRegistry interface {
	Register(RuntimeProvider) (HookHandle, error)
	Unregister(name string) error
	List() []RuntimeProviderInfo
	Get(name string) (Provider, bool)
	UseMiddleware(owner string, middleware ProviderMiddleware) HookHandle
}

type SessionController interface {
	SendUserMessage(context.Context, any) error
	AppendEntry(context.Context, SessionEntry) (string, error)
	SetName(context.Context, string) error
	Name(context.Context) (string, error)
	SetLabel(context.Context, string, string) error
	Compact(context.Context, CompactRequest) (CompactResult, error)
	Fork(context.Context, ForkSessionRequest) (SessionRef, error)
	Switch(context.Context, SwitchSessionRequest) (SessionRef, error)
	Tree(context.Context) (SessionTree, error)
}

type SessionEntry struct {
	ID        string
	ParentID  string
	Message   Message
	Label     string
	Metadata  map[string]any
	CreatedAt string
}

type CompactRequest struct {
	Reason       string
	TargetTokens int
	Metadata     map[string]any
}

type CompactResult struct {
	EntryID  string
	Summary  string
	Metadata map[string]any
}

type ForkSessionRequest struct {
	FromSessionID string
	FromEntryID   string
	Name          string
	Metadata      map[string]any
}

type SwitchSessionRequest struct {
	SessionID string
}

type SessionRef struct {
	ID       string
	Name     string
	Metadata map[string]any
}

type SessionTree struct {
	CurrentSessionID string
	Sessions         []SessionTreeNode
}

type SessionTreeNode struct {
	Session  SessionRef
	ParentID string
	EntryID  string
}

type ResourceKind string

const (
	ResourceKindPrompt        ResourceKind = "prompt"
	ResourceKindSkill         ResourceKind = "skill"
	ResourceKindContext       ResourceKind = "context"
	ResourceKindTheme         ResourceKind = "theme"
	ResourceKindEmbeddedAsset ResourceKind = "embedded_asset"
)

type ResourceRegistry interface {
	Register(ResourceProvider) HookHandle
	List(context.Context, ResourceQuery) ResourceList
	Get(context.Context, ResourceQuery) (Resource, error)
}

type ResourceProvider interface {
	Name() string
	ListResources(context.Context, ResourceQuery) ([]ResourceInfo, error)
	GetResource(context.Context, ResourceQuery) (Resource, error)
}

type ResourceQuery struct {
	Kind     ResourceKind
	URI      string
	Metadata map[string]any
}

type ResourceInfo struct {
	Kind        ResourceKind
	URI         string
	Name        string
	Description string
	Metadata    map[string]any
}

type Resource struct {
	ResourceInfo
	Content []byte
}

type ResourceList struct {
	Resources []ResourceInfo
	Errors    []ResourceProviderError
}

type ResourceProviderError struct {
	Provider string
	Err      error
}

func (e ResourceProviderError) Error() string {
	if e.Err == nil {
		return e.Provider
	}

	if e.Provider == "" {
		return e.Err.Error()
	}

	return e.Provider + ": " + e.Err.Error()
}

func (e ResourceProviderError) Unwrap() error { return e.Err }

type ModelController interface {
	ListModels() []model.ModelDef
	ListAvailableModels() []model.ModelDef
	GetModel(id string) (model.ModelDef, bool)
	GetModelForProvider(id, provider string) (model.ModelDef, bool)
	DefaultModelForProvider(provider string) (model.ModelDef, bool)
	CurrentModel(context.Context) (string, error)
	SetModel(context.Context, string) error
	ThinkingLevel(context.Context) (model.ThinkingLevel, error)
	SetThinkingLevel(context.Context, model.ThinkingLevel) error
	ClampThinkingLevel(model.ThinkingLevel, model.ModelDef) model.ThinkingLevel
}

type ExecRequest struct {
	ID          string
	ToolCallID  string
	Command     string
	Args        []string
	Dir         string
	Env         []string
	Reason      string
	Action      GuardianAction
	Metadata    map[string]any
	Sandbox     ExecSandboxRequest
	Guardian    ExecGuardianRequest
	Interactive bool
}

type ExecGuardianRequest struct {
	Skip        bool
	Description string
	Metadata    map[string]any
}

type ExecSandboxRequest struct {
	Skip       bool
	Profile    string
	Filesystem []SandboxFilesystemExpansion
	Network    []SandboxNetworkExpansion
	Metadata   map[string]any
}

type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Metadata map[string]any
}

// RuntimeContextOptions configures a default ExtensionContext.
type RuntimeContextOptions struct {
	Bus       Bus
	Hooks     Hooks
	Tools     ToolRegistry
	Providers ProviderRegistry
	Session   SessionController
	Resources ResourceRegistry
	Models    ModelController
	Config    Config
	Prefs     PreferenceWriter
	Exec      func(context.Context, ExecRequest) (ExecResult, error)
}

type runtimeContext struct {
	bus       Bus
	hooks     Hooks
	tools     ToolRegistry
	providers ProviderRegistry
	session   SessionController
	resources ResourceRegistry
	models    ModelController
	config    Config
	exec      func(context.Context, ExecRequest) (ExecResult, error)
}

// NewExtensionContext returns a nil-safe runtime context for extension
// registration. Missing services use no-op placeholders and unsupported
// operations return typed errors.
func NewExtensionContext(opts RuntimeContextOptions) ExtensionContext {
	ctx := &runtimeContext{
		bus:       opts.Bus,
		hooks:     opts.Hooks,
		tools:     opts.Tools,
		providers: opts.Providers,
		session:   opts.Session,
		resources: opts.Resources,
		models:    opts.Models,
		config:    ConfigOrDefault(opts.Config),
		exec:      opts.Exec,
	}
	if ctx.bus == nil {
		ctx.bus = NoopBus{}
	}
	if ctx.hooks == nil {
		ctx.hooks = NewRuntimeHooksWithBus(ctx.bus)
	}
	if ctx.tools == nil {
		ctx.tools = NewRuntimeToolRegistry(ctx.config)
	}
	if ctx.providers == nil {
		ctx.providers = NewRuntimeProviderRegistry(ctx.config)
	}
	if ctx.session == nil {
		ctx.session = NoopSessionController{}
	}
	if ctx.resources == nil {
		ctx.resources = NewRuntimeResourceRegistry()
	}
	if ctx.models == nil {
		prefs := opts.Prefs
		if prefs == nil {
			if writer, ok := ctx.config.(PreferenceWriter); ok {
				prefs = writer
			}
		}
		ctx.models = NewRuntimeModelController(RuntimeModelControllerOptions{
			Bus:   ctx.bus,
			Prefs: prefs,
		})
	}

	return ctx
}

func (c *runtimeContext) Bus() Bus                        { return c.bus }
func (c *runtimeContext) Hooks() Hooks                    { return c.hooks }
func (c *runtimeContext) Tools() ToolRegistry             { return c.tools }
func (c *runtimeContext) Providers() ProviderRegistry     { return c.providers }
func (c *runtimeContext) Session() SessionController      { return c.session }
func (c *runtimeContext) Resources() ResourceRegistry     { return c.resources }
func (c *runtimeContext) Models() ModelController         { return c.models }
func (c *runtimeContext) Config(s, n string, t any) error { return c.config.ExtensionConfig(s, n, t) }
func (c *runtimeContext) Exec(ctx context.Context, req ExecRequest) (ExecResult, error) {
	if c.exec == nil {
		return ExecResult{}, ErrRuntimeCapabilityUnsupported
	}

	return c.exec(ctx, req)
}

// NoopBus is a nil-safe Bus implementation.
type NoopBus struct{}

func (NoopBus) Publish(Event)      {}
func (NoopBus) On(string, Handler) {}
func (NoopBus) OnAll(Handler)      {}
func (NoopBus) Off(Handler)        {}
func (NoopBus) Close() error       { return nil }

type NoopSessionController struct{}
type NoopResourceRegistry struct{}

func (NoopSessionController) SendUserMessage(context.Context, any) error {
	return ErrRuntimeCapabilityUnsupported
}

func (NoopSessionController) AppendEntry(context.Context, SessionEntry) (string, error) {
	return "", ErrRuntimeCapabilityUnsupported
}

func (NoopSessionController) SetName(context.Context, string) error {
	return ErrRuntimeCapabilityUnsupported
}

func (NoopSessionController) Name(context.Context) (string, error) {
	return "", ErrRuntimeCapabilityUnsupported
}

func (NoopSessionController) SetLabel(context.Context, string, string) error {
	return ErrRuntimeCapabilityUnsupported
}

func (NoopSessionController) Compact(context.Context, CompactRequest) (CompactResult, error) {
	return CompactResult{}, ErrRuntimeCapabilityUnsupported
}

func (NoopSessionController) Fork(context.Context, ForkSessionRequest) (SessionRef, error) {
	return SessionRef{}, ErrRuntimeCapabilityUnsupported
}

func (NoopSessionController) Switch(context.Context, SwitchSessionRequest) (SessionRef, error) {
	return SessionRef{}, ErrRuntimeCapabilityUnsupported
}

func (NoopSessionController) Tree(context.Context) (SessionTree, error) {
	return SessionTree{}, ErrRuntimeCapabilityUnsupported
}

func (NoopResourceRegistry) Register(ResourceProvider) HookHandle { return noopHookHandle{} }

func (NoopResourceRegistry) List(context.Context, ResourceQuery) ResourceList {
	return ResourceList{Resources: []ResourceInfo{}}
}

func (NoopResourceRegistry) Get(context.Context, ResourceQuery) (Resource, error) {
	return Resource{}, ErrRuntimeNotFound
}

type RuntimeResourceRegistry struct {
	mu        sync.RWMutex
	providers []resourceProviderEntry
	nextID    int
}

type resourceProviderEntry struct {
	id       int
	provider ResourceProvider
}

func NewRuntimeResourceRegistry(providers ...ResourceProvider) *RuntimeResourceRegistry {
	registry := &RuntimeResourceRegistry{}
	for _, provider := range providers {
		registry.Register(provider)
	}

	return registry
}

func (r *RuntimeResourceRegistry) Register(provider ResourceProvider) HookHandle {
	if provider == nil {
		return noopHookHandle{}
	}

	r.mu.Lock()
	r.nextID++
	id := r.nextID
	r.providers = append(r.providers, resourceProviderEntry{id: id, provider: provider})
	r.mu.Unlock()

	return newCloseHandle(func() error {
		r.mu.Lock()
		defer r.mu.Unlock()

		for i, entry := range r.providers {
			if entry.id == id {
				r.providers = slices.Delete(r.providers, i, i+1)

				break
			}
		}

		return nil
	})
}

func (r *RuntimeResourceRegistry) List(ctx context.Context, query ResourceQuery) ResourceList {
	out := ResourceList{Resources: []ResourceInfo{}}
	for _, provider := range r.providerSnapshot() {
		resources, err := provider.ListResources(ctx, query)
		if err != nil {
			out.Errors = append(out.Errors, ResourceProviderError{Provider: provider.Name(), Err: err})

			continue
		}
		out.Resources = append(out.Resources, resources...)
	}

	return out
}

func (r *RuntimeResourceRegistry) Get(ctx context.Context, query ResourceQuery) (Resource, error) {
	var errs []error
	for _, provider := range r.providerSnapshot() {
		resource, err := provider.GetResource(ctx, query)
		if err == nil {
			return resource, nil
		}
		errs = append(errs, ResourceProviderError{Provider: provider.Name(), Err: err})
	}
	if len(errs) > 0 {
		return Resource{}, errors.Join(errs...)
	}

	return Resource{}, ErrRuntimeNotFound
}

func (r *RuntimeResourceRegistry) providerSnapshot() []ResourceProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers := make([]ResourceProvider, 0, len(r.providers))
	for _, entry := range r.providers {
		providers = append(providers, entry.provider)
	}

	return providers
}

type RuntimeModelControllerOptions struct {
	Bus   Bus
	Prefs PreferenceWriter
}

type RuntimeModelController struct {
	bus   Bus
	prefs PreferenceWriter
}

func NewRuntimeModelController(opts RuntimeModelControllerOptions) *RuntimeModelController {
	bus := opts.Bus
	if bus == nil {
		bus = NoopBus{}
	}

	return &RuntimeModelController{bus: bus, prefs: opts.Prefs}
}

func (RuntimeModelController) ListModels() []model.ModelDef { return model.ListAllModels() }

func (RuntimeModelController) ListAvailableModels() []model.ModelDef {
	return model.ListAvailableModels()
}

func (RuntimeModelController) GetModel(id string) (model.ModelDef, bool) {
	return model.GetModel(id)
}

func (RuntimeModelController) GetModelForProvider(id, provider string) (model.ModelDef, bool) {
	return model.GetModelForProvider(id, provider)
}

func (RuntimeModelController) DefaultModelForProvider(provider string) (model.ModelDef, bool) {
	return model.DefaultModelForProvider(provider)
}

func (m *RuntimeModelController) CurrentModel(context.Context) (string, error) {
	var prefs struct {
		Model string `json:"model"`
	}
	if m.prefs == nil {
		return "", ErrRuntimeCapabilityUnsupported
	}
	if err := m.prefs.Preferences(&prefs); err != nil {
		return "", err
	}

	return prefs.Model, nil
}

func (m *RuntimeModelController) SetModel(_ context.Context, id string) error {
	if m.prefs == nil {
		return ErrRuntimeCapabilityUnsupported
	}

	var prefs struct {
		Model string `json:"model"`
	}
	if err := m.prefs.Preferences(&prefs); err != nil {
		return err
	}
	prefs.Model = id
	if err := m.prefs.SavePreferences(&prefs); err != nil {
		return err
	}
	m.bus.Publish(NewEvent("model.change", map[string]any{"model": id}))

	return nil
}

func (m *RuntimeModelController) ThinkingLevel(context.Context) (model.ThinkingLevel, error) {
	if m.prefs == nil {
		return "", ErrRuntimeCapabilityUnsupported
	}

	var prefs struct {
		ThinkingLevel string `json:"thinking_level"`
	}
	if err := m.prefs.Preferences(&prefs); err != nil {
		return "", err
	}
	if prefs.ThinkingLevel == "" {
		return model.ThinkingOff, nil
	}

	return model.ParseThinkingLevel(prefs.ThinkingLevel)
}

func (m *RuntimeModelController) SetThinkingLevel(_ context.Context, level model.ThinkingLevel) error {
	if m.prefs == nil {
		return ErrRuntimeCapabilityUnsupported
	}
	if _, err := model.ParseThinkingLevel(string(level)); err != nil {
		return err
	}

	var prefs struct {
		ThinkingLevel string `json:"thinking_level"`
	}
	if err := m.prefs.Preferences(&prefs); err != nil {
		return err
	}
	prefs.ThinkingLevel = string(level)
	if err := m.prefs.SavePreferences(&prefs); err != nil {
		return err
	}
	m.bus.Publish(NewEvent("model.change", map[string]any{"thinking": string(level)}))

	return nil
}

func (RuntimeModelController) ClampThinkingLevel(level model.ThinkingLevel, def model.ModelDef) model.ThinkingLevel {
	return model.ClampForModel(level, def)
}
