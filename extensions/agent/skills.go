package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"weave/sdk"
)

// Skill represents a discovered skill with its metadata and body.
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

type skillFrontmatter struct {
	Name                   string         `yaml:"name"`
	Description            string         `yaml:"description"`
	DisableModelInvocation bool           `yaml:"disable-model-invocation"`
	License                string         `yaml:"license"`
	Compatibility          string         `yaml:"compatibility"`
	Metadata               map[string]any `yaml:"metadata"`
	AllowedTools           string         `yaml:"allowed-tools"`
}

var (
	skillNameRegex = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	reservedNames  = []string{"anthropic", "claude"}
)

func validateName(name string) error {
	if len(name) < 1 || len(name) > 64 {
		return errors.New("skill name must be 1-64 characters")
	}

	if !skillNameRegex.MatchString(name) {
		return errors.New("skill name must contain only lowercase a-z, 0-9, and hyphens (no leading/trailing/consecutive hyphens)")
	}

	lower := strings.ToLower(name)
	for _, reserved := range reservedNames {
		if lower == reserved {
			return fmt.Errorf("skill name cannot be %q", reserved)
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

func parseFrontmatter(data []byte) (string, skillFrontmatter, error) {
	content := strings.ReplaceAll(string(data), "\r\n", "\n")

	var fm skillFrontmatter

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

// Body returns the skill body (content after frontmatter).
func (s Skill) Body() string {
	return s.body
}

// discoverSkills scans the given paths for skill directories.
// Each path is a directory containing subdirectories, each of which should
// contain a SKILL.md. Skills are deduplicated by name (first path wins).
// Results are sorted by name. Invalid skills are skipped.
func discoverSkills(paths ...string) ([]Skill, error) {
	seen := make(map[string]bool)

	var skills []Skill

	for _, root := range paths {
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}

			return nil, fmt.Errorf("read skills dir %s: %w", root, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			info, err := entry.Info()
			if err != nil || info.Mode()&os.ModeSymlink != 0 {
				continue
			}

			name := entry.Name()
			if seen[name] {
				continue
			}

			dir := filepath.Join(root, name)

			skill, err := loadSkillFromDir(dir)
			if err != nil {
				continue
			}

			seen[name] = true

			skills = append(skills, skill)
		}
	}

	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})

	return skills, nil
}

// discoverExtensionSkills scans extension directories for skills/ subdirectories.
// It checks project-local .weave/extensions/ and global ~/.weave/extensions/.
// Only directories matching registered extension names are checked.
// Returns a list of <ext-dir>/skills/ paths.
func discoverExtensionSkills(projectDir, globalDir string) []string {
	var paths []string

	registered := make(map[string]bool)
	for _, name := range sdk.ListExtensions() {
		registered[name] = true
	}

	checkDir := func(extDir string) {
		entries, err := os.ReadDir(extDir)
		if err != nil {
			return
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			name := entry.Name()
			if !registered[name] {
				continue
			}

			skillsDir := filepath.Join(extDir, name, "skills")
			if _, err := os.Stat(skillsDir); err == nil {
				paths = append(paths, skillsDir)
			}
		}
	}

	if projectDir != "" {
		checkDir(filepath.Join(projectDir, ".weave", "extensions"))
	}

	if globalDir != "" {
		checkDir(filepath.Join(globalDir, "extensions"))
	}

	return paths
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")

	return s
}

// formatSkillsPrompt formats a list of skills as XML for inclusion in the
// system prompt. Returns an empty string if no skills are provided.
func formatSkillsPrompt(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<available_skills>\n")

	for _, s := range skills {
		b.WriteString("<skill>\n<name>")
		b.WriteString(escapeXML(s.Name))
		b.WriteString("</name>\n<description>")
		b.WriteString(escapeXML(s.Description))
		b.WriteString("</description>\n<location>")
		b.WriteString(escapeXML(s.FilePath))
		b.WriteString("</location>\n</skill>\n")
	}

	b.WriteString("</available_skills>")

	return b.String()
}
