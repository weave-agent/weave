package sdk

import "time"

type Event struct {
	Topic     string
	Payload   any
	Timestamp time.Time
}

func NewEvent(topic string, payload any) Event {
	return Event{
		Topic:     topic,
		Payload:   payload,
		Timestamp: time.Now(),
	}
}
