package tui

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/weave-agent/weave/sdk"

	tea "charm.land/bubbletea/v2"
)

var (
	sandboxerMu sync.RWMutex
	sandboxer   sdk.Sandboxer
)

func setSandboxer(s sdk.Sandboxer) {
	sandboxerMu.Lock()
	sandboxer = s
	sandboxerMu.Unlock()
}

func getSandboxer() sdk.Sandboxer {
	sandboxerMu.RLock()

	s := sandboxer

	sandboxerMu.RUnlock()

	return s
}

func init() {
	sdk.OnBusReady(func(bus sdk.Bus) {
		bus.On("sandbox.registered", func(ev sdk.Event) error {
			if s, ok := ev.Payload.(sdk.Sandboxer); ok {
				setSandboxer(s)
			}

			return nil
		})
	})

	sdk.RegisterExtensionWithScopeAndWriter("tui", "ui", func(cfg sdk.Config, ps sdk.PreferenceWriter, tuiCfg TUIConfig) (sdk.Extension, error) {
		t, err := NewTUI(cfg, ps, tuiCfg)
		if err != nil {
			return nil, err
		}

		sdk.RegisterUI("tui", t.ui)

		return t, nil
	})
}

// TUI is the terminal UI extension.
type TUI struct {
	cfg    sdk.Config
	ps     sdk.PreferenceStore
	tuiCfg TUIConfig

	mu      sync.Mutex
	program *tea.Program
	done    chan struct{}
	ui      *TUIImpl
}

// NewTUI creates a new TUI extension.
// Returns ErrNoTTY if stdin is not a terminal.
func NewTUI(cfg sdk.Config, ps sdk.PreferenceStore, tuiCfg TUIConfig) (*TUI, error) {
	if !isTerminal() {
		return nil, ErrNoTTY
	}

	ui := NewTUIImpl(nil, nil)

	return &TUI{
		cfg:    cfg,
		ps:     ps,
		done:   make(chan struct{}),
		ui:     ui,
		tuiCfg: tuiCfg,
	}, nil
}

// ErrNoTTY is returned when stdin is not a terminal.
var ErrNoTTY = errors.New("stdin is not a terminal (use -p for print mode)")

// isTerminal checks whether stdin is connected to a terminal.
func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}

	return fi.Mode()&os.ModeCharDevice != 0
}

// Name returns the extension name.
func (t *TUI) Name() string { return "tui" }

// Subscribe starts the Bubble Tea program in a goroutine, blocking until it exits.
// The bridge goroutine translates bus events into tea.Msg and forwards them.
func (t *TUI) Subscribe(bus sdk.Bus) error {
	events := make(chan sdk.Event, 256)

	var eventsMu sync.Mutex

	eventsClosed := false

	bus.OnAll(func(ev sdk.Event) error {
		eventsMu.Lock()
		if eventsClosed {
			eventsMu.Unlock()
			return nil
		}

		select {
		case events <- ev:
		default:
			sdk.Logger("tui").Warn("dropped event, channel full", "topic", ev.Topic)
		}

		eventsMu.Unlock()

		return nil
	})

	model := newModelWithConfig(bus, t.cfg, t.ps, t.ui, t.tuiCfg)

	t.mu.Lock()
	t.program = tea.NewProgram(model)
	t.mu.Unlock()

	// Register UI extensions before setting the program so that
	// SetStatus calls during registration are buffered (not sent).
	t.wireUIExtensions(bus)

	// Wire the UI implementation to the program.
	t.ui.SetProgram(t.program)

	go Bridge(t.program, events)

	_, err := t.program.Run()

	t.ui.Close()

	eventsMu.Lock()
	eventsClosed = true

	close(events)
	eventsMu.Unlock()

	endPayload := any(nil)
	if err != nil {
		endPayload = fmt.Sprintf("tui error: %v", err)
	}

	// Signal shutdown so the launcher's select (waiting on agent.end)
	// can unblock and proceed to wired.Close().
	bus.Publish(sdk.NewEvent(topicEnd, endPayload))

	close(t.done)

	return nil
}

// wireUIExtensions registers all UI extensions with the TUI's UI implementation.
// Extensions that implement UIExtensionWithBus also receive the event bus.
func (t *TUI) wireUIExtensions(bus sdk.Bus) {
	for _, ext := range sdk.GetUIExtensions(t.cfg) {
		ext.Register(t.ui)

		if withBus, ok := ext.(sdk.UIExtensionWithBus); ok {
			withBus.RegisterWithBus(t.ui, bus)
		}
	}
}

// Close sends a quit message to the Bubble Tea program and waits for it to finish.
func (t *TUI) Close() error {
	t.mu.Lock()
	p := t.program
	t.mu.Unlock()

	if p != nil {
		p.Quit()
		<-t.done
	}

	return nil
}
