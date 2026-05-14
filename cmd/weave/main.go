package main

import (
	"context"
	"os"
	"strings"

	"weave/internal/wire"
)

const debugFlag = "--weave-debug=true"

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
		if a == "--debug" {
			args = append(args[:i], args[i+1:]...)

			return append([]string{debugFlag}, args...)
		}

		if val, ok := strings.CutPrefix(a, "--debug="); ok {
			args = append(args[:i], args[i+1:]...)

			if val == "true" || val == "1" {
				return append([]string{debugFlag}, args...)
			}

			return append([]string{"--weave-debug=false"}, args...)
		}
	}

	return args
}
