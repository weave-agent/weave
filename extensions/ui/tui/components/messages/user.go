package messages

// UserMessage renders a user-sent message.
type UserMessage struct {
	content string
}

// NewUserMessage creates a new user message.
func NewUserMessage(content string) *UserMessage {
	return &UserMessage{content: content}
}

// Content returns the message text.
func (m *UserMessage) Content() string {
	return m.content
}

// View renders the user message.
func (m *UserMessage) View(width int) string {
	return m.content
}
