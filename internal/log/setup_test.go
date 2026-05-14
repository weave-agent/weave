package log

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "logs", "weave.log")

	err := Setup(logFile, false)
	require.NoError(t, err)

	defer func() {
		setupOnce = sync.Once{}
		setupError = nil

		initialized.Store(false)
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	}()

	_, err = os.Stat(filepath.Dir(logFile))
	assert.NoError(t, err, "log directory should be created")
}

func TestSetupWritesLogFile(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "weave.log")

	err := Setup(logFile, false)
	require.NoError(t, err)

	defer func() {
		setupOnce = sync.Once{}
		setupError = nil

		initialized.Store(false)
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	}()

	slog.Info("test message", "key", "value")

	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "test message")
	assert.Contains(t, string(content), `"key":"value"`)
}

func TestSetupDebugFlag(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "weave-debug.log")

	var buf bytes.Buffer

	err := Setup(logFile, true, &buf)
	require.NoError(t, err)

	defer func() {
		setupOnce = sync.Once{}
		setupError = nil

		initialized.Store(false)
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	}()

	slog.Debug("debug message")

	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "debug message")
	assert.Contains(t, buf.String(), "debug message", "extra writer should also receive debug output")
}

func TestSetupInfoLevelIgnoresDebug(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "weave-info.log")

	err := Setup(logFile, false)
	require.NoError(t, err)

	defer func() {
		setupOnce = sync.Once{}
		setupError = nil

		initialized.Store(false)
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	}()

	slog.Info("setup marker")
	slog.Debug("should not appear")

	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "setup marker")
	assert.NotContains(t, string(content), "should not appear")
}

func TestInitializedAfterSetup(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "weave.log")

	assert.False(t, Initialized())

	err := Setup(logFile, false)
	require.NoError(t, err)

	defer func() {
		setupOnce = sync.Once{}
		setupError = nil

		initialized.Store(false)
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	}()

	assert.True(t, Initialized())
}

func TestSetupOnce(t *testing.T) {
	tmpDir := t.TempDir()
	logFile1 := filepath.Join(tmpDir, "first.log")
	logFile2 := filepath.Join(tmpDir, "second.log")

	var buf1 bytes.Buffer

	err := Setup(logFile1, false, &buf1)
	require.NoError(t, err)

	defer func() {
		setupOnce = sync.Once{}
		setupError = nil

		initialized.Store(false)
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	}()

	slog.Info("first")

	// Second call should be ignored
	var buf2 bytes.Buffer

	err = Setup(logFile2, true, &buf2)
	require.NoError(t, err)

	slog.Info("second")

	// Both messages should go to the first setup
	content1, err := os.ReadFile(logFile1)
	require.NoError(t, err)
	assert.Contains(t, string(content1), "first")
	assert.Contains(t, string(content1), "second")

	// second.log should not exist
	_, err = os.Stat(logFile2)
	assert.True(t, os.IsNotExist(err), "second log file should not be created when Setup is called twice")
}
