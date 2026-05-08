package wire

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

var errNoInput = errors.New("no prompt provided and ui is disabled — use -p to provide a prompt or set ui: tui")

// Run is the main entry point for the weave CLI. It parses args, loads config,
// discovers extensions, and runs the launcher pipeline.
func Run(ctx context.Context, args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "install":
			return runInstall(args[1:])
		case "list":
			return runList(args[1:])
		case "update":
			return runUpdate(args[1:])
		case "uninstall":
			return runUninstall(args[1:])
		}
	}

	return run(ctx, args...)
}

func run(ctx context.Context, args ...string) (exitCode int) {
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

	projectDir := config.ProjectDirFromConfig(configFile)

	if cf.Prompt == "" && (cf.UI == "" || cf.UI == config.UIValueNone) {
		fmt.Fprintf(os.Stderr, "weave: %v\n", errNoInput)
		return 1
	}

	headless := cf.Prompt != ""

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

	if err := l.Run(ctx, projectDir, rest, configFile, cf.Core.AgentLoop, headless, cf.ExcludeExtensions); err != nil {
		fmt.Fprintf(os.Stderr, "weave: %v\n", err)
		return 1
	}

	return 0
}

func writePromptFile(prompt string) (path string, cleanup func(), ok bool) {
	f, err := os.CreateTemp("", "weave-prompt-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave: creating prompt file: %v\n", err)
		return "", nil, false
	}

	if _, err := f.WriteString(prompt); err != nil {
		promptFile := f.Name()
		_ = f.Close()
		_ = os.Remove(promptFile)

		fmt.Fprintf(os.Stderr, "weave: writing prompt file: %v\n", err)

		return "", nil, false
	}

	promptFile := f.Name()
	_ = f.Close()

	return promptFile, func() { _ = os.Remove(promptFile) }, true
}

func findModuleRoot() (string, error) {
	if root, err := findModuleRootFrom(os.Executable); err == nil {
		return root, nil
	}

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
