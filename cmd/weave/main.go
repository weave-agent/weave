package main

import (
	"context"
	"fmt"
	"os"

	"github.com/weave-agent/weave/internal/wire"
)

var revision = "unknown"

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Println("weave " + revision)
			os.Exit(0)
		}
	}

	os.Exit(wire.Run(context.Background(), os.Args[1:]))
}
