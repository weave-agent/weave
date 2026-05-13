package agent

import (
	"testing"

	"weave/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptBuilder_Creation(t *testing.T) {
	pb := newPromptBuilder(sdk.FilePathConfig(""))
	require.NotNil(t, pb)
	assert.NotNil(t, pb.cfg)
}
