package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nniel-ape/gonfig"
)

type File struct {
	Extensions []string          `short:"e" description:"List of extensions to load"`
	Prompt     string            `short:"p" description:"Prompt to pass to the agent"`
	Slots      map[string]string `description:"Extension slot assignments"`
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

	path, err := FindConfigPath(cwd)
	if err != nil {
		return "", nil, nil, err
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

	if f.Slots == nil {
		f.Slots = make(map[string]string)
	}

	return path, &f, rest, nil
}

func LoadFromDir(dir string, args []string) (string, *File, []string, error) {
	path, err := FindConfigPath(dir)
	if err != nil {
		return "", nil, nil, err
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

	if f.Slots == nil {
		f.Slots = make(map[string]string)
	}

	return path, &f, rest, nil
}
