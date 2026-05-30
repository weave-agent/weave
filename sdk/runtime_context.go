package sdk

import (
	"context"
	"errors"
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

// ToolRegistry is intentionally shallow until runtime tool activation lands.
type ToolRegistry interface{}

// ProviderRegistry is intentionally shallow until runtime provider middleware lands.
type ProviderRegistry interface{}

// SessionController is intentionally shallow until session runtime services land.
type SessionController interface{}

// ResourceRegistry is intentionally shallow until resource runtime services land.
type ResourceRegistry interface{}

// ModelController is intentionally shallow until model runtime services land.
type ModelController interface{}

type ExecRequest struct {
	Command string
	Args    []string
	Dir     string
	Env     []string
	Reason  string
}

type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
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
		ctx.hooks = NewRuntimeHooks()
	}
	if ctx.tools == nil {
		ctx.tools = NoopToolRegistry{}
	}
	if ctx.providers == nil {
		ctx.providers = NoopProviderRegistry{}
	}
	if ctx.session == nil {
		ctx.session = NoopSessionController{}
	}
	if ctx.resources == nil {
		ctx.resources = NoopResourceRegistry{}
	}
	if ctx.models == nil {
		ctx.models = NoopModelController{}
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

type NoopToolRegistry struct{}
type NoopProviderRegistry struct{}
type NoopSessionController struct{}
type NoopResourceRegistry struct{}
type NoopModelController struct{}
