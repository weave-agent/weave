package main

import (
	"context"
	"os"

	"github.com/weave-agent/weave/internal/wire"
)

func main() {
	os.Exit(wire.Run(context.Background(), os.Args[1:]))
}
