package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nniel-ape/gonfig"
)

type CoreConfig struct {
	AgentLoop string   `default:"loop" description:"Agent loop extension name"`
	Providers []string `default:"anthropic" description:"Provider extension names"`
}

type File struct {
	Extensions []string   `short:"e" description:"List of optional extensions to load"`
	Prompt     string     `short:"p" description:"Prompt to pass to the agent"`
	Core       CoreConfig `description:"Core agent configuration"`
}

// Core returns (coreExts, optionalExts) where coreExts contains the agent-loop
// and provider names, and optionalExts contains the user-specified extensions.
func (f *File) CoreExts() ([]string, []string) {
	core := []string{f.Core.AgentLoop}
	core = append(core, f.Core.Providers...)
	return core, f.Extensions
}

func FindConfigPath(startDir string) (string, error) {
	dir := startDir

	for {
		for _, name := range []string{".weave.yaml", ".weave/config.yaml"} {
			candidate := filepath.Join(dir, name)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no .weave.yaml or .weave/config.yaml found")
		}

		dir = parent
	}
}

func Load(args []string) (string, *File, []string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", nil, nil, fmt.Errorf("get working dir: %w", err)
	}

	return LoadFromDir(cwd, args)
}

// parseConfigFlag extracts -c/--config from args, returning the config path
// and remaining args.
func parseConfigFlag(args []string) (configPath string, rest []string) {
	for i := range args {
		if args[i] == "-c" || args[i] == "--config" {
			if i+1 < len(args) {
				return args[i+1], append(args[:i:i], args[i+2:]...)
			}
		} else if cfg, ok := strings.CutPrefix(args[i], "-c="); ok {
			return cfg, append(args[:i:i], args[i+1:]...)
		} else if cfg, ok := strings.CutPrefix(args[i], "--config="); ok {
			return cfg, append(args[:i:i], args[i+1:]...)
		}
	}

	return "", args
}

func LoadFromDir(dir string, args []string) (string, *File, []string, error) {
	configPath, args := parseConfigFlag(args)

	path := configPath

	if path == "" {
		var err error

		path, err = FindConfigPath(dir)
		if err != nil {
			return "", nil, nil, err
		}
	} else if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}

	var (
		f    File
		rest []string
	)

	if err := gonfig.Load(&f,
		gonfig.WithFile(path),
		gonfig.WithEnvPrefix("WEAVE"),
		gonfig.WithFlags(args),
		gonfig.WithRemainingArgs(&rest),
	); err != nil {
		return "", nil, nil, fmt.Errorf("load config: %w", err)
	}

	return path, &f, rest, nil
}
