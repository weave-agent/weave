package extmanage

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/weave-agent/weave/sdk"
)

// busMock is a simple mock of sdk.Bus for testing.
type busMock struct {
	publishFunc func(ev sdk.Event)
}

func (m *busMock) Publish(event sdk.Event) {
	if m.publishFunc != nil {
		m.publishFunc(event)
	}
}

func (m *busMock) On(topic string, h sdk.Handler) {}

func (m *busMock) OnAll(h sdk.Handler) {}

func (m *busMock) Off(h sdk.Handler) {}

func (m *busMock) Close() error { return nil }

// newRecorderBus returns a busMock that records published events.
func newRecorderBus(t *testing.T) (*busMock, func() []sdk.Event) {
	t.Helper()

	var mu sync.Mutex

	var recorded []sdk.Event

	bus := &busMock{
		publishFunc: func(ev sdk.Event) {
			mu.Lock()

			recorded = append(recorded, ev)
			mu.Unlock()
		},
	}

	getEvents := func() []sdk.Event {
		mu.Lock()
		defer mu.Unlock()

		out := make([]sdk.Event, len(recorded))
		copy(out, recorded)

		return out
	}

	return bus, getEvents
}

func TestFireUpdateCheck_NoExtensions(t *testing.T) {
	setupExtensionsDir(t)

	bus, events := newRecorderBus(t)
	FireUpdateCheck(bus)

	assert.Empty(t, events())
}

func TestFireUpdateCheck_OutdatedExtension(t *testing.T) {
	gitIsAvailable(t)
	extDir := setupExtensionsDir(t)

	bareDir := initBareRepo(t)

	// Clone from bare repo.
	localDir := filepath.Join(extDir, "my-tool")
	runGit(t, t.TempDir(), "clone", bareDir, localDir)

	// Advance the remote.
	tmpClone := filepath.Join(t.TempDir(), "advance-clone")
	runGit(t, t.TempDir(), "clone", bareDir, tmpClone)
	runGit(t, tmpClone, "config", "user.email", "test@test.com")
	runGit(t, tmpClone, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(tmpClone, "feature.go"), []byte("package main\n"), 0o600))
	runGit(t, tmpClone, "add", ".")
	runGit(t, tmpClone, "commit", "-m", "advance remote")
	runGit(t, tmpClone, "push", "origin", "HEAD")

	bus, events := newRecorderBus(t)
	FireUpdateCheck(bus)

	evts := events()
	require.Len(t, evts, 1)
	assert.Equal(t, "extension.outdated", evts[0].Topic)

	payload, ok := evts[0].Payload.(sdk.OutdatedEvent)
	require.True(t, ok)
	require.Len(t, payload.Extensions, 1)
	assert.Equal(t, "my-tool", payload.Extensions[0].Name)
	assert.NotEmpty(t, payload.Extensions[0].LocalHead)
	assert.NotEmpty(t, payload.Extensions[0].RemoteHead)
	assert.NotEqual(t, payload.Extensions[0].LocalHead, payload.Extensions[0].RemoteHead)
}

func TestFireUpdateCheck_UpToDateExtension(t *testing.T) {
	gitIsAvailable(t)
	extDir := setupExtensionsDir(t)

	// Create git extension with remote pointing to itself (up-to-date).
	gitExt := filepath.Join(extDir, "my-tool")
	require.NoError(t, os.MkdirAll(gitExt, 0o750))
	initGitRepo(t, gitExt)
	runGit(t, gitExt, "remote", "add", "origin", gitExt)

	bus, events := newRecorderBus(t)
	FireUpdateCheck(bus)

	assert.Empty(t, events())
}

func TestFireUpdateCheck_LocalExtensionSkipped(t *testing.T) {
	extDir := setupExtensionsDir(t)

	// Create a local (non-git) extension.
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, "local-tool"), 0o750))

	bus, events := newRecorderBus(t)
	FireUpdateCheck(bus)

	assert.Empty(t, events())
}

func TestFireUpdateCheck_MixedExtensions(t *testing.T) {
	gitIsAvailable(t)
	extDir := setupExtensionsDir(t)

	bareDir := initBareRepo(t)

	// Outdated git extension.
	outdatedDir := filepath.Join(extDir, "outdated-tool")
	runGit(t, t.TempDir(), "clone", bareDir, outdatedDir)

	// Advance the remote.
	tmpClone := filepath.Join(t.TempDir(), "advance-clone")
	runGit(t, t.TempDir(), "clone", bareDir, tmpClone)
	runGit(t, tmpClone, "config", "user.email", "test@test.com")
	runGit(t, tmpClone, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(tmpClone, "feature.go"), []byte("package main\n"), 0o600))
	runGit(t, tmpClone, "add", ".")
	runGit(t, tmpClone, "commit", "-m", "advance remote")
	runGit(t, tmpClone, "push", "origin", "HEAD")

	// Up-to-date git extension.
	upToDateDir := filepath.Join(extDir, "uptodate-tool")
	require.NoError(t, os.MkdirAll(upToDateDir, 0o750))
	initGitRepo(t, upToDateDir)
	runGit(t, upToDateDir, "remote", "add", "origin", upToDateDir)

	// Local extension (no git).
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, "local-tool"), 0o750))

	bus, events := newRecorderBus(t)
	FireUpdateCheck(bus)

	evts := events()
	require.Len(t, evts, 1)

	payload, ok := evts[0].Payload.(sdk.OutdatedEvent)
	require.True(t, ok)
	require.Len(t, payload.Extensions, 1)
	assert.Equal(t, "outdated-tool", payload.Extensions[0].Name)
}

func TestFireUpdateCheck_OfflineMode(t *testing.T) {
	setupExtensionsDir(t)
	t.Setenv("WEAVE_OFFLINE", "1")

	bus, events := newRecorderBus(t)
	FireUpdateCheck(bus)

	assert.Empty(t, events())
}
