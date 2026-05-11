package sdk

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegisterOutputWriterSetter(t *testing.T) {
	ResetOutputWriterSetters()

	var buf bytes.Buffer

	var called bool

	RegisterOutputWriterSetter(func(w io.Writer) {
		called = true
		_, _ = buf.WriteString("hello")
	})

	SetOutputWriters(&buf)
	assert.True(t, called)
	assert.Equal(t, "hello", buf.String())
}

func TestSetOutputWriters_MultipleSetters(t *testing.T) {
	ResetOutputWriterSetters()

	var buf bytes.Buffer

	RegisterOutputWriterSetter(func(w io.Writer) {
		_, _ = buf.WriteString("a")
	})
	RegisterOutputWriterSetter(func(w io.Writer) {
		_, _ = buf.WriteString("b")
	})

	SetOutputWriters(&buf)
	assert.Equal(t, "ab", buf.String())
}

func TestResetOutputWriterSetters(t *testing.T) {
	ResetOutputWriterSetters()

	var called bool

	RegisterOutputWriterSetter(func(w io.Writer) {
		called = true
	})

	ResetOutputWriterSetters()
	SetOutputWriters(nil)
	assert.False(t, called)
}
