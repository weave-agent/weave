package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

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
