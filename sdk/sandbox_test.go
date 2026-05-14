package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSandboxerMock_WrapCommand(t *testing.T) {
	mock := &SandboxerMock{
		AllowReadFunc:  func(s string) bool { return true },
		AllowWriteFunc: func(s string) bool { return true },
		WrapCommandFunc: func(cmd, dir string) (string, error) {
			return "wrapped:" + cmd, nil
		},
	}

	wrapped, err := mock.WrapCommand("ls", "/tmp")
	require.NoError(t, err)
	assert.Equal(t, "wrapped:ls", wrapped)
}

func TestSandboxerMock_AllowWrite(t *testing.T) {
	mock := &SandboxerMock{
		AllowWriteFunc: func(s string) bool { return s == "/allowed" },
	}

	assert.True(t, mock.AllowWrite("/allowed"))
	assert.False(t, mock.AllowWrite("/denied"))
}

func TestSandboxerMock_AllowRead(t *testing.T) {
	mock := &SandboxerMock{
		AllowReadFunc: func(s string) bool { return s == "/allowed" },
	}

	assert.True(t, mock.AllowRead("/allowed"))
	assert.False(t, mock.AllowRead("/denied"))
}

func TestSandboxerMock_Mode(t *testing.T) {
	mock := &SandboxerMock{
		ModeFunc: func() string { return "auto" },
	}

	assert.Equal(t, "auto", mock.Mode())
}

func TestSandboxerMock_SetMode(t *testing.T) {
	var lastMode string

	mock := &SandboxerMock{
		SetModeFunc: func(mode string) { lastMode = mode },
	}

	mock.SetMode("readonly")
	assert.Equal(t, "readonly", lastMode)
}
