package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"weave/config"
	"weave/launcher"
)

var (
	errAgentLoopRequired = errors.New("core.agent_loop is required")
	errProviderRequired  = errors.New("core.providers must include at least one provider")
	errDuplicateProvider = errors.New("core.providers contains duplicates")
	errNoInput           = errors.New("no prompt provided and ui is disabled — use -p to provide a prompt or set ui: tui")
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

	envProvider := os.Getenv("WEAVE_PROVIDER")
	if envProvider == "" && len(cf.Core.Providers) > 0 {
		if err := os.Setenv("WEAVE_PROVIDER", cf.Core.Providers[0]); err != nil {
			fmt.Fprintf(os.Stderr, "weave: setenv: %v\n", err)

			return 1
		}
	}

	allExts := cf.AllExtensions()

	// Skills extension is always included — it discovers skill directories
	// and injects descriptions into the system prompt even in headless mode.
	allExts = ensurePresent(allExts, "skills")

	// Add UI extension when no prompt is provided (interactive mode).
	// When a prompt is set (-p flag), the agent runs in print mode without TUI.
	if cf.Prompt == "" && cf.UI != "" && cf.UI != "none" {
		allExts = ensurePresent(allExts, cf.UI)
	}

	// Without a prompt and without a UI, the agent has no input and will hang.
	if cf.Prompt == "" && (cf.UI == "" || cf.UI == "none") {
		fmt.Fprintf(os.Stderr, "weave: %v\n", errNoInput)
		return 1
	}

	// Compute effective providers: config providers + env override if set.
	providers := cf.Core.Providers
	if envProvider != "" {
		providers = ensurePresent(providers, envProvider)
		allExts = ensurePresent(allExts, envProvider)
	}

	if cf.Prompt != "" {
		f, err := os.CreateTemp("", "weave-prompt-*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "weave: creating prompt file: %v\n", err)

			return 1
		}

		promptFile := f.Name()
		//nolint:wsl // cleanup defer adjacent to captured var
		defer func() { _ = os.Remove(promptFile) }()

		if _, err := f.WriteString(cf.Prompt); err != nil {
			_ = f.Close()

			fmt.Fprintf(os.Stderr, "weave: writing prompt file: %v\n", err)

			return 1
		}

		_ = f.Close()

		rest = append([]string{"--weave-prompt-file=" + promptFile}, rest...)
	}

	if err := l.Run(context.Background(), projectDir, allExts, rest, configFile, cf.Core.AgentLoop, providers); err != nil {
		fmt.Fprintf(os.Stderr, "weave: %v\n", err)
		return 1
	}

	return 0
}

func ensurePresent(exts []string, name string) []string {
	if slices.Contains(exts, name) {
		return exts
	}

	return append(exts, name)
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
