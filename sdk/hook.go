package sdk

import "io"

var outputWriterSetters []func(io.Writer)

// RegisterOutputWriterSetter registers a function that will be called with
// the output writer when the generated binary starts. Extensions that need
// to write to the shared output stream (e.g., subagent messaging) register
// their setter here instead of requiring a special import in generated code.
func RegisterOutputWriterSetter(fn func(io.Writer)) {
	outputWriterSetters = append(outputWriterSetters, fn)
}

// SetOutputWriters calls all registered output writer setters with w.
func SetOutputWriters(w io.Writer) {
	for _, fn := range outputWriterSetters {
		fn(w)
	}
}

// ResetOutputWriterSetters clears all registered setters. Used in tests.
func ResetOutputWriterSetters() {
	outputWriterSetters = nil
}
