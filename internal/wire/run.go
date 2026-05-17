package wire

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/weave-agent/weave/internal/extmanage"
	"github.com/weave-agent/weave/internal/launcher"
	"github.com/weave-agent/weave/settings"
)

var errNoInput = errors.New("no prompt provided and ui is disabled — use -p to provide a prompt or set ui: tui")

// Run is the main entry point for the weave CLI. It parses args, loads config,
// discovers extensions, and runs the launcher pipeline.
func Run(ctx context.Context, args []string) int {
	if code, ok := handleSubcommand(args); ok {
		return code
	}

	return run(ctx, args...)
}

func handleSubcommand(args []string) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}

	switch args[0] {
	case "install":
		return extmanage.RunInstall(args[1:]), true
	case "list":
		return extmanage.RunList(args[1:]), true
	case "update":
		return extmanage.RunUpdate(args[1:]), true
	case "uninstall":
		return extmanage.RunUninstall(args[1:]), true
	}

	return 0, false
}

func loadConfig(args []string) (configFile string, cf *settings.Settings, rest []string, err error) {
	var promptFile string

	promptFile, args = parseWeavePromptFileFlag(args)
	projectDirOverride, args := parseWeaveProjectDirFlag(args)

	configFile, cf, rest, err = settings.Load(args)
	if err != nil {
		return "", nil, nil, fmt.Errorf("load config: %w", err)
	}

	if promptFile != "" {
		prompt, readErr := os.ReadFile(promptFile)
		if readErr != nil {
			return "", nil, nil, fmt.Errorf("read prompt file: %w", readErr)
		}

		cf.Prompt = string(prompt)

		rest = append([]string{"--weave-prompt-file=" + promptFile}, rest...)
	}

	if projectDirOverride != "" {
		rest = append([]string{"--weave-project-dir=" + projectDirOverride}, rest...)
	}

	return configFile, cf, rest, nil
}

func parseWeavePromptFileFlag(args []string) (string, []string) {
	for i := range args {
		if args[i] == "--weave-prompt-file" {
			if i+1 < len(args) {
				return args[i+1], append(args[:i:i], args[i+2:]...)
			}

			return "", append(args[:i:i], args[i+1:]...)
		}

		if promptFile, ok := strings.CutPrefix(args[i], "--weave-prompt-file="); ok {
			return promptFile, append(args[:i:i], args[i+1:]...)
		}
	}

	return "", args
}

func parseWeaveProjectDirFlag(args []string) (string, []string) {
	for i := range args {
		if args[i] == "--weave-project-dir" {
			if i+1 < len(args) {
				return args[i+1], append(args[:i:i], args[i+2:]...)
			}

			return "", append(args[:i:i], args[i+1:]...)
		}

		if projectDir, ok := strings.CutPrefix(args[i], "--weave-project-dir="); ok {
			return projectDir, append(args[:i:i], args[i+1:]...)
		}
	}

	return "", args
}

// runBootstrap installs core extensions on first run when ~/.weave/extensions/
// is empty. Silently skips if --skip-bootstrap is set or extensions already exist.
func runBootstrap(ctx context.Context, cf *settings.Settings) {
	if cf.SkipBootstrap {
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}

	needsBootstrap, err := extmanage.NeedsBootstrap(homeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave: bootstrap check: %v\n", err)

		return
	}

	if !needsBootstrap {
		return
	}

	if _, err = extmanage.BootstrapCoreExtensions(ctx, homeDir, func(msg string) {
		fmt.Fprintln(os.Stderr, msg)
	}); err != nil {
		fmt.Fprintf(os.Stderr, "weave: bootstrap: %v\n", err)
	}
}

func buildLauncher(ctx context.Context, cf *settings.Settings, rest []string, configFile string) int {
	runBootstrap(ctx, cf)

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

	projectDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave: %v\n", err)
		return 1
	}

	// When a project-local config was found (not global fallback), derive the
	// project directory from the config file path. This ensures auto-discovery
	// scans the correct .weave/extensions/ directory when running from a subdir.
	if configFile != "" {
		globalDir, _ := settings.GlobalConfigDir()
		if globalDir == "" || !strings.HasPrefix(configFile, globalDir+string(os.PathSeparator)) {
			projectDir = settings.ProjectDirFromConfig(configFile)
		}
	}

	if override := weaveProjectDirFromRest(rest); override != "" {
		projectDir = override
	}

	if cf.Prompt == "" && (cf.UIExtension == "" || cf.UIExtension == "none") && !hasHelpFlag(rest) {
		fmt.Fprintf(os.Stderr, "weave: %v\n", errNoInput)
		return 1
	}

	headless := cf.Prompt != ""

	if cf.Prompt != "" && !hasWeavePromptFileFlag(rest) {
		promptFile, cleanup, ok := writePromptFile(cf.Prompt)
		if !ok {
			return 1
		}

		defer cleanup()

		rest = append([]string{"--weave-prompt-file=" + promptFile}, rest...)
	}

	// Forward CLI flags to the generated binary.
	rest = append(rest, cf.WeaveFlags()...)

	cache := launcher.NewCache(cacheDir)
	l := launcher.NewLauncher(cache, moduleRoot)

	if err := l.Run(ctx, projectDir, rest, configFile, cf.AgentLoop, headless, cf.ExcludeExtensions); err != nil {
		fmt.Fprintf(os.Stderr, "weave: %v\n", err)
		return 1
	}

	return 0
}

func hasWeavePromptFileFlag(args []string) bool {
	for _, a := range args {
		if a == "--weave-prompt-file" || strings.HasPrefix(a, "--weave-prompt-file=") {
			return true
		}
	}

	return false
}

func weaveProjectDirFromRest(args []string) string {
	for i, a := range args {
		if a == "--weave-project-dir" && i+1 < len(args) {
			return args[i+1]
		}

		if projectDir, ok := strings.CutPrefix(a, "--weave-project-dir="); ok {
			return projectDir
		}
	}

	return ""
}

func run(ctx context.Context, args ...string) (exitCode int) {
	configFile, cf, rest, err := loadConfig(args)
	if err != nil {
		var helpErr *settings.HelpError

		if errors.As(err, &helpErr) {
			fmt.Fprint(os.Stderr, helpErr.Text)

			return 0
		}

		fmt.Fprintf(os.Stderr, "weave: %v\n", err)

		return 1
	}

	return buildLauncher(ctx, cf, rest, configFile)
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

// hasHelpFlag reports whether args contains --help or -h.
func hasHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return true
		}
	}

	return false
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
			return strings.TrimSpace(name) == "github.com/weave-agent/weave"
		}
	}

	return false
}
