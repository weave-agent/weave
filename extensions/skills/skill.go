package skills

//go:generate moq -fmt goimports -skip-ensure -pkg skills -out mock_test.go ../../sdk Bus UI

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Skill struct {
	Name                   string
	Description            string
	FilePath               string
	BaseDir                string
	DisableModelInvocation bool
	License                string
	Compatibility          string
	Metadata               map[string]any
	AllowedTools           []string
	body                   string
}

type SkillFrontmatter struct {
	Name                   string         `yaml:"name"`
	Description            string         `yaml:"description"`
	DisableModelInvocation bool           `yaml:"disable-model-invocation"`
	License                string         `yaml:"license"`
	Compatibility          string         `yaml:"compatibility"`
	Metadata               map[string]any `yaml:"metadata"`
	AllowedTools           string         `yaml:"allowed-tools"`
}

var (
	nameRegex     = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	reservedNames = []string{"anthropic", "claude"}
)

func validateName(name string) error {
	if len(name) < 1 || len(name) > 64 {
		return errors.New("skill name must be 1-64 characters")
	}

	if !nameRegex.MatchString(name) {
		return errors.New("skill name must contain only lowercase a-z, 0-9, and hyphens (no leading/trailing/consecutive hyphens)")
	}

	lower := strings.ToLower(name)
	for _, reserved := range reservedNames {
		if strings.Contains(lower, reserved) {
			return fmt.Errorf("skill name cannot contain %q", reserved)
		}
	}

	return nil
}

func loadSkillFromDir(dir string) (Skill, error) {
	skillPath := filepath.Join(dir, "SKILL.md")

	data, err := os.ReadFile(skillPath)
	if err != nil {
		return Skill{}, fmt.Errorf("reading SKILL.md: %w", err)
	}

	body, fm, err := parseFrontmatter(data)
	if err != nil {
		return Skill{}, fmt.Errorf("parsing frontmatter: %w", err)
	}

	if fm.Name == "" {
		return Skill{}, errors.New("skill name is required in frontmatter")
	}

	if err := validateName(fm.Name); err != nil {
		return Skill{}, fmt.Errorf("invalid skill name %q: %w", fm.Name, err)
	}

	expectedDir := filepath.Base(dir)
	if fm.Name != expectedDir {
		return Skill{}, fmt.Errorf("skill name %q does not match directory name %q", fm.Name, expectedDir)
	}

	if fm.Description == "" {
		return Skill{}, errors.New("skill description is required in frontmatter")
	}

	var tools []string
	if fm.AllowedTools != "" {
		tools = strings.Fields(fm.AllowedTools)
	}

	return Skill{
		Name:                   fm.Name,
		Description:            fm.Description,
		FilePath:               skillPath,
		BaseDir:                dir,
		DisableModelInvocation: fm.DisableModelInvocation,
		License:                fm.License,
		Compatibility:          fm.Compatibility,
		Metadata:               fm.Metadata,
		AllowedTools:           tools,
		body:                   body,
	}, nil
}

func parseFrontmatter(data []byte) (string, SkillFrontmatter, error) {
	content := strings.ReplaceAll(string(data), "\r\n", "\n")

	var fm SkillFrontmatter

	if !strings.HasPrefix(content, "---\n") {
		return "", fm, errors.New("SKILL.md must start with YAML frontmatter (---)")
	}

	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return "", fm, errors.New("SKILL.md frontmatter not closed (missing ---)")
	}

	fmText := content[4 : 4+end]
	if err := yaml.Unmarshal([]byte(fmText), &fm); err != nil {
		return "", fm, fmt.Errorf("invalid YAML: %w", err)
	}

	body := content[4+end+5:]
	body = strings.TrimLeft(body, "\n")

	return body, fm, nil
}

func (s Skill) Body() string {
	return s.body
}
