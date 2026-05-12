package sdk

import "time"

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

// OutdatedInfo describes a single extension that has a newer version available.
type OutdatedInfo struct {
	Name       string
	LocalHead  string
	RemoteHead string
}

// OutdatedEvent is the payload for the "extension.outdated" bus event.
type OutdatedEvent struct {
	Extensions []OutdatedInfo
}
