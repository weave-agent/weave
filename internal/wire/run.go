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

const (
	cacheCommand    = "cache"
	helpFlagLong    = "--help"
	helpFlagShort   = "-h"
	promptFlagShort = "-p"
)

type runDeps struct {
	runBootstrap func(context.Context, *settings.Settings)
	runLauncher  func(context.Context, string, string, string, string, []string, string, string, bool, []string) error
}

func defaultRunDeps() runDeps {
	return runDeps{
		runBootstrap: runBootstrap,
		runLauncher:  runLauncher,
	}
}

// Run is the main entry point for the weave CLI. It parses args, loads config,
// discovers extensions, and runs the launcher pipeline.
// revision is the binary version set via ldflags (e.g. "v0.0.1-abc0123-2026-05-17"),
// or "unknown" for dev builds.
func Run(ctx context.Context, args []string, revision string) int {
	if code, ok := handleSubcommand(args); ok {
		return code
	}

	return run(ctx, args, revision)
}

func handleSubcommand(args []string) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}

	switch args[0] {
	case cacheCommand:
		return runCacheSubcommand(args[1:]), true
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

func runCacheSubcommand(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "weave cache: missing command")
		fmt.Fprintln(os.Stderr, "usage: weave cache clean")

		return 1
	}

	if args[0] != "clean" {
		fmt.Fprintf(os.Stderr, "weave cache: unknown command %q\n", args[0])
		fmt.Fprintln(os.Stderr, "usage: weave cache clean")

		return 1
	}

	if len(args) > 1 {
		fmt.Fprintln(os.Stderr, "weave cache clean: too many arguments")
		fmt.Fprintln(os.Stderr, "usage: weave cache clean")

		return 1
	}

	cacheDir, err := launcher.DefaultCacheDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave cache clean: %v\n", err)

		return 1
	}

	removed, err := launcher.NewCache(cacheDir).Clean()
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave cache clean: %v\n", err)

		return 1
	}

	_, _ = fmt.Fprintf(os.Stdout, "removed %d launcher cache entries\n", removed)

	return 0
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

func buildLauncherWithDeps(ctx context.Context, cf *settings.Settings, rest []string, configFile, revision string, deps runDeps) int {
	if hasHelpFlag(rest) {
		fmt.Fprint(os.Stderr, settings.GenerateFullHelp())

		return 0
	}

	if cf.Prompt == "" && (cf.UIExtension == "" || cf.UIExtension == "none") {
		fmt.Fprintf(os.Stderr, "weave: %v\n", errNoInput)

		return 1
	}

	if deps.runBootstrap == nil {
		deps.runBootstrap = runBootstrap
	}

	if deps.runLauncher == nil {
		deps.runLauncher = runLauncher
	}

	deps.runBootstrap(ctx, cf)

	cacheDir, err := launcher.DefaultCacheDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave: %v\n", err)
		return 1
	}

	moduleRoot, err := findModuleRoot(revision)
	if err != nil {
		fmt.Fprintf(os.Stderr, "weave: %v\n", err)
		return 1
	}

	// In release mode (no source tree), extract the semver tag for go.mod require.
	var moduleVersion string
	if moduleRoot == "" {
		moduleVersion = tagFromRevision(revision)
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

	if err := deps.runLauncher(ctx, cacheDir, moduleRoot, moduleVersion, projectDir, rest, configFile, cf.AgentLoop, headless, cf.ExcludeExtensions); err != nil {
		fmt.Fprintf(os.Stderr, "weave: %v\n", err)
		return 1
	}

	return 0
}

func runLauncher(
	ctx context.Context,
	cacheDir, moduleRoot, moduleVersion, projectDir string,
	args []string,
	configFile, agentLoop string,
	headless bool,
	exclude []string,
) error {
	cache := launcher.NewCache(cacheDir)
	l := launcher.NewLauncher(cache, moduleRoot, moduleVersion)

	if err := l.Run(ctx, projectDir, args, configFile, agentLoop, headless, exclude); err != nil {
		return fmt.Errorf("run launcher: %w", err)
	}

	return nil
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

func run(ctx context.Context, args []string, revision string) (exitCode int) {
	return runWithDeps(ctx, args, revision, defaultRunDeps())
}

func runWithDeps(ctx context.Context, args []string, revision string, deps runDeps) (exitCode int) {
	if hasHelpFlag(args) {
		fmt.Fprint(os.Stderr, settings.GenerateFullHelp())

		return 0
	}

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

	return buildLauncherWithDeps(ctx, cf, rest, configFile, revision, deps)
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

func findModuleRoot(revision string) (string, error) {
	if root, err := findModuleRootFrom(os.Executable); err == nil {
		return root, nil
	}

	if root, err := findModuleRootFrom(os.Getwd); err == nil {
		return root, nil
	}

	// Pre-built binary (e.g. Homebrew) — no local source tree. The launcher
	// will use the Go module proxy to resolve the published version instead
	// of a local replace directive.
	if revision != "unknown" {
		return "", nil
	}

	return "", errors.New("cannot find module root: go.mod not found walking up from executable or working directory")
}

// tagFromRevision extracts the semver tag from a goreleaser revision string.
// Revision format: <tag>-<shortCommit>-<commitDate> (e.g. "v0.0.1-abc0123-2026-05-17").
// The shortCommit is 7+ hex chars, so we split on the first hex-only segment.
func tagFromRevision(rev string) string {
	parts := strings.Split(rev, "-")

	for i, p := range parts {
		if isHexString(p) && len(p) >= 7 {
			return strings.Join(parts[:i], "-")
		}
	}

	return rev
}

func isHexString(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}

	return s != ""
}

var helpValueFlags = map[string]struct{}{
	"--config":             {},
	"--model":              {},
	"--output":             {},
	"--prompt":             {},
	"--resume":             {},
	"--sandbox":            {},
	"--subagent-id":        {},
	"--tools":              {},
	"--ui":                 {},
	"--weave-agent-loop":   {},
	"--weave-config":       {},
	"--weave-headless":     {},
	"--weave-model":        {},
	"--weave-output":       {},
	"--weave-project-dir":  {},
	"--weave-prompt-file":  {},
	"--weave-resume":       {},
	"--weave-sandbox-mode": {},
	"--weave-subagent-id":  {},
	"--weave-tools":        {},
	promptFlagShort:        {},
	"-r":                   {},
}

// hasHelpFlag reports whether args contains a real --help or -h flag, respecting
// -- and known flags whose next argument is a value.
func hasHelpFlag(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return false
		}

		flagName := arg
		if before, _, ok := strings.Cut(arg, "="); ok {
			flagName = before
		}

		if (flagName == helpFlagLong || flagName == helpFlagShort) && flagName == arg {
			return true
		}

		if _, takesValue := helpValueFlags[flagName]; takesValue && flagName == arg && i+1 < len(args) {
			i++
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
