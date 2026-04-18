package sdk

import (
	"testing"
	"time"
)

func TestNewEvent(t *testing.T) {
	before := time.Now()
	evt := NewEvent("test.topic", "hello")
	after := time.Now()

	if evt.Topic != "test.topic" {
		t.Errorf("Topic = %q, want %q", evt.Topic, "test.topic")
	}

	if evt.Payload != "hello" {
		t.Errorf("Payload = %v, want %v", evt.Payload, "hello")
	}

	if evt.Timestamp.Before(before) || evt.Timestamp.After(after) {
		t.Errorf("Timestamp %v not between %v and %v", evt.Timestamp, before, after)
	}
}

func TestEventNilPayload(t *testing.T) {
	evt := NewEvent("empty", nil)
	if evt.Payload != nil {
		t.Errorf("Payload = %v, want nil", evt.Payload)
	}
}

type mockConfig struct {
	filePath string
}

func (m *mockConfig) FilePath() string { return m.filePath }

var _ Config = (*mockConfig)(nil)

func TestConfigInterface(t *testing.T) {
	cfg := &mockConfig{filePath: "/path/to/.weave.yaml"}
	if v := cfg.FilePath(); v != "/path/to/.weave.yaml" {
		t.Errorf("FilePath = %q, want %q", v, "/path/to/.weave.yaml")
	}
}

type mockBus struct {
	published []Event
}

func (m *mockBus) Publish(e Event)                         { m.published = append(m.published, e) }
func (m *mockBus) Subscribe(topics ...string) <-chan Event { return nil }
func (m *mockBus) SubscribeAll() <-chan Event              { return nil }

var _ Bus = (*mockBus)(nil)

func TestExtensionFunc(t *testing.T) {
	var subscribed bool

	ext := NewExtensionFunc("test-ext", func(b Bus) {
		subscribed = true
	})

	if ext.Name() != "test-ext" {
		t.Errorf("Name() = %q, want %q", ext.Name(), "test-ext")
	}

	bus := &mockBus{}
	ext.Subscribe(bus)

	if !subscribed {
		t.Error("Subscribe callback was not called")
	}
}

func TestExtensionFuncSatisfiesInterface(t *testing.T) {
	var _ Extension = NewExtensionFunc("x", func(Bus) {})

	ext := Extension(NewExtensionFunc("check", func(b Bus) {
		b.Publish(NewEvent("fired", nil))
	}))
	bus := &mockBus{}
	ext.Subscribe(bus)

	if len(bus.published) != 1 {
		t.Fatalf("published events = %d, want 1", len(bus.published))
	}

	if bus.published[0].Topic != "fired" {
		t.Errorf("topic = %q, want %q", bus.published[0].Topic, "fired")
	}
}

func TestExtensionFuncMultipleSubscriptions(t *testing.T) {
	var calls []string

	ext := NewExtensionFunc("multi", func(b Bus) {
		calls = append(calls, "called")

		b.Publish(NewEvent("e1", 1))
		b.Publish(NewEvent("e2", 2))
	})

	bus := &mockBus{}
	ext.Subscribe(bus)

	if len(calls) != 1 {
		t.Errorf("callback calls = %d, want 1", len(calls))
	}

	if len(bus.published) != 2 {
		t.Errorf("published = %d, want 2", len(bus.published))
	}

	topics := []string{bus.published[0].Topic, bus.published[1].Topic}

	want := []string{"e1", "e2"}
	for i, w := range want {
		if topics[i] != w {
			t.Errorf("event[%d].Topic = %q, want %q", i, topics[i], w)
		}
	}
}
