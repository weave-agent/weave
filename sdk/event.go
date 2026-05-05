package sdk

import "time"

// Shared bus topic constants for cross-extension communication.
const (
	TopicSkillsLoaded     = "skills.loaded"
	TopicInstructionsLoaded = "instructions.loaded"
)

type Event struct {
	Topic     string
	Payload   any
	Timestamp time.Time
	TraceID   string
}

func NewEvent(topic string, payload any) Event {
	return Event{
		Topic:     topic,
		Payload:   payload,
		Timestamp: time.Now(),
	}
}
