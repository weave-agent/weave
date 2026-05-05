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

var errNoInput = errors.New("no prompt provided and ui is disabled — use -p to provide a prompt or set ui: tui")

func main() {
	os.Exit(dispatch(os.Args[1:]))
}

// dispatch routes to subcommands or the default run path.
func dispatch(args []string) int {
	if len(args) > 0 && args[0] == "install" {
		return runInstall(args[1:])
	}

	return run(args...)
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

	moduleRoot, err := findModuleRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave: %v\n", err)
		return 1
	}

	projectDir := resolveProjectDir(configFile)

	allExts, providers, rest, ok := resolveExtensionsAndMode(cf, rest)
	if !ok {
		return 1
	}

	if cf.Prompt != "" {
		promptFile, cleanup, ok := writePromptFile(cf.Prompt)
		if !ok {
			return 1
		}

		defer cleanup()

		rest = append([]string{"--weave-prompt-file=" + promptFile}, rest...)
	}

	cache := launcher.NewCache(cacheDir)
	l := launcher.NewLauncher(cache, moduleRoot)

	if err := l.Run(context.Background(), projectDir, allExts, rest, configFile, cf.Core.AgentLoop, providers); err != nil {
		fmt.Fprintf(os.Stderr, "weave: %v\n", err)
		return 1
	}

	return 0
}

func resolveProjectDir(configFile string) string {
	dir := filepath.Dir(configFile)
	if filepath.Base(dir) == ".weave" {
		dir = filepath.Dir(dir)
	}

	return dir
}

func resolveExtensionsAndMode(cf *config.File, rest []string) (allExts, providers, updatedRest []string, ok bool) {
	envProvider := os.Getenv("WEAVE_PROVIDER")

	// If WEAVE_PROVIDER was synthesized by a previous launcher invocation
	// (e.g. via /reload), ignore it so config changes take effect. The marker
	// is only set when this launcher synthesizes the value below.
	if os.Getenv("WEAVE_PROVIDER_AUTO") == "1" {
		envProvider = ""
		_ = os.Unsetenv("WEAVE_PROVIDER")
		_ = os.Unsetenv("WEAVE_PROVIDER_AUTO")
	}

	if envProvider == "" && len(cf.Core.Providers) > 0 {
		if err := os.Setenv("WEAVE_PROVIDER", cf.Core.Providers[0]); err != nil {
			fmt.Fprintf(os.Stderr, "weave: setenv: %v\n", err)
			return nil, nil, nil, false
		}

		if err := os.Setenv("WEAVE_PROVIDER_AUTO", "1"); err != nil {
			fmt.Fprintf(os.Stderr, "weave: setenv: %v\n", err)
			return nil, nil, nil, false
		}
	}

	allExts = cf.AllExtensions()
	allExts = ensurePresent(allExts, "skills")

	if cf.Prompt == "" && (cf.UI == "" || cf.UI == config.UIValueNone) {
		fmt.Fprintf(os.Stderr, "weave: %v\n", errNoInput)
		return nil, nil, nil, false
	}

	headless := cf.Prompt != ""
	if !headless {
		allExts = ensurePresent(allExts, cf.UI)
	}

	updatedRest = rest
	if headless {
		updatedRest = append([]string{"--weave-headless=true"}, updatedRest...)
	} else {
		updatedRest = append([]string{"--weave-headless=false"}, updatedRest...)
	}

	providers = cf.Core.Providers
	if envProvider != "" {
		providers = ensurePresent(providers, envProvider)
		allExts = ensurePresent(allExts, envProvider)
	}

	return allExts, providers, updatedRest, true
}

func writePromptFile(prompt string) (path string, cleanup func(), ok bool) {
	f, err := os.CreateTemp("", "weave-prompt-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave: creating prompt file: %v\n", err)
		return "", nil, false
	}

	if _, err := f.WriteString(prompt); err != nil {
		_ = f.Close()

		fmt.Fprintf(os.Stderr, "weave: writing prompt file: %v\n", err)

		return "", nil, false
	}

	promptFile := f.Name()
	_ = f.Close()

	return promptFile, func() { _ = os.Remove(promptFile) }, true
}

func ensurePresent(exts []string, name string) []string {
	if slices.Contains(exts, name) {
		return exts
	}

	return append(exts, name)
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
