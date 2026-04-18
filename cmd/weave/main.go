package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"weave/config"
	"weave/launcher"
)

func main() {
	os.Exit(run(os.Args[1:]...))
}

func run(args ...string) (exitCode int) {
	configFile, cf, rest, err := config.Load(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave: %v\n", err)
		return 1
	}

	cacheDir, err := launcher.DefaultCacheDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave: %v\n", err)
		return 1
	}

	cache := launcher.NewCache(cacheDir)
	moduleRoot, err := findModuleRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave: %v\n", err)
		return 1
	}

	l := launcher.NewLauncher(cache, moduleRoot)

	projectDir := filepath.Dir(configFile)
	if err := l.Run(context.Background(), projectDir, cf.Extensions, rest); err != nil {
		fmt.Fprintf(os.Stderr, "weave: %v\n", err)
		return 1
	}

	return 0
}

func findModuleRoot() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(exe)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot find module root")
	}
	return cwd, nil
}
