package filetracker

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTrackerRecordAndQuery(t *testing.T) {
	ft := New()

	assert.False(t, ft.WasRead("/foo/bar.go"))

	modTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	ft.RecordRead("/foo/bar.go", modTime)

	assert.True(t, ft.WasRead("/foo/bar.go"))

	gotTime, ok := ft.GetReadTime("/foo/bar.go")
	assert.True(t, ok)
	assert.Equal(t, modTime, gotTime)
}

func TestTrackerNotFound(t *testing.T) {
	ft := New()

	gotTime, ok := ft.GetReadTime("/not/recorded.go")
	assert.False(t, ok)
	assert.True(t, gotTime.IsZero())
}

func TestTrackerOverwrite(t *testing.T) {
	ft := New()

	oldTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newTime := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	ft.RecordRead("/foo.go", oldTime)
	ft.RecordRead("/foo.go", newTime)

	gotTime, ok := ft.GetReadTime("/foo.go")
	assert.True(t, ok)
	assert.Equal(t, newTime, gotTime)
}

func TestTrackerConcurrentAccess(t *testing.T) {
	ft := New()
	modTime := time.Now()

	var wg sync.WaitGroup

	// Concurrent writes
	for range 100 {
		wg.Go(func() {
			ft.RecordRead("/shared/file.go", modTime)
		})
	}

	// Concurrent reads
	for range 100 {
		wg.Go(func() {
			_ = ft.WasRead("/shared/file.go")
			_, _ = ft.GetReadTime("/shared/file.go")
		})
	}

	wg.Wait()

	assert.True(t, ft.WasRead("/shared/file.go"))
}

func TestTrackerMultipleFiles(t *testing.T) {
	ft := New()
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

	ft.RecordRead("/a.go", t1)
	ft.RecordRead("/b.go", t2)

	assert.True(t, ft.WasRead("/a.go"))
	assert.True(t, ft.WasRead("/b.go"))
	assert.False(t, ft.WasRead("/c.go"))

	gotA, ok := ft.GetReadTime("/a.go")
	assert.True(t, ok)
	assert.Equal(t, t1, gotA)

	gotB, ok := ft.GetReadTime("/b.go")
	assert.True(t, ok)
	assert.Equal(t, t2, gotB)
}
