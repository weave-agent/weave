package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nniel-ape/gonfig"
)

type File struct {
	Extensions []string          `description:"List of extensions to load"`
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
			return "", fmt.Errorf("no .weave.yaml or .weave/config.yaml found")
		}
		dir = parent
	}
}

func Load(path string) (*File, error) {
	var f File
	if err := gonfig.Load(&f,
		gonfig.WithFile(path),
		gonfig.WithEnvPrefix("WEAVE"),
	); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if f.Slots == nil {
		f.Slots = make(map[string]string)
	}
	return &f, nil
}
