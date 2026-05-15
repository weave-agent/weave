package messages

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompactionEntry_View(t *testing.T) {
	e := NewCompactionEntry(5, 10000, 3000)
	view := e.View(80)

	assert.Contains(t, view, "Compacted")
	assert.Contains(t, view, "5 messages summarized")
	assert.Contains(t, view, "10000")
	assert.Contains(t, view, "3000")
	assert.Contains(t, view, "7000 saved")
}

func TestCompactionEntry_ViewZeroWidth(t *testing.T) {
	e := NewCompactionEntry(3, 5000, 1000)
	view := e.View(0)

	assert.Contains(t, view, "Compacted")
	assert.Contains(t, view, "3 messages summarized")
}

func TestCompactionEntry_ViewZeroMessages(t *testing.T) {
	e := NewCompactionEntry(0, 0, 0)
	view := e.View(80)

	assert.Contains(t, view, "0 messages summarized")
	assert.Contains(t, view, "0 saved")
}
