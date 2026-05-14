package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockFileMuter is a minimal FileMuter implementation for testing.
type mockFileMuter struct {
	locked bool
}

func (m *mockFileMuter) Lock(_ string) func() {
	m.locked = true

	return func() { m.locked = false }
}

func TestGetFileMutex_NilDefault(t *testing.T) {
	SetFileMutex(nil)
	assert.Nil(t, GetFileMutex())
}

func TestSetFileMutex_SetAndGet(t *testing.T) {
	mock := &mockFileMuter{}
	SetFileMutex(mock)

	defer SetFileMutex(nil)

	got := GetFileMutex()
	assert.NotNil(t, got)
	assert.Equal(t, mock, got)
}

func TestSetFileMutex_Overwrite(t *testing.T) {
	first := &mockFileMuter{}
	second := &mockFileMuter{}

	SetFileMutex(first)
	assert.Equal(t, first, GetFileMutex())

	SetFileMutex(second)
	assert.Equal(t, second, GetFileMutex())

	SetFileMutex(nil)
	assert.Nil(t, GetFileMutex())
}

func TestFileMutexInterfaceMethods(t *testing.T) {
	mock := &mockFileMuter{}
	SetFileMutex(mock)

	defer SetFileMutex(nil)

	fm := GetFileMutex()
	assert.NotNil(t, fm)

	unlock := fm.Lock("/tmp/test.txt")

	assert.True(t, mock.locked)

	unlock()
	assert.False(t, mock.locked)
}
