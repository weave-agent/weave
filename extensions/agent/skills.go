package agent

import (
	"errors"
	"fmt"
	"io/fs"
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
	reservedNames  = []string{defaultProviderName, "claude"}
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
func discoverSkills(paths ...string) []Skill {
	seen := make(map[string]bool)

	var skills []Skill

	for _, root := range paths {
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}

			continue
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

	return skills
}

// isValidExtensionModule reports whether dir is a Go module directory containing
// both a go.mod file and at least one non-test .go file. This matches the
// validation performed by launcher.AutoDiscover.
func isValidExtensionModule(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		return false
	}

	found := false

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found {
			return nil //nolint:nilerr // skip problematic entries
		}

		if d.IsDir() {
			if path != dir {
				if _, statErr := os.Stat(filepath.Join(path, "go.mod")); statErr == nil {
					return fs.SkipDir
				}
			}

			return nil
		}

		name := d.Name()
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			found = true
		}

		return nil
	})

	return found
}

// discoverExtensionSkills scans extension directories for skills/ subdirectories.
// It checks project-local .weave/extensions/, global ~/.weave/extensions/, and
// built-in extensions/ relative to the module root. The scan is recursive so
// nested extensions are found. Only the first occurrence of each extension name
// contributes skills (precedence: project > global > built-in), matching the
// AutoDiscover selection semantics.
// Returns a list of <ext-dir>/skills/ paths.
func discoverExtensionSkills(projectDir, globalDir string) []string {
	var paths []string

	registered := make(map[string]bool)
	for _, name := range sdk.ListExtensions() {
		registered[name] = true
	}

	// Track which extension dirs have already been selected (first wins).
	selected := make(map[string]bool)

	scan := func(root string) {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil //nolint:nilerr // skip problematic entries
			}

			if !d.IsDir() || path == root {
				return nil
			}

			if strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}

			// Don't recurse into skills directories; they contain skill dirs, not extensions.
			if d.Name() == "skills" {
				return fs.SkipDir
			}

			name := d.Name()

			if !registered[name] || selected[name] {
				return nil
			}

			if !isValidExtensionModule(path) {
				return nil
			}

			skillsDir := filepath.Join(path, "skills")

			if info, err := os.Stat(skillsDir); err == nil && info.IsDir() {
				paths = append(paths, skillsDir)
			}

			selected[name] = true

			return nil
		})
	}

	if projectDir != "" {
		scan(filepath.Join(projectDir, ".weave", "extensions"))
	}

	if globalDir != "" {
		scan(filepath.Join(globalDir, "extensions"))
	}

	if moduleRoot := findModuleRoot(); moduleRoot != "" {
		scan(filepath.Join(moduleRoot, "extensions"))
	}

	return paths
}

// findModuleRoot returns the weave module root directory. It checks the
// WEAVE_MODULE_ROOT environment variable first (set by the launcher when
// exec-ing a generated binary), then walks up from the current working
// directory looking for a go.mod file that declares "module weave".
// Returns empty string if not found.
func findModuleRoot() string {
	if root := os.Getenv("WEAVE_MODULE_ROOT"); root != "" {
		return root
	}

	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			for line := range strings.SplitSeq(string(data), "\n") {
				line = strings.TrimSpace(line)

				if line == "" || strings.HasPrefix(line, "//") {
					continue
				}

				if name, ok := strings.CutPrefix(line, "module "); ok {
					if strings.TrimSpace(name) == "weave" {
						return dir
					}

					break
				}
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	return ""
}

// makeSkillHandler creates a slash command handler that pre-loads a skill's
// body into the conversation as an agent.prompt event. If the skill has
// AllowedTools set, the tool filter is saved and restricted for the duration
// of the skill's turn; the previous filter is restored when the turn ends.
func (a *AgentExtension) makeSkillHandler(skill Skill, bus sdk.Bus) func(args string) error {
	return func(args string) error {
		if len(skill.AllowedTools) > 0 {
			a.mu.Lock()
			a.savedToolFilter = sdk.GetToolFilter()

			sdk.SetToolFilter(skill.AllowedTools)

			a.skillFilterActive = true
			a.mu.Unlock()
		}

		body := skill.Body()

		var msg strings.Builder
		fmt.Fprintf(&msg, "<skill name=\"%s\" location=\"%s\">\n", escapeXML(skill.Name), escapeXML(skill.FilePath))
		fmt.Fprintf(&msg, "References are relative to %s.\n\n", escapeXML(skill.BaseDir))
		msg.WriteString("<skill_body trust=\"untrusted\">\n")
		msg.WriteString(body)
		msg.WriteString("\n</skill_body>\n</skill>")

		if args != "" {
			msg.WriteString("\n\n")
			msg.WriteString(args)
		}

		bus.Publish(sdk.NewEvent(TopicPrompt, msg.String()))

		return nil
	}
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")

	return s
}

// formatSkillsPrompt formats a list of skills as XML for inclusion in the
// system prompt. Returns an empty string if no enabled skills are provided.
func formatSkillsPrompt(skills []Skill) string {
	var enabled []Skill

	for _, s := range skills {
		if !s.DisableModelInvocation {
			enabled = append(enabled, s)
		}
	}

	if len(enabled) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<available_skills>\n")

	for _, s := range enabled {
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
