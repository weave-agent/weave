package cfg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"weave/sdk"

	"gopkg.in/yaml.v3"
)

type ConfigFile struct {
	Extensions []string          `yaml:"extensions"`
	Slots      map[string]string `yaml:"slots"`
}

type config struct {
	data map[string]any
}

var _ sdk.Config = (*config)(nil)

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

func Load(path string) (*ConfigFile, sdk.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read config: %w", err)
	}

	var cf ConfigFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, nil, fmt.Errorf("parse config: %w", err)
	}
	if cf.Slots == nil {
		cf.Slots = make(map[string]string)
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("parse config raw: %w", err)
	}

	return &cf, &config{data: raw}, nil
}

func (c *config) GetString(key string) string {
	v := c.resolve(key)
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func (c *config) GetInt(key string) int {
	v := c.resolve(key)
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func (c *config) GetBool(key string) bool {
	v := c.resolve(key)
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func (c *config) GetStringSlice(key string) []string {
	v := c.resolve(key)
	if v == nil {
		return nil
	}
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		result := make([]string, len(s))
		for i, item := range s {
			result[i] = fmt.Sprintf("%v", item)
		}
		return result
	}
	return nil
}

func (c *config) Sub(key string) sdk.Config {
	v := c.resolve(key)
	if v == nil {
		return &config{data: nil}
	}
	if m, ok := v.(map[string]any); ok {
		return &config{data: m}
	}
	return &config{data: nil}
}

func (c *config) resolve(key string) any {
	if c.data == nil {
		return nil
	}
	parts := strings.Split(key, ".")
	var current any = c.data
	for _, p := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = m[p]
		if !ok {
			return nil
		}
	}
	return current
}
