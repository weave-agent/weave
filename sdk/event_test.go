package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEvent_TraceIDPopulated(t *testing.T) {
	evt := NewEvent("test.topic", "payload")
	require.NotEmpty(t, evt.TraceID, "TraceID should be populated")
	assert.Equal(t, 32, len(evt.TraceID), "TraceID should be 32 hex characters")
	assert.Equal(t, "test.topic", evt.Topic)
	assert.Equal(t, "payload", evt.Payload)
}

func TestNewEvent_TraceIDUnique(t *testing.T) {
	ids := make(map[string]struct{})
	for i := range 100 {
		evt := NewEvent("test.topic", i)
		require.NotEmpty(t, evt.TraceID, "TraceID should be populated")
		_, exists := ids[evt.TraceID]
		assert.False(t, exists, "TraceID %s was duplicated", evt.TraceID)
		ids[evt.TraceID] = struct{}{}
	}
}
