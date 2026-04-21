package tui

import (
	"fmt"
	"sync"

	"weave/sdk"

	tea "github.com/charmbracelet/bubbletea"
)

func init() { //nolint:gochecknoinits // required for extension self-registration
	sdk.RegisterExtension("tui", func(cfg sdk.Config) (sdk.Extension, error) {
		return NewTUI(cfg)
	})
}

// TUI is the terminal UI extension.
type TUI struct {
	cfg sdk.Config

	mu      sync.Mutex
	program *tea.Program
	done    chan struct{}
}

// NewTUI creates a new TUI extension.
func NewTUI(cfg sdk.Config) (*TUI, error) {
	return &TUI{
		cfg:  cfg,
		done: make(chan struct{}),
	}, nil
}

// Name returns the extension name.
func (t *TUI) Name() string { return "tui" }

// Subscribe starts the Bubble Tea program in a goroutine, blocking until it exits.
func (t *TUI) Subscribe(bus sdk.Bus) {
	model := newModel(bus, t.cfg)

	t.mu.Lock()
	t.program = tea.NewProgram(model)
	t.mu.Unlock()

	_, err := t.program.Run()
	if err != nil {
		fmt.Printf("tui error: %v\n", err)
	}

	close(t.done)
}

// Close sends a quit message to the Bubble Tea program.
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
