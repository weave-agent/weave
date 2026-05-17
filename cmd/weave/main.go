package main

import (
	"context"
	"os"

	"github.com/weave-agent/weave/internal/wire"
)

var revision = "unknown"

//nolint:unused
func getRevision() string { return revision }

func main() {
	os.Exit(wire.Run(context.Background(), os.Args[1:]))
}
