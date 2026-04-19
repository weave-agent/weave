package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	if filepath.Base(projectDir) == ".weave" {
		projectDir = filepath.Dir(projectDir)
	}

	if err := l.Run(context.Background(), projectDir, cf.Extensions, rest); err != nil {
		fmt.Fprintf(os.Stderr, "weave: %v\n", err)
		return 1
	}

	return 0
}

func findModuleRoot() (string, error) {
	// Try walking up from the executable first (works when binary is in the repo tree).
	if root, err := findModuleRootFrom(os.Executable); err == nil {
		return root, nil
	}

	// Fallback: walk up from the working directory (works for go run / go install).
	if root, err := findModuleRootFrom(os.Getwd); err == nil {
		return root, nil
	}

	return "", errors.New("cannot find module root: go.mod not found walking up from executable or working directory")
}

func findModuleRootFrom(startFn func() (string, error)) (string, error) {
	dir, err := startFn()
	if err != nil {
		return "", err
	}

	for {
		if isWeaveModule(dir) {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	return "", errors.New("go.mod not found")
}

func isWeaveModule(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		if name, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(name) == "weave"
		}
	}

	return false
}
