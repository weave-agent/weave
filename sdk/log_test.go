package sdk

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggerAddsExtAttribute(t *testing.T) {
	var buf bytes.Buffer

	orig := slog.Default()

	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(orig)

	log := Logger("test-ext")
	log.Info("hello world", "key", "value")

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))

	assert.Equal(t, "test-ext", record["ext"])
	assert.Equal(t, "hello world", record["msg"])
	assert.Equal(t, "value", record["key"])
}

func TestLoggerUsesCurrentDefault(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	orig := slog.Default()
	defer slog.SetDefault(orig)

	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf1, &slog.HandlerOptions{Level: slog.LevelDebug})))

	log1 := Logger("lazy-ext")
	log1.Info("first")

	require.Contains(t, buf1.String(), "first")

	// Swap the default logger
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelDebug})))

	// A fresh Logger() call picks up the new default
	log2 := Logger("lazy-ext")
	log2.Info("second")

	require.Contains(t, buf2.String(), "second")
	assert.NotContains(t, buf1.String(), "second")
}

func TestLoggerReturnsNilSafe(t *testing.T) {
	orig := slog.Default()
	defer slog.SetDefault(orig)

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	log := Logger("safe-ext")
	assert.NotNil(t, log)
}
