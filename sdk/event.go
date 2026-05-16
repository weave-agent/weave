package sdk

import (
	"crypto/rand"
	"encoding/hex"
	"time"
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
		TraceID:   generateTraceID(),
	}
}

func generateTraceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// ReadDonePayload is the payload for the "tool.read.done" bus event.
type ReadDonePayload struct {
	Path    string    `json:"path"`
	ModTime time.Time `json:"mod_time"`
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
