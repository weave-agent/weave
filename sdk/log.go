package sdk

import "log/slog"

// Logger returns the default slog logger with an "ext" attribute set to the
// given extension name. Extensions should call Logger("myext") once and reuse
// the returned logger for all diagnostic output.
//
// Standard: extensions must use slog (via Logger or slog.Default) for all
// diagnostic output. Direct writes to os.Stderr or os.Stdout (log.Printf,
// fmt.Fprintf(os.Stderr, ...)) corrupt the Bubble Tea TUI display. The
// framework configures slog.Default() with a file handler (and stderr mirror
// in headless mode); Logger simply wraps that default.
func Logger(name string) *slog.Logger {
	return slog.Default().With("ext", name)
}
