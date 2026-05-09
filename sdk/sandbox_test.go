package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSandboxer_NilDefault(t *testing.T) {
	// Reset to ensure clean state
	SetSandboxer(nil)
	assert.Nil(t, GetSandboxer())
}

func TestSetSandboxer_SetAndGet(t *testing.T) {
	mock := &SandboxerMock{
		AllowReadFunc:  func(s string) bool { return true },
		AllowWriteFunc: func(s string) bool { return true },
		WrapCommandFunc: func(cmd, dir string) (string, error) {
			return "wrapped:" + cmd, nil
		},
	}

	SetSandboxer(mock)
	defer SetSandboxer(nil)

	got := GetSandboxer()
	require.NotNil(t, got)

	wrapped, err := got.WrapCommand("ls", "/tmp")
	require.NoError(t, err)
	assert.Equal(t, "wrapped:ls", wrapped)
	assert.True(t, got.AllowWrite("/tmp/file"))
	assert.True(t, got.AllowRead("/tmp/file"))
}

func TestSetSandboxer_Overwrite(t *testing.T) {
	first := &SandboxerMock{
		AllowWriteFunc: func(s string) bool { return false },
	}
	second := &SandboxerMock{
		AllowWriteFunc: func(s string) bool { return true },
	}

	SetSandboxer(first)
	assert.False(t, GetSandboxer().AllowWrite("/any"))

	SetSandboxer(second)
	assert.True(t, GetSandboxer().AllowWrite("/any"))

	SetSandboxer(nil)
}
