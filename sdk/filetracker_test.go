package sdk

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockFileTracker is a minimal FileTracker implementation for testing.
type mockFileTracker struct {
	recordedPath    string
	recordedModTime time.Time
	wasReadResult   bool
	readTime        time.Time
	readTimeOK      bool
}

func (m *mockFileTracker) RecordRead(path string, modTime time.Time) {
	m.recordedPath = path
	m.recordedModTime = modTime
}

func (m *mockFileTracker) WasRead(path string) bool {
	_ = path
	return m.wasReadResult
}

func (m *mockFileTracker) GetReadTime(path string) (time.Time, bool) {
	_ = path
	return m.readTime, m.readTimeOK
}

func TestGetFileTracker_NilDefault(t *testing.T) {
	SetFileTracker(nil)
	assert.Nil(t, GetFileTracker())
}

func TestSetFileTracker_SetAndGet(t *testing.T) {
	mock := &mockFileTracker{wasReadResult: true}

	SetFileTracker(mock)
	defer SetFileTracker(nil)

	got := GetFileTracker()
	assert.NotNil(t, got)
	assert.True(t, got.WasRead("/any"))
}

func TestSetFileTracker_Overwrite(t *testing.T) {
	first := &mockFileTracker{wasReadResult: false}
	second := &mockFileTracker{wasReadResult: true}

	SetFileTracker(first)
	assert.False(t, GetFileTracker().WasRead("/any"))

	SetFileTracker(second)
	assert.True(t, GetFileTracker().WasRead("/any"))

	SetFileTracker(nil)
	assert.Nil(t, GetFileTracker())
}

func TestFileTrackerInterfaceMethods(t *testing.T) {
	mock := &mockFileTracker{}

	SetFileTracker(mock)
	defer SetFileTracker(nil)

	ft := GetFileTracker()
	a := assert.New(t)

	ft.RecordRead("/test.go", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	a.Equal("/test.go", mock.recordedPath)
	a.Equal(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), mock.recordedModTime)

	mock.wasReadResult = true

	a.True(ft.WasRead("/test.go"))

	mock.readTime = time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	mock.readTimeOK = true

	gotTime, ok := ft.GetReadTime("/test.go")
	a.True(ok)
	a.Equal(mock.readTime, gotTime)
}
