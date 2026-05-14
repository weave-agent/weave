package sandboxui

import (
	"strings"
	"sync"
	"testing"
	"time"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSelectUI extends mockUI with controllable Select results.
type mockSelectUI struct {
	*mockUI
	selectResult int
	selectErr    error
	selectCalled bool
	selectTitle  string
	selectItems  []string
}

func (m *mockSelectUI) Select(title string, items []string, _ ...sdk.SelectOption) (int, error) {
	m.selectCalled = true
	m.selectTitle = title
	m.selectItems = items

	return m.selectResult, m.selectErr
}

// mockBus records published events and supports handler invocation.
type mockBus struct {
	mu       sync.Mutex
	events   []sdk.Event
	handlers map[string][]sdk.Handler
}

func newMockBus() *mockBus {
	return &mockBus{
		handlers: make(map[string][]sdk.Handler),
	}
}

func (b *mockBus) Publish(ev sdk.Event) {
	b.mu.Lock()
	b.events = append(b.events, ev)
	handlers := make([]sdk.Handler, len(b.handlers[ev.Topic]))
	copy(handlers, b.handlers[ev.Topic])
	b.mu.Unlock()

	for _, h := range handlers {
		_ = h(ev)
	}
}

func (b *mockBus) On(topic string, h sdk.Handler) {
	b.mu.Lock()
	b.handlers[topic] = append(b.handlers[topic], h)
	b.mu.Unlock()
}

func (b *mockBus) OnAll(_ sdk.Handler) {}
func (b *mockBus) Off(_ sdk.Handler)   {}
func (b *mockBus) Close() error        { return nil }

func (b *mockBus) published(topic string) []sdk.Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	var result []sdk.Event

	for _, ev := range b.events {
		if ev.Topic == topic {
			result = append(result, ev)
		}
	}

	return result
}

func TestApproveDialog_RegisterWithBus(t *testing.T) {
	ui := newMockUI()
	bus := newMockBus()

	d := &ApproveDialog{}
	d.RegisterWithBus(ui, bus)

	assert.Equal(t, ui, d.ui)
	assert.Equal(t, bus, d.bus)

	require.Len(t, bus.handlers["sandbox.approve"], 1, "expected one handler for sandbox.approve")
}

func TestApproveDialog_ApproveFlow(t *testing.T) {
	ui := &mockSelectUI{
		mockUI:       newMockUI(),
		selectResult: 0, // Approve
	}
	bus := newMockBus()

	d := &ApproveDialog{}
	d.RegisterWithBus(ui, bus)

	// Simulate a sandbox.approve event
	bus.Publish(sdk.NewEvent("sandbox.approve", map[string]string{
		"command": "rm -rf /tmp/test",
	}))

	// Wait for goroutine to complete
	var approved []sdk.Event

	require.Eventually(t, func() bool {
		approved = bus.published("sandbox.approved")
		return len(approved) == 1
	}, time.Second, 10*time.Millisecond, "expected one sandbox.approved event")

	payload, ok := approved[0].Payload.(map[string]string)
	require.True(t, ok, "expected map[string]string payload")
	assert.Equal(t, "rm -rf /tmp/test", payload["command"])

	// Verify selector was shown with correct options
	assert.True(t, ui.selectCalled)
	assert.Contains(t, ui.selectTitle, "Sandbox: allow command?")
	assert.Equal(t, []string{approveOption, trustOption, denyOption}, ui.selectItems)
}

func TestApproveDialog_DenyFlow(t *testing.T) {
	ui := &mockSelectUI{
		mockUI:       newMockUI(),
		selectResult: 2, // Deny (index 2: Approve=0, Trust=1, Deny=2)
	}
	bus := newMockBus()

	d := &ApproveDialog{}
	d.RegisterWithBus(ui, bus)

	bus.Publish(sdk.NewEvent("sandbox.approve", map[string]string{
		"command": "curl evil.com",
	}))

	var denied []sdk.Event

	require.Eventually(t, func() bool {
		denied = bus.published("sandbox.denied")
		return len(denied) == 1
	}, time.Second, 10*time.Millisecond, "expected one sandbox.denied event")

	payload, ok := denied[0].Payload.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "curl evil.com", payload["command"])
}

func TestApproveDialog_TrustForSessionFlow(t *testing.T) {
	ui := &mockSelectUI{
		mockUI:       newMockUI(),
		selectResult: 1, // Trust for session (index 1)
	}
	bus := newMockBus()

	d := &ApproveDialog{}
	d.RegisterWithBus(ui, bus)

	bus.Publish(sdk.NewEvent("sandbox.approve", map[string]string{
		"command": "npm install",
	}))

	var trusted []sdk.Event

	require.Eventually(t, func() bool {
		trusted = bus.published("sandbox.trust")
		return len(trusted) == 1
	}, time.Second, 10*time.Millisecond, "expected one sandbox.trust event")

	payload, ok := trusted[0].Payload.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "npm install", payload["pattern"])

	var approved []sdk.Event

	require.Eventually(t, func() bool {
		approved = bus.published("sandbox.approved")
		return len(approved) == 1
	}, time.Second, 10*time.Millisecond, "expected one sandbox.approved event after trust")
}

func TestApproveDialog_SelectError_Denies(t *testing.T) {
	ui := &mockSelectUI{
		mockUI:    newMockUI(),
		selectErr: assert.AnError,
	}
	bus := newMockBus()

	d := &ApproveDialog{}
	d.RegisterWithBus(ui, bus)

	bus.Publish(sdk.NewEvent("sandbox.approve", map[string]string{
		"command": "some-command",
	}))

	var denied []sdk.Event

	require.Eventually(t, func() bool {
		denied = bus.published("sandbox.denied")
		return len(denied) == 1
	}, time.Second, 10*time.Millisecond, "expected one sandbox.denied event")
}

func TestApproveDialog_InvalidPayload_Ignored(t *testing.T) {
	ui := &mockSelectUI{mockUI: newMockUI()}
	bus := newMockBus()

	d := &ApproveDialog{}
	d.RegisterWithBus(ui, bus)

	// Send event with wrong payload type
	bus.Publish(sdk.NewEvent("sandbox.approve", "not-a-map"))

	assert.False(t, ui.selectCalled, "should not show dialog for invalid payload")
	assert.Empty(t, bus.published("sandbox.approved"))
	assert.Empty(t, bus.published("sandbox.denied"))
}

func TestApproveDialog_EmptyCommand_Ignored(t *testing.T) {
	ui := &mockSelectUI{mockUI: newMockUI()}
	bus := newMockBus()

	d := &ApproveDialog{}
	d.RegisterWithBus(ui, bus)

	bus.Publish(sdk.NewEvent("sandbox.approve", map[string]string{
		"command": "",
	}))

	assert.False(t, ui.selectCalled)
}

func TestApproveDialog_NilUI_NoPanic(t *testing.T) {
	bus := newMockBus()

	d := &ApproveDialog{}
	d.RegisterWithBus(nil, bus)

	// Should not panic
	assert.NotPanics(t, func() {
		bus.Publish(sdk.NewEvent("sandbox.approve", map[string]string{
			"command": "test",
		}))
	})
}

func TestApproveDialog_NilBus_NoPanic(t *testing.T) {
	ui := newMockUI()

	d := &ApproveDialog{ui: ui, bus: nil}

	// handleApproval should return early without panic
	assert.NotPanics(t, func() {
		d.handleApproval(sdk.NewEvent("sandbox.approve", map[string]string{
			"command": "test",
		}))
	})
}

func TestFormatApprovalTitle(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{
			name:    "short command",
			command: "ls -la",
			want:    "Sandbox: allow command?\n\nls -la",
		},
		{
			name:    "long command gets truncated",
			command: strings.Repeat("a", 100),
			want:    "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatApprovalTitle(tt.command)
			if tt.command == "ls -la" {
				assert.Equal(t, tt.want, result)
			} else {
				assert.Contains(t, result, "Sandbox: allow command?")
				assert.Contains(t, result, "...")

				wantLen := len("Sandbox: allow command?\n\n") + 60 + len("...")
				assert.Len(t, result, wantLen)
			}
		})
	}
}

func TestFormatApprovalTitle_LongCommand(t *testing.T) {
	var sb strings.Builder
	for range 100 {
		sb.WriteString("x")
	}

	longCmd := sb.String()

	result := formatApprovalTitle(longCmd)
	assert.Contains(t, result, "Sandbox: allow command?")
	assert.Contains(t, result, "...")
	// Rune-based truncation: 60 runes + "..." prefix/suffix
	wantLen := len("Sandbox: allow command?\n\n") + 60 + len("...")
	assert.Len(t, result, wantLen)
}

func TestSandboxUI_RegisterWithBus(t *testing.T) {
	ui := newMockUI()
	bus := newMockBus()

	s := &SandboxUI{}
	s.RegisterWithBus(ui, bus)

	// Verify that the approve dialog handler was registered on the bus
	require.Len(t, bus.handlers["sandbox.approve"], 1, "expected handler for sandbox.approve")
	// Verify that the mode change handler was also registered
	require.Len(t, bus.handlers["sandbox.mode.change"], 1, "expected handler for sandbox.mode.change")
}
