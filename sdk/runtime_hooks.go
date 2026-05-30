package sdk

import (
	"context"
	"errors"
	"sort"
	"sync"
)

// HookFunc mutates hook state or stops hook execution.
type HookFunc[TReq any, TRes any] func(context.Context, *HookState[TReq, TRes]) error

// Hook exposes an ordered typed interceptor chain.
type Hook[TReq any, TRes any] interface {
	Use(owner string, fn HookFunc[TReq, TRes], opts ...HookOption) HookHandle
	Run(ctx context.Context, req TReq) (TRes, error)
	RunState(ctx context.Context, req TReq) (HookState[TReq, TRes], error)
}

// HookHandle removes a registered hook handler and runs its cleanup callbacks.
type HookHandle interface {
	Close() error
}

// HookState is passed through an ordered hook chain.
type HookState[TReq any, TRes any] struct {
	Request TReq
	Result  TRes

	stopped bool
}

// Stop prevents later handlers from running. The current handler still
// completes normally unless it returns an error.
func (s *HookState[TReq, TRes]) Stop() {
	s.stopped = true
}

// Stopped reports whether a handler stopped the hook chain.
func (s HookState[TReq, TRes]) Stopped() bool {
	return s.stopped
}

// HookOption configures a hook handler registration.
type HookOption func(*hookOptions)

type hookOptions struct {
	order   int
	cleanup []func() error
}

// WithHookOrder sets handler ordering. Lower values run first.
func WithHookOrder(order int) HookOption {
	return func(opts *hookOptions) {
		opts.order = order
	}
}

// WithHookCleanup registers cleanup to run when the handle closes.
func WithHookCleanup(fn func() error) HookOption {
	return func(opts *hookOptions) {
		if fn != nil {
			opts.cleanup = append(opts.cleanup, fn)
		}
	}
}

// RuntimeHook is the default in-memory Hook implementation.
type RuntimeHook[TReq any, TRes any] struct {
	mu       sync.RWMutex
	nextSeq  int
	entries  map[int]hookEntry[TReq, TRes]
	initFunc func(TReq) TRes
}

type hookEntry[TReq any, TRes any] struct {
	id      int
	seq     int
	order   int
	owner   string
	fn      HookFunc[TReq, TRes]
	cleanup []func() error
}

// NewHook returns a typed hook that runs handlers in order.
func NewHook[TReq any, TRes any](opts ...RuntimeHookOption[TReq, TRes]) *RuntimeHook[TReq, TRes] {
	hook := &RuntimeHook[TReq, TRes]{
		entries: make(map[int]hookEntry[TReq, TRes]),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(hook)
		}
	}

	return hook
}

// RuntimeHookOption configures a RuntimeHook.
type RuntimeHookOption[TReq any, TRes any] func(*RuntimeHook[TReq, TRes])

// WithHookInitialResult derives the initial result value from the request.
func WithHookInitialResult[TReq any, TRes any](fn func(TReq) TRes) RuntimeHookOption[TReq, TRes] {
	return func(h *RuntimeHook[TReq, TRes]) {
		h.initFunc = fn
	}
}

// Use registers a handler and returns a handle that can unregister it.
func (h *RuntimeHook[TReq, TRes]) Use(owner string, fn HookFunc[TReq, TRes], opts ...HookOption) HookHandle {
	if fn == nil {
		return noopHookHandle{}
	}

	cfg := hookOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.nextSeq++
	id := h.nextSeq
	h.entries[id] = hookEntry[TReq, TRes]{
		id:      id,
		seq:     h.nextSeq,
		order:   cfg.order,
		owner:   owner,
		fn:      fn,
		cleanup: append([]func() error(nil), cfg.cleanup...),
	}

	return &runtimeHookHandle[TReq, TRes]{hook: h, id: id}
}

// Run executes the hook and returns the final result.
func (h *RuntimeHook[TReq, TRes]) Run(ctx context.Context, req TReq) (TRes, error) {
	state, err := h.RunState(ctx, req)
	return state.Result, err
}

// RunState executes the hook and returns the final request and result state.
func (h *RuntimeHook[TReq, TRes]) RunState(ctx context.Context, req TReq) (HookState[TReq, TRes], error) {
	state := HookState[TReq, TRes]{Request: req}
	if h.initFunc != nil {
		state.Result = h.initFunc(req)
	}

	for _, entry := range h.snapshot() {
		if err := entry.fn(ctx, &state); err != nil {
			return state, err
		}
		if state.stopped {
			return state, nil
		}
	}

	return state, nil
}

func (h *RuntimeHook[TReq, TRes]) snapshot() []hookEntry[TReq, TRes] {
	h.mu.RLock()
	defer h.mu.RUnlock()

	entries := make([]hookEntry[TReq, TRes], 0, len(h.entries))
	for _, entry := range h.entries {
		entries = append(entries, entry)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].order == entries[j].order {
			return entries[i].seq < entries[j].seq
		}

		return entries[i].order < entries[j].order
	})

	return entries
}

func (h *RuntimeHook[TReq, TRes]) remove(id int) (hookEntry[TReq, TRes], bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	entry, ok := h.entries[id]
	if !ok {
		return hookEntry[TReq, TRes]{}, false
	}
	delete(h.entries, id)

	return entry, true
}

type runtimeHookHandle[TReq any, TRes any] struct {
	once sync.Once
	hook *RuntimeHook[TReq, TRes]
	id   int
	err  error
}

func (h *runtimeHookHandle[TReq, TRes]) Close() error {
	h.once.Do(func() {
		entry, ok := h.hook.remove(h.id)
		if !ok {
			return
		}
		for _, cleanup := range entry.cleanup {
			if cleanup != nil {
				h.err = errors.Join(h.err, cleanup())
			}
		}
	})

	return h.err
}

type noopHookHandle struct{}

func (noopHookHandle) Close() error { return nil }

// NewBusObserverHook publishes hook state to an existing bus topic.
func NewBusObserverHook[TReq any, TRes any](bus Bus, topic string, payload func(HookState[TReq, TRes]) any) HookFunc[TReq, TRes] {
	return func(_ context.Context, state *HookState[TReq, TRes]) error {
		if bus == nil || topic == "" {
			return nil
		}
		body := any(*state)
		if payload != nil {
			body = payload(*state)
		}
		bus.Publish(NewEvent(topic, body))

		return nil
	}
}

// Hooks groups the standard runtime hook families.
type Hooks interface {
	Input() Hook[InputHookRequest, InputHookResult]
	Prompt() Hook[PromptHookRequest, PromptHookResult]
	Context() Hook[ContextHookRequest, ContextHookResult]
	ProviderRequest() Hook[ProviderRequestHookRequest, ProviderRequestHookResult]
	ProviderResponse() Hook[ProviderResponseHookRequest, ProviderResponseHookResult]
	ToolCall() Hook[ToolCallRequest, ToolCallResult]
	ToolResult() Hook[ToolResultRequest, ToolResultHookResult]
	Message() Hook[MessageHookRequest, MessageHookResult]
	Turn() Hook[TurnHookRequest, TurnHookResult]
	Session() Hook[SessionHookRequest, SessionHookResult]
}

// RuntimeHooks is the default Hooks implementation.
type RuntimeHooks struct {
	input            *RuntimeHook[InputHookRequest, InputHookResult]
	prompt           *RuntimeHook[PromptHookRequest, PromptHookResult]
	context          *RuntimeHook[ContextHookRequest, ContextHookResult]
	providerRequest  *RuntimeHook[ProviderRequestHookRequest, ProviderRequestHookResult]
	providerResponse *RuntimeHook[ProviderResponseHookRequest, ProviderResponseHookResult]
	toolCall         *RuntimeHook[ToolCallRequest, ToolCallResult]
	toolResult       *RuntimeHook[ToolResultRequest, ToolResultHookResult]
	message          *RuntimeHook[MessageHookRequest, MessageHookResult]
	turn             *RuntimeHook[TurnHookRequest, TurnHookResult]
	session          *RuntimeHook[SessionHookRequest, SessionHookResult]
}

// NewRuntimeHooks returns standard runtime hook families.
func NewRuntimeHooks() *RuntimeHooks {
	return &RuntimeHooks{
		input:   NewHook[InputHookRequest, InputHookResult](),
		prompt:  NewHook[PromptHookRequest, PromptHookResult](),
		context: NewHook[ContextHookRequest, ContextHookResult](),
		providerRequest: NewHook[ProviderRequestHookRequest, ProviderRequestHookResult](WithHookInitialResult(func(req ProviderRequestHookRequest) ProviderRequestHookResult {
			return ProviderRequestHookResult{Request: req.Request}
		})),
		providerResponse: NewHook[ProviderResponseHookRequest, ProviderResponseHookResult](),
		toolCall:         NewHook[ToolCallRequest, ToolCallResult](WithHookInitialResult(func(req ToolCallRequest) ToolCallResult { return ToolCallResult{Call: req.Call, Continue: true} })),
		toolResult:       NewHook[ToolResultRequest, ToolResultHookResult](WithHookInitialResult(func(req ToolResultRequest) ToolResultHookResult { return ToolResultHookResult{Result: req.Result} })),
		message:          NewHook[MessageHookRequest, MessageHookResult](WithHookInitialResult(func(req MessageHookRequest) MessageHookResult { return MessageHookResult{Message: req.Message} })),
		turn:             NewHook[TurnHookRequest, TurnHookResult](),
		session:          NewHook[SessionHookRequest, SessionHookResult](),
	}
}

func (h *RuntimeHooks) Input() Hook[InputHookRequest, InputHookResult] { return h.input }
func (h *RuntimeHooks) Prompt() Hook[PromptHookRequest, PromptHookResult] {
	return h.prompt
}
func (h *RuntimeHooks) Context() Hook[ContextHookRequest, ContextHookResult] {
	return h.context
}
func (h *RuntimeHooks) ProviderRequest() Hook[ProviderRequestHookRequest, ProviderRequestHookResult] {
	return h.providerRequest
}
func (h *RuntimeHooks) ProviderResponse() Hook[ProviderResponseHookRequest, ProviderResponseHookResult] {
	return h.providerResponse
}
func (h *RuntimeHooks) ToolCall() Hook[ToolCallRequest, ToolCallResult] { return h.toolCall }
func (h *RuntimeHooks) ToolResult() Hook[ToolResultRequest, ToolResultHookResult] {
	return h.toolResult
}
func (h *RuntimeHooks) Message() Hook[MessageHookRequest, MessageHookResult] {
	return h.message
}
func (h *RuntimeHooks) Turn() Hook[TurnHookRequest, TurnHookResult] { return h.turn }
func (h *RuntimeHooks) Session() Hook[SessionHookRequest, SessionHookResult] {
	return h.session
}

type InputHookRequest struct {
	Content any
}

type InputHookResult struct {
	Content any
}

type PromptHookRequest struct {
	SystemPrompt string
	Messages     []Message
}

type PromptHookResult struct {
	SystemPrompt string
	Messages     []Message
}

type ContextHookRequest struct {
	Messages []Message
}

type ContextHookResult struct {
	Messages []Message
}

type ProviderRequestHookRequest struct {
	Provider string
	Request  ProviderRequest
}

type ProviderRequestHookResult struct {
	Request ProviderRequest
}

type ProviderResponseHookRequest struct {
	Provider string
	Event    ProviderEvent
}

type ProviderResponseHookResult struct {
	Event ProviderEvent
}

type ToolCallRequest struct {
	Call ToolCall
}

type ToolCallResult struct {
	Call     ToolCall
	Continue bool
}

type ToolResultRequest struct {
	Call   ToolCall
	Result ToolResult
}

type ToolResultHookResult struct {
	Result ToolResult
}

type MessageHookRequest struct {
	Message Message
}

type MessageHookResult struct {
	Message Message
}

type TurnHookRequest struct {
	Messages []Message
}

type TurnHookResult struct {
	Messages []Message
}

type SessionHookRequest struct {
	Event string
	Entry any
}

type SessionHookResult struct {
	Entry any
}
