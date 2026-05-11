package sdk

import (
	"io"
	"sync"
)

var (
	outputWriterSetters   []func(io.Writer)
	outputWriterSettersMu sync.Mutex
)

// RegisterOutputWriterSetter registers a function that will be called with
// the output writer when the generated binary starts. Extensions that need
// to write to the shared output stream (e.g., subagent messaging) register
// their setter here instead of requiring a special import in generated code.
func RegisterOutputWriterSetter(fn func(io.Writer)) {
	outputWriterSettersMu.Lock()
	defer outputWriterSettersMu.Unlock()

	outputWriterSetters = append(outputWriterSetters, fn)
}

// SetOutputWriters calls all registered output writer setters with w.
func SetOutputWriters(w io.Writer) {
	outputWriterSettersMu.Lock()
	setters := make([]func(io.Writer), len(outputWriterSetters))
	copy(setters, outputWriterSetters)
	outputWriterSettersMu.Unlock()

	for _, fn := range setters {
		fn(w)
	}
}

// ResetOutputWriterSetters clears all registered setters. Used in tests.
func ResetOutputWriterSetters() {
	outputWriterSettersMu.Lock()
	defer outputWriterSettersMu.Unlock()

	outputWriterSetters = nil
}
