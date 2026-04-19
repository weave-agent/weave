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

var (
	errAgentLoopRequired = errors.New("core.agent_loop is required")
	errProviderRequired  = errors.New("core.providers must include at least one provider")
	errDuplicateProvider = errors.New("core.providers contains duplicates")
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

	if ve := validateCoreConfig(cf); ve != nil {
		fmt.Fprintf(os.Stderr, "weave: %v\n", ve)
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

	coreExts, optExts := cf.CoreExts()
	allExts := mergeUnique(append(coreExts, optExts...))

	if cf.Prompt != "" {
		rest = append([]string{"--weave-prompt=" + cf.Prompt}, rest...)
	}

	if err := l.Run(context.Background(), projectDir, allExts, rest, configFile); err != nil {
		fmt.Fprintf(os.Stderr, "weave: %v\n", err)
		return 1
	}

	return 0
}

func mergeUnique(exts []string) []string {
	seen := make(map[string]bool, len(exts))
	result := make([]string, 0, len(exts))

	for _, e := range exts {
		if !seen[e] {
			seen[e] = true
			result = append(result, e)
		}
	}

	return result
}

func validateCoreConfig(cf *config.File) error {
	if cf.Core.AgentLoop == "" {
		return errAgentLoopRequired
	}

	if len(cf.Core.Providers) == 0 {
		return errProviderRequired
	}

	seen := make(map[string]bool, len(cf.Core.Providers))
	for _, p := range cf.Core.Providers {
		if seen[p] {
			return fmt.Errorf("%w: %q", errDuplicateProvider, p)
		}

		seen[p] = true
	}

	return nil
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
