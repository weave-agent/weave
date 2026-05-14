package main

import (
	"context"
	"os"

	"weave/internal/wire"
)

func main() {
	os.Exit(wire.Run(context.Background(), os.Args[1:]))
}
