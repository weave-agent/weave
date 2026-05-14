package sdk

import "log/slog"

// Logger returns the default slog logger with an "ext" attribute set to the
// given extension name. Extensions should call Logger("myext") once and reuse
// the returned logger for all diagnostic output.
func Logger(name string) *slog.Logger {
	return slog.Default().With("ext", name)
}
