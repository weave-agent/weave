package sdk

import (
	"context"
	"errors"
	"sort"
	"sync"
)

const (
	// TopicAgentPrompt publishes user input submitted to the agent loop.
	//
	// Deprecated: use Hooks.Input for behavior-changing interception.
	TopicAgentPrompt = "agent.prompt"

	// TopicProviderRequest publishes provider requests after typed hook mutation.
	//
	// Deprecated: use Hooks.ProviderRequest for behavior-changing interception.
	TopicProviderRequest = "provider.request"

	// TopicProviderResponse publishes provider stream events after typed hook
	// observation.
	//
	// Deprecated: use Hooks.ProviderResponse for behavior-changing interception.
	TopicProviderResponse = "provider.response"

	// TopicMessage publishes messages after typed hook mutation.
	//
	// Deprecated: use Hooks.Message for behavior-changing interception.
	TopicMessage = "message"

	// TopicTurn publishes turn lifecycle state after typed hook mutation.
	//
	// Deprecated: use Hooks.Turn for behavior-changing interception.
	TopicTurn = "turn"

	// TopicSessionResume publishes session resume payloads.
	//
	// Deprecated: use Hooks.Session for behavior-changing interception.
	TopicSessionResume = "session.resume"
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
func NewHook[TReq, TRes any](opts ...RuntimeHookOption[TReq, TRes]) *RuntimeHook[TReq, TRes] {
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
func WithHookInitialResult[TReq, TRes any](fn func(TReq) TRes) RuntimeHookOption[TReq, TRes] {
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

type hookHandles []HookHandle

func (h hookHandles) Close() error {
	var err error

	for _, handle := range h {
		if handle != nil {
			err = errors.Join(err, handle.Close())
		}
	}

	return err
}

// NewBusObserverHook publishes hook state to an existing bus topic.
func NewBusObserverHook[TReq, TRes any](bus Bus, topic string, payload func(HookState[TReq, TRes]) any) HookFunc[TReq, TRes] {
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

// NewRuntimeHooksWithBus returns standard runtime hook families with bus
// observer compatibility handlers installed.
func NewRuntimeHooksWithBus(bus Bus) *RuntimeHooks {
	hooks := NewRuntimeHooks()
	hooks.AttachBusObservers(bus)

	return hooks
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
		input: NewHook[InputHookRequest, InputHookResult](WithHookInitialResult(func(req InputHookRequest) InputHookResult {
			return InputHookResult(req)
		})),
		prompt: NewHook[PromptHookRequest, PromptHookResult](WithHookInitialResult(func(req PromptHookRequest) PromptHookResult {
			return PromptHookResult(req)
		})),
		context: NewHook[ContextHookRequest, ContextHookResult](WithHookInitialResult(func(req ContextHookRequest) ContextHookResult {
			return ContextHookResult(req)
		})),
		providerRequest: NewHook[ProviderRequestHookRequest, ProviderRequestHookResult](WithHookInitialResult(func(req ProviderRequestHookRequest) ProviderRequestHookResult {
			return ProviderRequestHookResult{Request: req.Request}
		})),
		providerResponse: NewHook[ProviderResponseHookRequest, ProviderResponseHookResult](WithHookInitialResult(func(req ProviderResponseHookRequest) ProviderResponseHookResult {
			return ProviderResponseHookResult{Event: req.Event}
		})),
		toolCall:   NewHook[ToolCallRequest, ToolCallResult](WithHookInitialResult(func(req ToolCallRequest) ToolCallResult { return ToolCallResult{Call: req.Call, Continue: true} })),
		toolResult: NewHook[ToolResultRequest, ToolResultHookResult](WithHookInitialResult(func(req ToolResultRequest) ToolResultHookResult { return ToolResultHookResult{Result: req.Result} })),
		message:    NewHook[MessageHookRequest, MessageHookResult](WithHookInitialResult(func(req MessageHookRequest) MessageHookResult { return MessageHookResult(req) })),
		turn:       NewHook[TurnHookRequest, TurnHookResult](WithHookInitialResult(func(req TurnHookRequest) TurnHookResult { return TurnHookResult(req) })),
		session:    NewHook[SessionHookRequest, SessionHookResult](WithHookInitialResult(func(req SessionHookRequest) SessionHookResult { return SessionHookResult{Entry: req.Entry} })),
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

// AttachBusObservers maps typed hook execution back onto stable bus topics for
// existing observer extensions. The registered observers do not mutate hook
// state; they only publish the post-mutation payload currently visible at their
// position in the hook chain.
func (h *RuntimeHooks) AttachBusObservers(bus Bus) HookHandle {
	if h == nil || bus == nil {
		return noopHookHandle{}
	}

	handles := hookHandles{
		h.Input().Use("sdk.bus_compat", NewBusObserverHook(bus, TopicAgentPrompt, func(state HookState[InputHookRequest, InputHookResult]) any {
			if state.Result.Content != nil {
				return state.Result.Content
			}

			return state.Request.Content
		}), WithHookOrder(10_000)),
		h.ProviderRequest().Use("sdk.bus_compat", NewBusObserverHook(bus, TopicProviderRequest, func(state HookState[ProviderRequestHookRequest, ProviderRequestHookResult]) any {
			return ProviderRequestBusPayload{Provider: state.Request.Provider, Request: state.Result.Request}
		}), WithHookOrder(10_000)),
		h.ProviderResponse().Use("sdk.bus_compat", NewBusObserverHook(bus, TopicProviderResponse, func(state HookState[ProviderResponseHookRequest, ProviderResponseHookResult]) any {
			event := state.Result.Event
			if event.Type == "" {
				event = state.Request.Event
			}

			return ProviderResponseBusPayload{Provider: state.Request.Provider, Event: event}
		}), WithHookOrder(10_000)),
		h.ToolCall().Use("sdk.bus_compat", NewBusObserverHook(bus, TopicToolStart, func(state HookState[ToolCallRequest, ToolCallResult]) any {
			call := state.Result.Call
			if call.Name == "" && call.ID == "" {
				call = state.Request.Call
			}

			return ToolProgress{ToolCallID: call.ID, ToolName: call.Name}
		}), WithHookOrder(10_000)),
		h.ToolCall().Use("sdk.bus_compat", NewBusObserverHook(bus, ProviderEventToolCall, func(state HookState[ToolCallRequest, ToolCallResult]) any {
			call := state.Result.Call
			if call.Name == "" && call.ID == "" {
				call = state.Request.Call
			}

			return call
		}), WithHookOrder(10_001)),
		h.ToolResult().Use("sdk.bus_compat", func(ctx context.Context, state *HookState[ToolResultRequest, ToolResultHookResult]) error {
			result := state.Result.Result

			topic := TopicToolComplete
			if result.IsError {
				topic = TopicToolError
			}

			return NewBusObserverHook(bus, topic, func(HookState[ToolResultRequest, ToolResultHookResult]) any {
				return ToolProgress{
					ToolCallID: state.Request.Call.ID,
					ToolName:   state.Request.Call.Name,
					Content:    result.Content,
					IsError:    result.IsError,
				}
			})(ctx, state)
		}, WithHookOrder(10_000)),
		h.Message().Use("sdk.bus_compat", NewBusObserverHook(bus, TopicMessage, func(state HookState[MessageHookRequest, MessageHookResult]) any {
			if state.Result.Message.Role != "" {
				return state.Result.Message
			}

			return state.Request.Message
		}), WithHookOrder(10_000)),
		h.Turn().Use("sdk.bus_compat", NewBusObserverHook(bus, TopicTurn, func(state HookState[TurnHookRequest, TurnHookResult]) any {
			if state.Result.Messages != nil {
				return state.Result
			}

			return TurnHookResult{Messages: state.Request.Messages}
		}), WithHookOrder(10_000)),
		h.Session().Use("sdk.bus_compat", func(_ context.Context, state *HookState[SessionHookRequest, SessionHookResult]) error {
			topic := state.Request.Event
			if topic == "" {
				return nil
			}

			payload := state.Result.Entry
			if payload == nil {
				payload = state.Request.Entry
			}

			bus.Publish(NewEvent(topic, payload))

			return nil
		}, WithHookOrder(10_000)),
	}

	return handles
}

type ProviderRequestBusPayload struct {
	Provider string          `json:"provider"`
	Request  ProviderRequest `json:"request"`
}

type ProviderResponseBusPayload struct {
	Provider string        `json:"provider"`
	Event    ProviderEvent `json:"event"`
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
