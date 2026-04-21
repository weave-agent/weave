package tui

import (
	"fmt"
	"os"
	"sync"

	"weave/sdk"

	tea "github.com/charmbracelet/bubbletea"
)

func init() { //nolint:gochecknoinits // required for extension self-registration
	sdk.RegisterExtension("tui", func(cfg sdk.Config) (sdk.Extension, error) {
		t, err := NewTUI(cfg)
		if err != nil {
			return nil, err
		}

		sdk.RegisterUI("tui", t.ui)
		return t, nil
	})
}

// TUI is the terminal UI extension.
type TUI struct {
	cfg sdk.Config

	mu      sync.Mutex
	program *tea.Program
	done    chan struct{}
	ui      *TUIImpl
}

// NewTUI creates a new TUI extension.
// Returns ErrNoTTY if stdin is not a terminal.
func NewTUI(cfg sdk.Config) (*TUI, error) {
	if !isTerminal() {
		return nil, ErrNoTTY
	}

	ui := NewTUIImpl(nil, nil)

	return &TUI{
		cfg:  cfg,
		done: make(chan struct{}),
		ui:   ui,
	}, nil
}

// ErrNoTTY is returned when stdin is not a terminal.
var ErrNoTTY = fmt.Errorf("stdin is not a terminal (use -p for print mode)")

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
// When the program exits (user quit or close), the bus is unsubscribed.
func (t *TUI) Subscribe(bus sdk.Bus) {
	events := bus.SubscribeAll()

	model := newModel(bus, t.cfg)

	t.mu.Lock()
	t.program = tea.NewProgram(model)
	t.mu.Unlock()

	// Wire the UI implementation to the program and model's registries.
	t.ui.SetProgram(t.program)
	t.ui.commands = model.commands
	t.ui.bindings = model.bindings

	go Bridge(t.program, events)

	_, err := t.program.Run()
	if err != nil {
		fmt.Printf("tui error: %v\n", err)
	}

	bus.Unsubscribe(events)

	close(t.done)
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
