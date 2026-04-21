package messages

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserMessage_Content(t *testing.T) {
	m := NewUserMessage("hello agent")
	assert.Equal(t, "hello agent", m.Content())
}

func TestUserMessage_View(t *testing.T) {
	m := NewUserMessage("fix the bug")
	assert.Equal(t, "fix the bug", m.View(80))
}

func TestUserMessage_EmptyContent(t *testing.T) {
	m := NewUserMessage("")
	assert.Equal(t, "", m.Content())
	assert.Equal(t, "", m.View(80))
}
