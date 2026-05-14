package log

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "logs", "weave.log")

	Setup(logFile, false)
	defer func() {
		setupOnce = sync.Once{}
		initialized.Store(false)
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	}()

	_, err := os.Stat(filepath.Dir(logFile))
	assert.NoError(t, err, "log directory should be created")
}

func TestSetupWritesLogFile(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "weave.log")

	Setup(logFile, false)
	defer func() {
		setupOnce = sync.Once{}
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
	Setup(logFile, true, &buf)
	defer func() {
		setupOnce = sync.Once{}
		initialized.Store(false)
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	}()

	slog.Debug("debug message")

	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "debug message")
}

func TestSetupInfoLevelIgnoresDebug(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "weave-info.log")

	Setup(logFile, false)
	defer func() {
		setupOnce = sync.Once{}
		initialized.Store(false)
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	}()

	slog.Info("setup marker")
	slog.Debug("should not appear")

	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "setup marker")
	assert.False(t, strings.Contains(string(content), "should not appear"))
}

func TestInitializedAfterSetup(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "weave.log")

	assert.False(t, Initialized())

	Setup(logFile, false)
	defer func() {
		setupOnce = sync.Once{}
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
	Setup(logFile1, false, &buf1)
	defer func() {
		setupOnce = sync.Once{}
		initialized.Store(false)
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	}()

	slog.Info("first")

	// Second call should be ignored
	var buf2 bytes.Buffer
	Setup(logFile2, true, &buf2)

	slog.Info("second")

	// Both messages should go to the first setup
	content1, err := os.ReadFile(logFile1)
	require.NoError(t, err)
	assert.Contains(t, string(content1), "first")
	assert.Contains(t, string(content1), "second")

	// second.log should not exist or be empty
	_, err = os.Stat(logFile2)
	assert.True(t, os.IsNotExist(err) || len(content1) > 0)
}
