package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"weave/cfg"
	"weave/launcher"
)

func main() {
	os.Exit(run(os.Args[1:]...))
}

func run(args ...string) (exitCode int) {
	fs := flag.NewFlagSet("weave", flag.ContinueOnError)
	configPath := fs.String("c", "", "path to config file")
	extOverride := fs.String("e", "", "comma-separated extension override")
	prompt := fs.String("p", "", "prompt to pass to the agent")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: weave [flags] [args...]\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return 2
	}

	rest := fs.Args()

	_ = prompt

	configFile, err := resolveConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave: %v\n", err)
		return 1
	}

	cf, _, err := cfg.Load(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave: %v\n", err)
		return 1
	}

	exts := cf.Extensions
	if *extOverride != "" {
		exts = parseCommaList(*extOverride)
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
	if err := l.Run(context.Background(), projectDir, exts, rest); err != nil {
		fmt.Fprintf(os.Stderr, "weave: %v\n", err)
		return 1
	}

	return 0
}

func resolveConfig(path string) (string, error) {
	if path != "" {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("resolve config path: %w", err)
		}
		return abs, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working dir: %w", err)
	}
	found, err := cfg.FindConfigPath(cwd)
	if err != nil {
		return "", err
	}
	return found, nil
}

func parseCommaList(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, item := range splitComma(s) {
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func splitComma(s string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			part := s[start:i]
			if part != "" {
				parts = append(parts, part)
			}
			start = i + 1
		}
	}
	return parts
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
