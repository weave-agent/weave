package main

import (
	"context"
	"os"
	"strings"

	"weave/internal/wire"
)

const (
	debugFlag      = "--weave-debug=true"
	debugFlagFalse = "--weave-debug=false"
	debugArg       = "--debug"
	debugArgPrefix = "--debug="
)

func main() {
	args := prependDebugFlag(os.Args[1:])
	os.Exit(wire.Run(context.Background(), args))
}

// prependDebugFlag checks WEAVE_DEBUG env var and prepends --weave-debug=true
// to args when set. The generated binary also checks WEAVE_DEBUG, but doing it
// here ensures the flag is forwarded through the launcher pipeline.
func prependDebugFlag(args []string) []string {
	if os.Getenv("WEAVE_DEBUG") == "1" || os.Getenv("WEAVE_DEBUG") == "true" {
		return append([]string{debugFlag}, args...)
	}

	// Also check for --debug in args and translate to --weave-debug.
	for i, a := range args {
		if a == debugArg {
			args = append(args[:i], args[i+1:]...)

			// Check if next arg is a boolean value (space-separated form).
			if i < len(args) {
				switch args[i] {
				case "true", "1":
					args = append(args[:i], args[i+1:]...)
					return append([]string{debugFlag}, args...)
				case "false", "0":
					args = append(args[:i], args[i+1:]...)
					return append([]string{debugFlagFalse}, args...)
				}
			}

			return append([]string{debugFlag}, args...)
		}

		if val, ok := strings.CutPrefix(a, debugArgPrefix); ok {
			args = append(args[:i], args[i+1:]...)

			if val == "true" || val == "1" {
				return append([]string{debugFlag}, args...)
			}

			return append([]string{debugFlagFalse}, args...)
		}
	}

	return args
}
