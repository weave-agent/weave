package log

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	setupOnce   sync.Once
	initialized atomic.Bool
)

// Setup configures slog.Default() with a JSON handler writing to a rotating
// log file via lumberjack. Optional extra writers are combined with the file
// output. The default log level is Info; debug sets it to Debug.
// Setup is safe to call multiple times — only the first call has effect.
func Setup(logFile string, debug bool, extraWriters ...io.Writer) {
	setupOnce.Do(func() {
		dir := filepath.Dir(logFile)
		if dir != "" && dir != "." {
			_ = os.MkdirAll(dir, 0o755)
		}

		lj := &lumberjack.Logger{
			Filename: logFile,
			MaxSize:  10,
			MaxAge:   30,
			Compress: false,
		}

		writers := []io.Writer{lj}
		writers = append(writers, extraWriters...)
		w := io.MultiWriter(writers...)

		level := slog.LevelInfo
		if debug {
			level = slog.LevelDebug
		}

		handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
		slog.SetDefault(slog.New(handler))
		initialized.Store(true)
	})
}

// Initialized returns true if Setup has been called successfully.
func Initialized() bool {
	return initialized.Load()
}
